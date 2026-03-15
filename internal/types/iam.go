package types

import (
	"bytes"
	"encoding/json"
	"fmt"
)

const IAMPolicyVersion = "2012-10-17"

type IAMPolicyEffect string

const (
	IAMPolicyEffectAllow IAMPolicyEffect = "Allow"
	IAMPolicyEffectDeny  IAMPolicyEffect = "Deny"
)

func (IAMPolicyEffect) Values() []IAMPolicyEffect {
	return []IAMPolicyEffect{
		IAMPolicyEffectAllow,
		IAMPolicyEffectDeny,
	}
}

type IAMBucketRoleName string

const (
	IAMBucketRoleEditor         IAMBucketRoleName = "Editor"
	IAMBucketRoleReadOnly       IAMBucketRoleName = "ReadOnly"
	IAMBucketRoleNamespaceAdmin IAMBucketRoleName = "NamespaceAdmin"
)

func (IAMBucketRoleName) Values() []IAMBucketRoleName {
	return []IAMBucketRoleName{
		IAMBucketRoleEditor,
		IAMBucketRoleReadOnly,
		IAMBucketRoleNamespaceAdmin,
	}
}

// StringList accepts either a single string or a JSON string array in API responses.
type StringList []string

func (s *StringList) UnmarshalJSON(data []byte) error {
	trimmed := bytes.TrimSpace(data)
	if bytes.Equal(trimmed, []byte("null")) {
		*s = nil
		return nil
	}

	if len(trimmed) > 0 && trimmed[0] == '[' {
		var values []string
		if err := json.Unmarshal(trimmed, &values); err != nil {
			return err
		}
		*s = values
		return nil
	}

	var value string
	if err := json.Unmarshal(trimmed, &value); err != nil {
		return fmt.Errorf("expected string or string array: %w", err)
	}

	*s = []string{value}
	return nil
}

type PolicyStatement struct {
	Effect   string     `json:"Effect"`
	Action   StringList `json:"Action"`
	Resource StringList `json:"Resource"`
}

type PolicyDocument struct {
	Version   string            `json:"Version"`
	Statement []PolicyStatement `json:"Statement"`
}

type Policy struct {
	AttachmentCount int    `json:"AttachmentCount"`
	CreateDate      string `json:"CreateDate"`
	DefaultVersion  string `json:"DefaultVersionId"`
	Description     string `json:"Description"`
	ID              string `json:"PolicyId"`
	Name            string `json:"PolicyName"`
	Path            string `json:"Path"`
	ARN             string `json:"Arn"`
	UpdateDate      string `json:"UpdateDate"`
}

type PolicyDetailed struct {
	Policy
	Document PolicyDocument `json:"-"`
	Users    []string       `json:"Users"`
}

type PolicyCreateInput struct {
	Name        string
	Description string
	Document    PolicyDocument
}

type PolicyUpdateInput struct {
	ARN         string
	Description string
	Document    PolicyDocument
}

type CreatePolicyResponse struct {
	CreatePolicyResult struct {
		Policy Policy `json:"Policy"`
	} `json:"CreatePolicyResult"`
}

type UpdatePolicyResponse struct {
	UpdatePolicyResult struct {
		Policy Policy `json:"Policy"`
	} `json:"UpdatePolicyResult"`
}

type GetPolicyResponse struct {
	PolicyDetailed struct {
		Policy
		Document string   `json:"Document"`
		Users    []string `json:"Users"`
	} `json:"PolicyDetailed"`
}

type BucketRole struct {
	Bucket string `json:"bucket"`
	Role   string `json:"role"`
}

type AccessKey struct {
	ID             string       `json:"-"`
	Name           string       `json:"-"`
	Secret         string       `json:"-"`
	CreatedAt      string       `json:"-"`
	Status         string       `json:"-"`
	OrganizationID string       `json:"-"`
	Roles          []BucketRole `json:"-"`
}

type AccessKeyCreateInput struct {
	Name  string
	Roles []BucketRole
}

type AccessKeyUpdateInput struct {
	ID    string
	Roles []BucketRole
}

type CreateAccessKeyResponse struct {
	CreateAccessKeyResult struct {
		AccessKey struct {
			AccessKeyID     string `json:"AccessKeyId"`
			SecretAccessKey string `json:"SecretAccessKey"`
			UserName        string `json:"UserName"`
			CreateDate      string `json:"CreateDate"`
		} `json:"AccessKey"`
	} `json:"CreateAccessKeyResult"`
}

type ListAccessKeysResponse struct {
	Marker      string                  `json:"Marker"`
	IsTruncated bool                    `json:"IsTruncated"`
	Keys        []ListAccessKeysKeyItem `json:"Keys"`
}

type ListAccessKeysKeyItem struct {
	AccessKeyID string       `json:"access_key_id"`
	UserName    string       `json:"username"`
	CreatedAt   json.Number  `json:"created_at"`
	Status      string       `json:"status"`
	NamespaceID string       `json:"namespace_id"`
	BucketsRole []BucketRole `json:"buckets_role"`
}

type UpdateAccessKeyRolesResponse struct {
	Status  string `json:"status"`
	Message string `json:"message"`
}
