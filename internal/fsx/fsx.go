// fsx.go implements filesystem-safety primitives for atomic writes and path validation.
//
// This package provides path guards to prevent escapes (empty, absolute, parent
// directory references) and atomic file writes via temp-file + rename, ensuring
// that concurrent readers are not caught mid-write.

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

// PathGuard validates a relative path for filesystem operations.
// It rejects empty paths, absolute paths (both Unix and Windows style), and
// any path containing ".." components, preventing directory-escape attacks.
// It is used to gate untrusted path inputs before writing.
func PathGuard(relPath string) error {
	if relPath == "" {
		return PathError("empty path")
	}

	// Check for absolute paths (both Windows and Unix styles)
	if filepath.IsAbs(relPath) || (len(relPath) > 0 && relPath[0] == '/') {
		return PathError("absolute path not allowed")
	}

	// Check for Windows-style absolute paths on non-Windows systems
	if len(relPath) > 1 && relPath[1] == ':' {
		return PathError("absolute path not allowed")
	}

	// Split by both separators to preserve ".." for validation (before cleaning would remove it)
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

// AtomicWriteBytes writes data to an absolute file path atomically.
// It creates missing parent directories, writes to a temporary file via
// os.CreateTemp, then atomically renames the temp file to the target path.
// On error, the temporary file is cleaned up. The rename is the atomic swap;
// concurrent readers are excluded from this instant by external synchronization,
// ensuring they never see partial writes.
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

	// The rename is the atomic swap. Concurrent readers are excluded from this
	// instant by external synchronization (e.g. a swap lock in store.Save / store.Load),
	// so on Windows the rename never loses a sharing-violation race against an open reader.
	if err := os.Rename(tmpPath, absPath); err != nil {
		return fmt.Errorf("rename: %w", err)
	}
	return nil
}

// AtomicWrite writes content to a file atomically using path validation and guarding.
// It validates the relative path using PathGuard (rejecting unsafe paths),
// then writes the content to dir/relPath using AtomicWriteBytes.
// This is the guarded convenience function; callers use it when the relative path
// is untrusted and the absolute base directory is controlled internally.
func AtomicWrite(dir, relPath, content string) error {
	if err := PathGuard(relPath); err != nil {
		return err
	}

	return AtomicWriteBytes(filepath.Join(dir, relPath), []byte(content))
}
