# Step II: DynamoDB Shopping Cart

**Contributors**

- **Khush:** DynamoDB table design (Part 1), Terraform and ECS deployment, consistency investigation (Part 3), CloudWatch monitoring (Part 6), same-laptop MySQL vs DynamoDB re-run
- **Dhruv:** shopping cart API on DynamoDB (Part 2), required 150-operation test (Part 4)

**Environment.** AWS us-east-1 (Learner Lab). The API runs on ECS Fargate (1 task, 0.25 vCPU, 512 MB) and stores carts in one DynamoDB table, `shopping-carts`. The same Terraform also creates the Step I MySQL database, so both versions run on identical infrastructure.

Full details: [reports/step2-dynamodb.md](https://github.com/mimiperezg/CS6650-hw08/blob/main/reports/step2-dynamodb.md)

## Key Results

**Required 150-operation test (Part 4)**

- DynamoDB completed **150/150 operations**, and get cart averaged **25.2 ms**, well under the 50 ms target.
- Run from the same laptop, MySQL and DynamoDB were **nearly identical**: **47.2 ms vs 49.9 ms average**, and get cart was **43.1 vs 43.0 ms**.
- Inside DynamoDB, a request took only **1.3 to 3.5 ms** (CloudWatch). Most of each response time is the network trip between the laptop and AWS.

**Partition strategy (Part 6)**

- During the 150-op test, **50 different carts each received exactly the same load**, and there were **0 throttling events**. No partition was hot.

**Eventual consistency (Part 3)**

- Through the API: **0 stale reads in 100** read-after-write checks.
- Directly against the table: **1 stale read in 500 (0.2%)**, correct on the very next read, **7.9 ms later**. With strongly consistent reads: 0 in 500.
- Strongly consistent reads cost **exactly twice** the read capacity, with no difference in speed.
- **No lost updates:** 10 clients adding to the same cart at once (100 adds) always ended at exactly 100.

## 1. Working Implementation

### 1.1 Table design [Khush]

In DynamoDB the table is designed around how data is accessed. All three endpoints work on **one cart by its ID** (create it, read it whole, add a product to it), so every decision follows from that:

| Decision | Choice | Why |
|---|---|---|
| Partition key | `cart_id` (Number) | Every request has the cart ID, so every operation is a direct lookup; each shopper uses their own cart, so load spreads over many keys |
| Sort key | None | The cart is always read whole, so it is stored as one item; splitting it per product would make every GET a multi-item query |
| Cart items | Embedded map `product_id -> quantity` | One read returns the whole cart; adding a product is one atomic update of one map entry, so concurrent adds cannot overwrite each other; 50 products fit easily in the 400 KB item limit |
| Tables | One | No JOINs in DynamoDB; two tables would mean two requests per GET |
| Secondary indexes | None | Nothing looks carts up by anything except `cart_id`; an index on `customer_id` would be needed for customer history |
| Capacity mode | On-demand | Shopping traffic is unpredictable and spiky; on-demand scales automatically and bills per request, so there is no throttling at peaks and no paying for idle capacity |
| Expiry | TTL on `expires_at` (7 days) | Carts are often abandoned; DynamoDB deletes expired carts automatically at no cost |

**Cart IDs.** The API needs integer IDs and DynamoDB has no auto-increment. Each new cart gets a **random positive 32-bit number**, saved only if that ID does not exist yet (a conditional write; on a collision the app retries). A shared counter item was rejected because every create would hit one key (a hot partition), and a UUID would break the integer ID type.

*Screenshot: `results/dynamodb-screenshots/dynamodb_table_overview.png`*

The deployed table: partition key `cart_id` (Number), no sort key, on-demand capacity.

*Screenshot: `results/dynamodb-screenshots/dynamodb_table_ttl.png`*

Time to Live enabled on `expires_at`.

### 1.2 API and infrastructure [Dhruv: API, Khush: infrastructure]

The DynamoDB version (`dynamo-service/`) implements the same three endpoints, bodies and error codes as Step I with the AWS SDK for Go v2:

| Endpoint | DynamoDB operation | How it works |
|---|---|---|
| `POST /v1/shopping-carts` (201) | PutItem | Condition "this `cart_id` does not exist"; retries with a new ID on a collision |
| `GET /v1/shopping-carts/{id}` (200) | GetItem | Reads the cart by key; normal or strongly consistent read set by the `CONSISTENT_READ` env var |
| `POST /v1/shopping-carts/{id}/items` (204) | UpdateItem | Adds to one map entry with the condition "this cart exists"; a failed condition returns 404 |

Terraform (`terraform/modules/dynamodb/`) creates the table, a CloudWatch dashboard and Contributor Insights next to the Step I MySQL module; `-var app_dir=dynamo-service` or `product-service` deploys either version.

## 2. Performance Testing Results

### 2.1 150-operation test compared with MySQL [Dhruv, Khush]

The Step I script (`loadtest/loadtest.py`) runs 50 rounds of create cart, add item, get cart, one request at a time. Times are measured on the laptop from sending a request to getting the answer; p95/p99 mean 95%/99% of requests were faster.

**Required runs** (each team against its own deployment, different laptops):

| | MySQL (Anton) | DynamoDB (Dhruv) | DynamoDB − MySQL (ms) |
|---|---|---|---|
| All 150 requests: average (ms) | 21.0 | 28.5 | +7.5 |
| All 150 requests: p95 / p99 (ms) | 29.2 / 33.4 | 29.6 / 32.9 | +0.4 / −0.5 |
| Create / add / get average (ms) | 21.0 / 22.6 / 19.6 | 32.9 / 27.5 / 25.2 | +11.9 / +4.9 / +5.6 |

Both were 150/150. Because the test sends one request at a time, each number is mostly network time, and these runs used different networks. **To compare fairly, both versions were re-run from the same laptop, 3 minutes apart [Khush]:**

| | MySQL | DynamoDB | DynamoDB − MySQL (ms) |
|---|---|---|---|
| **All 150 requests: average (ms)** | **47.2** | **49.9** | **+2.7** |
| All 150 requests: p95 / p99 (ms) | 59.7 / 76.7 | 62.7 / 93.7 | +3.0 / +17.0 |
| **Create cart: average (ms)** | **46.9** | **55.1** | **+8.2** |
| Add item: average (ms) | 51.6 | 51.7 | +0.1 |
| **Get cart: average (ms)** | **43.1** | **43.0** | **−0.1** |

- **Nearly identical, because the network dominates.** About 45 ms of each request is the laptop-to-AWS trip, while the database's own work is a few ms (1.3 to 3.5 ms inside DynamoDB). Even MySQL's JOIN and DynamoDB's single key lookup both finish in milliseconds at this size.
- **Create cart is slower on DynamoDB** mostly because of the first request of each run (186 ms), which opens new connections including the app's first HTTPS connection to DynamoDB; without it the gap shrinks to 52.4 vs 45.8 ms. The same first request and one other slow create set DynamoDB's p99.
- **Not tested:** many simultaneous users. That is where we would expect differences (MySQL's 25-connection pool and fixed instance vs DynamoDB's automatic scaling), covered in Step III Part 2.

### 2.2 Evidence the partition strategy works under load [Khush]

DynamoDB spreads a table over partitions by hashing the partition key; if many requests hit one key, its partition is overloaded and requests are throttled. CloudWatch and Contributor Insights (which ranks the most-accessed keys) were watched during Dhruv's 150-op run (2026-11-14, 17:09 to 17:14 EST):

| Measurement | Result |
|---|---|
| Distinct carts accessed | **50** |
| Load per cart | **Identical for every cart** (25 busiest checked) |
| Throttling events / system errors | **0 / 0** |
| Latency inside DynamoDB: get / create / add item | 1.29 / 3.54 / 3.26 ms |
| Capacity used: read / write units | 25 / 100 (0.5 per normal read, 1 per write) |

*Screenshot: `results/dynamodb-screenshots/dynamodb_contributor_insights_150op.png`*

The 10 most-accessed carts. Only one dot shows because all 10 sit on exactly the same value: no key stands out as hot.

*Screenshot: `results/dynamodb-screenshots/dynamodb_dashboard_150op.png`*

The DynamoDB dashboard for the same minutes: reads (1.3 ms) faster than writes (3.3 to 3.5 ms), capacity matching one small item per request, and no throttling or errors.

### 2.3 Eventual consistency behavior [Khush]

DynamoDB keeps three copies of each item. A **normal read** may hit a copy that has not received the latest write yet and return old data (a **stale read**); a **strongly consistent read** always returns the latest data at twice the read cost.

**Test 1: the three required scenarios through the API** (`consistency/consistency_test.py`, from a laptop): create then read (x50), add then read and check the quantity (x50), and 10 clients adding to the same cart at once (expected total 100).

| | Normal reads | Strong reads |
|---|---|---|
| Create then read: stale | 0 / 50 | 0 / 50 |
| Add then read: stale | 0 / 50 | 0 / 50 |
| 10 clients x 10 adds: final quantity | 100, 0 lost | 100, 0 lost |
| Read capacity | 52 units / 104 reads | 105 units / ~105 reads (**2x**) |

Test 1 could not show stale reads: copies catch up within milliseconds, but each request from the laptop takes 25 to 55 ms to arrive. **Test 2 read the table directly from inside AWS** (`consistency/direct_consistency_test.py` in CloudShell), 500 write-then-read checks of each kind:

| | Normal reads | Strong reads (control) |
|---|---|---|
| New cart then read: stale | **1 / 500 (0.2%)**, correct on the next read 7.9 ms later | 0 / 500 |
| Update then read: stale | 0 / 500 | 0 / 500 |

- **Stale reads are real but rare and short**, and only appeared when reading a brand-new cart. A user would practically never notice; a retry or a strong read for that one request removes the risk.
- **Concurrent updates are always safe**: each add is one atomic update of one item, so no update was ever lost.

### 2.4 What the results mean

- **Speed is a tie for this workload.** From the same laptop the databases were within 3 ms, so for shopping carts the choice should rest on data model, scaling and operations.
- **The key design spreads load evenly**, which is what lets the table scale to millions of carts.
- **Eventual consistency is a non-issue for carts**; where an app must see its own write, a strong read removes it at twice the read cost.

## 3. Implementation Notes [Khush]

**Partition key strategy and rationale.** Every access pattern (create, get, add item) is by cart ID, so `cart_id` is the partition key and every request is a direct lookup of one item. Because DynamoDB hashes the key and each shopper uses their own cart, traffic spreads evenly across partitions. Contributor Insights confirmed this during the 150-op test: 50 carts, identical load per cart, 0 throttling. The one place a hot key could have appeared, generating cart IDs, was handled with random IDs and a conditional write instead of a shared counter.

**Key differences from the MySQL implementation (Step I)**

| | MySQL (Step I) | DynamoDB (Step II) |
|---|---|---|
| Data layout | `carts` and `cart_items` tables linked by a foreign key | One item per cart, products embedded as a map |
| Get cart | JOIN across two tables | One GetItem by key |
| Safe concurrent add | Transaction with `SELECT ... FOR UPDATE` row lock | One atomic UpdateItem, no lock |
| Cart IDs | `AUTO_INCREMENT` | Random number plus "only if new" condition |
| Customer history | Index on `customer_id` | Would need a secondary index (not built) |
| Connections | Connection pool to configure (25 connections) | HTTPS requests through the AWS SDK; no pool to size |
| Capacity | Fixed instance size (db.t3.micro) | On-demand, scales automatically, billed per request |
| Network setup | Database in private subnets, security group from ECS only | Public AWS endpoint, access controlled by IAM role |

**Eventual consistency observations.** Normal reads can return old data for a few milliseconds after a write. We measured it once in 500 direct reads (a just-created cart briefly not found) and never through the API, where network delay gives the copies time to catch up. For a shopping cart this is acceptable. Where reading your own write matters, the application can use a strongly consistent read for that request at twice the read cost, or retry a "not found" once.

**NoSQL vs SQL trade-offs discovered**

- **The design is locked to the access patterns.** DynamoDB is fast and simple for "everything by cart ID", but every new kind of query (carts by customer, carts containing a product) needs a new index or a full table scan. MySQL can answer a new question with a new SQL query on the existing tables.
- **Performance was a tie for this workload.** From the same laptop, both averaged under 50 ms and differed by 2.7 ms; the network mattered far more than the database choice.
- **Data integrity moves into the application.** DynamoDB has no foreign keys and no column types beyond the key. Embedding items in the cart removes the orphaned-item problem, but the app is responsible for writing the right item shape.
- **Embedding only works for bounded data.** A cart of 50 products fits easily in one 400 KB item; unbounded lists (for example a customer's full order history) would need separate items under a sort key.
- **Less to operate.** There was no connection pool to tune, no instance size to choose and no private networking for the database, and capacity scaled by itself. The trade-off is cost per request instead of a fixed hourly price.
