# s3-blackbox-exporter

Minimal synthetic S3 Prometheus exporter for Ceph RGW, MinIO and S3-compatible endpoints.

It periodically performs a real client-side S3 flow:

1. `HEAD Bucket`
2. `PUT Object`
3. `HEAD Object`
4. `LIST Bucket` with exact object key as prefix
5. `GET Object`
6. SHA-256 body verification
7. `DELETE Object`

The exporter writes a small temporary object, reads it back, verifies the body, and removes it. If the object was created, cleanup is attempted even when a later operation fails.

Generated object key layout:

```text
<BASE_PREFIX>/<CLUSTER_NAME>/<AZ>/<uuidv7>.bin
```

Example:

```text
s3-blackbox-exporter/PRS1/az1/019ab6f4-b7d2-7bb8-a322-e23cf19c984a.bin
```

`PREFIX` is still supported as a backward-compatible alias for `BASE_PREFIX`.

## Metrics

| Metric | Type | Description |
|---|---:|---|
| `s3_probe_cycle_success` | gauge | Full probe cycle success: `1` if all critical operations succeeded, `0` otherwise |
| `s3_probe_cycle_duration_seconds` | histogram | Full probe cycle duration |
| `s3_probe_success` | gauge | Operation success: `1` success, `0` failure |
| `s3_probe_duration_seconds` | histogram | Operation latency |
| `s3_probe_http_status_total` | counter | HTTP status counter by `code` and `class` |
| `s3_probe_http_status_class` | gauge | Last observed status class: `2xx`, `4xx`, `5xx`, `timeout`, `other` |
| `s3_probe_last_http_status_code` | gauge | Last HTTP status code; `0` means no HTTP response |
| `s3_probe_error_total` | counter | Error counter by normalized error type |
| `s3_probe_error` | gauge | Last normalized error state |
| `s3_probe_object_size_bytes` | gauge | Configured synthetic object size |
| `s3_probe_last_run_timestamp_seconds` | gauge | Last completed operation timestamp |
| `s3_probe_exporter_build_info` | gauge | Build info |

Common labels:

- `cluster`
- `az`
- `endpoint`
- `bucket`
- `operation`

Operation values:

- `head_bucket`
- `put_object`
- `head_object`
- `list_bucket`
- `get_object`
- `delete_object`

Status classes:

- `2xx`
- `4xx`
- `5xx`
- `timeout`
- `other`

Error types:

- `none`
- `timeout`
- `dns`
- `tcp`
- `tls`
- `auth`
- `http_4xx`
- `http_5xx`
- `checksum`
- `unexpected_status`
- `body_read`
- `skipped`
- `unknown`

## Configuration

| Environment variable | Default | Description |
|---|---:|---|
| `ENDPOINT` | required | S3 endpoint, for example `https://s3.example.ru`; scheme is added automatically if omitted |
| `ACCESS_KEY` | required | S3 access key |
| `SECRET_KEY` | required | S3 secret key |
| `SERCRET_KEY` | empty | Backward-compatible alias for typo; `SECRET_KEY` has priority |
| `BUCKET` | required | Healthcheck bucket |
| `REGION` | `us-east-1` | S3 region |
| `CLUSTER_NAME` | `default` | Label value for `cluster`; also used in object key |
| `AZ` | `unknown` | Label value for `az`; also used in object key |
| `BASE_PREFIX` | `s3-blackbox-exporter` | Base object key prefix |
| `PREFIX` | empty | Backward-compatible alias for `BASE_PREFIX` |
| `INTERVAL` | `30s` | Probe interval; also accepts integer seconds |
| `TIMEOUT` | `10s` | Per-operation timeout; also accepts integer seconds |
| `OBJECT_SIZE_BYTES` | `1048576` | Generated object size |
| `LISTEN_ADDRESS` | `:9241` | HTTP listen address |
| `PATH_STYLE` | `true` | Use S3 path-style addressing; recommended for Ceph RGW |
| `INSECURE_SKIP_VERIFY` | `false` | Skip TLS verification |
| `DISABLE_SSL` | `false` | Use `http://` when `ENDPOINT` is provided without scheme |
| `RETRY_MODE` | `nop` | `nop` or `standard`; `nop` is recommended for raw blackbox monitoring |
| `RETRY_MAX_ATTEMPTS` | `1` | Used when `RETRY_MODE=standard` |
| `RETRY_MAX_BACKOFF` | `2s` | Used when `RETRY_MODE=standard` |
| `AWS_REQUEST_CHECKSUM_CALCULATION` | `when_required` | `when_required` or `when_supported`; keep `when_required` for Ceph RGW/S3-compatible endpoints |
| `AWS_RESPONSE_CHECKSUM_VALIDATION` | `when_required` | `when_required` or `when_supported`; keep `when_required` for Ceph RGW/S3-compatible endpoints |
| `DISABLE_INITIAL_PROBE` | `false` | Do not run probe immediately on startup |

Maximum `OBJECT_SIZE_BYTES` is `536870912` bytes.

For blackbox monitoring, keep:

```bash
RETRY_MODE="nop"
RETRY_MAX_ATTEMPTS="1"
AWS_REQUEST_CHECKSUM_CALCULATION="when_required"
AWS_RESPONSE_CHECKSUM_VALIDATION="when_required"
```

This prevents SDK retries from hiding first-attempt 5xx/timeouts and prevents AWS SDK S3 checksum behavior from forcing CRC32-style checksums that some Ceph RGW/S3-compatible endpoints do not support.

## Run as binary

```bash
export ENDPOINT="https://s3.example.ru"
export ACCESS_KEY="access-key"
export SECRET_KEY="secret-key"
export BUCKET="s3-healthcheck"
export REGION="us-east-1"
export CLUSTER_NAME="PRS1"
export AZ="az1"
export BASE_PREFIX="s3-blackbox-exporter"
export INTERVAL="30s"
export TIMEOUT="10s"
export OBJECT_SIZE_BYTES="1048576"
export PATH_STYLE="true"
export RETRY_MODE="nop"
export AWS_REQUEST_CHECKSUM_CALCULATION="when_required"
export AWS_RESPONSE_CHECKSUM_VALIDATION="when_required"

./s3-blackbox-exporter
```

Open:

```bash
curl -s http://127.0.0.1:9241/healthz
curl -s http://127.0.0.1:9241/metrics
```

## Run with Docker

```bash
docker run -d \
  --name s3-blackbox-exporter \
  --restart unless-stopped \
  -p 9241:9241 \
  -e ENDPOINT="https://s3.example.ru" \
  -e ACCESS_KEY="access-key" \
  -e SECRET_KEY="secret-key" \
  -e BUCKET="s3-healthcheck" \
  -e REGION="us-east-1" \
  -e CLUSTER_NAME="PRS1" \
  -e AZ="az1" \
  -e BASE_PREFIX="s3-blackbox-exporter" \
  -e INTERVAL="30s" \
  -e OBJECT_SIZE_BYTES="1048576" \
  -e PATH_STYLE="true" \
  -e RETRY_MODE="nop" \
  -e AWS_REQUEST_CHECKSUM_CALCULATION="when_required" \
  -e AWS_RESPONSE_CHECKSUM_VALIDATION="when_required" \
  yeti89/s3-blackbox-exporter:latest
```

## Run with Podman

See `deploy/podman-run.sh`.

## Lifecycle policy

The exporter deletes test objects after each successful cycle. Still, configure lifecycle cleanup for crash leftovers:

```json
{
  "Rules": [
    {
      "ID": "DeleteS3ProbeObjectsAfterOneDay",
      "Status": "Enabled",
      "Filter": {
        "Prefix": "s3-blackbox-exporter/"
      },
      "Expiration": {
        "Days": 1
      }
    }
  ]
}
```

## Build locally

```bash
go mod tidy
make test
make build
```

## Docker Hub pipeline

The Docker Hub workflow expects:

- GitHub variable: `DOCKERHUB_USERNAME`
- GitHub secret: `DOCKERHUB_TOKEN`

It pushes:

- `DOCKERHUB_USERNAME/s3-blackbox-exporter:latest`
- `DOCKERHUB_USERNAME/s3-blackbox-exporter:main-<sha>`
- `DOCKERHUB_USERNAME/s3-blackbox-exporter:<git-tag>` for tags like `v1.0.0`

## Binary release pipeline

Push a semver tag:

```bash
git tag v0.2.0
git push origin v0.2.0
```

GitHub Actions runs GoReleaser and publishes binaries to GitHub Releases.

## Example PromQL

Full cycle failed during the last 3 minutes:

```promql
min_over_time(s3_probe_cycle_success[3m]) == 0
```

Full cycle p95 latency:

```promql
histogram_quantile(
  0.95,
  sum by (le, cluster, az, endpoint, bucket) (
    rate(s3_probe_cycle_duration_seconds_bucket[5m])
  )
)
```

PUT failed during the last 3 minutes:

```promql
min_over_time(s3_probe_success{operation="put_object"}[3m]) == 0
```

GET failed during the last 3 minutes:

```promql
min_over_time(s3_probe_success{operation="get_object"}[3m]) == 0
```

Any 5xx observed:

```promql
increase(s3_probe_http_status_total{class="5xx"}[5m]) > 0
```

Timeout observed:

```promql
increase(s3_probe_error_total{type="timeout"}[5m]) > 0
```

PUT p95 latency:

```promql
histogram_quantile(
  0.95,
  sum by (le, cluster, az, endpoint, bucket) (
    rate(s3_probe_duration_seconds_bucket{operation="put_object"}[5m])
  )
)
```
