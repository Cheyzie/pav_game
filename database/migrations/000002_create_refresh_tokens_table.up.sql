CREATE TABLE refresh_tokens
(
    id SERIAL PRIMARY KEY,
    token VARCHAR(64) NOT NULL,
    session_name VARCHAR(64) NOT NULL,
    user_id INT NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    ip_address varchar(18) NOT NULL,
    expires_at TIMESTAMP NOT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    UNIQUE(user_id, session_name)
);