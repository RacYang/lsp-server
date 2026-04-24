-- Phase 3：房间事件日志、对局摘要与结算历史（PostgreSQL）。
CREATE TABLE IF NOT EXISTS room_events (
    id BIGSERIAL PRIMARY KEY,
    room_id TEXT NOT NULL,
    seq BIGINT NOT NULL,
    kind TEXT NOT NULL,
    payload BYTEA NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (room_id, seq)
);

CREATE INDEX IF NOT EXISTS idx_room_events_room_seq ON room_events (room_id, seq);

CREATE TABLE IF NOT EXISTS game_summaries (
    room_id TEXT PRIMARY KEY,
    rule_id TEXT NOT NULL,
    player_ids TEXT[] NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    ended_at TIMESTAMPTZ
);

CREATE TABLE IF NOT EXISTS settlements (
    id BIGSERIAL PRIMARY KEY,
    room_id TEXT NOT NULL,
    winner_user_ids TEXT[] NOT NULL,
    total_fan INT NOT NULL,
    detail_text TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_settlements_room ON settlements (room_id);
