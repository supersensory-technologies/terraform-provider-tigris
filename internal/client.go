package internal

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	v4 "github.com/aws/aws-sdk-go-v2/aws/signer/v4"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	s3types "github.com/aws/aws-sdk-go-v2/service/s3/types"
	shttp "github.com/aws/smithy-go/transport/http"
	"github.com/google/uuid"
	"github.com/tigrisdata/terraform-provider-tigris/internal/types"
)

const (
	// DefaultEndpoint is the default endpoint for Tigris object storage service.
	DefaultEndpoint = "https://fly.storage.tigris.dev"

	// DefaultIAMEndpoint is the default endpoint for Tigris IAM APIs.
	DefaultIAMEndpoint = "https://iam.storageapi.dev"

	// DefaultRegion is the default region for Tigris object storage service.
	DefaultRegion = "auto"

	// Tigris IAM uses the same SigV4 service scope as object storage. The official
	// JS SDK signs requests to iam.storageapi.dev with service "s3", so this client
	// intentionally mirrors that behavior for compatibility.
	sigV4ServiceS3 = "s3"

	// Headers for the requests to Tigris.
	HeaderContentType          = "Content-Type"
	HeaderAccept               = "Accept"
	HeaderAmzContentSha        = "X-Amz-Content-Sha256"
	HeaderAmzIdentityId        = "S3-Identity-Id"
	HeaderAmzAcl               = "X-Amz-Acl"
	HeaderAmzPublicListObjects = "X-Amz-Acl-Public-List-Objects-Enabled"

	// Tigris-specific headers for bucket configuration.
	HeaderAmzStorageClass          = "X-Amz-Storage-Class"
	HeaderTigrisRegions            = "X-Tigris-Regions"
	HeaderTigrisEnableSnapshot     = "X-Tigris-Enable-Snapshot"
	HeaderTigrisForkSourceBucket   = "X-Tigris-Fork-Source-Bucket"
	HeaderTigrisForkSourceSnapshot = "X-Tigris-Fork-Source-Bucket-Snapshot"
	HeaderTigrisSnapshot           = "X-Tigris-Snapshot"
	HeaderTigrisSnapshotVersion    = "X-Tigris-Snapshot-Version"
)

type Client struct {
	cfg            aws.Config
	signer         *v4.Signer
	credentials    aws.Credentials
	endpoint       string
	iamEndpoint    string
	httpClient     *http.Client
	s3Client       *s3.Client
	retryBaseDelay time.Duration // initial backoff delay; 0 uses default (3s)
}

func NewClient(endpoint, iamEndpoint, accessKeyID, secretAccessKey string) (*Client, error) {
	// Load AWS configuration
	cfg, err := config.LoadDefaultConfig(context.TODO(),
		config.WithRegion(DefaultRegion),
		config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(accessKeyID, secretAccessKey, "")),
	)
	if err != nil {
		return nil, err
	}

	// Create a signer
	signer := v4.NewSigner()

	// Create S3 service client
	svc := s3.NewFromConfig(cfg, func(o *s3.Options) {
		o.BaseEndpoint = aws.String(endpoint)
		o.Region = DefaultRegion
	})

	return &Client{
		cfg:    cfg,
		signer: signer,
		credentials: aws.Credentials{
			AccessKeyID:     accessKeyID,
			SecretAccessKey: secretAccessKey,
		},
		endpoint:    endpoint,
		iamEndpoint: iamEndpoint,
		httpClient:  &http.Client{},
		s3Client:    svc,
	}, nil
}

func (c *Client) CreatePolicy(ctx context.Context, input *types.PolicyCreateInput) (*types.Policy, error) {
	form := url.Values{}
	form.Set("PolicyName", input.Name)
	form.Set("Description", input.Description)
	form.Set("ReqUUID", uuid.NewString())

	document, err := json.Marshal(input.Document)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal policy document: %w", err)
	}
	form.Set("PolicyDocument", string(document))

	var resp types.CreatePolicyResponse
	if err := c.doIAMRequest(ctx, "/?Action=CreatePolicy", form, &resp); err != nil {
		return nil, err
	}

	if resp.CreatePolicyResult.Policy.ARN == "" {
		return nil, errors.New("failed to create policy")
	}

	return &resp.CreatePolicyResult.Policy, nil
}

func (c *Client) GetPolicy(ctx context.Context, arn string) (*types.PolicyDetailed, error) {
	form := url.Values{}
	form.Set("PolicyArn", arn)

	var resp types.GetPolicyResponse
	if err := c.doIAMRequest(ctx, "/?Action=GetPolicyDetailed", form, &resp); err != nil {
		return nil, err
	}

	if resp.PolicyDetailed.ARN == "" {
		return nil, &iamAPIError{
			statusCode: http.StatusNotFound,
			message:    "policy not found",
		}
	}

	var document types.PolicyDocument
	if err := json.Unmarshal([]byte(resp.PolicyDetailed.Document), &document); err != nil {
		return nil, fmt.Errorf("failed to parse policy document: %w", err)
	}

	return &types.PolicyDetailed{
		Policy: types.Policy{
			AttachmentCount: resp.PolicyDetailed.AttachmentCount,
			CreateDate:      resp.PolicyDetailed.CreateDate,
			DefaultVersion:  resp.PolicyDetailed.DefaultVersion,
			Description:     resp.PolicyDetailed.Description,
			ID:              resp.PolicyDetailed.ID,
			Name:            resp.PolicyDetailed.Name,
			Path:            resp.PolicyDetailed.Path,
			ARN:             resp.PolicyDetailed.ARN,
			UpdateDate:      resp.PolicyDetailed.UpdateDate,
		},
		Document: document,
		Users:    resp.PolicyDetailed.Users,
	}, nil
}

func (c *Client) UpdatePolicy(ctx context.Context, input *types.PolicyUpdateInput) (*types.Policy, error) {
	form := url.Values{}
	form.Set("PolicyArn", input.ARN)
	form.Set("ReqUUID", uuid.NewString())
	form.Set("Description", input.Description)

	document, err := json.Marshal(input.Document)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal policy document: %w", err)
	}
	form.Set("PolicyDocument", string(document))

	var resp types.UpdatePolicyResponse
	if err := c.doIAMRequest(ctx, "/?Action=UpdatePolicy", form, &resp); err != nil {
		return nil, err
	}

	if resp.UpdatePolicyResult.Policy.ARN == "" {
		return nil, errors.New("failed to update policy")
	}

	return &resp.UpdatePolicyResult.Policy, nil
}

func (c *Client) DeletePolicy(ctx context.Context, arn string) error {
	form := url.Values{}
	form.Set("PolicyArn", arn)

	return c.doIAMRequest(ctx, "/?Action=ForceDeletePolicy", form, nil)
}

func (c *Client) CreateAccessKey(ctx context.Context, input *types.AccessKeyCreateInput) (*types.AccessKey, error) {
	form := url.Values{}
	roles := input.Roles
	if roles == nil {
		roles = []types.BucketRole{}
	}

	reqBody, err := json.Marshal(map[string]interface{}{
		"req_uuid":     uuid.NewString(),
		"name":         input.Name,
		"buckets_role": roles,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to marshal access key request: %w", err)
	}
	form.Set("Req", string(reqBody))

	var resp types.CreateAccessKeyResponse
	if err := c.doIAMRequest(ctx, "/?Action=CreateAccessKeyWithBucketsRole", form, &resp); err != nil {
		return nil, err
	}

	if resp.CreateAccessKeyResult.AccessKey.AccessKeyID == "" {
		return nil, errors.New("unable to create access key")
	}

	return &types.AccessKey{
		ID:        resp.CreateAccessKeyResult.AccessKey.AccessKeyID,
		Name:      resp.CreateAccessKeyResult.AccessKey.UserName,
		Secret:    resp.CreateAccessKeyResult.AccessKey.SecretAccessKey,
		CreatedAt: resp.CreateAccessKeyResult.AccessKey.CreateDate,
		Status:    "active",
		Roles:     roles,
	}, nil
}

func (c *Client) GetAccessKey(ctx context.Context, id string) (*types.AccessKey, error) {
	form := url.Values{}
	form.Set("Action", "ListAccessKeys")
	form.Set("KeyId", id)

	var resp types.ListAccessKeysResponse
	if err := c.doIAMRequest(ctx, "/?Detailed", form, &resp); err != nil {
		return nil, err
	}

	if len(resp.Keys) == 0 {
		return nil, &iamAPIError{
			statusCode: http.StatusNotFound,
			message:    "access key not found",
		}
	}

	key := resp.Keys[0]
	return &types.AccessKey{
		ID:             key.AccessKeyID,
		Name:           key.UserName,
		CreatedAt:      key.CreatedAt.String(),
		Status:         key.Status,
		OrganizationID: key.NamespaceID,
		Roles:          key.BucketsRole,
	}, nil
}

func (c *Client) UpdateAccessKeyRoles(ctx context.Context, input *types.AccessKeyUpdateInput) error {
	roles := input.Roles
	if roles == nil {
		roles = []types.BucketRole{}
	}

	reqBody, err := json.Marshal(map[string]interface{}{
		"id":           input.ID,
		"buckets_role": roles,
	})
	if err != nil {
		return fmt.Errorf("failed to marshal access key role update: %w", err)
	}

	form := url.Values{}
	form.Set("Req", string(reqBody))

	var resp types.UpdateAccessKeyRolesResponse
	if err := c.doIAMRequest(ctx, "/?Action=UpdateAccessKeyWithBucketsRole", form, &resp); err != nil {
		return err
	}

	if resp.Status != "" && resp.Status != "success" {
		if resp.Message != "" {
			return errors.New(resp.Message)
		}
		return errors.New("failed to update access key roles")
	}

	return nil
}

func (c *Client) DeleteAccessKey(ctx context.Context, id, name string) error {
	form := url.Values{}
	form.Set("AccessKeyId", id)
	if name != "" {
		form.Set("UserName", name)
	}

	return c.doIAMRequest(ctx, "/?Action=DeleteAccessKey", form, nil)
}

func (c *Client) CreateBucket(ctx context.Context, input *types.BucketUpdateInput) error {
	if err := validateBucketRequest(input); err != nil {
		return err
	}

	var opts []func(*s3.Options)

	// Add storage class header for default tier.
	if input.DefaultStorageTier != nil {
		opts = append(opts, withHeader(HeaderAmzStorageClass, string(*input.DefaultStorageTier)))
	}

	// Add regions header (only when not global).
	if len(input.LocationRegions) > 0 {
		opts = append(opts, withHeader(HeaderTigrisRegions, strings.Join(input.LocationRegions, ",")))
	}

	// Add snapshot enable header.
	if input.EnableSnapshot != nil && *input.EnableSnapshot {
		opts = append(opts, withHeader(HeaderTigrisEnableSnapshot, "true"))
	}

	// Add fork source bucket header.
	if input.ForkSourceBucket != nil {
		opts = append(opts, withHeader(HeaderTigrisForkSourceBucket, *input.ForkSourceBucket))
	}

	// Add fork source snapshot header.
	if input.ForkSourceSnapshot != nil {
		opts = append(opts, withHeader(HeaderTigrisForkSourceSnapshot, *input.ForkSourceSnapshot))
	}

	_, err := c.s3Client.CreateBucket(ctx, &s3.CreateBucketInput{
		Bucket: aws.String(input.Bucket),
	}, opts...)

	return err
}

func (c *Client) UpdateBucket(ctx context.Context, input *types.BucketUpdateInput) error {
	if err := validateBucketRequest(input); err != nil {
		return err
	}

	// Set all the bucket attributes that need to be updated
	upReq := &types.BucketUpdateRequest{}

	// Set the website configuration if it's provided
	if input.Website != nil {
		upReq.Website = input.Website
	}

	// Set the shadow bucket configuration if it's provided
	if input.Shadow != nil {
		upReq.Shadow = input.Shadow
	}

	// Set the object regions if provided (for location updates).
	if input.LocationRegions != nil {
		regions := strings.Join(input.LocationRegions, ",")
		upReq.ObjectRegions = &regions
	}

	body, err := json.Marshal(upReq)
	if err != nil {
		return fmt.Errorf("failed to marshal update request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPatch, c.bucketURL(input.Bucket, nil), bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("failed to create update request: %w", err)
	}

	// Update bucket attributes that need to be updated via headers
	// Update the ACL if it's provided
	if input.ACL != nil {
		req.Header.Set(HeaderAmzAcl, string(*input.ACL))
	}
	if input.PublicObjectsListEnabled != nil {
		req.Header.Set(HeaderAmzPublicListObjects, fmt.Sprintf("%t", *input.PublicObjectsListEnabled))
	}

	//nolint:contextcheck
	resp, err := c.doRequestWithRetry(req)
	if err != nil {
		return fmt.Errorf("failed to send update request: %w", err)
	}
	defer resp.Body.Close()

	var upResp types.BucketUpdateResponse
	err = json.NewDecoder(resp.Body).Decode(&upResp)
	if err != nil {
		return fmt.Errorf("request failed with code: %d", resp.StatusCode)
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("update failed with error: %s", upResp.ErrorMessage)
	}

	return nil
}

func (c *Client) HeadBucket(ctx context.Context, bucketName string) (bool, error) {
	_, err := c.s3Client.HeadBucket(ctx, &s3.HeadBucketInput{
		Bucket: aws.String(bucketName),
	}, withHeader(HeaderAmzIdentityId, c.credentials.AccessKeyID))

	exists := true
	if err != nil {
		var notFoundErr *s3types.NotFound
		if ok := errors.As(err, &notFoundErr); ok {
			exists = false
			return exists, nil
		}
	}

	return exists, err
}

func (c *Client) DeleteBucket(ctx context.Context, bucketName string) error {
	_, err := c.s3Client.DeleteBucket(ctx, &s3.DeleteBucketInput{
		Bucket: aws.String(bucketName),
	})

	return err
}

func (c *Client) GetBucketMetadata(ctx context.Context, bucketName string) (*types.BucketMetadata, error) {
	params := map[string]string{
		"metadata": "",
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.bucketURL(bucketName, params), nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	//nolint:contextcheck
	resp, err := c.doRequestWithRetry(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("request failed with code: %d", resp.StatusCode)
	}

	// Parse the response body into a BucketMetadata struct
	var metadata types.BucketMetadata
	err = json.NewDecoder(resp.Body).Decode(&metadata)
	if err != nil {
		return nil, fmt.Errorf("failed to read bucket metadata: %w", err)
	}

	return &metadata, nil
}

func (c *Client) CreateSnapshot(ctx context.Context, sourceBucket, snapshotName string) (string, error) {
	// Build the header value for snapshot creation.
	headerValue := "true"
	if snapshotName != "" {
		headerValue = fmt.Sprintf("true; name=%s", snapshotName)
	}

	// Use raw HTTP request to capture the x-tigris-snapshot-version response header.
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, c.bucketURL(sourceBucket, nil), nil)
	if err != nil {
		return "", fmt.Errorf("failed to create snapshot request: %w", err)
	}

	req.Header.Set(HeaderTigrisSnapshot, headerValue)

	//nolint:contextcheck
	resp, err := c.doRequestWithRetry(req)
	if err != nil {
		return "", fmt.Errorf("failed to create snapshot: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return "", fmt.Errorf("snapshot creation failed with code: %d", resp.StatusCode)
	}

	version := resp.Header.Get(HeaderTigrisSnapshotVersion)
	if version == "" {
		return "", fmt.Errorf("snapshot version not returned in response headers")
	}

	return version, nil
}

func (c *Client) ListSnapshots(ctx context.Context, sourceBucket string) ([]types.SnapshotInfo, error) {
	// Use S3 ListBuckets with X-Tigris-Snapshot header set to the bucket name.
	output, err := c.s3Client.ListBuckets(ctx, &s3.ListBucketsInput{},
		withHeader(HeaderTigrisSnapshot, sourceBucket),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to list snapshots: %w", err)
	}

	snapshots := make([]types.SnapshotInfo, 0, len(output.Buckets))
	for _, b := range output.Buckets {
		// The Name field from ListBuckets has the format: "{version}; name={snapshot_name}"
		version, name := parseSnapshotBucketName(aws.ToString(b.Name))
		info := types.SnapshotInfo{
			Version: version,
			Name:    name,
		}
		if b.CreationDate != nil {
			info.CreatedAt = b.CreationDate.Format(time.RFC3339)
		}
		snapshots = append(snapshots, info)
	}

	return snapshots, nil
}

// parseSnapshotBucketName parses the snapshot Name field from the ListBuckets API.
// The format is "{version}; name={snapshot_name}". If the format doesn't match,
// the entire string is returned as the version with an empty name.
func parseSnapshotBucketName(raw string) (version, name string) {
	const sep = "; name="
	idx := strings.Index(raw, sep)
	if idx < 0 {
		return raw, ""
	}
	return raw[:idx], raw[idx+len(sep):]
}

func (c *Client) GetSnapshotByName(ctx context.Context, sourceBucket, name string) (*types.SnapshotInfo, error) {
	snapshots, err := c.ListSnapshots(ctx, sourceBucket)
	if err != nil {
		return nil, err
	}

	for _, s := range snapshots {
		if s.Name == name {
			return &s, nil
		}
	}

	return nil, fmt.Errorf("snapshot %q not found for bucket %q", name, sourceBucket)
}

func (c *Client) doRequestWithRetry(req *http.Request) (*http.Response, error) {
	maxRetries := 5
	backoffDelay := c.retryBaseDelay
	if backoffDelay == 0 {
		backoffDelay = 3 * time.Second
	}
	maxBackoffDelay := 20 * backoffDelay

	var lastStatusCode int

	for i := 0; i < maxRetries; i++ {
		// Clone the request to avoid issues with mutated request objects
		clonedReq, err := cloneRequest(req)
		if err != nil {
			return nil, fmt.Errorf("failed to clone request: %w", err)
		}

		resp, err := c.doSignedRequest(clonedReq)
		if err != nil {
			return nil, fmt.Errorf("failed to send request: %w", err)
		}

		// Check if the response status code indicates a server-side error (5xx)
		if resp.StatusCode >= 500 {
			lastStatusCode = resp.StatusCode
			resp.Body.Close()

			if i == maxRetries-1 {
				break
			}

			// Exponential backoff before retrying, respecting context cancellation
			timer := time.NewTimer(backoffDelay)
			select {
			case <-req.Context().Done():
				timer.Stop()
				return nil, req.Context().Err()
			case <-timer.C:
			}
			backoffDelay *= 2 // Double the delay for each retry
			if backoffDelay > maxBackoffDelay {
				backoffDelay = maxBackoffDelay
			}

			continue
		}

		return resp, nil
	}

	return nil, fmt.Errorf("request failed after %d retries with status %d", maxRetries, lastStatusCode)
}

func (c *Client) doSignedRequest(req *http.Request) (*http.Response, error) {
	// Sign the request
	err := c.signRequest(req)
	if err != nil {
		return nil, fmt.Errorf("failed to sign request: %w", err)
	}

	// Send the signed request using the wrapped http.Client
	return c.httpClient.Do(req)
}

func (c *Client) bucketURL(bucketName string, queryParams map[string]string) string {
	baseURL := fmt.Sprintf("%s/%s", c.endpoint, bucketName)
	if len(queryParams) == 0 {
		return baseURL
	}

	// Add query parameters to the URL
	query := url.Values{}
	for key, value := range queryParams {
		query.Add(key, value)
	}

	return fmt.Sprintf("%s?%s", baseURL, query.Encode())
}

func (c *Client) iamURL(path string) string {
	if strings.HasPrefix(path, "/") {
		return c.iamEndpoint + path
	}

	return c.iamEndpoint + "/" + path
}

func (c *Client) doIAMRequest(ctx context.Context, path string, form url.Values, out interface{}) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.iamURL(path), strings.NewReader(form.Encode()))
	if err != nil {
		return fmt.Errorf("failed to create IAM request: %w", err)
	}

	req.Header.Set(HeaderContentType, "application/x-www-form-urlencoded")
	req.Header.Set(HeaderAccept, "application/json")

	//nolint:contextcheck
	resp, err := c.doRequestWithRetry(req)
	if err != nil {
		return fmt.Errorf("failed to send IAM request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read IAM response: %w", err)
	}

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return parseIAMError(resp.StatusCode, body)
	}

	if out == nil || len(body) == 0 {
		return nil
	}

	if err := json.Unmarshal(body, out); err != nil {
		return fmt.Errorf("failed to decode IAM response: %w", err)
	}

	return nil
}

func (c *Client) signRequest(req *http.Request) error {
	// Get the current time for the request
	now := time.Now()

	// Set default headers
	if req.Header.Get(HeaderContentType) == "" {
		req.Header.Set(HeaderContentType, "application/json")
	}
	if req.Header.Get(HeaderAccept) == "" {
		req.Header.Set(HeaderAccept, "application/json")
	}

	// Buffer the request body if it exists
	var bodyBytes []byte
	var payloadHash string
	if req.Body != nil {
		var err error
		bodyBytes, err = io.ReadAll(req.Body)
		if err != nil {
			return fmt.Errorf("failed to read request body: %w", err)
		}
		req.Body = io.NopCloser(bytes.NewReader(bodyBytes))

		// Calculate the payload hash
		hash := sha256.New()
		hash.Write(bodyBytes)
		payloadHash = hex.EncodeToString(hash.Sum(nil))
	} else {
		// If there's no body, the hash should be the SHA-256 of an empty string
		payloadHash = hex.EncodeToString(sha256.New().Sum(nil))
	}

	// set the content sha256 header
	req.Header.Set(HeaderAmzContentSha, payloadHash)

	// Sign the request using the signer
	err := c.signer.SignHTTP(req.Context(), c.credentials, req, payloadHash, sigV4ServiceS3, DefaultRegion, now)
	if err != nil {
		return fmt.Errorf("failed to sign request: %w", err)
	}

	return nil
}

func cloneRequest(req *http.Request) (*http.Request, error) {
	// Create a shallow copy of the request
	clonedReq := req.Clone(req.Context())

	// Clone the body if it exists and is seekable
	if req.Body != nil {
		var buf bytes.Buffer
		_, err := buf.ReadFrom(req.Body)
		if err != nil {
			return nil, fmt.Errorf("failed to read request body: %w", err)
		}

		// Restore the original body to be read again
		req.Body = io.NopCloser(bytes.NewReader(buf.Bytes()))
		// Set the cloned request's body
		clonedReq.Body = io.NopCloser(bytes.NewReader(buf.Bytes()))
	}

	return clonedReq, nil
}

type iamAPIError struct {
	statusCode int
	message    string
}

func (e *iamAPIError) Error() string {
	if e.message == "" {
		return fmt.Sprintf("iam request failed with status %d", e.statusCode)
	}

	return fmt.Sprintf("iam request failed with status %d: %s", e.statusCode, e.message)
}

func (e *iamAPIError) NotFound() bool {
	return e.statusCode == http.StatusNotFound
}

func IsIAMNotFoundError(err error) bool {
	var apiErr *iamAPIError
	if errors.As(err, &apiErr) {
		return apiErr.NotFound()
	}

	return false
}

func parseIAMError(statusCode int, body []byte) error {
	var payload struct {
		Message string `json:"Message"`
	}
	if err := json.Unmarshal(body, &payload); err == nil && payload.Message != "" {
		return &iamAPIError{
			statusCode: statusCode,
			message:    payload.Message,
		}
	}

	return &iamAPIError{
		statusCode: statusCode,
		message:    strings.TrimSpace(string(body)),
	}
}

func validateBucketRequest(input *types.BucketUpdateInput) error {
	if input.Bucket == "" {
		return errors.New("bucket name is required")
	}

	return nil
}

func withHeader(key, value string) func(*s3.Options) {
	return func(options *s3.Options) {
		options.APIOptions = append(options.APIOptions, shttp.AddHeaderValue(key, value))
	}
}
