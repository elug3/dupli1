output "rds_endpoint" {
  description = "RDS hostname."
  value       = aws_db_instance.schick.address
}

output "rds_port" {
  description = "RDS port."
  value       = aws_db_instance.schick.port
}

output "rds_identifier" {
  description = "RDS instance identifier."
  value       = aws_db_instance.schick.id
}

output "rds_security_group_id" {
  description = "Security group attached to RDS."
  value       = aws_security_group.rds.id
}

output "db_secret_arn" {
  description = "Secrets Manager ARN with database credentials and URLs."
  value       = aws_secretsmanager_secret.db_credentials.arn
}

output "auth_db_url_secret_arn" {
  description = "Secrets Manager ARN for schick-auth DB_URL."
  value       = aws_secretsmanager_secret.auth_db_url.arn
}

output "product_db_url_secret_arn" {
  description = "Secrets Manager ARN for schick-product SCHICK_PRODUCT_DB."
  value       = aws_secretsmanager_secret.product_db_url.arn
}

output "auth_db_url_template" {
  description = "Auth connection string with password redacted."
  value       = "postgres://${var.db_username}:<password>@${aws_db_instance.schick.address}:${aws_db_instance.schick.port}/${var.db_name}?sslmode=require"
}

output "product_db_url_template" {
  description = "Product connection string with password redacted."
  value       = "postgres://${var.db_username}:<password>@${aws_db_instance.schick.address}:${aws_db_instance.schick.port}/${var.product_db_name}?sslmode=require"
}
