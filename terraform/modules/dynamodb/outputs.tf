output "table_name" {
  description = "Name of the shopping carts table"
  value       = aws_dynamodb_table.carts.name
}

output "table_arn" {
  description = "ARN of the shopping carts table"
  value       = aws_dynamodb_table.carts.arn
}
