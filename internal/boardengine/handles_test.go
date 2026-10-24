//go:build tier0

// handles_test.go — tests of entry handles: their assignment on write and load, and the Handle selector.

package boardengine_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/boardengine"
)

// handlesOf returns each entry's slug mapped to its handle, as the board reads.
func handlesOf(t *testing.T, b *boardengine.Board) map[string]string {
	t.Helper()
	tasks, err := b.ListTasksFull()
	if err != nil {
		t.Fatalf("ListTasksFull: %v", err)
	}
	handles := map[string]string{}
	for _, task := range tasks {
		handles[task.Slug] = task.Handle
	}
	return handles
}

// sameHandles reports whether got and want map the same slugs to the same handles.
func sameHandles(got, want map[string]string) bool {
	if len(got) != len(want) {
		return false
	}
	for slug, handle := range want {
		if got[slug] != handle {
			return false
		}
	}
	return true
}

// TestHandles_AssignedLowestFreeAndStable pins that a write gives a new task the lowest free T number and a new note the lowest free number of its type's letter.
// It pins that adding or removing an entry changes no other entry's handle and that a removed entry's number is given out again.
// It pins that a promote or a type change gives the entry the lowest free number of its new letter.
// Each step after the first addresses its entry by Handle, so the steps also pin that every selector-taking write finds an entry by handle, its letter in either case.
func TestHandles_AssignedLowestFreeAndStable(t *testing.T) {
	t.Parallel()
	b := boardengine.New(boardengine.Config{Path: t.TempDir(), Readme: "Home.md", DesignPrefix: "proposal-", Types: testTypes, Labels: testLabels, SkipGit: true})
	upsert := func(fields map[string]any) func() error {
		return func() error { _, err := b.UpsertTask(fields); return err }
	}

	steps := []struct {
		name  string
		write func() error
		want  map[string]string
	}{
		{"adds", func() error {
			for _, fields := range []map[string]any{
				{"slug": "t-one", "kind": "task", "labels": bugLabels},
				{"slug": "n-bug", "kind": "note", "labels": bugLabels, "priority": "high"},
				{"slug": "t-two", "kind": "task", "labels": bugLabels},
				{"slug": "n-idea", "kind": "note", "labels": []string{"enhancement"}},
				{"slug": "n-bug2", "kind": "note", "labels": bugLabels},
			} {
				if err := upsert(fields)(); err != nil {
					return err
				}
			}
			return nil
		}, map[string]string{"t-one": "T1", "n-bug": "B1", "t-two": "T2", "n-idea": "E1", "n-bug2": "B2"}},
		{"remove by handle frees its number", func() error { return b.RemoveTask(boardengine.Handle("T1")) },
			map[string]string{"n-bug": "B1", "t-two": "T2", "n-idea": "E1", "n-bug2": "B2"}},
		{"a new task takes the freed number", upsert(map[string]any{"slug": "t-three", "kind": "task", "labels": bugLabels}),
			map[string]string{"t-three": "T1", "n-bug": "B1", "t-two": "T2", "n-idea": "E1", "n-bug2": "B2"}},
		{"promote by handle gives the lowest free T", func() error { _, err := b.Promote(boardengine.Handle("b1")); return err },
			map[string]string{"t-three": "T1", "n-bug": "T3", "t-two": "T2", "n-idea": "E1", "n-bug2": "B2"}},
		{"a new note takes the number the promote freed", upsert(map[string]any{"slug": "n-bug3", "kind": "note", "labels": bugLabels}),
			map[string]string{"t-three": "T1", "n-bug": "T3", "t-two": "T2", "n-idea": "E1", "n-bug2": "B2", "n-bug3": "B1"}},
		{"a type change takes the new letter", upsert(map[string]any{"slug": "n-idea", "labels": bugLabels}),
			map[string]string{"t-three": "T1", "n-bug": "T3", "t-two": "T2", "n-idea": "B3", "n-bug2": "B2", "n-bug3": "B1"}},
		{"set-status, set-deps and rename by handle keep every handle", func() error {
			if err := b.SetStatus(boardengine.Handle("T2"), strPtr("active")); err != nil {
				return err
			}
			if _, err := b.SetDeps(boardengine.Handle("t2"), []string{"t-three"}); err != nil {
				return err
			}
			_, err := b.Rename(boardengine.Handle("B2"), "n-renamed")
			return err
		}, map[string]string{"t-three": "T1", "n-bug": "T3", "t-two": "T2", "n-idea": "B3", "n-renamed": "B2", "n-bug3": "B1"}},
	}
	for _, step := range steps {
		if err := step.write(); err != nil {
			t.Fatalf("%s: %v", step.name, err)
		}
		if got := handlesOf(t, b); !sameHandles(got, step.want) {
			t.Fatalf("after %s: handles = %v; want %v", step.name, got, step.want)
		}
	}

	got, found, err := b.GetTask(boardengine.Handle("t2"))
	if err != nil || !found || got.Slug != "t-two" || got.Status == nil || *got.Status != "active" || strings.Join(got.DependsOn, ",") != "t-three" {
		t.Errorf("GetTask(Handle t2) = %+v, found %v, err %v; want t-two, active, depending on t-three", got, found, err)
	}
	if _, found, _ := b.GetTask(boardengine.Handle("T9")); found {
		t.Errorf("GetTask(Handle T9) found an entry; want none")
	}
	if _, err := b.UpsertTask(map[string]any{"slug": "t-two", "handle": "T7"}); err == nil || !strings.Contains(err.Error(), "assigns an entry's handle") {
		t.Errorf("UpsertTask(handle) err = %v; want a refusal naming that the board assigns handles", err)
	}
}

// TestHandles_AssignedOnLoad pins that a board.json written before handles reads with them, in id order, keeping a valid stored handle and reassigning a duplicate,
// and that the first write persists them.
func TestHandles_AssignedOnLoad(t *testing.T) {
	t.Parallel()
	boardPath := t.TempDir()
	stored := `{"version":1,"entries":[
		{"id":5,"slug":"a","kind":"task","labels":["bug"]},
		{"id":2,"slug":"b","kind":"note","labels":["bug"]},
		{"id":9,"slug":"c","kind":"task","labels":["bug"],"handle":"T2"},
		{"id":1,"slug":"d","kind":"task","labels":["bug"]},
		{"id":10,"slug":"e","kind":"task","labels":["bug"],"handle":"T2"}]}`
	if err := os.WriteFile(filepath.Join(boardPath, "board.json"), []byte(stored), 0o644); err != nil {
		t.Fatalf("write board.json: %v", err)
	}
	b := boardengine.New(boardengine.Config{Path: boardPath, Readme: "Home.md", DesignPrefix: "proposal-", Types: testTypes, Labels: testLabels, SkipGit: true})
	want := map[string]string{"d": "T1", "b": "B1", "a": "T3", "c": "T2", "e": "T4"}

	if got := handlesOf(t, b); !sameHandles(got, want) {
		t.Errorf("handles before a write = %v; want %v", got, want)
	}
	brief, err := b.ListTasksBrief(nil)
	if err != nil || len(brief) != len(want) {
		t.Fatalf("ListTasksBrief = %+v, err %v; want %d entries", brief, err, len(want))
	}
	for _, entry := range brief {
		if entry.Handle != want[entry.Slug] {
			t.Errorf("ListTasksBrief entry %s has handle %q; want %q", entry.Slug, entry.Handle, want[entry.Slug])
		}
	}

	if err := b.Rerender(); err != nil {
		t.Fatalf("Rerender: %v", err)
	}
	raw, err := os.ReadFile(filepath.Join(boardPath, "board.json"))
	if err != nil {
		t.Fatalf("read board.json: %v", err)
	}
	if got := strings.Count(string(raw), `"handle"`); got != len(want) {
		t.Errorf("board.json names a handle %d times after a write; want %d:\n%s", got, len(want), raw)
	}
	if got := handlesOf(t, b); !sameHandles(got, want) {
		t.Errorf("handles after a write = %v; want %v", got, want)
	}
}

// TestHandles_UndecidedLetterKeepsNoDuplicate pins that a note whose type label is no longer configured keeps its stored handle.
// It pins that the note loses the handle when an entry of lower id already holds it.
func TestHandles_UndecidedLetterKeepsNoDuplicate(t *testing.T) {
	t.Parallel()
	boardPath := t.TempDir()
	stored := `{"version":1,"entries":[
		{"id":1,"slug":"decided","kind":"note","labels":["bug"],"handle":"B1"},
		{"id":2,"slug":"undecided-dup","kind":"note","labels":["removed-type"],"handle":"B1"},
		{"id":3,"slug":"undecided-free","kind":"note","labels":["removed-type"],"handle":"R7"}]}`
	if err := os.WriteFile(filepath.Join(boardPath, "board.json"), []byte(stored), 0o644); err != nil {
		t.Fatalf("write board.json: %v", err)
	}
	b := boardengine.New(boardengine.Config{Path: boardPath, Readme: "Home.md", DesignPrefix: "proposal-", Types: testTypes, Labels: testLabels, SkipGit: true})
	want := map[string]string{"decided": "B1", "undecided-dup": "", "undecided-free": "R7"}

	if got := handlesOf(t, b); !sameHandles(got, want) {
		t.Errorf("handles = %v; want %v", got, want)
	}
}
