ALTER TABLE settlements
    ADD COLUMN IF NOT EXISTS round_index INT,
    ADD COLUMN IF NOT EXISTS hand_index INT;

CREATE UNIQUE INDEX IF NOT EXISTS idx_settlements_room_round_hand
    ON settlements (room_id, round_index, hand_index)
    WHERE round_index IS NOT NULL AND hand_index IS NOT NULL;
