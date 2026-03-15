package types

import (
	"encoding/json"
	"testing"
)

func TestBucketProtectionSerialization(t *testing.T) {
	t.Parallel()

	t.Run("marshal protection enabled", func(t *testing.T) {
		t.Parallel()

		req := BucketUpdateRequest{
			Protection: &BucketProtection{Protected: true},
		}
		data, err := json.Marshal(req)
		if err != nil {
			t.Fatalf("failed to marshal: %v", err)
		}

		want := `{"protection":{"protected":true}}`
		if string(data) != want {
			t.Fatalf("unexpected JSON: got %s, want %s", string(data), want)
		}
	})

	t.Run("marshal protection disabled", func(t *testing.T) {
		t.Parallel()

		req := BucketUpdateRequest{
			Protection: &BucketProtection{Protected: false},
		}
		data, err := json.Marshal(req)
		if err != nil {
			t.Fatalf("failed to marshal: %v", err)
		}

		want := `{"protection":{"protected":false}}`
		if string(data) != want {
			t.Fatalf("unexpected JSON: got %s, want %s", string(data), want)
		}
	})

	t.Run("marshal omits protection when nil", func(t *testing.T) {
		t.Parallel()

		req := BucketUpdateRequest{}
		data, err := json.Marshal(req)
		if err != nil {
			t.Fatalf("failed to marshal: %v", err)
		}

		want := `{}`
		if string(data) != want {
			t.Fatalf("unexpected JSON: got %s, want %s", string(data), want)
		}
	})

	t.Run("unmarshal metadata with protection.protected true", func(t *testing.T) {
		t.Parallel()

		input := `{
			"name": "test-bucket",
			"cache_control": "",
			"object_regions": "",
			"storage_class": "STANDARD",
			"type": 0,
			"protection": {
				"protected": true
			}
		}`

		var metadata BucketMetadata
		if err := json.Unmarshal([]byte(input), &metadata); err != nil {
			t.Fatalf("failed to unmarshal: %v", err)
		}

		if !metadata.IsDeleteProtectionEnabled() {
			t.Fatalf("expected delete protection to be enabled")
		}
	})

	t.Run("unmarshal metadata without protection", func(t *testing.T) {
		t.Parallel()

		input := `{
			"name": "test-bucket",
			"cache_control": "",
			"object_regions": "",
			"storage_class": "STANDARD",
			"type": 0
		}`

		var metadata BucketMetadata
		if err := json.Unmarshal([]byte(input), &metadata); err != nil {
			t.Fatalf("failed to unmarshal: %v", err)
		}

		if metadata.IsDeleteProtectionEnabled() {
			t.Fatalf("expected delete protection to be disabled when protection is absent")
		}
	})

	t.Run("unmarshal metadata with protection.protected false", func(t *testing.T) {
		t.Parallel()

		input := `{
			"name": "test-bucket",
			"protection": {
				"protected": false
			}
		}`

		var metadata BucketMetadata
		if err := json.Unmarshal([]byte(input), &metadata); err != nil {
			t.Fatalf("failed to unmarshal: %v", err)
		}

		if metadata.IsDeleteProtectionEnabled() {
			t.Fatalf("expected delete protection to be disabled")
		}
	})
}
