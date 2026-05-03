# syntax=docker/dockerfile:1.7

FROM golang:1.23-alpine AS builder
WORKDIR /src

RUN apk add --no-cache ca-certificates git

COPY go.mod go.sum* ./
RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download

COPY . .

ARG VERSION=dev
ARG COMMIT=none
ARG DATE=unknown

RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=linux go build \
      -trimpath \
      -ldflags="-s -w -X main.version=${VERSION} -X main.commit=${COMMIT} -X main.date=${DATE}" \
      -o /out/s3-blackbox-exporter ./cmd/s3-blackbox-exporter

FROM alpine:3.20
RUN apk add --no-cache ca-certificates && \
    addgroup -S app && \
    adduser -S -G app -H -h /nonexistent -s /sbin/nologin app
COPY --from=builder /out/s3-blackbox-exporter /usr/local/bin/s3-blackbox-exporter
USER app:app
EXPOSE 9241
ENTRYPOINT ["/usr/local/bin/s3-blackbox-exporter"]
