# Tigris Terraform Provider Expansion Plan

Manage durable Tigris infrastructure and IAM configuration needed for Terraform workflows through idiomatic Terraform resources.

This document is a planning note for my fork. The goal is to make it easy for Tigris maintainers to react early to scope, API assumptions, and missing pieces before more implementation work lands.

## Scope

The provider should cover everything in Tigris that represents durable infrastructure or policy: buckets, bucket configuration, IAM objects, and relationships between them.

Things that are not Terraform-shaped (one-shot admin commands, interactive flows, debug/inspection, temporary operational actions with no durable state) are intentionally out of scope.

## Current provider surface

Today the provider already supports:

- `tigris_bucket`
- `tigris_bucket_public_access`
- `tigris_bucket_website_config`
- `tigris_bucket_shadow_config`
- `tigris_bucket_snapshot`
- `tigris_bucket_fork`

## Proposed roadmap

### Phase 1: IAM parity

The minimum needed to manage access control without dropping to `t3`.

| Item | Why it matters | Status |
|------|----------------|--------|
| `tigris_iam_policy` | Manage reusable IAM policies | Implemented in fork |
| `tigris_access_key` | Manage access keys and bucket roles | Implemented in fork |
| `tigris_bucket.delete_protection` | Prevent accidental bucket deletion | Implemented in fork |

### Phase 2: Relationship parity

Complete the IAM story by connecting policies to keys.

| Item | Why it matters | Status |
|------|----------------|--------|
| `tigris_access_key_policy_attachment` | Attach policies to access keys | Blocked on API confirmation |

### Phase 3: Bucket configuration parity

Declarative management for all durable bucket settings exposed by `t3 buckets set-*`.

| Item | Why it matters | Status |
|------|----------------|--------|
| `tigris_bucket_ttl` | Object expiration / retention | Planned |
| `tigris_bucket_transition` | Storage class lifecycle transitions | Planned |
| `tigris_bucket_notifications` | Event delivery configuration | Planned |
| `tigris_bucket_cors` | Browser access rules | Planned |
| `tigris_bucket_migration` | Migration from external S3 sources | Planned |
| Additional `tigris_bucket` attributes | Object ACL, directory listing, cache-control, custom domain, additional headers | Planned |

Phase 3 items are independent and can be split into separate PRs.

### Stretch: IAM user management

| Item | Why it matters | Status |
|------|----------------|--------|
| `tigris_iam_user_invitation` | Org membership via Terraform | Stretch goal |

Lower priority — org membership changes infrequently.

### Cross-cutting expectations (applies to all phases)

These are not a separate phase — they apply to every PR:

- Clear docs and examples for every resource
- Import works for all resources with documented import IDs
- Tests cover validation, expand/flatten, error classification, and tricky client behavior
- Stable UX: state reconciliation, sensitive field handling, and destroy behavior are predictable
- Maintainable PR slices that Tigris can merge incrementally

## Quality bar for each resource

- CRUD implemented
- Import works (where applicable) with documented import ID format
- State behavior is correct: read removes missing resources from state, sensitive fields survive refresh
- Error UX is acceptable: permission failures, not-found, and destroy-time behavior produce provider-owned messages, not raw transport noise
- Unit tests for validation, expand/flatten, and error classification
- Docs and examples generate cleanly
- Manual verification against real Tigris noted in PR description
- Non-obvious UX choices explicitly documented (especially destroy behavior)

## Evidence for Phase 1 API assumptions

The Phase 1 IAM work is based on reverse engineering from the `@tigrisdata/iam` npm package and behavior in the current Tigris APIs. The current assumptions are:

- IAM endpoint: `https://iam.storageapi.dev`
- Auth: AWS SigV4 using the same credentials as object storage
- Request format: `POST` with `application/x-www-form-urlencoded`
- Response format: JSON

| Action | Path shape | Purpose |
|--------|------------|---------|
| `ListPolicies` | `/?Action=ListPolicies` | List all policies |
| `GetPolicyDetailed` | `/?Action=GetPolicyDetailed` | Get policy plus attached users |
| `CreatePolicy` | `/?Action=CreatePolicy` | Create policy |
| `UpdatePolicy` | `/?Action=UpdatePolicy` | Update policy document |
| `ForceDeletePolicy` | `/?Action=ForceDeletePolicy` | Delete policy |
| `ListAccessKeys` | `/?Detailed` with action in form body | List keys with roles |
| `GetAccessKey` | `/?Detailed` with action and key id in form body | Get one key |
| `CreateAccessKeyWithBucketsRole` | `/?Action=CreateAccessKeyWithBucketsRole` | Create key with bucket roles |
| `UpdateAccessKeyWithBucketsRole` | `/?Action=UpdateAccessKeyWithBucketsRole` | Update key roles |
| `DeleteAccessKey` | `/?Action=DeleteAccessKey` | Delete key |

## Feedback requested from Tigris

Before investing further, maintainer input would help on:

1. Is Phase 1 the right Terraform surface for IAM, especially `tigris_iam_policy` and `tigris_access_key`?
2. Is there a supported API for policy-to-access-key attachment, or is the right model bucket roles plus inline policy management?
3. For bucket delete protection, should `terraform destroy` auto-disable protection first, or should the provider require an explicit update to `false` before deletion?
4. Are there existing or planned APIs for Phase 3 items that would change how these resources should be modeled?
5. If this work is useful, would you prefer one feature PR at a time, a draft PR for discussion, or issue-level design feedback first?

## Working approach

- Keep each resource or behavior in a small reviewable PR
- Preserve existing file layout and conventions
- Where new code diverges from existing patterns (e.g. IAM error typing), explain why rather than backporting changes to unrelated code
- Add tests before proposing upstream merges
- Avoid locking in API assumptions that Tigris considers internal or unstable
- Adjust the resource model if Tigris has a better supported shape in mind
