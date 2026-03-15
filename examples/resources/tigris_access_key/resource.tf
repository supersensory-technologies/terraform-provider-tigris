# Create an access key with editor access to a specific bucket.
resource "tigris_access_key" "nas_upload" {
  name = "nas-upload"

  bucket_role {
    bucket = "landing-zone.example.com"
    role   = "Editor"
  }
}
