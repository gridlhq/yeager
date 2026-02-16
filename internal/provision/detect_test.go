package provision

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDetectLanguages(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		files     map[string]string // filename → content
		wantLangs []LanguageName
	}{
		{
			name:      "empty directory",
			files:     map[string]string{},
			wantLangs: nil,
		},
		{
			name: "rust project",
			files: map[string]string{
				"Cargo.toml": `[package]
name = "myapp"
version = "0.1.0"
`,
			},
			wantLangs: []LanguageName{Rust},
		},
		{
			name: "node project",
			files: map[string]string{
				"package.json": `{"name": "myapp", "version": "1.0.0"}`,
			},
			wantLangs: []LanguageName{Node},
		},
		{
			name: "go project",
			files: map[string]string{
				"go.mod": `module example.com/myapp

go 1.22.0
`,
			},
			wantLangs: []LanguageName{Go},
		},
		{
			name: "python project with pyproject.toml",
			files: map[string]string{
				"pyproject.toml": `[project]
name = "myapp"
`,
			},
			wantLangs: []LanguageName{Python},
		},
		{
			name: "python project with requirements.txt",
			files: map[string]string{
				"requirements.txt": "flask==3.0.0\nrequests\n",
			},
			wantLangs: []LanguageName{Python},
		},
		{
			name: "ruby project",
			files: map[string]string{
				"Gemfile": `source "https://rubygems.org"
gem "rails"
`,
			},
			wantLangs: []LanguageName{Ruby},
		},
		{
			name: "multi-language project",
			files: map[string]string{
				"go.mod":       "module example.com/myapp\n\ngo 1.22.0\n",
				"package.json": `{"name": "frontend"}`,
			},
			wantLangs: []LanguageName{Node, Go},
		},
		{
			name: "all languages",
			files: map[string]string{
				"Cargo.toml":   "[package]\nname = \"app\"\n",
				"package.json": "{}",
				"go.mod":       "module m\n\ngo 1.22.0\n",
				"pyproject.toml": "[project]\nname = \"app\"\n",
				"Gemfile":      "source 'https://rubygems.org'\n",
			},
			wantLangs: []LanguageName{Rust, Node, Go, Python, Ruby},
		},
		{
			name: "python prefers pyproject.toml over requirements.txt",
			files: map[string]string{
				"pyproject.toml":   "[project]\nname = \"app\"\n",
				"requirements.txt": "flask\n",
			},
			wantLangs: []LanguageName{Python},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()

			for name, content := range tt.files {
				err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0644)
				require.NoError(t, err)
			}

			langs := DetectLanguages(dir)

			if tt.wantLangs == nil {
				assert.Empty(t, langs)
				return
			}

			gotNames := make([]LanguageName, len(langs))
			for i, l := range langs {
				gotNames[i] = l.Name
			}
			assert.Equal(t, tt.wantLangs, gotNames)
		})
	}
}

func TestDetectLanguage_RuntimeCommands(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		files          map[string]string
		wantRuntime    []string
		wantDepInstall []string
	}{
		{
			name: "rust",
			files: map[string]string{
				"Cargo.toml": "[package]\nname = \"app\"\n",
			},
			wantRuntime:    []string{"curl --proto '=https' --tlsv1.2 -sSf https://sh.rustup.rs | sh -s -- -y", ". \"$HOME/.cargo/env\""},
			wantDepInstall: []string{"cargo fetch"},
		},
		{
			name: "node without lockfile uses npm install",
			files: map[string]string{
				"package.json": "{}",
			},
			wantRuntime:    []string{"curl -o- https://raw.githubusercontent.com/nvm-sh/nvm/v0.40.1/install.sh | bash", "export NVM_DIR=\"$HOME/.nvm\" && . \"$NVM_DIR/nvm.sh\" && nvm install --lts"},
			wantDepInstall: []string{"export NVM_DIR=\"$HOME/.nvm\" && . \"$NVM_DIR/nvm.sh\" && npm install"},
		},
		{
			name: "node with nvmrc (no lockfile)",
			files: map[string]string{
				"package.json": "{}",
				".nvmrc":       "20",
			},
			wantRuntime:    []string{"curl -o- https://raw.githubusercontent.com/nvm-sh/nvm/v0.40.1/install.sh | bash", "export NVM_DIR=\"$HOME/.nvm\" && . \"$NVM_DIR/nvm.sh\" && nvm install 20"},
			wantDepInstall: []string{"export NVM_DIR=\"$HOME/.nvm\" && . \"$NVM_DIR/nvm.sh\" && npm install"},
		},
		{
			name: "node with package-lock",
			files: map[string]string{
				"package.json":      "{}",
				"package-lock.json": "{}",
			},
			wantRuntime:    []string{"curl -o- https://raw.githubusercontent.com/nvm-sh/nvm/v0.40.1/install.sh | bash", "export NVM_DIR=\"$HOME/.nvm\" && . \"$NVM_DIR/nvm.sh\" && nvm install --lts"},
			wantDepInstall: []string{"export NVM_DIR=\"$HOME/.nvm\" && . \"$NVM_DIR/nvm.sh\" && npm ci"},
		},
		{
			name: "node with yarn.lock",
			files: map[string]string{
				"package.json": "{}",
				"yarn.lock":    "",
			},
			wantRuntime:    []string{"curl -o- https://raw.githubusercontent.com/nvm-sh/nvm/v0.40.1/install.sh | bash", "export NVM_DIR=\"$HOME/.nvm\" && . \"$NVM_DIR/nvm.sh\" && nvm install --lts"},
			wantDepInstall: []string{"export NVM_DIR=\"$HOME/.nvm\" && . \"$NVM_DIR/nvm.sh\" && yarn install --frozen-lockfile"},
		},
		{
			name: "node with pnpm-lock",
			files: map[string]string{
				"package.json":    "{}",
				"pnpm-lock.yaml": "",
			},
			wantRuntime:    []string{"curl -o- https://raw.githubusercontent.com/nvm-sh/nvm/v0.40.1/install.sh | bash", "export NVM_DIR=\"$HOME/.nvm\" && . \"$NVM_DIR/nvm.sh\" && nvm install --lts"},
			wantDepInstall: []string{"export NVM_DIR=\"$HOME/.nvm\" && . \"$NVM_DIR/nvm.sh\" && npm install -g pnpm && pnpm install --frozen-lockfile"},
		},
		{
			name: "go project",
			files: map[string]string{
				"go.mod": "module m\n\ngo 1.22.0\n",
			},
			wantRuntime:    []string{"curl -fsSL https://go.dev/dl/go1.22.0.linux-arm64.tar.gz | tar -C /usr/local -xz", "echo 'export PATH=$PATH:/usr/local/go/bin:$HOME/go/bin' >> $HOME/.bashrc"},
			wantDepInstall: []string{"export PATH=$PATH:/usr/local/go/bin && go mod download"},
		},
		{
			name: "python with pyproject.toml",
			files: map[string]string{
				"pyproject.toml": "[project]\nname = \"app\"\n",
			},
			wantRuntime:    []string{"apt-get install -y python3 python3-pip python3-venv"},
			wantDepInstall: []string{"python3 -m pip install -e ."},
		},
		{
			name: "python with requirements.txt",
			files: map[string]string{
				"requirements.txt": "flask\n",
			},
			wantRuntime:    []string{"apt-get install -y python3 python3-pip python3-venv"},
			wantDepInstall: []string{"python3 -m pip install -r requirements.txt"},
		},
		{
			name: "ruby",
			files: map[string]string{
				"Gemfile": "source 'https://rubygems.org'\n",
			},
			wantRuntime:    []string{"apt-get install -y ruby-full"},
			wantDepInstall: []string{"bundle install"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()

			for name, content := range tt.files {
				err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0644)
				require.NoError(t, err)
			}

			langs := DetectLanguages(dir)
			require.NotEmpty(t, langs, "expected at least one language detected")

			lang := langs[0]
			assert.Equal(t, tt.wantRuntime, lang.RuntimeInstall)
			assert.Equal(t, tt.wantDepInstall, lang.DepInstall)
		})
	}
}

func TestParseGoVersion(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		content string
		want    string
	}{
		{
			name:    "standard version",
			content: "module m\n\ngo 1.22.0\n",
			want:    "1.22.0",
		},
		{
			name:    "minor only",
			content: "module m\n\ngo 1.22\n",
			want:    "1.22.0",
		},
		{
			name:    "with toolchain",
			content: "module m\n\ngo 1.23.4\n\ntoolchain go1.23.5\n",
			want:    "1.23.4",
		},
		{
			name:    "no go directive",
			content: "module m\n",
			want:    defaultGoVersion,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := parseGoVersion(tt.content)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestParseNvmrc(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		content string
		want    string
	}{
		{name: "major version", content: "20\n", want: "20"},
		{name: "minor version", content: "20.11\n", want: "20.11"},
		{name: "full version", content: "v20.11.1\n", want: "v20.11.1"},
		{name: "lts", content: "lts/*\n", want: "lts/*"},
		{name: "with whitespace", content: "  18  \n", want: "18"},
		{name: "empty file", content: "", want: "--lts"},
		{name: "whitespace only", content: "  \n  \n", want: "--lts"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := parseNvmrc(tt.content)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestLockfileForLanguage(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		lang     LanguageName
		files    map[string]string
		wantFile string
	}{
		{
			name:     "rust Cargo.lock",
			lang:     Rust,
			files:    map[string]string{"Cargo.lock": "lock content"},
			wantFile: "Cargo.lock",
		},
		{
			name:     "node package-lock.json",
			lang:     Node,
			files:    map[string]string{"package-lock.json": "{}"},
			wantFile: "package-lock.json",
		},
		{
			name:     "node yarn.lock",
			lang:     Node,
			files:    map[string]string{"yarn.lock": "lock"},
			wantFile: "yarn.lock",
		},
		{
			name:     "node pnpm-lock.yaml",
			lang:     Node,
			files:    map[string]string{"pnpm-lock.yaml": "lock"},
			wantFile: "pnpm-lock.yaml",
		},
		{
			name:     "go go.sum",
			lang:     Go,
			files:    map[string]string{"go.sum": "checksums"},
			wantFile: "go.sum",
		},
		{
			name:     "python no lockfile",
			lang:     Python,
			files:    map[string]string{},
			wantFile: "",
		},
		{
			name:     "ruby Gemfile.lock",
			lang:     Ruby,
			files:    map[string]string{"Gemfile.lock": "lock"},
			wantFile: "Gemfile.lock",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()

			for name, content := range tt.files {
				err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0644)
				require.NoError(t, err)
			}

			got := LockfileForLanguage(tt.lang, dir)
			if tt.wantFile == "" {
				assert.Empty(t, got)
			} else {
				assert.Equal(t, filepath.Join(dir, tt.wantFile), got)
			}
		})
	}
}
