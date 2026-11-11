// replayer_test.go drives heddle's Bolt replayer over a temporary directory standing in for the Bolt checkout.

package heddleengine

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// storeBytes encodes a store as the commit side of a replay request.
func storeBytes(t *testing.T, entries ...Entry) []byte {
	t.Helper()
	data, err := json.Marshal(Store{Entries: entries})
	if err != nil {
		t.Fatal(err)
	}
	return data
}

// compacted returns raw without insignificant whitespace, since a saved store re-indents the raw values it keeps.
func compacted(t *testing.T, raw json.RawMessage) string {
	t.Helper()
	var buffer bytes.Buffer
	if err := json.Compact(&buffer, raw); err != nil {
		t.Fatal(err)
	}
	return buffer.String()
}

// replayEntry builds an entry aimed at claude-file.
func replayEntry(slug, text string, order int) Entry {
	return entryFor(slug, text, order, "claude-file")
}

// seedOrigin saves origin and its overview into a fresh directory.
// A replay that changes neither then finds the overview standing.
func seedOrigin(t *testing.T, origin Store) string {
	t.Helper()
	dir := t.TempDir()
	saveStore(t, dir, origin)
	if _, err := RenderOverview(dir); err != nil {
		t.Fatalf("RenderOverview() = %v", err)
	}
	return dir
}

// applyReplay seeds origin into a fresh directory and replays one commit onto it.
// It returns the result and the store afterwards.
func applyReplay(t *testing.T, origin Store, base, parent, commit []byte) (fabricengine.ReplayResult, Store) {
	t.Helper()
	dir := seedOrigin(t, origin)
	result, err := Replayer{}.Apply(fabricengine.ReplayRequest{
		Dir:   dir,
		Files: []fabricengine.ReplayFile{{Path: FileName, Base: base, Before: parent, After: commit}},
	})
	if err != nil {
		t.Fatalf("Apply() = %v", err)
	}
	return result, loadStore(t, dir)
}

func TestReplayer(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	a, b := replayEntry("a", "A", 1), replayEntry("b", "B", 2)

	t.Run("NameAndClaims", func(t *testing.T) {
		t.Parallel()
		r := Replayer{}
		if r.Name() != "heddle" || !r.Claims("heddle.json") || !r.Claims("HEDDLE.md") ||
			r.Claims("board.json") || r.Claims("sub/heddle.json") || r.Claims("sub/HEDDLE.md") || r.Claims(OverrideFileName) {
			t.Error("Replayer must be named heddle and claim exactly heddle.json and HEDDLE.md at the root, leaving the override file unclaimed")
		}
		if _, isLedger := any(r).(fabricengine.RunLedger); isLedger {
			t.Error("Replayer is a run ledger")
		}
	})

	t.Run("TextOnlyTheCommitChangedIsApplied", func(t *testing.T) {
		t.Parallel()
		changed := replayEntry("a", "A local", 1)
		result, store := applyReplay(t, Store{Entries: []Entry{a, b}}, storeBytes(t, a, b), storeBytes(t, a, b), storeBytes(t, changed, b))
		if got, _ := store.Get("a"); got.Text != "A local" || result.Empty || len(result.Lost) != 0 {
			t.Errorf("entry a = %+v, result = %+v; want the local text applied", got, result)
		}
	})

	t.Run("OrderChangedOnBothSidesKeepsOriginsAndReportsLost", func(t *testing.T) {
		t.Parallel()
		local, origin := replayEntry("a", "A", 5), replayEntry("a", "A", 7)
		result, store := applyReplay(t, Store{Entries: []Entry{origin}}, storeBytes(t, a), storeBytes(t, a), storeBytes(t, local))
		if got, _ := store.Get("a"); got.Order != 7 {
			t.Errorf("order = %d; want origin's 7", got.Order)
		}
		if len(result.Lost) != 1 || result.Lost[0].Field != "order" || result.Lost[0].Local != "5" || !result.Empty {
			t.Errorf("result = %+v; want the local order 5 lost and nothing applied", result)
		}
	})

	t.Run("UnknownKeysSurviveAReplayedText", func(t *testing.T) {
		t.Parallel()
		withUnknown := a
		withUnknown.Unknown = map[string]json.RawMessage{"extra": json.RawMessage(`{"k":1}`)}
		changed := replayEntry("a", "A local", 1)
		_, store := applyReplay(t, Store{Entries: []Entry{withUnknown}, Unknown: map[string]json.RawMessage{"top": json.RawMessage(`true`)}},
			storeBytes(t, a), storeBytes(t, a), storeBytes(t, changed))
		got, _ := store.Get("a")
		if got.Text != "A local" || compacted(t, got.Unknown["extra"]) != `{"k":1}` || compacted(t, store.Unknown["top"]) != "true" {
			t.Errorf("entry = %+v, store unknown = %v; want the text replayed and every unknown key kept", got, store.Unknown)
		}
	})

	t.Run("AddedEntryIsAppendedAndRemovedOneDropped", func(t *testing.T) {
		t.Parallel()
		c := replayEntry("c", "C", 3)
		_, store := applyReplay(t, Store{Entries: []Entry{a, b}}, storeBytes(t, a, b), storeBytes(t, a, b), storeBytes(t, b, c))
		if _, held := store.Get("a"); held {
			t.Error("a was not dropped")
		}
		if got := store.Entries; len(got) != 2 || got[0].Slug != "b" || got[1].Slug != "c" {
			t.Errorf("entries = %+v; want b then c", got)
		}
	})

	t.Run("UnversionedSidesReadClaudeFileAsAgentsFile", func(t *testing.T) {
		t.Parallel()
		unversionedA := []byte(`{"entries": [{"slug": "a", "text": "A", "order": 1, "targets": ["claude-file"]}]}`)
		unversionedAC := []byte(`{"entries": [` +
			`{"slug": "a", "text": "A", "order": 1, "targets": ["claude-file"]},` +
			`{"slug": "c", "text": "C", "order": 3, "targets": ["claude-file"]}]}`)
		// Origin aims a at a different set, so a parent or commit left unmigrated shows as a lost targets change.
		origin := entryFor("a", "A", 1, "agents-file", "claude-prompt")
		result, store := applyReplay(t, Store{Entries: []Entry{origin}}, unversionedA, unversionedA, unversionedAC)
		added, _ := store.Get("c")
		if !slices.Equal(added.Targets, []string{"agents-file"}) || len(result.Lost) != 0 || len(result.Refused) != 0 {
			t.Errorf("added entry = %+v, result = %+v; want c read as agents-file with nothing lost or refused", added, result)
		}
		if kept, _ := store.Get("a"); !slices.Equal(kept.Targets, []string{"agents-file", "claude-prompt"}) {
			t.Errorf("a.Targets = %v; want origin's targets kept", kept.Targets)
		}
	})

	t.Run("UnversionedMergeBaseReadsClaudeFileAsAgentsFile", func(t *testing.T) {
		t.Parallel()
		base := []byte(`{"entries": [{"slug": "a", "text": "A", "order": 1, "targets": ["claude-file"]}]}`)
		parent := []byte(`{"entries": [{"slug": "a", "text": "A", "order": 1, "targets": ["claude-prompt", "claude-file"]}]}`)
		commit := []byte(`{"entries": [{"slug": "a", "text": "A", "order": 1, "targets": ["claude-prompt"]}]}`)
		// Origin still holds the merge base's targets, so the commit's change applies only when the base reads as agents-file.
		origin := entryFor("a", "A", 1, "agents-file")
		result, store := applyReplay(t, Store{Entries: []Entry{origin}}, base, parent, commit)
		if got, _ := store.Get("a"); !slices.Equal(got.Targets, []string{"claude-prompt"}) || len(result.Lost) != 0 {
			t.Errorf("a.Targets = %v, result = %+v; want the commit's targets applied with nothing lost", got.Targets, result)
		}
	})

	t.Run("InvalidAddedEntryIsRefusedAndTheRestLands", func(t *testing.T) {
		t.Parallel()
		empty := replayEntry("c", "  ", 3)
		changed := replayEntry("a", "A local", 1)
		result, store := applyReplay(t, Store{Entries: []Entry{a, b}}, storeBytes(t, a, b), storeBytes(t, a, b), storeBytes(t, changed, b, empty))
		if len(result.Refused) != 1 || result.Refused[0].Entry != "c" {
			t.Errorf("refused = %+v; want c refused", result.Refused)
		}
		if _, held := store.Get("c"); held {
			t.Error("the refused entry was added")
		}
		if got, _ := store.Get("a"); got.Text != "A local" {
			t.Errorf("a.Text = %q; want the rest of the commit applied", got.Text)
		}
	})

	t.Run("CommitWithNoEntryChangeWritesNoStoreAndIsEmptyOnceTheOverviewStands", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		request := fabricengine.ReplayRequest{
			Dir:   dir,
			Files: []fabricengine.ReplayFile{{Path: FileName, Base: storeBytes(t, a), Before: storeBytes(t, a), After: storeBytes(t, a)}},
		}
		for _, wantEmpty := range []bool{false, true} {
			result, err := Replayer{}.Apply(request)
			if err != nil || result.Empty != wantEmpty {
				t.Fatalf("Apply() = %+v, %v; want Empty %v", result, err, wantEmpty)
			}
		}
		if got := loadStore(t, dir); len(got.Entries) != 0 {
			t.Errorf("store after an empty replay = %+v; want it unwritten", got)
		}
	})

	t.Run("AppliedChangeRendersTheOverviewFromWhatOriginHolds", func(t *testing.T) {
		t.Parallel()
		c := replayEntry("c", "C", 3)
		dir := seedOrigin(t, Store{Entries: []Entry{a, b}})
		result, err := Replayer{}.Apply(fabricengine.ReplayRequest{
			Dir:   dir,
			Files: []fabricengine.ReplayFile{{Path: FileName, Base: storeBytes(t, a, b), Before: storeBytes(t, a, b), After: storeBytes(t, a, b, c)}},
		})
		if err != nil || result.Empty {
			t.Fatalf("Apply() = %+v, %v; want a non-empty result", result, err)
		}
		data, err := os.ReadFile(filepath.Join(dir, OverviewFileName))
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(data), "### `c`\n") {
			t.Errorf("%s = %q; want it to name the entry the commit added", OverviewFileName, data)
		}
	})

	t.Run("CommitsOwnOverviewNeverLands", func(t *testing.T) {
		t.Parallel()
		c := replayEntry("c", "C", 3)
		dir := seedOrigin(t, Store{Entries: []Entry{a, b}})
		stale := []byte("stale overview\n")
		_, err := Replayer{}.Apply(fabricengine.ReplayRequest{
			Dir: dir,
			Files: []fabricengine.ReplayFile{
				{Path: FileName, Base: storeBytes(t, a, b), Before: storeBytes(t, a, b), After: storeBytes(t, a, b, c)},
				{Path: OverviewFileName, Base: stale, Before: stale, After: []byte("local overview\n")},
			},
		})
		if err != nil {
			t.Fatalf("Apply() = %v", err)
		}
		got, err := os.ReadFile(filepath.Join(dir, OverviewFileName))
		if err != nil {
			t.Fatal(err)
		}
		want := t.TempDir()
		saveStore(t, want, Store{Entries: []Entry{a, b, c}})
		if _, err := RenderOverview(want); err != nil {
			t.Fatalf("RenderOverview() = %v", err)
		}
		wantText, err := os.ReadFile(filepath.Join(want, OverviewFileName))
		if err != nil {
			t.Fatal(err)
		}
		if string(got) != string(wantText) {
			t.Errorf("%s = %q; want the overview rendered from origin's merged store, %q", OverviewFileName, got, wantText)
		}
	})

	t.Run("ChangeToAnEntryOriginNoLongerHoldsIsLost", func(t *testing.T) {
		t.Parallel()
		changed := replayEntry("a", "A local", 1)
		result, store := applyReplay(t, Store{Entries: []Entry{b}}, storeBytes(t, a, b), storeBytes(t, a, b), storeBytes(t, changed, b))
		if _, held := store.Get("a"); held || len(result.Lost) != 1 || result.Lost[0].Entry != "a" {
			t.Errorf("result = %+v, store = %+v; want a reported lost and not added back", result, store)
		}
	})

	t.Run("RemovalOfAnEntryOriginChangedIsLostAndTheEntryKept", func(t *testing.T) {
		t.Parallel()
		originChanged := replayEntry("a", "A origin", 1)
		result, store := applyReplay(t, Store{Entries: []Entry{originChanged, b}}, storeBytes(t, a, b), storeBytes(t, a, b), storeBytes(t, b))
		if got, held := store.Get("a"); !held || got.Text != "A origin" || len(result.Lost) != 1 {
			t.Errorf("result = %+v, store = %+v; want the removal lost and a kept as origin holds it", result, store)
		}
	})

	t.Run("EntryAddedOnBothSidesReportsEachDifferingFieldLost", func(t *testing.T) {
		t.Parallel()
		local, origin := replayEntry("c", "local text", 3), replayEntry("c", "origin text", 3)
		result, store := applyReplay(t, Store{Entries: []Entry{origin}}, storeBytes(t), storeBytes(t), storeBytes(t, local))
		if got, _ := store.Get("c"); got.Text != "origin text" {
			t.Errorf("c.Text = %q; want origin's", got.Text)
		}
		if len(result.Lost) != 1 || result.Lost[0].Field != "text" || !result.Empty {
			t.Errorf("lost = %+v; want only the text field lost", result.Lost)
		}
	})
}
