# NANO Solution certified payment (인증결제) credentials for dupli1-payment.
# Operators put real values after contract; the initial version is an empty shell.

resource "aws_secretsmanager_secret" "nano_payment" {
  name        = "${var.project_name}/${var.environment}/nano-payment"
  description = "NANO PG credentials (NANO_API_KEY, NANO_LOGIN_ID, NANO_SHOPCODE, NANO_VER)"

  tags = {
    Environment = var.environment
    Project     = var.project_name
  }
}

resource "aws_secretsmanager_secret_version" "nano_payment" {
  secret_id = aws_secretsmanager_secret.nano_payment.id
  secret_string = jsonencode({
    NANO_API_KEY   = ""
    NANO_LOGIN_ID  = ""
    NANO_SHOPCODE  = ""
    NANO_VER       = ""
  })

  lifecycle {
    # Keep operator-rotated credentials; Terraform only seeds the first version.
    ignore_changes = [secret_string]
  }
}
