CREATE TABLE IF NOT EXISTS chapter_auto_commissions (
  id BIGSERIAL PRIMARY KEY,
  commander_id BIGINT NOT NULL REFERENCES commanders(commander_id) ON DELETE CASCADE,
  type BIGINT NOT NULL,
  config_id BIGINT NOT NULL,
  finish_time BIGINT NOT NULL,
  ticket_time BIGINT NOT NULL DEFAULT 0,
  cost_time BIGINT NOT NULL DEFAULT 0,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS chapter_auto_commissions_commander_idx
  ON chapter_auto_commissions (commander_id);
