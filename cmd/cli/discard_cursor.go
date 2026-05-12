package main

// CursorMode 描述手牌光标当前可执行的提交语义。
//
// 当玩家轮到出牌时是 CursorModeSingle（选 1 张 Enter 提交）；
// 川麻换三张阶段是 CursorModeMulti3（Space 标记 3 张，Enter 提交）；
// 其他场景（等待别家、定缺、claim、结算）是 CursorModeNone，禁用所有手牌操作。
type CursorMode int

const (
	// CursorModeNone 当前不允许操作手牌（不是你的回合或不在 discard/exchange 阶段）。
	CursorModeNone CursorMode = iota
	// CursorModeSingle 单选模式：方向键移动光标，Enter 提交即出牌。
	CursorModeSingle
	// CursorModeMulti3 三选模式：Space 标记/取消，凑齐 3 张 Enter 提交。
	CursorModeMulti3
)

// HandCursor 是手牌区光标的有限状态机。
//
// 字段保持简单可序列化，便于：
//   - 渲染层只读消费 (table_render);
//   - 输入层做幂等转换 (table_screen 主循环);
//   - 测试用 require.Equal 直接比较状态。
//
// 注意 Marked 槽位即为对应 Hand 的索引。Hand 顺序变化（如服务端推 InitialDeal 重新发牌）
// 时调用方必须 Reset，避免索引错位。
type HandCursor struct {
	Mode             CursorMode
	Index            int   // 当前光标位置；-1 表示尚未选择
	Marked           []int // 已标记的手牌索引（仅 Multi 模式有效）
	Pending          bool  // 已提交,等待服务器 ack
	PendingSinceStep int64 // 提交时看到的服务端步进；权威步进前进后解除 Pending。
}

// DeriveCursorMode 根据玩家视图派生当前应该启用的光标模式。
//
// 逻辑只看 InteractionModel.Phase；更细粒度动作留给提交时校验。
//
// 注意：川麻血战的"换三张"是 4 家并发选择（不是轮流），服务端 broadcast 4 条
// ActionNotify(seat=0..3, action="exchange_three")，client 侧 ActingSeat 的语义
// 在该阶段不再是"轮到谁"。所以这里只要 SeatIndex 合法就给 CursorModeMulti3，
// 否则会出现"非庄家按任何键都没反应"的死锁。
func DeriveCursorMode(view RoomView) CursorMode {
	model := DeriveInteractionModel(view)
	switch model.Phase {
	case PhaseDiscard:
		if view.SeatIndex == view.ActingSeat {
			return CursorModeSingle
		}
	case PhaseExchange:
		if view.SeatIndex >= 0 && view.SeatIndex < 4 {
			return CursorModeMulti3
		}
	}
	return CursorModeNone
}

// Move 在水平方向移动光标，自动 clamp 到 [0, handLen-1]。
//
// Pending 状态下移动会被忽略，避免在等服务器 ack 期间产生本地状态漂移。
// 当 Index 还是初始的 -1 时，第一次移动会落到 0（最左侧）。
func (c *HandCursor) Move(delta int, handLen int) {
	if c.Mode == CursorModeNone || c.Pending || handLen <= 0 {
		return
	}
	if c.Index < 0 {
		if delta < 0 {
			c.Index = handLen - 1
		} else {
			c.Index = 0
		}
		return
	}
	next := c.Index + delta
	if next < 0 {
		next = 0
	}
	if next >= handLen {
		next = handLen - 1
	}
	c.Index = next
}

// SetIndex 直接跳到目标索引，多用于鼠标点击或数字键直选。
func (c *HandCursor) SetIndex(idx int, handLen int) {
	if c.Pending {
		return
	}
	if idx < 0 || idx >= handLen {
		return
	}
	c.Index = idx
}

// ToggleMark 在 Multi3 模式下把当前光标位置加入或移出 Marked。
//
// 标记数量上限 3：达到上限再尝试加入会返回 false 表示拒绝。
// Single 模式或 Pending 时一律 false。
func (c *HandCursor) ToggleMark() bool {
	if c.Mode != CursorModeMulti3 || c.Pending || c.Index < 0 {
		return false
	}
	for i, idx := range c.Marked {
		if idx == c.Index {
			c.Marked = append(c.Marked[:i], c.Marked[i+1:]...)
			return true
		}
	}
	if len(c.Marked) >= 3 {
		return false
	}
	c.Marked = append(c.Marked, c.Index)
	return true
}

// CanSubmit 报告当前是否可以发出网络请求。
//
//   - Single：光标必须落在合法索引；
//   - Multi3：恰好 3 张标记。
func (c *HandCursor) CanSubmit() bool {
	if c.Pending {
		return false
	}
	switch c.Mode {
	case CursorModeSingle:
		return c.Index >= 0
	case CursorModeMulti3:
		return len(c.Marked) == 3
	}
	return false
}

// Submit 把状态切到 Pending,等待服务器 ack。
//
// 调用方在 Submit 后应立即发起网络请求；服务器返回成功调 Reset，失败调 RollbackPending。
func (c *HandCursor) Submit() bool {
	return c.SubmitAt(0)
}

// SubmitAt 把状态切到 Pending，并记录提交时的权威步进。
//
// 如果服务端和 bot 在两次渲染之间连续推进，光标可能看不到中间的 CursorModeNone。
// 记录 step 后，SyncMode 可在下一次权威事件到达时解除 Pending，避免 Enter 永久被禁用。
func (c *HandCursor) SubmitAt(step int64) bool {
	if !c.CanSubmit() {
		return false
	}
	c.Pending = true
	c.PendingSinceStep = step
	return true
}

// RollbackPending 在服务器返回失败时恢复"可操作"状态，让玩家重选。
func (c *HandCursor) RollbackPending() {
	c.Pending = false
	c.PendingSinceStep = 0
}

// Cancel 在玩家按 Esc 时清空选择；Pending 状态下拒绝取消（已经送出，等服务端权威结果）。
func (c *HandCursor) Cancel() {
	if c.Pending {
		return
	}
	c.Index = -1
	c.Marked = c.Marked[:0]
}

// Reset 把光标完全重置（用于回合结束 / 阶段切换 / 服务器 ack 成功）。
func (c *HandCursor) Reset() {
	c.Index = -1
	c.Marked = c.Marked[:0]
	c.Pending = false
	c.PendingSinceStep = 0
}

// SyncMode 在阶段切换时把 Mode 更新为最新派生值；如果 Mode 改变则 Reset 清除旧索引。
//
// 维护一个共同的不变量:在任何"可操作"模式下,Index 都必须落在合法手牌范围内,
// 否则 Space (ToggleMark) / Enter (Submit) 会因 Index<0 静默无效,反复出现"按
// 键没反应"的疑惑。
//
// 派生策略:
//  1. 切入 Single 出牌模式 → Index = handLen-1（顺手出最右,一般是刚摸到的牌）。
//  2. 切入 Multi3 换三张模式 → Index = 0（光标立刻落在最左张,Space 即可标记）;
//     仍然不替玩家自动 Mark,Mark 操作必须由玩家显式按 Space 触发。
//  3. 同模式下手牌长度变化（如自杠后摸新牌、自摸阶段补牌）→ 把越界的旧 Index
//     clamp 回末位;手牌瞬时为空 → 退到 -1。
func (c *HandCursor) SyncMode(view RoomView) {
	mode := DeriveCursorMode(view)
	handLen := 0
	if view.SeatIndex >= 0 && view.SeatIndex < 4 {
		handLen = len(view.Players[view.SeatIndex].Hand)
	}
	if c.Pending && c.PendingSinceStep > 0 && view.LastStep > c.PendingSinceStep {
		c.Pending = false
		c.PendingSinceStep = 0
	}
	if mode != c.Mode {
		c.Reset()
		c.Mode = mode
		if handLen > 0 {
			switch mode {
			case CursorModeSingle:
				// [D1.2] 切入出牌单选时，光标默认停在「刚摸到的那张牌」在排序后手牌中的位置。
				// 当 view.PendingTile 已被服务端权威下发且仍在手牌中时优先使用它定位；
				// 否则回退到最右一张（顺手出最右是 cli 长期约定，仅在没有摸牌信息时使用）。
				c.Index = handLen - 1
				if idx := indexOfPendingDrawTile(view, handLen); idx >= 0 {
					c.Index = idx
				}
			case CursorModeMulti3:
				c.Index = 0
			}
		}
		return
	}
	if mode == CursorModeSingle || mode == CursorModeMulti3 {
		switch {
		case handLen <= 0:
			c.Index = -1
		case c.Index < 0:
			switch mode {
			case CursorModeSingle:
				c.Index = handLen - 1
			case CursorModeMulti3:
				c.Index = 0
			}
		case c.Index >= handLen:
			c.Index = handLen - 1
		}
	}
}

// indexOfPendingDrawTile 在自家排序后手牌里找 view.PendingTile 的索引；
// 找不到返回 -1。仅对 SeatIndex==ActingSeat 且 PendingTile 非空时尝试匹配，
// 避免抢答 pending 牌（如他家被点的炮牌）误把光标移走。
func indexOfPendingDrawTile(view RoomView, handLen int) int {
	if view.PendingTile == "" || view.SeatIndex < 0 || view.SeatIndex > 3 {
		return -1
	}
	if view.SeatIndex != view.ActingSeat {
		return -1
	}
	hand := view.Players[view.SeatIndex].Hand
	for i := handLen - 1; i >= 0; i-- {
		if i < len(hand) && hand[i] == view.PendingTile {
			return i
		}
	}
	return -1
}

// IsMarked 判断索引是否在已标记集合中（渲染层用于决定是否凸起 / 染色）。
func (c *HandCursor) IsMarked(idx int) bool {
	for _, m := range c.Marked {
		if m == idx {
			return true
		}
	}
	return false
}
