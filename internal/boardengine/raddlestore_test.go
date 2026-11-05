// raddlestore_test.go — the read of the raddle store: which slugs it holds, an absent root and an unreadable store.json.

package boardengine

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// writeStoreFile writes content as store.json of the store directory sketch under boardDir.
func writeStoreFile(t *testing.T, boardDir, sketch, content string) string {
	t.Helper()
	dir := RaddleStoreDir(boardDir, sketch)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir store directory: %v", err)
	}
	path := filepath.Join(dir, raddleStoreFileName)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write store.json: %v", err)
	}
	return path
}

func TestReadRaddleStore(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	t.Run("HoldsASketchSlugAndEveryFiledSlug", func(t *testing.T) {
		t.Parallel()
		boardDir := t.TempDir()
		writeStoreFile(t, boardDir, "alpha", `{"created":"2026-09-01T08:00:00Z","hash":"h1","filed":["alpha-a","alpha-b"]}`)
		writeStoreFile(t, boardDir, "beta", `{"created":"2026-09-02T08:00:00Z","hash":"h2","filed":["beta-proj","beta-a"],"project":"beta-proj","extra":1}`)

		store, err := ReadRaddleStore(boardDir)
		if err != nil {
			t.Fatalf("ReadRaddleStore() error = %v; want nil", err)
		}

		tests := []struct {
			slug       string
			wantSketch string
			wantHeld   bool
		}{
			{"alpha", "alpha", true},
			{"alpha-b", "alpha", true},
			{"beta", "beta", true},
			{"beta-proj", "beta", true},
			{"beta-a", "beta", true},
			{"other", "", false},
		}
		for _, tt := range tests {
			entry, held := store.Holds(tt.slug)
			if held != tt.wantHeld || entry.Sketch != tt.wantSketch {
				t.Errorf("Holds(%q) = sketch %q, %t; want sketch %q, %t", tt.slug, entry.Sketch, held, tt.wantSketch, tt.wantHeld)
			}
		}
		entry, _ := store.Holds("beta-proj")
		if entry.Project != "beta-proj" || entry.Hash != "h2" || !entry.Created.Equal(time.Date(2026, 9, 2, 8, 0, 0, 0, time.UTC)) {
			t.Errorf("Holds(beta-proj) = project %q, hash %q, created %v; want beta-proj, h2, 2026-09-02T08:00:00Z", entry.Project, entry.Hash, entry.Created)
		}
	})

	t.Run("AnAbsentRootHoldsNone", func(t *testing.T) {
		t.Parallel()
		store, err := ReadRaddleStore(t.TempDir())
		if err != nil {
			t.Fatalf("ReadRaddleStore() error = %v; want nil", err)
		}
		if _, held := store.Holds("alpha"); held {
			t.Errorf("Holds(alpha) = true on an absent store root; want false")
		}
	})

	t.Run("AnUnreadableStoreFileIsAnErrorNamingIt", func(t *testing.T) {
		t.Parallel()
		boardDir := t.TempDir()
		path := writeStoreFile(t, boardDir, "broken", `{not json`)

		_, err := ReadRaddleStore(boardDir)
		if err == nil {
			t.Fatal("ReadRaddleStore() error = nil; want an error")
		}
		for _, want := range []string{path, "git -C " + boardDir + " checkout -- raddle/broken/store.json"} {
			if !strings.Contains(err.Error(), want) {
				t.Errorf("ReadRaddleStore() error = %q; want it to contain %q", err, want)
			}
		}
	})
}
