package provision

import (
	"strings"
	"testing"

	"github.com/gridlhq/yeager/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestCloudInitBasic(t *testing.T) {
	t.Parallel()

	ci := GenerateCloudInit(nil, config.SetupConfig{})
	raw := ci.Render()

	// Must start with cloud-init header.
	assert.True(t, strings.HasPrefix(raw, "#cloud-config\n"), "must start with #cloud-config header")

	// Must be valid YAML.
	var doc map[string]any
	err := yaml.Unmarshal([]byte(raw), &doc)
	require.NoError(t, err, "cloud-init must be valid YAML")

	// Must include base packages.
	pkgs, ok := doc["packages"].([]any)
	require.True(t, ok, "must have packages list")
	pkgStrings := toStringSlice(pkgs)
	assert.Contains(t, pkgStrings, "build-essential")
	assert.Contains(t, pkgStrings, "git")
	assert.Contains(t, pkgStrings, "rsync")
	assert.Contains(t, pkgStrings, "curl")
	assert.Contains(t, pkgStrings, "unzip")
}

func TestCloudInitSSHDPorts(t *testing.T) {
	t.Parallel()

	ci := GenerateCloudInit(nil, config.SetupConfig{})
	raw := ci.Render()

	// Must configure sshd on port 22 and 443.
	assert.Contains(t, raw, "Port 22")
	assert.Contains(t, raw, "Port 443")
}

func TestCloudInitWithLanguages(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		langs          []Language
		wantContains   []string
		wantNotContain []string
	}{
		{
			name: "rust runtime only",
			langs: []Language{
				{
					Name:           Rust,
					RuntimeInstall: []string{"curl --proto '=https' --tlsv1.2 -sSf https://sh.rustup.rs | sh -s -- -y", `. "$HOME/.cargo/env"`},
					DepInstall:     []string{"cargo fetch"},
				},
			},
			wantContains:   []string{"rustup.rs"},
			wantNotContain: []string{"cargo fetch"},
		},
		{
			name: "node runtime only",
			langs: []Language{
				{
					Name:           Node,
					RuntimeInstall: []string{"curl -o- https://raw.githubusercontent.com/nvm-sh/nvm/v0.40.1/install.sh | bash", `export NVM_DIR="$HOME/.nvm" && . "$NVM_DIR/nvm.sh" && nvm install --lts`},
					DepInstall:     []string{`export NVM_DIR="$HOME/.nvm" && . "$NVM_DIR/nvm.sh" && npm ci`},
				},
			},
			wantContains:   []string{"nvm"},
			wantNotContain: []string{"npm ci"},
		},
		{
			name: "go runtime only",
			langs: []Language{
				{
					Name:           Go,
					RuntimeInstall: []string{"curl -fsSL https://go.dev/dl/go1.22.0.linux-arm64.tar.gz | tar -C /usr/local -xz"},
					DepInstall:     []string{"go mod download"},
				},
			},
			wantContains:   []string{"go.dev/dl/go1.22.0"},
			wantNotContain: []string{"go mod download"},
		},
		{
			name:  "no languages",
			langs: nil,
			wantContains: []string{
				"build-essential",
			},
			wantNotContain: []string{"rustup", "nvm", "go.dev"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ci := GenerateCloudInit(tt.langs, config.SetupConfig{})
			raw := ci.Render()

			for _, s := range tt.wantContains {
				assert.Contains(t, raw, s)
			}
			for _, s := range tt.wantNotContain {
				assert.NotContains(t, raw, s)
			}
		})
	}
}

func TestCloudInitWithSetupPackages(t *testing.T) {
	t.Parallel()

	setup := config.SetupConfig{
		Packages: []string{"libpq-dev", "chromium-browser"},
	}

	ci := GenerateCloudInit(nil, setup)
	raw := ci.Render()

	var doc map[string]any
	err := yaml.Unmarshal([]byte(raw), &doc)
	require.NoError(t, err)

	pkgs := toStringSlice(doc["packages"].([]any))
	assert.Contains(t, pkgs, "libpq-dev")
	assert.Contains(t, pkgs, "chromium-browser")
}

func TestCloudInitExcludesSetupRunCommands(t *testing.T) {
	t.Parallel()

	// Setup run commands require project files (synced post-boot) and are
	// executed via SSH in Phase 4, NOT in cloud-init.
	setup := config.SetupConfig{
		Run: []string{
			"npx playwright install --with-deps",
			"cargo install cargo-nextest",
		},
	}

	ci := GenerateCloudInit(nil, setup)
	raw := ci.Render()

	assert.NotContains(t, raw, "npx playwright install --with-deps")
	assert.NotContains(t, raw, "cargo install cargo-nextest")
}

func TestCloudInitWithSetupAndLanguages(t *testing.T) {
	t.Parallel()

	langs := []Language{
		{
			Name:           Rust,
			RuntimeInstall: []string{"curl --proto '=https' --tlsv1.2 -sSf https://sh.rustup.rs | sh -s -- -y"},
			DepInstall:     []string{"cargo fetch"},
		},
	}
	setup := config.SetupConfig{
		Packages: []string{"libpq-dev"},
		Run:      []string{"cargo install cargo-nextest"},
	}

	ci := GenerateCloudInit(langs, setup)
	raw := ci.Render()

	// Valid YAML.
	var doc map[string]any
	err := yaml.Unmarshal([]byte(raw), &doc)
	require.NoError(t, err)

	// Must have both base + setup packages.
	pkgs := toStringSlice(doc["packages"].([]any))
	assert.Contains(t, pkgs, "build-essential")
	assert.Contains(t, pkgs, "libpq-dev")

	// Must have runtime install in runcmd.
	assert.Contains(t, raw, "rustup.rs")

	// Dep install and setup run commands are NOT in cloud-init (run post-sync).
	assert.NotContains(t, raw, "cargo fetch")
	assert.NotContains(t, raw, "cargo install cargo-nextest")
}

func TestCloudInitValidYAML(t *testing.T) {
	t.Parallel()

	// Generate with all options to verify YAML is always valid.
	langs := []Language{
		{
			Name:           Rust,
			RuntimeInstall: []string{"install rust"},
			DepInstall:     []string{"cargo fetch"},
		},
		{
			Name:           Node,
			RuntimeInstall: []string{"install nvm", "nvm install --lts"},
			DepInstall:     []string{"npm ci"},
		},
	}
	setup := config.SetupConfig{
		Packages: []string{"pkg1", "pkg2"},
	}

	ci := GenerateCloudInit(langs, setup)
	raw := ci.Render()

	var doc map[string]any
	err := yaml.Unmarshal([]byte(raw), &doc)
	require.NoError(t, err, "generated cloud-init must always be valid YAML")

	// Verify runcmd is a list (sshd config + runtime installs + mkdir).
	runcmd, ok := doc["runcmd"].([]any)
	require.True(t, ok, "runcmd must be a list")
	assert.NotEmpty(t, runcmd)

	// Verify setup packages are in the packages list.
	pkgs := toStringSlice(doc["packages"].([]any))
	assert.Contains(t, pkgs, "pkg1")
	assert.Contains(t, pkgs, "pkg2")
}

func TestSetupHash(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		a, b      config.SetupConfig
		wantEqual bool
	}{
		{
			name:      "identical empty",
			a:         config.SetupConfig{},
			b:         config.SetupConfig{},
			wantEqual: true,
		},
		{
			name: "identical with values",
			a: config.SetupConfig{
				Packages: []string{"libpq-dev"},
				Run:      []string{"echo hello"},
			},
			b: config.SetupConfig{
				Packages: []string{"libpq-dev"},
				Run:      []string{"echo hello"},
			},
			wantEqual: true,
		},
		{
			name: "different packages",
			a: config.SetupConfig{
				Packages: []string{"libpq-dev"},
			},
			b: config.SetupConfig{
				Packages: []string{"libssl-dev"},
			},
			wantEqual: false,
		},
		{
			name: "different run",
			a: config.SetupConfig{
				Run: []string{"echo hello"},
			},
			b: config.SetupConfig{
				Run: []string{"echo world"},
			},
			wantEqual: false,
		},
		{
			name: "packages vs empty",
			a: config.SetupConfig{
				Packages: []string{"libpq-dev"},
			},
			b:         config.SetupConfig{},
			wantEqual: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			hashA := SetupHash(tt.a)
			hashB := SetupHash(tt.b)
			if tt.wantEqual {
				assert.Equal(t, hashA, hashB)
			} else {
				assert.NotEqual(t, hashA, hashB)
			}
			// Hash should be non-empty.
			assert.NotEmpty(t, hashA)
			assert.Len(t, hashA, 16, "setup hash should be 16 hex chars")
		})
	}
}

func toStringSlice(vals []any) []string {
	out := make([]string, len(vals))
	for i, v := range vals {
		out[i] = v.(string)
	}
	return out
}
