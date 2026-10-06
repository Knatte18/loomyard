package shedadapters

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// reportName is a small test convention naming a stand-in round-report artifact, independent of
// any real round producer's own naming.
func reportName(round int) string {
	return fmt.Sprintf("round-%d-report.md", round)
}

func writeReport(t *testing.T, dir string, round int) {
	t.Helper()
	path := filepath.Join(dir, reportName(round))
	if err := os.WriteFile(path, []byte("report"), 0o644); err != nil {
		t.Fatalf("WriteFile(%q) = %v; want nil", path, err)
	}
}

//testtiming:keep pins the round scan: the highest present round, zero for an empty run dir, and that a gap stops the scan
func TestResolveRound(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		present []int
		want    int
	}{
		{"empty run dir returns zero", nil, 0},
		{"only round one present", []int{1}, 1},
		{"rounds one through three present", []int{1, 2, 3}, 3},
		{"a gap stops the scan before later rounds", []int{1, 3}, 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			for _, round := range tt.present {
				writeReport(t, dir, round)
			}

			got, err := ResolveRound(dir, reportName)
			if err != nil {
				t.Fatalf("ResolveRound(%q, ...) = _, %v; want nil error", dir, err)
			}
			if got != tt.want {
				t.Errorf("ResolveRound(%q, ...) = %d; want %d", dir, got, tt.want)
			}
		})
	}
}

func TestResolveRound_MissingRunDirReturnsError(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "does-not-exist")

	got, err := ResolveRound(dir, reportName)
	if err == nil {
		t.Fatalf("ResolveRound(%q, ...) = %d, nil; want a non-nil error", dir, got)
	}
}

func TestResolveRound_NonDirectoryReturnsError(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "not-a-dir")
	if err := os.WriteFile(path, []byte("x"), 0o644); err != nil {
		t.Fatalf("WriteFile(%q) = %v; want nil", path, err)
	}

	got, err := ResolveRound(path, reportName)
	if err == nil {
		t.Fatalf("ResolveRound(%q, ...) = %d, nil; want a non-nil error", path, got)
	}
}

// TestResolveRound_NonNotExistStatErrorIsReturned arranges a stat failure
// that is not fs.ErrNotExist (a permission failure from an unsearchable run
// dir), asserting that it surfaces as an error rather than being read as
// absence -- which would otherwise truncate the scan or re-seed an
// already-judged segment.
// Skipped when running as root (root bypasses directory permission checks)
// or on a platform where chmod cannot be used to arrange this.
func TestResolveRound_NonNotExistStatErrorIsReturned(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("chmod-based permission denial is not portable to windows")
	}
	if os.Geteuid() == 0 {
		t.Skip("running as root bypasses directory permission checks")
	}

	parent := t.TempDir()
	dir := filepath.Join(parent, "run")
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatalf("Mkdir(%q) = %v; want nil", dir, err)
	}
	writeReport(t, dir, 1)

	// Remove the directory's search (execute) bit so stat-ing a file inside
	// it fails with a permission error rather than not-exist.
	if err := os.Chmod(dir, 0o000); err != nil {
		t.Fatalf("Chmod(%q, 0o000) = %v; want nil", dir, err)
	}
	t.Cleanup(func() {
		_ = os.Chmod(dir, 0o755)
	})

	got, err := ResolveRound(dir, reportName)
	if err == nil {
		t.Fatalf("ResolveRound(%q, ...) = %d, nil; want a non-nil error from the permission-denied stat", dir, got)
	}
}

//testtiming:keep pins the on-disk filename spellings the producers and stencils name, which no behavior test asserts byte for byte
func TestRoundPathHelpers_PinExactFilenameSpellings(t *testing.T) {
	tests := []struct {
		name string
		fn   func(runDir string, round int) string
		want string
	}{
		{name: "verdictPath", fn: verdictPath, want: "round-5-bouncer-verdict.md"},
		{name: "ledgerPath", fn: ledgerPath, want: "round-5-bouncer-ledger.md"},
		{name: "focusPath", fn: focusPath, want: "round-5-focus.md"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.fn("/run", 5)
			want := filepath.Join("/run", tt.want)
			if got != want {
				t.Errorf("%s(%q, 5) = %q; want %q", tt.name, "/run", got, want)
			}
		})
	}
}
