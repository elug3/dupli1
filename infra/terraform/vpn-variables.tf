variable "vpn_instance_id" {
  description = "EC2 instance ID for the internal WireGuard VPN server."
  type        = string
  default     = "i-0f7a516c42a8b7afd"
}

variable "vpn_security_group_id" {
  description = "Security group attached to the VPN EC2 instance."
  type        = string
  default     = "sg-0159765703fa523f9"
}

variable "vpn_wireguard_port" {
  description = "WireGuard listen port on the VPN server."
  type        = number
  default     = 51820
}

variable "vpn_client_cidr" {
  description = "WireGuard client address pool routed into the VPC."
  type        = string
  default     = "10.8.0.0/24"
}

variable "private_route_table_id" {
  description = "Route table used by ECS private subnets."
  type        = string
  default     = "rtb-0f4cdca0f6d33a058"
}

variable "cloud_map_namespace_id" {
  description = "AWS Cloud Map private DNS namespace for schick.local."
  type        = string
  default     = "ns-l27mpgbbgh5llwiv"
}
