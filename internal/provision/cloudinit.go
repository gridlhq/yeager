package provision

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/gridlhq/yeager/internal/config"
)

// basePackages are always installed on every VM.
var basePackages = []string{
	"build-essential",
	"git",
	"rsync",
	"curl",
	"unzip",
	"jq",
	"htop",
	"tmux",
}

// CloudInit represents a generated cloud-init document.
type CloudInit struct {
	packages []string
	runcmd   []string
}

// GenerateCloudInit creates a cloud-init document for a VM.
// langs may be nil if no languages were detected.
// setup comes from the [setup] section of .yeager.toml.
//
// Cloud-init runs at first boot BEFORE project files are synced, so it only
// includes runtime installs (rustup, nvm, go) and system packages — NOT
// dependency installs (cargo fetch, npm ci) which need project files.
// Dep install and [setup] run commands are executed post-sync via SSH (Phase 4).
func GenerateCloudInit(langs []Language, setup config.SetupConfig) *CloudInit {
	ci := &CloudInit{}

	// 1. Base packages.
	ci.packages = append(ci.packages, basePackages...)

	// 2. Setup packages from .yeager.toml (apt packages are fine in cloud-init).
	ci.packages = append(ci.packages, setup.Packages...)

	// 3. Configure sshd on both port 22 and 443.
	ci.runcmd = append(ci.runcmd,
		// Ensure Port 22 is explicitly set (uncomment if commented, add if absent).
		`grep -q '^Port 22' /etc/ssh/sshd_config || sed -i 's/^#Port 22$/Port 22/' /etc/ssh/sshd_config`,
		`grep -q '^Port 22' /etc/ssh/sshd_config || echo 'Port 22' >> /etc/ssh/sshd_config`,
		`grep -q '^Port 443' /etc/ssh/sshd_config || echo 'Port 443' >> /etc/ssh/sshd_config`,
		`systemctl restart sshd`,
	)

	// 4. Per-language runtime installation (doesn't need project files).
	for _, lang := range langs {
		ci.runcmd = append(ci.runcmd, lang.RuntimeInstall...)
	}

	// 5. Create project directory for rsync target.
	ci.runcmd = append(ci.runcmd, "mkdir -p /home/ubuntu/project")

	// NOTE: Dependency installs (DepInstall) and [setup] run commands are NOT
	// included here. They require project files which aren't available until
	// after the first rsync. Phase 4 runs them post-sync via SSH.

	return ci
}

// Render returns the cloud-init document as a string.
func (ci *CloudInit) Render() string {
	var b strings.Builder

	b.WriteString("#cloud-config\n")

	// packages section.
	b.WriteString("packages:\n")
	for _, pkg := range ci.packages {
		fmt.Fprintf(&b, "  - %s\n", pkg)
	}

	// runcmd section.
	if len(ci.runcmd) > 0 {
		b.WriteString("runcmd:\n")
		for _, cmd := range ci.runcmd {
			fmt.Fprintf(&b, "  - |\n    %s\n", cmd)
		}
	}

	return b.String()
}

// SetupHash computes a stable hash of the setup config.
// Used to detect when the [setup] section has changed.
func SetupHash(setup config.SetupConfig) string {
	h := sha256.New()
	h.Write([]byte("packages:"))
	for _, p := range setup.Packages {
		h.Write([]byte(p))
		h.Write([]byte{0})
	}
	h.Write([]byte("run:"))
	for _, r := range setup.Run {
		h.Write([]byte(r))
		h.Write([]byte{0})
	}
	return hex.EncodeToString(h.Sum(nil))[:16]
}
