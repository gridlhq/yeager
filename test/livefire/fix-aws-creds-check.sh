#!/bin/bash
# Fix AWS credentials check in tests
# Tests should work with both env vars AND ~/.aws/credentials file

set -euo pipefail

echo "Fixing AWS credentials checks in LiveFire tests..."

for file in test/livefire/*.test.sh; do
  if [ -f "$file" ]; then
    # Check if file has AWS credential check
    if grep -q "AWS credentials not set" "$file" 2>/dev/null; then
      echo "Processing $file..."

      # Replace the credential check to be more lenient
      # Old: fails if env vars not set
      # New: checks if yg status works (credentials from any source)

      # Use perl for multi-line replacements
      perl -i.bak -0pe 's/if \[ -n "\$\{AWS_ACCESS_KEY_ID:-\}" \] && \[ -n "\$\{AWS_SECRET_ACCESS_KEY:-\}" \]; then\n    yg configure[^\n]*\n[^\n]*\n  else\n    log_fail "AWS credentials not set[^"]*"[^\n]*\n    exit 1\n  fi/# Check if AWS credentials are configured (env vars or ~\/.aws\/credentials)\n  if ! yg status > \/dev\/null 2>\&1 && ! [ -f ~\/.aws\/credentials ]; then\n    log_fail "AWS credentials not configured. Run: yg configure"\n    exit 1\n  fi/g' "$file"

      rm -f "${file}.bak"
      echo "  ✓ Fixed"
    fi
  fi
done

echo ""
echo "Done! All credential checks updated."
