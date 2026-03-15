package internal

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/YakDriver/regexache"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/validation"
	"github.com/tigrisdata/terraform-provider-tigris/internal/names"
	"github.com/tigrisdata/terraform-provider-tigris/internal/types"
)

func resourceTigrisBucket() *schema.Resource {
	return &schema.Resource{
		Description:          "Provides a Tigris bucket resource. This can be used to create and manage Tigris buckets.",
		CreateWithoutTimeout: resourceBucketCreate,
		ReadWithoutTimeout:   resourceBucketRead,
		UpdateWithoutTimeout: resourceBucketUpdate,
		DeleteWithoutTimeout: resourceBucketDelete,

		Timeouts: &schema.ResourceTimeout{
			Create: schema.DefaultTimeout(20 * time.Minute),
			Read:   schema.DefaultTimeout(20 * time.Minute),
			Update: schema.DefaultTimeout(20 * time.Minute),
			Delete: schema.DefaultTimeout(60 * time.Minute),
		},

		Importer: &schema.ResourceImporter{
			StateContext: schema.ImportStatePassthroughContext,
		},

		Schema: map[string]*schema.Schema{
			names.AttrBucket: {
				Type:        schema.TypeString,
				Required:    true,
				Description: "The name of the Tigris bucket.",
			},
			names.AttrLocation: {
				Type:        schema.TypeList,
				Optional:    true,
				Computed:    true,
				MaxItems:    1,
				Description: "The location configuration for the bucket. Controls data placement and replication.",
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						names.AttrLocationType: {
							Type:         schema.TypeString,
							Required:     true,
							Description:  "The location type: global, multi, dual, or single.",
							ValidateFunc: validation.StringInSlice(locationTypeValues(), false),
						},
						names.AttrLocationRegions: {
							Type:        schema.TypeSet,
							Optional:    true,
							Description: "The region codes. For multi: usa or eur. For single/dual: specific region codes like sjc, iad, ams, etc.",
							Elem: &schema.Schema{
								Type: schema.TypeString,
							},
						},
					},
				},
			},
			names.AttrDefaultStorageTier: {
				Type:         schema.TypeString,
				Optional:     true,
				Computed:     true,
				ForceNew:     true,
				Description:  "The default storage tier for objects in the bucket: STANDARD, STANDARD_IA, GLACIER, or GLACIER_IR.",
				ValidateFunc: validation.StringInSlice(storageTierValues(), false),
			},
			names.AttrEnableSnapshot: {
				Type:        schema.TypeBool,
				Optional:    true,
				Default:     false,
				ForceNew:    true,
				Description: "Enable snapshots for this bucket. Cannot be changed after creation.",
			},
			names.AttrDeleteProtection: {
				Type:        schema.TypeBool,
				Optional:    true,
				Default:     false,
				Description: "Enable deletion protection for this bucket. When enabled, the bucket cannot be deleted.",
			},
		},
	}
}

func resourceBucketCreate(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	svc := meta.(*Client)

	bucketName := d.Get(names.AttrBucket).(string)
	if err := validBucketName(bucketName); err != nil {
		return diag.FromErr(fmt.Errorf("invalid bucket name, %w", err))
	}

	input := &types.BucketUpdateInput{
		Bucket: bucketName,
	}

	// Handle location.
	if v, ok := d.GetOk(names.AttrLocation); ok {
		locationList := v.([]interface{})
		if len(locationList) > 0 {
			locationMap := locationList[0].(map[string]interface{})
			locationType := locationMap[names.AttrLocationType].(string)
			regionsRaw := locationMap[names.AttrLocationRegions].(*schema.Set).List()
			regions := make([]string, len(regionsRaw))
			for i, r := range regionsRaw {
				regions[i] = r.(string)
			}
			if err := validateLocation(locationType, regions); err != nil {
				return diag.FromErr(err)
			}
			if types.LocationType(locationType) != types.LocationTypeGlobal {
				input.LocationRegions = regions
			}
		}
	}

	// Handle default storage tier.
	if v, ok := d.GetOk(names.AttrDefaultStorageTier); ok {
		tier := types.StorageTier(v.(string))
		input.DefaultStorageTier = &tier
	}

	// Handle enable snapshot.
	if v, ok := d.GetOk(names.AttrEnableSnapshot); ok && v.(bool) {
		enableSnapshot := true
		input.EnableSnapshot = &enableSnapshot
	}

	tflog.Info(ctx, "Creating bucket", map[string]interface{}{
		"bucket_name": bucketName,
	})

	err := svc.CreateBucket(ctx, input)
	if err != nil {
		return diag.FromErr(fmt.Errorf("unable to create bucket, %w", err))
	}

	tflog.Info(ctx, "Bucket created successfully", map[string]interface{}{
		"bucket_name": bucketName,
	})

	d.SetId(bucketName)

	// Deletion protection cannot be set at creation time; apply as a post-create update.
	if d.Get(names.AttrDeleteProtection).(bool) {
		deleteProtection := true
		protectionInput := &types.BucketUpdateInput{
			Bucket:           bucketName,
			DeleteProtection: &deleteProtection,
		}

		tflog.Info(ctx, "Enabling delete protection on bucket", map[string]interface{}{
			"bucket_name": bucketName,
		})

		if err := svc.UpdateBucket(ctx, protectionInput); err != nil {
			// Roll back: delete the bucket so Terraform doesn't store a
			// tainted resource with deletion_protection=true in state
			// while the API has it disabled — that would permanently
			// block destroy.
			d.SetId("")
			if deleteErr := svc.DeleteBucket(ctx, bucketName); deleteErr != nil {
				return diag.FromErr(fmt.Errorf(
					"unable to enable deletion protection (%w); also failed to roll back bucket: %w",
					err, deleteErr,
				))
			}
			return diag.FromErr(fmt.Errorf(
				"unable to enable deletion protection (bucket rolled back): %w", err,
			))
		}
	}

	return resourceBucketRead(ctx, d, meta)
}

func resourceBucketRead(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	svc := meta.(*Client)

	bucketName := d.Id()

	tflog.Info(ctx, "Checking bucket existence", map[string]interface{}{
		"bucket_name": bucketName,
	})

	exists, err := svc.HeadBucket(ctx, bucketName)
	if err != nil {
		return diag.FromErr(fmt.Errorf("unable to read bucket, %w", err))
	}
	if !exists {
		tflog.Warn(ctx, "Bucket not found, removing from state", map[string]interface{}{
			"bucket_name": bucketName,
		})

		d.SetId("")
		return nil
	}

	// Fetch metadata to read location and storage tier.
	metadata, err := svc.GetBucketMetadata(ctx, bucketName)
	if err != nil {
		return diag.FromErr(fmt.Errorf("unable to read bucket metadata, %w", err))
	}

	var diags diag.Diagnostics

	diags = append(diags, diag.FromErr(d.Set(names.AttrBucket, bucketName))...)
	diags = append(diags, diag.FromErr(d.Set(names.AttrDefaultStorageTier, metadata.GetStorageClass()))...)

	locationType, regions := metadata.GetLocationTypeAndRegions()
	locationBlock := map[string]interface{}{
		names.AttrLocationType:    string(locationType),
		names.AttrLocationRegions: regions,
	}
	diags = append(diags, diag.FromErr(d.Set(names.AttrLocation, []interface{}{locationBlock}))...)
	diags = append(diags, diag.FromErr(d.Set(names.AttrEnableSnapshot, metadata.IsSnapshotEnabled()))...)
	diags = append(diags, diag.FromErr(d.Set(names.AttrDeleteProtection, metadata.IsDeleteProtectionEnabled()))...)

	return diags
}

func resourceBucketUpdate(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	svc := meta.(*Client)

	bucketName := d.Id()

	input := &types.BucketUpdateInput{
		Bucket: bucketName,
	}
	needsUpdate := false

	if d.HasChange(names.AttrLocation) {
		v := d.Get(names.AttrLocation)
		locationList := v.([]interface{})
		if len(locationList) > 0 {
			locationMap := locationList[0].(map[string]interface{})
			locationType := locationMap[names.AttrLocationType].(string)
			regionsRaw := locationMap[names.AttrLocationRegions].(*schema.Set).List()
			regions := make([]string, len(regionsRaw))
			for i, r := range regionsRaw {
				regions[i] = r.(string)
			}
			if err := validateLocation(locationType, regions); err != nil {
				return diag.FromErr(err)
			}
			if types.LocationType(locationType) == types.LocationTypeGlobal {
				input.LocationRegions = []string{}
			} else {
				input.LocationRegions = regions
			}
			needsUpdate = true
		}
	}

	if d.HasChange(names.AttrDeleteProtection) {
		deleteProtection := d.Get(names.AttrDeleteProtection).(bool)
		input.DeleteProtection = &deleteProtection
		needsUpdate = true
	}

	if needsUpdate {
		tflog.Info(ctx, "Updating bucket", map[string]interface{}{
			"bucket_name": bucketName,
		})

		if err := svc.UpdateBucket(ctx, input); err != nil {
			return diag.FromErr(fmt.Errorf("unable to update bucket, %w", err))
		}
	}

	return resourceBucketRead(ctx, d, meta)
}

func resourceBucketDelete(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	svc := meta.(*Client)

	bucketName := d.Id()

	if d.Get(names.AttrDeleteProtection).(bool) {
		return diag.Errorf(
			"bucket %q has %s enabled; set %s = false before destroying",
			bucketName, names.AttrDeleteProtection, names.AttrDeleteProtection,
		)
	}

	err := svc.DeleteBucket(ctx, bucketName)
	if err != nil {
		return diag.FromErr(fmt.Errorf("unable to delete bucket, %w", err))
	}

	d.SetId("")
	return nil
}

// validBucketName validates bucket name. Buckets names have to be DNS-compliant.
func validBucketName(value string) error {
	if (len(value) < 3) || (len(value) > 63) {
		return fmt.Errorf("%q must contain from 3 to 63 characters", value)
	}
	if !regexache.MustCompile(`^[0-9a-z-.]+$`).MatchString(value) {
		return fmt.Errorf("only lowercase alphanumeric characters and hyphens allowed in %q", value)
	}
	if regexache.MustCompile(`^(?:[0-9]{1,3}\.){3}[0-9]{1,3}$`).MatchString(value) {
		return fmt.Errorf("%q must not be formatted as an IP address", value)
	}
	if strings.HasPrefix(value, `.`) {
		return fmt.Errorf("%q cannot start with a period", value)
	}
	if strings.HasSuffix(value, `.`) {
		return fmt.Errorf("%q cannot end with a period", value)
	}
	if strings.Contains(value, `..`) {
		return fmt.Errorf("%q can be only one period between labels", value)
	}

	return nil
}

func locationTypeValues() []string {
	var lt types.LocationType
	values := make([]string, 0, len(lt.Values()))
	for _, v := range lt.Values() {
		values = append(values, string(v))
	}
	return values
}

func storageTierValues() []string {
	var st types.StorageTier
	values := make([]string, 0, len(st.Values()))
	for _, v := range st.Values() {
		values = append(values, string(v))
	}
	return values
}

func validateLocation(locationType string, regions []string) error {
	lt := types.LocationType(locationType)

	switch lt {
	case types.LocationTypeGlobal:
		if len(regions) > 0 {
			return fmt.Errorf("regions must not be specified for global location type")
		}
	case types.LocationTypeMulti:
		if len(regions) != 1 {
			return fmt.Errorf("exactly one geography must be specified for multi location type (usa or eur)")
		}
		if !stringInSlice(regions[0], types.ValidMultiRegions) {
			return fmt.Errorf("invalid multi-region geography %q, must be one of: %s", regions[0], strings.Join(types.ValidMultiRegions, ", "))
		}
	case types.LocationTypeDual:
		if len(regions) < 2 {
			return fmt.Errorf("at least two regions must be specified for dual location type")
		}
		for _, r := range regions {
			if !stringInSlice(r, types.ValidSingleRegions) {
				return fmt.Errorf("invalid region %q for dual location type, must be one of: %s", r, strings.Join(types.ValidSingleRegions, ", "))
			}
		}
	case types.LocationTypeSingle:
		if len(regions) != 1 {
			return fmt.Errorf("exactly one region must be specified for single location type")
		}
		if !stringInSlice(regions[0], types.ValidSingleRegions) {
			return fmt.Errorf("invalid region %q for single location type, must be one of: %s", regions[0], strings.Join(types.ValidSingleRegions, ", "))
		}
	default:
		return fmt.Errorf("invalid location type %q", locationType)
	}

	return nil
}

func stringInSlice(s string, list []string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}
