// notepath_test.go covers NotePath against a real t.TempDir() with real files, never a stubbed stat,
// since the guarantee is about the filesystem.
// Every test here is untagged Tier 1: it uses only os.WriteFile inside a t.TempDir() and t.TempDir
// itself, and spawns nothing.

package friction

import (
	"os"
	"path/filepath"
	"testing"
)

// touch creates an empty file at path, failing the test on any error.
func touch(t *testing.T, path string) {
	t.Helper()
	if err := os.WriteFile(path, nil, 0o644); err != nil {
		t.Fatalf("WriteFile(%q) = %v; want nil", path, err)
	}
}

// TestNotePath_NonClobbering pins the base case and the non-clobbering guarantee.
// A valid id against an empty directory yields the expected join, and calls with the same id against a directory where the earlier notes now exist yield "id.md", "id-2.md" and "id-3.md" in turn.
// A gap (id.md and id-3.md present, id-2.md absent) resolves to the first free name, id-2.md, not the highest plus one.
func TestNotePath_NonClobbering(t *testing.T) {
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
// An empty frictionDir is the off-state guard, so Tier 2's off state composes through NotePath with no boolean anywhere.
// An empty id, an id containing a path separator, an id containing "..", an id equal to "." or "..", and an id whose id+".md" equals ReportFileName are the id-sanitization rules.
func TestNotePath_ReturnsEmpty(t *testing.T) {
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
