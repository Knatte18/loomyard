// layer_test.go — unit tests for derived fields (layer.go).
//
// ComputeLayers depth assignment and RenderOrder.

package boardengine_test

import (
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/boardengine"
)

func TestComputeLayers(t *testing.T) {
	tests := []struct {
		name      string
		tasks     []boardengine.Task
		want      map[string]string
		wantError bool
	}{
		{
			name: "single task no deps",
			tasks: []boardengine.Task{
				{ID: 1, Slug: "a", Title: "Task A", DependsOn: []string{}},
			},
			want: map[string]string{"a": "A"},
		},
		{
			name: "A depends on B",
			tasks: []boardengine.Task{
				{ID: 1, Slug: "a", Title: "Task A", DependsOn: []string{"b"}},
				{ID: 2, Slug: "b", Title: "Task B", DependsOn: []string{}},
			},
			want: map[string]string{"a": "B", "b": "A"},
		},
		{
			name: "done task excluded from depth",
			tasks: []boardengine.Task{
				{ID: 1, Slug: "a", Title: "Task A", DependsOn: []string{"b"}},
				{ID: 2, Slug: "b", Title: "Task B", DependsOn: []string{}, Status: stringPtr("done")},
			},
			want: map[string]string{"a": "A", "b": "__done__"},
		},
		{
			name: "running task excluded from depth along a chain",
			tasks: []boardengine.Task{
				{ID: 1, Slug: "run", Title: "Running", Status: stringPtr(boardengine.RunStatus("plan", "burler"))},
				{ID: 2, Slug: "x", Title: "Task X", DependsOn: []string{"run"}},
				{ID: 3, Slug: "y", Title: "Task Y", DependsOn: []string{"x"}},
			},
			want: map[string]string{"run": "A", "x": "A", "y": "B"},
		},
		{
			name: "isolated task",
			tasks: []boardengine.Task{
				{ID: 1, Slug: "a", Title: "Task A", Isolated: true},
			},
			want: map[string]string{"a": "Z"},
		},
		{
			name: "chain of 3",
			tasks: []boardengine.Task{
				{ID: 1, Slug: "a", Title: "Task A", DependsOn: []string{"b"}},
				{ID: 2, Slug: "b", Title: "Task B", DependsOn: []string{"c"}},
				{ID: 3, Slug: "c", Title: "Task C", DependsOn: []string{}},
			},
			want: map[string]string{"a": "C", "b": "B", "c": "A"},
		},
		{
			name: "depth exceeds A..Y cap",
			tasks: func() []boardengine.Task {
				var tasks []boardengine.Task
				for i := 0; i < 26; i++ {
					slug := ""
					switch i {
					case 0:
						slug = "a"
					case 1:
						slug = "b"
					case 2:
						slug = "c"
					case 3:
						slug = "d"
					case 4:
						slug = "e"
					case 5:
						slug = "f"
					case 6:
						slug = "g"
					case 7:
						slug = "h"
					case 8:
						slug = "i"
					case 9:
						slug = "j"
					case 10:
						slug = "k"
					case 11:
						slug = "l"
					case 12:
						slug = "m"
					case 13:
						slug = "n"
					case 14:
						slug = "o"
					case 15:
						slug = "p"
					case 16:
						slug = "q"
					case 17:
						slug = "r"
					case 18:
						slug = "s"
					case 19:
						slug = "t"
					case 20:
						slug = "u"
					case 21:
						slug = "v"
					case 22:
						slug = "w"
					case 23:
						slug = "x"
					case 24:
						slug = "y"
					case 25:
						slug = "z"
					}

					var deps []string
					if i > 0 {
						deps = []string{tasks[i-1].Slug}
					}
					tasks = append(tasks, boardengine.Task{
						ID:        i + 1,
						Slug:      slug,
						Title:     "Task " + slug,
						DependsOn: deps,
					})
				}
				return tasks
			}(),
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := boardengine.ComputeLayers(tt.tasks)
			if (err != nil) != tt.wantError {
				t.Fatalf("ComputeLayers() error = %v, wantError %v", err, tt.wantError)
			}
			if err != nil {
				return
			}
			if len(got) != len(tt.want) {
				t.Fatalf("ComputeLayers() got %d entries, want %d", len(got), len(tt.want))
			}
			for slug, wantLayer := range tt.want {
				if gotLayer, ok := got[slug]; !ok {
					t.Errorf("ComputeLayers() missing slug %q", slug)
				} else if gotLayer != wantLayer {
					t.Errorf("ComputeLayers() for slug %q got %q, want %q", slug, gotLayer, wantLayer)
				}
			}
		})
	}
}

// TestRenderOrder asserts RenderOrder's bucket order, tasks before notes with done entries last, and ID order within a bucket.
//
//testtiming:keep pins the bucket order, the task-before-note order and the ID sort within a bucket, which the render goldens do not assert
func TestRenderOrder(t *testing.T) {
	tests := []struct {
		name  string
		tasks []boardengine.Task
		check func(t *testing.T, result []boardengine.TaskWithLayer)
	}{
		{
			name: "buckets in correct order",
			tasks: []boardengine.Task{
				{ID: 1, Slug: "done1", Title: "Done Task", Status: stringPtr("done")},
				{ID: 3, Slug: "z1", Title: "Isolated Task", Isolated: true},
				{ID: 4, Slug: "a1", Title: "Layer A Task", DependsOn: []string{}},
				{ID: 5, Slug: "b1", Title: "Layer B Task", DependsOn: []string{"a1"}},
			},
			check: func(t *testing.T, result []boardengine.TaskWithLayer) {
				if len(result) != 4 {
					t.Fatalf("RenderOrder() got %d tasks, want 4", len(result))
				}
				// Expected order: a1(A), b1(B), z1(Z), done1(__done__)
				wantOrder := []string{"a1", "b1", "z1", "done1"}
				wantLayers := []string{"A", "B", "Z", "__done__"}
				for i, slug := range wantOrder {
					if result[i].Slug != slug {
						t.Errorf("RenderOrder() position %d got slug %q, want %q", i, result[i].Slug, slug)
					}
					if result[i].Layer != wantLayers[i] {
						t.Errorf("RenderOrder() position %d got layer %q, want %q", i, result[i].Layer, wantLayers[i])
					}
				}
			},
		},
		{
			name: "open tasks sort before open notes, then layer, and done goes last",
			tasks: []boardengine.Task{
				{ID: 1, Slug: "done1", Title: "Done", Kind: boardengine.KindTask, Status: stringPtr("done")},
				{ID: 2, Slug: "n2", Title: "Note 2", Kind: boardengine.KindNote},
				{ID: 3, Slug: "t1b", Title: "Task dependent", Kind: boardengine.KindTask, DependsOn: []string{"t1a"}},
				{ID: 4, Slug: "t1a", Title: "Task root", Kind: boardengine.KindTask},
				{ID: 5, Slug: "n1", Title: "Note 1", Kind: boardengine.KindNote},
				{ID: 6, Slug: "done-note", Title: "Done note", Kind: boardengine.KindNote, Status: stringPtr("done")},
			},
			check: func(t *testing.T, result []boardengine.TaskWithLayer) {
				var got []string
				for _, r := range result {
					got = append(got, r.Slug)
				}
				want := []string{"t1a", "t1b", "n2", "n1", "done1", "done-note"}
				if strings.Join(got, ",") != strings.Join(want, ",") {
					t.Errorf("RenderOrder() order = %v, want %v", got, want)
				}
			},
		},
		{
			name: "tasks within bucket sorted by ID",
			tasks: []boardengine.Task{
				{ID: 3, Slug: "c", Title: "Task C", DependsOn: []string{}},
				{ID: 1, Slug: "a", Title: "Task A", DependsOn: []string{}},
				{ID: 2, Slug: "b", Title: "Task B", DependsOn: []string{}},
			},
			check: func(t *testing.T, result []boardengine.TaskWithLayer) {
				if len(result) != 3 {
					t.Fatalf("RenderOrder() got %d tasks, want 3", len(result))
				}
				// All in layer A, should be sorted by ID
				for i, id := range []int{1, 2, 3} {
					if result[i].ID != id {
						t.Errorf("RenderOrder() position %d got ID %d, want %d", i, result[i].ID, id)
					}
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := boardengine.RenderOrder(tt.tasks)
			if err != nil {
				t.Fatalf("RenderOrder() error = %v", err)
			}
			tt.check(t, result)
		})
	}
}

func stringPtr(s string) *string {
	return &s
}
