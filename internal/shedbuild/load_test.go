// load_test.go covers Load's path handling only -- a well-formed round trip, an absent file, a
// relative path, and a directory path -- deliberately not a second copy of the Parse table.

package shedbuild

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestLoad_WellFormedFileParsesToSameRecipeAsParse writes a well-formed recipe into a t.TempDir()
// file and asserts Load returns exactly the same Recipe value Parse returns for the same bytes.
func TestLoad_WellFormedFileParsesToSameRecipeAsParse(t *testing.T) {
	t.Parallel()

	data := []byte(`
version: 1
entry: start
terminals: [done]
producers:
  - name: row1
    engine: bouncer
    config:
      key: value
`)

	dir := t.TempDir()
	path := filepath.Join(dir, "recipe.yaml")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("WriteFile(%q) = %v; want no error", path, err)
	}

	wantRecipe, wantErr := Parse(data)
	if wantErr != nil {
		t.Fatalf("Parse(fixture bytes) = _, %v; want no error", wantErr)
	}

	got, err := Load(path)
	if err != nil {
		t.Fatalf("Load(%q) = _, %v; want no error", path, err)
	}
	if !recipeEqual(got, wantRecipe) {
		t.Errorf("Load(%q) = %+v; want %+v", path, got, wantRecipe)
	}
}

// TestLoad_UnreadableAbsolutePathErrors asserts an absolute path naming no file, and one naming a directory, each error.
func TestLoad_UnreadableAbsolutePathErrors(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	tests := []struct {
		name string
		path string
	}{
		{"no file", filepath.Join(dir, "does-not-exist.yaml")},
		{"directory", dir},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if _, err := Load(tt.path); err == nil {
				t.Fatalf("Load(%q) = _, nil; want error", tt.path)
			}
		})
	}
}

// TestLoad_RelativePathErrorsBeforeAnyRead asserts a relative path errors with the must-be-absolute
// message, using a relative path that does resolve to a real file from the test's working
// directory -- the only way to make the reject-before-read assertion meaningful.
func TestLoad_RelativePathErrorsBeforeAnyRead(t *testing.T) {
	t.Parallel()

	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("os.Getwd() = _, %v; want no error", err)
	}

	relName := "shedbuild_load_test_relative_fixture.yaml"
	absPath := filepath.Join(cwd, relName)
	if err := os.WriteFile(absPath, []byte("version: 1\nentry: start\nterminals: [done]\nproducers:\n  - name: row1\n    engine: bouncer\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(%q) = %v; want no error", absPath, err)
	}
	t.Cleanup(func() {
		os.Remove(absPath)
	})

	got, err := Load(relName)
	if err == nil {
		t.Fatalf("Load(%q) = %+v, nil; want error", relName, got)
	}
	if !strings.Contains(err.Error(), "must be absolute") {
		t.Errorf("Load(%q) error = %q; want substring %q", relName, err.Error(), "must be absolute")
	}
}
