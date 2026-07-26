package internal

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	v4 "github.com/aws/aws-sdk-go-v2/aws/signer/v4"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/tigrisdata/terraform-provider-tigris/internal/names"
)

// newRawTestClient extends newS3TestClient with the fields the raw signed
// request path needs. That helper only wires the S3 client, because the tests
// using it never leave the S3 SDK; enabling delete protection is a hand-rolled
// PATCH, which needs the signer and HTTP client too.
func newRawTestClient(t *testing.T, serverURL string) *Client {
	t.Helper()

	c := newS3TestClient(t, serverURL)
	c.signer = v4.NewSigner()
	c.httpClient = &http.Client{}
	c.retryBaseDelay = time.Millisecond

	return c
}

// Delete protection cannot be set at creation time, so the provider creates the
// bucket and then PATCHes it. When that PATCH fails the bucket is rolled back,
// and the two possible outcomes of that rollback must leave Terraform in
// different states: returning an error with an empty ID makes Terraform forget
// the resource, which is only correct if the bucket is really gone.

// protectionFailureServer creates successfully, refuses to enable protection,
// and answers DELETE with deleteStatus. A 4xx is used for the failures so the
// client's 5xx retry loop does not slow the test down.
func protectionFailureServer(t *testing.T, deleteStatus int) *httptest.Server {
	t.Helper()

	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPut: // CreateBucket
			w.WriteHeader(http.StatusOK)
		case http.MethodPatch: // enable delete protection
			writeXML(t, w, http.StatusBadRequest,
				`<?xml version="1.0" encoding="UTF-8"?><Error><Code>InvalidRequest</Code></Error>`)
		case http.MethodDelete: // rollback
			if deleteStatus >= 300 {
				writeXML(t, w, deleteStatus,
					`<?xml version="1.0" encoding="UTF-8"?><Error><Code>AccessDenied</Code></Error>`)
				return
			}
			w.WriteHeader(deleteStatus)
		default:
			w.WriteHeader(http.StatusOK)
		}
	}))
}

func newProtectedBucketData(t *testing.T) *schema.ResourceData {
	t.Helper()

	return schema.TestResourceDataRaw(t, resourceTigrisBucket().Schema, map[string]interface{}{
		names.AttrBucket:           "my-bucket",
		names.AttrDeleteProtection: true,
	})
}

// A successful rollback means the bucket no longer exists, so the ID must be
// cleared and Terraform must not keep the resource.
func TestBucketCreate_RollbackSucceeds_ClearsID(t *testing.T) {
	t.Parallel()

	srv := protectionFailureServer(t, http.StatusNoContent)
	defer srv.Close()

	d := newProtectedBucketData(t)

	diags := resourceBucketCreate(context.Background(), d, newRawTestClient(t, srv.URL))

	if !diags.HasError() {
		t.Fatal("expected an error when enabling delete protection fails")
	}
	if d.Id() != "" {
		t.Fatalf("rollback succeeded, so the ID must be cleared; got %q", d.Id())
	}
}

// A failed rollback means the bucket still exists remotely, and it may already
// have protection enabled if the PATCH succeeded with its response lost.
// Clearing the ID here would orphan it outside Terraform's management.
func TestBucketCreate_RollbackFails_RetainsID(t *testing.T) {
	t.Parallel()

	srv := protectionFailureServer(t, http.StatusForbidden)
	defer srv.Close()

	d := newProtectedBucketData(t)

	diags := resourceBucketCreate(context.Background(), d, newRawTestClient(t, srv.URL))

	if !diags.HasError() {
		t.Fatal("expected an error when both protection and rollback fail")
	}
	if d.Id() == "" {
		t.Fatal("rollback failed and the bucket still exists, so the ID must be retained")
	}
}
