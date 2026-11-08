// livestate_states_selftest_test.go proves the one per-state claim neither the cross product nor States checks.
// The claim is that the baseline hub the clean state leaves untouched carries no operator content.
// Every other state asserts its own establishment inside its Apply.
// States also refuses a structural state that changes nothing.

package fabricengine_test

import (
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/lymdirs"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// assertCleanHubEstablished fails t unless every checkout in h is clean of operator content.
//
// The warp checkout and the board checkout must carry no git status --porcelain output.
// The prime weft sibling may carry exactly one line, for the still-empty wiring directory WireJunctions creates.
// Git never tracks an empty directory, so the line shows as untracked:
// "?? _lym/" at anchor "." or "?? backend/" at anchor "backend".
// This is fabric's own machine-local wiring scratch, not operator dirt.
// Any other line is a genuine finding, not baseline noise.
func assertCleanHubEstablished(t *testing.T, h *hubforge.Hub) {
	t.Helper()

	for _, checkout := range []string{h.PrimeWorktree(), h.BoardDir()} {
		if status := gitkit.GitStatusPorcelain(t, checkout); status != "" {
			t.Errorf("clean state failed to establish: %s is not clean: %s", checkout, status)
		}
	}

	weftStatus := gitkit.GitStatusPorcelain(t, h.PrimeRecords())
	if weftStatus == "" {
		return
	}

	allowed := "?? " + h.Anchor + "/"
	if h.Anchor == "." {
		allowed = "?? " + lymdirs.LymDirName + "/"
	}
	for _, line := range strings.Split(weftStatus, "\n") {
		if line == "" || line == allowed {
			continue
		}
		t.Errorf("clean state failed to establish: %s carries unexpected dirt: %q (full status: %s)", h.PrimeRecords(), line, weftStatus)
	}
}

// TestCleanHubIsClean proves a freshly built nested-anchor hub carries no operator dirt,
// so the clean control state of the cross product starts from a genuinely clean hub.
func TestCleanHubIsClean(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	assertCleanHubEstablished(t, hubforge.CopyHub(t, hubforge.Shape{Anchor: "backend"}))
}
