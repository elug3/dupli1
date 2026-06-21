output "vpn_instance_id" {
  description = "WireGuard VPN EC2 instance ID."
  value       = var.vpn_instance_id
}

output "vpn_public_ip" {
  description = "WireGuard VPN server public endpoint."
  value       = data.aws_instance.vpn.public_ip
}

output "vpn_private_ip" {
  description = "WireGuard VPN server private IP in the VPC."
  value       = data.aws_instance.vpn.private_ip
}

output "vpn_wireguard_port" {
  description = "WireGuard UDP port."
  value       = var.vpn_wireguard_port
}

output "internal_api_url" {
  description = "Internal API base URL reachable over VPN."
  value       = "http://internal.schick.local"
}

output "internal_api_service_arn" {
  description = "Cloud Map service ARN for internal.schick.local."
  value       = aws_service_discovery_service.internal_api.arn
}

output "vpn_client_config_secret_arn" {
  description = "Secrets Manager ARN containing the WireGuard client config."
  value       = aws_secretsmanager_secret.vpn_client_config.arn
}
