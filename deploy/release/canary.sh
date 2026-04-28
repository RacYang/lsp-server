#!/usr/bin/env bash
set -euo pipefail

namespace="${NAMESPACE:-lsp}"
service="${SERVICE:?SERVICE 必须为 gate、room 或 lobby}"
image_repo="${IMAGE_REPO:?IMAGE_REPO 必须指向镜像仓库}"
release_tag="${RELEASE_TAG:?RELEASE_TAG 必须匹配 vX.Y.Z}"
git_short_sha="${GIT_SHORT_SHA:-$(git rev-parse --short HEAD)}"
dry_run="${DRY_RUN:-1}"

release_pattern='^v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(-[0-9A-Za-z.-]+)?$'

case "${service}" in
  gate|room|lobby) ;;
  *) echo "未知服务: ${service}" >&2; exit 1 ;;
esac

if [[ ! "${release_tag}" =~ ${release_pattern} ]]; then
  echo "release tag 不符合 vX.Y.Z 规范: ${release_tag}" >&2
  exit 1
fi

if ! git tag -v "${release_tag}" >/dev/null 2>&1; then
  echo "release tag 未通过签名校验: ${release_tag}" >&2
  exit 1
fi

deployment="lsp-${service}"
container="${service}"
image="${image_repo}/${service}:${service}-${git_short_sha}"

run() {
  if [[ "${dry_run}" == "1" ]]; then
    printf '[dry-run] %q ' "$@"
    printf '\n'
    return 0
  fi
  "$@"
}

observe() {
  local step="$1"
  echo "观察 ${step}：请确认 SLO 大盘无 page 告警，默认窗口 30 分钟"
  if [[ "${dry_run}" != "1" ]]; then
    sleep "${OBSERVE_SECONDS:-1800}"
  fi
}

run kubectl -n "${namespace}" set image "deployment/${deployment}" "${container}=${image}"
run kubectl -n "${namespace}" rollout status "deployment/${deployment}"
run kubectl -n "${namespace}" annotate "deployment/${deployment}" \
  "lsp.racoo.cn/release-tag=${release_tag}" \
  "lsp.racoo.cn/build-sha=${git_short_sha}" \
  --overwrite

for step in "金丝雀" "10%" "50%" "100%"; do
  echo "进入 ${service} ${step} 放量"
  run kubectl -n "${namespace}" annotate "deployment/${deployment}" "lsp.racoo.cn/canary-step=${step}" --overwrite
  observe "${step}"
done

echo "${service} 灰度发布完成: ${image}"
