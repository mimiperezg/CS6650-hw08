output "address" {
  description = "Hostname of the MySQL instance"
  value       = aws_db_instance.this.address
}

output "port" {
  description = "Port of the MySQL instance"
  value       = aws_db_instance.this.port
}

output "db_name" {
  description = "Database name"
  value       = aws_db_instance.this.db_name
}

output "username" {
  description = "Master username"
  value       = aws_db_instance.this.username
}

output "password" {
  description = "Master password"
  value       = random_password.db.result
  sensitive   = true
}
