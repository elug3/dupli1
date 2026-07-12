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
  default     = 1
}

variable "ecs_asg_min_size" {
  description = "Minimum number of ECS container instances."
  type        = number
  default     = 1
}

variable "ecs_asg_max_size" {
  description = "Maximum number of ECS container instances."
  type        = number
  default     = 2
}

variable "image_tag" {
  description = "ECR image tag for backend services."
  type        = string
  default     = "latest"
}

variable "jwt_secret" {
  description = "HS256 JWT fallback secret injected into task definitions."
  type        = string
  default     = "dupli1-prod-jwt-change-me"
  sensitive   = true
}

variable "desired_count" {
  description = "Desired task count per application service."
  type        = number
  default     = 1
}
