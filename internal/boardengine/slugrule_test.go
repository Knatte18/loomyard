// slugrule_test.go — the pair slug rule the board applies to the task and project slugs a write makes.
// The rule runs through Config.ValidateSlug.

package boardengine_test

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/boardengine"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// withRecordsSuffix returns base with the records suffix appended, a slug the pair slug rule refuses.
func withRecordsSuffix(base string) string {
	return fabricengine.RecordsBranchName(base)
}

// refusingSlugRule refuses a slug ending in the records suffix, and the reserved Bolt directory name.
func refusingSlugRule(slug string) error {
	if _, suffixed := fabricengine.WeftWarpSlug(slug); suffixed {
		return fmt.Errorf("slug %q ends in the records suffix", slug)
	}
	if slug == fabricengine.BoltDirName {
		return fmt.Errorf("slug %q is reserved", slug)
	}
	return nil
}

// TestSlugRule pins that a write making a task or project slug is refused when the seam refuses it.
// The refusal names the entry, the rule's reason and a slug the rule admits.
// A write that leaves an entry's slug and kind as they were is never judged.
func TestSlugRule(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	stored := withRecordsSuffix("old")

	refused := []struct {
		name    string
		write   func(b *boardengine.Board) error
		wantHas []string
		// wantNoSuggestion requires the refusal to offer no slug and to ask for another one.
		wantNoSuggestion bool
	}{
		{"NewTask", func(b *boardengine.Board) error {
			_, err := b.UpsertTask(map[string]any{"slug": withRecordsSuffix("x"), "kind": "task", "labels": bugLabels})
			return err
		}, []string{fmt.Sprintf("%q", withRecordsSuffix("x")), "ends in the records suffix", `use the slug "x"`}, false},
		{"NewProject", func(b *boardengine.Board) error {
			_, err := b.UpsertTask(map[string]any{"slug": withRecordsSuffix("x"), "kind": "project", "labels": bugLabels})
			return err
		}, []string{"ends in the records suffix", `use the slug "x"`}, false},
		{"NewTaskInABatch", func(b *boardengine.Board) error {
			_, err := b.UpsertTasksBatch([]map[string]any{{"slug": withRecordsSuffix("y"), "kind": "task", "labels": bugLabels}})
			return err
		}, []string{"ends in the records suffix", `use the slug "y"`}, false},
		{"MergeIntoANewTask", func(b *boardengine.Board) error {
			_, _, err := b.MergeTasks(nil, map[string]any{"slug": withRecordsSuffix("z"), "kind": "task", "labels": bugLabels}, nil)
			return err
		}, []string{"ends in the records suffix", `use the slug "z"`}, false},
		{"PromoteOfANote", func(b *boardengine.Board) error {
			if _, err := b.UpsertTask(map[string]any{"slug": withRecordsSuffix("n"), "kind": "note", "labels": bugLabels}); err != nil {
				return fmt.Errorf("seed note: %w", err)
			}
			_, err := b.Promote(withRecordsSuffix("n"))
			return err
		}, []string{"ends in the records suffix", "lym board rename", `"n"`}, false},
		{"RenameToSuchASlug", func(b *boardengine.Board) error {
			_, err := b.Rename("renamable", withRecordsSuffix("r"))
			return err
		}, []string{"ends in the records suffix", `use the slug "r"`}, false},
		{"ReservedName", func(b *boardengine.Board) error {
			_, err := b.UpsertTask(map[string]any{"slug": fabricengine.BoltDirName, "kind": "task", "labels": bugLabels})
			return err
		}, []string{"is reserved", `use the slug "` + fabricengine.BoltDirName + `-task"`}, false},
		{"SuggestionNamingAnExistingEntry", func(b *boardengine.Board) error {
			_, err := b.UpsertTask(map[string]any{"slug": withRecordsSuffix("taken"), "kind": "task", "labels": bugLabels})
			return err
		}, []string{"ends in the records suffix", "choose another slug"}, true},
	}
	for _, tt := range refused {
		t.Run("Refused_"+tt.name, func(t *testing.T) {
			t.Parallel()
			b := newSlugRuleBoard(t)
			err := tt.write(b)
			if err == nil {
				t.Fatal("write error = nil; want a refusal")
			}
			for _, want := range tt.wantHas {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("write error = %q; want it to contain %q", err, want)
				}
			}
			if tt.wantNoSuggestion && strings.Contains(err.Error(), "use the slug") {
				t.Errorf("write error = %q; want no slug offered, since the suggestion names an existing entry", err)
			}
		})
	}

	passes := []struct {
		name  string
		write func(b *boardengine.Board) error
	}{
		{"BodyEditOfAnExistingSuchTask", func(b *boardengine.Board) error {
			_, err := b.UpsertTask(map[string]any{"slug": stored, "body": "edited"})
			return err
		}},
		{"TaskBecomingAProjectKeepsItsSlug", func(b *boardengine.Board) error {
			_, err := b.UpsertTask(map[string]any{"slug": stored, "kind": "project"})
			return err
		}},
		{"NoteCreatedWithSuchASlug", func(b *boardengine.Board) error {
			_, err := b.UpsertTask(map[string]any{"slug": withRecordsSuffix("idea"), "kind": "note", "labels": bugLabels})
			return err
		}},
		{"TaskWithAnAdmittedSlug", func(b *boardengine.Board) error {
			_, err := b.UpsertTask(map[string]any{"slug": "fine", "kind": "task", "labels": bugLabels})
			return err
		}},
	}
	for _, tt := range passes {
		t.Run("Passes_"+tt.name, func(t *testing.T) {
			t.Parallel()
			if err := tt.write(newSlugRuleBoard(t)); err != nil {
				t.Errorf("write error = %v; want nil", err)
			}
		})
	}
}

// newSlugRuleBoard returns a board whose ValidateSlug seam is refusingSlugRule.
// It holds three tasks: old, with the records suffix as its slug, which a stored board can hold;
// renamable, a task a write may rename; and taken, which a suggestion can collide with.
func newSlugRuleBoard(t *testing.T) *boardengine.Board {
	t.Helper()
	b, _ := newSlugRuleBoardWith(t, refusingSlugRule)
	return b
}

// newSlugRuleBoardWith returns a board whose ValidateSlug seam is validate, which may be nil, and the board's directory.
func newSlugRuleBoardWith(t *testing.T, validate func(slug string) error) (*boardengine.Board, string) {
	t.Helper()
	var boardPath string
	b := newProjectBoard(t, func(cfg *boardengine.Config) {
		boardPath = cfg.Path
		cfg.ValidateSlug = validate
	})
	seedBoardJSON(t, boardPath,
		fmt.Sprintf(`{"id":0,"slug":%q,"title":"Old","kind":"task","labels":["bug"]}`, withRecordsSuffix("old")),
		`{"id":1,"slug":"renamable","title":"Renamable","kind":"task","labels":["bug"]}`,
		`{"id":2,"slug":"taken","title":"Taken","kind":"task","labels":["bug"]}`,
	)
	return b, boardPath
}

// writeRaddleStore writes store.json into the raddle store directory of sketch under boardDir.
// It serves the rows that need a store before a filing write exists.
func writeRaddleStore(t *testing.T, boardDir, sketch string, entry boardengine.RaddleStoreEntry) {
	t.Helper()
	data, err := json.Marshal(entry)
	if err != nil {
		t.Fatalf("marshal store entry: %v", err)
	}
	dir := boardengine.RaddleStoreDir(boardDir, sketch)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir store directory: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "store.json"), data, 0o644); err != nil {
		t.Fatalf("write store.json: %v", err)
	}
}

// TestSlugRule_RaddleStore pins that the board refuses a made slug the raddle store holds.
// The refusal covers a Sketch slug and a filed slug, with or without the pair slug seam.
// It admits the live run's own Sketch, and refuses only a slug-making write while a store file is unreadable.
func TestSlugRule_RaddleStore(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	// The board's fixed clock stamps every entry created in a row with this time.
	// A store entry recording that time belongs to such an entry's own Sketch.
	// A store entry recording an earlier time belongs to a Sketch filed before.
	boardClock := time.Date(2026, 10, 1, 8, 0, 0, 0, time.UTC)
	held := boardengine.RaddleStoreEntry{Created: boardClock.Add(-24 * time.Hour), Hash: "h", Filed: []string{"part-a", "part-b", "plan-proj"}, Project: "plan-proj"}
	own := held
	own.Created = boardClock
	discussedNote := func(b *boardengine.Board, slug string, labels []string) error {
		if _, err := b.UpsertTask(map[string]any{"slug": slug, "kind": "note", "labels": labels}); err != nil {
			return fmt.Errorf("seed note: %w", err)
		}
		return b.StoreDecisionRecord(slug, []byte("record\n"))
	}
	newTask := func(slug string) func(b *boardengine.Board) error {
		return func(b *boardengine.Board) error {
			_, err := b.UpsertTask(map[string]any{"slug": slug, "kind": "task", "labels": bugLabels})
			return err
		}
	}

	refused := []struct {
		name     string
		validate func(slug string) error
		write    func(b *boardengine.Board) error
		wantHas  []string
	}{
		{"NewTaskUnderAStoreHeldSketchSlug", refusingSlugRule, newTask("plan-me"), []string{`"plan-me"`, "Sketch", `use the slug "plan-me-task"`}},
		{"NewTaskUnderAFiledSlug", refusingSlugRule, newTask("part-a"), []string{`"part-a"`, `Sketch "plan-me"`, `use the slug "part-a-task"`}},
		{"NewProjectUnderAPrunedFiledProjectSlug", refusingSlugRule, func(b *boardengine.Board) error {
			_, err := b.UpsertTask(map[string]any{"slug": "plan-proj", "kind": "project", "labels": bugLabels})
			return err
		}, []string{`"plan-proj"`, `Sketch "plan-me"`}},
		{"RenameToAHeldSlug", refusingSlugRule, func(b *boardengine.Board) error {
			_, err := b.Rename("renamable", "plan-me")
			return err
		}, []string{`"plan-me"`, "Sketch"}},
		{"RaddlePromotionOfADiscussedNoteUnderAFiledSlug", refusingSlugRule, func(b *boardengine.Board) error {
			if err := discussedNote(b, "part-b", []string{boardengine.SketchType}); err != nil {
				return err
			}
			_, err := b.PromoteTo("part-b", "raddle")
			return err
		}, []string{`"part-b"`, `Sketch "plan-me"`, "lym board rename"}},
		{"NilSeamAndAStoreHit", nil, newTask("part-a"), []string{`"part-a"`, `Sketch "plan-me"`}},
	}
	for _, tt := range refused {
		t.Run("Refused_"+tt.name, func(t *testing.T) {
			t.Parallel()
			b, boardPath := newSlugRuleBoardWith(t, tt.validate)
			writeRaddleStore(t, boardPath, "plan-me", held)
			err := tt.write(b)
			if err == nil {
				t.Fatal("write error = nil; want a refusal")
			}
			for _, want := range tt.wantHas {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("write error = %q; want it to contain %q", err, want)
				}
			}
		})
	}

	t.Run("Passes_TheLiveRunsOwnSketch", func(t *testing.T) {
		t.Parallel()
		b, boardPath := newSlugRuleBoardWith(t, refusingSlugRule)
		if err := discussedNote(b, "plan-me", []string{boardengine.SketchType}); err != nil {
			t.Fatal(err)
		}
		writeRaddleStore(t, boardPath, "plan-me", own)
		if _, err := b.PromoteTo("plan-me", "raddle"); err != nil {
			t.Errorf("PromoteTo() error = %v; want nil for the Sketch whose created stamp the store records", err)
		}
	})

	t.Run("UnreadableStoreFile", func(t *testing.T) {
		t.Parallel()
		b, boardPath := newSlugRuleBoardWith(t, refusingSlugRule)
		dir := boardengine.RaddleStoreDir(boardPath, "broken")
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "store.json"), []byte("{not json"), 0o644); err != nil {
			t.Fatal(err)
		}

		err := newTask("fresh")(b)
		if err == nil || !strings.Contains(err.Error(), "store.json") || !strings.Contains(err.Error(), "checkout --") {
			t.Errorf("slug-making write error = %v; want a refusal naming store.json and the checkout", err)
		}
		status := "done"
		if err := b.SetStatus("renamable", &status); err != nil {
			t.Errorf("SetStatus() error = %v; want nil, since a write that makes no slug goes on", err)
		}
	})
}
