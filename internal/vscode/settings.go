// settings.go reads the prime's .vscode/settings.json for splicing into a workspace file.

package vscode

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

// ReadSettings returns the bytes of <dir>/.vscode/settings.json,
// the file WriteConfig writes under its own dir.
// A missing file returns nil, nil, which BuildWorkspace turns into {};
// any other read error is returned wrapped with the path.
func ReadSettings(dir string) ([]byte, error) {
	path := filepath.Join(dir, ".vscode", "settings.json")
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	return data, nil
}
