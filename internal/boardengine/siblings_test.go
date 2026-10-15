// siblings_test.go — the sibling check of a depends_on write (siblings.go), driven through the Board facade.

package boardengine_test

import (
	"errors"
	"maps"
	"reflect"
	"slices"
	"testing"

	"github.com/Knatte18/loomyard/internal/boardengine"
)

// siblingEntry returns the upsert fields of a task carrying the type label bug and the given module labels.
func siblingEntry(slug string, moduleLabels []string, extra map[string]any) map[string]any {
	fields := map[string]any{
		"slug": slug, "title": slug, "kind": "task",
		"labels": append([]string{"bug"}, moduleLabels...),
	}
	maps.Copy(fields, extra)
	return fields
}

// TestWriteSiblings pins which tasks a depends_on write reports as siblings, and that a refused write reports none.
func TestWriteSiblings(t *testing.T) {
	t.Parallel()

	board := []string{"board"}
	waitsOnD := map[string]any{"depends_on": []string{"D"}}
	running := "running · Webster"
	pair := boardengine.Sibling{Task: "X", Sibling: "Y", Deps: []string{"D"}, Labels: []string{"board"}}

	setDeps := func(selector string, deps ...string) func(b *boardengine.Board) ([]boardengine.Sibling, error) {
		return func(b *boardengine.Board) ([]boardengine.Sibling, error) { return b.SetDeps(selector, deps) }
	}
	upsert := func(fields map[string]any) func(b *boardengine.Board) ([]boardengine.Sibling, error) {
		return func(b *boardengine.Board) ([]boardengine.Sibling, error) {
			_, siblings, err := b.UpsertTaskSiblings(fields)
			return siblings, err
		}
	}
	merge := func(remove []string, fields map[string]any, status *boardengine.MergeStatusUpdate) func(b *boardengine.Board) ([]boardengine.Sibling, error) {
		return func(b *boardengine.Board) ([]boardengine.Sibling, error) {
			_, siblings, err := b.MergeTasks(remove, fields, status)
			return siblings, err
		}
	}

	rows := []struct {
		name string
		seed []map[string]any
		act  func(b *boardengine.Board) ([]boardengine.Sibling, error)
		want []boardengine.Sibling
		// stored maps a slug to the depends_on the board must hold after a successful write.
		stored map[string][]string
		// refused expects a non-nil error and no siblings; refusedWith, when set, must also match through errors.Is.
		refused     bool
		refusedWith error
	}{
		{
			name: "set-deps adding a dependency an open sibling waits on",
			seed: []map[string]any{siblingEntry("D", nil, nil), siblingEntry("X", board, nil), siblingEntry("Y", board, waitsOnD)},
			act:  setDeps("X", "D"), want: []boardengine.Sibling{pair}, stored: map[string][]string{"X": {"D"}},
		},
		{
			name: "upsert adding a dependency an open sibling waits on",
			seed: []map[string]any{siblingEntry("D", nil, nil), siblingEntry("X", board, nil), siblingEntry("Y", board, waitsOnD)},
			act:  upsert(map[string]any{"slug": "X", "depends_on": []string{"D"}}), want: []boardengine.Sibling{pair},
		},
		{
			name: "merge upserting a dependency an open sibling waits on",
			seed: []map[string]any{siblingEntry("D", nil, nil), siblingEntry("X", board, nil), siblingEntry("Y", board, waitsOnD)},
			act:  merge(nil, map[string]any{"slug": "X", "depends_on": []string{"D"}}, nil), want: []boardengine.Sibling{pair},
		},
		{
			name: "two siblings come in board order",
			seed: []map[string]any{siblingEntry("D", nil, nil), siblingEntry("X", board, nil), siblingEntry("Y1", board, waitsOnD), siblingEntry("Y2", board, waitsOnD)},
			act:  setDeps("X", "D"),
			want: []boardengine.Sibling{
				{Task: "X", Sibling: "Y1", Deps: []string{"D"}, Labels: board},
				{Task: "X", Sibling: "Y2", Deps: []string{"D"}, Labels: board},
			},
		},
		{
			name: "only the type label shared",
			seed: []map[string]any{siblingEntry("D", nil, nil), siblingEntry("X", nil, nil), siblingEntry("Y", nil, waitsOnD)},
			act:  setDeps("X", "D"),
		},
		{
			name: "sibling done",
			seed: []map[string]any{siblingEntry("D", nil, nil), siblingEntry("X", board, nil), siblingEntry("Y", board, map[string]any{"depends_on": []string{"D"}, "status": "done"})},
			act:  setDeps("X", "D"),
		},
		{
			name: "sibling abandoned",
			seed: []map[string]any{siblingEntry("D", nil, nil), siblingEntry("X", board, nil), siblingEntry("Y", board, map[string]any{"depends_on": []string{"D"}, "status": "abandoned"})},
			act:  setDeps("X", "D"),
		},
		{
			name: "sibling running",
			seed: []map[string]any{siblingEntry("D", nil, nil), siblingEntry("X", board, nil), siblingEntry("Y", board, map[string]any{"depends_on": []string{"D"}, "status": running})},
			act:  setDeps("X", "D"), want: []boardengine.Sibling{pair},
		},
		{
			name: "written task done",
			seed: []map[string]any{siblingEntry("D", nil, nil), siblingEntry("X", board, nil), siblingEntry("Y", board, waitsOnD)},
			act:  upsert(map[string]any{"slug": "X", "depends_on": []string{"D"}, "status": "done"}),
		},
		{
			name: "dependency done",
			seed: []map[string]any{siblingEntry("D", nil, map[string]any{"status": "done"}), siblingEntry("X", board, nil), siblingEntry("Y", board, waitsOnD)},
			act:  setDeps("X", "D"),
		},
		{
			name: "no other task waits on the dependency",
			seed: []map[string]any{siblingEntry("D", nil, nil), siblingEntry("X", board, nil)},
			act:  setDeps("X", "D"),
		},
		{
			name: "dependency already held and a new one nobody else waits on",
			seed: []map[string]any{siblingEntry("D", nil, nil), siblingEntry("E", nil, nil), siblingEntry("X", board, waitsOnD), siblingEntry("Y", board, waitsOnD)},
			act:  setDeps("X", "D", "E"),
		},
		{
			name: "only the newly added dependency is reported",
			seed: []map[string]any{siblingEntry("D", nil, nil), siblingEntry("E", nil, nil), siblingEntry("X", board, waitsOnD), siblingEntry("Y", board, map[string]any{"depends_on": []string{"D", "E"}})},
			act:  setDeps("X", "D", "E"), want: []boardengine.Sibling{{Task: "X", Sibling: "Y", Deps: []string{"E"}, Labels: board}},
		},
		{
			name: "upsert without depends_on adding the shared label",
			seed: []map[string]any{siblingEntry("D", nil, nil), siblingEntry("X", nil, waitsOnD), siblingEntry("Y", board, waitsOnD)},
			act:  upsert(map[string]any{"slug": "X", "labels": []string{"bug", "board"}}),
		},
		{
			name: "cleared dependency list",
			seed: []map[string]any{siblingEntry("D", nil, nil), siblingEntry("X", board, waitsOnD), siblingEntry("Y", board, waitsOnD)},
			act:  setDeps("X"),
		},
		{
			name: "labels as the write left them",
			seed: []map[string]any{siblingEntry("D", nil, nil), siblingEntry("X", board, nil), siblingEntry("Y", []string{"cli"}, waitsOnD)},
			act:  upsert(map[string]any{"slug": "X", "depends_on": []string{"D"}, "labels": []string{"bug", "cli"}}),
			want: []boardengine.Sibling{{Task: "X", Sibling: "Y", Deps: []string{"D"}, Labels: []string{"cli"}}},
		},
		{
			name: "batch reports a pair once with both sides' dependencies",
			seed: []map[string]any{siblingEntry("D", nil, nil), siblingEntry("E", nil, nil), siblingEntry("X", board, nil), siblingEntry("Z", board, map[string]any{"depends_on": []string{"E"}})},
			act: func(b *boardengine.Board) ([]boardengine.Sibling, error) {
				return b.UpsertTasksBatch([]map[string]any{
					{"slug": "X", "depends_on": []string{"D", "E"}},
					{"slug": "Z", "depends_on": []string{"E", "D"}},
				})
			},
			want: []boardengine.Sibling{{Task: "X", Sibling: "Z", Deps: []string{"D", "E"}, Labels: board}},
		},
		{
			name: "merge whose set_status marks the written task done",
			seed: []map[string]any{siblingEntry("D", nil, nil), siblingEntry("X", board, nil), siblingEntry("Y", board, waitsOnD)},
			act:  merge(nil, map[string]any{"slug": "X", "depends_on": []string{"D"}}, &boardengine.MergeStatusUpdate{Selector: "X", Status: strPtr("done")}),
		},
		{
			name: "merge removing the sibling",
			seed: []map[string]any{siblingEntry("D", nil, nil), siblingEntry("X", board, nil), siblingEntry("Y", board, waitsOnD)},
			act:  merge([]string{"Y"}, map[string]any{"slug": "X", "depends_on": []string{"D"}}, nil),
		},
		{
			name: "merge folding an entry that already waited on the dependency",
			seed: []map[string]any{siblingEntry("D", nil, nil), siblingEntry("A", board, waitsOnD), siblingEntry("C", board, waitsOnD)},
			act:  merge([]string{"A"}, siblingEntry("B", board, waitsOnD), nil),
		},
		{
			name: "dependency listed twice",
			seed: []map[string]any{siblingEntry("D", nil, nil), siblingEntry("X", board, nil), siblingEntry("Y", board, waitsOnD)},
			act:  setDeps("X", "D", "D"), want: []boardengine.Sibling{pair},
		},
		{
			name: "note refused a dependency",
			seed: []map[string]any{siblingEntry("D", nil, nil), siblingEntry("Y", board, waitsOnD), siblingEntry("N", nil, map[string]any{"kind": "note"})},
			act:  setDeps("N", "D"), refused: true,
		},
		{
			name: "unknown dependency refused",
			seed: []map[string]any{siblingEntry("D", nil, nil), siblingEntry("X", board, nil), siblingEntry("Y", board, waitsOnD)},
			act:  setDeps("X", "D", "ghost"), refused: true,
		},
		{
			name: "run-locked task refused after the siblings were computed",
			seed: []map[string]any{siblingEntry("D", nil, nil), siblingEntry("X", board, map[string]any{"status": running}), siblingEntry("Y", board, waitsOnD)},
			act:  setDeps("X", "D"), refused: true, refusedWith: boardengine.ErrRunLocked,
		},
	}
	for _, tt := range rows {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			cfg := boardengine.Config{
				Path: t.TempDir(), Readme: "Home.md", DesignPrefix: "proposal-", SkipGit: true,
				Types: testTypes, Labels: []boardengine.Label{{Name: "board"}, {Name: "cli"}},
			}
			b := boardengine.New(cfg)
			for _, fields := range tt.seed {
				if _, err := b.UpsertTask(fields); err != nil {
					t.Fatalf("seed %v: %v", fields["slug"], err)
				}
			}

			got, err := tt.act(b)
			if tt.refused {
				if err == nil {
					t.Fatalf("write err = nil; want a refusal")
				}
				if tt.refusedWith != nil && !errors.Is(err, tt.refusedWith) {
					t.Errorf("write err = %v; want errors.Is %v", err, tt.refusedWith)
				}
				if got != nil {
					t.Errorf("refused write siblings = %+v; want none", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("write: %v", err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("siblings = %+v; want %+v", got, tt.want)
			}
			for slug, want := range tt.stored {
				task, found, err := b.GetTask(slug)
				if err != nil || !found {
					t.Fatalf("GetTask(%q) = found %v, err %v", slug, found, err)
				}
				if !slices.Equal(task.DependsOn, want) {
					t.Errorf("%s depends_on = %v; want %v", slug, task.DependsOn, want)
				}
			}
		})
	}
}
