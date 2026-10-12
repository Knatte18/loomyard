// newest.go answers how recently a path, or anything under it, was modified.

package fsx

import (
	"io/fs"
	"os"
	"path/filepath"
	"time"
)

// NewestModTime returns the newest modification time of path and of everything under it.
// A file or a link answers its own modification time; a link is never followed.
// A directory answers the newest time among itself and every entry below it.
// A missing path returns an error satisfying errors.Is(err, fs.ErrNotExist).
func NewestModTime(path string) (time.Time, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return time.Time{}, err
	}
	newest := info.ModTime()
	if !info.IsDir() {
		return newest, nil
	}

	err = filepath.WalkDir(path, func(_ string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		entryInfo, err := entry.Info()
		if err != nil {
			return err
		}
		if entryInfo.ModTime().After(newest) {
			newest = entryInfo.ModTime()
		}
		return nil
	})
	if err != nil {
		return time.Time{}, err
	}
	return newest, nil
}
