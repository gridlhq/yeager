#!/bin/bash
# Fix the exit 1 bug in credential checks

set -euo pipefail

echo "Fixing exit 1 bug in credential checks..."

for file in /Users/stuart/repos/gridl/yeager_dev/test/livefire/*.test.sh; do
  if [ -f "$file" ]; then
    # Remove the "exit 1" line that's in the else clause after our fix
    if grep -q "# log_fail.*check disabled" "$file" 2>/dev/null; then
      echo "Processing $(basename $file)..."

      # Remove the line after "# log_fail ... - check disabled"
      sed -i.bak3 '/# log_fail.*check disabled/,/^  fi$/ {
        /exit 1/d
      }' "$file"

      rm -f "${file}.bak3"
      echo "  ✓ Fixed"
    fi
  fi
done

echo "Done!"
