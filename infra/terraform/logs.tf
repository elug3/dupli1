resource "aws_cloudwatch_log_group" "service" {
  for_each = toset(concat(local.app_services, ["nats", "ecs-agent"]))

  name              = "/dupli1/${var.environment}/${each.key}"
  retention_in_days = 14

  tags = {
    Name = "${local.name_prefix}-${each.key}"
  }
}
