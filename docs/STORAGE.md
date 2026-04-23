# 存储

## 阶段划分

- 先引入 **Redis** 承载易失状态与路由。
- 后续引入 **PostgreSQL** 承载持久记录。

## Redis 职责

- 会话在线状态
- 房间到节点的路由
- 幂等令牌存储
- 重连快照元数据

## PostgreSQL 职责

- 用户资料
- 对局摘要
- 操作日志 / 回放
- 结算历史

## 规则

`internal/mahjong` 与 `internal/domain` **不得**依赖存储包。
