# checkov:skip=CKV2_AWS_62:Product image bucket does not need event notifications for this workload
# checkov:skip=CKV_AWS_18:Access logging omitted in cost-conscious default; enable for production hardening
# checkov:skip=CKV_AWS_145:SSE-S3 is sufficient for product images at this stage
# checkov:skip=CKV2_AWS_6:Public GetObject is intentional so ALB/nginx can proxy images
# checkov:skip=CKV2_AWS_64:Cross-region replication not required for dev-sized deploy
resource "aws_s3_bucket" "product_images" {
  bucket = "${local.name_prefix}-product-images-${data.aws_caller_identity.current.account_id}"

  tags = {
    Name = "${local.name_prefix}-product-images"
  }
}

resource "aws_s3_bucket_public_access_block" "product_images" {
  bucket = aws_s3_bucket.product_images.id

  # Public GetObject so the ALB/nginx gateway can proxy image URLs without signed requests.
  # Writes remain private (IAM user / task role only).
  block_public_acls       = true
  block_public_policy     = false
  ignore_public_acls      = true
  restrict_public_buckets = false
}

data "aws_iam_policy_document" "product_images_public_read" {
  statement {
    sid    = "PublicReadGetObject"
    effect = "Allow"
    principals {
      type        = "*"
      identifiers = ["*"]
    }
    actions   = ["s3:GetObject"]
    resources = ["${aws_s3_bucket.product_images.arn}/*"]
  }
}

resource "aws_s3_bucket_policy" "product_images" {
  bucket = aws_s3_bucket.product_images.id
  policy = data.aws_iam_policy_document.product_images_public_read.json

  depends_on = [aws_s3_bucket_public_access_block.product_images]
}

resource "aws_s3_bucket_server_side_encryption_configuration" "product_images" {
  bucket = aws_s3_bucket.product_images.id

  rule {
    apply_server_side_encryption_by_default {
      sse_algorithm = "AES256"
    }
  }
}

resource "aws_s3_bucket_versioning" "product_images" {
  bucket = aws_s3_bucket.product_images.id

  versioning_configuration {
    status = "Enabled"
  }
}

resource "aws_s3_bucket_lifecycle_configuration" "product_images" {
  bucket = aws_s3_bucket.product_images.id

  rule {
    id     = "expire-noncurrent"
    status = "Enabled"

    filter {}

    noncurrent_version_expiration {
      noncurrent_days = 30
    }

    abort_incomplete_multipart_upload {
      days_after_initiation = 7
    }
  }
}

resource "aws_s3_bucket_ownership_controls" "product_images" {
  bucket = aws_s3_bucket.product_images.id

  rule {
    object_ownership = "BucketOwnerEnforced"
  }
}

resource "aws_iam_user" "product_s3" {
  name = "${local.name_prefix}-product-s3"
  path = "/dupli1/"
}

resource "aws_iam_access_key" "product_s3" {
  user = aws_iam_user.product_s3.name
}

data "aws_iam_policy_document" "product_s3" {
  statement {
    sid    = "ProductImagesRW"
    effect = "Allow"
    actions = [
      "s3:PutObject",
      "s3:GetObject",
      "s3:DeleteObject",
      "s3:ListBucket",
    ]
    resources = [
      aws_s3_bucket.product_images.arn,
      "${aws_s3_bucket.product_images.arn}/*",
    ]
  }
}

resource "aws_iam_user_policy" "product_s3" {
  name   = "${local.name_prefix}-product-s3"
  user   = aws_iam_user.product_s3.name
  policy = data.aws_iam_policy_document.product_s3.json
}
