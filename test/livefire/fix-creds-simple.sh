#!/bin/bash
# Simple fix: comment out AWS credential env var checks
# Yeager will use ~/.aws/credentials automatically

set -euo pipefail

echo "Commenting out AWS env var checks..."

for file in /Users/stuart/repos/gridl/yeager_dev/test/livefire/*.test.sh; do
  if [ -f "$file" ]; then
    if grep -q "AWS credentials not set" "$file" 2>/dev/null; then
      echo "Processing $(basename $file)..."

      # Comment out the env var check and exit
      sed -i.bak2 \
        -e 's/^  if \[ -n "\${AWS_ACCESS_KEY_ID:-}" \] && \[ -n "\${AWS_SECRET_ACCESS_KEY:-}" \]; then/  # AWS credentials check disabled - yg uses ~\/.aws\/credentials\n  if false; then/' \
        -e 's/^    log_fail "AWS credentials not set.*$/    # log_fail "AWS credentials not set" - check disabled/' \
        "$file"

      rm -f "${file}.bak2"
      echo "  ✓ Fixed"
    fi
  fi
done

echo "Done!"
