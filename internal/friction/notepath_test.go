// notepath_test.go covers NotePath against real files in t.TempDir(), not a stubbed stat.
// The guarantee is about the filesystem, so the tests check it there.
// Every test here is in tier 0.
// It uses only os.WriteFile and t.TempDir, and spawns nothing.

package friction

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// touch creates an empty file at path, failing the test on any error.
func touch(t *testing.T, path string) {
	t.Helper()
	if err := os.WriteFile(path, nil, 0o644); err != nil {
		t.Fatalf("WriteFile(%q) = %v; want nil", path, err)
	}
}

// TestNotePath_NonClobbering pins the base case and the non-clobbering guarantee.
// Repeated calls with the same id return "id.md", "id-2.md" and "id-3.md" in turn, skipping notes on disk.
// A gap (id.md and id-3.md present, id-2.md absent) resolves to the first free name, id-2.md, not the highest plus one.
func TestNotePath_NonClobbering(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	t.Run("SequentialReinvocationSkipsExisting", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		for _, name := range []string{"spawn.md", "spawn-2.md", "spawn-3.md"} {
			want := filepath.Join(dir, name)
			got := NotePath(dir, "spawn")
			if got != want {
				t.Fatalf("NotePath(%q, %q) = %q; want %q", dir, "spawn", got, want)
			}
			touch(t, got)
		}
	})

	t.Run("GapResolvesToFirstFree", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		touch(t, filepath.Join(dir, "spawn.md"))
		touch(t, filepath.Join(dir, "spawn-3.md"))

		want := filepath.Join(dir, "spawn-2.md")
		if got := NotePath(dir, "spawn"); got != want {
			t.Errorf("NotePath(gap) = %q; want %q", got, want)
		}
	})
}

// TestNotePath_ReturnsEmpty pins every guard that returns "".
// An empty frictionDir is the off state, and NotePath returns "" for it, so no boolean is needed.
// The id sanitization rejects an empty id and an id with a path separator.
// It also rejects an id containing "..", an id equal to "." or "..", and an id whose id+".md" equals ReportFileName.
func TestNotePath_ReturnsEmpty(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	dir := t.TempDir()
	reportStem := ReportFileName[:len(ReportFileName)-len(".md")]

	tests := []struct {
		name string
		dir  string
		id   string
	}{
		{"EmptyFrictionDir", "", "some-id"},
		{"EmptyID", dir, ""},
		{"ForwardSlash", dir, "sub/id"},
		{"Backslash", dir, `sub\id`},
		{"DotDotElement", dir, "sub/../id"},
		{"SingleDot", dir, "."},
		{"DoubleDot", dir, ".."},
		{"CollidesWithReportFileName", dir, reportStem},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := NotePath(tt.dir, tt.id); got != "" {
				t.Errorf("NotePath(%q, %q) = %q; want \"\"", tt.dir, tt.id, got)
			}
		})
	}
}
