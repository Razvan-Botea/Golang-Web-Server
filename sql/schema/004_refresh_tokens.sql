-- +goose Up
Create table refresh_tokens (
    token text primary key,
    created_at timestamp NOT NULL,
    updated_at timestamp NOT NULL,
    user_id UUID NOT NULL, FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE,
    expires_at timestamp NOT NULL,
    revoked_at timestamp
);

-- +goose Down
DROP TABLE refresh_tokens;