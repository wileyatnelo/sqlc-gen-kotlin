CREATE TABLE events (
    id       BIGSERIAL PRIMARY KEY,
    name     text  NOT NULL,
    payload  jsonb NOT NULL,
    metadata jsonb
);
