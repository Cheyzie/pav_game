CREATE TABLE IF NOT EXISTS prompts
(
    id SERIAL PRIMARY KEY,
    user_id INT NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    written_in VARCHAR(255) NOT NULL,
    question VARCHAR(1024) NOT NULL,
    truth VARCHAR(255) NOT NULL,
    category VARCHAR(255) NOT NULL,
    times_used INT NOT NULL DEFAULT 0,
    guessed_correctly INT NOT NULL DEFAULT 0,
    blocked_at TIMESTAMP,
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP NOT NULL DEFAULT NOW()
);