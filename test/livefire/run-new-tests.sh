#!/bin/bash
# Run newly implemented LiveFire tests for config behavior and artifacts
#
# Prerequisites:
# - Built binary: make build
# - AWS credentials configured
# - ~125 minutes and $1.70 AWS budget

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"

echo "===================================="
echo "LiveFire Tests - Config & Artifacts"
echo "===================================="
echo ""
echo "Location: $PROJECT_ROOT/test/livefire"
echo "Test Count: 17 scenarios"
echo "Estimated Duration: ~125 minutes"
echo "Estimated Cost: ~$1.70"
echo ""

# Check if binary exists
if [ ! -f "$PROJECT_ROOT/yg" ]; then
    echo "ERROR: yg binary not found at $PROJECT_ROOT/yg"
    echo "Run 'make build' first"
    exit 1
fi

# Check AWS credentials
if [ -z "${AWS_ACCESS_KEY_ID:-}" ] && [ -z "${AWS_PROFILE:-}" ]; then
    echo "ERROR: AWS credentials not configured"
    echo "Set AWS_ACCESS_KEY_ID/AWS_SECRET_ACCESS_KEY or AWS_PROFILE"
    exit 1
fi

echo "Prerequisites:"
echo "  ✓ Binary found at $PROJECT_ROOT/yg"
echo "  ✓ AWS credentials configured"
echo ""

# Function to run tests with tag
run_tests() {
    local tag=$1
    local description=$2
    local timeout=$3

    echo "===================================="
    echo "Running: $description"
    echo "Tag: @$tag"
    echo "===================================="
    echo ""

    cd "$PROJECT_ROOT/test/livefire"

    LIVEFIRE_TAGS="@$tag" go test -tags livefire -v -count=1 -timeout "$timeout" . 2>&1 | \
        tee "livefire-${tag}-$(date +%Y%m%d-%H%M%S).log"

    local exit_code=${PIPESTATUS[0]}

    if [ $exit_code -eq 0 ]; then
        echo ""
        echo "✓ $description - PASSED"
        echo ""
    else
        echo ""
        echo "✗ $description - FAILED (exit code $exit_code)"
        echo ""
        return $exit_code
    fi
}

# Parse command line arguments
if [ $# -eq 0 ]; then
    echo "Usage: $0 [all|config|artifacts|scenario-name]"
    echo ""
    echo "Options:"
    echo "  all          - Run all config and artifact tests (~125 min, $1.70)"
    echo "  config       - Run config behavior tests only (~90 min, $1.20)"
    echo "  artifacts    - Run artifact tests only (~35 min, $0.50)"
    echo ""
    echo "Examples:"
    echo "  $0 all"
    echo "  $0 config"
    echo "  $0 artifacts"
    echo ""
    exit 0
fi

case "${1:-}" in
    all)
        echo "Running ALL new tests (config + artifacts)..."
        echo ""
        run_tests "config" "Config Behavior Tests (12 scenarios)" "100m"
        run_tests "artifacts" "Artifact Tests (5 scenarios)" "45m"
        echo ""
        echo "===================================="
        echo "All tests completed successfully!"
        echo "===================================="
        ;;

    config)
        run_tests "config" "Config Behavior Tests (12 scenarios)" "100m"
        ;;

    artifacts)
        run_tests "artifacts" "Artifact Tests (5 scenarios)" "45m"
        ;;

    *)
        echo "ERROR: Unknown option: $1"
        echo "Run '$0' without arguments to see usage"
        exit 1
        ;;
esac

echo ""
echo "Logs saved to: $PROJECT_ROOT/test/livefire/livefire-*.log"
echo ""
