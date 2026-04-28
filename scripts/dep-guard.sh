#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="${LSP_ROOT:-$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)}"
CONFIG_FILE="${ROOT_DIR}/.build/config.yaml"

if ! command -v yq >/dev/null 2>&1; then
  echo "yq is required" >&2
  exit 1
fi

tmp_file="$(mktemp)"
trap 'rm -f "${tmp_file}"' EXIT

yq -r '.deps.denylist[]' "${CONFIG_FILE}" >"${tmp_file}"

while IFS= read -r dep; do
  if [[ -z "${dep}" ]]; then
    continue
  fi
  if grep -R --fixed-strings --line-number --binary-files=without-match \
    --include='*.go' --include='go.mod' --include='go.sum' \
    "${dep}" "${ROOT_DIR}/go.mod" "${ROOT_DIR}/go.sum" "${ROOT_DIR}/internal" "${ROOT_DIR}/cmd" "${ROOT_DIR}/pkg" >/dev/null 2>&1; then
    echo "forbidden dependency detected: ${dep}" >&2
    exit 1
  fi
done <"${tmp_file}"
