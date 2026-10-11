// renumber_test.go pins what loomcli owns of the card verbs: the lease held around every apply step, the begun bound, and the wording of each refusal.
// The renumber itself is pinned by internal/planparser's tests.

package loomcli

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/planparser"
	"github.com/Knatte18/loomyard/internal/testkit/plankit"
	"github.com/Knatte18/loomyard/internal/websterengine"
)

// cardVerbFixture is a plan of three cards under a temp anchor whose scratch directories do not exist yet.
type cardVerbFixture struct {
	anchor  string
	planDir string
	deps    renumberDeps
}

func newCardVerbFixture(t *testing.T, cards ...plankit.Card) cardVerbFixture {
	t.Helper()
	if len(cards) == 0 {
		cards = []plankit.Card{
			{Number: 1, Slug: "alpha", Intent: "Stands apart; see card 2."},
			{Number: 2, Slug: "beta"},
			{Number: 3, Slug: "gamma"},
		}
	}
	anchor := t.TempDir()
	c := &loomCLI{}
	c.env.AnchorPath = anchor
	d := c.newRenumberDeps()
	plankit.Write(t, d.planDir, plankit.Plan{Cards: cards})
	return cardVerbFixture{anchor: anchor, planDir: d.planDir, deps: d}
}

// leaseWatch records the lease's acquisition and release and fails the test at any watched call made while it is not held.
type leaseWatch struct {
	t        *testing.T
	held     bool
	acquired int
	released int
}

// watch wraps d so its run-record read, its file seam and the verb's output are checked against the lease.
func (w *leaseWatch) watch(d renumberDeps) renumberDeps {
	acquire, load, ops := d.acquireLease, d.loadState, d.ops
	d.acquireLease = func() (func() error, error) {
		release, err := acquire()
		if err != nil {
			return nil, err
		}
		w.held = true
		w.acquired++
		return func() error {
			w.held = false
			w.released++
			return release()
		}, nil
	}
	d.loadState = func() (*websterengine.State, error) {
		w.require("loadState")
		return load()
	}
	d.ops.ReadFile = func(name string) ([]byte, error) { w.require("ReadFile"); return ops.ReadFile(name) }
	d.ops.WriteFile = func(name string, data []byte, perm os.FileMode) error {
		w.require("WriteFile")
		return ops.WriteFile(name, data, perm)
	}
	d.ops.Rename = func(oldpath, newpath string) error { w.require("Rename"); return ops.Rename(oldpath, newpath) }
	d.ops.Remove = func(name string) error { w.require("Remove"); return ops.Remove(name) }
	d.ops.MkdirAll = func(path string, perm os.FileMode) error { w.require("MkdirAll"); return ops.MkdirAll(path, perm) }
	return d
}

func (w *leaseWatch) require(call string) {
	w.t.Helper()
	if !w.held {
		w.t.Errorf("%s reached while the lease is not held", call)
	}
}

// Write fails the test when the verb prints while it still holds the lease.
func (w *leaseWatch) Write(p []byte) (int, error) {
	if w.held {
		w.t.Errorf("output printed while the lease is still held")
	}
	return len(p), nil
}

// failingOps returns d's file seam with the first times calls of op on the file with the given base name failing.
func failingOps(ops planparser.RenumberIO, op, base string, times int) planparser.RenumberIO {
	remaining := times
	fail := func(name string) error {
		if filepath.Base(name) == base && remaining > 0 {
			remaining--
			return fmt.Errorf("injected %s failure on %s", op, base)
		}
		return nil
	}
	switch op {
	case "ReadFile":
		real := ops.ReadFile
		ops.ReadFile = func(name string) ([]byte, error) {
			if err := fail(name); err != nil {
				return nil, err
			}
			return real(name)
		}
	case "WriteFile":
		real := ops.WriteFile
		ops.WriteFile = func(name string, data []byte, perm os.FileMode) error {
			if err := fail(name); err != nil {
				return err
			}
			return real(name, data, perm)
		}
	case "Remove":
		real := ops.Remove
		ops.Remove = func(name string) error {
			if err := fail(name); err != nil {
				return err
			}
			return real(name)
		}
	}
	return ops
}

// cardVerbRun runs one card verb against d, printing on out, and returns its exit code.
type cardVerbRun func(d renumberDeps, out io.Writer) int

func runInsert(position string) cardVerbRun {
	return func(d renumberDeps, out io.Writer) int { return insertCardVerb(out, d, position, "new", "a new card") }
}

func runRemove(position string) cardVerbRun {
	return func(d renumberDeps, out io.Writer) int { return removeCardVerb(out, d, position) }
}

func runRestore(d renumberDeps, out io.Writer) int { return restoreRenumberVerb(out, d) }

func decodeEnvelope(t *testing.T, raw []byte) map[string]any {
	t.Helper()
	var envelope map[string]any
	if err := json.Unmarshal(raw, &envelope); err != nil {
		t.Fatalf("output %q is not one JSON envelope: %v", raw, err)
	}
	return envelope
}

// planDirTree returns the content of every file under dir keyed by relative path.
func planDirTree(t *testing.T, dir string) map[string]string {
	t.Helper()
	files := map[string]string{}
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(dir, path)
		files[rel] = string(data)
		return err
	})
	if err != nil {
		t.Fatalf("walk %s: %v", dir, err)
	}
	return files
}

// interruptInsert leaves a journal on disk by failing the reversal of an insert.
func interruptInsert(t *testing.T, d renumberDeps) renumberDeps {
	t.Helper()
	failing := d
	failing.ops = failingOps(d.ops, "WriteFile", "00-overview.md", 2)
	var out bytes.Buffer
	if code := insertCardVerb(&out, failing, "2", "new", "a new card"); code != 1 {
		t.Fatalf("interrupted insert exit = %d (%s); want 1", code, out.String())
	}
	return d
}

func TestRenumberVerbs(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		// prepare may leave state for the verb and returns the deps to run it with.
		prepare func(t *testing.T, d renumberDeps) renumberDeps
		run     cardVerbRun
		check   func(t *testing.T, envelope map[string]any)
	}{
		{
			name: "insert reports moves, the new file and prose mentions",
			run:  runInsert("2"),
			check: func(t *testing.T, e map[string]any) {
				if e["created"] != "02-new.md" {
					t.Errorf("created = %v; want 02-new.md", e["created"])
				}
				moved, _ := e["moved"].([]any)
				if len(moved) != 2 || moved[0].(map[string]any)["old_file"] != "03-gamma.md" || moved[0].(map[string]any)["new_file"] != "04-gamma.md" || moved[0].(map[string]any)["old_number"] != float64(3) || moved[0].(map[string]any)["new_number"] != float64(4) {
					t.Errorf("moved = %v; want gamma and beta moved up by one, gamma first", e["moved"])
				}
				mentions, _ := e["prose_mentions"].([]any)
				if len(mentions) != 1 || mentions[0].(map[string]any)["file"] != "01-alpha.md" || mentions[0].(map[string]any)["text"] != "**Intent:** Stands apart; see card 2." {
					t.Errorf("prose_mentions = %v; want alpha's line naming card 2", e["prose_mentions"])
				}
			},
		},
		{
			name: "remove reports the removed file",
			run:  runRemove("2"),
			check: func(t *testing.T, e map[string]any) {
				if e["removed"] != "02-beta.md" {
					t.Errorf("removed = %v; want 02-beta.md", e["removed"])
				}
				if moved, _ := e["moved"].([]any); len(moved) != 1 {
					t.Errorf("moved = %v; want gamma moved down by one", e["moved"])
				}
			},
		},
		{
			name:    "restore after a failed reversal reports what it restored and removed",
			prepare: interruptInsert,
			run:     runRestore,
			check: func(t *testing.T, e map[string]any) {
				if restored, _ := e["restored"].([]any); !slices.Contains(restored, any("00-overview.md")) {
					t.Errorf("restored = %v; want it to hold the overview", e["restored"])
				}
				if removed, _ := e["removed"].([]any); !slices.Contains(removed, any("02-new.md")) {
					t.Errorf("removed = %v; want it to hold the inserted card", e["removed"])
				}
				if e["journal"] == "" || e["plan_dir"] == "" {
					t.Errorf("envelope = %v; want journal and plan_dir", e)
				}
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			fx := newCardVerbFixture(t)
			d := fx.deps
			if tc.prepare != nil {
				d = tc.prepare(t, d)
			}
			watch := &leaseWatch{t: t}

			var out bytes.Buffer
			code := tc.run(watch.watch(d), io.MultiWriter(&out, watch))

			if code != 0 {
				t.Fatalf("exit = %d (%s); want 0", code, out.String())
			}
			if watch.acquired != 1 || watch.released != 1 || watch.held {
				t.Errorf("lease acquired %d, released %d, held %v; want one acquisition, one release and not held", watch.acquired, watch.released, watch.held)
			}
			tc.check(t, decodeEnvelope(t, out.Bytes()))
		})
	}
}

func TestRenumberVerbs_Refusals(t *testing.T) {
	t.Parallel()

	doneBatch := map[int]*websterengine.BatchState{1: {Cards: []string{"01-alpha", "02-beta"}, Terminal: true, Status: websterengine.DigestStatusDone}}
	openBatch := map[int]*websterengine.BatchState{1: {Cards: []string{"01-alpha"}}}
	withState := func(batches map[int]*websterengine.BatchState) func(*testing.T, renumberDeps) renumberDeps {
		return func(_ *testing.T, d renumberDeps) renumberDeps {
			d.loadState = func() (*websterengine.State, error) { return &websterengine.State{Batches: batches}, nil }
			return d
		}
	}
	withJournalRead := func(op, base string, times int) func(*testing.T, renumberDeps) renumberDeps {
		return func(_ *testing.T, d renumberDeps) renumberDeps {
			d.ops = failingOps(d.ops, op, base, times)
			return d
		}
	}

	tests := []struct {
		name    string
		prepare func(t *testing.T, d renumberDeps) renumberDeps
		run     cardVerbRun
		// want lists fragments the message must hold: the cause and the way forward.
		want []string
		// untouched asserts the plan directory is byte-identical after the refusal.
		untouched bool
		// then are the other verbs run afterwards, each refusing with the same way forward.
		then []cardVerbRun
	}{
		{name: "insert: non-integer position", run: runInsert("two"), want: []string{"insert-card", `"two" is not an integer`, "way forward: pass a position from first_card to the last card, or one past it for insert-card"}},
		{name: "remove: non-integer position", run: runRemove("two"), want: []string{"remove-card", `"two" is not an integer`, "way forward: pass a position from first_card to the last card"}},
		{name: "insert: position out of range", run: runInsert("9"), want: []string{"way forward: pass a position from first_card to the last card, or one past it for insert-card"}, untouched: true},
		{name: "remove: position out of range", run: runRemove("9"), want: []string{"way forward: pass a position from first_card to the last card"}, untouched: true},
		{name: "insert: invalid slug", run: func(d renumberDeps, out io.Writer) int { return insertCardVerb(out, d, "2", "Bad/Slug", "x") }, want: []string{"way forward: pass lowercase letters and digits joined by single hyphens"}, untouched: true},
		{name: "insert: invalid intent", run: func(d renumberDeps, out io.Writer) int { return insertCardVerb(out, d, "2", "new", " ") }, want: []string{"way forward: pass a non-empty one-line --intent"}, untouched: true},
		{name: "insert: at a done card", prepare: withState(doneBatch), run: runInsert("2"), want: []string{"card 2, the last card webster has begun", "way forward: a begun card is frozen, so insert or remove after card 2"}, untouched: true},
		{name: "remove: at a done card", prepare: withState(doneBatch), run: runRemove("1"), want: []string{"way forward: a begun card is frozen, so insert or remove after card 2"}, untouched: true},
		{name: "insert: at a batch begun and not terminal", prepare: withState(openBatch), run: runInsert("1"), want: []string{"card 1, the last card webster has begun", "way forward: a begun card is frozen, so insert or remove after card 1"}, untouched: true},
		{name: "remove: at a batch begun and not terminal", prepare: withState(openBatch), run: runRemove("1"), want: []string{"way forward: a begun card is frozen, so insert or remove after card 1"}, untouched: true},
		{name: "insert: run record unreadable", prepare: func(_ *testing.T, d renumberDeps) renumberDeps {
			d.loadState = func() (*websterengine.State, error) { return nil, errors.New("state.json is malformed") }
			return d
		}, run: runInsert("2"), want: []string{"state.json is malformed", "way forward: transient, re-run the refused verb; nothing is mutated"}, untouched: true},
		{name: "remove: lease unavailable", prepare: func(_ *testing.T, d renumberDeps) renumberDeps {
			d.acquireLease = func() (func() error, error) { return nil, errors.New("lock held") }
			return d
		}, run: runRemove("2"), want: []string{"lock held", "way forward: transient, re-run the refused verb; nothing is mutated"}, untouched: true},
		{name: "insert: journal unreadable", prepare: withJournalRead("ReadFile", "renumber-journal.json", 1), run: runInsert("2"), want: []string{"way forward: transient, re-run the refused verb; nothing is mutated"}, untouched: true},
		{name: "insert: journal present", prepare: interruptInsert, run: runInsert("2"), want: []string{"a renumber journal already exists", "way forward: run lyx loom restore-renumber, then re-run the verb"}},
		{name: "remove: journal present", prepare: interruptInsert, run: runRemove("2"), want: []string{"way forward: run lyx loom restore-renumber, then re-run the verb"}},
		{name: "insert: failed reversal", prepare: func(t *testing.T, d renumberDeps) renumberDeps {
			d.ops = failingOps(d.ops, "WriteFile", "00-overview.md", 2)
			return d
		}, run: runInsert("2"), want: []string{"renumber reversal failed", "way forward: run lyx loom restore-renumber"}, then: []cardVerbRun{runRemove("2"), runInsert("2")}},
		{name: "insert: plan does not parse", prepare: func(t *testing.T, d renumberDeps) renumberDeps {
			if err := os.Remove(filepath.Join(d.planDir, "00-overview.md")); err != nil {
				t.Fatal(err)
			}
			return d
		}, run: runInsert("2"), want: []string{"plan overview not found", "way forward: fix the file the message names so lyx loom validate-plan parses it, then re-run the verb"}},
		{name: "remove: index and card files disagree", prepare: func(t *testing.T, d renumberDeps) renumberDeps {
			plankit.WriteTree(t, d.planDir, map[string]string{"09-orphan.md": "# Card 9 — orphan\n"})
			return d
		}, run: runRemove("2"), want: []string{"index-file-mismatch", "way forward: run lyx loom validate-plan and fix its index-file-mismatch and card-numbering findings, then re-run the verb"}},
		{name: "restore: no journal", run: runRestore, want: []string{"no renumber journal to restore", "way forward: nothing to restore, lyx loom insert-card or lyx loom remove-card is the verb to run"}, untouched: true},
		{name: "restore: journal does not decode", prepare: func(t *testing.T, d renumberDeps) renumberDeps {
			plankit.WriteTree(t, filepath.Dir(d.journalPath), map[string]string{"renumber-journal.json": "not json"})
			return d
		}, run: runRestore, want: []string{"does not decode", "way forward: delete the named journal file, then run lyx loom validate-plan, whose findings name any card left inconsistent"}, untouched: true},
		{name: "restore: journal names a path outside the plan directory", prepare: func(t *testing.T, d renumberDeps) renumberDeps {
			plankit.WriteTree(t, filepath.Dir(d.journalPath), map[string]string{"renumber-journal.json": `{"files":{},"created":["../x.md"]}`})
			return d
		}, run: runRestore, want: []string{"../x.md", "way forward: delete the named journal file"}, untouched: true},
		{name: "restore: failure partway", prepare: func(t *testing.T, d renumberDeps) renumberDeps {
			interruptInsert(t, d)
			d.ops = failingOps(d.ops, "WriteFile", "03-gamma.md", 1)
			return d
		}, run: runRestore, want: []string{"way forward: transient, re-run lyx loom restore-renumber, the journal is kept"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			fx := newCardVerbFixture(t)
			d := fx.deps
			if tc.prepare != nil {
				d = tc.prepare(t, d)
			}
			before := planDirTree(t, fx.planDir)

			var out bytes.Buffer
			code := tc.run(d, &out)

			if code != 1 {
				t.Fatalf("exit = %d (%s); want 1", code, out.String())
			}
			message, _ := decodeEnvelope(t, out.Bytes())["error"].(string)
			for _, fragment := range tc.want {
				if !strings.Contains(message, fragment) {
					t.Errorf("message %q does not contain %q", message, fragment)
				}
			}
			if tc.untouched && !maps.Equal(before, planDirTree(t, fx.planDir)) {
				t.Errorf("the plan directory changed")
			}
			for _, next := range tc.then {
				var nextOut bytes.Buffer
				if code := next(d, &nextOut); code != 1 {
					t.Fatalf("follow-up exit = %d (%s); want 1", code, nextOut.String())
				}
				nextMessage, _ := decodeEnvelope(t, nextOut.Bytes())["error"].(string)
				if !strings.Contains(nextMessage, "way forward: run lyx loom restore-renumber") {
					t.Errorf("follow-up message %q does not name restore-renumber", nextMessage)
				}
			}
		})
	}
}
