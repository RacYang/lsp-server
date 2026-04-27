#!/usr/bin/env bash
# 聚合 pre-push 中的 git 语义校验：受保护分支与 tag；stdin 与 git pre-push 一致。
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
tmp_file="$(mktemp)"
trap 'rm -f "${tmp_file}"' EXIT
cat >"${tmp_file}"

bash "${ROOT_DIR}/scripts/verify-protected-branch-push.sh" <"${tmp_file}"
python3 "${ROOT_DIR}/scripts/verify-git-tags.py" <"${tmp_file}"
