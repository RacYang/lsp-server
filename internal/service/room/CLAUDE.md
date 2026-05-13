# internal/service/room

房间服务层：管理局面状态机、阶段切换、deadline 所有权、PhaseToken 校验与持久化快照。

## 硬约束

- `RoundState.phaseReason` 与 `RoundState.phaseStartUnixMs` 只允许在 `phase.go::enterPhase` 内直接赋值。
- 持久化恢复路径（`engine_persist.go::buildRoundStateFromPersist` / `finalizeRoundInvariants`）属于"重新加载快照"语义，视作 `enterPhase` 的等价初始化。
- engine、scheduler、actor 与其它分支必须改为调用 `rs.enterPhase(reason)`，不得绕过。
- `PhaseToken` 由 room 生成并持有，gate 透传，客户端操作携带以防止阶段竞态。

## 相关

- **ADR**：`docs/adr/0045-phase-deadline-single-owner-and-phase-token.md`
- **房间 FSM**：`docs/ROOM-FSM.md`
