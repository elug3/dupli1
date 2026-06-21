variable "aws_region" {
  description = "AWS region for Schick production resources."
  type        = string
  default     = "us-east-1"
}

variable "project_name" {
  description = "Resource name prefix."
  type        = string
  default     = "schick"
}

variable "environment" {
  description = "Deployment environment label."
  type        = string
  default     = "production"
}

variable "vpc_id" {
  description = "VPC that hosts ECS services and RDS."
  type        = string
  default     = "vpc-0e143b53ca2a4714c"
}

variable "private_subnet_ids" {
  description = "Private subnets for the RDS subnet group."
  type        = list(string)
  default = [
    "subnet-01fd0882721f10499",
    "subnet-006b8428713711816",
  ]
}

variable "ecs_security_group_id" {
  description = "Security group attached to ECS tasks that need database access."
  type        = string
  default     = "sg-06581371272cab230"
}

variable "db_instance_class" {
  description = "RDS instance class."
  type        = string
  default     = "db.t3.micro"
}

variable "db_allocated_storage_gb" {
  description = "Initial allocated storage for RDS."
  type        = number
  default     = 20
}

variable "db_name" {
  description = "Primary database created on the RDS instance."
  type        = string
  default     = "schick_db"
}

variable "db_username" {
  description = "Master database username."
  type        = string
  default     = "schick"
}

variable "product_db_name" {
  description = "Secondary database used by schick-product."
  type        = string
  default     = "products"
}

variable "backup_retention_period" {
  description = "Number of days to retain automated backups."
  type        = number
  default     = 7
}

variable "deletion_protection" {
  description = "Prevent accidental RDS deletion."
  type        = bool
  default     = true
}
