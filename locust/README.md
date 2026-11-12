# Load testing with Locust

[`locustfile.py`](locustfile.py) simulates users of the [Product Service](../product-service/README.md). Each user:

- adds a product with `POST /v1/products/{id}/details` (expects `204`), and
- reads back a product it added with `GET /v1/products/{id}` (expects `200` and the same `product_id`).

Reads run three times as often as writes. Each user waits 0.5–2 seconds between requests. Any other status code is counted as a failure.

## Choosing the HTTP client

The same tasks are available with two Locust HTTP clients:

| Class | Client | When to use it |
|-------|--------|----------------|
| `ProductHttpUser` | `HttpUser`, built on the `requests` library | Locust's standard client |
| `ProductFastHttpUser` | `FastHttpUser`, built on `geventhttpclient` | Uses much less CPU per request, so one machine can generate far more load |

With the web UI, start Locust with `--class-picker` and choose the class in the browser (see [Running](#running)). Without the UI, put the class name after `locust`.

If you neither pick nor name a class, Locust runs both and splits the users evenly between them, so both clients' requests are combined in the same results.

If Locust warns that its **CPU usage is above 90%**, the load-generating machine is the bottleneck, not the service. Switch to `ProductFastHttpUser`, or run Locust on a more powerful machine.

## Setup

Requires Python 3.9 or newer.

```bash
cd hw5/locust
python3 -m venv .venv
source .venv/bin/activate
pip install -r requirements.txt
```

## Running

Replace `<HOST>` with `localhost` for a local server, or with the ECS task's public IP (see [Find the public IP](../product-service/README.md#4-find-the-public-ip)).

**With the web UI:**

```bash
locust ProductHttpUser     --host http://<PUBLIC_IP>:8080                    # UI on :8089
locust ProductFastHttpUser --host http://<PUBLIC_IP>:8080 --web-port 8090    # UI on :8090
locust --class-picker --host http://<HOST>:8080
```

Open <http://localhost:8089>, choose `ProductHttpUser` or `ProductFastHttpUser` under **User Classes**, enter the number of users and the spawn rate, and click **Start**. The UI shows live request rates, response times, and failures, with charts under the **Charts** tab.

**Without the UI** (prints a summary when it finishes):

```bash
locust ProductFastHttpUser --host http://<HOST>:8080 --headless -u 50 -r 10 -t 1m
```

Replace `ProductFastHttpUser` with `ProductHttpUser` to use the standard client.

| Flag | Meaning |
|------|---------|
| `-u 50` | Simulate 50 users at once |
| `-r 10` | Start 10 new users per second until all 50 are running |
| `-t 1m` | Stop after 1 minute (`30s`, `5m`, and so on also work) |
| `--csv results` | Also save the stats to `results_stats.csv` and related files |
| `--html report.html` | Also save an HTML report with charts |

## Reading the results

| Column | Meaning |
|--------|---------|
| `# reqs` / `# fails` | Requests sent, and how many failed |
| `Avg`, `Med`, `Min`, `Max` | Response times in milliseconds |
| `req/s` | Throughput: requests completed per second |
| `50%` … `99%` | Response time percentiles. `95%` means 95% of requests were at least that fast. |

Response times from your machine to AWS include the network round trip, so they're much higher than the times you see against `localhost`.
