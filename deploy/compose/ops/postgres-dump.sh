#!/usr/bin/env bash
# 单机 Compose 形态下的 PostgreSQL 全量备份占位脚本。
# 备份产物不进入 Git；运维侧负责把 ${BACKUP_DIR} 转存到对象存储或异地磁盘。
# 详见 ADR-0026 与 ADR-0030。
set -euo pipefail

backup_dir="${BACKUP_DIR:?BACKUP_DIR 必须指向宿主机绝对路径}"
service="${POSTGRES_SERVICE:-postgres}"
db="${POSTGRES_DB:?POSTGRES_DB 必填}"
user="${POSTGRES_USER:?POSTGRES_USER 必填}"
ts="$(date -u +%Y%m%dT%H%M%SZ)"
out="${backup_dir%/}/lsp-${db}-${ts}.sql.gz"

mkdir -p "${backup_dir}"

docker compose exec -T "${service}" pg_dump \
  --username "${user}" \
  --dbname "${db}" \
  --format=plain \
  --no-owner \
  --no-privileges \
| gzip -c > "${out}"

echo "备份完成: ${out}"
echo "请按 ADR-0026 周期把备份转存到对象存储或异地磁盘。"
