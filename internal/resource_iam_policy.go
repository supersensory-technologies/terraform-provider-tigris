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

func resourceTigrisIAMPolicy() *schema.Resource {
	return &schema.Resource{
		Description:          "Provides a Tigris IAM policy resource.",
		CreateWithoutTimeout: resourceIAMPolicyCreate,
		ReadWithoutTimeout:   resourceIAMPolicyRead,
		UpdateWithoutTimeout: resourceIAMPolicyUpdate,
		DeleteWithoutTimeout: resourceIAMPolicyDelete,

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
				Description: "The IAM policy name.",
			},
			names.AttrDescription: {
				Type:        schema.TypeString,
				Optional:    true,
				Description: "The IAM policy description.",
			},
			names.AttrStatement: {
				Type:        schema.TypeList,
				Required:    true,
				MinItems:    1,
				Description: "The policy statements for this document.",
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						names.AttrEffect: {
							Type:         schema.TypeString,
							Required:     true,
							Description:  "The statement effect. Must be Allow or Deny.",
							ValidateFunc: validation.StringInSlice(iamPolicyEffectValues(), false),
						},
						names.AttrActions: {
							Type:        schema.TypeList,
							Required:    true,
							MinItems:    1,
							Description: "The S3 actions granted or denied by this statement.",
							Elem: &schema.Schema{
								Type: schema.TypeString,
							},
						},
						names.AttrResources: {
							Type:        schema.TypeList,
							Required:    true,
							MinItems:    1,
							Description: "The resource ARNs referenced by this statement.",
							Elem: &schema.Schema{
								Type: schema.TypeString,
							},
						},
					},
				},
			},
			names.AttrArn: {
				Type:        schema.TypeString,
				Computed:    true,
				Description: "The ARN of the IAM policy.",
			},
			names.AttrPolicyID: {
				Type:        schema.TypeString,
				Computed:    true,
				Description: "The IAM policy identifier.",
			},
		},
	}
}

func resourceIAMPolicyCreate(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	svc := meta.(*Client)

	input := &types.PolicyCreateInput{
		Name:        d.Get(names.AttrName).(string),
		Description: d.Get(names.AttrDescription).(string),
		Document:    expandPolicyDocument(d.Get(names.AttrStatement).([]interface{})),
	}

	tflog.Info(ctx, "Creating IAM policy", map[string]interface{}{
		"policy_name": input.Name,
	})

	policy, err := svc.CreatePolicy(ctx, input)
	if err != nil {
		return diag.FromErr(fmt.Errorf("unable to create IAM policy: %w", err))
	}

	d.SetId(policy.ARN)

	return resourceIAMPolicyRead(ctx, d, meta)
}

func resourceIAMPolicyRead(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	svc := meta.(*Client)
	arn := d.Id()

	tflog.Info(ctx, "Reading IAM policy", map[string]interface{}{
		"policy_arn": arn,
	})

	policy, err := svc.GetPolicy(ctx, arn)
	if err != nil {
		if IsIAMNotFoundError(err) || strings.Contains(strings.ToLower(err.Error()), "not found") {
			d.SetId("")
			return nil
		}

		return diag.FromErr(fmt.Errorf("unable to read IAM policy: %w", err))
	}

	d.Set(names.AttrName, policy.Name)
	d.Set(names.AttrDescription, policy.Description)
	d.Set(names.AttrStatement, flattenPolicyDocument(policy.Document))
	d.Set(names.AttrArn, policy.ARN)
	d.Set(names.AttrPolicyID, policy.ID)

	return nil
}

func resourceIAMPolicyUpdate(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	svc := meta.(*Client)

	if !d.HasChanges(names.AttrDescription, names.AttrStatement) {
		return resourceIAMPolicyRead(ctx, d, meta)
	}

	input := &types.PolicyUpdateInput{
		ARN:         d.Id(),
		Description: d.Get(names.AttrDescription).(string),
		Document:    expandPolicyDocument(d.Get(names.AttrStatement).([]interface{})),
	}

	tflog.Info(ctx, "Updating IAM policy", map[string]interface{}{
		"policy_arn": input.ARN,
	})

	if _, err := svc.UpdatePolicy(ctx, input); err != nil {
		return diag.FromErr(fmt.Errorf("unable to update IAM policy: %w", err))
	}

	return resourceIAMPolicyRead(ctx, d, meta)
}

func resourceIAMPolicyDelete(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	svc := meta.(*Client)
	arn := d.Id()

	tflog.Info(ctx, "Deleting IAM policy", map[string]interface{}{
		"policy_arn": arn,
	})

	if err := svc.DeletePolicy(ctx, arn); err != nil && !IsIAMNotFoundError(err) && !strings.Contains(strings.ToLower(err.Error()), "not found") {
		return diag.FromErr(fmt.Errorf("unable to delete IAM policy: %w", err))
	}

	d.SetId("")
	return nil
}

func expandPolicyDocument(statementList []interface{}) types.PolicyDocument {
	statements := make([]types.PolicyStatement, 0, len(statementList))
	for _, rawStatement := range statementList {
		statementMap := rawStatement.(map[string]interface{})
		statements = append(statements, types.PolicyStatement{
			Effect:   statementMap[names.AttrEffect].(string),
			Action:   stringListFromInterfaces(statementMap[names.AttrActions].([]interface{})),
			Resource: stringListFromInterfaces(statementMap[names.AttrResources].([]interface{})),
		})
	}

	return types.PolicyDocument{
		Version:   types.IAMPolicyVersion,
		Statement: statements,
	}
}

func flattenPolicyDocument(document types.PolicyDocument) []interface{} {
	statements := make([]interface{}, 0, len(document.Statement))
	for _, statement := range document.Statement {
		statements = append(statements, map[string]interface{}{
			names.AttrEffect:    statement.Effect,
			names.AttrActions:   []string(statement.Action),
			names.AttrResources: []string(statement.Resource),
		})
	}

	return statements
}

func iamPolicyEffectValues() []string {
	var effect types.IAMPolicyEffect

	values := make([]string, 0, len(effect.Values()))
	for _, value := range effect.Values() {
		values = append(values, string(value))
	}

	return values
}

func stringListFromInterfaces(values []interface{}) types.StringList {
	result := make([]string, 0, len(values))
	for _, value := range values {
		result = append(result, value.(string))
	}
	return types.StringList(result)
}
