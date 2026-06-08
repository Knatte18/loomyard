package main

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

// These tests cover main's own responsibility — module routing — not the wiki
// behaviour itself (that lives in internal/wiki). They drive run() directly so
// no binary build or os.Exit is involved.

func TestRunNoArgs(t *testing.T) {
	var out bytes.Buffer
	if code := run(nil, &out); code != 1 {
		t.Fatalf("expected exit 1 for no args, got %d", code)
	}
	if out.Len() != 0 {
		t.Fatalf("expected no module output, got %q", out.String())
	}
}

func TestRunUnknownModule(t *testing.T) {
	var out bytes.Buffer
	if code := run([]string{"bogus", "list"}, &out); code != 1 {
		t.Fatalf("expected exit 1 for unknown module, got %d", code)
	}
	if out.Len() != 0 {
		t.Fatalf("expected no module output, got %q", out.String())
	}
}

func TestRunDispatchesToWiki(t *testing.T) {
	t.Setenv("WIKI_SKIP_GIT", "1")
	wikiPath := t.TempDir()

	var out bytes.Buffer
	code := run([]string{"wiki", "--wiki-path", wikiPath, "rerender"}, &out)
	if code != 0 {
		t.Fatalf("expected exit 0, got %d; output: %s", code, out.String())
	}

	// run must forward the wiki module's JSON to out unchanged.
	var result map[string]any
	if err := json.Unmarshal(out.Bytes(), &result); err != nil {
		t.Fatalf("failed to parse wiki output: %v; output: %s", err, out.String())
	}
	if ok, _ := result["ok"].(bool); !ok {
		t.Fatalf("expected ok=true from dispatched wiki command, got %v", result)
	}
}

func TestRunWikiErrorPropagatesExitCode(t *testing.T) {
	t.Setenv("WIKI_SKIP_GIT", "1")
	wikiPath := t.TempDir()

	// remove of a nonexistent task fails — exit code must bubble up through run.
	var out bytes.Buffer
	code := run([]string{"wiki", "--wiki-path", wikiPath, "remove", `{"id_or_slug":"nope"}`}, &out)
	if code != 1 {
		t.Fatalf("expected exit 1 from failing wiki command, got %d; output: %s", code, out.String())
	}
	if !strings.Contains(out.String(), `"ok":false`) {
		t.Fatalf("expected error JSON on out, got %q", out.String())
	}
}
