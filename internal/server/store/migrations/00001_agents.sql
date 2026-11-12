-- +goose Up
CREATE TABLE agents (
    id           uuid        PRIMARY KEY,
    token_sha256 bytea       NOT NULL UNIQUE,
    hostname     text        NOT NULL,
    created_at   timestamptz NOT NULL DEFAULT now()
);

-- +goose Down
DROP TABLE agents;
