// move_test.go table-drives MoveKeys: where an old key's value and comments land, and which old keys are dropped.

package yamlengine_test

import (
	"slices"
	"testing"

	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
	"github.com/Knatte18/loomyard/internal/yamlengine"
)

func TestMoveKeys(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	discussion := map[string]string{"discussion": "discussion.model"}
	flat := map[string]string{"old": "area.new"}
	tests := []struct {
		name     string
		existing string
		prefix   []string
		moves    map[string]string
		want     string
		changes  []yamlengine.Move
		wantErr  bool
	}{
		{
			name:     "an alias moved ahead of its anchor holds a copy of the value",
			existing: "code_max_bounces: &n 2\ncode_hard_max_bounces: *n\n",
			moves:    map[string]string{"code_max_bounces": "code.max_bounces", "code_hard_max_bounces": "code.hard_max_bounces"},
			want:     "code:\n    hard_max_bounces: 2\n    max_bounces: &n 2\n",
			changes: []yamlengine.Move{
				{Old: "code_hard_max_bounces", New: "code.hard_max_bounces", Outcome: yamlengine.MoveDone},
				{Old: "code_max_bounces", New: "code.max_bounces", Outcome: yamlengine.MoveDone},
			},
		},
		{
			name:     "a flat key moves under a new area beside a key the area holds",
			existing: "code_max_bounces: 2\ncode:\n    auto_accept: true\n",
			moves:    map[string]string{"code_max_bounces": "code.max_bounces"},
			want:     "code:\n    auto_accept: true\n    max_bounces: 2\n",
			changes:  []yamlengine.Move{{Old: "code_max_bounces", New: "code.max_bounces", Outcome: yamlengine.MoveDone}},
		},
		{
			name:     "a scalar under the area's own name moves into the area",
			existing: "discussion: sonnet[medium]\n",
			moves:    discussion,
			want:     "discussion:\n    model: sonnet[medium]\n",
			changes:  []yamlengine.Move{{Old: "discussion", New: "discussion.model", Outcome: yamlengine.MoveDone}},
		},
		{
			name:     "a mapping under the area's own name is the new area and stays",
			existing: "discussion:\n  model: x\n",
			moves:    discussion,
			want:     "discussion:\n  model: x\n",
		},
		{
			name:     "the area's own key moves before a key sorting ahead of it moves into the area",
			existing: "part_writer: w\nparts: m\n",
			moves:    map[string]string{"part_writer": "parts.writer", "parts": "parts.model"},
			want:     "parts:\n    model: m\n    writer: w\n",
			changes: []yamlengine.Move{
				{Old: "part_writer", New: "parts.writer", Outcome: yamlengine.MoveDone},
				{Old: "parts", New: "parts.model", Outcome: yamlengine.MoveDone},
			},
		},
		{
			name:     "a null old key is dropped",
			existing: "old:\nkeep: 1\n",
			moves:    flat,
			want:     "keep: 1\n",
			changes:  []yamlengine.Move{{Old: "old", New: "area.new", Outcome: yamlengine.MoveDroppedNull}},
		},
		{
			name:     "the new path's value wins over the old key's",
			existing: "old: 1\narea:\n    new: 2\n",
			moves:    flat,
			want:     "area:\n    new: 2\n",
			changes:  []yamlengine.Move{{Old: "old", New: "area.new", Outcome: yamlengine.MoveDroppedNewSet}},
		},
		{
			name:     "a moved key keeps its comments and its neighbours keep theirs",
			existing: "first: 1 # one\n# about old\nold: 2 # two\n# about last\nlast: 3\n",
			moves:    flat,
			want:     "first: 1 # one\n# about last\nlast: 3\narea:\n    # about old\n    new: 2 # two\n",
			changes:  []yamlengine.Move{{Old: "old", New: "area.new", Outcome: yamlengine.MoveDone}},
		},
		{
			name:     "a prefix moves only the keys below it",
			existing: "old: 1\nrecipes:\n    loom:\n        old: 2\n",
			prefix:   []string{"recipes", "loom"},
			moves:    flat,
			want:     "old: 1\nrecipes:\n    loom:\n        area:\n            new: 2\n",
			changes:  []yamlengine.Move{{Old: "old", New: "area.new", Outcome: yamlengine.MoveDone}},
		},
		{
			name:     "an absent prefix changes nothing",
			existing: "old: 1\n",
			prefix:   []string{"recipes", "loom"},
			moves:    flat,
			want:     "old: 1\n",
		},
		{name: "an unparseable document is refused", existing: "old: [unclosed\n", moves: flat, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, changes, err := yamlengine.MoveKeys([]byte(tt.existing), tt.prefix, tt.moves)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("MoveKeys(%q) = %q; want an error", tt.existing, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("MoveKeys(%q) = %v; want nil", tt.existing, err)
			}
			if string(got) != tt.want {
				t.Errorf("MoveKeys(%q) = %q; want %q", tt.existing, got, tt.want)
			}
			if !slices.Equal(changes, tt.changes) {
				t.Errorf("MoveKeys(%q) changes = %v; want %v", tt.existing, changes, tt.changes)
			}

			again, changes, err := yamlengine.MoveKeys(got, tt.prefix, tt.moves)
			if err != nil || string(again) != string(got) || len(changes) != 0 {
				t.Errorf("second MoveKeys = %q, %v, %v; want %q unchanged", again, changes, err, got)
			}
		})
	}
}
