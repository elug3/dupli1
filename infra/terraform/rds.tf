resource "random_password" "db_master" {
  length  = 32
  special = false
}

resource "aws_db_subnet_group" "dupli1" {
  name        = "${local.name_prefix}-rds"
  description = "Private subnets for Dupli1 RDS"
  subnet_ids  = aws_subnet.private[*].id

  tags = {
    Name = "${local.name_prefix}-rds"
  }
}

resource "aws_db_parameter_group" "dupli1" {
  name        = "${local.name_prefix}-postgres16"
  family      = "postgres16"
  description = "Dupli1 PostgreSQL 16 parameters"

  tags = {
    Name = "${local.name_prefix}-postgres16"
  }
}

resource "aws_db_instance" "dupli1" {
  identifier = local.name_prefix

  engine         = "postgres"
  engine_version = "16"
  instance_class = var.db_instance_class

  allocated_storage     = var.db_allocated_storage_gb
  max_allocated_storage = var.db_allocated_storage_gb * 2
  storage_type          = "gp3"
  storage_encrypted     = true

  db_name  = var.db_name
  username = var.db_username
  password = random_password.db_master.result

  db_subnet_group_name   = aws_db_subnet_group.dupli1.name
  vpc_security_group_ids = [aws_security_group.rds.id]
  parameter_group_name   = aws_db_parameter_group.dupli1.name

  publicly_accessible = false
  multi_az            = false

  backup_retention_period = var.backup_retention_period
  backup_window           = "03:00-04:00"
  maintenance_window      = "sun:04:00-sun:05:00"

  deletion_protection       = var.deletion_protection
  skip_final_snapshot       = false
  final_snapshot_identifier = "${local.name_prefix}-final"

  auto_minor_version_upgrade = true
  copy_tags_to_snapshot      = true

  tags = {
    Name = local.name_prefix
  }
}
