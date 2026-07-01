data "aws_iam_policy_document" "ecs_task_execution_secrets" {
  statement {
    sid    = "ReadDupli1DatabaseSecrets"
    effect = "Allow"

    actions = [
      "secretsmanager:GetSecretValue",
    ]

    resources = [
      aws_secretsmanager_secret.db_credentials.arn,
      aws_secretsmanager_secret.auth_db_url.arn,
      aws_secretsmanager_secret.product_db_url.arn,
    ]
  }
}

resource "aws_iam_role_policy" "ecs_task_execution_secrets" {
  name   = "${var.project_name}-${var.environment}-ecs-db-secrets"
  role   = "ecsTaskExecutionRole"
  policy = data.aws_iam_policy_document.ecs_task_execution_secrets.json
}
