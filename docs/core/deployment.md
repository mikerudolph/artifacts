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

## Initialize an empty database

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

It provisions disposable Postgres and MinIO, runs migration/bootstrap retries, starts two non-root servers with read-only root filesystems, exercises real REST/Git operations, verifies readiness and restart recovery, and removes its containers and network. The evidence directory remains. This test does not deploy to a Kubernetes cluster or ECS account.
