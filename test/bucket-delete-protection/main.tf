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

variable "delete_protection" {
  type    = bool
  default = true
}

# Test: delete protection lifecycle
#
# 1. Apply with delete_protection = true (post-create PATCH) — this file
# 2. Verify destroy is blocked (delete handler guard) — pre-destroy.sh
# 3. Set delete_protection = false so cleanup can run — pre-destroy.sh
# 4. Destroy — the shared cleanup in scripts/run-tests.sh
resource "tigris_bucket" "protected" {
  bucket            = "${var.test_id}-protected-bucket"
  delete_protection = var.delete_protection
}

output "bucket" {
  value = tigris_bucket.protected.bucket
}

output "delete_protection" {
  value = tigris_bucket.protected.delete_protection
}
