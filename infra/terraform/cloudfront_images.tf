# CloudFront CDN for private product-images bucket (OAC).
# Browsers never talk to S3 directly; see docs/product-images-browser-access.md.

resource "aws_cloudfront_origin_access_control" "product_images" {
  name                              = "${local.name_prefix}-product-images"
  description                       = "SigV4 OAC for Dupli1 product images"
  origin_access_control_origin_type = "s3"
  signing_behavior                  = "always"
  signing_protocol                  = "sigv4"
}

locals {
  product_images_cdn_aliases     = var.product_images_cdn_aliases
  product_images_cdn_use_aliases = length(local.product_images_cdn_aliases) > 0
  product_images_cdn_cert_arn    = var.product_images_cdn_certificate_arn != "" ? var.product_images_cdn_certificate_arn : var.acm_certificate_arn
  # CachingOptimized managed policy
  cloudfront_caching_optimized_policy_id = "658327ea-f89d-4fab-a63d-7e88639e58f6"
}

resource "aws_cloudfront_distribution" "product_images" {
  enabled             = true
  is_ipv6_enabled     = true
  comment             = "${local.name_prefix} product images"
  price_class         = var.product_images_cdn_price_class
  aliases             = local.product_images_cdn_aliases
  wait_for_deployment = true

  origin {
    domain_name              = aws_s3_bucket.product_images.bucket_regional_domain_name
    origin_id                = "product-images-s3"
    origin_access_control_id = aws_cloudfront_origin_access_control.product_images.id
  }

  default_cache_behavior {
    target_origin_id       = "product-images-s3"
    viewer_protocol_policy = "redirect-to-https"
    allowed_methods        = ["GET", "HEAD", "OPTIONS"]
    cached_methods         = ["GET", "HEAD"]
    compress               = true
    cache_policy_id        = local.cloudfront_caching_optimized_policy_id
  }

  restrictions {
    geo_restriction {
      restriction_type = "none"
    }
  }

  dynamic "viewer_certificate" {
    for_each = local.product_images_cdn_use_aliases ? [1] : []
    content {
      acm_certificate_arn      = local.product_images_cdn_cert_arn
      ssl_support_method       = "sni-only"
      minimum_protocol_version = "TLSv1.2_2021"
    }
  }

  dynamic "viewer_certificate" {
    for_each = local.product_images_cdn_use_aliases ? [] : [1]
    content {
      cloudfront_default_certificate = true
    }
  }

  tags = {
    Name        = "${local.name_prefix}-product-images-cdn"
    Environment = var.environment
    Project     = var.project_name
  }
}

resource "aws_s3_bucket_policy" "product_images" {
  bucket = aws_s3_bucket.product_images.id

  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [
      {
        Sid    = "AllowCloudFrontServicePrincipalRead"
        Effect = "Allow"
        Principal = {
          Service = "cloudfront.amazonaws.com"
        }
        Action   = ["s3:GetObject"]
        Resource = ["${aws_s3_bucket.product_images.arn}/*"]
        Condition = {
          StringEquals = {
            "AWS:SourceArn" = aws_cloudfront_distribution.product_images.arn
          }
        }
      }
    ]
  })
}

resource "aws_route53_record" "product_images_cdn" {
  for_each = toset(local.product_images_cdn_aliases)

  zone_id = var.route53_zone_id
  name    = each.value
  type    = "A"

  alias {
    name                   = aws_cloudfront_distribution.product_images.domain_name
    zone_id                = aws_cloudfront_distribution.product_images.hosted_zone_id
    evaluate_target_health = false
  }
}

resource "aws_route53_record" "product_images_cdn_aaaa" {
  for_each = toset(local.product_images_cdn_aliases)

  zone_id = var.route53_zone_id
  name    = each.value
  type    = "AAAA"

  alias {
    name                   = aws_cloudfront_distribution.product_images.domain_name
    zone_id                = aws_cloudfront_distribution.product_images.hosted_zone_id
    evaluate_target_health = false
  }
}

locals {
  product_images_public_base = local.product_images_cdn_use_aliases ? "https://${local.product_images_cdn_aliases[0]}" : "https://${aws_cloudfront_distribution.product_images.domain_name}"
}
