output "vpc_id" {
  description = "VPC ID."
  value       = aws_vpc.main.id
}

output "public_subnet_ids" {
  description = "Public subnet IDs (ALB + ECS)."
  value       = aws_subnet.public[*].id
}

output "private_subnet_ids" {
  description = "Private subnet IDs (RDS)."
  value       = aws_subnet.private[*].id
}

output "alb_dns_name" {
  description = "Public ALB DNS name."
  value       = aws_lb.main.dns_name
}

output "alb_url" {
  description = "Base URL for the gateway."
  value       = var.certificate_arn != "" ? "https://${aws_lb.main.dns_name}" : "http://${aws_lb.main.dns_name}"
}

output "ecs_cluster_name" {
  description = "ECS cluster name (set GitHub variable ECS_CLUSTER to this)."
  value       = aws_ecs_cluster.main.name
}

output "ecr_repository_urls" {
  description = "ECR repository URLs keyed by short service name."
  value       = { for k, r in aws_ecr_repository.service : k => r.repository_url }
}

output "rds_endpoint" {
  description = "RDS hostname."
  value       = aws_db_instance.dupli1.address
}

output "rds_port" {
  description = "RDS port."
  value       = aws_db_instance.dupli1.port
}

output "rds_identifier" {
  description = "RDS instance identifier."
  value       = aws_db_instance.dupli1.id
}

output "rds_security_group_id" {
  description = "Security group attached to RDS."
  value       = aws_security_group.rds.id
}

output "s3_product_images_bucket" {
  description = "S3 bucket for product images."
  value       = aws_s3_bucket.product_images.bucket
}

output "cloudwatch_log_groups" {
  description = "CloudWatch log group names."
  value       = { for k, g in aws_cloudwatch_log_group.service : k => g.name }
}

output "db_secret_arn" {
  description = "Secrets Manager ARN with database credentials and URLs."
  value       = aws_secretsmanager_secret.db_credentials.arn
}

output "auth_db_url_secret_arn" {
  description = "Secrets Manager ARN for dupli1-auth DB_URL."
  value       = aws_secretsmanager_secret.auth_db_url.arn
}

output "product_db_url_secret_arn" {
  description = "Secrets Manager ARN for dupli1-product DUPLI1_PRODUCT_DB."
  value       = aws_secretsmanager_secret.product_db_url.arn
}

output "app_secret_arn" {
  description = "Secrets Manager ARN for application secrets bundle."
  value       = aws_secretsmanager_secret.app.arn
}

output "owner_email" {
  description = "Seeded owner email."
  value       = var.owner_email
}

output "service_discovery_domain" {
  description = "Cloud Map private DNS domain."
  value       = local.service_domain
}

output "auth_db_url_template" {
  description = "Auth connection string with password redacted."
  value       = "postgres://${var.db_username}:<password>@${aws_db_instance.dupli1.address}:${aws_db_instance.dupli1.port}/${var.db_name}?sslmode=require"
}

output "product_db_url_template" {
  description = "Product connection string with password redacted."
  value       = "postgres://${var.db_username}:<password>@${aws_db_instance.dupli1.address}:${aws_db_instance.dupli1.port}/${var.product_db_name}?sslmode=require"
}

output "github_actions_config" {
  description = "Values to set in GitHub Actions variables/secrets."
  value = {
    AWS_REGION  = var.aws_region
    ECS_CLUSTER = aws_ecs_cluster.main.name
  }
}
