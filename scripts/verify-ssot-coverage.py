#!/usr/bin/env python3
"""校验 SSOT 策略域到规则的覆盖：每个策略域须至少有一条规则引用。

策略域即 .build/config.yaml 中定义可执行策略的顶层/二级键。
覆盖通过扫描规则正文中的 ADR 引用和领域关键词判定。
新增 SSOT 策略节时，须同时补规则和 enforcer，否则本脚本报缺。
"""

from __future__ import annotations

import pathlib
import re
import sys
from collections import defaultdict

ROOT = pathlib.Path(__file__).resolve().parents[1]
RULES_DIR = ROOT / ".claude" / "rules"

# 每个策略域的定义：(域名称, 触发关键词列表, 该域关联的 ADR 编号集合)
# 关键词在规则正文中命中任意一个即视为覆盖。ADR 引用命中任意一个也视为覆盖。
POLICY_DOMAINS: list[tuple[str, list[str], set[str]]] = [
    # Git 工作流 (ADR-0007)
    ("git 分支命名", ["git.branch", "分支命名", "topic_pattern"], {"0007"}),
    ("git 受保护分支推送", ["protected_push", "受保护分支", "non-fast-forward", "fast-forward"], {"0007"}),
    ("git 合并策略", ["git.merge", "squash", "合并策略", "merge commit"], {"0007"}),
    ("git 强推策略", ["git.history", "force.push", "强推", "force-with-lease"], {"0007"}),
    ("git 标签", ["git.tags", "release_pattern", "发布标签"], {"0007"}),
    ("git 提交 Trailer", ["trailer", "Made-with"], {"0007"}),
    ("git 仓库卫生", ["repo_hygiene", "仓库卫生", "forbidden_basenames", "binary_blocked"], {"0007"}),
    ("git 工作树卫生", ["workspace_space", "工作树卫生", "Finder 副本"], {"0007"}),
    ("git Hook 与 CI 映射", ["ci_parity", "hook.*CI", "pre-commit.*pre-push"], {"0007"}),
    ("git 提交签名", ["signing", "签名提交", "signed-commit"], {"0007"}),
    # 提交信息 (ADR-0000 / ADR-0004)
    ("提交信息格式", ["commit.*conventional", "提交信息", "type(scope)"], {"0000", "0004"}),
    # 语言与书写 (ADR-0004 / ADR-0005)
    ("文档中文", ["docs_paths", "文档.*中文", "cjk_ratio"], {"0004"}),
    ("注释中文", ["commenting", "注释.*中文"], {"0004", "0005"}),
    # 架构 (ADR-0000)
    ("架构边界", ["architecture-boundaries", "分层", "handler.*store"], {"0000"}),
    ("架构 lint", ["arch-lint", "go-arch-lint"], {"0000"}),
    # 日志 (ADR-0006 / ADR-0033 / ADR-0034 / ADR-0035)
    ("日志门面", ["facade_packages", "forbidden_packages", "直调", "slog.*zap", "logx"], {"0006"}),
    ("日志消息中文", ["message.*中文", "消息.*中文"], {"0004", "0006"}),
    ("日志字段命名", ["field_naming", "field_schema", "字段命名", "结构化字段"], {"0006"}),
    ("日志上下文边界", ["context_boundary", "WithTraceID", "Context 注入"], {"0006"}),
    ("日志上下文禁止手写", ["forbidden_literal", "不得手写.*trace_id", "字面量键名"], {"0006"}),
    ("日志 PII 脱敏", ["pii_redact", "敏感字段", "token.*password"], {"0033"}),
    ("日志与指标边界", ["forbidden_field_keys", "qps.*mailbox", "运行态字段.*指标"], {"0006", "0019"}),
    ("日志采样策略", ["sampling", "采样"], {"0033"}),
    ("日志动态级别", ["dynamic_level", "AtomicLevel", "动态级别"], {"0034"}),
    ("OpenTelemetry 桥接", ["otel", "OpenTelemetry", "OTel"], {"0035"}),
    # Proto (ADR-0012)
    ("Proto 基线", ["proto.baseline", "buf breaking", "向后兼容"], {"0012"}),
    ("Proto 规范", ["proto-standards", "package.*go_package"], {"0000", "0012"}),
    # 依赖 (ADR-0001)
    ("依赖策略", ["deps.denylist", "拒绝框架", "denylist"], {"0001"}),
    # 麻将 (ADR-0040 / ADR-0041)
    ("麻将规则纯净性", ["mahjong.purity", "不得.*session", "不得.*网络"], {"0040"}),
    ("麻将规则 ID", ["mahjong.rule.*id", "全拼音", "region_variant"], {"0041"}),
    # Redis (ADR-0010)
    ("Redis 键命名", ["redis.*key", "键名.*构造", "集中构造"], {"0010"}),
    # 指标 (ADR-0019)
    ("指标命名", ["metrics.*prefix", "lsp_", "指标.*命名空间"], {"0019"}),
    # 测试 (ADR-0000 / ADR-0043)
    ("测试规范", ["覆盖率", "cover.*threshold", "表驱动", "根因"], {"0000", "0043"}),
    # 持久化迁移
    ("数据库迁移", ["migration.*sql", "DDL", "迁移文件"], {"0000"}),
    # Lint 策略 (ADR-0036)
    ("nolint 策略", ["nolint:", "豁免.*linter"], {"0036"}),
    # Room 阶段所有权 (ADR-0045)
    ("Room 阶段所有权", ["phaseReason", "enterPhase", "enterPhase"], {"0045"}),
    # Gate 会话恢复 (ADR-0014)
    ("Gate 会话恢复", ["session_token", "AdvertiseAddr", "Resume"], {"0014"}),
    # 治理自身 (ADR-0042)
    ("治理元约束", ["governance.*meta", "rules_max_count", "commands_max_count", "entry_md"], {"0042"}),
    ("配置完整性", ["config.schema", "SSOT.*schema", "config_schema"], {"0000", "0042"}),
    ("文档链接完整性", ["doc-link", "交叉引用", "悬空链接"], {"0042"}),
    ("命令形态", ["command-shape", "When to use.*Inputs.*Steps.*Verify"], {"0042"}),
    ("规则形态", ["rule-shape", "负例.*三元组"], {"0042"}),
    ("模板形态", ["template-shape", "manifest.yaml.*用途"], {"0042"}),
    ("包注释", ["doc.go", "包注释"], {"0005"}),
    ("错误处理", ["error.*%w", "错误链", "errorlint"], {"0000"}),
    # 阶段与工具
    ("阶段门控", ["stage_gates", "cover_strict", "alpha.*beta.*ga"], {"0000"}),
    ("工具版本锁定", ["tools.*version", "verify-tools", "版本锁定"], {"0000"}),
    ("可观测规则一致性", ["observability.*rule", "ObserveStorage", "store.*op"], {"0019"}),
]


def extract_adr_refs(text: str) -> set[str]:
    """从规则正文提取所有引用的 ADR 编号。"""
    refs: set[str] = set()
    for m in re.finditer(r"docs/adr/(\d{4})-", text):
        refs.add(m.group(1))
    return refs


def main() -> int:
    all_rule_texts: dict[str, str] = {}
    for rule_file in sorted(RULES_DIR.glob("*.md")):
        all_rule_texts[rule_file.name] = rule_file.read_text()

    uncovered: list[str] = []
    for domain_name, keywords, adr_set in POLICY_DOMAINS:
        covered = False
        for rule_text in all_rule_texts.values():
            rule_adrs = extract_adr_refs(rule_text)
            if rule_adrs & adr_set:
                covered = True
                break
            for kw in keywords:
                if re.search(kw, rule_text, re.IGNORECASE):
                    covered = True
                    break
            if covered:
                break
        if not covered:
            uncovered.append(domain_name)

    if uncovered:
        print(f"SSOT 策略域缺少规则覆盖 ({len(uncovered)} 个):", file=sys.stderr)
        for domain in uncovered:
            print(f"  - {domain}", file=sys.stderr)
        print("\n请为以上域新增规则（/add-constraint）或为既有规则补充对应 ADR 引用。", file=sys.stderr)
        return 1

    return 0


if __name__ == "__main__":
    raise SystemExit(main())
