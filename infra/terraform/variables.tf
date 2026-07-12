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

variable "vpc_cidr" {
  description = "CIDR block for the VPC."
  type        = string
  default     = "10.20.0.0/16"
}

variable "az_count" {
  description = "Number of availability zones (min 2 for ALB/RDS subnet groups)."
  type        = number
  default     = 2
}

variable "ecs_instance_type" {
  description = "EC2 instance type for the ECS capacity provider."
  type        = string
  default     = "t3.large"
}

variable "ecs_asg_min_size" {
  description = "Minimum ECS EC2 instances."
  type        = number
  default     = 1
}

variable "ecs_asg_max_size" {
  description = "Maximum ECS EC2 instances."
  type        = number
  default     = 2
}

variable "ecs_asg_desired_capacity" {
  description = "Desired ECS EC2 instances."
  type        = number
  default     = 1
}

variable "ecs_image_tag" {
  description = "ECR image tag for application services (use latest after first CI push)."
  type        = string
  default     = "latest"
}

variable "db_instance_class" {
  description = "RDS instance class."
  type        = string
  default     = "db.t3.micro"
}

variable "db_allocated_storage_gb" {
  description = "Initial allocated storage for RDS (GiB)."
  type        = number
  default     = 20
}

variable "db_name" {
  description = "Primary database created on the RDS instance (auth)."
  type        = string
  default     = "dupli1_db"
}

variable "db_username" {
  description = "Master database username."
  type        = string
  default     = "dupli1"
}

variable "product_db_name" {
  description = "Database used by dupli1-product."
  type        = string
  default     = "products"
}

variable "order_db_name" {
  description = "Database used by dupli1-order."
  type        = string
  default     = "orders"
}

variable "cart_db_name" {
  description = "Database used by dupli1-cart."
  type        = string
  default     = "cart"
}

variable "payment_db_name" {
  description = "Database used by dupli1-payment."
  type        = string
  default     = "payments"
}

variable "backup_retention_period" {
  description = "Number of days to retain automated RDS backups."
  type        = number
  default     = 7
}

variable "deletion_protection" {
  description = "Prevent accidental RDS deletion."
  type        = bool
  default     = true
}

variable "owner_email" {
  description = "Seeded owner account email for dupli1-auth."
  type        = string
  default     = "admin@dupli1.com"
}

variable "owner_password" {
  description = "Seeded owner account password. Leave empty to auto-generate."
  type        = string
  default     = ""
  sensitive   = true
}

variable "certificate_arn" {
  description = "Optional ACM certificate ARN for HTTPS on the ALB. Empty = HTTP only."
  type        = string
  default     = ""
}

variable "enable_container_insights" {
  description = "Enable ECS Container Insights."
  type        = bool
  default     = false
}
