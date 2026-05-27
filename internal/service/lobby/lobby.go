package lobby

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	_ "racoo.cn/lsp/internal/mahjong/builtin" // 注册内置麻将规则。
	"racoo.cn/lsp/internal/mahjong/rules"
	"racoo.cn/lsp/pkg/logx"
)

// RoomRegistry 提供大厅房间状态的持久化接口，供进程重启后恢复房间列表。
// 实现方负责序列化；nil 注册表退化为纯内存模式（与未配置 Redis 时等价）。
type RoomRegistry interface {
	UpsertRoom(ctx context.Context, rec RoomRecord) error
	DeleteRoom(ctx context.Context, roomID string) error
	ListAll(ctx context.Context) ([]RoomRecord, error)
}

// RoomRecord 是持久化到外部存储的房间完整快照，含座位分配。
type RoomRecord struct {
	RoomID      string           `json:"room_id"`
	NodeID      string           `json:"node_id"`
	RuleID      string           `json:"rule_id"`
	DisplayName string           `json:"display_name"`
	Private     bool             `json:"private"`
	CreatedAtMs int64            `json:"created_at_ms"`
	MaxSeats    int32            `json:"max_seats"`
	Seats       map[string]int32 `json:"seats"`
}

var (
	// ErrRoomNotFound 表示房间尚未创建或已被移除。
	ErrRoomNotFound = errors.New("room not found")
	// ErrRoomFull 表示房间 4 个座位已占满。
	ErrRoomFull = errors.New("room full")
	// ErrInvalidArgument 表示调用参数缺失。
	ErrInvalidArgument = errors.New("invalid argument")
)

const (
	defaultNodeID   = "room-local"
	defaultRuleID   = "sichuan_xuezhandaodi_huansanzhang"
	defaultMaxSeats = int32(4)
	waitingStage    = "waiting"
)

// RoomMeta 是大厅公开房间摘要；私密房仅可凭 room_id 手动加入。
type RoomMeta struct {
	RoomID      string
	RuleID      string
	DisplayName string
	SeatCount   int32
	MaxSeats    int32
	CreatedAtMs int64
	Stage       string
	RuleMeta    RuleMeta
}

// RuleMeta 是大厅可读的规则摘要；由麻将规则注册表统一投影。
type RuleMeta struct {
	RuleID          string
	DisplayName     string
	ShortDesc       string
	EnabledFeatures []string
	MaxHands        int32
}

type BotSeat struct {
	SeatIndex int32
	UserID    string
}

type roomMeta struct {
	ruleID      string
	displayName string
	private     bool
	createdAtMs int64
	maxSeats    int32
}

// Service 为大厅服务：维护房间到 room 节点映射与简单座位分配。
type Service struct {
	mu        sync.Mutex
	roomIDs   map[string]string
	seats     map[string]map[string]int32
	metas     map[string]roomMeta
	newRoomID func() (string, error)
	registry  RoomRegistry // 可空；非空时关键操作双写 Redis
}

// New 创建纯内存的大厅服务实例。
func New() *Service {
	return &Service{
		roomIDs:   make(map[string]string),
		seats:     make(map[string]map[string]int32),
		metas:     make(map[string]roomMeta),
		newRoomID: randomRoomID,
	}
}

// NewWithRegistry 创建带 Redis 持久化的大厅服务。
// 创建后应调用 RecoverFromRegistry 从已有记录恢复内存状态。
func NewWithRegistry(reg RoomRegistry) *Service {
	svc := New()
	svc.registry = reg
	return svc
}

// RecoverFromRegistry 从 RoomRegistry 恢复大厅内存状态；nil 注册表时为空操作。
// 应在服务开始接受请求前调用。
func (s *Service) RecoverFromRegistry(ctx context.Context) error {
	if s == nil || s.registry == nil {
		return nil
	}
	records, err := s.registry.ListAll(ctx)
	if err != nil {
		return fmt.Errorf("从注册表恢复大厅状态: %w", err)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, rec := range records {
		s.roomIDs[rec.RoomID] = rec.NodeID
		seats := make(map[string]int32, len(rec.Seats))
		for uid, seat := range rec.Seats {
			seats[uid] = seat
		}
		s.seats[rec.RoomID] = seats
		s.metas[rec.RoomID] = roomMeta{
			ruleID:      rec.RuleID,
			displayName: rec.DisplayName,
			private:     rec.Private,
			createdAtMs: rec.CreatedAtMs,
			maxSeats:    rec.MaxSeats,
		}
	}
	logx.Info(ctx, "大厅状态从注册表恢复完毕", "rooms", len(records))
	return nil
}

// CreateRoom 创建房间并绑定到 room-local；后续会由调度器/etcd claim 替换。
func (s *Service) CreateRoom(ctx context.Context, roomID string) (string, error) {
	if s == nil {
		return "", fmt.Errorf("nil lobby service")
	}
	if roomID == "" {
		return "", fmt.Errorf("%w: empty room_id", ErrInvalidArgument)
	}
	s.mu.Lock()
	if nodeID, ok := s.roomIDs[roomID]; ok {
		s.mu.Unlock()
		return nodeID, nil
	}
	s.ensureRoomLocked(roomID, defaultRuleID, "", false)
	rec, _ := s.buildRecordLocked(roomID)
	s.mu.Unlock()
	s.persistUpsert(ctx, rec)
	return defaultNodeID, nil
}

// CreateRoomWithMeta 创建带大厅元数据的房间，并让创建者直接占用 0 号座位。
func (s *Service) CreateRoomWithMeta(ctx context.Context, ruleID, displayName string, private bool, creatorUserID string) (string, int32, error) {
	if s == nil {
		return "", 0, fmt.Errorf("nil lobby service")
	}
	if creatorUserID == "" {
		return "", 0, fmt.Errorf("%w: empty creator_user_id", ErrInvalidArgument)
	}
	s.mu.Lock()
	roomID, err := s.allocateRoomIDLocked()
	if err != nil {
		s.mu.Unlock()
		return "", 0, err
	}
	s.ensureRoomLocked(roomID, ruleID, displayName, private)
	s.seats[roomID][creatorUserID] = 0
	rec, _ := s.buildRecordLocked(roomID)
	s.mu.Unlock()
	s.persistUpsert(ctx, rec)
	return roomID, 0, nil
}

// ListRooms 返回公开、未满且仍处于等待态的大厅房间。
func (s *Service) ListRooms(_ context.Context, pageSize int32, pageToken string) ([]RoomMeta, string, error) {
	if s == nil {
		return nil, "", fmt.Errorf("nil lobby service")
	}
	if pageSize <= 0 || pageSize > 100 {
		pageSize = 20
	}
	cursorCreatedAt, cursorRoomID, err := parsePageToken(pageToken)
	if err != nil {
		return nil, "", err
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	rooms := make([]RoomMeta, 0, len(s.roomIDs))
	for roomID, meta := range s.metas {
		seatCount := int32(len(s.seats[roomID])) //nolint:gosec // 房间座位数上限固定为 4
		if meta.private || seatCount >= meta.maxSeats || meta.stage() != waitingStage {
			continue
		}
		rooms = append(rooms, RoomMeta{
			RoomID:      roomID,
			RuleID:      normalizeRuleID(meta.ruleID),
			DisplayName: meta.displayName,
			SeatCount:   seatCount,
			MaxSeats:    meta.maxSeats,
			CreatedAtMs: meta.createdAtMs,
			Stage:       meta.stage(),
			RuleMeta:    ruleMeta(normalizeRuleID(meta.ruleID)),
		})
	}
	sort.Slice(rooms, func(i, j int) bool {
		if rooms[i].CreatedAtMs == rooms[j].CreatedAtMs {
			return rooms[i].RoomID < rooms[j].RoomID
		}
		return rooms[i].CreatedAtMs < rooms[j].CreatedAtMs
	})

	start := 0
	for start < len(rooms) && beforeOrEqualCursor(rooms[start], cursorCreatedAt, cursorRoomID) {
		start++
	}
	end := start + int(pageSize)
	if end > len(rooms) {
		end = len(rooms)
	}
	out := append([]RoomMeta(nil), rooms[start:end]...)
	if end >= len(rooms) {
		return out, "", nil
	}
	last := out[len(out)-1]
	return out, formatPageToken(last.CreatedAtMs, last.RoomID), nil
}

// ListRules 返回当前进程已注册且可用于创建房间的规则清单。
func (s *Service) ListRules(_ context.Context) ([]RuleMeta, error) {
	if s == nil {
		return nil, fmt.Errorf("nil lobby service")
	}
	registered := rules.List()
	out := make([]RuleMeta, 0, len(registered))
	for _, r := range registered {
		out = append(out, ruleMetaFromRule(r))
	}
	return out, nil
}

func ruleMeta(ruleID string) RuleMeta {
	ruleID = normalizeRuleID(ruleID)
	for _, r := range rules.List() {
		if r.ID() == ruleID {
			return ruleMetaFromRule(r)
		}
	}
	return RuleMeta{RuleID: ruleID}
}

func ruleMetaFromRule(r rules.Rule) RuleMeta {
	if r == nil {
		return RuleMeta{}
	}
	meta := rules.CapabilitiesOf(r).Metadata
	return RuleMeta{
		RuleID:          r.ID(),
		DisplayName:     meta.DisplayName,
		ShortDesc:       meta.ShortDesc,
		EnabledFeatures: append([]string(nil), meta.EnabledFeatures...),
		MaxHands:        meta.MaxHands,
	}
}

// AutoMatch 优先加入最早创建的公开未满房；没有候选时创建一个公开房。
func (s *Service) AutoMatch(ctx context.Context, ruleID, userID string) (string, int32, error) {
	if s == nil {
		return "", 0, fmt.Errorf("nil lobby service")
	}
	if userID == "" {
		return "", 0, fmt.Errorf("%w: empty user_id", ErrInvalidArgument)
	}
	ruleID = normalizeRuleID(ruleID)
	rooms, _, err := s.ListRooms(ctx, 100, "")
	if err != nil {
		return "", 0, err
	}
	for _, room := range rooms {
		if normalizeRuleID(room.RuleID) != ruleID {
			continue
		}
		seat, joinErr := s.JoinRoom(ctx, room.RoomID, userID)
		if joinErr == nil {
			return room.RoomID, seat, nil
		}
		if !errors.Is(joinErr, ErrRoomFull) {
			return "", 0, joinErr
		}
	}
	roomID, seat, err := s.CreateRoomWithMeta(ctx, ruleID, "", false, userID)
	if err != nil {
		return "", 0, err
	}
	return roomID, seat, nil
}

func (s *Service) ensureRoomLocked(roomID, ruleID, displayName string, private bool) {
	s.roomIDs[roomID] = defaultNodeID
	s.seats[roomID] = make(map[string]int32)
	if displayName == "" {
		displayName = roomID
	}
	s.metas[roomID] = roomMeta{
		ruleID:      normalizeRuleID(ruleID),
		displayName: displayName,
		private:     private,
		createdAtMs: time.Now().UnixMilli(),
		maxSeats:    defaultMaxSeats,
	}
}

// JoinRoom 为测试与基线阶段分配座位；重复加入返回原座位。
func (s *Service) JoinRoom(ctx context.Context, roomID, userID string) (int32, error) {
	if s == nil {
		return 0, fmt.Errorf("nil lobby service")
	}
	if roomID == "" || userID == "" {
		return 0, fmt.Errorf("%w: empty room_id or user_id", ErrInvalidArgument)
	}
	s.mu.Lock()
	if _, ok := s.roomIDs[roomID]; !ok {
		s.ensureRoomLocked(roomID, defaultRuleID, "", false)
	}
	if seat, ok := s.seats[roomID][userID]; ok {
		s.mu.Unlock()
		return seat, nil
	}
	used := make(map[int32]bool, len(s.seats[roomID]))
	for _, seat := range s.seats[roomID] {
		used[seat] = true
	}
	if len(used) >= 4 {
		s.mu.Unlock()
		return 0, ErrRoomFull
	}
	var seat int32
	for ; seat < 4; seat++ {
		if !used[seat] {
			break
		}
	}
	s.seats[roomID][userID] = seat
	rec, _ := s.buildRecordLocked(roomID)
	s.mu.Unlock()
	s.persistUpsert(ctx, rec)
	return seat, nil
}

// LeaveRoom 立即从大厅座位索引中移除用户，不等待 room actor 完成托管或结算。
func (s *Service) LeaveRoom(ctx context.Context, roomID, userID string) error {
	if s == nil {
		return fmt.Errorf("nil lobby service")
	}
	if roomID == "" || userID == "" {
		return fmt.Errorf("%w: empty room_id or user_id", ErrInvalidArgument)
	}
	s.mu.Lock()
	seats, ok := s.seats[roomID]
	if !ok {
		s.mu.Unlock()
		return ErrRoomNotFound
	}
	delete(seats, userID)
	rec, _ := s.buildRecordLocked(roomID)
	s.mu.Unlock()
	s.persistUpsert(ctx, rec)
	return nil
}

// AddBot 在等待态房间中分配机器人座位；真实出牌由上层 bot supervisor 驱动。
func (s *Service) AddBot(ctx context.Context, roomID string, count int32, maxBots int) ([]BotSeat, error) {
	if s == nil {
		return nil, fmt.Errorf("nil lobby service")
	}
	if roomID == "" || count <= 0 {
		return nil, fmt.Errorf("%w: invalid add bot request", ErrInvalidArgument)
	}
	if maxBots <= 0 {
		maxBots = 3
	}
	s.mu.Lock()
	if _, ok := s.roomIDs[roomID]; !ok {
		s.mu.Unlock()
		return nil, ErrRoomNotFound
	}
	used := make(map[int32]bool, len(s.seats[roomID]))
	bots := 0
	for userID, seat := range s.seats[roomID] {
		used[seat] = true
		if strings.HasPrefix(userID, "bot:") {
			bots++
		}
	}
	target := int(count)
	added := make([]BotSeat, 0, target)
	for len(added) < target && len(used) < 4 && bots < maxBots {
		var seat int32
		for ; seat < 4; seat++ {
			if !used[seat] {
				break
			}
		}
		userID := fmt.Sprintf("bot:%s:%d", roomID, seat)
		s.seats[roomID][userID] = seat
		used[seat] = true
		bots++
		added = append(added, BotSeat{SeatIndex: seat, UserID: userID})
	}
	rec, _ := s.buildRecordLocked(roomID)
	s.mu.Unlock()
	if len(added) > 0 {
		s.persistUpsert(ctx, rec)
	}
	return added, nil
}

// DeleteRoom 从大厅内存与持久化注册表中删除指定房间记录，用于 CreateRoom 两阶段提交失败时的回滚。
// 若房间不存在则静默返回（幂等）。
func (s *Service) DeleteRoom(ctx context.Context, roomID string) {
	if s == nil || roomID == "" {
		return
	}
	s.mu.Lock()
	delete(s.roomIDs, roomID)
	delete(s.seats, roomID)
	delete(s.metas, roomID)
	s.mu.Unlock()
	if s.registry != nil {
		delCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
		defer cancel()
		if err := s.registry.DeleteRoom(delCtx, roomID); err != nil {
			logx.Warn(logx.WithRoomID(ctx, roomID), "大厅房间持久化删除失败", "err", err.Error())
		}
	}
}

// GetRoom 查询房间归属节点。
func (s *Service) GetRoom(_ context.Context, roomID string) (string, error) {
	if s == nil {
		return "", fmt.Errorf("nil lobby service")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	nodeID, ok := s.roomIDs[roomID]
	if !ok {
		return "", ErrRoomNotFound
	}
	return nodeID, nil
}

// buildRecordLocked 在持锁状态下将指定房间的当前内存状态序列化为 RoomRecord。
// 调用方必须已持有 s.mu。
func (s *Service) buildRecordLocked(roomID string) (RoomRecord, bool) {
	meta, ok := s.metas[roomID]
	if !ok {
		return RoomRecord{}, false
	}
	nodeID := s.roomIDs[roomID]
	seats := make(map[string]int32, len(s.seats[roomID]))
	for uid, seat := range s.seats[roomID] {
		seats[uid] = seat
	}
	return RoomRecord{
		RoomID:      roomID,
		NodeID:      nodeID,
		RuleID:      meta.ruleID,
		DisplayName: meta.displayName,
		Private:     meta.private,
		CreatedAtMs: meta.createdAtMs,
		MaxSeats:    meta.maxSeats,
		Seats:       seats,
	}, true
}

// persistUpsert 将房间记录同步写入注册表；写失败仅记警告，不影响主流程。
// 写操作在互斥锁释放后执行，避免持锁期间阻塞。
func (s *Service) persistUpsert(ctx context.Context, rec RoomRecord) {
	if s.registry == nil {
		return
	}
	writeCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	if err := s.registry.UpsertRoom(writeCtx, rec); err != nil {
		logx.Warn(logx.WithRoomID(ctx, rec.RoomID), "大厅房间持久化写入失败", "err", err.Error())
	}
}

func (s *Service) allocateRoomIDLocked() (string, error) {
	for i := 0; i < 32; i++ {
		roomID, err := s.newRoomID()
		if err != nil {
			return "", err
		}
		if _, ok := s.roomIDs[roomID]; !ok {
			return roomID, nil
		}
	}
	return "", fmt.Errorf("allocate room id: %w", ErrInvalidArgument)
}

func randomRoomID() (string, error) {
	const alphabet = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789"
	buf := make([]byte, 6)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("random room id: %w", err)
	}
	for i, b := range buf {
		buf[i] = alphabet[int(b)%len(alphabet)]
	}
	return string(buf), nil
}

func normalizeRuleID(ruleID string) string {
	if strings.TrimSpace(ruleID) == "" {
		return defaultRuleID
	}
	return strings.TrimSpace(ruleID)
}

func (m roomMeta) stage() string {
	return waitingStage
}

func parsePageToken(token string) (int64, string, error) {
	token = strings.TrimSpace(token)
	if token == "" {
		return 0, "", nil
	}
	parts := strings.SplitN(token, "|", 2)
	if len(parts) != 2 {
		return 0, "", fmt.Errorf("%w: invalid page_token", ErrInvalidArgument)
	}
	createdAt, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil || parts[1] == "" {
		return 0, "", fmt.Errorf("%w: invalid page_token", ErrInvalidArgument)
	}
	return createdAt, parts[1], nil
}

func formatPageToken(createdAt int64, roomID string) string {
	return strconv.FormatInt(createdAt, 10) + "|" + roomID
}

func beforeOrEqualCursor(room RoomMeta, createdAt int64, roomID string) bool {
	if createdAt == 0 && roomID == "" {
		return false
	}
	if room.CreatedAtMs != createdAt {
		return room.CreatedAtMs < createdAt
	}
	return room.RoomID <= roomID
}
