// raddlecontext_integration_test.go builds the raddle board context over a hub forged by hubforge.
// This package's own testmain_test.go arms the hermetic git test environment for the whole binary,
// so this file adds no TestMain.

package loomcli

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/raddleparser"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

func TestRaddleBoardContext(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	hub := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	hubforge.AddPair(t, hub, "the-sketch")

	// A board write cannot file an entry with a label the config lacks, so the entries are written to the board file directly.
	board := `{"version":1,"entries":[` +
		`{"id":1,"slug":"the-sketch","title":"Sketch","kind":"task","recipe":"raddle","labels":["sketch","mod-a"],"revised":"2026-11-01T09:00:00Z"},` +
		`{"id":2,"slug":"open-other","title":"Open","kind":"task","labels":["bug"],"revised":"2026-11-01T10:00:00Z"},` +
		`{"id":3,"slug":"done-one","title":"Done","kind":"task","labels":["bug"],"status":"done"}]}`
	if err := os.WriteFile(filepath.Join(hub.BoardDir(), "board.json"), []byte(board), 0o644); err != nil {
		t.Fatalf("write board.json: %v", err)
	}
	storeDir := filepath.Join(hub.BoardDir(), "raddle", "old-sketch")
	if err := os.MkdirAll(storeDir, 0o755); err != nil {
		t.Fatalf("create the store directory: %v", err)
	}
	if err := os.WriteFile(filepath.Join(storeDir, "store.json"), []byte(`{"hash":"h","filed":["old-part"]}`), 0o644); err != nil {
		t.Fatalf("write store.json: %v", err)
	}
	location, err := lymcwd.ResolveWorktree(hub.PairCodeWorktree("the-sketch"))
	if err != nil {
		t.Fatalf("ResolveWorktree() error = %v; want nil", err)
	}

	got, err := raddleBoardContext(location, true)
	if err != nil {
		t.Fatalf("raddleBoardContext() error = %v; want nil", err)
	}

	if got.Sketch != "the-sketch" || got.Entry != "the-sketch" || !got.Filing {
		t.Errorf("context = sketch %q, entry %q, filing %v; want the run's own entry and filing", got.Sketch, got.Entry, got.Filing)
	}
	if want := []string{"mod-a"}; !slices.Equal(got.ModuleLabels, want) {
		t.Errorf("ModuleLabels = %v; want %v, the Sketch's labels that are not type labels", got.ModuleLabels, want)
	}
	open := map[string]bool{}
	for _, entry := range got.Entries {
		open[entry.Slug] = entry.Open
	}
	if want := map[string]bool{"the-sketch": true, "open-other": true, "done-one": false}; !equalOpenness(open, want) {
		t.Errorf("entries open = %v; want %v", open, want)
	}
	for _, entry := range got.Entries {
		if entry.Slug == "open-other" && entry.Revised != "2026-11-01T10:00:00Z" {
			t.Errorf("open-other revised = %q; want its RFC 3339 stamp", entry.Revised)
		}
	}
	if want := []string{"old-sketch", "old-part"}; !slices.Equal(got.Store, want) {
		t.Errorf("Store = %v; want %v", got.Store, want)
	}
	for _, label := range []string{"bug", "sketch"} {
		if !slices.Contains(got.TypeLabels, label) {
			t.Errorf("TypeLabels = %v; want it to hold %q", got.TypeLabels, label)
		}
	}
	for _, recipe := range []string{raddleparser.RecipeTapestry, raddleparser.RecipeDarn} {
		if len(got.Profiles[recipe]) == 0 {
			t.Errorf("Profiles[%s] = none; want the profiles that hold a block for it", recipe)
		}
	}
}

// equalOpenness reports whether two slug-to-open maps hold the same pairs.
func equalOpenness(got, want map[string]bool) bool {
	if len(got) != len(want) {
		return false
	}
	for slug, open := range want {
		if value, held := got[slug]; !held || value != open {
			return false
		}
	}
	return true
}
