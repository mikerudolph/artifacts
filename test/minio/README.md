# MinIO test fixture

Build the test-only image with `make test-deps` before running focused Go tests that use S3. `make verify`, `make test`, and `make coverage` build it automatically. The container acceptance driver also builds it before starting its owned services. The optional MinIO service in Docker Compose uses the same build context. Docker reuses successful build layers on subsequent runs.

The image builds the same MinIO server release previously pulled from Quay, `RELEASE.2025-09-07T16-13-09Z` (`07c3a429bfed433e49018cb0f78a52145d4bedeb`), and the MinIO client release `RELEASE.2025-08-13T08-35-41Z` (`7394ce0dd2a80935aded936b09fa12cbb3cb8096`). Go pseudo-versions pin these sources and the Go checksum database verifies module downloads. The Dockerfile pins its base images by digest and includes the upstream licenses. It does not add MinIO to the Artifacts production image.

Fresh CI pulls of the former Quay image failed on [run 37404425750](https://github.com/mikerudolph/artifacts/actions/runs/37404425750), although an existing local cache still worked. Independent fresh registry requests returned authentication failures for both Quay and Docker Hub. MinIO's [upstream documentation](https://github.com/minio/minio#source-only-distribution) describes source-only distribution. Building the pinned source makes the fixture independent of those prebuilt image repositories.

The local tag is `artifacts-test-minio:2025-09-07`. Keep the Makefile, Docker Compose, `internal/testkit/minio.go`, and `scripts/verify-container.py` aligned if it changes. This is a disposable compatibility-test dependency, not a production storage recommendation. No fixture images are published by this workflow.
