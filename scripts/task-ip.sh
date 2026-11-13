#!/usr/bin/env bash
# Prints the URL of the running ECS task, e.g. http://203.0.113.7:8080
#
# Reads the region, cluster and service from the Terraform outputs, so it
# queries the region the stack was deployed to, not the AWS CLI's default.
# There's no load balancer, so the task's public IP changes on every deploy.
#
# Usage:
#   scripts/task-ip.sh

set -euo pipefail

REPO_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
PORT=8080

# -raw exits 0 and prints a warning on stdout when there's no state, so read
# the outputs as JSON (which is {} then) and pick the values out with jq.
OUTPUTS="$(terraform -chdir="$REPO_DIR/terraform" output -json 2>/dev/null || echo '{}')"
REGION="$(jq -r '.aws_region.value // empty' <<<"$OUTPUTS")"
CLUSTER="$(jq -r '.ecs_cluster_name.value // empty' <<<"$OUTPUTS")"
SERVICE="$(jq -r '.ecs_service_name.value // empty' <<<"$OUTPUTS")"
if [[ -z "$REGION" || -z "$CLUSTER" || -z "$SERVICE" ]]; then
  echo "No Terraform outputs found. Run terraform apply in terraform/ first." >&2
  exit 1
fi

TASK="$(aws ecs list-tasks --region "$REGION" --cluster "$CLUSTER" --service-name "$SERVICE" \
  --desired-status RUNNING --query 'taskArns[0]' --output text)"
if [[ -z "$TASK" || "$TASK" == "None" ]]; then
  echo "No running task yet in $CLUSTER ($REGION); wait until the ECS service shows RUNNING." >&2
  exit 1
fi

ENI="$(aws ecs describe-tasks --region "$REGION" --cluster "$CLUSTER" --tasks "$TASK" \
  --query "tasks[0].attachments[0].details[?name=='networkInterfaceId'].value" --output text)"
if [[ -z "$ENI" || "$ENI" == "None" ]]; then
  echo "The task has no network interface yet; wait until it shows RUNNING." >&2
  exit 1
fi

IP="$(aws ec2 describe-network-interfaces --region "$REGION" --network-interface-ids "$ENI" \
  --query 'NetworkInterfaces[0].Association.PublicIp' --output text)"
if [[ -z "$IP" || "$IP" == "None" ]]; then
  echo "The task has no public IP yet; wait until it shows RUNNING." >&2
  exit 1
fi

echo "http://$IP:$PORT"
