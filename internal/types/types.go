package types

import "strings"

type BucketCannedACL string

// Enum values for BucketCannedACL.
const (
	BucketCannedACLPrivate    BucketCannedACL = "private"
	BucketCannedACLPublicRead BucketCannedACL = "public-read"
)

func (BucketCannedACL) Values() []BucketCannedACL {
	return []BucketCannedACL{
		BucketCannedACLPrivate,
		BucketCannedACLPublicRead,
	}
}

// StorageTier represents the default storage tier for a bucket.
type StorageTier string

const (
	StorageTierStandard   StorageTier = "STANDARD"
	StorageTierStandardIA StorageTier = "STANDARD_IA"
	StorageTierGlacier    StorageTier = "GLACIER"
	StorageTierGlacierIR  StorageTier = "GLACIER_IR"
)

func (StorageTier) Values() []StorageTier {
	return []StorageTier{
		StorageTierStandard,
		StorageTierStandardIA,
		StorageTierGlacier,
		StorageTierGlacierIR,
	}
}

// LocationType represents the bucket location type.
type LocationType string

const (
	LocationTypeGlobal LocationType = "global"
	LocationTypeMulti  LocationType = "multi"
	LocationTypeDual   LocationType = "dual"
	LocationTypeSingle LocationType = "single"
)

func (LocationType) Values() []LocationType {
	return []LocationType{
		LocationTypeGlobal,
		LocationTypeMulti,
		LocationTypeDual,
		LocationTypeSingle,
	}
}

// Valid region values.
var (
	ValidMultiRegions  = []string{"usa", "eur"}
	ValidSingleRegions = []string{"ams", "fra", "gru", "iad", "jnb", "lhr", "nrt", "ord", "sin", "sjc", "syd"}
)

// BucketType represents the type of a Tigris bucket.
const (
	// BucketTypeDefault is a standard bucket without snapshots.
	BucketTypeDefault = 0
	// BucketTypeSnapshot is a bucket with snapshots enabled.
	BucketTypeSnapshot = 1
)

type BucketMetadata struct {
	Name          string               `json:"name"`
	CacheControl  string               `json:"cache_control"`
	ObjectRegions string               `json:"object_regions"`
	StorageClass  string               `json:"storage_class"`
	Type          int                  `json:"type"`
	MD            *BucketMD            `json:"md"`
	Shadow        *BucketShadowConfig  `json:"shadow_bucket"`
	Website       *BucketWebsiteConfig `json:"website"`
	ForkInfo      *ForkInfo            `json:"ForkInfo"`
	Protection    *BucketProtection    `json:"protection"`
}

type BucketMD struct {
	ACL                      *BucketCannedACL `json:"X-Amz-Acl"`
	PublicObjectsListEnabled *string          `json:"x-amz-acl-public-list-objects-enabled"`
}

func (b *BucketMetadata) GetBucketCannedACL() BucketCannedACL {
	if b.MD == nil || b.MD.ACL == nil {
		return BucketCannedACLPrivate
	}

	return *b.MD.ACL
}

func (b *BucketMetadata) GetPublicObjectsListEnabled() bool {
	if b.MD == nil || b.MD.PublicObjectsListEnabled == nil {
		return true
	}

	if *b.MD.PublicObjectsListEnabled == "true" {
		return true
	}

	return false
}

// IsDeleteProtectionEnabled returns whether delete protection is enabled.
func (b *BucketMetadata) IsDeleteProtectionEnabled() bool {
	if b.Protection == nil {
		return false
	}
	return b.Protection.Protected
}

// GetStorageClass returns the storage class, defaulting to STANDARD.
func (b *BucketMetadata) GetStorageClass() string {
	if b.StorageClass == "" {
		return string(StorageTierStandard)
	}
	return b.StorageClass
}

// GetLocationTypeAndRegions parses object_regions into a LocationType and region list.
func (b *BucketMetadata) GetLocationTypeAndRegions() (LocationType, []string) {
	if b.ObjectRegions == "" {
		return LocationTypeGlobal, nil
	}

	regions := strings.Split(b.ObjectRegions, ",")
	for i := range regions {
		regions[i] = strings.TrimSpace(regions[i])
	}

	// Filter out empty strings
	filtered := make([]string, 0, len(regions))
	for _, r := range regions {
		if r != "" {
			filtered = append(filtered, r)
		}
	}
	regions = filtered

	if len(regions) == 0 {
		return LocationTypeGlobal, nil
	}

	if len(regions) == 1 {
		// Check if it's a multi-region value
		for _, mr := range ValidMultiRegions {
			if regions[0] == mr {
				return LocationTypeMulti, regions
			}
		}
		return LocationTypeSingle, regions
	}

	// 2+ regions is dual location type
	return LocationTypeDual, regions
}

// IsSnapshotEnabled returns true if snapshots are enabled for this bucket.
func (b *BucketMetadata) IsSnapshotEnabled() bool {
	return b.Type == BucketTypeSnapshot
}

type BucketWebsiteConfig struct {
	DomainName string `json:"domain_name"`
}

type BucketShadowConfig struct {
	AccessKey    string `json:"access_key"`
	SecretKey    string `json:"secret_key"`
	Region       string `json:"region"`
	Name         string `json:"name"`
	Endpoint     string `json:"endpoint"`
	WriteThrough bool   `json:"write_through"`
}

// SnapshotInfo represents a single snapshot.
type SnapshotInfo struct {
	Version   string
	Name      string
	CreatedAt string
}

// ForkParentInfo represents a parent bucket in fork info.
type ForkParentInfo struct {
	BucketName        string `json:"BucketName"`
	ForkCreatedAt     string `json:"ForkCreatedAt"`
	Snapshot          string `json:"Snapshot"`
	SnapshotCreatedAt string `json:"SnapshotCreatedAt"`
}

// ForkInfo represents the fork information from bucket metadata.
type ForkInfo struct {
	HasChildren bool             `json:"HasChildren"`
	Parents     []ForkParentInfo `json:"Parents"`
}

// BucketProtection represents the protection configuration for a bucket.
type BucketProtection struct {
	Protected bool `json:"protected"`
}

// BucketUpdateInput is the input for the CreateBucket and UpdateBucket functions.
type BucketUpdateInput struct {
	// The name of the bucket to create.
	Bucket string

	// The canned ACL to apply to the bucket.
	ACL *BucketCannedACL

	// Whether to enable public object listing.
	PublicObjectsListEnabled *bool

	// The website configuration for the bucket.
	Website *BucketWebsiteConfig

	// The shadow bucket configuration for the bucket.
	Shadow *BucketShadowConfig

	// The default storage tier for the bucket (set at creation).
	DefaultStorageTier *StorageTier

	// The location regions for the bucket.
	LocationRegions []string

	// Whether to enable snapshots (set at creation, cannot be changed).
	EnableSnapshot *bool

	// The source bucket for creating a fork.
	ForkSourceBucket *string

	// The snapshot version to fork from.
	ForkSourceSnapshot *string

	// Whether to enable delete protection for the bucket.
	DeleteProtection *bool
}

// BucketUpdateRequest is the request body for the UpdateBucket API.
type BucketUpdateRequest struct {
	Website       *BucketWebsiteConfig `json:"website,omitempty"`
	Shadow        *BucketShadowConfig  `json:"shadow_bucket,omitempty"`
	ObjectRegions *string              `json:"object_regions,omitempty"`
	Protection    *BucketProtection    `json:"protection,omitempty"`
}

type BucketUpdateResponse struct {
	// The success status of the update.
	Update string `json:"Update"`

	// The error message if the update failed.
	ErrorMessage string `json:"Message"`

	// The error code if the update failed.
	ErrorCode string `json:"Code"`
}
