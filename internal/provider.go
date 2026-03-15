// provider.go
package internal

import (
	"fmt"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/validation"
)

func Provider() *schema.Provider {
	return &schema.Provider{
		Schema: map[string]*schema.Schema{
			"access_key": {
				Type:        schema.TypeString,
				Optional:    true,
				DefaultFunc: schema.EnvDefaultFunc("AWS_ACCESS_KEY_ID", nil),
				Description: "The access key. It can also be sourced from the AWS_ACCESS_KEY_ID environment variable.",
			},
			"secret_key": {
				Type:        schema.TypeString,
				Optional:    true,
				Sensitive:   true,
				DefaultFunc: schema.EnvDefaultFunc("AWS_SECRET_ACCESS_KEY", nil),
				Description: "The secret key. It can also be sourced from the AWS_SECRET_ACCESS_KEY environment variable.",
			},
			"endpoint": {
				Type:        schema.TypeString,
				Optional:    true,
				Default:     DefaultEndpoint,
				Description: "The endpoint for the Tigris object storage service.",
			},
			"iam_endpoint": {
				Type:         schema.TypeString,
				Optional:     true,
				DefaultFunc:  schema.EnvDefaultFunc("TIGRIS_IAM_ENDPOINT", DefaultIAMEndpoint),
				ValidateFunc: validation.IsURLWithHTTPS,
				Description:  "The endpoint for the Tigris IAM service. It can also be sourced from the TIGRIS_IAM_ENDPOINT environment variable.",
			},
		},
		ResourcesMap: map[string]*schema.Resource{
			"tigris_bucket":                resourceTigrisBucket(),
			"tigris_bucket_public_access":  resourceTigrisBucketPublicAccess(),
			"tigris_bucket_website_config": resourceTigrisBucketWebsiteConfig(),
			"tigris_bucket_shadow_config":  resourceTigrisBucketShadowConfig(),
			"tigris_bucket_snapshot":       resourceTigrisBucketSnapshot(),
			"tigris_bucket_fork":           resourceTigrisBucketFork(),
			"tigris_iam_policy":            resourceTigrisIAMPolicy(),
			"tigris_access_key":            resourceTigrisAccessKey(),
		},
		ConfigureFunc: providerConfigure,
	}
}

func providerConfigure(d *schema.ResourceData) (interface{}, error) {
	accessKey := d.Get("access_key").(string)
	secretKey := d.Get("secret_key").(string)
	endpoint := d.Get("endpoint").(string)
	iamEndpoint := d.Get("iam_endpoint").(string)

	svc, err := NewClient(endpoint, iamEndpoint, accessKey, secretKey)
	if err != nil {
		return nil, fmt.Errorf("unable to load SDK config, %w", err)
	}

	return svc, nil
}
