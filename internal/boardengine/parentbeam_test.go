//go:build tier0

// parentbeam_test.go — the board's half of the parent resolver: ParentBeam over a list of entries, and the parent_beam a Board's list and ParentBeamOf carry.

package boardengine_test

import (
	"errors"
	"testing"

	"github.com/Knatte18/loomyard/internal/boardengine"
)

// TestParentBeam pins the beam an entry resolves to over a plain list, and the parent_beam a Board's list and ParentBeamOf carry from its Config.ParentOf seam.
func TestParentBeam(t *testing.T) {
	t.Parallel()
	done := "done"
	entry := func(slug, kind, project string, beam bool, status *string) boardengine.Task {
		return boardengine.Task{Slug: slug, Kind: kind, Project: project, Beam: beam, Status: status}
	}
	tests := []struct {
		name  string
		tasks []boardengine.Task
		slug  string
		want  string
	}{
		{"NoProjectIsThePrime", []boardengine.Task{entry("a", boardengine.KindTask, "", false, nil)}, "a", ""},
		{"AMemberOfABeamReturnsTheBeam", []boardengine.Task{
			entry("beam", boardengine.KindProject, "", true, nil),
			entry("a", boardengine.KindTask, "beam", false, nil),
		}, "a", "beam"},
		{"ATaskUnderAGroupingProjectUnderABeamReturnsTheBeam", []boardengine.Task{
			entry("beam", boardengine.KindProject, "", true, nil),
			entry("group", boardengine.KindProject, "beam", false, nil),
			entry("a", boardengine.KindTask, "group", false, nil),
		}, "a", "beam"},
		{"AMemberWhoseOnlyBeamIsDoneIsThePrime", []boardengine.Task{
			entry("beam", boardengine.KindProject, "", true, &done),
			entry("a", boardengine.KindTask, "beam", false, nil),
		}, "a", ""},
		{"AGroupingProjectAloneIsThePrime", []boardengine.Task{
			entry("group", boardengine.KindProject, "", false, nil),
			entry("a", boardengine.KindTask, "group", false, nil),
		}, "a", ""},
		{"NestedBeamsReturnTheNearest", []boardengine.Task{
			entry("outer", boardengine.KindProject, "", true, nil),
			entry("inner", boardengine.KindProject, "outer", true, nil),
			entry("a", boardengine.KindTask, "inner", false, nil),
		}, "a", "inner"},
		{"ADoneBeamIsSkippedForTheOneAboveIt", []boardengine.Task{
			entry("outer", boardengine.KindProject, "", true, nil),
			entry("inner", boardengine.KindProject, "outer", true, &done),
			entry("a", boardengine.KindTask, "inner", false, nil),
		}, "a", "outer"},
		{"AProjectChainWithACycleIsThePrime", []boardengine.Task{
			entry("one", boardengine.KindProject, "two", true, nil),
			entry("two", boardengine.KindProject, "one", true, nil),
			entry("a", boardengine.KindTask, "one", false, nil),
		}, "a", ""},
		{"ASlugTheListDoesNotHoldIsThePrime", []boardengine.Task{entry("a", boardengine.KindTask, "", false, nil)}, "ghost", ""},
		{"AProjectNamingNoEntryIsThePrime", []boardengine.Task{entry("a", boardengine.KindTask, "ghost", false, nil)}, "a", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := boardengine.ParentBeam(tt.tasks, tt.slug); got != tt.want {
				t.Errorf("ParentBeam(%q) = %q; want %q", tt.slug, got, tt.want)
			}
		})
	}

	t.Run("BoardCarriesTheParentBeamTheSeamAnswers", func(t *testing.T) {
		t.Parallel()
		answers := map[string]string{"a": "beam"}
		b := newProjectBoard(t, func(cfg *boardengine.Config) {
			seedBoardJSON(t, cfg.Path,
				`{"id":0,"slug":"a","title":"A","kind":"task","labels":["bug"]}`,
				`{"id":1,"slug":"b","title":"B","kind":"task","labels":["bug"]}`,
			)
			cfg.ParentOf = func(slug string) (string, error) { return answers[slug], nil }
		})
		listed, err := b.ListTasksBrief(nil)
		if err != nil {
			t.Fatalf("ListTasksBrief() error = %v; want nil", err)
		}
		got := map[string]string{}
		for _, brief := range listed {
			got[brief.Slug] = brief.ParentBeam
		}
		if got["a"] != "beam" || got["b"] != "" {
			t.Errorf("listed parent_beam = %v; want a under beam and b under the prime", got)
		}
		if beam, err := b.ParentBeamOf("a"); err != nil || beam != "beam" {
			t.Errorf("ParentBeamOf(a) = %q, %v; want beam", beam, err)
		}
	})

	t.Run("BoardWithoutASeamReadsItsOwnEntries", func(t *testing.T) {
		t.Parallel()
		b := newProjectBoard(t, func(cfg *boardengine.Config) {
			seedBoardJSON(t, cfg.Path,
				`{"id":0,"slug":"beam","title":"Beam","kind":"project","labels":["bug"],"beam":true}`,
				`{"id":1,"slug":"a","title":"A","kind":"task","labels":["bug"],"project":"beam"}`,
			)
		})
		if beam, err := b.ParentBeamOf("a"); err != nil || beam != "beam" {
			t.Errorf("ParentBeamOf(a) = %q, %v; want beam", beam, err)
		}
		listed, err := b.ListTasksBrief(nil)
		if err != nil {
			t.Fatalf("ListTasksBrief() error = %v; want nil", err)
		}
		for _, brief := range listed {
			if want := map[string]string{"a": "beam", "beam": ""}[brief.Slug]; brief.ParentBeam != want {
				t.Errorf("listed parent_beam of %s = %q; want %q", brief.Slug, brief.ParentBeam, want)
			}
		}
	})

	t.Run("AFailingSeamLeavesTheCellEmptyAndTheListSucceeds", func(t *testing.T) {
		t.Parallel()
		errSeam := errors.New("origin record unreadable")
		b := newProjectBoard(t, func(cfg *boardengine.Config) {
			seedBoardJSON(t, cfg.Path, `{"id":0,"slug":"a","title":"A","kind":"task","labels":["bug"]}`)
			cfg.ParentOf = func(string) (string, error) { return "beam", errSeam }
		})
		listed, err := b.ListTasksBrief(nil)
		if err != nil || len(listed) != 1 || listed[0].ParentBeam != "" {
			t.Errorf("ListTasksBrief() = %+v, %v; want one entry with an empty parent_beam and no error", listed, err)
		}
		if _, err := b.ParentBeamOf("a"); !errors.Is(err, errSeam) {
			t.Errorf("ParentBeamOf(a) error = %v; want the seam's error", err)
		}
	})

	t.Run("ReadTasksReadsTheLocalFileAndAnAbsentOneIsEmpty", func(t *testing.T) {
		t.Parallel()
		var boardPath string
		newProjectBoard(t, func(cfg *boardengine.Config) {
			boardPath = cfg.Path
			seedBoardJSON(t, cfg.Path, `{"id":0,"slug":"a","title":"A","kind":"task","labels":["bug"]}`)
		})
		tasks, err := boardengine.ReadTasks(boardPath)
		if err != nil || len(tasks) != 1 || tasks[0].Slug != "a" {
			t.Errorf("ReadTasks() = %+v, %v; want entry a", tasks, err)
		}
		if tasks, err := boardengine.ReadTasks(t.TempDir()); err != nil || len(tasks) != 0 {
			t.Errorf("ReadTasks() on an absent file = %+v, %v; want an empty list", tasks, err)
		}
	})
}
