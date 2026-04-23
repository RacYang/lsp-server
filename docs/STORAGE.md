# 存储

## 阶段划分

- Phase 2 先引入 **etcd + Redis**：前者负责控制面，后者负责易失数据面。
- 后续引入 **PostgreSQL** 承载持久记录。

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

- 用户资料
- 对局摘要
- 操作日志 / 回放
- 结算历史

## 规则

`internal/mahjong` 与 `internal/domain` **不得**依赖存储包。

## 权威关系

- 房间归属以 etcd 为准。
- Redis 中的房间路由仅是缓存，不得单独决定最终请求落点。
