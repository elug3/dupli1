locals {
  db_host = aws_db_instance.dupli1.address
  db_port = aws_db_instance.dupli1.port

  auth_db_url = "postgres://${var.db_username}:${random_password.db_master.result}@${local.db_host}:${local.db_port}/${var.db_name}?sslmode=require"
  product_db_url = "postgres://${var.db_username}:${random_password.db_master.result}@${local.db_host}:${local.db_port}/${var.product_db_name}?sslmode=require"
}

resource "aws_secretsmanager_secret" "db_credentials" {
  name        = "${var.project_name}/${var.environment}/database"
  description = "Dupli1 RDS credentials and connection metadata"

  tags = {
    Environment = var.environment
    Project     = var.project_name
  }
}

resource "aws_secretsmanager_secret_version" "db_credentials" {
  secret_id = aws_secretsmanager_secret.db_credentials.id

  secret_string = jsonencode({
    username         = var.db_username
    password         = random_password.db_master.result
    host             = local.db_host
    port             = local.db_port
    engine           = "postgres"
    dbname_auth      = var.db_name
    dbname_product   = var.product_db_name
    auth_db_url      = local.auth_db_url
    product_db_url   = local.product_db_url
  })
}

resource "aws_secretsmanager_secret" "auth_db_url" {
  name        = "${var.project_name}/${var.environment}/auth-db-url"
  description = "Full DB_URL connection string for dupli1-auth"

  tags = {
    Environment = var.environment
    Project     = var.project_name
  }
}

resource "aws_secretsmanager_secret_version" "auth_db_url" {
  secret_id     = aws_secretsmanager_secret.auth_db_url.id
  secret_string = local.auth_db_url
}

resource "aws_secretsmanager_secret" "product_db_url" {
  name        = "${var.project_name}/${var.environment}/product-db-url"
  description = "Full DUPLI1_PRODUCT_DB connection string for dupli1-product"

  tags = {
    Environment = var.environment
    Project     = var.project_name
  }
}

resource "aws_secretsmanager_secret_version" "product_db_url" {
  secret_id     = aws_secretsmanager_secret.product_db_url.id
  secret_string = local.product_db_url
}
