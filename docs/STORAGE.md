# 存储

## 阶段划分

- Phase 2 先引入 **etcd + Redis**：前者负责控制面，后者负责易失数据面。
- Phase 3 起引入 **PostgreSQL** 承载房间事件、对局摘要与结算历史。
- Phase 6 增加 PostgreSQL 备份恢复演练与 SLO 观测基线。

## etcd 职责

- 节点注册与租约心跳
- 房间归属与房间亲和路由
- `gate` / `lobby` / `room` 的服务发现

## Redis 职责

- 会话在线状态
- 房间到节点的只读路由缓存
- 幂等令牌存储
- 重连快照元数据

## Redis 键布局

- `lsp:session:{user_id}`：在线会话信息
- `lsp:idem:{scope}:{idempotency_key}`：幂等窗口缓存
- `lsp:room:snapmeta:{room_id}`：重连/快照元数据
- `lsp:route:room:{room_id}`：从 etcd 回填的房间路由缓存

## PostgreSQL 职责

- 对局摘要 `game_summaries`
- 房间事件日志 `room_events`（append-only，`seq` 单调递增）
- 结算历史 `settlements`
- 备份恢复演练入口见 [ADR-0026](adr/0026-postgres-backup-and-restore.md) 与 `deploy/ops/postgres-restore.md`。

### 事件游标（Phase 3）

- 稳定游标格式：`{room_id}:{seq}`，与 `room_events.seq` 对齐。
- `cluster.v1.RoomService.SnapshotRoom` 返回的快照游标用于 `StreamEvents.since_cursor` 重放边界，详见 [ADR-0014](adr/0014-reconnect-session-and-snapshot-cutover.md)。

## 规则

`internal/mahjong` 与 `internal/domain` **不得**依赖存储包。

## 权威关系

- 房间归属以 etcd 为准。
- Redis 中的房间路由仅是缓存，不得单独决定最终请求落点。

## Phase 5 运行时约定

- `snapmeta.round_json` 带 `schema_version`；恢复遇到未来版本时不继续反序列化进行中局面，而是降级到重新准备，避免启动阻断。
- WS 入口的进程内幂等缓存只保护当前进程快速重放；跨进程 `ApplyEvent.idempotency_key` 仍以 Redis 为准。
- Redis 与 PostgreSQL 关键操作会记录到 `lsp_storage_op_seconds{store,op,result}`，用于观察 p99 尾延迟。

## Phase 6 运维约定

- PostgreSQL 备份恢复演练的目标为 RPO 不超过 15 分钟、RTO 不超过 60 分钟。
- 恢复后需通过 `SnapshotRoom` 与 `StreamEvents` 校验目标房间可读性。
- 恢复报告只保留执行编号、耗时与校验结论，真实 DSN、对象存储签名 URL 与临时凭据不得进入 Git。
