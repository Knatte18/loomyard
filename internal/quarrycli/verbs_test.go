// verbs_test.go covers the four verbs' answer paths against a fixture repository under t.TempDir().
// Its golden tests check glyphs' JSON and --text output byte for byte against the facade's own rendering.
// All four reach quarry.TOC, Glyphs, Resolve and Expand, which read files and spawn no process,
// so this file stays in tier 0.

package quarrycli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/quarry/quarry"

	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// writeFixtureRepo writes files, keyed by repository-relative path, under a fresh t.TempDir().
// It returns that directory's absolute path, ready to hand to RunCLIIn as cwd.
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

// TestRunCLIIn_EachVerb_Answers drives each of the four verbs against a fixture repository.
// A positive answer must exit 0 with a parseable JSON payload.
// Expand on a directory exits non-zero, because a directory is not a type and answers not_found.
func TestRunCLIIn_EachVerb_Answers(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	root := writeFixtureRepo(t, map[string]string{"sub/a.go": "package sub\n\nfunc Foo() {}\n\ntype T struct{}\n"})

	tests := []struct {
		name      string
		args      []string
		wantExit0 bool
	}{
		{"toc", []string{"toc", "sub"}, true},
		{"glyphs", []string{"glyphs", "sub"}, true},
		{"resolve", []string{"resolve", "sub#Foo"}, true},
		{"expand a type glyph", []string{"expand", "sub#T"}, true},
		{"expand a directory", []string{"expand", "sub"}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var out bytes.Buffer
			exitCode := RunCLIIn(root, &out, tt.args)

			if !tt.wantExit0 {
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

// TestRunCLIIn_Resolve_PerGlyphAnswers asserts resolve prints one answer per glyph in argument order.
// It exits 1 on any negative answer and 0 when every glyph is found.
func TestRunCLIIn_Resolve_PerGlyphAnswers(t *testing.T) {
	tiergate.Require(t, 0)
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

// TestRunCLIIn_Glyphs_GoldenAgainstFacade asserts glyphs' emitted bytes are byte-identical to the facade's.
// Both the JSON and the --text output are checked, against the facade's RenderGlyphsJSON and RenderGlyphsText.
// The expected answer comes straight from quarry.Open and Glyphs.
// That copied-verbatim guarantee is what this verb exists to provide.
func TestRunCLIIn_Glyphs_GoldenAgainstFacade(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	root := writeFixtureRepo(t, map[string]string{
		"sub/a.go": "package sub\n\nfunc Foo() {}\n\nfunc Bar() {}\n",
	})

	tests := []struct {
		name   string
		args   []string
		render func(t *testing.T, answer quarry.GlyphsAnswer) string
	}{
		{"json", []string{"glyphs", "sub"}, func(t *testing.T, answer quarry.GlyphsAnswer) string {
			data, err := quarry.RenderGlyphsJSON(answer)
			if err != nil {
				t.Fatalf("quarry.RenderGlyphsJSON(...) failed: %v", err)
			}
			return string(data)
		}},
		{"text", []string{"glyphs", "--text", "sub"}, func(t *testing.T, answer quarry.GlyphsAnswer) string {
			return quarry.RenderGlyphsText(answer)
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var out bytes.Buffer
			exitCode := RunCLIIn(root, &out, tt.args)
			if exitCode != 0 {
				t.Fatalf("RunCLIIn(%v) = %d; want 0; output: %s", tt.args, exitCode, out.String())
			}

			repo, err := quarry.Open(root)
			if err != nil {
				t.Fatalf("quarry.Open(%q) failed: %v", root, err)
			}
			answer, err := repo.Glyphs("sub")
			if err != nil {
				t.Fatalf("repo.Glyphs(%q) failed: %v", "sub", err)
			}
			want := tt.render(t, answer)

			if out.String() != want {
				t.Errorf("RunCLIIn(%v) output = %q; want byte-identical to facade rendering %q", tt.args, out.String(), want)
			}
		})
	}
}

// TestRunCLIIn_GlyphsText_EmptyAnswer asserts that glyphs --text writes nothing and exits 0.
// The directory holds no symbols and no incomplete files.
func TestRunCLIIn_GlyphsText_EmptyAnswer(t *testing.T) {
	tiergate.Require(t, 0)
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

// TestRunCLIIn_GlyphsText_TestFileFilter asserts a line filter on _test.go drops exactly the test-file symbol lines.
// Each symbol is one line carrying its own file path.
func TestRunCLIIn_GlyphsText_TestFileFilter(t *testing.T) {
	tiergate.Require(t, 0)
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

// TestRunCLIIn_TextFlagRefusals asserts the --text paths that fail exit non-zero.
// A failing glyphs query under --text still emits the JSON error envelope.
// --text is glyphs-only, so the other verbs reject it.
func TestRunCLIIn_TextFlagRefusals(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	root := writeFixtureRepo(t, map[string]string{"sub/a.go": "package sub\n"})

	tests := []struct {
		name         string
		args         []string
		wantEnvelope bool
	}{
		{"glyphs on a missing directory", []string{"glyphs", "--text", "missing"}, true},
		{"toc rejects the flag", []string{"toc", "--text", "sub"}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var out bytes.Buffer
			exitCode := RunCLIIn(root, &out, tt.args)
			if exitCode == 0 {
				t.Fatalf("RunCLIIn(%v) = 0; want non-zero; output: %s", tt.args, out.String())
			}
			if !tt.wantEnvelope {
				return
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
		})
	}
}
