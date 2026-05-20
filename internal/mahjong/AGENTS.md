# internal/mahjong

本包只承载确定性玩法逻辑：牌理、动作合法性、番种计分、规则能力组合与终局结算。

## 硬约束

- 禁止 import 会话、传输、存储、集群、app、bot 或命令入口包。
- 规则 ID 格式固定为 `<region>_<variant>_<option>`，全部小写全拼音，禁止拼音首字母缩写和英文混写。
- 新玩法优先通过 `rules.CapabilitySet` 组合能力，不复制 room engine 逻辑。
- 新规则 ID 必须先加入 `.build/config.yaml` 的 `mahjong.rules.allowed_ids` 再注册。
- 跨进程恢复只通过 `rule_id`、通用局面字段和规则运行态 JSON。

## 相关

- **ADR**：`docs/adr/0040-composable-mahjong-rule-capabilities.md`、`docs/adr/0041-mahjong-rule-id-naming.md`
- **规则引擎**：`docs/RULE-ENGINE.md`
