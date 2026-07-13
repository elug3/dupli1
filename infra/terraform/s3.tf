resource "aws_s3_bucket" "product_images" {
  bucket_prefix = "${var.project_name}-product-images-"

  tags = {
    Name        = "${local.name_prefix}-product-images"
    Environment = var.environment
    Project     = var.project_name
  }
}

resource "aws_s3_bucket_public_access_block" "product_images" {
  bucket = aws_s3_bucket.product_images.id

  block_public_acls       = true
  block_public_policy     = true
  ignore_public_acls      = true
  restrict_public_buckets = true
}

resource "aws_s3_bucket_server_side_encryption_configuration" "product_images" {
  bucket = aws_s3_bucket.product_images.id

  rule {
    apply_server_side_encryption_by_default {
      sse_algorithm = "AES256"
    }
  }
}

resource "aws_s3_bucket_ownership_controls" "product_images" {
  bucket = aws_s3_bucket.product_images.id

  rule {
    object_ownership = "BucketOwnerEnforced"
  }
}

# Bucket stays private (account Block Public Access). Product uploads via IAM
# access keys; expose objects later via CloudFront OAC or the gateway if needed.

resource "aws_iam_user" "product_s3" {
  name = "${local.name_prefix}-product-s3"

  tags = {
    Environment = var.environment
    Project     = var.project_name
  }
}

resource "aws_iam_user_policy" "product_s3" {
  name = "${local.name_prefix}-product-s3"
  user = aws_iam_user.product_s3.name

  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [
      {
        Sid    = "ListBucket"
        Effect = "Allow"
        Action = ["s3:ListBucket"]
        Resource = [aws_s3_bucket.product_images.arn]
      },
      {
        Sid    = "ObjectRW"
        Effect = "Allow"
        Action = [
          "s3:GetObject",
          "s3:PutObject",
          "s3:DeleteObject",
        ]
        Resource = ["${aws_s3_bucket.product_images.arn}/*"]
      }
    ]
  })
}

resource "aws_iam_access_key" "product_s3" {
  user = aws_iam_user.product_s3.name
}

resource "aws_secretsmanager_secret" "product_s3" {
  name        = "${var.project_name}/${var.environment}/product-s3"
  description = "S3 access credentials for dupli1-product image uploads"

  tags = {
    Environment = var.environment
    Project     = var.project_name
  }
}

resource "aws_secretsmanager_secret_version" "product_s3" {
  secret_id = aws_secretsmanager_secret.product_s3.id
  secret_string = jsonencode({
    S3_ENDPOINT        = "https://s3.${var.aws_region}.amazonaws.com"
    S3_PUBLIC_ENDPOINT = "https://${aws_s3_bucket.product_images.bucket_regional_domain_name}"
    S3_ACCESS_KEY      = aws_iam_access_key.product_s3.id
    S3_SECRET_KEY      = aws_iam_access_key.product_s3.secret
    S3_BUCKET          = aws_s3_bucket.product_images.id
  })
}
