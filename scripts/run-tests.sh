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
  bucket-lifecycle
  bucket-delete-protection
)

# Suites that must be idempotent: a second plan after apply has to report no
# changes. This catches perpetual diffs (for example, lifecycle rule ordering).
IDEMPOTENT_SUITES=(
  bucket-lifecycle
)
FAILED_SUITES=()

is_idempotent_suite() {
  local s
  for s in "${IDEMPOTENT_SUITES[@]}"; do
    [ "$s" = "$1" ] && return 0
  done
  return 1
}

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

  # Apply
  if terraform apply -auto-approve -input=false -var "test_id=${TEST_ID}"; then
    echo "==> PASS: ${suite}"

    # For idempotent suites, a second plan must report no changes.
    if is_idempotent_suite "$suite"; then
      if terraform plan -detailed-exitcode -input=false -var "test_id=${TEST_ID}" > /dev/null 2>&1; then
        echo "==> PASS: ${suite} (idempotent)"
      else
        FAILED_SUITES+=("$suite")
        echo "==> FAIL: ${suite} (second plan reported changes)"
      fi
    fi
  else
    FAILED_SUITES+=("$suite")
    echo "==> FAIL: ${suite}"
  fi

  # Optional per-suite hook, run before the shared destroy below. A suite whose
  # resources cannot be destroyed in the state the test leaves them in (for
  # example, a bucket with deletion protection enabled) uses this to make them
  # destroyable, and to assert any behaviour that only the pre-destroy state can
  # show. Suite-specific knowledge stays in the suite directory.
  if [ -x "${suite_dir}/pre-destroy.sh" ]; then
    if TEST_ID="${TEST_ID}" "${suite_dir}/pre-destroy.sh"; then
      echo "==> PASS: ${suite} (pre-destroy)"
    else
      FAILED_SUITES+=("$suite")
      echo "==> FAIL: ${suite} (pre-destroy)"
    fi
  fi

  # Always destroy to clean up resources
  terraform destroy -auto-approve -input=false -var "test_id=${TEST_ID}" 2>/dev/null || true

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
