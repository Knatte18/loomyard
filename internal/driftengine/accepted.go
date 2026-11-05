// accepted.go — the profile-file naming rule and the file copy the judge snapshot uses: which files under a profile directory hold one test's profile, and a copy of one file.
//
// A test is named by its profile files: a file under a profile directory holds a test's profile when its path relative to the directory, with or without its last extension, is the test's name; a dotfile has no extension to drop, so its whole name is the test's name.

package driftengine

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
)

// testFiles returns the slash-separated paths, relative to dir, of the regular files under dir that hold test's profile, and none when dir does not exist.
func testFiles(dir, test string) ([]string, error) {
	if _, err := os.Stat(dir); errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	var files []string
	err := filepath.WalkDir(dir, func(full string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !entry.Type().IsRegular() {
			return nil
		}
		rel, err := filepath.Rel(dir, full)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		base := path.Base(rel)
		ext := path.Ext(base)
		// A dotfile's whole name is its extension, so it has no stem to be named by.
		if rel == test || (ext != "" && ext != base && rel[:len(rel)-len(ext)] == test) {
			files = append(files, rel)
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("drift: read the profile files under %s: %w", dir, err)
	}
	return files, nil
}

// copyFile copies the file at from to to, creating to's directory.
func copyFile(from, to string) error {
	data, err := os.ReadFile(from)
	if err != nil {
		return fmt.Errorf("drift: read %s: %w", from, err)
	}
	if err := os.MkdirAll(filepath.Dir(to), 0o755); err != nil {
		return fmt.Errorf("drift: create %s: %w", filepath.Dir(to), err)
	}
	if err := os.WriteFile(to, data, 0o644); err != nil {
		return fmt.Errorf("drift: write %s: %w", to, err)
	}
	return nil
}
