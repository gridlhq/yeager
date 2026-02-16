package provision

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// LanguageName identifies a detected language.
type LanguageName string

const (
	Rust   LanguageName = "rust"
	Node   LanguageName = "node"
	Go     LanguageName = "go"
	Python LanguageName = "python"
	Ruby   LanguageName = "ruby"
)

const (
	defaultGoVersion = "1.22.0"
	nvmVersion       = "v0.40.1"
)

// Language represents a detected language with its provisioning commands.
type Language struct {
	Name           LanguageName
	DisplayName    string   // e.g. "Rust (Cargo.toml)"
	RuntimeInstall []string // shell commands to install the runtime
	DepInstall     []string // shell commands to install dependencies
}

// DetectLanguages scans a project directory for known manifest files
// and returns the detected languages in a stable order.
// Returns nil if no languages are detected.
func DetectLanguages(dir string) []Language {
	var langs []Language

	// Detection order is stable: Rust, Node, Go, Python, Ruby.
	// This matches the priority table in FEATURES.md.

	if fileExists(dir, "Cargo.toml") {
		langs = append(langs, detectRust(dir))
	}

	if fileExists(dir, "package.json") {
		langs = append(langs, detectNode(dir))
	}

	if fileExists(dir, "go.mod") {
		langs = append(langs, detectGo(dir))
	}

	// Python: prefer pyproject.toml over requirements.txt.
	if fileExists(dir, "pyproject.toml") {
		langs = append(langs, detectPython(dir, "pyproject.toml"))
	} else if fileExists(dir, "requirements.txt") {
		langs = append(langs, detectPython(dir, "requirements.txt"))
	}

	if fileExists(dir, "Gemfile") {
		langs = append(langs, detectRuby(dir))
	}

	return langs
}

func detectRust(_ string) Language {
	return Language{
		Name:        Rust,
		DisplayName: "Rust (Cargo.toml)",
		RuntimeInstall: []string{
			"curl --proto '=https' --tlsv1.2 -sSf https://sh.rustup.rs | sh -s -- -y",
			`. "$HOME/.cargo/env"`,
		},
		DepInstall: []string{"cargo fetch"},
	}
}

func detectNode(dir string) Language {
	// Detect Node version from .nvmrc if present.
	nodeVersion := "--lts"
	if content, err := os.ReadFile(filepath.Join(dir, ".nvmrc")); err == nil {
		nodeVersion = parseNvmrc(string(content))
	}

	nvmInstall := fmt.Sprintf("curl -o- https://raw.githubusercontent.com/nvm-sh/nvm/%s/install.sh | bash", nvmVersion)
	nvmLoad := `export NVM_DIR="$HOME/.nvm" && . "$NVM_DIR/nvm.sh"`
	nodeInstall := fmt.Sprintf("%s && nvm install %s", nvmLoad, nodeVersion)

	// Detect package manager from lockfile.
	// npm ci requires package-lock.json — fall back to npm install if absent.
	depCmd := fmt.Sprintf("%s && npm install", nvmLoad)
	if fileExists(dir, "package-lock.json") {
		depCmd = fmt.Sprintf("%s && npm ci", nvmLoad)
	} else if fileExists(dir, "yarn.lock") {
		depCmd = fmt.Sprintf("%s && yarn install --frozen-lockfile", nvmLoad)
	} else if fileExists(dir, "pnpm-lock.yaml") {
		depCmd = fmt.Sprintf("%s && npm install -g pnpm && pnpm install --frozen-lockfile", nvmLoad)
	}

	return Language{
		Name:           Node,
		DisplayName:    "Node (package.json)",
		RuntimeInstall: []string{nvmInstall, nodeInstall},
		DepInstall:     []string{depCmd},
	}
}

func detectGo(dir string) Language {
	version := defaultGoVersion
	if content, err := os.ReadFile(filepath.Join(dir, "go.mod")); err == nil {
		version = parseGoVersion(string(content))
	}

	return Language{
		Name:        Go,
		DisplayName: "Go (go.mod)",
		RuntimeInstall: []string{
			fmt.Sprintf("curl -fsSL https://go.dev/dl/go%s.linux-arm64.tar.gz | tar -C /usr/local -xz", version),
			"echo 'export PATH=$PATH:/usr/local/go/bin:$HOME/go/bin' >> $HOME/.bashrc",
		},
		DepInstall: []string{
			"export PATH=$PATH:/usr/local/go/bin && go mod download",
		},
	}
}

func detectPython(_ string, manifest string) Language {
	depCmd := "python3 -m pip install -e ."
	if manifest == "requirements.txt" {
		depCmd = "python3 -m pip install -r requirements.txt"
	}

	return Language{
		Name:        Python,
		DisplayName: fmt.Sprintf("Python (%s)", manifest),
		RuntimeInstall: []string{
			"apt-get install -y python3 python3-pip python3-venv",
		},
		DepInstall: []string{depCmd},
	}
}

func detectRuby(_ string) Language {
	return Language{
		Name:        Ruby,
		DisplayName: "Ruby (Gemfile)",
		RuntimeInstall: []string{
			"apt-get install -y ruby-full",
		},
		DepInstall: []string{"bundle install"},
	}
}

// LockfileForLanguage returns the path to the lockfile for a language,
// or empty string if none exists.
func LockfileForLanguage(lang LanguageName, dir string) string {
	candidates := lockfileCandidates[lang]
	for _, c := range candidates {
		path := filepath.Join(dir, c)
		if _, err := os.Stat(path); err == nil {
			return path
		}
	}
	return ""
}

var lockfileCandidates = map[LanguageName][]string{
	Rust:   {"Cargo.lock"},
	Node:   {"package-lock.json", "yarn.lock", "pnpm-lock.yaml"},
	Go:     {"go.sum"},
	Python: {"poetry.lock", "Pipfile.lock", "requirements.txt"},
	Ruby:   {"Gemfile.lock"},
}

// parseGoVersion extracts the Go version from go.mod content.
// Returns defaultGoVersion if the directive is not found.
var goVersionRe = regexp.MustCompile(`(?m)^go\s+(\d+\.\d+(?:\.\d+)?)$`)

func parseGoVersion(content string) string {
	m := goVersionRe.FindStringSubmatch(content)
	if m == nil {
		return defaultGoVersion
	}
	v := m[1]
	// Normalize "1.22" → "1.22.0".
	if strings.Count(v, ".") == 1 {
		v += ".0"
	}
	return v
}

// parseNvmrc trims whitespace from .nvmrc content.
// Returns "--lts" if the file is empty or whitespace-only.
func parseNvmrc(content string) string {
	v := strings.TrimSpace(content)
	if v == "" {
		return "--lts"
	}
	return v
}

func fileExists(dir, name string) bool {
	_, err := os.Stat(filepath.Join(dir, name))
	return err == nil
}
