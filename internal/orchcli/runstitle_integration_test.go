//go:build tier1

// runstitle_integration_test.go proves the pre-run wiring names the runs strand's window after the worktree the verb runs in: the prime reads `beam: prime` and a beam reads `beam: <slug>`.

package orchcli

import (
	"bytes"
	"testing"

	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
)

func TestRunsWindowTitle_WiredFromTheWorktreeTheVerbRunsIn(t *testing.T) {
	t.Parallel()

	const slug = "b"
	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	hubforge.AddPairWith(t, h, slug, fabricengine.AddOptions{SkipPush: true})
	if marked, err := fabricengine.MarkBeam(fabricengine.NewMutations(h.Location.HubPath), h.Location, slug); err != nil || !marked {
		t.Fatalf("MarkBeam(%q) = (%v, %v); want (true, nil)", slug, marked, err)
	}
	beam, err := lyxcwd.ResolveWorktree(h.PairCodeWorktree(slug))
	if err != nil {
		t.Fatalf("ResolveWorktree(%q): %v", slug, err)
	}

	tests := []struct {
		name string
		cwd  string
		want string
	}{
		{"PrimeReadsPrime", h.Location.AnchorPath(), "beam: prime"},
		{"BeamReadsItsSlug", beam.AnchorPath(), "beam: " + slug},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// status runs the pre-run in full and only then talks to reed, whose answer this test ignores.
			c := &orchCLI{}
			var out bytes.Buffer
			clihelp.ExecuteIn(commandOf(c), tt.cwd, &out, []string{"status"})

			if c.runsWindowTitle != tt.want {
				t.Errorf("runsWindowTitle after status in %s = %q; want %q; output: %s", tt.cwd, c.runsWindowTitle, tt.want, out.String())
			}
		})
	}
}
