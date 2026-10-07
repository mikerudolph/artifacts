---
title: Production deployment
description: Build one image, initialize Postgres, provision a control token, and start serving.
---

Deploy one release image with three commands: `migrate`, `bootstrap --account acme`, and `serve`. Postgres holds publication metadata and credentials; S3-compatible storage holds durable Git objects. Each server needs independent disposable cache and scratch space. See [configuration](/artifacts/core/configuration/) for the complete settings and multi-instance constraints.

## Build the image

```bash
docker build --pull -t artifacts:local .
export ARTIFACTS_IMAGE=artifacts:local
```

The Dockerfile pins its Go and Alpine base images by digest, includes Git and CA certificates, and runs as UID/GID `10001:10001`. Its build context includes only application source and Go dependency manifests. Rebuild regularly to pick up Alpine package security updates; update base digests deliberately. Base pinning does not pin the packages installed by `apk`.

This repository does not yet publish a supported registry image. Push your built image to your registry and use that immutable image digest for setup jobs and all serving instances.

## Install into an existing database

A fresh database is optional. Artifacts can own a dedicated schema in a database that already contains another application. Provision separate migration and runtime logins through your database provider. An administrator can create the schema owned by the migration role:

```sql
GRANT CONNECT ON DATABASE appdb TO artifacts_migration, artifacts_runtime;
CREATE SCHEMA artifacts AUTHORIZATION artifacts_migration;
GRANT USAGE ON SCHEMA artifacts TO artifacts_runtime;
```

Set `DATABASE_URL` to the migration login for `appdb`, and use the same schema setting for every Artifacts command and server:

```bash
export ARTIFACTS_DATABASE_SCHEMA=artifacts
docker run --rm --read-only \
  --env DATABASE_URL --env ARTIFACTS_DATABASE_SCHEMA \
  "$ARTIFACTS_IMAGE" migrate
```

The migration role needs ownership of its schema and objects, but does not need database ownership or database `CREATE` when the schema already exists. Alternatively, an identity with database `CREATE` can run `migrate --create-schema` to create a missing schema owned by that identity. Normal migration and serving fail if the selected schema is absent or inaccessible. Artifacts does not create roles or change neighboring applications' grants. Select an empty schema; unrelated tables, functions, types, or migration ledgers cause migration to fail.

After migration, run these grants as the migration role:

```sql
GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA artifacts TO artifacts_runtime;
REVOKE INSERT, UPDATE, DELETE ON artifacts.schema_migrations FROM artifacts_runtime;
ALTER DEFAULT PRIVILEGES IN SCHEMA artifacts
  GRANT SELECT, INSERT, UPDATE, DELETE ON TABLES TO artifacts_runtime;
```

The runtime role needs no schema `CREATE`, ownership, or database `CREATE`. Run bootstrap and serving with this login and `ARTIFACTS_DATABASE_SCHEMA=artifacts`; set `ARTIFACTS_SKIP_MIGRATIONS=true` for serving, token creation, and compaction. Default privileges must be set by the role that creates future migration objects. Current migrations create no sequences.

Keep the schema setting in the shared environment for migration jobs, bootstrap jobs, and all servers. Pass `--env ARTIFACTS_DATABASE_SCHEMA` to every Docker command below when using a named schema. Independent installations also need separate object-storage prefixes and cache roots. This installs a new Artifacts instance; moving an existing installation from `public` requires a separate data-movement procedure. Merely changing the setting does not relocate tables or repository history.

## Existing default-schema deployments

Create the database and its owner role through your database provider. Inject the owner's connection string as `DATABASE_URL` in a one-off migration job, then run:

```bash
docker run --rm --read-only --env DATABASE_URL "$ARTIFACTS_IMAGE" migrate
```

The database must already exist and be reachable from the container. `migrate` creates or upgrades the schema and is safe to rerun. It needs only database configuration, with no S3 or authentication settings. Run it once per deployment before starting servers. Stop old servers before upgrading: mixed-release rolling upgrades are not established.

Keep the schema owner's credentials in the migration job. Provision a separate runtime login and grant it the following privileges as the owner, substituting your database and role names:

```sql
GRANT CONNECT ON DATABASE artifacts TO artifacts_runtime;
GRANT USAGE ON SCHEMA public TO artifacts_runtime;
GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA public TO artifacts_runtime;
GRANT USAGE, SELECT ON ALL SEQUENCES IN SCHEMA public TO artifacts_runtime;
ALTER DEFAULT PRIVILEGES IN SCHEMA public
  GRANT SELECT, INSERT, UPDATE, DELETE ON TABLES TO artifacts_runtime;
ALTER DEFAULT PRIVILEGES IN SCHEMA public
  GRANT USAGE, SELECT ON SEQUENCES TO artifacts_runtime;
```

Run default-privilege statements as the role that creates migration objects. The runtime role must not own the schema or have `CREATE` on it. With `ARTIFACTS_SKIP_MIGRATIONS=true`, serving validates the schema without running DDL and rejects missing, dirty, older, or newer schemas.

## Bootstrap the first account

Generate a high-entropy control token in your secret manager and inject it as `ARTIFACTS_BOOTSTRAP_TOKEN`. Set `DATABASE_URL` to the runtime connection string, then run:

```bash
docker run --rm --read-only \
  --env DATABASE_URL --env ARTIFACTS_BOOTSTRAP_TOKEN \
  "$ARTIFACTS_IMAGE" bootstrap --account acme
```

Alternatively, mount a secret file readable by UID 10001 and set `ARTIFACTS_BOOTSTRAP_TOKEN_FILE` to its container path. Set exactly one token source. Tokens must contain 32–4096 bytes without embedded whitespace; surrounding whitespace is trimmed. Bootstrap stores only the token hash and never prints the token.

Rerunning bootstrap with the same account and token succeeds, including concurrent retries. Reusing that token for another account fails atomically. Supplying a new token adds another credential; it does not revoke old credentials. Bootstrap requires a current schema and does not run migrations. This creates account access, not sample repositories. Give the secret to your application as its control-plane credential; workers should receive [repository-scoped credentials](/artifacts/core/authentication/).

## RDS and Aurora IAM authentication

IAM authentication is independent of schema selection. Enable IAM database authentication on the RDS PostgreSQL instance or Aurora PostgreSQL cluster, then grant `rds_iam` to each database login that will use IAM. Keep migration and runtime permissions separate; IAM login does not replace SQL grants. See AWS's [database account setup](https://docs.aws.amazon.com/AmazonRDS/latest/UserGuide/UsingWithRDS.IAMDBAuth.DBAccounts.html).

```sql
GRANT rds_iam TO artifacts_migration, artifacts_runtime;
```

Give each workload role `rds-db:connect` for its specific database user. The policy resource uses the database resource ID (or Aurora cluster resource ID), not the instance display name. For example, substitute your region, account, resource ID, and username into `arn:aws:rds-db:us-east-1:123456789012:dbuser:db-RESOURCE_ID/artifacts_runtime`. See AWS's [IAM policy format](https://docs.aws.amazon.com/AmazonRDS/latest/UserGuide/UsingWithRDS.IAMDBAuth.IAMPolicy.html).

For the recommended `verify-full` mode, mount a trusted RDS CA bundle readable by the image's non-root user. Configure the direct writer endpoint, database, and appropriate login without a password:

```bash
export ARTIFACTS_DATABASE_AUTH=rds-iam
export ARTIFACTS_DATABASE_REGION=us-east-1
export ARTIFACTS_DATABASE_SCHEMA=artifacts
export DATABASE_URL='postgres://artifacts_runtime@cluster.example.us-east-1.rds.amazonaws.com:5432/appdb?sslmode=verify-full&sslrootcert=/etc/rds/global-bundle.pem&pool_max_conns=10'
```

Use the actual RDS/Aurora endpoint rather than a custom DNS alias or IP address. Provision the CA bundle through deployment configuration; Artifacts does not download it at startup. AWS provides [verified TLS connection guidance](https://docs.aws.amazon.com/AmazonRDS/latest/UserGuide/UsingWithRDS.IAMDBAuth.Connecting.AWSCLI.PostgreSQL.html). Pass the database auth, region, and schema variables into every container, mount the CA file at the configured path when configured, and expose the workload identity to the SDK. Use the migration username and workload role for `migrate`; use the runtime identity for bootstrap and serving.

Deployments that deliberately use encryption without server verification can instead configure `sslmode=require` and omit `sslrootcert` and `PGSSLROOTCERT`. This applies to `serve`, `migrate`, `bootstrap`, `token create`, and `compact`. TLS remains mandatory, but an impersonating server could receive the signed login token and database traffic. IAM authenticates the workload to RDS; it does not replace client verification of the database server.

`verify-ca` is also supported and verifies the certificate chain without checking the endpoint hostname. In pgx, `require` with CA material performs chain verification too. Artifacts logs one warning per process when IAM runs without full server identity verification. `verify-full` remains recommended; `disable`, `allow`, and `prefer` are rejected. See PostgreSQL's [TLS mode comparison](https://www.postgresql.org/docs/current/libpq-ssl.html#LIBPQ-SSL-PROTECTION) for the trade-offs.

The SDK credential chain supports workload roles and configured AWS profiles. Database signing uses `ARTIFACTS_DATABASE_REGION`, falling back to standard SDK region configuration; `S3_REGION` does not set it. Omit static AWS credentials when using a workload role. Omit database passwords, including matching password-file entries and `PGPASSWORD`. Connections sign on creation; existing sessions are reused, and newly opened connections retrieve credentials through the refreshable provider. There is no application-level transaction replay after connection loss. Preserve the original idempotency key when recovering an uncertain supported REST write.

Local verification covers both connection hooks over TLS, renewed signing credentials, schema isolation, and real REST/Git traffic. Real AWS login, workload-role renewal, failover, and RDS Proxy remain unverified. Qualify the direct writer endpoint first. Proxy endpoints require their own policy and acceptance run, including migration/session pinning; this change does not add Aurora topology discovery or failover plugins.

## S3 server-side encryption

When a bucket policy requires explicit encryption headers, configure the serving instances and compaction jobs consistently:

```bash
export S3_SSE=aws:kms
export S3_SSE_KMS_KEY_ID=arn:aws:kms:us-east-1:123456789012:key/your-key-id
```

Pass `--env S3_SSE --env S3_SSE_KMS_KEY_ID` to Docker serving and compaction commands, or inject the same settings into your workload. Use `S3_SSE=AES256` with no key identifier for SSE-S3. Leave both unset to use the bucket's existing defaults without sending encryption headers. With `aws:kms` and no key identifier, AWS S3 uses its AWS-managed `aws/s3` key; set an explicit customer-managed key ARN when your bucket policy requires one. See the [S3 encryption request contract](https://docs.aws.amazon.com/AmazonS3/latest/API/API_PutObject.html).

For SSE-KMS, provision a key in the bucket's region and grant the workload the necessary S3 access plus `kms:GenerateDataKey` for writes and `kms:Decrypt` for reads through both IAM and the KMS key policy. Use a full key ARN to avoid alias resolution surprises, particularly across accounts. Configure bucket policy to deny missing or incorrect encryption headers when enforcement is required. Artifacts does not create keys, edit policies, or fall back to weaker encryption on failure. AWS documents [SSE-KMS permissions and key selection](https://docs.aws.amazon.com/AmazonS3/latest/userguide/UsingKMSEncryption.html).

The setting affects new uploads, including indexes, pack files, checkpoints, and copies. Existing objects remain under their previous encryption policy; a matching immutable object may be reused without uploading it again. Retain decrypt access to every key used by stored history, including fork ancestors. Changing this setting is not a key-rotation or historical re-encryption procedure. Encryption adds request headers without changing application object bytes, request counts, or local staging; KMS can add provider calls, cost, and quotas.

Before admitting traffic, run the verification harness against an owned test repository using the intended workload role and bucket policy. Confirm object encryption metadata, REST/Git readback, compaction, and reconstruction after cache loss. Also verify denial with missing headers or insufficient key permissions. `/readyz` only checks readability; no automatic startup write probe is performed. Local MinIO and HTTP contract tests do not establish AWS KMS permissions or policy enforcement.

## Start servers

Create a private S3 bucket and inject the runtime database connection, S3 settings, and public HTTPS origin. The server can use the AWS SDK credential provider chain for workload identity when static credentials are omitted.

The following example assumes `DATABASE_URL`, `S3_BUCKET`, `S3_REGION`, and `ARTIFACTS_PUBLIC_URL` are already exported. It uses static S3 credentials injected by the environment for a local Docker deployment:

```bash
docker run -d --name artifacts --read-only \
  --cap-drop ALL --security-opt no-new-privileges \
  --tmpfs /var/cache/artifacts:rw,uid=10001,gid=10001,mode=0700 \
  --publish 127.0.0.1:8080:8080 \
  --env DATABASE_URL --env ARTIFACTS_PUBLIC_URL \
  --env ARTIFACTS_STORAGE=s3 --env S3_BUCKET --env S3_REGION \
  --env AWS_ACCESS_KEY_ID --env AWS_SECRET_ACCESS_KEY \
  --env ARTIFACTS_SKIP_MIGRATIONS=true \
  "$ARTIFACTS_IMAGE" serve
```

Use a disk-backed writable cache volume for larger repositories; tmpfs consumes memory. The image sets `ARTIFACTS_CACHE_DIR=/var/cache/artifacts/repos` and `TMPDIR=/var/cache/artifacts/tmp`. Mounting the parent covers both. Provision writable mounts for UID/GID 10001; arbitrary host bind mounts do not inherit the image directory ownership. Cache space must accommodate full local repositories, staging, and repacking. Filesystem object storage additionally requires a durable writable mount at `/var/lib/artifacts/objects` and is not a substitute for shared S3 across independent nodes.

Do not inject the bootstrap token into serving containers. Stored control tokens authenticate requests without `ARTIFACTS_API_TOKEN`. Terminate TLS at the ingress or load balancer and set `ARTIFACTS_PUBLIC_URL` to that externally reachable origin.

## Kubernetes and ECS lifecycle

Use a Kubernetes Job or ECS one-off task for migration, followed by bootstrap, then start the service using the same image. Override the image command with `migrate` or `bootstrap --account acme`; the default command is `serve`.

For Kubernetes, use the [Pod security context](https://kubernetes.io/docs/tasks/configure-pod-container/security-context/) settings `runAsUser: 10001`, `runAsGroup: 10001`, `fsGroup: 10001`, `readOnlyRootFilesystem: true`, `allowPrivilegeEscalation: false`, and drop all capabilities. Mount an `emptyDir` at `/var/cache/artifacts`. Use HTTP liveness `/healthz` and readiness `/readyz` on port 8080; give readiness probes more than the application's two-second dependency deadline. Size startup allowances for database connections and workload-identity discovery.

For ECS, configure the [container definition](https://docs.aws.amazon.com/AmazonECS/latest/APIReference/API_ContainerDefinition.html) with user `10001:10001`, enable a read-only root filesystem, and mount writable ephemeral storage at `/var/cache/artifacts` with matching ownership. Use an ALB health check on `/readyz`; a container health check can use the image's BusyBox `wget -q -O /dev/null http://127.0.0.1:8080/healthz`. Configure S3 access through the task role and inject database credentials from your secret manager.

Both endpoints are unauthenticated, support GET/HEAD, and expose only status codes. `/healthz` reports the HTTP process is alive. `/readyz` checks the exact schema version and object-storage readability within two seconds, returning 503 when unavailable or draining. For S3, allow `GetObject` and the bucket listing permission needed to distinguish a missing probe object from access denial. Readiness does not prove write permissions; normal operations also need object read/write permissions.

SIGTERM stops accepting new connections, marks the server unready, stops background maintenance, and lets active requests finish for up to `ARTIFACTS_SHUTDOWN_TIMEOUT` (default `30s`). At the deadline it closes active connections and exits unsuccessfully. Set Kubernetes termination grace or ECS stop timeout above that duration, allowing time for routing changes and process cleanup. A lost response can still follow a committed write; use the documented idempotency and reconciliation behavior.

## Validate a deployment image

With Docker, Go, Git, and Python 3 installed, run the owned-container acceptance test:

```bash
python3 scripts/verify-container.py --image "$ARTIFACTS_IMAGE" --evidence /tmp/artifacts-container-evidence
```

For installation alongside an existing application, add `--database-schema artifacts` and use a fresh evidence directory. That mode creates conflicting table and migration-ledger names in `public`, migrates with a schema owner lacking database `CREATE`, runs with a restricted runtime login, and checks the neighboring application's data before and after the REST/Git workflow.

Add `--s3-sse AES256` or `--s3-sse aws:kms` to verify encrypted object storage. The driver creates a disposable local MinIO encryption key, checks stored objects' encryption metadata, and exercises compaction and cold-cache recovery. This verifies the S3-compatible path, not the AWS KMS service.

It provisions disposable Postgres and MinIO, runs migration/bootstrap retries, starts two non-root servers with read-only root filesystems, exercises real REST/Git operations, verifies readiness and restart recovery, and removes its containers and network. The evidence directory remains. This test does not deploy to a Kubernetes cluster or ECS account.
