data "aws_instance" "vpn" {
  instance_id = var.vpn_instance_id
}

resource "aws_security_group_rule" "vpn_wireguard_ingress" {
  type              = "ingress"
  security_group_id = var.vpn_security_group_id
  description       = "WireGuard VPN"
  from_port         = var.vpn_wireguard_port
  to_port           = var.vpn_wireguard_port
  protocol          = "udp"
  cidr_blocks       = ["0.0.0.0/0"]
}

resource "aws_security_group_rule" "ecs_from_vpn_http" {
  type                     = "ingress"
  security_group_id        = var.ecs_security_group_id
  description              = "Internal API via VPN (nginx proxy)"
  from_port                = 80
  to_port                  = 80
  protocol                 = "tcp"
  source_security_group_id = var.vpn_security_group_id
}

resource "aws_security_group_rule" "ecs_from_vpn_clients_http" {
  type              = "ingress"
  security_group_id = var.ecs_security_group_id
  description       = "Internal API from WireGuard clients"
  from_port         = 80
  to_port           = 80
  protocol          = "tcp"
  cidr_blocks       = [var.vpn_client_cidr]
}

resource "aws_security_group_rule" "ecs_from_vpn_clients_app" {
  type              = "ingress"
  security_group_id = var.ecs_security_group_id
  description       = "Direct service access from WireGuard clients"
  from_port         = 8080
  to_port           = 8080
  protocol          = "tcp"
  cidr_blocks       = [var.vpn_client_cidr]
}

resource "aws_route" "vpn_clients" {
  route_table_id         = var.private_route_table_id
  destination_cidr_block = var.vpn_client_cidr
  network_interface_id   = data.aws_instance.vpn.network_interface_id
}

resource "aws_service_discovery_service" "internal_api" {
  name = "internal"

  dns_config {
    namespace_id = var.cloud_map_namespace_id

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
    Environment = var.environment
    Project     = var.project_name
  }
}

resource "aws_secretsmanager_secret" "vpn_client_config" {
  name        = "${var.project_name}/${var.environment}/vpn/client-config"
  description = "WireGuard client configuration for Schick internal API access"

  tags = {
    Environment = var.environment
    Project     = var.project_name
  }
}
