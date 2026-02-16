// Package dist provides version and distribution utilities.
package dist

import (
	"os"
	"strings"
)

// ReadVersionFile reads the version from version.txt at the given path.
// Returns the trimmed version string or an error.
func ReadVersionFile(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(data)), nil
}
