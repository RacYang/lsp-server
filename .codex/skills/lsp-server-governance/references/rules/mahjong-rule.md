---
description: 麻将算法层必须保持纯净，规则 ID 必须使用全拼音命名
globs: ["internal/mahjong/**/*.go"]
---

# 麻将规则约束

- 麻将相关包仅包含确定性玩法逻辑。
- `internal/mahjong` 内不得出现网络、会话或存储关切。
- 新玩法优先通过 `rules.CapabilitySet` 组合开局、吃碰杠胡、轮转、计分、结算、终局与投影能力，不复制 room engine。
- 规则 ID 固定使用 `<region>_<variant>_<option>`，全部为小写全拼音，不使用拼音首字母缩写。
- 新玩法必须先进入 `.build/config.yaml` 的 `mahjong.rules.allowed_ids`，再注册到规则表。

---

- **ADR**：`docs/adr/0041-mahjong-rule-id-naming.md`
- **Enforcer**：`scripts/verify-mahjong-rule-ids.py`
- **负例**：`.build/negatives/mahjong_rule_id_abbreviation.txt.neg`
