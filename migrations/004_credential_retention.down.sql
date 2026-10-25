DROP INDEX repo_tokens_cleanup_idx;
DROP INDEX repo_tokens_repo_state_idx;
ALTER TABLE repo_tokens DROP CONSTRAINT repo_tokens_revocation_time;
ALTER TABLE repo_tokens DROP COLUMN revoked_at;
