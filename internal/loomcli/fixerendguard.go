// fixerendguard.go wraps the goto guard so a move onto a row ends the pair's gate fixers before the move is written.

package loomcli

import (
	"fmt"
	"os"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gatefixer"
	"github.com/Knatte18/loomyard/internal/runproc"
	"github.com/Knatte18/loomyard/internal/shedengine"
)

// endGateFixers ends every process of each gatefixer role that the pair named by slug has registered under runProcDir.
// It calls end for each role and spares the calling process.
// An empty slug, the prime's or one outside a hub, ends nothing.
// A slug read error and a failed end are returned wrapping their cause.
func endGateFixers(runProcDir string, slug func() (string, error), end func(dir, owner, role string, callerPID int) ([]runproc.Ended, error)) error {
	owner, err := slug()
	if err != nil {
		return fmt.Errorf("loomcli: read the pair slug to end its gate fixers: %w", err)
	}
	if owner == "" {
		return nil
	}
	for _, role := range gatefixer.Roles() {
		if _, err := end(runProcDir, owner, role, os.Getpid()); err != nil {
			return fmt.Errorf("loomcli: end the %s processes of pair %s: %w", role, owner, err)
		}
	}
	return nil
}

// fixerEndingGuard returns a goto guard that runs inner first.
// Only when inner admits the target does it call endFixers, before the move is written.
// A refusal or error from inner is returned unchanged and ends nothing;
// an endFixers error is returned as the guard's error, so the move is refused and the run stays where it is.
func fixerEndingGuard(inner shedengine.GotoGuard, endFixers func() error) shedengine.GotoGuard {
	return func(target string) (string, error) {
		refusal, err := inner(target)
		if err != nil || refusal != "" {
			return refusal, err
		}
		if err := endFixers(); err != nil {
			return "", err
		}
		return "", nil
	}
}

// gotoGuardEndingFixers wraps inner so a goto it admits first ends this worktree's gate fixers.
// Every site that moves the run onto a row builds its guard through it.
func (c *loomCLI) gotoGuardEndingFixers(inner shedengine.GotoGuard) shedengine.GotoGuard {
	end := c.endRunFixers
	if c.endFixers != nil {
		end = c.endFixers
	}
	return fixerEndingGuard(inner, end)
}

// endRunFixers ends every gate fixer registered under this worktree's pair.
// A receiver with no location ends nothing.
func (c *loomCLI) endRunFixers() error {
	if c.location == nil {
		return nil
	}
	return endGateFixers(fabricengine.RunProcDir(c.location.HubPath), func() (string, error) { return pairSlugOf(c.location) }, runproc.EndRole)
}
