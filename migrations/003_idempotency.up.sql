CREATE TABLE idempotency_results (
    scope TEXT NOT NULL,
    key TEXT NOT NULL,
    digest TEXT NOT NULL,
    result JSONB NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (scope, key)
);
