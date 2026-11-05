// accepted.go — the accepted profile: each accepted test's output kept as it was when the test was accepted, and the profile built from the baseline and those outputs.
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
	"slices"
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

// requireBaselineTest refuses an empty test and a test with no profile file under the baseline directory, naming the test and the directory.
func requireBaselineTest(p Paths, test string) error {
	if test == "" {
		return noBaselineTestRefusal(p, `""`)
	}
	files, err := testFiles(p.BaselineDir, test)
	if err != nil {
		return err
	}
	if len(files) == 0 {
		return noBaselineTestRefusal(p, test)
	}
	return nil
}

// noBaselineTestRefusal is the refusal of an acceptance whose test name, as shown, names no profile file under the baseline directory.
func noBaselineTestRefusal(p Paths, shown string) error {
	return fmt.Errorf("drift accept refused: %s is no test of the baseline, since no file under %s is named for it; way forward: name the test as its profile file under that directory is named, with or without the file's extension", shown, p.BaselineDir)
}

// keepAcceptedOutput moves test's profile files from the accepting directory, where an acceptance capture left them, to the kept outputs, replacing those an earlier acceptance of test kept.
// The new files are moved before the earlier ones they do not replace are removed, so a failure while moving removes no earlier kept file.
func keepAcceptedOutput(p Paths, test string) error {
	staged, err := testFiles(p.AcceptingDir, test)
	if err != nil {
		return err
	}
	if len(staged) == 0 {
		return fmt.Errorf("drift accept refused: no output of %s was captured for this acceptance under %s; way forward: lym drift accept <test> --reason <text>, which captures it first", test, p.AcceptingDir)
	}
	for _, rel := range staged {
		kept := filepath.Join(p.AcceptedTestsDir, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(kept), 0o755); err != nil {
			return fmt.Errorf("drift: create %s: %w", filepath.Dir(kept), err)
		}
		if err := os.Rename(filepath.Join(p.AcceptingDir, filepath.FromSlash(rel)), kept); err != nil {
			return fmt.Errorf("drift: keep the accepted output of %s: %w", test, err)
		}
	}
	return dropAcceptedOutput(p, test, staged)
}

// dropAcceptedOutput removes the output an acceptance of test kept, except the files named in except, which are slash-separated paths relative to the kept outputs' directory; a test with none kept is no error.
func dropAcceptedOutput(p Paths, test string, except []string) error {
	kept, err := testFiles(p.AcceptedTestsDir, test)
	if err != nil {
		return err
	}
	for _, rel := range kept {
		if slices.Contains(except, rel) {
			continue
		}
		if err := os.Remove(filepath.Join(p.AcceptedTestsDir, filepath.FromSlash(rel))); err != nil {
			return fmt.Errorf("drift: drop the accepted output of %s: %w", test, err)
		}
	}
	return nil
}

// buildAcceptedProfile rebuilds the accepted profile directory as the baseline with each accepted test's profile files replaced by the output kept at its acceptance.
// It returns the accepted tests that have no kept output, whose baseline files it leaves out.
func buildAcceptedProfile(p Paths, scope ScopeRecord) (missing []string, err error) {
	if err := os.RemoveAll(p.AcceptedDir); err != nil {
		return nil, fmt.Errorf("drift: empty the accepted profile %s: %w", p.AcceptedDir, err)
	}
	if err := os.CopyFS(p.AcceptedDir, os.DirFS(p.BaselineDir)); err != nil {
		return nil, fmt.Errorf("drift: copy the baseline into the accepted profile %s: %w", p.AcceptedDir, err)
	}
	for _, acceptance := range scope.Accepted {
		baseline, err := testFiles(p.AcceptedDir, acceptance.Test)
		if err != nil {
			return nil, err
		}
		for _, rel := range baseline {
			if err := os.Remove(filepath.Join(p.AcceptedDir, filepath.FromSlash(rel))); err != nil {
				return nil, fmt.Errorf("drift: replace the baseline output of %s: %w", acceptance.Test, err)
			}
		}
		kept, err := testFiles(p.AcceptedTestsDir, acceptance.Test)
		if err != nil {
			return nil, err
		}
		if len(kept) == 0 {
			missing = append(missing, acceptance.Test)
			continue
		}
		for _, rel := range kept {
			if err := copyFile(filepath.Join(p.AcceptedTestsDir, filepath.FromSlash(rel)), filepath.Join(p.AcceptedDir, filepath.FromSlash(rel))); err != nil {
				return nil, err
			}
		}
	}
	return missing, nil
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
