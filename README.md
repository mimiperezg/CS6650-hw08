# CS6650 HW08: MySQL vs DynamoDB Shopping Cart

A shopping cart API built twice on the same AWS infrastructure: once on Amazon RDS MySQL (Step I) and once on DynamoDB (Step II), then compared using our own test data (Step III).

Both versions implement the same three endpoints from the HW5 API spec ([`api.yaml`](api.yaml)):

| Endpoint | Success | What it does |
|---|---|---|
| `POST /v1/shopping-carts` | 201 | Create a cart for a customer, returns `shopping_cart_id` |
| `GET /v1/shopping-carts/{id}` | 200 | Return the cart and all its items (404 if unknown) |
| `POST /v1/shopping-carts/{id}/items` | 204 | Add a product to the cart; adding it again increases the quantity |

## Team

| Step | Teammate | Contribution |
|---|---|---|
| Step I: MySQL | Milena | Schema design, shopping cart API, required test script |
| Step I: MySQL | Anton | RDS Terraform, ECS deployment, required test run, concurrent load test, CloudWatch |
| Step II: DynamoDB | Khush | Table design, DynamoDB Terraform and deployment, consistency tests, CloudWatch, same-laptop comparison |
| Step II: DynamoDB | Dhruv | DynamoDB API, required test run |
| Step III: Comparison | Dhruv | Data verification, performance comparison tables |
| Step III: Comparison | Khush | Consistency assessment, learning notes, final assembly |
| Step III: Comparison | Anton | Resource efficiency analysis |
| Step III: Comparison | Milena | Scenario and architecture recommendations |

## Repository layout

| Path | Contents |
|---|---|
| [`product-service/`](product-service/) | Go API on MySQL (Step I): `carts.go` handlers, `db.go` connection pool and table creation |
| [`dynamo-service/`](dynamo-service/) | Go API on DynamoDB (Step II), same endpoints and error format |
| [`schema.sql`](schema.sql) | MySQL schema |
| [`terraform/`](terraform/) | One shared config: network, ECR, ECS, logging, and the `rds` and `dynamodb` modules |
| [`loadtest/`](loadtest/) | `loadtest.py`, the required 150-operation test |
| [`locust/`](locust/) | `cartfile.py`, the concurrent cart load test ([README](locust/README.md)) |
| [`scripts/`](scripts/) | `task-ip.sh` (find the task URL) and `run-load.sh` (10/50/100-user runs) ([README](scripts/README.md)) |
| [`consistency/`](consistency/) | DynamoDB read-after-write tests, through the API and directly against the table |
| [`results/`](results/) | All test output and screenshots (see [Results](#results)) |
| [`reports/`](reports/) | Step I report ([MySQL](reports/step1-mysqldb.md)) and Step II report ([full](reports/step2-dynamodb.md), [group version](reports/step2-dynamodb-group-version.md)) |

## Infrastructure

Everything ran in AWS us-east-1 (Learner Lab) on the single-task base from HW5: one ECS Fargate task (0.25 vCPU, 512 MB) with a public IP and no load balancer. The same Terraform creates both databases, so both versions run on identical infrastructure.

- **MySQL:** RDS MySQL 8.0 on db.t3.micro in two private subnets. Only the ECS tasks' security group can reach port 3306.
- **DynamoDB:** one on-demand table, `shopping-carts`, reached through the task's IAM role.
- **Choosing the app:** the `app_dir` variable picks which folder's Dockerfile is built and deployed (`product-service` or `dynamo-service`). Each store's environment variables are merged into the ECS task, so neither group edits the other's code.

## How to run

Prerequisites: Terraform, Docker, the AWS CLI logged in to the Learner Lab account (`aws sts get-caller-identity`), `jq`, and Python 3.9+.

**1. Deploy** (from `terraform/`):

```bash
terraform init
terraform apply                                  # MySQL version (default app_dir=product-service)
terraform apply -var app_dir=dynamo-service      # DynamoDB version
```

Other variables: `aws_region` (default `us-east-1`) and `consistent_read` (`true` for strongly consistent DynamoDB reads). Switching an already-deployed stack to the other app needs a forced image rebuild and task restart; see [scripts/README.md](scripts/README.md#using-the-scripts-for-dynamodb).

**2. Set up the test clients** (from the repo root):

```bash
python3 -m venv locust/.venv
locust/.venv/bin/pip install -r locust/requirements.txt
```

**3. Required 150-operation test:**

```bash
locust/.venv/bin/python loadtest/loadtest.py "$(scripts/task-ip.sh)" results/mysql_test_results.json
locust/.venv/bin/python loadtest/loadtest.py "$(scripts/task-ip.sh)" results/dynamodb_test_results.json
```

**4. Concurrent load test** (10, 50 and 100 users, 4 minutes each, about 15 minutes total):

```bash
scripts/run-load.sh 1                    # results/mysql-load-1/
STORE=dynamodb scripts/run-load.sh 1     # results/dynamodb-load-1/
```

**5. Consistency tests** (DynamoDB):

```bash
locust/.venv/bin/python consistency/consistency_test.py "$(scripts/task-ip.sh)" dynamo-eventual consistency/results/dynamo-eventual.json
python3 consistency/direct_consistency_test.py eventual    # in CloudShell, us-east-1
python3 consistency/direct_consistency_test.py strong
```

**6. Tear down** (from `terraform/`). RDS bills by the hour until it is deleted:

```bash
terraform destroy -auto-approve
```

## Step I: MySQL

### Steps taken

1. Added an `rds` Terraform module: MySQL 8.0 on db.t3.micro, two private subnets, a DB subnet group, a security group open only to the ECS tasks, and a generated password passed to the task as `DB_*` environment variables.
2. Designed the schema and implemented the three endpoints in Go with a connection pool.
3. Ran the required 150-operation test against the ECS deployment.
4. Ran a concurrent Locust load test at 10, 50 and 100 users, logging each level's UTC window.
5. Captured RDS and ECS CloudWatch metrics for the test window, then destroyed the stack.

### Key decisions

| Decision | Choice | Why |
|---|---|---|
| Tables | `carts` and `cart_items`, linked by a foreign key | A cart has a variable number of items and must exist before anything is added; two tables store each fact once |
| Item key | Composite primary key `(cart_id, product_id)` | One row per product per cart; enables the upsert and keeps a cart's items together so get cart needs no extra index |
| Integrity | `ON DELETE CASCADE`, `CHECK (quantity > 0)` | No orphaned items and no invalid quantities, enforced by the database |
| Customer lookup | Index on `customer_id` | Supports customer history without a full table scan |
| Get cart | One `LEFT JOIN` query | Returns the cart and its items in one round trip; an empty cart still returns 200 |
| Add item | Transaction with `SELECT ... FOR UPDATE`, then `INSERT ... ON DUPLICATE KEY UPDATE` | Locks only that cart's row, so concurrent adds to one cart can't overwrite each other while other carts aren't blocked |
| Connection pool | 25 open, 25 idle, 5-minute lifetime | Leaves headroom under db.t3.micro's connection limit for each ECS task's pool |
| Private subnets | Own route table with only the local route, `publicly_accessible = false` | New subnets in the default VPC inherit its main route table, which routes to the internet gateway |
| AZ selection | Skip Local Zones | In us-west-2, a Local Zone (`us-west-2-lax-1a`) sorts ahead of `us-west-2a` and may not offer db.t3.micro |
| Load testing | Locust at 10/50/100 users in addition to the required test | The required test is sequential, so it can't show pool behavior or performance under different loads |

Full details: [reports/step1-mysqldb.md](reports/step1-mysqldb.md).

## Step II: DynamoDB

### Steps taken

1. Designed the table around the access patterns: every operation is on one cart by its ID.
2. Added a `dynamodb` Terraform module (table, CloudWatch dashboard, Contributor Insights) next to the RDS module.
3. Implemented the same three endpoints with the AWS SDK for Go v2 in `dynamo-service/`.
4. Ran the required 150-operation test and monitored partition load and throttling in CloudWatch.
5. Tested eventual consistency through the API and directly against the table from CloudShell.
6. Re-ran both versions from the same laptop for a fair comparison.

### Key decisions

| Decision | Choice | Why |
|---|---|---|
| Partition key | `cart_id` (Number), no sort key | Every request has the cart ID, so every operation is a direct lookup, and load spreads over many keys |
| Cart items | Embedded map `product_id -> quantity` | One read returns the whole cart; adding a product is one atomic update of one item |
| Cart IDs | Random 32-bit number with a conditional write | No auto-increment in DynamoDB; a shared counter would be a hot key |
| Indexes | None | Nothing looks carts up by anything except `cart_id` |
| Capacity | On-demand | Shopping traffic is spiky; no capacity to plan and no throttling at peaks |
| Expiry | TTL on `expires_at` (7 days) | Abandoned carts are deleted automatically at no cost |
| Read mode | Eventually consistent by default, switchable with `consistent_read` | Half the read cost; a strong read is available where a user must see their own write |

Full details: [reports/step2-dynamodb.md](reports/step2-dynamodb.md).

## Step III: Comparison

1. Merged both required result files into [`results/combined_results.json`](results/combined_results.json) (300 records, each tagged with its database) and verified the counts.
2. Compared performance overall and per operation. Because the two required runs came from different laptops, both versions were also re-run from one laptop 3 minutes apart ([`results/same-client/`](results/same-client/)).
3. Assessed consistency (ACID vs eventual), resource efficiency and scaling, then made recommendations for four real-world scenarios.

## Results

| Test | MySQL | DynamoDB |
|---|---|---|
| Required test, 150 operations | 150/150, 21.0 ms average, get cart 19.6 ms | 150/150, 28.5 ms average, get cart 25.2 ms |
| Same laptop, 3 minutes apart | 47.2 ms average, get cart 43.1 ms | 49.9 ms average, get cart 43.0 ms |
| Concurrent load, 10/50/100 users | 0 failures; 7.9 to 77.7 req/s at a flat 20 to 21 ms median; connections peaked at 14 of 25 | Not load-tested |
| Partition load | n/a | 50 carts, identical load, 0 throttling |
| Consistency | Single copy, every read sees the last committed write | 0 stale reads in 100 through the API; 1 in 500 directly against the table, correct on the next read |

**Takeaways:**

- **Performance was a tie.** From the same laptop, the databases were within 3 ms; inside DynamoDB a request took only 1.3 to 3.5 ms, so the network dominated every measurement. The required runs' 7.5 ms gap came from the two different clients.
- **Shopping carts fit DynamoDB:** every access is by cart ID, carts expire, and concurrent adds are atomic without locks. MySQL is the better fit for data with relationships, multi-row transactions or new query patterns, such as orders, inventory and reporting.
- **The operational cost differs:** MySQL needed private networking, a connection pool and an instance size; DynamoDB moved that effort into the code (random IDs, conditional writes, typed values).

### Where the results are

| Path | Contents |
|---|---|
| `results/mysql_test_results.json`, `results/dynamodb_test_results.json` | Required 150-operation test results |
| `results/combined_results.json` | Both required results merged, used for all Step III analysis |
| `results/same-client/` | Both versions re-run from one laptop |
| `results/mysql-load-1/` | MySQL Locust runs at 10, 50 and 100 users (CSV, HTML report, log, `config.txt` with UTC windows) |
| `results/mysql-screenshots/` | RDS console and CLI, CloudWatch (RDS and ECS) and Locust screenshots |
| `results/dynamodb-screenshots/` | Table settings, CloudWatch dashboard and Contributor Insights |
| `consistency/results/` | Consistency test results, through the API and direct |
