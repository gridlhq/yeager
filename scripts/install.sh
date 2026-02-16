#!/bin/sh
# yeager install script
# Usage: curl -fsSL https://yeager.sh/install | sh
#
# Environment variables:
#   YEAGER_INSTALL_DIR  — install directory (default: /usr/local/bin)
#   YEAGER_VERSION      — version to install (default: latest)
#   YEAGER_PRERELEASE   — set to 1 to install latest pre-release (used by staging.yeager.sh)

set -e

GITHUB_REPO="gridlhq/yeager"
BINARY_NAME="yg"
DEFAULT_INSTALL_DIR="/usr/local/bin"

# ── helpers ──────────────────────────────────────────────────────

log() {
    printf "yeager | %s\n" "$1"
}

err() {
    printf "yeager | error: %s\n" "$1" >&2
    exit 1
}

# ── detect platform ──────────────────────────────────────────────

detect_os() {
    case "$(uname -s)" in
        Linux*)  echo "linux" ;;
        Darwin*) echo "darwin" ;;
        *)       err "unsupported OS: $(uname -s). yeager supports Linux and macOS." ;;
    esac
}

detect_arch() {
    case "$(uname -m)" in
        x86_64|amd64)   echo "amd64" ;;
        aarch64|arm64)  echo "arm64" ;;
        *)              err "unsupported architecture: $(uname -m). yeager supports x86_64 and arm64." ;;
    esac
}

# ── fetch tools ──────────────────────────────────────────────────

has_cmd() {
    command -v "$1" >/dev/null 2>&1
}

download() {
    url="$1"
    output="$2"
    if has_cmd curl; then
        curl -fsSL -o "$output" "$url"
    elif has_cmd wget; then
        wget -qO "$output" "$url"
    else
        err "curl or wget required but neither found"
    fi
}

download_text() {
    url="$1"
    if has_cmd curl; then
        curl -fsSL "$url"
    elif has_cmd wget; then
        wget -qO- "$url"
    fi
}

# ── resolve version ─────────────────────────────────────────────

resolve_version() {
    if [ -n "${YEAGER_VERSION:-}" ]; then
        echo "$YEAGER_VERSION"
        return
    fi

    # YEAGER_PRERELEASE=1 fetches from all releases (including pre-releases).
    # Default fetches from /releases/latest (stable only).
    if [ "${YEAGER_PRERELEASE:-}" = "1" ]; then
        api_url="https://api.github.com/repos/${GITHUB_REPO}/releases"
    else
        api_url="https://api.github.com/repos/${GITHUB_REPO}/releases/latest"
    fi

    latest=$(download_text "$api_url" 2>/dev/null \
        | grep '"tag_name"' | head -1 | sed 's/.*"tag_name": *"//;s/".*//')

    if [ -z "$latest" ]; then
        err "could not determine latest version. Set YEAGER_VERSION explicitly."
    fi

    # Strip leading 'v' if present.
    echo "$latest" | sed 's/^v//'
}

# ── verify checksum ─────────────────────────────────────────────

verify_checksum() {
    archive_path="$1"
    expected_name="$2"
    checksums_url="$3"

    checksums=$(download_text "$checksums_url" 2>/dev/null) || return 0

    expected=$(echo "$checksums" | grep "$expected_name" | awk '{print $1}')
    if [ -z "$expected" ]; then
        log "warning: checksum not found for $expected_name, skipping verification"
        return 0
    fi

    if has_cmd sha256sum; then
        actual=$(sha256sum "$archive_path" | awk '{print $1}')
    elif has_cmd shasum; then
        actual=$(shasum -a 256 "$archive_path" | awk '{print $1}')
    else
        log "warning: sha256sum/shasum not found, skipping checksum verification"
        return 0
    fi

    if [ "$actual" != "$expected" ]; then
        err "checksum mismatch for $expected_name (expected $expected, got $actual)"
    fi

    log "checksum verified"
}

# ── main ─────────────────────────────────────────────────────────

main() {
    os=$(detect_os)
    arch=$(detect_arch)
    version=$(resolve_version)
    install_dir="${YEAGER_INSTALL_DIR:-$DEFAULT_INSTALL_DIR}"

    log "installing yeager v${version} (${os}/${arch})"

    archive_name="yeager_${version}_${os}_${arch}.tar.gz"
    base_url="https://github.com/${GITHUB_REPO}/releases/download/v${version}"
    archive_url="${base_url}/${archive_name}"
    checksums_url="${base_url}/checksums.txt"

    # Create temp directory.
    tmp_dir=$(mktemp -d)
    trap 'rm -rf "$tmp_dir"' EXIT

    # Download archive.
    log "downloading ${archive_name}..."
    download "$archive_url" "${tmp_dir}/${archive_name}"

    # Verify checksum.
    verify_checksum "${tmp_dir}/${archive_name}" "$archive_name" "$checksums_url"

    # Extract binary.
    tar -xzf "${tmp_dir}/${archive_name}" -C "$tmp_dir" "$BINARY_NAME"

    if [ ! -f "${tmp_dir}/${BINARY_NAME}" ]; then
        err "binary not found in archive"
    fi

    # Install.
    if [ -w "$install_dir" ]; then
        mv "${tmp_dir}/${BINARY_NAME}" "${install_dir}/${BINARY_NAME}"
    else
        log "installing to ${install_dir} (requires sudo)"
        sudo mv "${tmp_dir}/${BINARY_NAME}" "${install_dir}/${BINARY_NAME}"
    fi

    chmod +x "${install_dir}/${BINARY_NAME}"

    # Verify.
    if has_cmd "$BINARY_NAME"; then
        installed_version=$("$BINARY_NAME" --version 2>/dev/null || echo "unknown")
        log "installed: ${installed_version}"
    else
        log "installed to ${install_dir}/${BINARY_NAME}"
        log "make sure ${install_dir} is in your PATH"
    fi
}

main
