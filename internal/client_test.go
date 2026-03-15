package internal

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	v4 "github.com/aws/aws-sdk-go-v2/aws/signer/v4"
)

// newTestClient creates a Client that talks to the given test server
// with minimal retry delay for fast tests.
func newTestClient(t *testing.T, serverURL string) *Client {
	t.Helper()
	return &Client{
		signer: v4.NewSigner(),
		credentials: aws.Credentials{
			AccessKeyID:     "test-key",
			SecretAccessKey: "test-secret",
		},
		endpoint:       serverURL,
		httpClient:     &http.Client{},
		retryBaseDelay: 1 * time.Millisecond,
	}
}

func writeBody(t *testing.T, w http.ResponseWriter, body string) {
	t.Helper()
	if _, err := w.Write([]byte(body)); err != nil {
		t.Errorf("failed to write response body: %v", err)
	}
}

func TestDoRequestWithRetry_Success(t *testing.T) {
	t.Parallel()

	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusOK)
		writeBody(t, w, "OK")
	}))
	defer srv.Close()

	client := newTestClient(t, srv.URL)
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, srv.URL+"/test", nil)
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}

	resp, err := client.doRequestWithRetry(req)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	if calls.Load() != 1 {
		t.Fatalf("expected 1 call, got %d", calls.Load())
	}
}

func TestDoRequestWithRetry_RetriesThenSucceeds(t *testing.T) {
	t.Parallel()

	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := calls.Add(1)
		if n <= 2 {
			w.WriteHeader(http.StatusInternalServerError)
			writeBody(t, w, "error")
			return
		}
		w.WriteHeader(http.StatusOK)
		writeBody(t, w, "OK")
	}))
	defer srv.Close()

	client := newTestClient(t, srv.URL)
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, srv.URL+"/test", nil)
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}

	resp, err := client.doRequestWithRetry(req)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	if calls.Load() != 3 {
		t.Fatalf("expected 3 calls (2 failures + 1 success), got %d", calls.Load())
	}
}

func TestDoRequestWithRetry_ExhaustsRetries(t *testing.T) {
	t.Parallel()

	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusBadGateway)
		writeBody(t, w, "bad gateway")
	}))
	defer srv.Close()

	client := newTestClient(t, srv.URL)
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, srv.URL+"/test", nil)
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}

	resp, err := client.doRequestWithRetry(req) //nolint:bodyclose // resp is nil on exhaustion
	if err == nil {
		t.Fatalf("expected error after retry exhaustion, got nil")
	}
	if resp != nil {
		t.Fatalf("expected nil response after retry exhaustion, got %v", resp)
	}
	if !strings.Contains(err.Error(), "failed after 5 retries") {
		t.Fatalf("expected retry exhaustion error message, got: %v", err)
	}
	if !strings.Contains(err.Error(), "502") {
		t.Fatalf("expected status code in error message, got: %v", err)
	}
	if calls.Load() != 5 {
		t.Fatalf("expected 5 calls, got %d", calls.Load())
	}
}

func TestDoRequestWithRetry_RespectsContextCancellation(t *testing.T) {
	t.Parallel()

	called := make(chan struct{}, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case called <- struct{}{}:
		default:
		}
		w.WriteHeader(http.StatusInternalServerError)
		writeBody(t, w, "error")
	}))
	defer srv.Close()

	client := newTestClient(t, srv.URL)
	// Use a longer backoff so the cancellation hits during the wait
	client.retryBaseDelay = 5 * time.Second

	ctx, cancel := context.WithCancel(context.Background())
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL+"/test", nil)
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}

	go func() {
		<-called
		cancel()
	}()

	resp, err := client.doRequestWithRetry(req) //nolint:bodyclose // resp is nil on cancellation
	if err == nil {
		t.Fatalf("expected context cancellation error, got nil")
	}
	if resp != nil {
		t.Fatalf("expected nil response on cancellation, got %v", resp)
	}
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got: %v", err)
	}
}

func TestDoRequestWithRetry_WithBody(t *testing.T) {
	t.Parallel()

	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := calls.Add(1)
		if n <= 1 {
			w.WriteHeader(http.StatusInternalServerError)
			writeBody(t, w, "error")
			return
		}
		w.WriteHeader(http.StatusOK)
		writeBody(t, w, "OK")
	}))
	defer srv.Close()

	client := newTestClient(t, srv.URL)
	body := strings.NewReader("test-body")
	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, srv.URL+"/test", body)
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}

	resp, err := client.doRequestWithRetry(req)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	if calls.Load() != 2 {
		t.Fatalf("expected 2 calls (1 failure + 1 success), got %d", calls.Load())
	}
}
