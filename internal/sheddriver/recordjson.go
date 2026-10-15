// recordjson.go declares the indented-JSON writer and reader the tackler's attempt, envelope and marker records share.

package sheddriver

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

// writeRecordJSON creates path's directory and writes v there as indented JSON.
func writeRecordJSON(path string, v any) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return fmt.Errorf("sheddriver: encode record %q: %w", path, err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("sheddriver: create record directory: %w", err)
	}
	if err := os.WriteFile(path, append(data, '\n'), 0o644); err != nil {
		return fmt.Errorf("sheddriver: write record %q: %w", path, err)
	}
	return nil
}

// readRecordJSON decodes the JSON file at path into v; an absent file returns found false.
func readRecordJSON(path string, v any) (found bool, err error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("sheddriver: read record %q: %w", path, err)
	}
	if err := json.Unmarshal(data, v); err != nil {
		return false, fmt.Errorf("sheddriver: parse record %q: %w", path, err)
	}
	return true, nil
}
