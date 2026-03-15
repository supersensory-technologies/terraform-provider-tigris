package names

const (
	// Attributes for the terraform resources.
	AttrBucket             = "bucket"
	AttrAcl                = "acl"
	AttrPublicListObjects  = "public_list_objects"
	AttrDomainName         = "domain_name"
	AttrShadowAccessKey    = "shadow_access_key"
	AttrShadowSecretKey    = "shadow_secret_key"
	AttrShadowRegion       = "shadow_region"
	AttrShadowBucket       = "shadow_bucket"
	AttrShadowEndpoint     = "shadow_endpoint"
	AttrShadowWriteThrough = "shadow_write_through"

	// Bucket location and storage tier attributes.
	AttrLocation           = "location"
	AttrLocationType       = "type"
	AttrLocationRegions    = "regions"
	AttrDefaultStorageTier = "default_storage_tier"

	// Bucket protection attributes.
	AttrDeleteProtection = "deletion_protection"

	// Snapshot and fork attributes.
	AttrEnableSnapshot           = "enable_snapshot"
	AttrSourceBucket             = "source_bucket"
	AttrSnapshotName             = "snapshot_name"
	AttrSnapshotVersion          = "snapshot_version"
	AttrSnapshotCreatedAt        = "snapshot_created_at"
	AttrForkSourceBucket         = "fork_source_bucket"
	AttrForkSourceBucketSnapshot = "fork_source_bucket_snapshot"
	AttrForkCreatedAt            = "fork_created_at"
)
