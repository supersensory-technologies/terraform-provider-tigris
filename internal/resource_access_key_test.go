package internal

import (
	"testing"

	"github.com/tigrisdata/terraform-provider-tigris/internal/types"
)

func TestValidateBucketRoles(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name    string
		roles   []types.BucketRole
		wantErr bool
	}{
		{
			name:  "empty roles",
			roles: nil,
		},
		{
			name: "bucket specific role",
			roles: []types.BucketRole{
				{Bucket: "example-bucket", Role: string(types.IAMBucketRoleEditor)},
			},
		},
		{
			name: "namespace admin wildcard",
			roles: []types.BucketRole{
				{Bucket: "*", Role: string(types.IAMBucketRoleNamespaceAdmin)},
			},
		},
		{
			name: "wildcard with non admin role",
			roles: []types.BucketRole{
				{Bucket: "*", Role: string(types.IAMBucketRoleEditor)},
			},
			wantErr: true,
		},
		{
			name: "wildcard mixed with specific bucket",
			roles: []types.BucketRole{
				{Bucket: "*", Role: string(types.IAMBucketRoleNamespaceAdmin)},
				{Bucket: "example-bucket", Role: string(types.IAMBucketRoleEditor)},
			},
			wantErr: true,
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			err := validateBucketRoles(tc.roles)
			if tc.wantErr && err == nil {
				t.Fatalf("expected error")
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}
