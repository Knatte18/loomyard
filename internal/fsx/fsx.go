// fsx.go implements filesystem-safety primitives for atomic writes and path validation.
//
// This package provides path guards to prevent escapes (empty, absolute, parent directory
// references) and atomic file writes via temp-file + rename, ensuring that concurrent readers are
// not caught mid-write.

package fsx

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// PathError represents an invalid or unsafe path.
type PathError string

// Error returns the error message.
func (e PathError) Error() string {
	return string(e)
}

// PathGuard validates a relative path, rejecting empty, absolute, and parent-directory-reference
// paths.
func PathGuard(relPath string) error {
	if relPath == "" {
		return PathError("empty path")
	}

	if filepath.IsAbs(relPath) || (len(relPath) > 0 && relPath[0] == '/') {
		return PathError("absolute path not allowed")
	}

	if len(relPath) > 1 && relPath[1] == ':' {
		return PathError("absolute path not allowed")
	}

	parts := strings.FieldsFunc(relPath, func(r rune) bool {
		return r == '\\' || r == '/'
	})
	for _, c := range parts {
		if c == ".." {
			return PathError("parent directory reference not allowed")
		}
	}

	return nil
}

// AtomicWriteBytes writes data to an absolute file path atomically via a temp file and rename,
// ensuring concurrent readers never see partial writes.
func AtomicWriteBytes(absPath string, data []byte) error {
	dir := filepath.Dir(absPath)

	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("mkdir: %w", err)
	}

	tmpFile, err := os.CreateTemp(dir, ".tmp-")
	if err != nil {
		return fmt.Errorf("create temp: %w", err)
	}
	tmpPath := tmpFile.Name()
	defer os.Remove(tmpPath) // cleanup on error

	if _, err := tmpFile.Write(data); err != nil {
		tmpFile.Close()
		return fmt.Errorf("write: %w", err)
	}
	if err := tmpFile.Close(); err != nil {
		return fmt.Errorf("close: %w", err)
	}

	if err := os.Rename(tmpPath, absPath); err != nil {
		return fmt.Errorf("rename: %w", err)
	}
	return nil
}

// AtomicWrite writes content to dir/relPath atomically after validating the relative path via
// PathGuard.
func AtomicWrite(dir, relPath, content string) error {
	if err := PathGuard(relPath); err != nil {
		return err
	}

	return AtomicWriteBytes(filepath.Join(dir, relPath), []byte(content))
}
