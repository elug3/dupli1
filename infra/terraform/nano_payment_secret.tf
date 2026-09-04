# NANO Solution certified payment (인증결제) credentials for dupli1-payment.
# Operators put real values after contract; the initial version is an empty shell.
# 연동 테스트 (인증결제 API guide): NANO_VER/SHOPCODE=240000005, LOGIN_ID=shoptest,
# API_KEY=R7L9PxM5V8K2Jc4N6dWqY1Eb3T5XhZU2 with BaseURL https://dev3.nanopay.co.kr.

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
