// position_test.go exercises toLSPPosition's UTF-16 conversion (both on a
// pure-ASCII line, where byte column and UTF-16 offset coincide, and on a
// line with a multi-byte rune before the target column, where they diverge)
// and formatLocation's round trip back to a display string. Entirely
// offline and spawn-free: stdlib only, no exec.Command.

package codeintelengine

import (
	"os"
	"path/filepath"
	"testing"
)

// writeSourceFile writes content to a file under dir and returns its path.
func writeSourceFile(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile(%s): %v", path, err)
	}
	return path
}

func TestToLSPPosition_ASCIILine(t *testing.T) {
	dir := t.TempDir()
	// Line 3 is "func main() {}"; "func " is 5 ASCII bytes, so byte column
	// 6 (1-based) points at the 'm' of "main" — byte offset and UTF-16
	// offset coincide on a pure-ASCII line.
	path := writeSourceFile(t, dir, "main.go", "package main\n\nfunc main() {}\n")

	got, err := toLSPPosition(Position{File: path, Line: 3, Character: 6})
	if err != nil {
		t.Fatalf("toLSPPosition() returned unexpected error: %v", err)
	}

	want := lspPosition{Line: 2, Character: 5}
	if got != want {
		t.Errorf("toLSPPosition(ASCII line) = %+v; want %+v", got, want)
	}
}

func TestToLSPPosition_MultiByteRuneBeforeColumn(t *testing.T) {
	dir := t.TempDir()
	// Line 3 is "café hello"; "café " is 6 bytes ('é' is a 2-byte UTF-8
	// encoding) but only 5 UTF-16 code units ('é' is a single BMP code
	// unit). Character 7 (1-based) selects byte column 6 — right after the
	// trailing space — so the byte offset (6) and the UTF-16 offset (5)
	// diverge, exercising the mismatch toLSPPosition exists to correct.
	path := writeSourceFile(t, dir, "cafe.txt", "line one\nline two\ncafé hello\n")

	got, err := toLSPPosition(Position{File: path, Line: 3, Character: 7})
	if err != nil {
		t.Fatalf("toLSPPosition() returned unexpected error: %v", err)
	}

	want := lspPosition{Line: 2, Character: 5}
	if got != want {
		t.Errorf("toLSPPosition(multi-byte rune line) = %+v; want %+v (byte offset 6 would be wrong)", got, want)
	}
}

func TestFormatLocation_RoundTrip(t *testing.T) {
	loc := lspLocation{
		URI: "file:///tmp/example/foo.go",
		Range: lspRange{
			Start: lspPosition{Line: 9, Character: 5},
			End:   lspPosition{Line: 9, Character: 12},
		},
	}

	got := formatLocation(loc)
	want := "/tmp/example/foo.go:10:6"
	if got != want {
		t.Errorf("formatLocation(%+v) = %q; want %q", loc, got, want)
	}
}
