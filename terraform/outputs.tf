output "ecs_cluster_name" {
  description = "Name of the created ECS cluster"
  value       = module.ecs.cluster_name
}

output "ecs_service_name" {
  description = "Name of the running ECS service"
  value       = module.ecs.service_name
}

output "rds_address" {
  description = "Private hostname of the MySQL instance"
  value       = module.rds.address
}

output "aws_region" {
  description = "Region everything was deployed to"
  value       = var.aws_region
}

output "dynamodb_table_name" {
  description = "Name of the DynamoDB shopping carts table"
  value       = module.dynamodb.table_name
}
