# AGENTS

请遵守本仓库的治理流水线：

1. 治理源变更请编辑 `.build/config.yaml`。
2. 派生产物请运行 `make generate`。
3. 在完成实质性工作前运行 `make verify`。
4. 保持麻将逻辑与传输层、存储层代码隔离。
5. 文档、注释与日志 message 以中文为主（见 ADR-0004～0006 与 `make verify-lang`）。
6. Git 工作流见 [ADR-0007](docs/adr/0007-git-workflow-policy.md)：在 `main` 外使用 `feat/`、`fix/` 等 topic 分支命名；不要依赖 `--no-verify` 跳过 hook，除非人类维护者明确授权。
