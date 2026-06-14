package worktree

import (
	"fmt"
	"os"
	"path/filepath"
)

// removeLinks scans the immediate children of dir and removes any symlinks or
// NTFS junctions found. It returns the count of removed links and the first
// error encountered. Regular files and real subdirectories are left untouched.
// If dir does not exist, returns (0, err) from the os.ReadDir failure.
func removeLinks(dir string) (int, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return 0, err
	}

	count := 0
	for _, entry := range entries {
		fullPath := filepath.Join(dir, entry.Name())
		info, err := os.Lstat(fullPath)
		if err != nil {
			return count, fmt.Errorf("lstat %s: %w", fullPath, err)
		}

		// Check if this is a symlink or junction using bitmask test
		// (handles both POSIX symlinks and Windows junctions, which may report
		// ModeSymlink|ModeIrregular)
		if info.Mode()&os.ModeSymlink != 0 {
			if err := os.Remove(fullPath); err != nil {
				return count, fmt.Errorf("remove link %s: %w", fullPath, err)
			}
			count++
		}
	}

	return count, nil
}
