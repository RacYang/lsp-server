---
description: PostgreSQL 迁移文件须遵循命名、编号连续与幂等 DDL 约束
globs: ["internal/store/postgres/migrations/*.sql"]
---

# 数据库迁移规范

- 迁移文件命名格式为 `NNN_descriptive_name.sql`，编号从 001 连续递增。
- 禁止在迁移中使用裸 `DROP TABLE`、`DROP COLUMN`、`TRUNCATE`、`ALTER TYPE`。
- `CREATE TABLE`、`CREATE INDEX`、`ADD COLUMN` 必须使用 `IF NOT EXISTS` 前缀，确保迁移幂等可重放。

---

- **ADR**：`docs/adr/0000-engineering-charter.md`
- **Enforcer**：`scripts/verify-postgres-migrations.py`
- **负例**：`.build/negatives/db_migration_bad_name.sql.neg`
