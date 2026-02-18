//go:build livefire

package livefire

import (
	"context"
	"crypto/md5"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ---------------------------------------------------------------------------
// JSON validation steps
// ---------------------------------------------------------------------------

func outputShouldBeValidJSON(ctx context.Context) error {
	sc := scFrom(ctx)
	var obj map[string]interface{}
	if err := json.Unmarshal([]byte(sc.output), &obj); err != nil {
		return fmt.Errorf("output is not valid JSON: %v\nOutput:\n%s",
			err, truncateOutput(sc.output))
	}
	return nil
}

func jsonOutputShouldHaveField(ctx context.Context, field string) error {
	sc := scFrom(ctx)
	var obj map[string]interface{}
	if err := json.Unmarshal([]byte(sc.output), &obj); err != nil {
		return fmt.Errorf("output is not valid JSON: %v", err)
	}
	if _, exists := obj[field]; !exists {
		return fmt.Errorf("JSON output missing field %q\nJSON:\n%s",
			field, truncateOutput(sc.output))
	}
	return nil
}

// ---------------------------------------------------------------------------
// File sync step definitions
// ---------------------------------------------------------------------------

func iCreateLargeFile(ctx context.Context, sizeMB int, filename string) (context.Context, error) {
	sc := scFrom(ctx)
	path := filepath.Join(sc.projectDir, filename)
	// Create file by writing zeros (faster than /dev/urandom for tests)
	f, err := os.Create(path)
	if err != nil {
		return ctx, err
	}
	defer f.Close()

	// Write in chunks to avoid memory issues
	chunk := make([]byte, 1024*1024) // 1MB chunks
	for i := 0; i < sizeMB; i++ {
		if _, err := f.Write(chunk); err != nil {
			return ctx, err
		}
	}
	return ctx, nil
}

func iCreateGitignoreWithPatterns(ctx context.Context, pattern1, pattern2 string) (context.Context, error) {
	sc := scFrom(ctx)
	content := fmt.Sprintf("%s\n%s\n", pattern1, pattern2)
	return ctx, writeTestFile(sc.projectDir, ".gitignore", content)
}

func iCreateFileWithContent(ctx context.Context, filename, content string) (context.Context, error) {
	sc := scFrom(ctx)
	return ctx, writeTestFile(sc.projectDir, filename, content)
}

func iCreateSymlink(ctx context.Context, linkname, target string) (context.Context, error) {
	sc := scFrom(ctx)
	linkPath := filepath.Join(sc.projectDir, linkname)
	targetPath := target // Relative path
	return ctx, os.Symlink(targetPath, linkPath)
}

func iCreateFileWithMode(ctx context.Context, filename string, mode int) (context.Context, error) {
	sc := scFrom(ctx)
	path := filepath.Join(sc.projectDir, filename)
	if err := writeTestFile(sc.projectDir, filename, "#!/bin/bash\necho test\n"); err != nil {
		return ctx, err
	}
	return ctx, os.Chmod(path, os.FileMode(mode))
}

func iCreateBinaryFile(ctx context.Context, filename string) (context.Context, error) {
	sc := scFrom(ctx)
	path := filepath.Join(sc.projectDir, filename)
	// Create a simple binary file with varied bytes
	data := make([]byte, 1024)
	for i := range data {
		data[i] = byte(i % 256)
	}
	return ctx, os.WriteFile(path, data, 0o644)
}

func iComputeChecksumAs(ctx context.Context, filename, varname string) (context.Context, error) {
	sc := scFrom(ctx)
	path := filepath.Join(sc.projectDir, filename)
	data, err := os.ReadFile(path)
	if err != nil {
		return ctx, err
	}

	// Compute both SHA256 and MD5 checksums
	sha256sum := fmt.Sprintf("%x", sha256.Sum256(data))
	md5sum := fmt.Sprintf("%x", md5.Sum(data))

	// Store in scenario context
	if sc.envOverrides == nil {
		sc.envOverrides = make(map[string]string)
	}
	sc.envOverrides[varname] = sha256sum
	sc.envOverrides["local_checksum"] = sha256sum
	sc.envOverrides["local_md5"] = md5sum
	return ctx, nil
}

func outputShouldContainStoredChecksum(ctx context.Context) error {
	sc := scFrom(ctx)
	stored := sc.envOverrides["local_checksum"]
	if stored == "" {
		stored = sc.envOverrides["local_md5"]
	}
	if stored == "" {
		return fmt.Errorf("no stored checksum found in context")
	}
	// Only check first 8 characters for partial match (checksums are long)
	if len(stored) > 8 {
		stored = stored[:8]
	}
	if !strings.Contains(sc.output, stored) {
		return fmt.Errorf("output does not contain stored checksum prefix %q\nOutput:\n%s",
			stored, truncateOutput(sc.output))
	}
	return nil
}

func iCreateNestedDirectories(ctx context.Context, levels int) (context.Context, error) {
	sc := scFrom(ctx)
	// Create a/b/c/d/e/f/g/h/i/j structure
	dirs := []string{"a", "b", "c", "d", "e", "f", "g", "h", "i", "j"}
	path := sc.projectDir
	for i := 0; i < levels && i < len(dirs); i++ {
		path = filepath.Join(path, dirs[i])
	}
	return ctx, os.MkdirAll(path, 0o755)
}

// ---------------------------------------------------------------------------
// Status test step definitions
// ---------------------------------------------------------------------------

func iCorruptStateFile(ctx context.Context, filepath_ string) (context.Context, error) {
	sc := scFrom(ctx)
	path := filepath.Join(sc.projectDir, filepath_)
	// Ensure directory exists
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return ctx, err
	}
	// Write invalid JSON
	return ctx, os.WriteFile(path, []byte("CORRUPTED_INVALID_JSON{{{"), 0o644)
}
