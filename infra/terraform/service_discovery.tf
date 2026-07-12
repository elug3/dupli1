resource "aws_service_discovery_private_dns_namespace" "main" {
  name        = local.service_domain
  description = "Dupli1 internal service discovery"
  vpc         = aws_vpc.main.id

  tags = {
    Name = local.name_prefix
  }
}

resource "aws_service_discovery_service" "svc" {
  for_each = toset(concat(local.app_services, ["nats"]))

  name = "dupli1-${each.key}"

  dns_config {
    namespace_id = aws_service_discovery_private_dns_namespace.main.id

    dns_records {
      ttl  = 10
      type = "A"
    }

    routing_policy = "MULTIVALUE"
  }

  health_check_custom_config {
    failure_threshold = 1
  }

  tags = {
    Name = "dupli1-${each.key}"
  }
}
