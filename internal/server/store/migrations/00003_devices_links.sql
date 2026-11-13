-- +goose Up
CREATE TABLE devices (
    id           uuid        PRIMARY KEY,
    token_sha256 bytea       NOT NULL UNIQUE,
    -- NULL when the phone has no notifications.
    push_token   text,
    -- The app-wide notification switch.
    notify       boolean     NOT NULL DEFAULT true,
    created_at   timestamptz NOT NULL DEFAULT now()
);

-- A phone's link to an agent. The display name and the server's notification switch belong to
-- the link, so each phone keeps its own.
CREATE TABLE links (
    device_id  uuid        NOT NULL REFERENCES devices (id) ON DELETE CASCADE,
    agent_id   uuid        NOT NULL REFERENCES agents (id) ON DELETE CASCADE,
    name       text        NOT NULL,
    notify     boolean     NOT NULL DEFAULT true,
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (device_id, agent_id)
);

-- +goose Down
DROP TABLE links;
DROP TABLE devices;
