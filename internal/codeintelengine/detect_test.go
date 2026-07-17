// detect_test.go table-drives DetectLanguage against synthetic marker trees
// built under t.TempDir with os.WriteFile — no git spawn, no exec.Command,
// entirely offline and spawn-free per the batch's leaf test tier.

package codeintelengine

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// touchMarkers creates an empty file for each name directly under dir.
func touchMarkers(t *testing.T, dir string, names ...string) {
	t.Helper()
	for _, name := range names {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte{}, 0o644); err != nil {
			t.Fatalf("WriteFile(%s): %v", path, err)
		}
	}
}

func TestDetectLanguage_SingleLanguageCases(t *testing.T) {
	tests := []struct {
		name    string
		markers []string
		want    string
	}{
		{name: "go", markers: []string{"go.mod"}, want: "go"},
		{name: "rust", markers: []string{"Cargo.toml"}, want: "rust"},
		{name: "csharp", markers: []string{".sln"}, want: "csharp"},
		{name: "python", markers: []string{"pyproject.toml"}, want: "python"},
	}
	reg := builtins()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			touchMarkers(t, dir, tt.markers...)

			gotLang, _, err := DetectLanguage(dir, reg, "")
			if err != nil {
				t.Fatalf("DetectLanguage(%v) returned unexpected error: %v", tt.markers, err)
			}
			if gotLang != tt.want {
				t.Errorf("DetectLanguage(%v) = %q; want %q", tt.markers, gotLang, tt.want)
			}
		})
	}
}

func TestDetectLanguage_TypeScriptRequiresBothMarkers(t *testing.T) {
	reg := builtins()

	t.Run("package.json alone does not match", func(t *testing.T) {
		dir := t.TempDir()
		touchMarkers(t, dir, "package.json")

		_, _, err := DetectLanguage(dir, reg, "")
		if !errors.Is(err, ErrNoLanguage) {
			t.Fatalf("DetectLanguage(package.json only) err = %v; want ErrNoLanguage", err)
		}
	})

	t.Run("tsconfig.json alone does not match", func(t *testing.T) {
		dir := t.TempDir()
		touchMarkers(t, dir, "tsconfig.json")

		_, _, err := DetectLanguage(dir, reg, "")
		if !errors.Is(err, ErrNoLanguage) {
			t.Fatalf("DetectLanguage(tsconfig.json only) err = %v; want ErrNoLanguage", err)
		}
	})

	t.Run("both markers match typescript", func(t *testing.T) {
		dir := t.TempDir()
		touchMarkers(t, dir, "package.json", "tsconfig.json")

		gotLang, _, err := DetectLanguage(dir, reg, "")
		if err != nil {
			t.Fatalf("DetectLanguage(both markers) returned unexpected error: %v", err)
		}
		if gotLang != "typescript" {
			t.Errorf("DetectLanguage(both markers) = %q; want %q", gotLang, "typescript")
		}
	})
}

func TestDetectLanguage_PrecedenceOrder(t *testing.T) {
	dir := t.TempDir()
	// A directory that satisfies both "go" (go.mod) and "typescript"
	// (package.json + tsconfig.json) markers resolves to "go" because it
	// comes first in the pinned precedence slice.
	touchMarkers(t, dir, "go.mod", "package.json", "tsconfig.json")

	gotLang, _, err := DetectLanguage(dir, builtins(), "")
	if err != nil {
		t.Fatalf("DetectLanguage(go+typescript markers) returned unexpected error: %v", err)
	}
	if gotLang != "go" {
		t.Errorf("DetectLanguage(go+typescript markers) = %q; want %q (precedence)", gotLang, "go")
	}
}

func TestDetectLanguage_LangOverride(t *testing.T) {
	dir := t.TempDir()
	// The override bypasses marker detection entirely, so an empty
	// directory with no markers still resolves via the override.
	reg := builtins()

	gotLang, gotEntry, err := DetectLanguage(dir, reg, "rust")
	if err != nil {
		t.Fatalf("DetectLanguage(override=rust) returned unexpected error: %v", err)
	}
	if gotLang != "rust" {
		t.Errorf("DetectLanguage(override=rust) = %q; want %q", gotLang, "rust")
	}
	if gotEntry.Match != "any" {
		t.Errorf("DetectLanguage(override=rust) entry = %+v; want the rust builtin entry", gotEntry)
	}
}

func TestDetectLanguage_UnknownLangOverride(t *testing.T) {
	dir := t.TempDir()
	reg := builtins()

	_, _, err := DetectLanguage(dir, reg, "cobol")
	if err == nil {
		t.Fatal("DetectLanguage(override=cobol) returned nil error; want an error naming known languages")
	}
	if !strings.Contains(err.Error(), `"cobol"`) {
		t.Errorf("DetectLanguage(override=cobol) error = %q; want it to name the override %q", err.Error(), "cobol")
	}
	if !strings.Contains(err.Error(), "go") {
		t.Errorf("DetectLanguage(override=cobol) error = %q; want it to list a known language %q", err.Error(), "go")
	}
}

func TestDetectLanguage_NoMarkerYieldsErrNoLanguage(t *testing.T) {
	dir := t.TempDir()
	reg := builtins()

	_, _, err := DetectLanguage(dir, reg, "")
	if !errors.Is(err, ErrNoLanguage) {
		t.Fatalf("DetectLanguage(empty dir) err = %v; want errors.Is(err, ErrNoLanguage)", err)
	}
}
