// targetsnapshot.go implements TargetSnapshot, the seam that names a review round's target by an id.
// It also lists the paths that changed since an earlier id.
// A segment's review producer and its Bouncer both hold one, so each reads the target the same way.

package shedadapters

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"sort"

	"github.com/Knatte18/loomyard/internal/burlerengine"
	"github.com/Knatte18/loomyard/internal/logger"
)

// TargetSnapshot names the target of a review round by an id and lists what changed since an earlier id.
// Overlay scope reads the target files under WorktreeRoot and keeps a copy of them per id under CacheDir.
// Source scope asks the two closures, which answer from the code checkout's git trees.
// A closure left nil makes source scope answer an error, or false from ChangedSince.
// The zero value is not usable; Scope must name one of the two scopes.
type TargetSnapshot struct {
	// Scope selects overlay or source behavior.
	Scope burlerengine.FixScope
	// WorktreeRoot is the absolute root the overlay target paths are relative to.
	WorktreeRoot string
	// Paths are the worktree-relative target paths, files or directories.
	Paths []string
	// CacheDir is the absolute directory holding one overlay copy per id.
	CacheDir string
	// SourceTreeID returns the id of the checkout's current tree.
	SourceTreeID func() (string, error)
	// SourceChanges returns the paths that differ between two tree ids.
	SourceChanges func(fromID, toID string) ([]string, error)
}

// ID returns the id of the target as it stands now.
// Overlay scope returns the hex SHA-256 over every target file's path and bytes, in path order;
// a target path that does not exist is skipped.
// Source scope returns the checkout's tree id.
func (s TargetSnapshot) ID() (string, error) {
	switch s.Scope {
	case burlerengine.FixScopeOverlay:
		return s.overlayID()
	case burlerengine.FixScopeSource:
		return s.sourceID()
	}
	return "", fmt.Errorf("shedadapters: target snapshot scope %q is neither overlay nor source", s.Scope)
}

// Take returns the target's id as ID does and, in overlay scope, keeps a copy of the target files under CacheDir/<id>.
// An existing copy for the id is left as it is.
func (s TargetSnapshot) Take() (string, error) {
	id, err := s.ID()
	if err != nil || s.Scope != burlerengine.FixScopeOverlay {
		return id, err
	}
	return id, s.copyOverlay(id)
}

// ChangedSince returns the sorted paths that differ between the target at fromID and the target now.
// It reports false when the answer is unknown: a missing copy, a read error, an unwired closure or a closure error.
// A false answer is logged at Warn and never an error, since callers fall back to treating the whole target as changed.
func (s TargetSnapshot) ChangedSince(fromID string) ([]string, bool) {
	var changed []string
	var err error
	switch s.Scope {
	case burlerengine.FixScopeOverlay:
		changed, err = s.overlayChangedSince(fromID)
	case burlerengine.FixScopeSource:
		changed, err = s.sourceChangedSince(fromID)
	default:
		err = fmt.Errorf("target snapshot scope %q is neither overlay nor source", s.Scope)
	}
	if err != nil {
		logger.Warn("shedadapters: target snapshot could not list the changed paths", "scope", s.Scope, "fromID", fromID, "error", err)
		return nil, false
	}
	return changed, true
}

// sourceID returns the checkout's tree id through SourceTreeID.
func (s TargetSnapshot) sourceID() (string, error) {
	if s.SourceTreeID == nil {
		return "", fmt.Errorf("shedadapters: source target snapshot has no tree id reader")
	}
	return s.SourceTreeID()
}

// sourceChangedSince lists the paths between fromID and the checkout's current tree id.
func (s TargetSnapshot) sourceChangedSince(fromID string) ([]string, error) {
	if s.SourceChanges == nil {
		return nil, fmt.Errorf("source target snapshot has no changed-path reader")
	}
	current, err := s.sourceID()
	if err != nil {
		return nil, err
	}
	return s.SourceChanges(fromID, current)
}

// overlayID hashes every target file's relative path and bytes in path order.
func (s TargetSnapshot) overlayID() (string, error) {
	files, err := listTargetFiles(s.WorktreeRoot, s.Paths)
	if err != nil {
		return "", err
	}
	hash := sha256.New()
	for _, rel := range files {
		content, err := os.ReadFile(filepath.Join(s.WorktreeRoot, filepath.FromSlash(rel)))
		if err != nil {
			return "", fmt.Errorf("shedadapters: read target file %q: %w", rel, err)
		}
		hash.Write([]byte(rel))
		hash.Write([]byte{0})
		hash.Write(content)
		hash.Write([]byte{0})
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

// copyOverlay copies every target file to CacheDir/<id>/<relative path>, unless that directory exists already.
// The copy is built beside its destination and renamed into place, so a crash never leaves a half copy under the id.
func (s TargetSnapshot) copyOverlay(id string) error {
	destination := filepath.Join(s.CacheDir, id)
	if _, err := os.Stat(destination); err == nil {
		return nil
	}
	files, err := listTargetFiles(s.WorktreeRoot, s.Paths)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(s.CacheDir, 0o755); err != nil {
		return fmt.Errorf("shedadapters: create the target snapshot cache %q: %w", s.CacheDir, err)
	}
	staging, err := os.MkdirTemp(s.CacheDir, ".take-*")
	if err != nil {
		return fmt.Errorf("shedadapters: create a target snapshot staging directory in %q: %w", s.CacheDir, err)
	}
	for _, rel := range files {
		content, err := os.ReadFile(filepath.Join(s.WorktreeRoot, filepath.FromSlash(rel)))
		if err != nil {
			_ = os.RemoveAll(staging)
			return fmt.Errorf("shedadapters: read target file %q: %w", rel, err)
		}
		target := filepath.Join(staging, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			_ = os.RemoveAll(staging)
			return fmt.Errorf("shedadapters: create the snapshot directory for %q: %w", rel, err)
		}
		if err := os.WriteFile(target, content, 0o644); err != nil {
			_ = os.RemoveAll(staging)
			return fmt.Errorf("shedadapters: copy target file %q: %w", rel, err)
		}
	}
	if err := os.Rename(staging, destination); err != nil {
		_ = os.RemoveAll(staging)
		return fmt.Errorf("shedadapters: place the target snapshot %q: %w", destination, err)
	}
	return nil
}

// overlayChangedSince compares the copy under CacheDir/<fromID> with the target files now.
func (s TargetSnapshot) overlayChangedSince(fromID string) ([]string, error) {
	cacheRoot := filepath.Join(s.CacheDir, fromID)
	if _, err := os.Stat(cacheRoot); err != nil {
		return nil, fmt.Errorf("no target snapshot copy for %q: %w", fromID, err)
	}
	before, err := listTargetFiles(cacheRoot, []string{"."})
	if err != nil {
		return nil, err
	}
	after, err := listTargetFiles(s.WorktreeRoot, s.Paths)
	if err != nil {
		return nil, err
	}
	changed := map[string]bool{}
	for _, rel := range before {
		if !slices.Contains(after, rel) {
			changed[rel] = true
		}
	}
	for _, rel := range after {
		if !slices.Contains(before, rel) {
			changed[rel] = true
			continue
		}
		same, err := sameFileBytes(filepath.Join(cacheRoot, filepath.FromSlash(rel)), filepath.Join(s.WorktreeRoot, filepath.FromSlash(rel)))
		if err != nil {
			return nil, err
		}
		if !same {
			changed[rel] = true
		}
	}
	paths := make([]string, 0, len(changed))
	for rel := range changed {
		paths = append(paths, rel)
	}
	sort.Strings(paths)
	return paths, nil
}

// sameFileBytes reports whether the two files hold the same bytes.
func sameFileBytes(left, right string) (bool, error) {
	leftBytes, err := os.ReadFile(left)
	if err != nil {
		return false, fmt.Errorf("shedadapters: read %q: %w", left, err)
	}
	rightBytes, err := os.ReadFile(right)
	if err != nil {
		return false, fmt.Errorf("shedadapters: read %q: %w", right, err)
	}
	return bytes.Equal(leftBytes, rightBytes), nil
}

// listTargetFiles returns the sorted, slash-separated root-relative paths of the regular files under paths.
// A path that does not exist is skipped, and a directory contributes every regular file below it.
func listTargetFiles(root string, paths []string) ([]string, error) {
	seen := map[string]bool{}
	for _, path := range paths {
		start := filepath.Join(root, filepath.FromSlash(path))
		if _, err := os.Lstat(start); os.IsNotExist(err) {
			continue
		}
		err := filepath.WalkDir(start, func(current string, entry fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if !entry.Type().IsRegular() {
				return nil
			}
			rel, err := filepath.Rel(root, current)
			if err != nil {
				return err
			}
			seen[filepath.ToSlash(rel)] = true
			return nil
		})
		if err != nil {
			return nil, fmt.Errorf("shedadapters: list the target files under %q: %w", start, err)
		}
	}
	files := make([]string, 0, len(seen))
	for rel := range seen {
		files = append(files, rel)
	}
	sort.Strings(files)
	return files, nil
}
