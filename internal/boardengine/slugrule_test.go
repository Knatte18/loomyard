//go:build tier0

// slugrule_test.go — the pair slug rule the board applies to the task and project slugs a write makes, through Config.ValidateSlug.

package boardengine_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/boardengine"
	"github.com/Knatte18/loomyard/internal/fabricengine"
)

// withRecordsSuffix returns base with the records suffix appended, a slug the pair slug rule refuses.
func withRecordsSuffix(base string) string {
	return fabricengine.RecordsBranchName(base)
}

// refusingSlugRule refuses a slug ending in the records suffix and the name _board, as the pair slug rule does.
func refusingSlugRule(slug string) error {
	if _, suffixed := fabricengine.WeftWarpSlug(slug); suffixed {
		return fmt.Errorf("slug %q ends in the records suffix", slug)
	}
	if slug == "_board" {
		return fmt.Errorf("slug %q is reserved", slug)
	}
	return nil
}

// TestSlugRule pins that a write making a task or project slug is refused when the seam refuses it, naming the entry, the rule's reason and a slug the rule admits,
// and that a write leaving an entry's slug and kind as they were is never judged.
func TestSlugRule(t *testing.T) {
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
		}, []string{"ends in the records suffix", `use the slug "n"`}, false},
		{"RenameToSuchASlug", func(b *boardengine.Board) error {
			_, err := b.Rename("renamable", withRecordsSuffix("r"))
			return err
		}, []string{"ends in the records suffix", `use the slug "r"`}, false},
		{"ReservedName", func(b *boardengine.Board) error {
			_, err := b.UpsertTask(map[string]any{"slug": "_board", "kind": "task", "labels": bugLabels})
			return err
		}, []string{"is reserved", `use the slug "_board-task"`}, false},
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
// It holds the task old with the records suffix as a slug a stored board can hold, the task renamable, and the task taken, which a suggestion can collide with.
func newSlugRuleBoard(t *testing.T) *boardengine.Board {
	t.Helper()
	var boardPath string
	b := newProjectBoard(t, func(cfg *boardengine.Config) {
		boardPath = cfg.Path
		cfg.ValidateSlug = refusingSlugRule
	})
	seedBoardJSON(t, boardPath,
		fmt.Sprintf(`{"id":0,"slug":%q,"title":"Old","kind":"task","labels":["bug"]}`, withRecordsSuffix("old")),
		`{"id":1,"slug":"renamable","title":"Renamable","kind":"task","labels":["bug"]}`,
		`{"id":2,"slug":"taken","title":"Taken","kind":"task","labels":["bug"]}`,
	)
	return b
}
