FROM golang:1.25-alpine@sha256:1ae0735f00daffa3aaf1363a5184c0d2dc55c78e3db4ec70241cdac97bf84b59 AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -buildvcs=false -o /artifacts ./cmd/artifacts

FROM alpine:3.23@sha256:85fe1e81d6758c208f3e1eed4338a1997e19d4be002d4dd32d3100c9a8c010a0
RUN apk add --no-cache ca-certificates git \
    && addgroup -g 10001 artifacts \
    && adduser -D -u 10001 -G artifacts artifacts \
    && mkdir -p /var/lib/artifacts/objects /var/cache/artifacts/repos /var/cache/artifacts/tmp \
    && chown -R 10001:10001 /var/lib/artifacts /var/cache/artifacts
COPY --from=build /artifacts /usr/local/bin/artifacts
ENV ARTIFACTS_DATA_DIR=/var/lib/artifacts/objects \
    ARTIFACTS_CACHE_DIR=/var/cache/artifacts/repos \
    TMPDIR=/var/cache/artifacts/tmp
WORKDIR /home/artifacts
USER 10001:10001
EXPOSE 8080
STOPSIGNAL SIGTERM
ENTRYPOINT ["artifacts"]
CMD ["serve"]
