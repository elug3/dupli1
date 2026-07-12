locals {
  db_host = aws_db_instance.dupli1.address
  db_port = aws_db_instance.dupli1.port

  auth_db_url    = "postgres://${var.db_username}:${random_password.db_master.result}@${local.db_host}:${local.db_port}/${var.db_name}?sslmode=require"
  product_db_url = "postgres://${var.db_username}:${random_password.db_master.result}@${local.db_host}:${local.db_port}/${var.product_db_name}?sslmode=require"
  order_db_url   = "postgres://${var.db_username}:${random_password.db_master.result}@${local.db_host}:${local.db_port}/${var.order_db_name}?sslmode=require"
  cart_db_url    = "postgres://${var.db_username}:${random_password.db_master.result}@${local.db_host}:${local.db_port}/${var.cart_db_name}?sslmode=require"
  payment_db_url = "postgres://${var.db_username}:${random_password.db_master.result}@${local.db_host}:${local.db_port}/${var.payment_db_name}?sslmode=require"

  owner_password_effective = var.owner_password != "" ? var.owner_password : random_password.owner.result
}

resource "random_password" "owner" {
  length  = 24
  special = false
}

resource "random_password" "jwt_secret" {
  length  = 48
  special = false
}

resource "random_password" "web_service" {
  length  = 32
  special = false
}

resource "random_password" "order_service" {
  length  = 32
  special = false
}

resource "aws_secretsmanager_secret" "db_credentials" {
  name        = "${var.project_name}/${var.environment}/database"
  description = "Dupli1 RDS credentials and connection metadata"

  tags = local.common_tags
}

resource "aws_secretsmanager_secret_version" "db_credentials" {
  secret_id = aws_secretsmanager_secret.db_credentials.id

  secret_string = jsonencode({
    username       = var.db_username
    password       = random_password.db_master.result
    host           = local.db_host
    port           = local.db_port
    engine         = "postgres"
    dbname_auth    = var.db_name
    dbname_product = var.product_db_name
    dbname_order   = var.order_db_name
    dbname_cart    = var.cart_db_name
    dbname_payment = var.payment_db_name
    auth_db_url    = local.auth_db_url
    product_db_url = local.product_db_url
    order_db_url   = local.order_db_url
    cart_db_url    = local.cart_db_url
    payment_db_url = local.payment_db_url
  })
}

resource "aws_secretsmanager_secret" "auth_db_url" {
  name        = "${var.project_name}/${var.environment}/auth-db-url"
  description = "Full DB_URL connection string for dupli1-auth"
  tags        = local.common_tags
}

resource "aws_secretsmanager_secret_version" "auth_db_url" {
  secret_id     = aws_secretsmanager_secret.auth_db_url.id
  secret_string = local.auth_db_url
}

resource "aws_secretsmanager_secret" "product_db_url" {
  name        = "${var.project_name}/${var.environment}/product-db-url"
  description = "Full DUPLI1_PRODUCT_DB connection string for dupli1-product"
  tags        = local.common_tags
}

resource "aws_secretsmanager_secret_version" "product_db_url" {
  secret_id     = aws_secretsmanager_secret.product_db_url.id
  secret_string = local.product_db_url
}

resource "aws_secretsmanager_secret" "order_db_url" {
  name        = "${var.project_name}/${var.environment}/order-db-url"
  description = "Full DUPLI1_ORDER_DB connection string for dupli1-order"
  tags        = local.common_tags
}

resource "aws_secretsmanager_secret_version" "order_db_url" {
  secret_id     = aws_secretsmanager_secret.order_db_url.id
  secret_string = local.order_db_url
}

resource "aws_secretsmanager_secret" "cart_db_url" {
  name        = "${var.project_name}/${var.environment}/cart-db-url"
  description = "Full DUPLI1_CART_DB connection string for dupli1-cart"
  tags        = local.common_tags
}

resource "aws_secretsmanager_secret_version" "cart_db_url" {
  secret_id     = aws_secretsmanager_secret.cart_db_url.id
  secret_string = local.cart_db_url
}

resource "aws_secretsmanager_secret" "payment_db_url" {
  name        = "${var.project_name}/${var.environment}/payment-db-url"
  description = "Full DUPLI1_PAYMENT_DB connection string for dupli1-payment"
  tags        = local.common_tags
}

resource "aws_secretsmanager_secret_version" "payment_db_url" {
  secret_id     = aws_secretsmanager_secret.payment_db_url.id
  secret_string = local.payment_db_url
}

resource "aws_secretsmanager_secret" "app" {
  name        = "${var.project_name}/${var.environment}/app"
  description = "Dupli1 application secrets (JWT, seeded accounts, S3 keys)"
  tags        = local.common_tags
}

resource "aws_secretsmanager_secret_version" "app" {
  secret_id = aws_secretsmanager_secret.app.id

  secret_string = jsonencode({
    jwt_secret                    = random_password.jwt_secret.result
    owner_email                   = var.owner_email
    owner_password                = local.owner_password_effective
    dupli1_web_service_email      = "dupli1-web@service.dupli1.com"
    dupli1_web_service_password   = random_password.web_service.result
    dupli1_order_service_email    = "dupli1-order@service.dupli1.com"
    dupli1_order_service_password = random_password.order_service.result
    s3_access_key                 = aws_iam_access_key.product_s3.id
    s3_secret_key                 = aws_iam_access_key.product_s3.secret
    s3_bucket                     = aws_s3_bucket.product_images.bucket
  })
}

# Individual secret strings for ECS task secret injection (valueFrom must be a plain string secret).
resource "aws_secretsmanager_secret" "jwt_secret" {
  name = "${var.project_name}/${var.environment}/jwt-secret"
  tags = local.common_tags
}

resource "aws_secretsmanager_secret_version" "jwt_secret" {
  secret_id     = aws_secretsmanager_secret.jwt_secret.id
  secret_string = random_password.jwt_secret.result
}

resource "aws_secretsmanager_secret" "owner_password" {
  name = "${var.project_name}/${var.environment}/owner-password"
  tags = local.common_tags
}

resource "aws_secretsmanager_secret_version" "owner_password" {
  secret_id     = aws_secretsmanager_secret.owner_password.id
  secret_string = local.owner_password_effective
}

resource "aws_secretsmanager_secret" "web_service_password" {
  name = "${var.project_name}/${var.environment}/web-service-password"
  tags = local.common_tags
}

resource "aws_secretsmanager_secret_version" "web_service_password" {
  secret_id     = aws_secretsmanager_secret.web_service_password.id
  secret_string = random_password.web_service.result
}

resource "aws_secretsmanager_secret" "order_service_password" {
  name = "${var.project_name}/${var.environment}/order-service-password"
  tags = local.common_tags
}

resource "aws_secretsmanager_secret_version" "order_service_password" {
  secret_id     = aws_secretsmanager_secret.order_service_password.id
  secret_string = random_password.order_service.result
}

resource "aws_secretsmanager_secret" "s3_access_key" {
  name = "${var.project_name}/${var.environment}/s3-access-key"
  tags = local.common_tags
}

resource "aws_secretsmanager_secret_version" "s3_access_key" {
  secret_id     = aws_secretsmanager_secret.s3_access_key.id
  secret_string = aws_iam_access_key.product_s3.id
}

resource "aws_secretsmanager_secret" "s3_secret_key" {
  name = "${var.project_name}/${var.environment}/s3-secret-key"
  tags = local.common_tags
}

resource "aws_secretsmanager_secret_version" "s3_secret_key" {
  secret_id     = aws_secretsmanager_secret.s3_secret_key.id
  secret_string = aws_iam_access_key.product_s3.secret
}
