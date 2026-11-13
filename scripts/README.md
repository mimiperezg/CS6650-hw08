# Scripts

Helpers for testing the deployed shopping cart service. Run them from the repo root. They work the same whether the service stores carts in MySQL or DynamoDB, because both go through the same ECS service and the same API.

| Script | What it does |
|---|---|
| [`task-ip.sh`](task-ip.sh) | Prints the URL of the running ECS task |
| [`run-load.sh`](run-load.sh) | Runs the cart load test at 10, 50 and 100 users and saves the results |

## Prerequisites

- The stack is deployed with `terraform apply` in `terraform/`, and the ECS task is running.
- The AWS CLI is logged in to the same account (`aws sts get-caller-identity`).
- `jq` is installed (`brew install jq`).
- Locust is installed in `locust/.venv`:

  ```bash
  python3 -m venv locust/.venv
  locust/.venv/bin/pip install -r locust/requirements.txt
  ```

## task-ip.sh

```bash
scripts/task-ip.sh
# http://203.0.113.7:8080
```

There's no load balancer, so the task gets a new public IP every time it starts. This script looks it up.

- It reads the region, cluster and service names from the Terraform outputs (`aws_region`, `ecs_cluster_name`, `ecs_service_name`). It always queries the region the stack was deployed to, even if your AWS CLI defaults to another one.
- It exits with a message instead of a URL if Terraform has no state, or if the task isn't running yet or has no public IP yet. Wait a minute and run it again.

Use it anywhere a URL is needed, for example with the required 150-operation test:

```bash
locust/.venv/bin/python loadtest/loadtest.py "$(scripts/task-ip.sh)" results/mysql_test_results.json
```

## run-load.sh

```bash
scripts/run-load.sh <run> [host]
```

It runs `CartUser` from [`locust/cartfile.py`](../locust/cartfile.py) at 10, then 50, then 100 users, for 4 minutes each with a 60-second pause between levels. The pause makes each level show up as a separate step on CloudWatch's 1-minute graphs. The whole run takes about 15 minutes.

- **`<run>`**: a name for this run, such as `1`. Results go to `results/<store>-load-<run>/`. An existing folder is never overwritten, so use a new name for each attempt.
- **`[host]`**: the service URL. If you leave it out, the script gets it from `task-ip.sh`.

Before starting, it checks that the service answers at `<host>/metrics`. If a level has failed requests, it records that and still runs the next level.

### Output

```
results/<store>-load-<run>/
├── config.txt     run name, store, host, git commit, Locust version, settings,
│                  and each level's start and end time in UTC
├── u10/
│   ├── locust_stats.csv           totals per endpoint: requests, failures, percentiles
│   ├── locust_stats_history.csv   the same, every few seconds during the run
│   ├── locust_failures.csv        each kind of failure and how often it happened
│   ├── report.html                Locust's report with charts
│   └── locust.log                 Locust's console output
├── u50/
└── u100/
```

Use the times in `config.txt` as the time range when reading the metrics for each level in CloudWatch.

### Settings

Set these as environment variables before the command. All of them are optional.

| Variable | Default | What it changes |
|---|---|---|
| `STORE` | `mysql` | The results folder name: `results/<STORE>-load-<run>/`. It's also recorded in `config.txt`. It doesn't change the requests sent. |
| `DURATION` | `4m` | How long each level runs (`30s`, `2m`, and so on) |
| `GAP` | `60` | Seconds to wait between levels |

The user counts (10, 50, 100) and the spawn rate (10 users per second) are fixed, so every run is comparable.

## Using the scripts for DynamoDB

The DynamoDB service uses the same API, so the scripts and the Locust scenario work unchanged.

The Terraform variable `app_dir` picks which folder's Dockerfile builds the image ECS runs (default `product-service`). Terraform doesn't notice when only the source changes, so switching apps needs a forced rebuild and a task restart, from `terraform/`:

```bash
terraform apply -var app_dir=<dynamodb-app-folder> \
  -replace=docker_image.app -replace=docker_registry_image.app
aws ecs update-service --region "$(terraform output -raw aws_region)" \
  --cluster CS6650L2-cluster --service CS6650L2 --force-new-deployment
```

Switch back the same way with `-var app_dir=product-service`. Once the new task is running, name the results after the store:

```bash
STORE=dynamodb scripts/run-load.sh 1
# results go to results/dynamodb-load-1/
```

For the required test, only the output file name changes:

```bash
locust/.venv/bin/python loadtest/loadtest.py "$(scripts/task-ip.sh)" results/dynamodb_test_results.json
```

Keep `DURATION` and `GAP` at their defaults, and run from the same machine and region as the MySQL runs, so the MySQL and DynamoDB results can be compared directly.
