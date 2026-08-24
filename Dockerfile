FROM golang:1.25-alpine AS build
WORKDIR /src
COPY go.mod go.sum* ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /artifacts ./cmd/artifacts

FROM alpine:3.21
RUN apk add --no-cache ca-certificates git
COPY --from=build /artifacts /usr/local/bin/artifacts
ENTRYPOINT ["artifacts"]
CMD ["serve"]
