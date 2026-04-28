#!/usr/bin/env bash
set -euo pipefail

namespace="${NAMESPACE:-lsp}"
service="${SERVICE:?SERVICE 必须为 gate、room 或 lobby}"
image_repo="${IMAGE_REPO:?IMAGE_REPO 必须指向镜像仓库}"
rollback_tag="${ROLLBACK_TAG:?ROLLBACK_TAG 必须匹配 vX.Y.Z}"
rollback_sha="${ROLLBACK_SHA:?ROLLBACK_SHA 必须为已验证的回滚版本短 SHA}"
dry_run="${DRY_RUN:-1}"

release_pattern='^v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(-[0-9A-Za-z.-]+)?$'

case "${service}" in
  gate|room|lobby) ;;
  *) echo "未知服务: ${service}" >&2; exit 1 ;;
esac

if [[ ! "${rollback_tag}" =~ ${release_pattern} ]]; then
  echo "回滚 tag 不符合 vX.Y.Z 规范: ${rollback_tag}" >&2
  exit 1
fi

if ! git tag -v "${rollback_tag}" >/dev/null 2>&1; then
  echo "回滚 tag 未通过签名校验: ${rollback_tag}" >&2
  exit 1
fi

deployment="lsp-${service}"
container="${service}"
image="${image_repo}/${service}:${service}-${rollback_sha}"

run() {
  if [[ "${dry_run}" == "1" ]]; then
    printf '[dry-run] %q ' "$@"
    printf '\n'
    return 0
  fi
  "$@"
}

run kubectl -n "${namespace}" set image "deployment/${deployment}" "${container}=${image}"
run kubectl -n "${namespace}" rollout status "deployment/${deployment}"
run kubectl -n "${namespace}" annotate "deployment/${deployment}" \
  "lsp.racoo.cn/rollback-tag=${rollback_tag}" \
  "lsp.racoo.cn/rollback-sha=${rollback_sha}" \
  "lsp.racoo.cn/hotfix-forbidden=true" \
  --overwrite

echo "${service} 已通过镜像 tag 回滚到 ${image}"
