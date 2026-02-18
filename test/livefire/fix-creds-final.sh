#!/bin/bash
# Final fix for credential checks - replace entire if/else/fi block

set -euo pipefail

echo "Applying final credential check fix..."

for file in /Users/stuart/repos/gridl/yeager_dev/test/livefire/*.test.sh; do
  if [ -f "$file" ]; then
    echo "Processing $(basename $file)..."

    # Use a Python script for more reliable multi-line replacement
    python3 - "$file" << 'PYTHON_SCRIPT'
import sys
import re

filepath = sys.argv[1]

with open(filepath, 'r') as f:
    content = f.read()

# Replace the broken credential check with a simple comment
# Match from "# AWS credentials check disabled" through the next "fi"
pattern = r'  # AWS credentials check disabled[^\n]*\n.*?(?=\n\n  log_info "Setup complete")'
replacement = '  # AWS credentials check: Using ~/.aws/credentials (no env vars needed)'

content = re.sub(pattern, replacement, content, flags=re.DOTALL)

with open(filepath, 'w') as f:
    f.write(content)

print(f"  ✓ Fixed {filepath}")
PYTHON_SCRIPT

  done
done

echo "Done!"
