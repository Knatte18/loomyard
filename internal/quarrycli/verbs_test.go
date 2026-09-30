// verbs_test.go covers the four verbs' own answer-path behaviour against a fixture repository built under t.TempDir(): RunCLIIn's exit code and parseable-JSON contract on success, resolve's per-glyph answers (one document per glyph, exit 1 on any negative answer), and golden assertions that glyphs' JSON and --text output are each byte-identical to the facade's own rendering of the same answer.
// All four reach quarry.TOC, Glyphs, Resolve and Expand -- file readers, none of which spawns a process --
// so this file stays untagged and tier1-pure.

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

// decodeDocuments decodes every JSON document in out, in order.
func decodeDocuments(t *testing.T, out string) []map[string]any {
	t.Helper()
	dec := json.NewDecoder(strings.NewReader(out))
	var docs []map[string]any
	for dec.More() {
		var doc map[string]any
		if err := dec.Decode(&doc); err != nil {
			t.Fatalf("output is not a stream of JSON documents: %v; got: %q", err, out)
		}
		docs = append(docs, doc)
	}
	return docs
}

// TestRunCLIIn_Resolve_PerGlyphAnswers asserts resolve prints one answer per glyph in argument order, and exits 1 on any negative answer but 0 when every glyph is found.
func TestRunCLIIn_Resolve_PerGlyphAnswers(t *testing.T) {
	root := writeFixtureRepo(t, map[string]string{"sub/a.go": "package sub\n\nfunc Foo() {}\n"})

	t.Run("not_found", func(t *testing.T) {
		args := []string{"resolve", "sub#Foo", "sub#Missing"}
		var out bytes.Buffer
		if exitCode := RunCLIIn(root, &out, args); exitCode != 1 {
			t.Fatalf("RunCLIIn(%v) = %d; want 1; output: %s", args, exitCode, out.String())
		}

		docs := decodeDocuments(t, out.String())
		if len(docs) != 2 {
			t.Fatalf("RunCLIIn(%v) printed %d documents; want 2; got: %q", args, len(docs), out.String())
		}
		if docs[1]["status"] != "not_found" {
			t.Errorf("second document status = %v; want not_found", docs[1]["status"])
		}
		for i, doc := range docs {
			if _, has := doc["ok"]; has {
				t.Errorf("document %d carries an ok key; got: %v", i, doc)
			}
		}

		repo, err := quarry.Open(root)
		if err != nil {
			t.Fatalf("quarry.Open(%q) failed: %v", root, err)
		}
		results, err := repo.Resolve(args[1:])
		if err != nil {
			t.Fatalf("repo.Resolve failed: %v", err)
		}
		var want []byte
		for _, r := range results {
			data, err := quarry.RenderResolveJSON(r)
			if err != nil {
				t.Fatalf("quarry.RenderResolveJSON failed: %v", err)
			}
			want = append(want, data...)
		}
		if !bytes.Equal(out.Bytes(), want) {
			t.Errorf("output = %q; want byte-identical to facade rendering %q", out.String(), string(want))
		}
	})

	t.Run("grammar_rejection", func(t *testing.T) {
		args := []string{"resolve", "sub#Foo", "sub##Foo"}
		var out bytes.Buffer
		if exitCode := RunCLIIn(root, &out, args); exitCode != 1 {
			t.Fatalf("RunCLIIn(%v) = %d; want 1; output: %s", args, exitCode, out.String())
		}

		docs := decodeDocuments(t, out.String())
		if len(docs) != 2 {
			t.Fatalf("RunCLIIn(%v) printed %d documents; want 2; got: %q", args, len(docs), out.String())
		}
		for _, key := range []string{"error", "reason"} {
			if _, has := docs[1][key]; !has {
				t.Errorf("second document missing %q key; got: %v", key, docs[1])
			}
		}
		if _, has := docs[1]["status"]; has {
			t.Errorf("second document carries a status key; got: %v", docs[1])
		}
		for i, doc := range docs {
			if _, has := doc["ok"]; has {
				t.Errorf("document %d carries an ok key; got: %v", i, doc)
			}
		}
	})

	t.Run("all_found", func(t *testing.T) {
		args := []string{"resolve", "sub#Foo", "sub#Foo"}
		var out bytes.Buffer
		if exitCode := RunCLIIn(root, &out, args); exitCode != 0 {
			t.Fatalf("RunCLIIn(%v) = %d; want 0; output: %s", args, exitCode, out.String())
		}

		docs := decodeDocuments(t, out.String())
		if len(docs) != 2 {
			t.Fatalf("RunCLIIn(%v) printed %d documents; want 2; got: %q", args, len(docs), out.String())
		}
		for i, doc := range docs {
			if doc["status"] != "found" {
				t.Errorf("document %d status = %v; want found", i, doc["status"])
			}
		}
	})
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

// TestRunCLIIn_GlyphsText_GoldenAgainstFacade asserts glyphs --text emits exactly the facade's own RenderGlyphsText rendering of the same answer, and exits 0.
func TestRunCLIIn_GlyphsText_GoldenAgainstFacade(t *testing.T) {
	root := writeFixtureRepo(t, map[string]string{
		"sub/a.go": "package sub\n\nfunc Foo() {}\n\nfunc Bar() {}\n",
	})

	var out bytes.Buffer
	exitCode := RunCLIIn(root, &out, []string{"glyphs", "--text", "sub"})
	if exitCode != 0 {
		t.Fatalf("RunCLIIn(glyphs --text sub) = %d; want 0; output: %s", exitCode, out.String())
	}

	repo, err := quarry.Open(root)
	if err != nil {
		t.Fatalf("quarry.Open(%q) failed: %v", root, err)
	}
	answer, err := repo.Glyphs("sub")
	if err != nil {
		t.Fatalf("repo.Glyphs(%q) failed: %v", "sub", err)
	}
	want := quarry.RenderGlyphsText(answer)

	if out.String() != want {
		t.Errorf("RunCLIIn(glyphs --text sub) output = %q; want byte-identical to facade rendering %q", out.String(), want)
	}
}

// TestRunCLIIn_GlyphsText_EmptyAnswer asserts a directory with no symbols and no incomplete files makes glyphs --text write nothing and exit 0.
func TestRunCLIIn_GlyphsText_EmptyAnswer(t *testing.T) {
	root := writeFixtureRepo(t, map[string]string{"docs/notes.txt": "just prose\n"})

	var out bytes.Buffer
	exitCode := RunCLIIn(root, &out, []string{"glyphs", "--text", "docs"})
	if exitCode != 0 {
		t.Fatalf("RunCLIIn(glyphs --text docs) = %d; want 0; output: %s", exitCode, out.String())
	}
	if out.Len() != 0 {
		t.Errorf("RunCLIIn(glyphs --text docs) wrote %q; want zero bytes", out.String())
	}
}

// TestRunCLIIn_GlyphsText_TestFileFilter asserts a line filter on _test.go drops exactly the test-file symbol lines, because each symbol is one line carrying its own file path.
func TestRunCLIIn_GlyphsText_TestFileFilter(t *testing.T) {
	root := writeFixtureRepo(t, map[string]string{
		"sub/a.go":      "package sub\n\nfunc Prod() {}\n",
		"sub/a_test.go": "package sub\n\nfunc Check() {}\n",
	})

	var out bytes.Buffer
	exitCode := RunCLIIn(root, &out, []string{"glyphs", "--text", "sub"})
	if exitCode != 0 {
		t.Fatalf("RunCLIIn(glyphs --text sub) = %d; want 0; output: %s", exitCode, out.String())
	}

	// A line's id is its last field, so the lines are selected by id, never by substring.
	lineByID := map[string][]string{}
	var kept []string
	for _, line := range strings.Split(strings.TrimSuffix(out.String(), "\n"), "\n") {
		fields := strings.Fields(line)
		if len(fields) > 0 {
			id := fields[len(fields)-1]
			lineByID[id] = append(lineByID[id], line)
		}
		if !strings.Contains(line, "_test.go") {
			kept = append(kept, line)
		}
	}

	checkLines, prodLines := lineByID["sub#Check"], lineByID["sub#Prod"]
	if len(checkLines) != 1 || len(prodLines) != 1 {
		t.Fatalf("got %d sub#Check and %d sub#Prod lines; want exactly one each; output: %q", len(checkLines), len(prodLines), out.String())
	}
	if !strings.Contains(checkLines[0], "_test.go") {
		t.Errorf("line for the test-file function lacks _test.go: %q", checkLines[0])
	}
	if strings.Contains(prodLines[0], "_test.go") {
		t.Errorf("line for the non-test function contains _test.go: %q", prodLines[0])
	}
	if len(kept) != 1 || kept[0] != prodLines[0] {
		t.Errorf("lines left after dropping _test.go = %q; want exactly the sub#Prod line %q", kept, prodLines[0])
	}
}

// TestRunCLIIn_GlyphsText_MissingDir asserts a failing query under --text still emits the JSON error envelope and exits non-zero.
func TestRunCLIIn_GlyphsText_MissingDir(t *testing.T) {
	root := writeFixtureRepo(t, map[string]string{"sub/a.go": "package sub\n"})

	var out bytes.Buffer
	exitCode := RunCLIIn(root, &out, []string{"glyphs", "--text", "missing"})
	if exitCode == 0 {
		t.Fatalf("RunCLIIn(glyphs --text missing) = 0; want non-zero; output: %s", out.String())
	}

	var payload map[string]any
	if err := json.Unmarshal(out.Bytes(), &payload); err != nil {
		t.Fatalf("output is not a JSON document: %v; got: %q", err, out.String())
	}
	if payload["ok"] != false {
		t.Errorf("ok = %v; want false", payload["ok"])
	}
	if _, has := payload["error"]; !has {
		t.Errorf("payload lacks an error key; got: %v", payload)
	}
}

// TestRunCLIIn_TOC_RejectsTextFlag asserts --text is glyphs-only: the other verbs reject it.
func TestRunCLIIn_TOC_RejectsTextFlag(t *testing.T) {
	root := writeFixtureRepo(t, map[string]string{"sub/a.go": "package sub\n"})

	var out bytes.Buffer
	if exitCode := RunCLIIn(root, &out, []string{"toc", "--text", "sub"}); exitCode == 0 {
		t.Errorf("RunCLIIn(toc --text sub) = 0; want non-zero; output: %s", out.String())
	}
}
