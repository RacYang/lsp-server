#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
PROFILE_FILE="${1:-${ROOT_DIR}/coverage.out}"
CONFIG_FILE="${ROOT_DIR}/.build/config.yaml"

if [[ ! -f "${PROFILE_FILE}" ]]; then
  echo "coverage profile not found: ${PROFILE_FILE}" >&2
  exit 1
fi

thresholds_json="$(yq -o=json '.coverage.thresholds' "${CONFIG_FILE}")"
exclude_json="$(yq -o=json '.coverage.extra_exclude' "${CONFIG_FILE}")"
generated_marker="$(yq -r '.paths.generated_marker' "${CONFIG_FILE}")"

python3 - "$PROFILE_FILE" "$thresholds_json" "$exclude_json" "$generated_marker" <<'PY'
import fnmatch
import json
import pathlib
import sys

profile_path = pathlib.Path(sys.argv[1])
thresholds = json.loads(sys.argv[2])
extra_exclude = json.loads(sys.argv[3])
generated_marker = sys.argv[4]

def package_threshold(pkg: str) -> int:
    for pattern, value in thresholds.items():
        if pattern == "default":
            continue
        if fnmatch.fnmatch(pkg + "/", pattern.replace("...", "*")) or fnmatch.fnmatch(pkg, pattern.replace("/...", "")):
            return int(value)
    return int(thresholds["default"])

coverage = {}
with profile_path.open() as fh:
    next(fh)
    for line in fh:
        fields = line.strip().split()
        if len(fields) != 3:
            continue
        location, statements, count = fields
        file_path = location.split(":")[0]
        if generated_marker in file_path:
          continue
        if any(fnmatch.fnmatch(file_path, pattern) for pattern in extra_exclude):
          continue
        pkg = str(pathlib.Path(file_path).parent)
        bucket = coverage.setdefault(pkg, [0, 0])
        bucket[0] += int(statements)
        if int(count) > 0:
            bucket[1] += int(statements)

failed = False
for pkg, (total, covered) in sorted(coverage.items()):
    if total == 0:
        continue
    percent = covered * 100 / total
    threshold = package_threshold(pkg)
    if percent + 1e-9 < threshold:
        failed = True
        print(f"coverage below threshold for {pkg}: {percent:.2f}% < {threshold}%")

if failed:
    sys.exit(1)
PY
