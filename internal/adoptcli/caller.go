// caller.go is the `lym adopt` call itself.
// It resolves --here to the request it runs and sets the orch default.
// It runs the adopt in the foreground and prints its envelope.

package adoptcli

import (
	"io"
	"os"

	"github.com/Knatte18/loomyard/internal/agentname"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/fabriclayout"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/machinename"
	"github.com/Knatte18/loomyard/internal/output"
	"github.com/Knatte18/loomyard/internal/pauserecord"
)

// callerDeps holds the seams of a `lym adopt` call.
// ExemptPause lets every pause entry pass the process's network gate and returns the function that restores the gate.
// Strand is the caller's full agent name, empty for a plain shell.
// Worktree is the name of the worktree the call runs in.
// PullBolt brings Bolt up to date with origin;
// a pull that cannot, an unreachable origin among them, leaves the local Bolt and returns the skip's detail.
// Steps is what the adopt reads and writes.
// Stderr receives the line of each step that ends.
type callerDeps struct {
	ExemptPause func() (restore func())
	Strand      string
	Worktree    string
	PullBolt    func() (skipped string, err error)
	Steps       adoptDeps
	Stderr      io.Writer
}

// hubCallerDeps fills callerDeps over the hub the prime belongs to.
func hubCallerDeps(prime *lymcwd.Location) (callerDeps, error) {
	steps, err := hubDepsAs(prime, machinename.Name)
	if err != nil {
		return callerDeps{}, err
	}
	hubPath := prime.HubPath
	return callerDeps{
		ExemptPause: pauserecord.SetGateExemptAll,
		Strand:      os.Getenv(agentname.StrandNameEnv),
		Steps:       steps,
		Stderr:      os.Stderr,
		PullBolt: func() (string, error) {
			_, detail, err := fabricengine.NewBolt(fabriclayout.BoardDir(hubPath)).Pull(fabricengine.EnvSyncOptions(), fabricengine.NewMutations(hubPath))
			return detail, err
		},
	}, nil
}

// callFlags holds the flags that pick what a `lym adopt` call does, rather than what an adopt carries.
type callFlags struct {
	Here     bool
	Archives bool
}

// runsInOrch reports whether strand, a full agent name, is an orch session, a numbered orch included.
// An empty or unparsable name is not.
func runsInOrch(strand string) bool {
	name, err := agentname.Parse(strand)
	return err == nil && agentname.MatchesRole(name.Role, agentname.RoleOrch)
}

// runAdopt runs a `lym adopt` call.
// No pause holds its network calls, since an adopt is the operator's own takeover.
// With --here it first resolves the subject of the call's worktree, and may end without an adopt.
// It then runs the adopt in this process and prints the end envelope on out:
// ok for an adopt whose check went green, an error envelope with what the adopt did for any other end.
// It returns the exit code.
func runAdopt(deps callerDeps, req request, flags callFlags, out io.Writer) int {
	defer deps.ExemptPause()()

	if flags.Here {
		resolved, end, err := hereRequest(deps, req)
		if err != nil {
			return output.Err(out, err.Error())
		}
		if end != nil {
			return output.Ok(out, end)
		}
		req = resolved
	}

	req = normalizedRequest(req)
	req.NoOrch = req.NoOrch || runsInOrch(deps.Strand)
	result, err := runForeground(deps.Steps, req, deps.Stderr)
	fields := adoptFields(result)
	if err != nil {
		return output.ErrFields(out, err.Error(), fields)
	}
	return output.Ok(out, fields)
}
