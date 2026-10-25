CREATE SCHEMA finalplans;
SET search_path = finalplans,pg_temp;
\ir ../../../migrations/001_init.up.sql
\ir ../../../migrations/002_v2_storage.up.sql
\ir ../../../migrations/003_idempotency.up.sql
\ir ../../../migrations/004_credential_retention.up.sql
INSERT INTO accounts(id) VALUES('fixture');
INSERT INTO namespaces(id,account_id,name) VALUES('fixture','fixture','fixture');
INSERT INTO repos(id,namespace_id,account_id,name) SELECT 'repo-'||i,'fixture','fixture','repo-'||i FROM generate_series(0,999) i;
INSERT INTO repo_tokens(id,repo_id,hash,scope,state,expires_at,revoked_at)
SELECT i::text,'repo-'||(i%1000),i::text,'read',CASE WHEN i%10=0 THEN 'revoked' ELSE 'active' END,
CASE WHEN i<=50000 THEN now()-interval '10 days' ELSE now()+interval '1 day' END,
CASE WHEN i%10=0 THEN now()-interval '9 days' END FROM generate_series(1,100000) i;
ANALYZE repo_tokens;
PREPARE listing(text,boolean,text) AS SELECT id,repo_id,hash,scope,state,created_at,expires_at FROM repo_tokens
WHERE repo_id=$1 AND ($2 OR state=$3) ORDER BY created_at DESC LIMIT 20;
PREPARE counting(text,boolean,text) AS SELECT count(*) FROM repo_tokens WHERE repo_id=$1 AND ($2 OR state=$3);
EXPLAIN (ANALYZE,BUFFERS) EXECUTE listing('repo-1',true,'all');
EXPLAIN (ANALYZE,BUFFERS) EXECUTE listing('repo-1',false,'active');
EXPLAIN (ANALYZE,BUFFERS) EXECUTE counting('repo-1',true,'all');
EXPLAIN (ANALYZE,BUFFERS) EXECUTE counting('repo-1',false,'active');
BEGIN;
EXPLAIN (ANALYZE,BUFFERS) WITH expired AS (
SELECT id FROM repo_tokens WHERE LEAST(expires_at,revoked_at)<now()-interval '7 days'
ORDER BY LEAST(expires_at,revoked_at) LIMIT 1000 FOR UPDATE SKIP LOCKED
) DELETE FROM repo_tokens WHERE id IN (SELECT id FROM expired);
ROLLBACK;
DROP SCHEMA finalplans CASCADE;
