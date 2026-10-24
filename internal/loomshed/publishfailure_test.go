// publishfailure_test.go covers the Plan-Review rubric's note of webster's run record, with no repository.

package loomshed

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/Knatte18/loomyard/internal/websterengine"
)

// testGateTiers is a tier-list lookup over fixed `landing` and `publish` lists, refusing any other gate.
func testGateTiers(gate string) ([]int, error) {
	switch gate {
	case "landing":
		return []int{0, 1}, nil
	case "publish":
		return []int{2}, nil
	}
	return nil, fmt.Errorf("gates holds no list for %q", gate)
}

func TestWebsterRecordNote(t *testing.T) {
	t.Parallel()

	followUpRule := "A done card is frozen: its work has landed, and the plan gate refuses an edit to it.\nA finding against a done card lands as a follow-up card after the last begun batch, with its Card Index line in `00-overview.md`."
	unreadable := "Webster's run record could not be read, so which cards are frozen is unknown; the plan gate reports the read error."
	tests := []struct {
		name  string
		write func(t *testing.T, anchorPath string)
		want  string
	}{
		{name: "no record renders none", want: "none"},
		{
			name: "begun batches, done cards and the follow-up rule",
			write: func(t *testing.T, anchorPath string) {
				st := &websterengine.State{Batches: map[int]*websterengine.BatchState{
					2: {Slug: "second", Cards: []string{"02-second", "03-third"}},
					1: {Slug: "first", Cards: []string{"01-first"}, Terminal: true, Status: websterengine.DigestStatusDone},
				}}
				if err := websterengine.SaveState(websterengine.Dir(anchorPath), websterengine.ScratchDir(anchorPath), st); err != nil {
					t.Fatal(err)
				}
			},
			want: "Webster has begun these batches:\n\n- batch 1 (done): 01-first\n- batch 2 (in flight): 02-second, 03-third\n\nDone cards: 01-first\n\n" + followUpRule,
		},
		{
			name: "unreadable record renders the fixed note",
			write: func(t *testing.T, anchorPath string) {
				dir := websterengine.Dir(anchorPath)
				if err := os.MkdirAll(dir, 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(dir, "state.json"), []byte("{not json"), 0o644); err != nil {
					t.Fatal(err)
				}
			},
			want: unreadable,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			anchorPath := t.TempDir()
			if tt.write != nil {
				tt.write(t, anchorPath)
			}
			if got := WebsterRecordNote(anchorPath); got != tt.want {
				t.Errorf("note = %q; want %q", got, tt.want)
			}
		})
	}
}
