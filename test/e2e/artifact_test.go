//go:build e2e

package e2e

import (
	"testing"
	"time"
)

// TestE2E_ArtifactUpload verifies that artifacts specified in .yeager.toml
// are uploaded to S3 after command execution.
func TestE2E_ArtifactUpload(t *testing.T) {
	dir := uniqueDir(t)
	setupProjectWithArtifacts(t, dir)
	t.Cleanup(func() { destroyProject(t, dir) })

	// Run the command that produces artifacts.
	out := runFKSuccess(t, dir, 10*time.Minute, "go", "run", "main.go")
	requireContainsAll(t, out,
		"running: go run main.go",
		"done",
		"output saved",
		"uploaded 1 artifact(s)",
	)
}
