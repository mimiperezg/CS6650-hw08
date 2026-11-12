# Observability

The [Product Service](../product-service/README.md) records three signals for every endpoint, known as **RED**:

| Signal | Question it answers | How it's measured |
|--------|---------------------|-------------------|
| **Rate** | How busy is it? | Requests per second |
| **Errors** | How often does it fail? | Percentage of responses that aren't `2xx` |
| **Duration** | How slow is it? | Latency at p50 (median) and p95, not the average |

Each signal is available in two forms:

- **Metrics**, which Prometheus scrapes from `/metrics` and Grafana graphs live. Use them to watch a load test while it runs.
- **Logs**, one JSON line per request. Use them to investigate a problem after it happened, when no one was watching a dashboard.

Both come from one middleware, [`observability.go`](../product-service/observability.go), so they always describe the same requests.

## Running Prometheus and Grafana

This stack runs only on your machine. It is not deployed to AWS.

From `hw5/`:

```bash
docker compose up --build
```

| What | URL |
|------|-----|
| Product Service | <http://localhost:8080> |
| Prometheus | <http://localhost:9090> |
| Grafana dashboard | <http://localhost:3000>. No login required; the **Product Service: RED** dashboard opens as the home page. |

Then generate traffic against the local service, for example with [Locust](../locust/README.md):

```bash
locust ProductFastHttpUser --host http://localhost:8080 --headless -u 50 -r 10 -t 2m
```

The dashboard refreshes every 5 seconds. It needs about 20 seconds of traffic before the first points appear, because each rate is calculated over several scrapes.

To check that Prometheus is collecting data, open <http://localhost:9090/targets>. `product-service` should show as **UP**.

To stop the stack:

```bash
docker compose down
```

Prometheus keeps its data inside its container, so `down` deletes it. Products are also lost, because the service stores them in memory.

## The metrics

`GET /metrics` returns these metrics, plus the Go runtime and process metrics that the client library adds automatically:

| Metric | Type | Labels | RED signal |
|--------|------|--------|------------|
| `http_requests_total` | counter | `method`, `endpoint`, `status` | Rate |
| `http_request_errors_total` | counter | `method`, `endpoint`, `status` | Errors (counts only non-`2xx` responses) |
| `http_request_duration_seconds` | histogram | `method`, `endpoint` | Duration |

The `endpoint` label holds the route template, such as `/v1/products/:productId`, not the actual path. If each product ID got its own label value, every new ID would create new time series, and Prometheus's memory use would grow without limit. Requests to paths that match no route are labeled `unmatched`. Prometheus's own requests to `/metrics` are not counted.

The histogram's buckets range from 0.1ms to about 3.3s, with each bucket double the size of the previous one. Prometheus's default buckets start at 5ms, but nearly every request to this in-memory service finishes in under 1ms. With the default buckets, every request would fall into the first bucket, and p50 and p95 would both be reported as "somewhere under 5ms".

### The dashboard's queries

The dashboard has one panel per signal, with one line per endpoint. You can also run these queries yourself at <http://localhost:9090/query>. Replace `$__rate_interval` with a time window such as `1m`.

**Rate**, in requests per second:

```promql
sum by (method, endpoint) (rate(http_requests_total[$__rate_interval]))
```

**Errors**, as a fraction of requests:

```promql
(
  sum by (method, endpoint) (rate(http_request_errors_total[$__rate_interval]))
  or sum by (method, endpoint) (rate(http_requests_total[$__rate_interval])) * 0
)
/ sum by (method, endpoint) (rate(http_requests_total[$__rate_interval]))
```

The `or ... * 0` part makes an endpoint that has never returned an error show as 0%. Without it, the endpoint has no error series at all, and the panel shows "No data" for it.

**Duration**, at p95 (use `0.50` for p50):

```promql
histogram_quantile(0.95, sum by (le, method, endpoint) (rate(http_request_duration_seconds_bucket[$__rate_interval])))
```

These latencies are measured inside the server. Locust's response times are higher because they also include the network and Locust's own overhead. The difference between the two tells you where time is being spent: if Locust's p95 rises but the server's doesn't, the delay is outside the service.

## The logs

Each request produces one JSON line on stdout:

```json
{"time":"2026-10-18T22:14:41.463668755Z","level":"INFO","msg":"request","method":"GET","endpoint":"/v1/products/:productId","path":"/v1/products/60","status":404,"duration_ms":0.020834}
```

`endpoint` is the same route template the metrics use. `path` is the URL that was actually requested, so you can see which product ID was involved.

### Getting RED from local logs

This command calculates each endpoint's request count, error percentage, p50 and p95 from the logs of the Docker Compose service:

```bash
docker compose logs --no-log-prefix product-service \
  | jq -R 'fromjson? | select(.msg == "request")' \
  | jq -s 'group_by(.method + " " + .endpoint) | map(
      (map(.duration_ms) | sort) as $d | {
        endpoint: (.[0].method + " " + .[0].endpoint),
        requests: length,
        error_pct: (100 * (map(select(.status < 200 or .status > 299)) | length) / length),
        p50_ms: $d[(length * 0.50 | floor)],
        p95_ms: $d[(length * 0.95 | floor)]
      })'
```

`fromjson?` skips any lines that aren't JSON, such as startup messages.

### Getting RED from CloudWatch, for the AWS deployment

On ECS, the container's stdout goes to the CloudWatch log group `/ecs/CS6650L2`. CloudWatch Logs Insights reads the JSON fields automatically. In the AWS console, open **CloudWatch → Logs Insights**, select the log group, and run:

**Requests, p50 and p95 per endpoint:**

```
filter msg = "request"
| stats count(*) as requests, pct(duration_ms, 50) as p50_ms, pct(duration_ms, 95) as p95_ms by method, endpoint
```

**Errors per endpoint and status code:**

```
filter msg = "request" and (status < 200 or status > 299)
| stats count(*) as errors by method, endpoint, status
```

**Request rate over time**, which Insights can display as a line chart under **Visualization**:

```
filter msg = "request"
| stats count(*) as requests by bin(1m), endpoint
```

To get error percentages, divide each endpoint's error count by its request count.

The deployed service writes JSON logs only after you redeploy it with this code. See [Redeploying after a code change](../product-service/README.md#redeploying-after-a-code-change).
