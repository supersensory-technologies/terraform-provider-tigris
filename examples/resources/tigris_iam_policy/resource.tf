# Create an IAM policy that allows read/write access to a bucket.
resource "tigris_iam_policy" "upload_only" {
  name        = "upload-only"
  description = "Allow upload and read operations for the landing zone bucket"

  statement {
    effect = "Allow"
    actions = [
      "s3:PutObject",
      "s3:GetObject",
      "s3:ListBucket",
    ]
    resources = [
      "arn:aws:s3:::landing-zone.example.com",
      "arn:aws:s3:::landing-zone.example.com/*",
    ]
  }
}
