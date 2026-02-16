package sync

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/gridlhq/yeager/internal/config"
	"github.com/gridlhq/yeager/internal/provision"
)

// SyncResult holds statistics from an rsync invocation.
type SyncResult struct {
	FilesTransferred int   // number of files actually sent
	TotalFiles       int   // total number of files considered
	BytesTransferred int64 // total transferred file size in bytes
}

// DefaultExcludes are always excluded from rsync regardless of language.
var DefaultExcludes = []string{
	".git/",
	"node_modules/",
	"target/",
	"__pycache__/",
	".venv/",
	"dist/",
	"build/",
	".next/",
	".tox/",
	".mypy_cache/",
	".pytest_cache/",
	".cargo/",
	".gradle/",
	"*.pyc",
	".DS_Store",
}

// languageExtraExcludes are additional excludes per language
// (only things NOT already in DefaultExcludes).
var languageExtraExcludes = map[provision.LanguageName][]string{
	provision.Go: {"vendor/"},
}

// Options configures an rsync invocation.
type Options struct {
	SourceDir  string // local directory (must end with /)
	RemoteDir  string // remote directory (must end with /)
	Host       string // remote IP or hostname
	User       string // SSH user
	SSHPort    int    // SSH port (22 or 443)
	SSHKeyPath string // path to SSH private key (optional)
	SyncConfig config.SyncConfig
	Languages  []provision.LanguageName // detected project languages
}

// BuildArgs constructs the rsync argument list.
// Does not execute rsync — just builds the args for testability.
func BuildArgs(opts Options) []string {
	args := []string{
		"-az",
		"--delete",
		"--stats",
	}

	// SSH transport.
	sshCmd := fmt.Sprintf("ssh -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null -p %d", opts.SSHPort)
	if opts.SSHKeyPath != "" {
		sshCmd += fmt.Sprintf(" -i %q", opts.SSHKeyPath)
	}
	args = append(args, "-e", sshCmd)

	// Includes first (rsync evaluates rules in order).
	for _, inc := range opts.SyncConfig.Include {
		args = append(args, "--include", inc)
	}

	// .gitignore filter (before explicit excludes so it takes effect).
	args = append(args, "--filter", ":- .gitignore")

	// User-configured excludes from .yeager.toml [sync] section.
	for _, exc := range opts.SyncConfig.Exclude {
		args = append(args, "--exclude", exc)
	}

	// Language-specific excludes.
	for _, exc := range LanguageExcludes(opts.Languages) {
		args = append(args, "--exclude", exc)
	}

	// Default excludes.
	for _, exc := range DefaultExcludes {
		args = append(args, "--exclude", exc)
	}

	// Source and destination.
	args = append(args, opts.SourceDir)
	args = append(args, fmt.Sprintf("%s@%s:%s", opts.User, opts.Host, opts.RemoteDir))

	return args
}

// LanguageExcludes returns extra exclude patterns for the given languages.
func LanguageExcludes(langs []provision.LanguageName) []string {
	var extras []string
	seen := make(map[string]bool)
	for _, lang := range langs {
		for _, exc := range languageExtraExcludes[lang] {
			if !seen[exc] {
				extras = append(extras, exc)
				seen[exc] = true
			}
		}
	}
	return extras
}

var (
	reNumFiles     = regexp.MustCompile(`Number of files: ([\d,]+)`)
	reTransferred  = regexp.MustCompile(`Number of files transferred: ([\d,]+)`)
	reTotalSize    = regexp.MustCompile(`Total transferred file size: ([\d,]+)`)
)

// ParseStats extracts sync statistics from rsync --stats output.
// Returns a zero SyncResult if parsing fails (best-effort).
func ParseStats(output string) SyncResult {
	var r SyncResult
	if m := reNumFiles.FindStringSubmatch(output); len(m) > 1 {
		r.TotalFiles = parseCommaInt(m[1])
	}
	if m := reTransferred.FindStringSubmatch(output); len(m) > 1 {
		r.FilesTransferred = parseCommaInt(m[1])
	}
	if m := reTotalSize.FindStringSubmatch(output); len(m) > 1 {
		r.BytesTransferred = parseCommaInt64(m[1])
	}
	return r
}

// FormatBytes formats bytes into a human-readable string.
func FormatBytes(b int64) string {
	switch {
	case b >= 1<<30:
		return fmt.Sprintf("%.1f GB", float64(b)/float64(1<<30))
	case b >= 1<<20:
		return fmt.Sprintf("%.1f MB", float64(b)/float64(1<<20))
	case b >= 1<<10:
		return fmt.Sprintf("%.1f KB", float64(b)/float64(1<<10))
	default:
		return fmt.Sprintf("%d B", b)
	}
}

func parseCommaInt(s string) int {
	s = strings.ReplaceAll(s, ",", "")
	n, _ := strconv.Atoi(s)
	return n
}

func parseCommaInt64(s string) int64 {
	s = strings.ReplaceAll(s, ",", "")
	n, _ := strconv.ParseInt(s, 10, 64)
	return n
}
