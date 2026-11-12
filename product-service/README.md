# Product Service

A Go service built with [Gin](https://github.com/gin-gonic/gin) that implements the Product endpoints from [`../api.yaml`](../api.yaml). Products are stored in memory, so they are lost when the server stops.

| Method | Path | Description |
|--------|------|-------------|
| `GET`  | `/v1/products/{productId}` | Get a product by ID |
| `POST` | `/v1/products/{productId}/details` | Add a product, or update an existing one |
| `GET`  | `/metrics` | Request rate, error and latency metrics for Prometheus |

Every request is also logged to stdout as one JSON line, with its endpoint, status code and duration. To run the service with a Prometheus and Grafana dashboard on your machine, see [Observability](../observability/README.md).

## Running the service

The server listens on port `8080`.

**Locally** (requires Go 1.27+):

```bash
cd hw5/product-service
go run .
```

**With Docker** (build from this directory, where the `Dockerfile` is):

```bash
cd hw5/product-service
docker build -t product-service .
docker run -p 8080:8080 product-service
```

The examples below use `localhost:8080`. If the service is deployed elsewhere, replace that with the server's address.

## Deploying to AWS

The Terraform configuration in [`../terraform`](../terraform) deploys the service to AWS. In a single `terraform apply`, it:

1. creates an ECR repository (a private Docker image registry) named `ecr_service`,
2. builds the Docker image on your machine and pushes it to that repository,
3. creates a security group that allows inbound traffic on port `8080`,
4. runs the image as a Fargate task in an ECS cluster, in the account's default VPC, with a public IP,
5. sends the container's logs to a CloudWatch log group.

### Prerequisites

Install these on the machine you deploy from:

| Tool | Version | Why |
|------|---------|-----|
| [Terraform](https://developer.hashicorp.com/terraform/install) | 1.2 or newer | Creates the infrastructure |
| [Docker](https://docs.docker.com/get-docker/) | Any recent version, **running** | Builds and pushes the image |
| [AWS CLI](https://docs.aws.amazon.com/cli/latest/userguide/getting-started-install.html) | v2 | Stores your credentials; finds the public IP |

You also need an AWS account that has an IAM role named **`LabRole`**. AWS Academy Learner Lab accounts include this role. ECS uses it to pull the image from ECR and to write logs. In any other AWS account, create a role with that name that ECS tasks can assume, with the `AmazonECSTaskExecutionRolePolicy` policy attached. Otherwise, `terraform apply` fails with a "role not found" error.

### 1. Get the code

```bash
git clone https://github.com/AntonMenchaca/distributed-systems-6650.git
cd distributed-systems-6650/hw5/terraform
```

### 2. Set up AWS credentials

Terraform and the Docker push use the credentials from your AWS CLI setup.

- **AWS Academy Learner Lab:** start the lab, open **AWS Details**, and click **Show** next to **AWS CLI**. Copy the whole block, including `aws_session_token`, into `~/.aws/credentials`, replacing whatever is there.
- **Any other AWS account:** run `aws configure` and enter your access key, secret key, and the region `us-east-1`.

The Learner Lab credentials block doesn't include a region, and the `aws` commands below need one. Set it once:

```bash
aws configure set region us-east-1
```

Check that the credentials work:

```bash
aws sts get-caller-identity
```

This should print your account ID. If you get `The security token included in the request is invalid`, your credentials have expired. Learner Lab credentials expire when the lab session ends, so copy fresh ones each time you start the lab.

### 3. Deploy

```bash
terraform init      # downloads the AWS and Docker providers (first time only)
terraform plan      # optional: preview what will be created
terraform apply     # type "yes" to confirm
```

The first apply takes a few minutes, mostly to build and push the image.

**Settings you can change** (defaults are in [`../terraform/variables.tf`](../terraform/variables.tf)):

| Variable | Default | Meaning |
|----------|---------|---------|
| `aws_region` | `us-east-1` | AWS region to deploy into |
| `ecr_repository_name` | `ecr_service` | Name of the ECR repository |
| `service_name` | `CS6650L2` | Prefix for the cluster, service, security group, and log group names |
| `container_port` | `8080` | Port the app listens on and the security group opens |
| `ecs_count` | `1` | Number of tasks to run |
| `log_retention_days` | `7` | How long CloudWatch keeps logs |

Override one with `-var`, for example:

```bash
terraform apply -var="ecs_count=2"
```

### 4. Find the public IP

Terraform doesn't output the IP, because ECS assigns it when the task starts. After the task reaches the **RUNNING** state (about a minute after `apply` finishes), run:

```bash
TASK=$(aws ecs list-tasks --cluster CS6650L2-cluster --service-name CS6650L2 \
  --query 'taskArns[0]' --output text)
ENI=$(aws ecs describe-tasks --cluster CS6650L2-cluster --tasks "$TASK" \
  --query "tasks[0].attachments[0].details[?name=='networkInterfaceId'].value" --output text)
aws ec2 describe-network-interfaces --network-interface-ids "$ENI" \
  --query 'NetworkInterfaces[0].Association.PublicIp' --output text
```

If you changed `service_name`, replace `CS6650L2` with your value.

You can also find it in the AWS console: **ECS → Clusters → CS6650L2-cluster → Tasks →** select the running task, and read **Public IP** on the **Configuration** tab.

Then call the API, **including port `8080`**:

```bash
curl http://<PUBLIC_IP>:8080/v1/products/1
```

The first call returns a `404`, because no products exist yet. Use the POST example in [Add or update a product](#add-or-update-a-product) to add one. Opening `http://<PUBLIC_IP>` without the port times out, and the bare path `/` returns `404 page not found`; both are expected.

The public IP changes every time the task restarts, so look it up again after a redeploy.

### Redeploying after a code change

Terraform doesn't watch the Go source files, so a plain `terraform apply` after editing the code may not build or push a new image. To force a rebuild and push, run:

```bash
terraform apply -replace=docker_image.app -replace=docker_registry_image.app
```

Running tasks keep using the old image, so restart them to pull the new `:latest`:

```bash
aws ecs update-service --cluster CS6650L2-cluster --service CS6650L2 --force-new-deployment
```

### Troubleshooting

| Symptom | Cause | Fix |
|---------|-------|-----|
| `The security token included in the request is invalid` | AWS credentials have expired | Copy fresh credentials into `~/.aws/credentials` (step 2) |
| `Cannot connect to the Docker daemon` | Docker isn't running, or Terraform can't find it | Start Docker Desktop (or the Docker service) and run `apply` again. On a Mac, also check that **Settings → Advanced → Allow the default Docker socket to be used** is turned on. |
| `You must specify a region` | The AWS CLI has no region set | Run `aws configure set region us-east-1` |
| Task stops with `exec /docker-product-service: exec format error` | The image was built for the wrong CPU type | Keep `platform = "linux/amd64"` in the `docker_image` build block in [`../terraform/main.tf`](../terraform/main.tf); Fargate runs on x86 by default. On Apple Silicon, Docker emulates x86 for this, so the build is slower. |
| `RepositoryAlreadyExistsException` or `... already exists` | Resources from an earlier deployment are still in the account, but this machine's Terraform state doesn't know about them | Run `terraform destroy` from the machine that created them, or delete them in the AWS console, then apply again |
| Requests to the IP time out | Port missing from the URL, or the task isn't running | Use `http://<PUBLIC_IP>:8080/...`, and check that the task is **RUNNING** |

### Tearing down

Delete everything this configuration created:

```bash
terraform destroy
```

Terraform tracks what it created in `terraform.tfstate`, which is ignored by git. So run `destroy` from the same machine and directory that ran `apply`.

## The Product object

Both endpoints use this JSON object. **Every field is required.**

```json
{
  "product_id": 1,
  "sku": "ABC-123-XYZ",
  "manufacturer": "Acme Corporation",
  "category_id": 456,
  "weight": 1250,
  "some_other_id": 789
}
```

| Field | Type | Rules |
|-------|------|-------|
| `product_id` | integer | 1 or greater; must match `productId` in the URL |
| `sku` | string | 1–100 characters |
| `manufacturer` | string | 1–200 characters |
| `category_id` | integer | 1 or greater |
| `weight` | integer | weight in grams; 0 or greater |
| `some_other_id` | integer | 1 or greater |

All integers must be whole numbers that fit in 32 bits (at most 2,147,483,647). Numbers sent as strings, such as `"456"`, are rejected.

## Add or update a product

`POST /v1/products/{productId}/details`

Saves the product in the request body. If a product with that ID already exists, it is replaced.

```bash
curl -i -X POST localhost:8080/v1/products/1/details \
  -H 'Content-Type: application/json' \
  -d '{
    "product_id": 1,
    "sku": "ABC-123-XYZ",
    "manufacturer": "Acme Corporation",
    "category_id": 456,
    "weight": 1250,
    "some_other_id": 789
  }'
```

**Success:** `204 No Content`, with an empty body.

```
HTTP/1.1 204 No Content
```

**Invalid input:** `400 Bad Request`. This happens when a field is missing or breaks a rule, the JSON is malformed, the URL's ID isn't a positive integer, or the body's `product_id` doesn't match the URL. For example, posting product 1's body to `/v1/products/2/details` returns:

```json
{
  "error": "INVALID_INPUT",
  "message": "The provided input data is invalid",
  "details": "product_id in body (1) does not match productId in path (2)"
}
```

Leaving out a required field (here, `category_id`) returns:

```json
{
  "error": "INVALID_INPUT",
  "message": "The provided input data is invalid",
  "details": "Key: 'Product.CategoryID' Error:Field validation for 'CategoryID' failed on the 'required' tag"
}
```

## Get a product

`GET /v1/products/{productId}`

```bash
curl -i localhost:8080/v1/products/1
```

**Success:** `200 OK`, with the product.

```json
{
  "product_id": 1,
  "sku": "ABC-123-XYZ",
  "manufacturer": "Acme Corporation",
  "category_id": 456,
  "weight": 1250,
  "some_other_id": 789
}
```

**Not found:** `404 Not Found`, when no product has that ID.

```json
{
  "error": "PRODUCT_NOT_FOUND",
  "message": "Product not found",
  "details": "no product with ID 99"
}
```

An ID that isn't a positive integer, such as `/v1/products/abc`, also returns `404`, because `api.yaml` defines no `400` response for this endpoint.

## Error responses

Every error uses the `Error` format from `api.yaml`:

| Field | Description |
|-------|-------------|
| `error` | Error code: `INVALID_INPUT`, `PRODUCT_NOT_FOUND`, or `INTERNAL_ERROR` |
| `message` | Short, readable description |
| `details` | What exactly went wrong (may be left out) |

An unexpected server error returns `500 Internal Server Error` with the code `INTERNAL_ERROR`.
