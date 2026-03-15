terraform {
  required_providers {
    tigris = {
      source = "tigrisdata/tigris"
    }
  }
}

provider "tigris" {}

variable "test_id" {
  type    = string
  default = "t"
}

variable "deletion_protection" {
  type    = bool
  default = true
}

# Test: deletion protection lifecycle
#
# 1. Apply with deletion_protection = true (post-create PATCH)
# 2. Verify destroy is blocked (delete handler guard)
# 3. Set deletion_protection = false, apply, then destroy
resource "tigris_bucket" "protected" {
  bucket              = "${var.test_id}-protected-bucket"
  deletion_protection = var.deletion_protection
}

output "bucket" {
  value = tigris_bucket.protected.bucket
}

output "deletion_protection" {
  value = tigris_bucket.protected.deletion_protection
}
