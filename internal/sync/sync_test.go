package sync

import (
	"strings"
	"testing"

	"github.com/gridlhq/yeager/internal/config"
	"github.com/gridlhq/yeager/internal/provision"
	"github.com/stretchr/testify/assert"
)

func TestDefaultExcludes(t *testing.T) {
	t.Parallel()

	// Default excludes must always include these.
	for _, exc := range []string{".git/", "node_modules/", "target/", "__pycache__/", ".venv/", "dist/", "build/", ".next/", ".tox/"} {
		assert.Contains(t, DefaultExcludes, exc)
	}
}

func TestBuildArgs(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		opts           Options
		wantContain    []string // exact elements in args slice
		wantSubstring  []string // substrings in the joined args string
	}{
		{
			name: "basic rsync args",
			opts: Options{
				SourceDir: "/home/user/project/",
				RemoteDir: "/home/ubuntu/project/",
				Host:      "1.2.3.4",
				User:      "ubuntu",
				SSHPort:   22,
			},
			wantContain: []string{
				"-az",
				"--delete",
				"--stats",
				"/home/user/project/",
				"ubuntu@1.2.3.4:/home/ubuntu/project/",
			},
			wantSubstring: []string{
				"-p 22",
				"StrictHostKeyChecking=no",
			},
		},
		{
			name: "includes default excludes",
			opts: Options{
				SourceDir: "/src/",
				RemoteDir: "/dst/",
				Host:      "1.2.3.4",
				User:      "ubuntu",
				SSHPort:   22,
			},
			wantContain: []string{
				"--exclude", ".git/",
				"node_modules/",
				"target/",
			},
		},
		{
			name: "with gitignore filter",
			opts: Options{
				SourceDir: "/src/",
				RemoteDir: "/dst/",
				Host:      "1.2.3.4",
				User:      "ubuntu",
				SSHPort:   22,
			},
			wantContain: []string{
				"--filter", ":- .gitignore",
			},
		},
		{
			name: "with sync config excludes",
			opts: Options{
				SourceDir: "/src/",
				RemoteDir: "/dst/",
				Host:      "1.2.3.4",
				User:      "ubuntu",
				SSHPort:   22,
				SyncConfig: config.SyncConfig{
					Exclude: []string{"data/", "logs/"},
				},
			},
			wantContain: []string{
				"--exclude", "data/",
				"--exclude", "logs/",
			},
		},
		{
			name: "with sync config includes",
			opts: Options{
				SourceDir: "/src/",
				RemoteDir: "/dst/",
				Host:      "1.2.3.4",
				User:      "ubuntu",
				SSHPort:   22,
				SyncConfig: config.SyncConfig{
					Include: []string{"fixtures/large-dataset.bin"},
				},
			},
			wantContain: []string{
				"--include", "fixtures/large-dataset.bin",
			},
		},
		{
			name: "port 443 fallback",
			opts: Options{
				SourceDir: "/src/",
				RemoteDir: "/dst/",
				Host:      "1.2.3.4",
				User:      "ubuntu",
				SSHPort:   443,
			},
			wantSubstring: []string{"-p 443"},
		},
		{
			name: "with SSH key",
			opts: Options{
				SourceDir:  "/src/",
				RemoteDir:  "/dst/",
				Host:       "1.2.3.4",
				User:       "ubuntu",
				SSHPort:    22,
				SSHKeyPath: "/tmp/yeager-key",
			},
			wantSubstring: []string{`-i "/tmp/yeager-key"`},
		},
		{
			name: "with language-specific excludes",
			opts: Options{
				SourceDir: "/src/",
				RemoteDir: "/dst/",
				Host:      "1.2.3.4",
				User:      "ubuntu",
				SSHPort:   22,
				Languages: []provision.LanguageName{provision.Go},
			},
			wantContain: []string{
				"--exclude", "vendor/",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			args := BuildArgs(tt.opts)

			for _, want := range tt.wantContain {
				assert.Contains(t, args, want, "missing expected arg %q", want)
			}

			// Check substrings (for args embedded in the SSH command string).
			if len(tt.wantSubstring) > 0 {
				joined := strings.Join(args, " ")
				for _, sub := range tt.wantSubstring {
					assert.Contains(t, joined, sub, "missing substring %q in args", sub)
				}
			}
		})
	}
}

func TestBuildArgs_IncludeBeforeExclude(t *testing.T) {
	t.Parallel()

	opts := Options{
		SourceDir: "/src/",
		RemoteDir: "/dst/",
		Host:      "1.2.3.4",
		User:      "ubuntu",
		SSHPort:   22,
		SyncConfig: config.SyncConfig{
			Include: []string{"kept.bin"},
			Exclude: []string{"data/"},
		},
	}

	args := BuildArgs(opts)

	// Find positions of --include and the first --exclude.
	includeIdx := -1
	excludeIdx := -1
	for i, a := range args {
		if a == "--include" && includeIdx == -1 {
			includeIdx = i
		}
		if a == "--exclude" && excludeIdx == -1 {
			excludeIdx = i
		}
	}

	assert.Greater(t, includeIdx, -1, "--include must be present")
	assert.Greater(t, excludeIdx, -1, "--exclude must be present")
	assert.Less(t, includeIdx, excludeIdx, "--include must come before --exclude")
}

func TestBuildArgs_GitignoreFilterBeforeExcludes(t *testing.T) {
	t.Parallel()

	opts := Options{
		SourceDir: "/src/",
		RemoteDir: "/dst/",
		Host:      "1.2.3.4",
		User:      "ubuntu",
		SSHPort:   22,
	}

	args := BuildArgs(opts)

	filterIdx := -1
	excludeIdx := -1
	for i, a := range args {
		if a == "--filter" && filterIdx == -1 {
			filterIdx = i
		}
		if a == "--exclude" && excludeIdx == -1 {
			excludeIdx = i
		}
	}

	assert.Greater(t, filterIdx, -1, "--filter must be present")
	assert.Greater(t, excludeIdx, -1, "--exclude must be present")
	assert.Less(t, filterIdx, excludeIdx, "--filter must come before --exclude")
}

func TestLanguageExcludes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		langs    []provision.LanguageName
		wantHave []string
	}{
		{
			name:     "go vendor",
			langs:    []provision.LanguageName{provision.Go},
			wantHave: []string{"vendor/"},
		},
		{
			name:     "rust target already in defaults",
			langs:    []provision.LanguageName{provision.Rust},
			wantHave: nil, // target/ is already in defaults
		},
		{
			name:     "multiple languages",
			langs:    []provision.LanguageName{provision.Go, provision.Node},
			wantHave: []string{"vendor/"},
		},
		{
			name:     "no languages",
			langs:    nil,
			wantHave: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			extras := LanguageExcludes(tt.langs)
			if tt.wantHave == nil {
				assert.Empty(t, extras, "expected no extra excludes")
			} else {
				for _, want := range tt.wantHave {
					assert.Contains(t, extras, want)
				}
			}
		})
	}
}

func TestParseStats(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		output string
		want   SyncResult
	}{
		{
			name: "typical rsync output",
			output: `Number of files: 847
Number of files transferred: 847
Total file size: 12,582,912 bytes
Total transferred file size: 12,582,912 bytes
Literal data: 11,234,567 bytes
Matched data: 0 bytes`,
			want: SyncResult{
				TotalFiles:       847,
				FilesTransferred: 847,
				BytesTransferred: 12_582_912,
			},
		},
		{
			name: "warm run with few changes",
			output: `Number of files: 847
Number of files transferred: 3
Total file size: 12,582,912 bytes
Total transferred file size: 4,096 bytes
Literal data: 1,234 bytes
Matched data: 2,862 bytes`,
			want: SyncResult{
				TotalFiles:       847,
				FilesTransferred: 3,
				BytesTransferred: 4_096,
			},
		},
		{
			name: "no files transferred",
			output: `Number of files: 200
Number of files transferred: 0
Total file size: 5,000,000 bytes
Total transferred file size: 0 bytes`,
			want: SyncResult{
				TotalFiles:       200,
				FilesTransferred: 0,
				BytesTransferred: 0,
			},
		},
		{
			name: "no commas in small numbers",
			output: `Number of files: 5
Number of files transferred: 2
Total file size: 1024 bytes
Total transferred file size: 512 bytes`,
			want: SyncResult{
				TotalFiles:       5,
				FilesTransferred: 2,
				BytesTransferred: 512,
			},
		},
		{
			name:   "empty output returns zero result",
			output: "",
			want:   SyncResult{},
		},
		{
			name:   "garbage input returns zero result",
			output: "this is not rsync output at all",
			want:   SyncResult{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := ParseStats(tt.output)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestFormatBytes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		bytes    int64
		expected string
	}{
		{0, "0 B"},
		{512, "512 B"},
		{1023, "1023 B"},
		{1024, "1.0 KB"},
		{1536, "1.5 KB"},
		{1048576, "1.0 MB"},
		{12_582_912, "12.0 MB"},
		{1073741824, "1.0 GB"},
		{2_684_354_560, "2.5 GB"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.expected, FormatBytes(tt.bytes))
		})
	}
}
