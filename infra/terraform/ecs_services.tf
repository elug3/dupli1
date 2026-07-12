locals {
  registry       = "${data.aws_caller_identity.current.account_id}.dkr.ecr.${var.aws_region}.amazonaws.com"
  alb_url        = var.certificate_arn != "" ? "https://${aws_lb.main.dns_name}" : "http://${aws_lb.main.dns_name}"
  s3_endpoint    = "s3.${var.aws_region}.amazonaws.com"
  s3_bucket_host = "${aws_s3_bucket.product_images.bucket}.s3.${var.aws_region}.amazonaws.com"

  discovery = {
    auth         = "dupli1-auth.${local.service_domain}"
    product      = "dupli1-product.${local.service_domain}"
    order        = "dupli1-order.${local.service_domain}"
    cart         = "dupli1-cart.${local.service_domain}"
    payment      = "dupli1-payment.${local.service_domain}"
    notification = "dupli1-notification.${local.service_domain}"
    nats         = "dupli1-nats.${local.service_domain}"
  }
}

# -----------------------------------------------------------------------------
# NATS (public image — no ECR)
# -----------------------------------------------------------------------------
resource "aws_ecs_task_definition" "nats" {
  family                   = "${local.name_prefix}-nats"
  requires_compatibilities = ["EC2"]
  network_mode             = "awsvpc"
  execution_role_arn       = aws_iam_role.ecs_task_execution.arn
  cpu                      = "256"
  memory                   = "512"

  container_definitions = jsonencode([
    {
      name      = "nats"
      image     = "nats:2-alpine"
      essential = true
      command   = ["-js"]
      portMappings = [
        { containerPort = 4222, protocol = "tcp" },
        { containerPort = 8222, protocol = "tcp" },
      ]
      logConfiguration = {
        logDriver = "awslogs"
        options = {
          "awslogs-group"         = aws_cloudwatch_log_group.service["nats"].name
          "awslogs-region"        = var.aws_region
          "awslogs-stream-prefix" = "nats"
        }
      }
    }
  ])
}

resource "aws_ecs_service" "nats" {
  name            = "dupli1-nats"
  cluster         = aws_ecs_cluster.main.id
  task_definition = aws_ecs_task_definition.nats.arn
  desired_count   = 1

  capacity_provider_strategy {
    capacity_provider = aws_ecs_capacity_provider.ec2.name
    weight            = 1
  }

  network_configuration {
    subnets         = aws_subnet.public[*].id
    security_groups = [aws_security_group.ecs.id]
  }

  service_registries {
    registry_arn = aws_service_discovery_service.svc["nats"].arn
  }

  deployment_minimum_healthy_percent = 0
  deployment_maximum_percent         = 100

  depends_on = [aws_ecs_cluster_capacity_providers.main]

  lifecycle {
    ignore_changes = [desired_count]
  }
}

# -----------------------------------------------------------------------------
# Auth
# -----------------------------------------------------------------------------
resource "aws_ecs_task_definition" "auth" {
  family                   = "${local.name_prefix}-auth"
  requires_compatibilities = ["EC2"]
  network_mode             = "awsvpc"
  execution_role_arn       = aws_iam_role.ecs_task_execution.arn
  task_role_arn            = aws_iam_role.ecs_task.arn
  cpu                      = "256"
  memory                   = "512"

  container_definitions = jsonencode([
    {
      name      = "dupli1-auth"
      image     = "${local.registry}/dupli1-auth:${var.ecs_image_tag}"
      essential = true
      portMappings = [
        { containerPort = 8080, protocol = "tcp" }
      ]
      environment = [
        { name = "DUPLI1_AUTH_ADDR", value = ":8080" },
        { name = "NATS_URL", value = "nats://${local.discovery.nats}:4222" },
        { name = "OWNER_EMAIL", value = var.owner_email },
        { name = "DUPLI1_WEB_SERVICE_EMAIL", value = "dupli1-web@service.dupli1.com" },
        { name = "DUPLI1_ORDER_SERVICE_EMAIL", value = "dupli1-order@service.dupli1.com" },
      ]
      secrets = [
        { name = "DB_URL", valueFrom = aws_secretsmanager_secret.auth_db_url.arn },
        { name = "JWT_SECRET", valueFrom = aws_secretsmanager_secret.jwt_secret.arn },
        { name = "OWNER_PASSWORD", valueFrom = aws_secretsmanager_secret.owner_password.arn },
        { name = "DUPLI1_WEB_SERVICE_PASSWORD", valueFrom = aws_secretsmanager_secret.web_service_password.arn },
        { name = "DUPLI1_ORDER_SERVICE_PASSWORD", valueFrom = aws_secretsmanager_secret.order_service_password.arn },
      ]
      logConfiguration = {
        logDriver = "awslogs"
        options = {
          "awslogs-group"         = aws_cloudwatch_log_group.service["auth"].name
          "awslogs-region"        = var.aws_region
          "awslogs-stream-prefix" = "auth"
        }
      }
      healthCheck = {
        command     = ["CMD-SHELL", "wget -qO- http://localhost:8080/health || exit 1"]
        interval    = 15
        timeout     = 5
        retries     = 5
        startPeriod = 60
      }
    }
  ])
}

resource "aws_ecs_service" "auth" {
  name            = "dupli1-auth"
  cluster         = aws_ecs_cluster.main.id
  task_definition = aws_ecs_task_definition.auth.arn
  desired_count   = 1

  capacity_provider_strategy {
    capacity_provider = aws_ecs_capacity_provider.ec2.name
    weight            = 1
  }

  network_configuration {
    subnets         = aws_subnet.public[*].id
    security_groups = [aws_security_group.ecs.id]
  }

  service_registries {
    registry_arn = aws_service_discovery_service.svc["auth"].arn
  }

  deployment_minimum_healthy_percent = 0
  deployment_maximum_percent         = 200
  enable_execute_command             = false

  depends_on = [aws_ecs_service.nats, aws_db_instance.dupli1]

  lifecycle {
    ignore_changes = [task_definition, desired_count]
  }
}

# -----------------------------------------------------------------------------
# Product
# -----------------------------------------------------------------------------
resource "aws_ecs_task_definition" "product" {
  family                   = "${local.name_prefix}-product"
  requires_compatibilities = ["EC2"]
  network_mode             = "awsvpc"
  execution_role_arn       = aws_iam_role.ecs_task_execution.arn
  task_role_arn            = aws_iam_role.ecs_task.arn
  cpu                      = "256"
  memory                   = "512"

  container_definitions = jsonencode([
    {
      name      = "dupli1-product"
      image     = "${local.registry}/dupli1-product:${var.ecs_image_tag}"
      essential = true
      portMappings = [
        { containerPort = 8080, protocol = "tcp" }
      ]
      environment = [
        { name = "SERVER_HOST", value = "0.0.0.0" },
        { name = "SERVER_PORT", value = "8080" },
        { name = "AUTH_JWKS_URL", value = "http://${local.discovery.auth}:8080/api/v1/auth/.well-known/jwks.json" },
        { name = "NATS_URL", value = "nats://${local.discovery.nats}:4222" },
        { name = "S3_ENDPOINT", value = "https://${local.s3_endpoint}" },
        { name = "S3_PUBLIC_ENDPOINT", value = local.alb_url },
        { name = "S3_BUCKET", value = aws_s3_bucket.product_images.bucket },
      ]
      secrets = [
        { name = "DUPLI1_PRODUCT_DB", valueFrom = aws_secretsmanager_secret.product_db_url.arn },
        { name = "JWT_SECRET", valueFrom = aws_secretsmanager_secret.jwt_secret.arn },
        { name = "S3_ACCESS_KEY", valueFrom = aws_secretsmanager_secret.s3_access_key.arn },
        { name = "S3_SECRET_KEY", valueFrom = aws_secretsmanager_secret.s3_secret_key.arn },
      ]
      logConfiguration = {
        logDriver = "awslogs"
        options = {
          "awslogs-group"         = aws_cloudwatch_log_group.service["product"].name
          "awslogs-region"        = var.aws_region
          "awslogs-stream-prefix" = "product"
        }
      }
      healthCheck = {
        command     = ["CMD-SHELL", "wget -qO- http://localhost:8080/api/v1/products/health || exit 1"]
        interval    = 15
        timeout     = 5
        retries     = 5
        startPeriod = 60
      }
    }
  ])
}

resource "aws_ecs_service" "product" {
  name            = "dupli1-product"
  cluster         = aws_ecs_cluster.main.id
  task_definition = aws_ecs_task_definition.product.arn
  desired_count   = 1

  capacity_provider_strategy {
    capacity_provider = aws_ecs_capacity_provider.ec2.name
    weight            = 1
  }

  network_configuration {
    subnets         = aws_subnet.public[*].id
    security_groups = [aws_security_group.ecs.id]
  }

  service_registries {
    registry_arn = aws_service_discovery_service.svc["product"].arn
  }

  deployment_minimum_healthy_percent = 0
  deployment_maximum_percent         = 200

  depends_on = [aws_ecs_service.auth]

  lifecycle {
    ignore_changes = [task_definition, desired_count]
  }
}

# -----------------------------------------------------------------------------
# Order
# -----------------------------------------------------------------------------
resource "aws_ecs_task_definition" "order" {
  family                   = "${local.name_prefix}-order"
  requires_compatibilities = ["EC2"]
  network_mode             = "awsvpc"
  execution_role_arn       = aws_iam_role.ecs_task_execution.arn
  task_role_arn            = aws_iam_role.ecs_task.arn
  cpu                      = "256"
  memory                   = "512"

  container_definitions = jsonencode([
    {
      name      = "dupli1-order"
      image     = "${local.registry}/dupli1-order:${var.ecs_image_tag}"
      essential = true
      portMappings = [
        { containerPort = 8080, protocol = "tcp" }
      ]
      environment = [
        { name = "DUPLI1_ORDER_ADDR", value = ":8080" },
        { name = "DUPLI1_INVENTORY_URL", value = "http://${local.discovery.product}:8080" },
        { name = "DUPLI1_PRODUCT_URL", value = "http://${local.discovery.product}:8080" },
        { name = "DUPLI1_AUTH_URL", value = "http://${local.discovery.auth}:8080" },
        { name = "AUTH_JWKS_URL", value = "http://${local.discovery.auth}:8080/api/v1/auth/.well-known/jwks.json" },
        { name = "NATS_URL", value = "nats://${local.discovery.nats}:4222" },
        { name = "DUPLI1_ORDER_SERVICE_EMAIL", value = "dupli1-order@service.dupli1.com" },
      ]
      secrets = [
        { name = "DUPLI1_ORDER_DB", valueFrom = aws_secretsmanager_secret.order_db_url.arn },
        { name = "JWT_SECRET", valueFrom = aws_secretsmanager_secret.jwt_secret.arn },
        { name = "DUPLI1_ORDER_SERVICE_PASSWORD", valueFrom = aws_secretsmanager_secret.order_service_password.arn },
      ]
      logConfiguration = {
        logDriver = "awslogs"
        options = {
          "awslogs-group"         = aws_cloudwatch_log_group.service["order"].name
          "awslogs-region"        = var.aws_region
          "awslogs-stream-prefix" = "order"
        }
      }
      healthCheck = {
        command     = ["CMD-SHELL", "wget -qO- http://localhost:8080/health || exit 1"]
        interval    = 15
        timeout     = 5
        retries     = 5
        startPeriod = 60
      }
    }
  ])
}

resource "aws_ecs_service" "order" {
  name            = "dupli1-order"
  cluster         = aws_ecs_cluster.main.id
  task_definition = aws_ecs_task_definition.order.arn
  desired_count   = 1

  capacity_provider_strategy {
    capacity_provider = aws_ecs_capacity_provider.ec2.name
    weight            = 1
  }

  network_configuration {
    subnets         = aws_subnet.public[*].id
    security_groups = [aws_security_group.ecs.id]
  }

  service_registries {
    registry_arn = aws_service_discovery_service.svc["order"].arn
  }

  deployment_minimum_healthy_percent = 0
  deployment_maximum_percent         = 200

  depends_on = [aws_ecs_service.product, aws_ecs_service.nats]

  lifecycle {
    ignore_changes = [task_definition, desired_count]
  }
}

# -----------------------------------------------------------------------------
# Cart
# -----------------------------------------------------------------------------
resource "aws_ecs_task_definition" "cart" {
  family                   = "${local.name_prefix}-cart"
  requires_compatibilities = ["EC2"]
  network_mode             = "awsvpc"
  execution_role_arn       = aws_iam_role.ecs_task_execution.arn
  task_role_arn            = aws_iam_role.ecs_task.arn
  cpu                      = "256"
  memory                   = "512"

  container_definitions = jsonencode([
    {
      name      = "dupli1-cart"
      image     = "${local.registry}/dupli1-cart:${var.ecs_image_tag}"
      essential = true
      portMappings = [
        { containerPort = 8080, protocol = "tcp" }
      ]
      environment = [
        { name = "DUPLI1_CART_ADDR", value = ":8080" },
        { name = "DUPLI1_PRODUCT_URL", value = "http://${local.discovery.product}:8080" },
        { name = "DUPLI1_INVENTORY_URL", value = "http://${local.discovery.product}:8080" },
        { name = "AUTH_JWKS_URL", value = "http://${local.discovery.auth}:8080/api/v1/auth/.well-known/jwks.json" },
      ]
      secrets = [
        { name = "DUPLI1_CART_DB", valueFrom = aws_secretsmanager_secret.cart_db_url.arn },
        { name = "JWT_SECRET", valueFrom = aws_secretsmanager_secret.jwt_secret.arn },
      ]
      logConfiguration = {
        logDriver = "awslogs"
        options = {
          "awslogs-group"         = aws_cloudwatch_log_group.service["cart"].name
          "awslogs-region"        = var.aws_region
          "awslogs-stream-prefix" = "cart"
        }
      }
      healthCheck = {
        command     = ["CMD-SHELL", "wget -qO- http://localhost:8080/health || exit 1"]
        interval    = 15
        timeout     = 5
        retries     = 5
        startPeriod = 60
      }
    }
  ])
}

resource "aws_ecs_service" "cart" {
  name            = "dupli1-cart"
  cluster         = aws_ecs_cluster.main.id
  task_definition = aws_ecs_task_definition.cart.arn
  desired_count   = 1

  capacity_provider_strategy {
    capacity_provider = aws_ecs_capacity_provider.ec2.name
    weight            = 1
  }

  network_configuration {
    subnets         = aws_subnet.public[*].id
    security_groups = [aws_security_group.ecs.id]
  }

  service_registries {
    registry_arn = aws_service_discovery_service.svc["cart"].arn
  }

  deployment_minimum_healthy_percent = 0
  deployment_maximum_percent         = 200

  depends_on = [aws_ecs_service.product]

  lifecycle {
    ignore_changes = [task_definition, desired_count]
  }
}

# -----------------------------------------------------------------------------
# Payment
# -----------------------------------------------------------------------------
resource "aws_ecs_task_definition" "payment" {
  family                   = "${local.name_prefix}-payment"
  requires_compatibilities = ["EC2"]
  network_mode             = "awsvpc"
  execution_role_arn       = aws_iam_role.ecs_task_execution.arn
  task_role_arn            = aws_iam_role.ecs_task.arn
  cpu                      = "256"
  memory                   = "512"

  container_definitions = jsonencode([
    {
      name      = "dupli1-payment"
      image     = "${local.registry}/dupli1-payment:${var.ecs_image_tag}"
      essential = true
      portMappings = [
        { containerPort = 8080, protocol = "tcp" }
      ]
      environment = [
        { name = "DUPLI1_PAYMENT_ADDR", value = ":8080" },
        { name = "DUPLI1_ORDER_URL", value = "http://${local.discovery.order}:8080" },
        { name = "DUPLI1_PAYMENT_PUBLIC_URL", value = local.alb_url },
        { name = "AUTH_JWKS_URL", value = "http://${local.discovery.auth}:8080/api/v1/auth/.well-known/jwks.json" },
        { name = "NATS_URL", value = "nats://${local.discovery.nats}:4222" },
        { name = "STRIPE_SUCCESS_URL", value = "${local.alb_url}/checkout/success" },
        { name = "STRIPE_CANCEL_URL", value = "${local.alb_url}/checkout/cancel" },
      ]
      secrets = [
        { name = "DUPLI1_PAYMENT_DB", valueFrom = aws_secretsmanager_secret.payment_db_url.arn },
        { name = "JWT_SECRET", valueFrom = aws_secretsmanager_secret.jwt_secret.arn },
      ]
      logConfiguration = {
        logDriver = "awslogs"
        options = {
          "awslogs-group"         = aws_cloudwatch_log_group.service["payment"].name
          "awslogs-region"        = var.aws_region
          "awslogs-stream-prefix" = "payment"
        }
      }
      healthCheck = {
        command     = ["CMD-SHELL", "wget -qO- http://localhost:8080/health || exit 1"]
        interval    = 15
        timeout     = 5
        retries     = 5
        startPeriod = 60
      }
    }
  ])
}

resource "aws_ecs_service" "payment" {
  name            = "dupli1-payment"
  cluster         = aws_ecs_cluster.main.id
  task_definition = aws_ecs_task_definition.payment.arn
  desired_count   = 1

  capacity_provider_strategy {
    capacity_provider = aws_ecs_capacity_provider.ec2.name
    weight            = 1
  }

  network_configuration {
    subnets         = aws_subnet.public[*].id
    security_groups = [aws_security_group.ecs.id]
  }

  service_registries {
    registry_arn = aws_service_discovery_service.svc["payment"].arn
  }

  deployment_minimum_healthy_percent = 0
  deployment_maximum_percent         = 200

  depends_on = [aws_ecs_service.order, aws_ecs_service.nats]

  lifecycle {
    ignore_changes = [task_definition, desired_count]
  }
}

# -----------------------------------------------------------------------------
# Notification
# -----------------------------------------------------------------------------
resource "aws_ecs_task_definition" "notification" {
  family                   = "${local.name_prefix}-notification"
  requires_compatibilities = ["EC2"]
  network_mode             = "awsvpc"
  execution_role_arn       = aws_iam_role.ecs_task_execution.arn
  task_role_arn            = aws_iam_role.ecs_task.arn
  cpu                      = "256"
  memory                   = "256"

  container_definitions = jsonencode([
    {
      name      = "dupli1-notification"
      image     = "${local.registry}/dupli1-notification:${var.ecs_image_tag}"
      essential = true
      portMappings = [
        { containerPort = 8080, protocol = "tcp" }
      ]
      environment = [
        { name = "DUPLI1_NOTIFICATION_ADDR", value = ":8080" },
        { name = "NATS_URL", value = "nats://${local.discovery.nats}:4222" },
      ]
      logConfiguration = {
        logDriver = "awslogs"
        options = {
          "awslogs-group"         = aws_cloudwatch_log_group.service["notification"].name
          "awslogs-region"        = var.aws_region
          "awslogs-stream-prefix" = "notification"
        }
      }
      healthCheck = {
        command     = ["CMD-SHELL", "wget -qO- http://localhost:8080/health || exit 1"]
        interval    = 15
        timeout     = 5
        retries     = 5
        startPeriod = 30
      }
    }
  ])
}

resource "aws_ecs_service" "notification" {
  name            = "dupli1-notification"
  cluster         = aws_ecs_cluster.main.id
  task_definition = aws_ecs_task_definition.notification.arn
  desired_count   = 1

  capacity_provider_strategy {
    capacity_provider = aws_ecs_capacity_provider.ec2.name
    weight            = 1
  }

  network_configuration {
    subnets         = aws_subnet.public[*].id
    security_groups = [aws_security_group.ecs.id]
  }

  service_registries {
    registry_arn = aws_service_discovery_service.svc["notification"].arn
  }

  deployment_minimum_healthy_percent = 0
  deployment_maximum_percent         = 200

  depends_on = [aws_ecs_service.nats]

  lifecycle {
    ignore_changes = [task_definition, desired_count]
  }
}

# -----------------------------------------------------------------------------
# Proxy (ALB target)
# -----------------------------------------------------------------------------
resource "aws_ecs_task_definition" "proxy" {
  family                   = "${local.name_prefix}-proxy"
  requires_compatibilities = ["EC2"]
  network_mode             = "awsvpc"
  execution_role_arn       = aws_iam_role.ecs_task_execution.arn
  task_role_arn            = aws_iam_role.ecs_task.arn
  cpu                      = "256"
  memory                   = "256"

  container_definitions = jsonencode([
    {
      name      = "dupli1-proxy"
      image     = "${local.registry}/dupli1-proxy:${var.ecs_image_tag}"
      essential = true
      portMappings = [
        { containerPort = 80, protocol = "tcp" }
      ]
      environment = [
        { name = "S3_BUCKET_HOST", value = local.s3_bucket_host },
        { name = "S3_BUCKET_NAME", value = aws_s3_bucket.product_images.bucket },
        { name = "SERVICE_DOMAIN", value = local.service_domain },
      ]
      logConfiguration = {
        logDriver = "awslogs"
        options = {
          "awslogs-group"         = aws_cloudwatch_log_group.service["proxy"].name
          "awslogs-region"        = var.aws_region
          "awslogs-stream-prefix" = "proxy"
        }
      }
      healthCheck = {
        command     = ["CMD-SHELL", "wget -qO- http://localhost/gateway/health || exit 1"]
        interval    = 15
        timeout     = 5
        retries     = 3
        startPeriod = 20
      }
    }
  ])
}

resource "aws_ecs_service" "proxy" {
  name            = "dupli1-proxy"
  cluster         = aws_ecs_cluster.main.id
  task_definition = aws_ecs_task_definition.proxy.arn
  desired_count   = 1

  capacity_provider_strategy {
    capacity_provider = aws_ecs_capacity_provider.ec2.name
    weight            = 1
  }

  network_configuration {
    subnets         = aws_subnet.public[*].id
    security_groups = [aws_security_group.ecs.id]
  }

  service_registries {
    registry_arn = aws_service_discovery_service.svc["proxy"].arn
  }

  load_balancer {
    target_group_arn = aws_lb_target_group.proxy.arn
    container_name   = "dupli1-proxy"
    container_port   = 80
  }

  deployment_minimum_healthy_percent = 50
  deployment_maximum_percent         = 200

  depends_on = [
    aws_lb_listener.http,
    aws_ecs_service.auth,
    aws_ecs_service.product,
    aws_ecs_service.order,
    aws_ecs_service.cart,
    aws_ecs_service.payment,
  ]

  lifecycle {
    ignore_changes = [task_definition, desired_count]
  }
}
