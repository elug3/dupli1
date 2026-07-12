# Single-AZ NAT for private-subnet ECS tasks (ECR, Secrets Manager, CloudWatch).
# Dev-sized: one NAT avoids multi-AZ NAT cost (~$32/mo vs ~$64/mo).

resource "aws_eip" "nat" {
  domain = "vpc"

  tags = {
    Name        = "${local.name_prefix}-nat-eip"
    Environment = var.environment
    Project     = var.project_name
  }
}

resource "aws_nat_gateway" "prod" {
  allocation_id = aws_eip.nat.id
  subnet_id     = var.public_subnet_ids[0]

  tags = {
    Name        = "${local.name_prefix}-nat"
    Environment = var.environment
    Project     = var.project_name
  }
}

resource "aws_route" "private_default_nat" {
  for_each = toset(data.aws_route_tables.private.ids)

  route_table_id         = each.value
  destination_cidr_block = "0.0.0.0/0"
  nat_gateway_id         = aws_nat_gateway.prod.id
}
