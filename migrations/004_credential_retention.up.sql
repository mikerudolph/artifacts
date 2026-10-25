ALTER TABLE repo_tokens ADD COLUMN revoked_at TIMESTAMPTZ;

UPDATE repo_tokens SET revoked_at = statement_timestamp() WHERE state = 'revoked';

ALTER TABLE repo_tokens ADD CONSTRAINT repo_tokens_revocation_time
    CHECK ((state = 'revoked') = (revoked_at IS NOT NULL));

CREATE INDEX repo_tokens_repo_state_idx ON repo_tokens (repo_id, state);
CREATE INDEX repo_tokens_cleanup_idx ON repo_tokens (LEAST(expires_at, revoked_at));
