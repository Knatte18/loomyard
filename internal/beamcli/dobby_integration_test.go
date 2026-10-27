//go:build tier1

// dobby_integration_test.go runs "lym beam dobby" through RunCLIIn over a hubforge hub, on paths dobby cannot place.

package beamcli_test

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/Knatte18/loomyard/internal/beamcli"
	"github.com/Knatte18/loomyard/internal/dobbyengine"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/runproc"
	"github.com/Knatte18/loomyard/internal/testkit/envelope"
)

func TestDobbyCmd_RefusesAPathItCannotPlace(t *testing.T) {
	t.Parallel()

	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	hubforge.AddPair(t, h, "task1")
	loose := filepath.Join(h.Path, "loose")
	if err := os.MkdirAll(loose, 0o755); err != nil {
		t.Fatalf("MkdirAll(%s) = %v; want nil", loose, err)
	}

	tests := []struct {
		name string
		root string
	}{
		{"DirectoryOfTheHubThatIsNoWorktree", loose},
		{"TaskPairWhoseOriginRecordCarriesNoBeamMark", h.PairCodeWorktree("task1")},
	}
	for _, tt := range tests {
		// The rows share one hub and read its run-process directory, so they run in sequence.
		t.Run(tt.name, func(t *testing.T) {
			var out bytes.Buffer
			beamcli.RunCLIIn(h.Path, &out, []string{"dobby", tt.root})

			envelope.RequireErr(t, out.String(), "lym beam start")
			owners, err := runproc.Owners(fabricengine.RunProcDir(h.Path))
			if err != nil || len(owners) != 0 {
				t.Errorf("runproc.Owners() = %v, %v; want no record written", owners, err)
			}
			if _, err := os.Stat(filepath.Join(tt.root, ".lym", "beam", dobbyengine.LockFileName)); !os.IsNotExist(err) {
				t.Errorf("Stat(dobby lock) = %v; want it absent", err)
			}
			// A refusal appends to the log only where a beam directory exists, and creates neither.
			if _, err := os.Stat(filepath.Join(tt.root, ".lym", "beam")); !os.IsNotExist(err) {
				t.Errorf("Stat(.lym/beam) = %v; want it absent", err)
			}
			if _, err := os.Stat(filepath.Join(tt.root, ".lym", "beam", dobbyengine.LogFileName)); !os.IsNotExist(err) {
				t.Errorf("Stat(dobby.log) = %v; want it absent", err)
			}
		})
	}
}
