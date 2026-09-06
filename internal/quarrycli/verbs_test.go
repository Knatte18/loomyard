// verbs_test.go covers the four verbs' own answer-path behaviour against a fixture repository
// built under t.TempDir(): RunCLIIn's exit code and parseable-JSON contract on success, a bad
// glyph argument's JSON error envelope and non-zero exit, and a golden assertion that glyphs'
// output is byte-identical to the facade's own rendering of the same answer. All four reach
// quarry.TOC, Glyphs, Resolve and Expand -- file readers, none of which spawns a process -- so
// this file stays untagged and tier1-pure.

package quarrycli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/quarry/quarry"
)

// writeFixtureRepo writes files (keyed by repository-relative path) under a fresh t.TempDir() and
// returns that directory's absolute path, ready to hand to RunCLIIn as cwd.
func writeFixtureRepo(t *testing.T, files map[string]string) string {
	t.Helper()
	root := t.TempDir()
	for rel, content := range files {
		full := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatalf("MkdirAll(%q) failed: %v", filepath.Dir(full), err)
		}
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			t.Fatalf("WriteFile(%q) failed: %v", full, err)
		}
	}
	return root
}

// TestRunCLIIn_EachVerb_Success drives each of the four verbs against a fixture repository and
// asserts exit 0 and a parseable JSON payload.
func TestRunCLIIn_EachVerb_Success(t *testing.T) {
	root := writeFixtureRepo(t, map[string]string{"sub/a.go": "package sub\n\nfunc Foo() {}\n"})

	tests := []struct {
		name string
		args []string
	}{
		{"toc", []string{"toc", "sub"}},
		{"glyphs", []string{"glyphs", "sub"}},
		{"resolve", []string{"resolve", "sub#Foo"}},
		{"expand", []string{"expand", "sub"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var out bytes.Buffer
			exitCode := RunCLIIn(root, &out, tt.args)

			if tt.name == "expand" {
				// sub is a directory, not a type: expand rejects it as not_found -- this case
				// covers the negative-answer path, not the success one.
				if exitCode == 0 {
					t.Errorf("RunCLIIn(%v) = 0; want non-zero for a non-type target", tt.args)
				}
				return
			}

			if exitCode != 0 {
				t.Fatalf("RunCLIIn(%v) = %d; want 0; output: %s", tt.args, exitCode, out.String())
			}

			var payload map[string]any
			if err := json.Unmarshal(out.Bytes(), &payload); err != nil {
				t.Fatalf("RunCLIIn(%v) output is not valid JSON: %v; got: %q", tt.args, err, out.String())
			}
		})
	}
}

// TestRunCLIIn_Expand_Success drives expand against a real type glyph and asserts exit 0 and a
// parseable JSON payload.
func TestRunCLIIn_Expand_Success(t *testing.T) {
	root := writeFixtureRepo(t, map[string]string{"sub/a.go": "package sub\n\ntype T struct{}\n"})

	var out bytes.Buffer
	exitCode := RunCLIIn(root, &out, []string{"expand", "sub#T"})

	if exitCode != 0 {
		t.Fatalf("RunCLIIn(expand sub#T) = %d; want 0; output: %s", exitCode, out.String())
	}

	var payload map[string]any
	if err := json.Unmarshal(out.Bytes(), &payload); err != nil {
		t.Fatalf("RunCLIIn(expand sub#T) output is not valid JSON: %v; got: %q", err, out.String())
	}
}

// TestRunCLIIn_Resolve_BadGlyphArgument asserts a glyph the grammar itself rejects produces a JSON
// error envelope and a non-zero exit, rather than being mixed in among any found results.
func TestRunCLIIn_Resolve_BadGlyphArgument(t *testing.T) {
	root := writeFixtureRepo(t, map[string]string{"sub/a.go": "package sub\n\nfunc Foo() {}\n"})

	var out bytes.Buffer
	exitCode := RunCLIIn(root, &out, []string{"resolve", "sub##Foo"})

	if exitCode == 0 {
		t.Fatalf("RunCLIIn(resolve sub##Foo) = 0; want non-zero for a malformed glyph")
	}

	var env map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(out.String())), &env); err != nil {
		t.Fatalf("RunCLIIn(resolve sub##Foo) output is not valid JSON: %v; got: %q", err, out.String())
	}
	if ok, _ := env["ok"].(bool); ok {
		t.Errorf("RunCLIIn(resolve sub##Foo) ok = true; want false")
	}
	if _, has := env["error"]; !has {
		t.Errorf("RunCLIIn(resolve sub##Foo) output missing \"error\" key; got: %q", out.String())
	}
}

// TestRunCLIIn_Glyphs_GoldenAgainstFacade asserts glyphs' emitted bytes are byte-identical to the
// facade's own RenderGlyphsJSON rendering of the same answer, computed directly against
// quarry.Open/Glyphs -- the copied-verbatim guarantee this verb exists to provide.
func TestRunCLIIn_Glyphs_GoldenAgainstFacade(t *testing.T) {
	root := writeFixtureRepo(t, map[string]string{
		"sub/a.go": "package sub\n\nfunc Foo() {}\n\nfunc Bar() {}\n",
	})

	var out bytes.Buffer
	exitCode := RunCLIIn(root, &out, []string{"glyphs", "sub"})
	if exitCode != 0 {
		t.Fatalf("RunCLIIn(glyphs sub) = %d; want 0; output: %s", exitCode, out.String())
	}

	repo, err := quarry.Open(root)
	if err != nil {
		t.Fatalf("quarry.Open(%q) failed: %v", root, err)
	}
	want, err := repo.Glyphs("sub")
	if err != nil {
		t.Fatalf("repo.Glyphs(%q) failed: %v", "sub", err)
	}
	wantBytes, err := quarry.RenderGlyphsJSON(want)
	if err != nil {
		t.Fatalf("quarry.RenderGlyphsJSON(...) failed: %v", err)
	}

	if !bytes.Equal(out.Bytes(), wantBytes) {
		t.Errorf("RunCLIIn(glyphs sub) output = %q; want byte-identical to facade rendering %q", out.String(), string(wantBytes))
	}
}
