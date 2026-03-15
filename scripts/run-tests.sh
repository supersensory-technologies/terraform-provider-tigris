#!/usr/bin/env bash
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
PROVIDER_BIN_DIR="${REPO_ROOT}"
TEST_ID="${TEST_ID:-t-$(date +%s | tail -c 7)}"

SUITES=(
  bucket-basic
  bucket-location
  bucket-snapshot
  bucket-fork
  bucket-update
  bucket-public-access
  bucket-delete-protection
)
FAILED_SUITES=()

# Create a temporary .terraformrc with dev_overrides
export TF_CLI_CONFIG_FILE="${REPO_ROOT}/.terraformrc-test"
cat > "$TF_CLI_CONFIG_FILE" <<EOF
provider_installation {
  dev_overrides {
    "tigrisdata/tigris" = "${PROVIDER_BIN_DIR}"
  }
  direct {}
}
EOF

cleanup() {
  rm -f "$TF_CLI_CONFIG_FILE"
}
trap cleanup EXIT

echo "==> Using TEST_ID=${TEST_ID}"
echo ""

for suite in "${SUITES[@]}"; do
  echo "==> Testing ${suite}..."
  suite_dir="${REPO_ROOT}/test/${suite}"
  cd "$suite_dir"

  # Clean up any leftover state from previous runs to avoid stale resource conflicts
  rm -rf .terraform .terraform.lock.hcl terraform.tfstate terraform.tfstate.backup

  # With dev_overrides, init still runs but skips provider download
  terraform init -input=false > /dev/null 2>&1 || true

  if [ "$suite" = "bucket-delete-protection" ]; then
    # Helper: best-effort cleanup for protected buckets
    cleanup_protected() {
      terraform apply -auto-approve -input=false \
        -var "test_id=${TEST_ID}" \
        -var "deletion_protection=false" 2>/dev/null || true
      terraform destroy -auto-approve -input=false \
        -var "test_id=${TEST_ID}" \
        -var "deletion_protection=false" 2>/dev/null || true
    }

    # Special test: verify that destroy is blocked while protection is enabled
    if ! terraform apply -auto-approve -input=false -var "test_id=${TEST_ID}"; then
      FAILED_SUITES+=("$suite")
      echo "==> FAIL: ${suite} (apply with protection enabled)"
      cleanup_protected
      echo ""
      continue
    fi

    # Destroy MUST fail while protection is on
    if terraform destroy -auto-approve -input=false -var "test_id=${TEST_ID}" 2>/dev/null; then
      FAILED_SUITES+=("$suite")
      echo "==> FAIL: ${suite} (destroy succeeded but should have been blocked)"
      echo ""
      continue
    fi
    echo "   destroy correctly blocked by deletion_protection"

    # Disable protection, then destroy — both must succeed
    if ! terraform apply -auto-approve -input=false \
      -var "test_id=${TEST_ID}" \
      -var "deletion_protection=false"; then
      FAILED_SUITES+=("$suite")
      echo "==> FAIL: ${suite} (could not disable protection)"
      cleanup_protected
      echo ""
      continue
    fi
    echo "   deletion_protection disabled"

    if ! terraform destroy -auto-approve -input=false \
      -var "test_id=${TEST_ID}" \
      -var "deletion_protection=false"; then
      FAILED_SUITES+=("$suite")
      echo "==> FAIL: ${suite} (destroy failed after disabling protection)"
      echo ""
      continue
    fi

    echo "==> PASS: ${suite}"
  else
    # Standard suite: apply then destroy
    if terraform apply -auto-approve -input=false -var "test_id=${TEST_ID}"; then
      echo "==> PASS: ${suite}"
    else
      FAILED_SUITES+=("$suite")
      echo "==> FAIL: ${suite}"
    fi

    # Always destroy to clean up resources
    terraform destroy -auto-approve -input=false -var "test_id=${TEST_ID}" 2>/dev/null || true
  fi

  echo ""
done

# Report results
echo "==> Results:"
echo "   Total:  ${#SUITES[@]}"
echo "   Failed: ${#FAILED_SUITES[@]}"

if [ ${#FAILED_SUITES[@]} -gt 0 ]; then
  echo "   Failed suites: ${FAILED_SUITES[*]}"
  exit 1
fi

echo "   All suites passed."
