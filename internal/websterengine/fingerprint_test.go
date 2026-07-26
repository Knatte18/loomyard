// fingerprint_test.go covers fingerprint's identity properties: identical
// directories fingerprint identically, and a rename, a one-byte content
// edit, or an added batch file each change the result, while non-.md
// entries and subdirectories are ignored entirely. Tier 1: no git, only
// t.TempDir().

package websterengine

import (
	"os"
	"path/filepath"
	"testing"
)

// fingerprintWriteFiles writes every entry of files (keyed by relative
// path) into dir, creating any needed subdirectories.
func fingerprintWriteFiles(t *testing.T, dir string, files map[string]string) {
	t.Helper()

	for name, content := range files {
		path := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("mkdir for %s: %v", name, err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
}

func TestFingerprint_IdenticalDirsMatch(t *testing.T) {
	t.Parallel()

	files := map[string]string{
		"00-overview.md": "overview content",
		"01-first.md":    "first content",
	}

	dirA := t.TempDir()
	dirB := t.TempDir()
	fingerprintWriteFiles(t, dirA, files)
	fingerprintWriteFiles(t, dirB, files)

	fpA, err := fingerprint(dirA)
	if err != nil {
		t.Fatalf("fingerprint(dirA) error = %v; want nil", err)
	}
	fpB, err := fingerprint(dirB)
	if err != nil {
		t.Fatalf("fingerprint(dirB) error = %v; want nil", err)
	}

	if fpA != fpB {
		t.Errorf("fingerprint(dirA) = %q; fingerprint(dirB) = %q; want equal for identical content", fpA, fpB)
	}
}

func TestFingerprint_ChangesOnRename(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	fingerprintWriteFiles(t, dir, map[string]string{"01-first.md": "content"})
	before, err := fingerprint(dir)
	if err != nil {
		t.Fatalf("fingerprint() error = %v; want nil", err)
	}

	if err := os.Rename(filepath.Join(dir, "01-first.md"), filepath.Join(dir, "01-renamed.md")); err != nil {
		t.Fatalf("rename: %v", err)
	}

	after, err := fingerprint(dir)
	if err != nil {
		t.Fatalf("fingerprint() error = %v; want nil", err)
	}

	if before == after {
		t.Errorf("fingerprint() = %q both before and after a rename; want it to change", before)
	}
}

func TestFingerprint_ChangesOnByteEdit(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	fingerprintWriteFiles(t, dir, map[string]string{"01-first.md": "content"})
	before, err := fingerprint(dir)
	if err != nil {
		t.Fatalf("fingerprint() error = %v; want nil", err)
	}

	fingerprintWriteFiles(t, dir, map[string]string{"01-first.md": "contenu"}) // one byte differs

	after, err := fingerprint(dir)
	if err != nil {
		t.Fatalf("fingerprint() error = %v; want nil", err)
	}

	if before == after {
		t.Errorf("fingerprint() = %q both before and after a one-byte edit; want it to change", before)
	}
}

func TestFingerprint_ChangesOnAddedBatchFile(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	fingerprintWriteFiles(t, dir, map[string]string{"01-first.md": "content"})
	before, err := fingerprint(dir)
	if err != nil {
		t.Fatalf("fingerprint() error = %v; want nil", err)
	}

	fingerprintWriteFiles(t, dir, map[string]string{"02-second.md": "more content"})

	after, err := fingerprint(dir)
	if err != nil {
		t.Fatalf("fingerprint() error = %v; want nil", err)
	}

	if before == after {
		t.Errorf("fingerprint() = %q both before and after adding a batch file; want it to change", before)
	}
}

func TestFingerprint_IgnoresNonMarkdownAndSubdirs(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	fingerprintWriteFiles(t, dir, map[string]string{"01-first.md": "content"})
	before, err := fingerprint(dir)
	if err != nil {
		t.Fatalf("fingerprint() error = %v; want nil", err)
	}

	// Add a non-.md file and a subdirectory containing a .md file; neither
	// should affect the fingerprint since only top-level *.md files count.
	fingerprintWriteFiles(t, dir, map[string]string{
		"notes.txt":           "ignored",
		"reports/report-1.md": "also ignored: this is inside a subdirectory",
	})

	after, err := fingerprint(dir)
	if err != nil {
		t.Fatalf("fingerprint() error = %v; want nil", err)
	}

	if before != after {
		t.Errorf("fingerprint() changed after adding a non-.md file and a subdirectory .md file; want unchanged (got %q, want %q)", after, before)
	}
}
