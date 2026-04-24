ALTER TABLE settlements
    ADD COLUMN IF NOT EXISTS seat_scores_json BYTEA,
    ADD COLUMN IF NOT EXISTS penalties_json BYTEA,
    ADD COLUMN IF NOT EXISTS payload BYTEA;
