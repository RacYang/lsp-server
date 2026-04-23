#!/usr/bin/env bash
# 校验向受保护分支的推送：须为 fast-forward（禁止非 FF 更新）。
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
CONFIG_FILE="${ROOT_DIR}/.build/config.yaml"

is_all_zero() {
  local sha="$1"
  [[ "$sha" =~ ^0+$ ]]
}

run_check() {
  local stdin_data="$1"
  local protected_csv
  protected_csv="$(yq -o=csv '.git.protected_branches | join(",")' "${CONFIG_FILE}")"
  IFS=',' read -r -a protected_arr <<<"${protected_csv}"

  while IFS= read -r line; do
    [[ -z "${line// }" ]] && continue
    read -r local_ref local_sha remote_ref remote_sha <<<"${line}"
    [[ -z "${local_ref}" ]] && continue

    if [[ "${remote_ref}" != refs/heads/* ]]; then
      continue
    fi

    local short_name="${remote_ref#refs/heads/}"
    local is_protected=0
    for b in "${protected_arr[@]}"; do
      if [[ "${short_name}" == "${b}" ]]; then
        is_protected=1
        break
      fi
    done
    if [[ "${is_protected}" -eq 0 ]]; then
      continue
    fi

    if is_all_zero "${local_sha}"; then
      continue
    fi

    if is_all_zero "${remote_sha}"; then
      continue
    fi

    if ! git merge-base --is-ancestor "${remote_sha}" "${local_sha}" 2>/dev/null; then
      echo "禁止向受保护分支 ${short_name} 进行非 fast-forward 推送" >&2
      return 1
    fi
  done <<<"${stdin_data}"
  return 0
}

if [[ "${1:-}" == "--file" ]]; then
  stdin_data="$(cat "$2")"
  if ! run_check "${stdin_data}"; then
    exit 1
  fi
  exit 0
fi

stdin_data="$(cat)"
if ! run_check "${stdin_data}"; then
  exit 1
fi
exit 0
