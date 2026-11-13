# Region to deploy into
variable "aws_region" {
  type    = string
  default = "us-east-1"
}

# ECR & ECS settings
variable "ecr_repository_name" {
  type    = string
  default = "ecr_service"
}

variable "service_name" {
  type    = string
  default = "CS6650L2"
}

# Folder (relative to the repo root) whose Dockerfile builds the image ECS
# runs. Lets the MySQL and DynamoDB versions of the service live in separate
# folders and be deployed on the same infrastructure.
variable "app_dir" {
  type    = string
  default = "product-service"
}

variable "container_port" {
  type    = number
  default = 8080
}

variable "ecs_count" {
  type    = number
  default = 1
}

# How long to keep logs
variable "log_retention_days" {
  type    = number
  default = 7
}
