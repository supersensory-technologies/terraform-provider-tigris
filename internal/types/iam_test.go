package types

import (
	"encoding/json"
	"testing"
)

func TestListAccessKeysResponseNumericCreatedAt(t *testing.T) {
	t.Parallel()

	// The API can return created_at as a JSON number (epoch millis) instead of a string.
	// This test verifies json.Number handles that correctly.
	input := `{
		"Marker": "",
		"IsTruncated": false,
		"Keys": [
			{
				"access_key_id": "tid_key_abc123",
				"username": "test-key",
				"created_at": 1770550772652,
				"status": "Active",
				"namespace_id": "ns_123",
				"buckets_role": [{"bucket": "my-bucket", "role": "Editor"}]
			}
		]
	}`

	var resp ListAccessKeysResponse
	if err := json.Unmarshal([]byte(input), &resp); err != nil {
		t.Fatalf("failed to unmarshal numeric created_at: %v", err)
	}

	if len(resp.Keys) != 1 {
		t.Fatalf("expected 1 key, got %d", len(resp.Keys))
	}

	key := resp.Keys[0]
	if key.AccessKeyID != "tid_key_abc123" {
		t.Fatalf("unexpected access_key_id: got %q", key.AccessKeyID)
	}
	if key.CreatedAt.String() != "1770550772652" {
		t.Fatalf("unexpected created_at: got %q, want %q", key.CreatedAt.String(), "1770550772652")
	}
	if key.Status != "Active" {
		t.Fatalf("unexpected status: got %q", key.Status)
	}
	if len(key.BucketsRole) != 1 || key.BucketsRole[0].Bucket != "my-bucket" || key.BucketsRole[0].Role != "Editor" {
		t.Fatalf("unexpected buckets_role: got %+v", key.BucketsRole)
	}
}

func TestStringListUnmarshal(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name    string
		input   string
		want    []string
		wantErr bool
	}{
		{
			name:  "single string",
			input: `"s3:GetObject"`,
			want:  []string{"s3:GetObject"},
		},
		{
			name:  "string list",
			input: `["s3:GetObject","s3:PutObject"]`,
			want:  []string{"s3:GetObject", "s3:PutObject"},
		},
		{
			name:    "invalid payload",
			input:   `123`,
			wantErr: true,
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var got StringList
			err := json.Unmarshal([]byte(tc.input), &got)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error for %q", tc.input)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if len(got) != len(tc.want) {
				t.Fatalf("unexpected length: got %d want %d", len(got), len(tc.want))
			}

			for i := range tc.want {
				if got[i] != tc.want[i] {
					t.Fatalf("unexpected value at %d: got %q want %q", i, got[i], tc.want[i])
				}
			}
		})
	}
}
