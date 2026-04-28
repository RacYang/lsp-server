#!/usr/bin/env bash
set -euo pipefail

scenario="${SCENARIO:-a}"
config="${CONFIG:-bench/scenarios/scenario_${scenario}/config.yaml}"
run_id="${RUN_ID:-$(date -u +%Y%m%dT%H%M%SZ)-${scenario}}"
out_dir="${OUT_DIR:-bench/runs/${run_id}}"

go run ./cmd/loadgen -scenario "${scenario}" -config "${config}" -out "${out_dir}"

echo "压测结果已写入 ${out_dir}"
