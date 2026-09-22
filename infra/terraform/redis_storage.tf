# Durable storage for the Redis task.
#
# Redis is not a cache in this stack. auth's refresh-token ledger lives in it
# (auth/pkg/service/service.go Refresh), and a lookup miss is treated as
# revoked — so an empty Redis rejects every refresh token in existence and
# signs out every customer and operator at once. manage-web keeps its admin
# sessions there too. Until this file existed the task had no volume, so any
# restart or host replacement did exactly that.
#
# EFS rather than a host bind mount because the task is placed by ECS and can
# land on either instance; only a network file system survives the move. The
# dataset is a few MB of keys, and the write rate is logins, refreshes and
# rate-limit counters, so bursting throughput is ample.

resource "aws_efs_file_system" "redis" {
  creation_token = "${local.name_prefix}-redis"
  encrypted      = true

  # No lifecycle_policy on purpose: an AOF is rewritten continuously, so it
  # would never be cold enough for Infrequent Access to pay off, and a
  # transition would only add first-byte latency to recovery reads.
  performance_mode = "generalPurpose"
  throughput_mode  = "bursting"

  tags = {
    Name        = "${local.name_prefix}-redis"
    Environment = var.environment
    Project     = var.project_name
  }
}

# Daily AWS Backup snapshots. Negligible at this size, and the one thing that
# still covers a bad AOF — persistence alone does not protect against writing
# corruption to disk and then faithfully restoring it.
resource "aws_efs_backup_policy" "redis" {
  file_system_id = aws_efs_file_system.redis.id

  backup_policy {
    status = "ENABLED"
  }
}

resource "aws_security_group" "redis_efs" {
  name        = "${local.name_prefix}-redis-efs"
  description = "NFS from the Dupli1 Redis task to its EFS mount targets"
  vpc_id      = var.vpc_id

  # Only the awsvpc task ENIs, not the whole VPC CIDR: this is the one place
  # the session ledger is readable at rest, so it does not get the blanket
  # ingress the service SG uses for discovery.
  ingress {
    description     = "NFS from ECS awsvpc tasks"
    from_port       = 2049
    to_port         = 2049
    protocol        = "tcp"
    security_groups = [aws_security_group.ecs_tasks.id]
  }

  tags = {
    Name        = "${local.name_prefix}-redis-efs"
    Environment = var.environment
    Project     = var.project_name
  }
}

# One per private subnet, so the task mounts in whichever AZ it is placed in.
resource "aws_efs_mount_target" "redis" {
  for_each = toset(var.private_subnet_ids)

  file_system_id  = aws_efs_file_system.redis.id
  subnet_id       = each.value
  security_groups = [aws_security_group.redis_efs.id]
}

# uid/gid 999 is the redis user in redis:7-alpine. The access point pins every
# write to that identity and roots the task at its own directory, so the
# container never needs to run as root to own /data, and a future task cannot
# reach these files by mounting the file system root.
resource "aws_efs_access_point" "redis" {
  file_system_id = aws_efs_file_system.redis.id

  posix_user {
    uid = 999
    gid = 999
  }

  root_directory {
    path = "/redis"

    creation_info {
      owner_uid   = 999
      owner_gid   = 999
      permissions = "0755"
    }
  }

  tags = {
    Name        = "${local.name_prefix}-redis"
    Environment = var.environment
    Project     = var.project_name
  }
}
