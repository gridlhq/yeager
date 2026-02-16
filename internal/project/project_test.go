package project

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResolve(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		path        string
		wantHash    string
		wantName    string
		wantAbsPath string
	}{
		{
			name:        "simple project path",
			path:        "/Users/dev/repos/my-app",
			wantHash:    "fccbc07aaf9f",
			wantName:    "my-app",
			wantAbsPath: "/Users/dev/repos/my-app",
		},
		{
			name:        "nested path",
			path:        "/home/user/code/org/service",
			wantHash:    "2dbdf3cfc91b",
			wantName:    "service",
			wantAbsPath: "/home/user/code/org/service",
		},
		{
			name:        "trailing slash is normalized",
			path:        "/Users/dev/repos/my-app/",
			wantHash:    "fccbc07aaf9f",
			wantName:    "my-app",
			wantAbsPath: "/Users/dev/repos/my-app",
		},
		{
			name:        "double slash is normalized",
			path:        "/Users/dev//repos//my-app",
			wantHash:    "fccbc07aaf9f",
			wantName:    "my-app",
			wantAbsPath: "/Users/dev/repos/my-app",
		},
		{
			name:        "root path",
			path:        "/",
			wantHash:    "8a5edab28263",
			wantName:    "/",
			wantAbsPath: "/",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			proj, err := Resolve(tt.path)
			require.NoError(t, err)
			assert.Equal(t, tt.wantHash, proj.Hash)
			assert.Equal(t, tt.wantName, proj.DisplayName)
			assert.Equal(t, tt.wantAbsPath, proj.AbsPath)
		})
	}
}

func TestResolveErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		path    string
		wantErr string
	}{
		{
			name:    "empty path",
			path:    "",
			wantErr: "project path cannot be empty",
		},
		{
			name:    "relative path",
			path:    "repos/my-app",
			wantErr: "project path must be absolute",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			_, err := Resolve(tt.path)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}

func TestHashStability(t *testing.T) {
	t.Parallel()

	// Same path must always produce the same hash.
	path := "/Users/dev/repos/my-project"
	p1, err := Resolve(path)
	require.NoError(t, err)
	p2, err := Resolve(path)
	require.NoError(t, err)
	assert.Equal(t, p1.Hash, p2.Hash)
}

func TestHashUniqueness(t *testing.T) {
	t.Parallel()

	// Different paths must produce different hashes.
	p1, err := Resolve("/Users/dev/project-a")
	require.NoError(t, err)
	p2, err := Resolve("/Users/dev/project-b")
	require.NoError(t, err)
	assert.NotEqual(t, p1.Hash, p2.Hash)
}
