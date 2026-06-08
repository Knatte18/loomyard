// layer_test.go — unit tests for derived fields (layer.go).
//
// ComputeLayers depth assignment, RenderOrder, and ExtendedTitle.

package board_test

import (
	"testing"

	"github.com/Knatte18/mhgo/internal/board"
)

func TestComputeLayers(t *testing.T) {
	tests := []struct {
		name      string
		tasks     []board.Task
		want      map[string]string
		wantError bool
	}{
		{
			name: "single task no deps",
			tasks: []board.Task{
				{ID: 1, Slug: "a", Title: "Task A", DependsOn: []string{}},
			},
			want: map[string]string{"a": "A"},
		},
		{
			name: "A depends on B",
			tasks: []board.Task{
				{ID: 1, Slug: "a", Title: "Task A", DependsOn: []string{"b"}},
				{ID: 2, Slug: "b", Title: "Task B", DependsOn: []string{}},
			},
			want: map[string]string{"a": "B", "b": "A"},
		},
		{
			name: "done task excluded from depth",
			tasks: []board.Task{
				{ID: 1, Slug: "a", Title: "Task A", DependsOn: []string{"b"}},
				{ID: 2, Slug: "b", Title: "Task B", DependsOn: []string{}, Status: stringPtr("done")},
			},
			want: map[string]string{"a": "A", "b": "__done__"},
		},
		{
			name: "deferred task",
			tasks: []board.Task{
				{ID: 1, Slug: "a", Title: "Task A", Deferred: true},
			},
			want: map[string]string{"a": "__deferred__"},
		},
		{
			name: "isolated task",
			tasks: []board.Task{
				{ID: 1, Slug: "a", Title: "Task A", Isolated: true},
			},
			want: map[string]string{"a": "Z"},
		},
		{
			name: "chain of 3",
			tasks: []board.Task{
				{ID: 1, Slug: "a", Title: "Task A", DependsOn: []string{"b"}},
				{ID: 2, Slug: "b", Title: "Task B", DependsOn: []string{"c"}},
				{ID: 3, Slug: "c", Title: "Task C", DependsOn: []string{}},
			},
			want: map[string]string{"a": "C", "b": "B", "c": "A"},
		},
		{
			name: "depth exceeds A..Y cap",
			tasks: func() []board.Task {
				var tasks []board.Task
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
					tasks = append(tasks, board.Task{
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
			got, err := board.ComputeLayers(tt.tasks)
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

func TestRenderOrder(t *testing.T) {
	tests := []struct {
		name  string
		tasks []board.Task
		check func(t *testing.T, result []board.TaskWithLayer)
	}{
		{
			name: "buckets in correct order",
			tasks: []board.Task{
				{ID: 1, Slug: "done1", Title: "Done Task", Status: stringPtr("done")},
				{ID: 2, Slug: "deferred1", Title: "Deferred Task", Deferred: true},
				{ID: 3, Slug: "z1", Title: "Isolated Task", Isolated: true},
				{ID: 4, Slug: "a1", Title: "Layer A Task", DependsOn: []string{}},
				{ID: 5, Slug: "b1", Title: "Layer B Task", DependsOn: []string{"a1"}},
			},
			check: func(t *testing.T, result []board.TaskWithLayer) {
				if len(result) != 5 {
					t.Fatalf("RenderOrder() got %d tasks, want 5", len(result))
				}
				// Expected order: a1(A), b1(B), z1(Z), deferred1(__deferred__), done1(__done__)
				wantOrder := []string{"a1", "b1", "z1", "deferred1", "done1"}
				wantLayers := []string{"A", "B", "Z", "__deferred__", "__done__"}
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
			name: "tasks within bucket sorted by ID",
			tasks: []board.Task{
				{ID: 3, Slug: "c", Title: "Task C", DependsOn: []string{}},
				{ID: 1, Slug: "a", Title: "Task A", DependsOn: []string{}},
				{ID: 2, Slug: "b", Title: "Task B", DependsOn: []string{}},
			},
			check: func(t *testing.T, result []board.TaskWithLayer) {
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
			result, err := board.RenderOrder(tt.tasks)
			if err != nil {
				t.Fatalf("RenderOrder() error = %v", err)
			}
			tt.check(t, result)
		})
	}
}

func TestExtendedTitle(t *testing.T) {
	tests := []struct {
		name  string
		task  board.Task
		layer string
		want  string
	}{
		{
			name:  "letter bucket",
			task:  board.Task{Title: "My Task"},
			layer: "A",
			want:  "My Task [A]",
		},
		{
			name:  "Z bucket",
			task:  board.Task{Title: "Isolated"},
			layer: "Z",
			want:  "Isolated [Z]",
		},
		{
			name:  "done bucket",
			task:  board.Task{Title: "Completed"},
			layer: "__done__",
			want:  "Completed",
		},
		{
			name:  "deferred bucket",
			task:  board.Task{Title: "Postponed"},
			layer: "__deferred__",
			want:  "Postponed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := board.ExtendedTitle(tt.task, tt.layer)
			if got != tt.want {
				t.Errorf("ExtendedTitle() got %q, want %q", got, tt.want)
			}
		})
	}
}

func stringPtr(s string) *string {
	return &s
}
