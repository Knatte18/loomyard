// passagereader_test.go covers the adapter from heddle's stored entries to a shuttle session's standing text.

package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Knatte18/loomyard/internal/heddleengine"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// TestReadHeddleStanding checks that a prompt entry comes back as a passage.
// It also checks that a file entry comes back as a file entry.
// Each carries its slug, text and engine key, the key being empty for an entry that reaches every engine.
func TestReadHeddleStanding(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	boltDir := t.TempDir()
	store := `{"entries": [
  {"slug": "reader-test-prompt", "text": "prompt text", "order": 1, "targets": ["claude-prompt"]},
  {"slug": "reader-test-file", "text": "file text", "order": 2, "targets": ["agents-file"]}
]}`
	if err := os.WriteFile(filepath.Join(boltDir, heddleengine.FileName), []byte(store), 0o644); err != nil {
		t.Fatalf("write the heddle store: %v", err)
	}

	got, err := readHeddleStanding(boltDir, "claude", "writer")
	if err != nil {
		t.Fatalf("readHeddleStanding() error: %v", err)
	}

	find := func(passages []shuttleengine.Passage, slug string) (shuttleengine.Passage, bool) {
		for _, passage := range passages {
			if passage.Slug == slug {
				return passage, true
			}
		}
		return shuttleengine.Passage{}, false
	}
	wantPrompt := shuttleengine.Passage{Slug: "reader-test-prompt", Text: "prompt text", Engine: "claude"}
	if passage, ok := find(got.Passages, wantPrompt.Slug); !ok || passage != wantPrompt {
		t.Errorf("readHeddleStanding() passages = %+v; want one %+v", got.Passages, wantPrompt)
	}
	// An agents-file entry reaches every engine, so its passage carries no engine key.
	wantFile := shuttleengine.Passage{Slug: "reader-test-file", Text: "file text"}
	if entry, ok := find(got.FileEntries, wantFile.Slug); !ok || entry != wantFile {
		t.Errorf("readHeddleStanding() file entries = %+v; want one %+v", got.FileEntries, wantFile)
	}
}
