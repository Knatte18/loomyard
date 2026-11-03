// runstitle_integration_test.go proves the pre-run wiring names the runs strand's window after the worktree the verb runs in: the prime reads `beam: prime`, and a beam reads `beam: <slug>`, opened with its board handle once the board names one.

package orchcli

import (
	"bytes"
	"fmt"
	"testing"

	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/testkit/boardkit"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

func TestRunsWindowTitle_WiredFromTheWorktreeTheVerbRunsIn(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	const slug = "b"
	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	hubforge.AddPairWith(t, h, slug, fabricengine.AddOptions{SkipPush: true})
	if marked, err := fabricengine.MarkBeam(fabricengine.NewMutations(h.Location.HubPath), h.Location, slug); err != nil || !marked {
		t.Fatalf("MarkBeam(%q) = (%v, %v); want (true, nil)", slug, marked, err)
	}
	beam, err := lymcwd.ResolveWorktree(h.PairCodeWorktree(slug))
	if err != nil {
		t.Fatalf("ResolveWorktree(%q): %v", slug, err)
	}

	// wired runs status, which runs the pre-run in full and only then talks to reed, whose answer this test ignores.
	wired := func(cwd string) *orchCLI {
		c := &orchCLI{}
		var out bytes.Buffer
		clihelp.ExecuteIn(commandOf(c), cwd, &out, []string{"status"})
		if c.runsWindowTitle == nil {
			t.Fatalf("status in %s wired no runs window title; output: %s", cwd, out.String())
		}
		return c
	}
	prime, beamCLI := wired(h.Location.AnchorPath()), wired(beam.AnchorPath())

	// The steps run in order, not in parallel: the title is read from the board at each call, and the second step writes the board.
	steps := []struct {
		name       string
		writeBoard bool
		wantPrime  string
		wantBeam   string
	}{
		{"BeforeTheBoardNamesTheBeamItReadsItsSlug", false, "beam: prime", "beam: " + slug},
		{"OnceTheBoardNamesTheBeamItOpensWithItsHandle", true, "beam: prime", "P1: beam: " + slug},
	}
	for _, step := range steps {
		if step.writeBoard {
			boardkit.WriteBoard(t, h.BoardDir(), fmt.Sprintf(`{"id":1,"slug":%q,"kind":"project","labels":["bug"],"beam":true,"handle":"P1"}`, slug))
		}
		if got := prime.runsWindowTitle(); got != step.wantPrime {
			t.Errorf("%s: prime runsWindowTitle() = %q; want %q", step.name, got, step.wantPrime)
		}
		if got := beamCLI.runsWindowTitle(); got != step.wantBeam {
			t.Errorf("%s: beam runsWindowTitle() = %q; want %q", step.name, got, step.wantBeam)
		}
	}
}
