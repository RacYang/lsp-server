---
title: lsp-cli 二进制分发
status: accepted
date: 2026-04-29
---

# ADR-0032 lsp-cli 二进制分发

## 状态

已采纳。本 ADR 定义 `lsp-cli` 玩家终端客户端的构建、归档与 GitHub Releases 发布方式。

## 背景

`lsp-cli` 最初通过 `go run ./cmd/cli` 启动，适合开发者调试，但不适合玩家试用：

1. 玩家本地不一定安装 Go 工具链。
2. macOS、Linux 与 Windows 需要不同二进制。
3. 发布产物需要校验和，便于下载后确认完整性。
4. 仓库已有 tag release 规范，但没有客户端二进制流水线。

## 决策

### 1. 发布目标

`release.cli_targets` 是发布目标的 SSOT，当前包含：

- `darwin/arm64`
- `darwin/amd64`
- `linux/amd64`
- `linux/arm64`
- `windows/amd64`

`Makefile` 的 `build-cli-all` 与 `.goreleaser.yaml` 的构建矩阵必须与该列表一致，并由 `scripts/verify-cli-release-targets.py` 校验。

### 2. 本地构建

仓库提供两个 Make 目标：

- `make build-cli`：构建当前平台二进制到 `dist/lsp-cli`。
- `make build-cli-all`：交叉构建五个平台产物并生成 `dist/SHA256SUMS`。

构建统一使用 `-trimpath` 与 `-ldflags "-s -w"`，并注入 `version`、`commit` 与 `buildDate`。

### 3. GitHub Releases

使用 GoReleaser 作为发布工具：

- tag 匹配现有 `git.tags.release_pattern` 时触发 `.github/workflows/release.yml`。
- release job 先执行 `make verify`，再执行 `goreleaser release --clean`。
- GoReleaser 生成 tar.gz/zip archive 与 SHA256 checksum。
- GitHub Release 默认为 draft，由维护者人工发布。

### 4. 签名边界

当前不启用代码签名、notarization 或 cosign artifact signing。Windows 首次运行可能出现 SmartScreen 提示，macOS 可能需要用户确认未签名二进制。若未来 `git.signing.policy` 升级到强制或需要公开分发给非技术用户，应另立 ADR 讨论发布产物签名。

## 后果

- 玩家可以直接下载 release artifact 或使用 `scripts/lsp-cli.sh` / `scripts/lsp-cli.ps1` 启动。
- 维护者可以在本地用 `make build-cli` 快速验证，也可以用 GoReleaser snapshot dry-run 检查归档。
- 发布目标新增或删除会触发 enforcer，避免 SSOT、Makefile 与 GoReleaser 配置漂移。
- `dist/` 仍为本地产物目录，不进入 Git 跟踪范围。
