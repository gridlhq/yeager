package project

import (
	"crypto/sha256"
	"fmt"
	"path/filepath"
)

// Project represents a yeager project identified by its directory path.
type Project struct {
	// AbsPath is the normalized absolute path of the project directory.
	AbsPath string

	// Hash is a stable, short identifier derived from AbsPath.
	// Used for VM lookup, S3 paths, and state directory keys.
	Hash string

	// DisplayName is a human-readable name derived from the directory basename.
	DisplayName string
}

// Resolve creates a Project from an absolute directory path.
// The path is normalized (trailing slashes removed, cleaned).
// Returns an error if the path is empty or not absolute.
func Resolve(absPath string) (Project, error) {
	if absPath == "" {
		return Project{}, fmt.Errorf("project path cannot be empty")
	}

	cleaned := filepath.Clean(absPath)

	if !filepath.IsAbs(cleaned) {
		return Project{}, fmt.Errorf("project path must be absolute: %s", absPath)
	}

	return Project{
		AbsPath:     cleaned,
		Hash:        hashPath(cleaned),
		DisplayName: deriveName(cleaned),
	}, nil
}

// hashPath produces a short, stable hash from a normalized path.
// Uses SHA-256 truncated to 12 hex characters (48 bits of entropy).
func hashPath(path string) string {
	h := sha256.Sum256([]byte(path))
	return fmt.Sprintf("%x", h[:6])
}

// deriveName extracts a display name from the directory path.
// Uses the last path component (basename).
func deriveName(path string) string {
	name := filepath.Base(path)
	// filepath.Base returns "/" for root path
	if name == "/" {
		return "/"
	}
	return name
}
