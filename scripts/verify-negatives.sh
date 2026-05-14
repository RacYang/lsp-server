#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
RULES_DIR="${ROOT_DIR}/.claude/rules"

extract_field() {
  local file="$1"
  local field="$2"
  # .claude/rules/*.md 格式：参考节中 - **Enforcer**：`value` 或 - **负例**：`value`
  python3 -c "
import sys, re
text = open(sys.argv[1]).read()
# 匹配正文中的 - **Field**：\`value\`
m = re.search(rf'\*\*{sys.argv[2]}\*\*[：:]\s*\x60([^\x60]+)\x60', text)
if m:
    print(m.group(1))
" "${file}" "${field}"
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

go 1.26.3
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

go 1.26.3
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

run_proto_breaking_negative() {
  local negative_file="$1"
  local tmp_dir
  tmp_dir="$(mktemp -d)"
  trap 'rm -rf "${tmp_dir}"' RETURN
  mkdir -p "${tmp_dir}/api/proto/client/v1"
  cp "${ROOT_DIR}/buf.yaml" "${tmp_dir}/buf.yaml"
  cat >"${tmp_dir}/api/proto/client/v1/sample.proto" <<EOF
syntax = "proto3";

package client.v1;

option go_package = "negative.test/api/gen/go/client/v1;clientv1";

message BreakingSample {
  string stable_field = 1;
}
EOF
  (cd "${tmp_dir}" && git init -q && git add . && git -c user.name=negative -c user.email=negative@example.invalid commit -qm baseline)
  cp "${negative_file}" "${tmp_dir}/api/proto/client/v1/sample.proto"
  if (cd "${tmp_dir}" && buf breaking --against ".git#ref=HEAD") >/dev/null 2>&1; then
    fail_unexpected_pass "${negative_file}"
  fi
}

run_deps_negative() {
  local negative_file="$1"
  local tmp_dir
  tmp_dir="$(mktemp -d)"
  trap 'rm -rf "${tmp_dir}"' RETURN
  mkdir -p "${tmp_dir}/.build" "${tmp_dir}/cmd/negative" "${tmp_dir}/internal" "${tmp_dir}/pkg"
  cp "${ROOT_DIR}/.build/config.yaml" "${tmp_dir}/.build/config.yaml"
  cat >"${tmp_dir}/go.mod" <<EOF
module negative.test/sample

go 1.26.3

require github.com/topfreegames/pitaya v0.0.0
EOF
  touch "${tmp_dir}/go.sum"
  cp "${negative_file}" "${tmp_dir}/cmd/negative/main.go"
  if LSP_ROOT="${tmp_dir}" bash "${ROOT_DIR}/scripts/dep-guard.sh" >/dev/null 2>&1; then
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

run_log_boundaries_negative() {
  local negative_file="$1"
  if python3 "${ROOT_DIR}/scripts/verify-log-boundaries.py" --file "${negative_file}" >/dev/null 2>&1; then
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

run_redis_keys_negative() {
  local negative_file="$1"
  if python3 "${ROOT_DIR}/scripts/verify-redis-keys.py" --file "${negative_file}" >/dev/null 2>&1; then
    fail_unexpected_pass "${negative_file}"
  fi
}

run_metrics_naming_negative() {
  local negative_file="$1"
  if python3 "${ROOT_DIR}/scripts/verify-metrics-naming.py" --file "${negative_file}" >/dev/null 2>&1; then
    fail_unexpected_pass "${negative_file}"
  fi
}

run_cli_release_targets_negative() {
  local negative_file="$1"
  if python3 "${ROOT_DIR}/scripts/verify-cli-release-targets.py" --file "${negative_file}" >/dev/null 2>&1; then
    fail_unexpected_pass "${negative_file}"
  fi
}

run_mahjong_rule_id_negative() {
  local negative_file="$1"
  if python3 "${ROOT_DIR}/scripts/verify-mahjong-rule-ids.py" --file "${negative_file}" >/dev/null 2>&1; then
    fail_unexpected_pass "${negative_file}"
  fi
}

run_gate_session_routing_negative() {
  local negative_file="$1"
  if python3 "${ROOT_DIR}/scripts/verify-gate-session-routing.py" --file "${negative_file}" >/dev/null 2>&1; then
    fail_unexpected_pass "${negative_file}"
  fi
}

run_source_shape_negative() {
  local negative_file="$1"
  local check="$2"
  if python3 "${ROOT_DIR}/scripts/verify-source-shape.py" --check "${check}" --file "${negative_file}" >/dev/null 2>&1; then
    fail_unexpected_pass "${negative_file}"
  fi
}

run_claude_command_shape_negative() {
  local negative_file="$1"
  if python3 "${ROOT_DIR}/scripts/verify-skeleton.py" --command-file "${negative_file}" >/dev/null 2>&1; then
    fail_unexpected_pass "${negative_file}"
  fi
}

run_config_schema_negative() {
  local negative_file="$1"
  local tmp_dir
  tmp_dir="$(mktemp -d)"
  trap 'rm -rf "${tmp_dir}"' RETURN
  mkdir -p "${tmp_dir}/.build/schema"
  cp "${ROOT_DIR}/.build/schema/config.schema.json" "${tmp_dir}/.build/schema/config.schema.json"
  cp "${negative_file}" "${tmp_dir}/.build/config.yaml"
  if (cd "${tmp_dir}" && python3 "${ROOT_DIR}/scripts/verify-config-schema.py" --file "${tmp_dir}/.build/config.yaml") >/dev/null 2>&1; then
    fail_unexpected_pass "${negative_file}"
  fi
}

run_doc_link_negative() {
  local negative_file="$1"
  if python3 "${ROOT_DIR}/scripts/verify-doc-links.py" --file "${negative_file}" >/dev/null 2>&1; then
    fail_unexpected_pass "${negative_file}"
  fi
}

run_db_migration_negative() {
  local negative_file="$1"
  local tmp_dir
  tmp_dir="$(mktemp -d)"
  trap 'rm -rf "${tmp_dir}"' RETURN
  mkdir -p "${tmp_dir}/migrations"
  cp "${negative_file}" "${tmp_dir}/migrations/bad_migration.sql"
  # 需要提供一个会触发命名错误的文件名，直接复制到以原始名命名的文件
  local base_name
  base_name="$(basename "${negative_file}" .neg)"
  cp "${negative_file}" "${tmp_dir}/migrations/${base_name}"
  if python3 "${ROOT_DIR}/scripts/verify-postgres-migrations.py" --dir "${tmp_dir}/migrations" >/dev/null 2>&1; then
    fail_unexpected_pass "${negative_file}"
  fi
}

run_command_shape_negative() {
  local negative_file="$1"
  if python3 "${ROOT_DIR}/scripts/verify-skeleton.py" --command-file "${negative_file}" >/dev/null 2>&1; then
    fail_unexpected_pass "${negative_file}"
  fi
}

run_template_shape_negative() {
  local negative_file="$1"
  if python3 "${ROOT_DIR}/scripts/verify-skeleton.py" --manifest-file "${negative_file}" >/dev/null 2>&1; then
    fail_unexpected_pass "${negative_file}"
  fi
}

run_nolint_policy_negatives() {
  local negative_file
  for negative_file in "${ROOT_DIR}"/.build/negatives/nolint_policy_*.go.neg; do
    [[ -f "${negative_file}" ]] || continue
    if python3 "${ROOT_DIR}/scripts/verify-nolint-policy.py" --file "${negative_file}" >/dev/null 2>&1; then
      fail_unexpected_pass "${negative_file}"
    fi
  done
}

run_room_phase_owner_negative() {
  local negative_file="$1"
  local tmp_dir
  tmp_dir="$(mktemp -d)"
  trap 'rm -rf "${tmp_dir}"' RETURN
  mkdir -p "${tmp_dir}/internal/service/room"
  cp "${negative_file}" "${tmp_dir}/internal/service/room/sample.go"
  cd "${tmp_dir}"
  if python3 "${ROOT_DIR}/scripts/verify-room-phase-owner.py" >/dev/null 2>&1; then
    cd "${ROOT_DIR}"
    fail_unexpected_pass "${negative_file}"
  fi
  cd "${ROOT_DIR}"
}

for rule in "${RULES_DIR}"/*.md; do
  [[ -f "${rule}" ]] || continue
  enforcer="$(extract_field "${rule}" "Enforcer")"
  negative_rel="$(extract_field "${rule}" "负例")"
  negative_file="${ROOT_DIR}/${negative_rel}"

  if [[ ! -f "${negative_file}" ]]; then
    echo "missing negative file: ${negative_rel}" >&2
    exit 1
  fi

  case "${negative_rel}" in
    *proto_breaking_change*.proto.neg)
      run_proto_breaking_negative "${negative_file}"
      ;;
    *.proto.neg)
      run_proto_negative "${negative_file}"
      ;;
    *deps_forbidden_dependency*.go.neg)
      run_deps_negative "${negative_file}"
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
    *lang_log_english_message*.go.neg|*lang_log_literal_required_key*.go.neg|*lang_log_unknown_field_key*.go.neg|*lang_log_metric_like_key*.go.neg|*lang_log_pii_field_key*.go.neg)
      run_log_calls_negative "${negative_file}"
      ;;
    *lang_log_missing_boundary*.go.neg)
      run_log_boundaries_negative "${negative_file}"
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
    *redis_keys*.go.neg)
      run_redis_keys_negative "${negative_file}"
      ;;
    *metrics_bad_prefix*.go.neg)
      run_metrics_naming_negative "${negative_file}"
      ;;
    *release_cli_targets*.yaml.neg)
      run_cli_release_targets_negative "${negative_file}"
      ;;
    *mahjong_rule_id*.txt.neg)
      run_mahjong_rule_id_negative "${negative_file}"
      ;;
    *gate_session_routing*.go.neg)
      run_gate_session_routing_negative "${negative_file}"
      ;;
    *naming_go*.go.neg)
      run_source_shape_negative "${negative_file}" "naming"
      ;;
    *package_doc*.go.neg)
      run_source_shape_negative "${negative_file}" "package-doc"
      ;;
    *generated_marker*.go.neg)
      run_source_shape_negative "${negative_file}" "generated-marker"
      ;;
    *test_layout*.go.neg)
      run_source_shape_negative "${negative_file}" "test-layout"
      ;;
    *file_header*.sh.neg|*file_header*.py.neg)
      run_source_shape_negative "${negative_file}" "file-header"
      ;;
    *rule_shape*.mdc.neg)
      run_source_shape_negative "${negative_file}" "rule-shape"
      ;;
    *command_shape*.md.neg)
      run_command_shape_negative "${negative_file}"
      ;;
    *template_shape*.yaml.neg|*template_shape*.yml.neg)
      run_template_shape_negative "${negative_file}"
      ;;
    *nolint_policy*.go.neg)
      run_nolint_policy_negatives
      ;;
    *room_phase_owner*.go.neg)
      run_room_phase_owner_negative "${negative_file}"
      ;;
    *config_schema*.yaml.neg)
      run_config_schema_negative "${negative_file}"
      ;;
    *doc_link*.md.neg)
      run_doc_link_negative "${negative_file}"
      ;;
    *db_migration*.sql.neg)
      run_db_migration_negative "${negative_file}"
      ;;
    *claude_command_missing_desc*.md.neg)
      run_claude_command_shape_negative "${negative_file}"
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
