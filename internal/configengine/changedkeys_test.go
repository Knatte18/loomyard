//go:build tier0

// changedkeys_test.go pins ChangedKeys, the key-by-key comparison of a config file at a task's fork point, its tip and its parent's tip.

package configengine_test

import (
	"testing"

	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/google/go-cmp/cmp"
)

// TestChangedKeys pins which keys the comparison reports: only a key whose tip value differs from the base and the parent, a comment-only edit and a value the parent already holds reporting none, and a document that does not parse an error.
func TestChangedKeys(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name              string
		base, tip, parent string
		want              []configengine.KeyChange
		wantErr           bool
	}{
		{
			name: "a comments-only change returns none",
			base: "a: 1\n",
			tip:  "# a note\na: 1\n",
			want: nil,
		},
		{
			name:   "a key the parent already holds returns none",
			base:   "a: 1\n",
			tip:    "a: 2\n",
			parent: "a: 2\n",
			want:   nil,
		},
		{
			name:   "a changed nested key returns its dotted path with three values",
			base:   "loom:\n  code_max_bounces: 2\n  other: x\n",
			tip:    "loom:\n  code_max_bounces: 1\n  other: x\n",
			parent: "loom:\n  code_max_bounces: 2\n  other: x\n",
			want:   []configengine.KeyChange{{Key: "loom.code_max_bounces", Base: "2", Tip: "1", Parent: "2"}},
		},
		{
			name:   "a key absent at the base reads absent",
			base:   "a: 1\n",
			tip:    "a: 1\nb: 5\n",
			parent: "a: 1\n",
			want:   []configengine.KeyChange{{Key: "b", Base: configengine.AbsentValue, Tip: "5", Parent: configengine.AbsentValue}},
		},
		{
			name: "a file the task added reads every key absent at the base and the parent",
			tip:  "b: 5\na: 1\n",
			want: []configengine.KeyChange{
				{Key: "a", Base: configengine.AbsentValue, Tip: "1", Parent: configengine.AbsentValue},
				{Key: "b", Base: configengine.AbsentValue, Tip: "5", Parent: configengine.AbsentValue},
			},
		},
		{
			name:   "a key the tip drops reads absent at the tip",
			base:   "a: 1\n",
			tip:    "",
			parent: "a: 1\n",
			want:   []configengine.KeyChange{{Key: "a", Base: "1", Tip: configengine.AbsentValue, Parent: "1"}},
		},
		{
			name:    "an unparseable tip is an error",
			base:    "a: 1\n",
			tip:     "a: [1\n",
			parent:  "a: 1\n",
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := configengine.ChangedKeys([]byte(tt.base), []byte(tt.tip), []byte(tt.parent))
			if (err != nil) != tt.wantErr {
				t.Fatalf("ChangedKeys() error = %v, wantErr %t", err, tt.wantErr)
			}
			if diff := cmp.Diff(tt.want, got); diff != "" {
				t.Errorf("ChangedKeys() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}
