variable "aws_region" {
  description = "AWS region for Dupli1 resources."
  type        = string
  default     = "us-east-1"
}

variable "project_name" {
  description = "Resource name prefix."
  type        = string
  default     = "dupli1"
}

variable "environment" {
  description = "Deployment environment label."
  type        = string
  default     = "production"
}

variable "vpc_id" {
  description = "Existing VPC that hosts ECS and RDS."
  type        = string
  default     = "vpc-0e143b53ca2a4714c"
}

variable "public_subnet_ids" {
  description = "Public subnets for ALB and NAT."
  type        = list(string)
  default = [
    "subnet-0d757c4cf8d71963b",
    "subnet-02c1003124987322c",
  ]
}

variable "private_subnet_ids" {
  description = "Private subnets for ECS tasks and EC2 container instances."
  type        = list(string)
  default = [
    "subnet-01fd0882721f10499",
    "subnet-006b8428713711816",
  ]
}

variable "ecs_cluster_name" {
  description = "Existing ECS cluster name."
  type        = string
  default     = "production"
}

variable "service_discovery_namespace_id" {
  description = "Cloud Map private DNS namespace ID (dupli1.local)."
  type        = string
  default     = "ns-5d53uocv3zvhmgrz"
}

variable "rds_security_group_id" {
  description = "Security group attached to the existing RDS instance."
  type        = string
  default     = "sg-073c0d32fa81e03e6"
}

variable "rds_instance_identifier" {
  description = "Existing RDS instance identifier."
  type        = string
  default     = "dupli1-production"
}

variable "auth_db_url_secret_arn" {
  description = "Secrets Manager ARN for auth DB_URL."
  type        = string
  default     = "arn:aws:secretsmanager:us-east-1:845061289093:secret:dupli1/production/auth-db-url-B6TnOD"
}

variable "product_db_url_secret_arn" {
  description = "Secrets Manager ARN for product DUPLI1_PRODUCT_DB."
  type        = string
  default     = "arn:aws:secretsmanager:us-east-1:845061289093:secret:dupli1/production/product-db-url-kaL4uk"
}

variable "order_db_url_secret_arn" {
  description = "Secrets Manager ARN for order DUPLI1_ORDER_DB."
  type        = string
  default     = "arn:aws:secretsmanager:us-east-1:845061289093:secret:dupli1/production/order-db-url-RAI64m"
}

variable "cart_db_url_secret_arn" {
  description = "Secrets Manager ARN for cart DUPLI1_CART_DB."
  type        = string
  default     = "arn:aws:secretsmanager:us-east-1:845061289093:secret:dupli1/production/cart-db-url-GSYbXs"
}

variable "payment_db_url_secret_arn" {
  description = "Secrets Manager ARN for payment DUPLI1_PAYMENT_DB."
  type        = string
  default     = "arn:aws:secretsmanager:us-east-1:845061289093:secret:dupli1/production/payments-db-url-XxgHJp"
}

variable "jwt_secret_arn" {
  description = "Secrets Manager ARN for JWT_SECRET (HS256 fallback)."
  type        = string
  default     = "arn:aws:secretsmanager:us-east-1:845061289093:secret:dupli1/production/jwt-secret-tTYcMy"
}

variable "acm_certificate_arn" {
  description = "ACM certificate ARN for HTTPS on the ALB (dupli1.com)."
  type        = string
  default     = "arn:aws:acm:us-east-1:845061289093:certificate/a5e612a6-8bec-4d02-8f98-cc8484aa2fc1"
}

variable "route53_zone_id" {
  description = "Public Route53 hosted zone ID for dupli1.com."
  type        = string
  default     = "Z04998762RV4NUS16WWXV"
}

variable "public_dns_names" {
  description = "Public DNS names that should alias to the ALB."
  type        = list(string)
  default     = ["dupli1.com", "www.dupli1.com"]
}

variable "ecs_ami_id" {
  description = "ECS-optimized AMI. Empty = latest Amazon Linux 2023 from SSM."
  type        = string
  default     = ""
}

variable "ecs_instance_type" {
  description = "EC2 instance type for the ECS capacity provider ASG."
  type        = string
  default     = "t3.large"
}

variable "ecs_asg_desired_capacity" {
  description = "Desired number of ECS container instances."
  type        = number
  default     = 5
}

variable "ecs_asg_min_size" {
  description = "Minimum number of ECS container instances."
  type        = number
  default     = 5
}

variable "ecs_asg_max_size" {
  description = "Maximum number of ECS container instances."
  type        = number
  default     = 6
}

variable "image_tag" {
  description = "ECR image tag for backend services."
  type        = string
  default     = "latest"
}

variable "desired_count" {
  description = "Desired task count per application service."
  type        = number
  default     = 1
}
