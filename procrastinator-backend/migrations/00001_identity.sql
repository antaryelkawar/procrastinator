-- +goose Up
-- Identity substrate (change: owner-model-rework).
-- `users` is the user registry (no RLS: the API middleware queries it unbound).
-- `households` + `household_members` carry the household-sharing model.
-- RLS on households / household_members is added by 00004.

CREATE TABLE users (
    id text PRIMARY KEY CHECK (id ~ '^[A-Za-z0-9_-]{1,64}$'),
    created_at timestamptz NOT NULL DEFAULT now()
);

-- Explicit provisioning of dev/test users (never implicit from a request).
INSERT INTO users (id) VALUES ('test-user'), ('test-user-b');

CREATE TABLE households (
    id text PRIMARY KEY CHECK (id ~ '^[A-Za-z0-9_-]{1,64}$'),
    -- The household's owning user (the creator).
    owner_id text NOT NULL REFERENCES users (id),
    display_name text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX idx_households_owner ON households (owner_id);

-- Many-to-many membership: a user may belong to multiple households and a
-- household may have multiple users; membership is unique per (household, user).
CREATE TABLE household_members (
    household_id text NOT NULL REFERENCES households (id) ON DELETE CASCADE,
    -- Members are users from the registry.
    user_id text NOT NULL REFERENCES users (id),
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (household_id, user_id)
);
CREATE INDEX idx_household_members_user ON household_members (user_id);

-- +goose Down
DROP TABLE IF EXISTS household_members;
DROP TABLE IF EXISTS households;
DROP TABLE IF EXISTS users;
