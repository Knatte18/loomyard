// guardstep.go derives the guard step every card gate ends with: the root module's `//lyx:guard` tests, found by a walk of the worktree that spawns no process.

package websterengine

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"

	"github.com/Knatte18/loomyard/internal/impactset"
)

// guardTests is what one render of the card gates found in the root module's test files.
// The zero value renders no guard step.
type guardTests struct {
	// names maps a worktree-relative guard package directory, "." for the root, to its sorted guard test names.
	// It holds only directories with at least one.
	names map[string][]string

	// goFiles maps each directory of names to the worktree-relative, slash-separated paths of the `.go` files it directly holds.
	goFiles map[string][]string

	// scanErr is the error the guard scan returned for a test file's content, nil when it accepted every test file.
	scanErr *impactset.GuardScanError
}

// scanGuardTests finds the root module's `//lyx:guard` tests under worktree.
// A worktree with no `go.mod` at its root yields the zero value.
// The walk skips, below the root, a directory named testdata or vendor, one whose name begins with `_` or `.`, and one holding a `go.mod` with its whole subtree.
// A path the walk cannot read, or a test file that cannot be opened, is an error naming it.
// A guard scan failure sitting in a test file's content is kept in scanErr with no error returned.
func scanGuardTests(worktree string) (guardTests, error) {
	if _, err := os.Stat(filepath.Join(worktree, "go.mod")); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return guardTests{}, nil
		}
		return guardTests{}, walkError(filepath.Join(worktree, "go.mod"), err)
	}

	goFiles := map[string][]string{}
	testDirs := map[string]bool{}
	walk := func(current string, entry fs.DirEntry, err error) error {
		if err != nil {
			return walkError(current, err)
		}
		if entry.IsDir() {
			if current == worktree {
				return nil
			}
			return skipGuardDir(current, entry.Name())
		}
		if !strings.HasSuffix(entry.Name(), ".go") {
			return nil
		}
		relative, err := filepath.Rel(worktree, current)
		if err != nil {
			return walkError(current, err)
		}
		relative = filepath.ToSlash(relative)
		dir := path.Dir(relative)
		goFiles[dir] = append(goFiles[dir], relative)
		if !strings.HasSuffix(entry.Name(), "_test.go") {
			return nil
		}
		file, err := os.Open(current)
		if err != nil {
			return walkError(current, err)
		}
		if err := file.Close(); err != nil {
			return walkError(current, err)
		}
		testDirs[dir] = true
		return nil
	}
	if err := filepath.WalkDir(worktree, walk); err != nil {
		return guardTests{}, err
	}

	dirs := make([]string, 0, len(testDirs))
	for dir := range testDirs {
		dirs = append(dirs, dir)
	}
	slices.Sort(dirs)
	found, err := impactset.FindGuardTests(worktree, dirs)
	if err != nil {
		var scanErr *impactset.GuardScanError
		if errors.As(err, &scanErr) {
			return guardTests{scanErr: scanErr}, nil
		}
		return guardTests{}, fmt.Errorf("webster: scan the worktree for guard tests: %w", err)
	}

	guards := guardTests{names: map[string][]string{}, goFiles: map[string][]string{}}
	for dir, names := range found {
		if len(names) == 0 {
			continue
		}
		guards.names[dir] = names
		guards.goFiles[dir] = goFiles[dir]
	}
	return guards, nil
}

// skipGuardDir returns filepath.SkipDir for a directory below the root that the walk leaves out, and nil for one it enters.
func skipGuardDir(current, name string) error {
	if name == "testdata" || name == "vendor" || strings.HasPrefix(name, "_") || strings.HasPrefix(name, ".") {
		return filepath.SkipDir
	}
	if _, err := os.Stat(filepath.Join(current, "go.mod")); err == nil {
		return filepath.SkipDir
	} else if !errors.Is(err, fs.ErrNotExist) {
		return walkError(current, err)
	}
	return nil
}

// walkError wraps a failure to read target while walking the worktree for guard tests.
func walkError(target string, err error) error {
	return fmt.Errorf("webster: walk the worktree for guard tests: %s: %w", target, err)
}
