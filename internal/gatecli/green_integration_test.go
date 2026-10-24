//go:build tier1

// green_integration_test.go covers the one green-record rule that needs a real git repository: a pass is recorded only while the tree it ran on is still HEAD's.

package gatecli

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/gateslot"
	"github.com/Knatte18/loomyard/internal/gitkit"
)

func TestGreenRunRecordPass(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		// moveHead commits a change after the call started, so HEAD's tree is no longer the one the pass ran on.
		moveHead bool
		// noCarry declares the tier with carry: false.
		noCarry     bool
		wantEntries int
	}{
		{name: "a pass on the tree the call started on is recorded", wantEntries: 1},
		{name: "a carry: false tier's pass is not recorded", noCarry: true, wantEntries: 0},
		{name: "a tree that changed during the run records nothing", moveHead: true, wantEntries: 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			root := t.TempDir()
			gitkit.Git(t, root, "init", "-q")
			gitkit.CommitFile(t, root, "a.txt", "one\n", "first")
			tree, commit, err := headIdentity(root)
			if err != nil {
				t.Fatalf("headIdentity() = %v; want nil", err)
			}
			green := &greenRun{path: filepath.Join(t.TempDir(), greenFileName), root: root, tree: tree, commit: commit}

			if tt.moveHead {
				gitkit.CommitFile(t, root, "b.txt", "two\n", "second")
			}
			tier := gateslot.Tier{Command: "go test {args}", Dir: ".", Counter: "go-json", TimeoutMin: 1, Parallel: 1, Slots: 1, NoCarry: tt.noCarry}
			run := tierRun{number: 1, tier: tier}
			green.recordPass(context.Background(), run, []invocation{{dir: "."}}, greenPlan{}, nil, time.Now())

			if got := len(readGreen(green.path).Entries); got != tt.wantEntries {
				t.Errorf("green record entries after recordPass = %d; want %d", got, tt.wantEntries)
			}
		})
	}
}
