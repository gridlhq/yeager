#!/bin/bash
# Fix test syntax: replace "yg run <cmd>" with "yg <cmd>"
# The Yeager CLI uses "yg <command>" not "yg run <command>"

set -euo pipefail

echo "Fixing test syntax in LiveFire tests..."

# Find all .sh files in livefire directory
for file in test/livefire/*.test.sh; do
  if [ -f "$file" ]; then
    echo "Processing $file..."

    # Replace "yg run " with "yg " (with space after)
    # Also handle variations like "$YG run", "yg run", etc.
    sed -i.bak \
      -e 's/yg run /yg /g' \
      -e 's/\$YG run /\$YG /g' \
      -e 's/"yg run /"yg /g' \
      -e "s/'yg run /'yg /g" \
      "$file"

    # Remove backup file
    rm -f "${file}.bak"

    echo "  ✓ Fixed"
  fi
done

echo ""
echo "Done! All test files updated."
echo ""
echo "Summary of changes:"
grep -r "yg run" test/livefire/*.test.sh 2>/dev/null | wc -l || echo "0 instances of 'yg run' remaining"
