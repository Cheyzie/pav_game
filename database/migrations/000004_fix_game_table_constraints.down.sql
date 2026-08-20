ALTER TABLE submissions DROP CONSTRAINT IF EXISTS submissions_round_player_key;

DROP INDEX IF EXISTS games_room_code_idx;
DROP INDEX IF EXISTS submissions_player_id_idx;
DROP INDEX IF EXISTS submissions_round_id_idx;
DROP INDEX IF EXISTS submissions_game_id_idx;
DROP INDEX IF EXISTS rounds_prompt_id_idx;
DROP INDEX IF EXISTS rounds_game_id_idx;

-- Restoring these will fail if the data now holds more than one round per game
-- or more than one submission per round.
ALTER TABLE submissions ADD CONSTRAINT submissions_round_id_key UNIQUE (round_id);
ALTER TABLE submissions ADD CONSTRAINT submissions_game_id_key UNIQUE (game_id);
ALTER TABLE rounds ADD CONSTRAINT rounds_prompt_id_key UNIQUE (prompt_id);
ALTER TABLE rounds ADD CONSTRAINT rounds_game_id_key UNIQUE (game_id);
