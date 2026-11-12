-- +goose Up
-- When the Killy server first saw each parent, so the battle queue keeps its order across restarts.
CREATE TABLE first_detections (
    agent_id    uuid        NOT NULL REFERENCES agents (id) ON DELETE CASCADE,
    pid         integer     NOT NULL,
    starttime   bigint      NOT NULL,
    detected_at timestamptz NOT NULL,
    PRIMARY KEY (agent_id, pid, starttime)
);

-- +goose Down
DROP TABLE first_detections;
