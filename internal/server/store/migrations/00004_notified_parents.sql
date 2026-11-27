-- +goose Up
-- Which device was already notified of which parent. A row goes away with its parent's
-- first-detection row (the parent left the snapshot) and with its link (the phone removed the
-- server), so a parent that comes back, and a phone that links again, are notified again.
CREAT TABLE notified_parents (
    agent_id    uuid        NOT NULL,
    pid         integer     NOT NULL,
    starttime   bigint      NOT NULL,
    device_id   uuid        NOT NULL,
    notified_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (agent_id, pid, starttime, device_id),
    FOREIGN KEY (agent_id, pid, starttime) REFERENCES first_detections (agent_id, pid, starttime) ON DELETE CASCADE,
    FOREIGN KEY (device_id, agent_id) REFERENCES links (device_id, agent_id) ON DELETE CASCADE
);

-- For the cascade from links, whose key starts with device_id.
CREATE INDEX notified_parents_link ON notified_parents (device_id, agent_id);

-- +goose Down
DROP TABLE notified_parents;
