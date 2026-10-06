FROM golang:1.25-alpine@sha256:1ae0735f00daffa3aaf1363a5184c0d2dc55c78e3db4ec70241cdac97bf84b59 AS build
ENV CGO_ENABLED=0 GOBIN=/out
RUN go install github.com/minio/minio@v0.0.0-20250907161309-07c3a429bfed
RUN go install github.com/minio/mc@v0.0.0-20250813083541-7394ce0dd2a8

FROM alpine:3.23@sha256:85fe1e81d6758c208f3e1eed4338a1997e19d4be002d4dd32d3100c9a8c010a0
RUN apk add --no-cache ca-certificates
COPY --from=build /out/minio /out/mc /usr/local/bin/
COPY --from=build /go/pkg/mod/github.com/minio/minio@v0.0.0-20250907161309-07c3a429bfed/LICENSE /licenses/minio-LICENSE
COPY --from=build /go/pkg/mod/github.com/minio/mc@v0.0.0-20250813083541-7394ce0dd2a8/LICENSE /licenses/mc-LICENSE
EXPOSE 9000
ENTRYPOINT ["/usr/local/bin/minio"]
CMD ["server", "/data"]
