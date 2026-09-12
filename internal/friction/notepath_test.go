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

// TestNotePath_EmptyFrictionDir pins the off-state guard: an empty frictionDir returns "" so Tier
// 2's off state composes through NotePath with no boolean anywhere.
func TestNotePath_EmptyFrictionDir(t *testing.T) {
	if got := NotePath("", "some-id"); got != "" {
		t.Errorf(`NotePath("", "some-id") = %q; want ""`, got)
	}
}

// TestNotePath_ValidIDAgainstEmptyDir pins the base case: a valid id against an empty directory
// yields the expected join.
func TestNotePath_ValidIDAgainstEmptyDir(t *testing.T) {
	dir := t.TempDir()
	want := filepath.Join(dir, "some-id.md")
	if got := NotePath(dir, "some-id"); got != want {
		t.Errorf("NotePath(%q, %q) = %q; want %q", dir, "some-id", got, want)
	}
}

// TestNotePath_SequentialReinvocationSkipsExisting pins the non-clobbering guarantee: two calls with
// the same id against a directory where the first note now exists yield "id.md" then "id-2.md", and a
// third yields "id-3.md".
func TestNotePath_SequentialReinvocationSkipsExisting(t *testing.T) {
	dir := t.TempDir()
	id := "spawn"

	first := NotePath(dir, id)
	wantFirst := filepath.Join(dir, "spawn.md")
	if first != wantFirst {
		t.Fatalf("NotePath(1st) = %q; want %q", first, wantFirst)
	}
	touch(t, first)

	second := NotePath(dir, id)
	wantSecond := filepath.Join(dir, "spawn-2.md")
	if second != wantSecond {
		t.Fatalf("NotePath(2nd) = %q; want %q", second, wantSecond)
	}
	touch(t, second)

	third := NotePath(dir, id)
	wantThird := filepath.Join(dir, "spawn-3.md")
	if third != wantThird {
		t.Fatalf("NotePath(3rd) = %q; want %q", third, wantThird)
	}
}

// TestNotePath_GapResolvesToFirstFree pins "first free, not highest plus one": a gap (id.md and
// id-3.md present, id-2.md absent) resolves to id-2.md rather than skipping ahead.
func TestNotePath_GapResolvesToFirstFree(t *testing.T) {
	dir := t.TempDir()
	touch(t, filepath.Join(dir, "spawn.md"))
	touch(t, filepath.Join(dir, "spawn-3.md"))

	want := filepath.Join(dir, "spawn-2.md")
	if got := NotePath(dir, "spawn"); got != want {
		t.Errorf("NotePath(gap) = %q; want %q", got, want)
	}
}

// TestNotePath_SanitizesID pins every id-sanitization rule: an empty id, an id containing a path
// separator, an id containing "..", an id equal to "." or "..", and an id whose id+".md" equals
// ReportFileName all return "".
func TestNotePath_SanitizesID(t *testing.T) {
	dir := t.TempDir()
	reportStem := ReportFileName[:len(ReportFileName)-len(".md")]

	tests := []struct {
		name string
		id   string
	}{
		{"Empty", ""},
		{"ForwardSlash", "sub/id"},
		{"Backslash", `sub\id`},
		{"DotDotElement", "sub/../id"},
		{"SingleDot", "."},
		{"DoubleDot", ".."},
		{"CollidesWithReportFileName", reportStem},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := NotePath(dir, tt.id); got != "" {
				t.Errorf("NotePath(%q, %q) = %q; want \"\"", dir, tt.id, got)
			}
		})
	}
}
