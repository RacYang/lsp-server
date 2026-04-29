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
)

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
	defaultRuleID   = "sichuan_xzdd"
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
}

// New 创建大厅服务实例。
func New() *Service {
	return &Service{
		roomIDs:   make(map[string]string),
		seats:     make(map[string]map[string]int32),
		metas:     make(map[string]roomMeta),
		newRoomID: randomRoomID,
	}
}

// CreateRoom 创建房间并绑定到 room-local；后续会由调度器/etcd claim 替换。
func (s *Service) CreateRoom(_ context.Context, roomID string) (string, error) {
	if s == nil {
		return "", fmt.Errorf("nil lobby service")
	}
	if roomID == "" {
		return "", fmt.Errorf("%w: empty room_id", ErrInvalidArgument)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if nodeID, ok := s.roomIDs[roomID]; ok {
		return nodeID, nil
	}
	s.ensureRoomLocked(roomID, defaultRuleID, "", false)
	return defaultNodeID, nil
}

// CreateRoomWithMeta 创建带大厅元数据的房间，并让创建者直接占用 0 号座位。
func (s *Service) CreateRoomWithMeta(_ context.Context, ruleID, displayName string, private bool, creatorUserID string) (string, int32, error) {
	if s == nil {
		return "", 0, fmt.Errorf("nil lobby service")
	}
	if creatorUserID == "" {
		return "", 0, fmt.Errorf("%w: empty creator_user_id", ErrInvalidArgument)
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	roomID, err := s.allocateRoomIDLocked()
	if err != nil {
		return "", 0, err
	}
	s.ensureRoomLocked(roomID, ruleID, displayName, private)
	s.seats[roomID][creatorUserID] = 0
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
func (s *Service) JoinRoom(_ context.Context, roomID, userID string) (int32, error) {
	if s == nil {
		return 0, fmt.Errorf("nil lobby service")
	}
	if roomID == "" || userID == "" {
		return 0, fmt.Errorf("%w: empty room_id or user_id", ErrInvalidArgument)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.roomIDs[roomID]; !ok {
		s.ensureRoomLocked(roomID, defaultRuleID, "", false)
	}
	if seat, ok := s.seats[roomID][userID]; ok {
		return seat, nil
	}
	seatCount := len(s.seats[roomID])
	if seatCount >= 4 {
		return 0, ErrRoomFull
	}
	seat := int32(seatCount) //nolint:gosec // 最大仅 0..3，已由上方边界限制
	s.seats[roomID][userID] = seat
	return seat, nil
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
