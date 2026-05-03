#!/usr/bin/env bash
set -euo pipefail

podman run -d \
  --name s3-blackbox-exporter \
  --restart=always \
  -p 9241:9241 \
  -e ENDPOINT="https://s3.example.ru" \
  -e ACCESS_KEY="change-me-access-key" \
  -e SECRET_KEY="change-me-secret-key" \
  -e BUCKET="s3-healthcheck" \
  -e REGION="us-east-1" \
  -e CLUSTER_NAME="PRS1" \
  -e AZ="az1" \
  -e BASE_PREFIX="s3-blackbox-exporter" \
  -e INTERVAL="30s" \
  -e TIMEOUT="10s" \
  -e OBJECT_SIZE_BYTES="1048576" \
  -e LISTEN_ADDRESS=":9241" \
  -e PATH_STYLE="true" \
  -e INSECURE_SKIP_VERIFY="false" \
  -e RETRY_MODE="nop" \
  docker.io/yeti89/s3-blackbox-exporter:latest
