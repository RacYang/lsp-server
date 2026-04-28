# PostgreSQL 备份恢复演练

本文是 ADR-0026 的执行 runbook，用于预发与生产事故演练。真实备份文件、WAL 归档与对象存储地址不得提交到 Git。

## 前置条件

- 已确认目标环境的 PostgreSQL 全量备份与 WAL 归档可读取。
- 已准备临时 PostgreSQL 实例，实例网络只允许 `room` 演练进程访问。
- 已准备只读校验用的 `room_id`、目标恢复时间点与预期事件游标。
- 已准备临时 `room` 配置文件，只替换 `postgres.dsn` 指向临时实例。

## 恢复步骤

1. 冻结演练窗口内的目标房间写入，记录冻结时间与目标恢复时间点。
2. 从最新全量备份恢复到临时 PostgreSQL 实例。
3. 回放 WAL 到目标时间点，确认 `room_events` 与 `game_summaries` schema 版本可读。
4. 启动临时 `room` 进程，`postgres.dsn` 指向临时实例，Redis 与 etcd 指向预发隔离资源。
5. 调用 `cluster.v1.RoomService.SnapshotRoom`，确认快照能返回座位、阶段与最近事件游标。
6. 调用 `cluster.v1.RoomService.StreamEvents`，从快照游标继续读取事件，确认无断档与重复。
7. 关闭临时 `room` 进程，销毁临时 PostgreSQL 实例。
8. 输出演练报告，记录 RPO、RTO、数据缺口、异常、责任人与下一次演练日期。

## 报告模板

```text
run_id:
environment:
backup_source:
target_restore_time:
full_backup_started_at:
full_backup_finished_at:
wal_replay_finished_at:
room_validation_finished_at:
rpo_minutes:
rto_minutes:
validated_room_ids:
snapshot_room_result:
stream_events_result:
data_gap:
incidents:
next_drill_due:
```

## 通过标准

- RPO 不超过 15 分钟。
- RTO 不超过 60 分钟。
- `SnapshotRoom` 与 `StreamEvents` 均能读取目标房间。
- 恢复演练报告不包含真实 DSN、密码、对象存储签名 URL 或其它凭据。

## 频率

- 生产环境每月至少一次恢复演练。
- 预发环境在 schema、存储或部署脚本发生重大变化后补跑一次。
