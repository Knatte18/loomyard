// raddlehandoff_test.go covers the hand-off line Beam-Up mails a beam's orch.

package loomcli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/boardengine"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// TestRaddleHandOff asserts that a beam gets the hand-off line only when a stored proposal filed its project.
// The line names the Sketch, the record read and the proposal read.
func TestRaddleHandOff(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	const project = "the-beam"
	tests := []struct {
		name string
		// store is the store.json record of the Sketch "the-sketch", nil for an empty store.
		store     *boardengine.RaddleStoreEntry
		wantFound bool
	}{
		{
			name:      "StoredProposalThatFiledTheProjectNamesTheThreeReads",
			store:     &boardengine.RaddleStoreEntry{Hash: "h", Filed: []string{project, "a-task"}, Project: project},
			wantFound: true,
		},
		{
			name:  "StoredProposalThatFiledOnlyTasksHandsOffNothing",
			store: &boardengine.RaddleStoreEntry{Hash: "h", Filed: []string{"a-task", project}},
		},
		{name: "EmptyStoreHandsOffNothing"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			hub := t.TempDir()
			if tt.store != nil {
				directory := boardengine.RaddleStoreDir(fabricengine.BoardDir(hub), "the-sketch")
				if err := os.MkdirAll(directory, 0o755); err != nil {
					t.Fatal(err)
				}
				data, err := json.Marshal(tt.store)
				if err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(directory, "store.json"), data, 0o644); err != nil {
					t.Fatal(err)
				}
			}

			line, found, err := raddleHandOff(&lymcwd.Location{HubPath: hub}, project)

			if err != nil || found != tt.wantFound {
				t.Fatalf("raddleHandOff() = (%q, %v, %v); want found %v and no error", line, found, err, tt.wantFound)
			}
			if !tt.wantFound {
				if line != "" {
					t.Errorf("raddleHandOff() line = %q; want none", line)
				}
				return
			}
			for _, want := range []string{"the-sketch", "lym board get the-beam --record", "lym board raddle the-beam"} {
				if !strings.Contains(line, want) {
					t.Errorf("raddleHandOff() line = %q; want it to carry %q", line, want)
				}
			}
		})
	}
}
