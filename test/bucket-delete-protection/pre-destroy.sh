#!/usr/bin/env bash
#
# Pre-destroy hook for the bucket-delete-protection suite.
#
# The suite deliberately leaves a bucket with delete_protection enabled, which
# the shared destroy in scripts/run-tests.sh cannot remove. Two things have to
# happen here, and only here, because both are specific to this suite:
#
#   1. Assert that a destroy really is blocked while protection is on. This is
#      the behaviour under test and it is only observable before cleanup.
#   2. Disable protection, so the shared destroy can remove the bucket.
#
set -euo pipefail

cd "$(dirname "$0")"

: "${TEST_ID:?TEST_ID must be set by scripts/run-tests.sh}"

bucket="${TEST_ID}-protected-bucket"

# A destroy MUST fail while delete protection is enabled. Note the inverted
# sense: success here is the failure case.
if terraform destroy -auto-approve -input=false \
  -var "test_id=${TEST_ID}" > /dev/null 2>&1; then
  echo "   destroy succeeded but delete_protection should have blocked it" >&2
  exit 1
fi
echo "   destroy correctly blocked by delete_protection"

# Disable protection so the shared destroy can clean up.
if ! terraform apply -auto-approve -input=false \
  -var "test_id=${TEST_ID}" \
  -var "delete_protection=false"; then
  echo "   failed to disable delete_protection on ${bucket}" >&2
  echo "   NOTE: ${bucket} is still protected and will survive cleanup;" >&2
  echo "         remove it manually once protection is cleared" >&2
  exit 1
fi
echo "   delete_protection disabled"
