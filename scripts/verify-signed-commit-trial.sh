#!/usr/bin/env bash
set -uo pipefail

range="${1:-}"
if [[ -z "$range" ]]; then
	if [[ -n "${GITHUB_BASE_REF:-}" ]] && git rev-parse --verify "origin/${GITHUB_BASE_REF}" >/dev/null 2>&1; then
		base="$(git merge-base "origin/${GITHUB_BASE_REF}" HEAD)"
		range="${base}..HEAD"
	elif git rev-parse --verify HEAD^ >/dev/null 2>&1; then
		range="HEAD^..HEAD"
	else
		range="HEAD"
	fi
fi

commits=()
while IFS= read -r sha; do
	commits+=("$sha")
done < <(git rev-list --no-merges "$range" 2>/dev/null || true)
if [[ "${#commits[@]}" -eq 0 ]]; then
	commits=("HEAD")
fi

failed=0
for sha in "${commits[@]}"; do
	if git verify-commit "$sha" >/dev/null 2>&1; then
		echo "signed-commit-trial: $sha verified"
	else
		echo "::warning title=Signed commit trial::commit $sha has no trusted signature"
		failed=$((failed + 1))
	fi
done

if tag="$(git describe --exact-match --tags HEAD 2>/dev/null)"; then
	if git tag -v "$tag" >/dev/null 2>&1; then
		echo "signed-commit-trial: tag $tag verified"
	else
		echo "::warning title=Signed tag trial::tag $tag has no trusted signature"
		failed=$((failed + 1))
	fi
else
	echo "signed-commit-trial: HEAD is not tagged, skipping tag verification"
fi

echo "signed-commit-trial: warnings=$failed"
exit 0
