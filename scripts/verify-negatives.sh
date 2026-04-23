#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
RULES_DIR="${ROOT_DIR}/.cursor/rules"

extract_field() {
  local file="$1"
  local field="$2"
  awk '
    BEGIN { in_fm = 0 }
    NR == 1 && $0 == "---" { in_fm = 1; next }
    in_fm && $0 == "---" { exit }
    in_fm { print }
  ' "${file}" | yq -r ".${field} // \"\"" -
}

fail_unexpected_pass() {
  echo "negative sample unexpectedly passed: $1" >&2
  exit 1
}

run_golangci_negative() {
  local negative_file="$1"
  local enforcer="$2"
  local tmp_dir
  tmp_dir="$(mktemp -d)"
  trap 'rm -rf "${tmp_dir}"' RETURN

  cat >"${tmp_dir}/go.mod" <<EOF
module negative.test/sample

go 1.26.2
EOF
  mkdir -p "${tmp_dir}/sample"
  cp "${negative_file}" "${tmp_dir}/sample/main.go"

  local linters
  linters="${enforcer#*#}"
  linters="${linters#\{}"
  linters="${linters%\}}"
  if (cd "${tmp_dir}" && golangci-lint run --disable-all --enable "${linters}" ./...) >/dev/null 2>&1; then
    fail_unexpected_pass "${negative_file}"
  fi
}

run_arch_negative() {
  local negative_file="$1"
  local target_dir="$2"
  local imported_dir="$3"
  local tmp_dir
  tmp_dir="$(mktemp -d)"
  trap 'rm -rf "${tmp_dir}"' RETURN

  cat >"${tmp_dir}/go.mod" <<EOF
module racoo.cn/lsp

go 1.26.2
EOF
  mkdir -p "${tmp_dir}/internal/${target_dir}" "${tmp_dir}/internal/${imported_dir}"
  cat >"${tmp_dir}/internal/${imported_dir}/stub.go" <<EOF
package $(basename "${imported_dir}")

type Stub struct{}
EOF
  cp "${negative_file}" "${tmp_dir}/internal/${target_dir}/negative.go"
  cat >"${tmp_dir}/.go-arch-lint.yml" <<EOF
version: 3
workdir: .
allow:
  depOnAnyVendor: true
components:
  ${target_dir}: { in: internal/${target_dir}/** }
  ${imported_dir}: { in: internal/${imported_dir}/** }
deps:
  ${target_dir}:
    anyVendorDeps: true
  ${imported_dir}:
    anyProjectDeps: true
EOF
  if (cd "${tmp_dir}" && go-arch-lint check) >/dev/null 2>&1; then
    fail_unexpected_pass "${negative_file}"
  fi
}

run_proto_negative() {
  local negative_file="$1"
  local tmp_dir
  tmp_dir="$(mktemp -d)"
  trap 'rm -rf "${tmp_dir}"' RETURN
  mkdir -p "${tmp_dir}/api/proto/client"
  cp "${ROOT_DIR}/buf.yaml" "${tmp_dir}/buf.yaml"
  cp "${negative_file}" "${tmp_dir}/api/proto/client/negative.proto"
  if (cd "${tmp_dir}" && buf lint ./...) >/dev/null 2>&1; then
    fail_unexpected_pass "${negative_file}"
  fi
}

run_commit_negative() {
  local negative_file="$1"
  if python3 "${ROOT_DIR}/scripts/verify-commit-msg.py" "${negative_file}" >/dev/null 2>&1; then
    fail_unexpected_pass "${negative_file}"
  fi
}

run_lang_docs_negative() {
  local negative_file="$1"
  if python3 "${ROOT_DIR}/scripts/verify-lang-docs.py" --file "${negative_file}" >/dev/null 2>&1; then
    fail_unexpected_pass "${negative_file}"
  fi
}

run_lang_comments_negative() {
  local negative_file="$1"
  if python3 "${ROOT_DIR}/scripts/verify-lang-comments.py" --file "${negative_file}" >/dev/null 2>&1; then
    fail_unexpected_pass "${negative_file}"
  fi
}

run_no_direct_logging_negative() {
  local negative_file="$1"
  if python3 "${ROOT_DIR}/scripts/verify-no-direct-logging.py" --file "${negative_file}" >/dev/null 2>&1; then
    fail_unexpected_pass "${negative_file}"
  fi
}

run_log_calls_negative() {
  local negative_file="$1"
  if python3 "${ROOT_DIR}/scripts/verify-log-calls.py" --file "${negative_file}" >/dev/null 2>&1; then
    fail_unexpected_pass "${negative_file}"
  fi
}

run_git_branch_negative() {
  local negative_file="$1"
  if python3 "${ROOT_DIR}/scripts/verify-branch-name.py" --file "${negative_file}" >/dev/null 2>&1; then
    fail_unexpected_pass "${negative_file}"
  fi
}

run_git_protected_push_negative() {
  local negative_file="$1"
  if bash "${ROOT_DIR}/scripts/verify-protected-branch-push.sh" --file "${negative_file}" >/dev/null 2>&1; then
    fail_unexpected_pass "${negative_file}"
  fi
}

run_git_tag_negative() {
  local negative_file="$1"
  if python3 "${ROOT_DIR}/scripts/verify-git-tags.py" --file "${negative_file}" >/dev/null 2>&1; then
    fail_unexpected_pass "${negative_file}"
  fi
}

run_git_repo_hygiene_negative() {
  local negative_file="$1"
  if python3 "${ROOT_DIR}/scripts/verify-repo-hygiene.py" --file "${negative_file}" >/dev/null 2>&1; then
    fail_unexpected_pass "${negative_file}"
  fi
}

run_git_hooks_parity_negative() {
  local negative_file="$1"
  if python3 "${ROOT_DIR}/scripts/verify-hooks-parity.py" --file "${negative_file}" >/dev/null 2>&1; then
    fail_unexpected_pass "${negative_file}"
  fi
}

for rule in "${RULES_DIR}"/*.mdc; do
  [[ -f "${rule}" ]] || continue
  kind="$(extract_field "${rule}" "kind")"
  [[ "${kind}" == "constraint" ]] || continue
  enforcer="$(extract_field "${rule}" "enforcer")"
  negative_rel="$(extract_field "${rule}" "negative_test")"
  negative_file="${ROOT_DIR}/${negative_rel}"

  if [[ ! -f "${negative_file}" ]]; then
    echo "missing negative file: ${negative_rel}" >&2
    exit 1
  fi

  case "${negative_rel}" in
    *.proto.neg)
      run_proto_negative "${negative_file}"
      ;;
    *commit*.neg)
      run_commit_negative "${negative_file}"
      ;;
    *lang_docs*.md.neg)
      run_lang_docs_negative "${negative_file}"
      ;;
    *arch*.go.neg)
      run_arch_negative "${negative_file}" "handler" "store"
      ;;
    *mahjong*.go.neg)
      run_arch_negative "${negative_file}" "mahjong" "session"
      ;;
    *lang_direct_slog*.go.neg)
      run_no_direct_logging_negative "${negative_file}"
      ;;
    *lang_log_english_message*.go.neg)
      run_log_calls_negative "${negative_file}"
      ;;
    *lang_code_english_comment*.go.neg)
      run_lang_comments_negative "${negative_file}"
      ;;
    *git_branch*.txt.neg)
      run_git_branch_negative "${negative_file}"
      ;;
    *git_protected_branch_push*.txt.neg)
      run_git_protected_push_negative "${negative_file}"
      ;;
    *git_tag*.txt.neg)
      run_git_tag_negative "${negative_file}"
      ;;
    *git_repo_hygiene*.txt.neg)
      run_git_repo_hygiene_negative "${negative_file}"
      ;;
    *git_hooks_parity*.yml.neg|*git_hooks_parity*.yaml.neg)
      run_git_hooks_parity_negative "${negative_file}"
      ;;
    *.go.neg)
      run_golangci_negative "${negative_file}" "${enforcer}"
      ;;
    *)
      echo "unsupported negative sample type: ${negative_rel}" >&2
      exit 1
      ;;
  esac
done
