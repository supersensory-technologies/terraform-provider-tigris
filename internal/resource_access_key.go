package internal

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/validation"
	"github.com/tigrisdata/terraform-provider-tigris/internal/names"
	"github.com/tigrisdata/terraform-provider-tigris/internal/types"
)

func resourceTigrisAccessKey() *schema.Resource {
	return &schema.Resource{
		Description:          "Provides a Tigris access key resource.",
		CreateWithoutTimeout: resourceAccessKeyCreate,
		ReadWithoutTimeout:   resourceAccessKeyRead,
		UpdateWithoutTimeout: resourceAccessKeyUpdate,
		DeleteWithoutTimeout: resourceAccessKeyDelete,

		Timeouts: &schema.ResourceTimeout{
			Create: schema.DefaultTimeout(20 * time.Minute),
			Read:   schema.DefaultTimeout(20 * time.Minute),
			Update: schema.DefaultTimeout(20 * time.Minute),
			Delete: schema.DefaultTimeout(20 * time.Minute),
		},

		Importer: &schema.ResourceImporter{
			StateContext: schema.ImportStatePassthroughContext,
		},

		Schema: map[string]*schema.Schema{
			names.AttrName: {
				Type:        schema.TypeString,
				Required:    true,
				ForceNew:    true,
				Description: "The access key name.",
			},
			names.AttrBucketRole: {
				Type:        schema.TypeList,
				Optional:    true,
				Description: "Bucket roles granted to this access key.",
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						names.AttrBucket: {
							Type:        schema.TypeString,
							Required:    true,
							Description: "The bucket name, or `*` for namespace-wide admin access.",
						},
						names.AttrRole: {
							Type:         schema.TypeString,
							Required:     true,
							Description:  "The role assigned to the bucket.",
							ValidateFunc: validation.StringInSlice(iamBucketRoleValues(), false),
						},
					},
				},
			},
			names.AttrAccessKeyID: {
				Type:        schema.TypeString,
				Computed:    true,
				Description: "The access key ID.",
			},
			names.AttrSecretAccessKey: {
				Type:        schema.TypeString,
				Computed:    true,
				Sensitive:   true,
				Description: "The secret access key. This is only returned when the key is created.",
			},
			names.AttrStatus: {
				Type:        schema.TypeString,
				Computed:    true,
				Description: "The current access key status.",
			},
		},
	}
}

func resourceAccessKeyCreate(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	svc := meta.(*Client)

	input := &types.AccessKeyCreateInput{
		Name:  d.Get(names.AttrName).(string),
		Roles: expandBucketRoles(d.Get(names.AttrBucketRole).([]interface{})),
	}
	if err := validateBucketRoles(input.Roles); err != nil {
		return diag.FromErr(err)
	}

	tflog.Info(ctx, "Creating access key", map[string]interface{}{
		"access_key_name": input.Name,
	})

	key, err := svc.CreateAccessKey(ctx, input)
	if err != nil {
		return diag.FromErr(fmt.Errorf("unable to create access key: %w", err))
	}

	d.SetId(key.ID)
	d.Set(names.AttrName, key.Name)
	d.Set(names.AttrBucketRole, flattenBucketRoles(key.Roles))
	d.Set(names.AttrAccessKeyID, key.ID)
	d.Set(names.AttrSecretAccessKey, key.Secret)
	d.Set(names.AttrStatus, key.Status)

	return nil
}

func resourceAccessKeyRead(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	svc := meta.(*Client)
	id := d.Id()

	tflog.Info(ctx, "Reading access key", map[string]interface{}{
		"access_key_id": id,
	})

	key, err := svc.GetAccessKey(ctx, id)
	if err != nil {
		if IsIAMNotFoundError(err) || strings.Contains(strings.ToLower(err.Error()), "not found") {
			d.SetId("")
			return nil
		}

		return diag.FromErr(fmt.Errorf("unable to read access key: %w", err))
	}

	d.Set(names.AttrName, key.Name)
	d.Set(names.AttrBucketRole, flattenBucketRoles(key.Roles))
	d.Set(names.AttrAccessKeyID, key.ID)
	d.Set(names.AttrStatus, key.Status)

	return nil
}

func resourceAccessKeyUpdate(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	svc := meta.(*Client)

	if !d.HasChange(names.AttrBucketRole) {
		return resourceAccessKeyRead(ctx, d, meta)
	}

	roles := expandBucketRoles(d.Get(names.AttrBucketRole).([]interface{}))
	if err := validateBucketRoles(roles); err != nil {
		return diag.FromErr(err)
	}

	tflog.Info(ctx, "Updating access key roles", map[string]interface{}{
		"access_key_id": d.Id(),
	})

	if err := svc.UpdateAccessKeyRoles(ctx, &types.AccessKeyUpdateInput{
		ID:    d.Id(),
		Roles: roles,
	}); err != nil {
		return diag.FromErr(fmt.Errorf("unable to update access key roles: %w", err))
	}

	return resourceAccessKeyRead(ctx, d, meta)
}

func resourceAccessKeyDelete(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	svc := meta.(*Client)

	tflog.Info(ctx, "Deleting access key", map[string]interface{}{
		"access_key_id": d.Id(),
	})

	err := svc.DeleteAccessKey(ctx, d.Id(), d.Get(names.AttrName).(string))
	if err != nil && !IsIAMNotFoundError(err) && !strings.Contains(strings.ToLower(err.Error()), "not found") {
		return diag.FromErr(fmt.Errorf("unable to delete access key: %w", err))
	}

	d.SetId("")
	return nil
}

func expandBucketRoles(roleList []interface{}) []types.BucketRole {
	roles := make([]types.BucketRole, 0, len(roleList))
	for _, rawRole := range roleList {
		roleMap := rawRole.(map[string]interface{})
		roles = append(roles, types.BucketRole{
			Bucket: roleMap[names.AttrBucket].(string),
			Role:   roleMap[names.AttrRole].(string),
		})
	}

	return roles
}

func flattenBucketRoles(roles []types.BucketRole) []interface{} {
	result := make([]interface{}, 0, len(roles))
	for _, role := range roles {
		result = append(result, map[string]interface{}{
			names.AttrBucket: role.Bucket,
			names.AttrRole:   role.Role,
		})
	}

	return result
}

func validateBucketRoles(roles []types.BucketRole) error {
	for _, role := range roles {
		if role.Bucket == "" {
			return fmt.Errorf("bucket role bucket must not be empty")
		}
		if role.Bucket == "*" && (len(roles) > 1 || role.Role != string(types.IAMBucketRoleNamespaceAdmin)) {
			return fmt.Errorf("wildcard bucket role requires a single role with role %q", types.IAMBucketRoleNamespaceAdmin)
		}
	}

	return nil
}

func iamBucketRoleValues() []string {
	var role types.IAMBucketRoleName

	values := make([]string, 0, len(role.Values()))
	for _, value := range role.Values() {
		values = append(values, string(value))
	}

	return values
}
