# Web service account used by dupli1-web BFF to call POST /api/v1/auth/register.
# Auth seeds/syncs the user on boot; web logs in with the same credentials.

resource "random_password" "web_service" {
  length  = 32
  special = false
}

resource "aws_secretsmanager_secret" "web_service" {
  name        = "${var.project_name}/${var.environment}/web-service-account"
  description = "dupli1-web service account email/password for customer registration"

  tags = {
    Environment = var.environment
    Project     = var.project_name
  }
}

resource "aws_secretsmanager_secret_version" "web_service" {
  secret_id = aws_secretsmanager_secret.web_service.id
  secret_string = jsonencode({
    DUPLI1_WEB_SERVICE_EMAIL    = var.web_service_email
    DUPLI1_WEB_SERVICE_PASSWORD = random_password.web_service.result
  })

  lifecycle {
    # Keep operator-rotated passwords; random_password only seeds the first version.
    ignore_changes = [secret_string]
  }
}
