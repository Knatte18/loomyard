package mergeresolve

import (
	"os"
	"path/filepath"
	"testing"
)

// TestScanUnresolved_LineStartMarkers covers each of the three conflict-marker prefixes appearing
// at the start of a line: every such file is reported unresolved.
func TestScanUnresolved_LineStartMarkers(t *testing.T) {
	tests := []struct {
		name    string
		content string
	}{
		{"ours", "<<<<<<< HEAD\nmine\n"},
		{"middle", "=======\n"},
		{"theirs", ">>>>>>> feature\ntheirs\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			if err := os.WriteFile(filepath.Join(dir, "f.txt"), []byte(tt.content), 0o644); err != nil {
				t.Fatalf("WriteFile: %v", err)
			}

			unresolved, err := scanUnresolved(dir, []string{"f.txt"})
			if err != nil {
				t.Fatalf("scanUnresolved() error = %v; want nil", err)
			}
			if len(unresolved) != 1 || unresolved[0] != "f.txt" {
				t.Errorf("scanUnresolved() = %v; want [\"f.txt\"]", unresolved)
			}
		})
	}
}

// TestScanUnresolved_MidLineMarkerPasses verifies that a marker string appearing mid-line, never at
// the very start of a line, does not fail the scan — the match is line-anchored, not a substring
// match anywhere in a line.
func TestScanUnresolved_MidLineMarkerPasses(t *testing.T) {
	dir := t.TempDir()
	content := "note: this line mentions <<<<<<< HEAD only in passing\nordinary content\n"
	if err := os.WriteFile(filepath.Join(dir, "f.txt"), []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	unresolved, err := scanUnresolved(dir, []string{"f.txt"})
	if err != nil {
		t.Fatalf("scanUnresolved() error = %v; want nil", err)
	}
	if len(unresolved) != 0 {
		t.Errorf("scanUnresolved() = %v; want empty (mid-line marker text must pass)", unresolved)
	}
}

// TestScanUnresolved_MissingFileIsResolvedByDeletion verifies a path whose file no longer exists is
// treated as resolved by deletion, not reported unresolved and not an error.
func TestScanUnresolved_MissingFileIsResolvedByDeletion(t *testing.T) {
	dir := t.TempDir()

	unresolved, err := scanUnresolved(dir, []string{"gone.txt"})
	if err != nil {
		t.Fatalf("scanUnresolved() error = %v; want nil", err)
	}
	if len(unresolved) != 0 {
		t.Errorf("scanUnresolved() = %v; want empty for a deleted path", unresolved)
	}
}

// TestScanUnresolved_ReadErrorIsGenuineFailure verifies a read error that is not a not-exist error
// (here, attempting to read a directory as a file) is returned as a genuine, distinct failure rather
// than treated as resolved-by-deletion.
func TestScanUnresolved_ReadErrorIsGenuineFailure(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "isdir"), 0o755); err != nil {
		t.Fatalf("Mkdir: %v", err)
	}

	_, err := scanUnresolved(dir, []string{"isdir"})
	if err == nil {
		t.Fatal("scanUnresolved() error = nil; want non-nil for a read error distinct from deletion")
	}
}
