-- Migration 000003 declared game_id/prompt_id/round_id as UNIQUE, which limits
-- a game to a single round, a prompt to a single use across the whole database,
-- and a round to a single submission. The game needs many-to-one on all four.

ALTER TABLE rounds DROP CONSTRAINT IF EXISTS rounds_game_id_key;
ALTER TABLE rounds DROP CONSTRAINT IF EXISTS rounds_prompt_id_key;

ALTER TABLE submissions DROP CONSTRAINT IF EXISTS submissions_game_id_key;
ALTER TABLE submissions DROP CONSTRAINT IF EXISTS submissions_round_id_key;

-- Dropping those constraints also drops the indexes that backed them, which the
-- foreign-key lookups still need.
CREATE INDEX IF NOT EXISTS rounds_game_id_idx ON rounds (game_id);
CREATE INDEX IF NOT EXISTS rounds_prompt_id_idx ON rounds (prompt_id);
CREATE INDEX IF NOT EXISTS submissions_game_id_idx ON submissions (game_id);
CREATE INDEX IF NOT EXISTS submissions_round_id_idx ON submissions (round_id);
CREATE INDEX IF NOT EXISTS submissions_player_id_idx ON submissions (player_id);
CREATE INDEX IF NOT EXISTS games_room_code_idx ON games (room_code);

-- The real constraint: one answer per player per round.
ALTER TABLE submissions ADD CONSTRAINT submissions_round_player_key UNIQUE (round_id, player_id);
