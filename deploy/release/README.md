# 灰度发布与回滚

发布顺序固定为 `gate` 先行、`room` 随后、`lobby` 最后。`gate` 与 `room` 必须分开放量，避免连接层与房间事件循环同时变化。

## 灰度

```bash
SERVICE=gate \
IMAGE_REPO=registry.example.com/lsp \
RELEASE_TAG=v1.2.3 \
DRY_RUN=0 \
bash deploy/release/canary.sh
```

脚本放量阶段为金丝雀、10%、50%、100%。每档默认观察 30 分钟，可通过 `OBSERVE_SECONDS` 在预发环境缩短。

## 回滚

```bash
SERVICE=gate \
IMAGE_REPO=registry.example.com/lsp \
ROLLBACK_TAG=v1.2.2 \
ROLLBACK_SHA=abc1234 \
DRY_RUN=0 \
bash deploy/release/rollback.sh
```

回滚只能通过镜像 tag 切换完成，禁止原地修改代码热修。

## 硬回滚条件

- SLO 燃尽预算超过 25%。
- `severity=page` 告警持续 5 分钟。
- `lsp_rate_limited_total` 或 `lsp_actor_queue_depth` 陡升。
- 未知消息率或重连失败率在一个观察窗口内持续上升。
