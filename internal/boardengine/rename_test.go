// rename_test.go — tests for Board.Rename: what a rename keeps, rewrites and reports, and what it refuses.

package boardengine_test

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/boardengine"
)

// newRenameBoard returns a board whose clock reads *now, holding other, old, waiter, prose, exact, starts, ends and near in that store order.
// old carries every field a rename must keep, a kept unknown key included, and mentions its own slug;
// waiter is a run-held task depending on other and old;
// prose, exact, starts and ends mention old as a whole slug mid-text, as the whole text, at its start and at its end.
// near holds it only beside a hyphen, underscore, letter or digit.
func newRenameBoard(t *testing.T, now *time.Time) (*boardengine.Board, string) {
	t.Helper()
	boardPath := t.TempDir()
	b := boardengine.New(boardengine.Config{Path: boardPath, Readme: "Home.md", DesignPrefix: "proposal-", Types: testTypes, Labels: testLabels, SkipGit: true,
		Clock: func() time.Time { return *now }})
	entries := []map[string]any{
		{"slug": "other", "title": "Other", "kind": boardengine.KindTask, "labels": bugLabels},
		{"slug": "old", "title": "Old", "kind": boardengine.KindTask, "labels": []string{"bug", "undecided"}, "priority": "high", "issues": []int{3},
			"brief": "short", "body": "The body names old here.", "recipe": "darn", "short_name": "o", "status": "active",
			"run_settings": map[string]any{"darn": map[string]any{"review_max_bounces": 1}}, "profile": "light"},
		{"slug": "waiter", "title": "Waiter", "kind": boardengine.KindTask, "labels": bugLabels, "depends_on": []string{"other", "old"}},
		{"slug": "prose", "title": "Prose", "labels": bugLabels, "body": "Picks up after old lands."},
		{"slug": "exact", "title": "Exact", "labels": bugLabels, "body": "old"},
		{"slug": "starts", "title": "Starts", "labels": bugLabels, "body": "old comes first"},
		{"slug": "ends", "title": "Ends", "labels": bugLabels, "brief": "comes after old"},
		{"slug": "near", "title": "Near", "labels": bugLabels, "brief": "old-thing, bold, old_x, old2 and 2old are other words"},
	}
	for _, fields := range entries {
		if _, err := b.UpsertTask(fields); err != nil {
			t.Fatalf("UpsertTask %v: %v", fields["slug"], err)
		}
	}
	if err := b.SetRunStatus("waiter", boardengine.RunStatus("running", "Webster")); err != nil {
		t.Fatalf("SetRunStatus: %v", err)
	}
	addEntryKeys(t, boardPath, "old", `"future_key":{"n":1.50}`)
	// A write re-indents the kept key's raw bytes, so the fixture is saved once to compare byte-equal entries.
	if err := b.Rerender(); err != nil {
		t.Fatalf("Rerender: %v", err)
	}
	return b, boardPath
}

// TestRename_KeepsEntryRewritesDependentsAndReportsMentions pins that a rename changes only the entry's slug, keeping its id, position, dates and kept keys.
// It also pins that the rename rewrites a run-held dependent's depends_on without revising it, renders under the new slug and reports the whole-slug mentions.
func TestRename_KeepsEntryRewritesDependentsAndReportsMentions(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 10, 1, 8, 0, 0, 0, time.UTC)
	b, boardPath := newRenameBoard(t, &now)
	before, _, err := b.GetTask("old")
	if err != nil {
		t.Fatalf("GetTask(old): %v", err)
	}
	waiterBefore, _, err := b.GetTask("waiter")
	if err != nil {
		t.Fatalf("GetTask(waiter): %v", err)
	}

	now = now.Add(time.Hour)
	result, err := b.Rename(before.ID, "new")
	if err != nil {
		t.Fatalf("Rename(%d, new): %v", before.ID, err)
	}

	want := before
	want.Slug = "new"
	if !reflect.DeepEqual(result.Task, want) {
		t.Errorf("Rename().Task = %+v; want %+v", result.Task, want)
	}
	if result.From != "old" {
		t.Errorf("Rename().From = %q; want %q", result.From, "old")
	}
	if wantMentions := []string{"new", "prose", "exact", "starts", "ends"}; !slices.Equal(result.Mentions, wantMentions) {
		t.Errorf("Rename().Mentions = %v; want %v", result.Mentions, wantMentions)
	}

	full, err := b.ListTasksFull()
	if err != nil {
		t.Fatalf("ListTasksFull: %v", err)
	}
	var slugs []string
	for _, task := range full {
		slugs = append(slugs, task.Slug)
	}
	if wantSlugs := []string{"other", "new", "waiter", "prose", "exact", "starts", "ends", "near"}; !slices.Equal(slugs, wantSlugs) {
		t.Errorf("store order after rename = %v; want %v", slugs, wantSlugs)
	}
	if !reflect.DeepEqual(full[1], want) {
		t.Errorf("stored renamed entry = %+v; want %+v", full[1], want)
	}
	waiterWant := waiterBefore
	waiterWant.DependsOn = []string{"other", "new"}
	if !reflect.DeepEqual(full[2], waiterWant) {
		t.Errorf("stored waiter = %+v; want %+v", full[2], waiterWant)
	}

	if _, err := os.Stat(filepath.Join(boardPath, "proposal-old.md")); !os.IsNotExist(err) {
		t.Errorf("design doc of the old slug must be gone; stat err = %v", err)
	}
	if _, err := os.Stat(filepath.Join(boardPath, "proposal-new.md")); err != nil {
		t.Errorf("design doc of the new slug is missing: %v", err)
	}
	readme, err := os.ReadFile(filepath.Join(boardPath, "Home.md"))
	if err != nil {
		t.Fatalf("read Home.md: %v", err)
	}
	if strings.Contains(string(readme), "`old`") || !strings.Contains(string(readme), "`new`") {
		t.Errorf("Home.md must name `new` and not `old`:\n%s", readme)
	}
}

// TestRename_Refusals pins every refusal of a rename, each leaving the board files as they were.
func TestRename_Refusals(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		selector   any
		to         string
		wantErrHas string
		wantLocked bool
	}{
		{"AbsentSource", "ghost", "fresh", "task not found", false},
		{"TargetExists", "old", "other", `"other" already names an entry`, false},
		{"TargetIsSource", "old", "old", `"old" already names an entry`, false},
		{"EmptyTarget", "old", "", "non-empty slug", false},
		{"TargetTooLong", "old", strings.Repeat("x", boardengine.MaxSlugLength+1), "pick a shorter target slug", false},
		{"RunHeldSource", "waiter", "fresh", `"waiter" is held by a run`, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			now := time.Date(2026, 10, 1, 8, 0, 0, 0, time.UTC)
			b, boardPath := newRenameBoard(t, &now)
			before := dataFiles(t, boardPath)

			_, err := b.Rename(tt.selector, tt.to)
			if err == nil || !strings.Contains(err.Error(), tt.wantErrHas) {
				t.Fatalf("Rename(%v, %q) error = %v; want one containing %q", tt.selector, tt.to, err, tt.wantErrHas)
			}
			if got := errors.Is(err, boardengine.ErrRunLocked); got != tt.wantLocked {
				t.Errorf("errors.Is(Rename(%v, %q), ErrRunLocked) = %v; want %v", tt.selector, tt.to, got, tt.wantLocked)
			}
			assertSameFiles(t, before, dataFiles(t, boardPath))
		})
	}
}
