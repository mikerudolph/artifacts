ALTER TABLE repos ADD COLUMN account_id TEXT REFERENCES accounts (id);
UPDATE repos r SET account_id = n.account_id FROM namespaces n WHERE n.id = r.namespace_id;
ALTER TABLE repos ALTER COLUMN account_id SET NOT NULL;
ALTER TABLE repos ADD COLUMN storage_version INT NOT NULL DEFAULT 1;
ALTER TABLE repos ALTER COLUMN storage_version SET DEFAULT 2;
ALTER TABLE repos ADD COLUMN wal_sequence BIGINT NOT NULL DEFAULT 0;
ALTER TABLE repos ADD COLUMN failure TEXT NOT NULL DEFAULT '';
ALTER TABLE repos ADD COLUMN deleted_at TIMESTAMPTZ;
CREATE INDEX repos_account_idx ON repos (account_id, namespace_id);

CREATE TABLE pack_wal (
    repo_id TEXT NOT NULL REFERENCES repos (id),
    sequence BIGINT NOT NULL,
    pack_key TEXT NOT NULL,
    index_key TEXT NOT NULL,
    checksum TEXT NOT NULL,
    size BIGINT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (repo_id, sequence)
);

CREATE TABLE pack_ref_updates (
    repo_id TEXT NOT NULL,
    sequence BIGINT NOT NULL,
    name TEXT NOT NULL,
    old_sha TEXT NOT NULL,
    new_sha TEXT NOT NULL,
    PRIMARY KEY (repo_id, sequence, name),
    FOREIGN KEY (repo_id, sequence) REFERENCES pack_wal (repo_id, sequence) ON DELETE CASCADE
);

CREATE TABLE checkpoints (
    repo_id TEXT PRIMARY KEY REFERENCES repos (id),
    sequence BIGINT NOT NULL,
    pack_key TEXT NOT NULL,
    index_key TEXT NOT NULL,
    checksum TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE repo_forks (
    repo_id TEXT PRIMARY KEY REFERENCES repos (id),
    parent_repo_id TEXT NOT NULL REFERENCES repos (id),
    parent_sequence BIGINT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX repo_forks_parent_idx ON repo_forks (parent_repo_id);

ALTER TABLE jobs ADD COLUMN payload JSONB NOT NULL DEFAULT '{}'::jsonb;
ALTER TABLE jobs ADD COLUMN lease_until TIMESTAMPTZ;
ALTER TABLE jobs ADD COLUMN attempts INT NOT NULL DEFAULT 0;
