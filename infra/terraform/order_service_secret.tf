# Order service account used by dupli1-order to call product stock/reservations.
# Auth seeds/syncs the user on boot; order logs in and refreshes a Bearer token.

resource "random_password" "order_service" {
  length  = 32
  special = false
}

resource "aws_secretsmanager_secret" "order_service" {
  name        = "${var.project_name}/${var.environment}/order-service-account"
  description = "dupli1-order service account email/password for product stock reservations"

  tags = {
    Environment = var.environment
    Project     = var.project_name
  }
}

resource "aws_secretsmanager_secret_version" "order_service" {
  secret_id = aws_secretsmanager_secret.order_service.id
  secret_string = jsonencode({
    DUPLI1_ORDER_SERVICE_EMAIL    = var.order_service_email
    DUPLI1_ORDER_SERVICE_PASSWORD = random_password.order_service.result
  })

  lifecycle {
    # Keep operator-rotated passwords; random_password only seeds the first version.
    ignore_changes = [secret_string]
  }
}
