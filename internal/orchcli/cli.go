// cli.go builds the cobra command tree for the orch module and the RunCLI seam that wires it into the standard io.Writer-based call contract.
// The parent "orch" command's PersistentPreRunE resolves cwd -> prime check -> orch, shuttle and reed config -> reed engine -> claude engine -> shuttleengine.Runner -> paths exactly once per invocation, into a receiver every verb closes over, so no verb re-resolves geometry itself.

package orchcli

import (
	"fmt"
	"io"
	"time"

	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/hubgeom"
	"github.com/Knatte18/loomyard/internal/hubreconcile"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/orchengine"
	"github.com/Knatte18/loomyard/internal/output"
	"github.com/Knatte18/loomyard/internal/reedengine"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/shuttleengine/claudeengine"
	"github.com/spf13/cobra"
)

// strandOps is the slice of reed the verbs read, so untagged tests substitute a fake.
type strandOps interface {
	Strands() ([]reedengine.StrandStatus, error)
	RemoveStrand(guid string) error
}

// reedStrandOps adapts *reedengine.Engine to strandOps.
type reedStrandOps struct {
	status func() (reedengine.StatusResult, error)
	remove func(guid string, recursive bool) (reedengine.Removed, error)
}

// newReedStrandOps wraps reed's own Status and RemoveStrand.
func newReedStrandOps(reed *reedengine.Engine) reedStrandOps {
	return reedStrandOps{status: reed.Status, remove: reed.RemoveStrand}
}

// Strands returns the strands reed tracks in this session.
func (o reedStrandOps) Strands() ([]reedengine.StrandStatus, error) {
	result, err := o.status()
	if err != nil {
		return nil, err
	}
	return result.Strands, nil
}

// RemoveStrand removes guid without cascade; the orch strand is parentless and childless.
func (o reedStrandOps) RemoveStrand(guid string) error {
	_, err := o.remove(guid, false)
	return err
}

// orchCLI is the receiver every orch verb hangs off of, populated by PersistentPreRunE.
type orchCLI struct {
	location    *lyxcwd.Location
	cfg         orchengine.Config
	shuttleCfg  shuttleengine.Config
	runner      *shuttleengine.Runner
	strands     strandOps
	paths       orchengine.Paths
	stencilsDir string

	reedUp       func() error
	starter      sessionStarter
	spawnWatcher func() error
	sleep        func(time.Duration)
}

// Command returns the cobra command tree for the orch module.
func Command() *cobra.Command {
	c := &orchCLI{}

	parent := &cobra.Command{
		Use:   "orch",
		Short: "run the hub orchestrator as a reed strand with automatic handoff",
		Long: `orch hosts the hub orchestrator as an interactive Claude session in a reed
strand of the hub's prime worktree. A detached watcher reads each turn's
context usage and, past a threshold and while the session is idle, has the
session write a handoff, clears it and resumes it from that handoff.
Every verb runs from the hub's prime worktree only.`,
		RunE: clihelp.GroupRunE,
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			// The bare group and its unknown-subcommand path need no repository.
			if cmd.Name() == "orch" {
				return nil
			}
			ctx := cmd.Context()
			out := cmd.OutOrStdout()
			fail := func(err error) error {
				output.Err(out, err.Error())
				clihelp.Abort(ctx, 1)
				return nil
			}

			cwd, err := lyxcwd.CwdFrom(ctx)
			if err != nil {
				return fail(err)
			}
			location, err := lyxcwd.Resolve(cwd)
			if err != nil {
				return fail(err)
			}

			if err := fabricengine.RequireDrivableWorktree(location); err != nil {
				return fail(fmt.Errorf("orch: lyx orch runs from the hub's prime worktree only: %w", err))
			}
			primeName, primeNameErr := fabricengine.PrimeName(location)
			if err := refuseNonPrime(location.WorktreeName, primeName, primeNameErr); err != nil {
				return fail(err)
			}

			// Only start reconciles, and before any module config loads, so a retired key cannot refuse the verb ahead of the reconcile that removes it.
			if cmd.Name() == "start" {
				if geometry, inHub := hubgeom.ReconcileGeometry(location); inHub {
					if err := hubreconcile.Ensure(geometry, hubreconcile.Options{}); err != nil {
						return fail(err)
					}
				}
			}

			orchCfg, err := orchengine.LoadConfig(location.AnchorPath(), "orch")
			if err != nil {
				return fail(err)
			}
			shuttleCfg, err := shuttleengine.LoadConfig(location.AnchorPath(), "shuttle")
			if err != nil {
				return fail(err)
			}
			reedCfg, err := reedengine.LoadConfig(location.AnchorPath(), "reed")
			if err != nil {
				return fail(err)
			}

			reedGeom, err := hubgeom.ReedGeometry(location)
			if err != nil {
				return fail(err)
			}
			reed := reedengine.New(reedCfg, reedGeom)

			c.location = location
			c.cfg = orchCfg
			c.shuttleCfg = shuttleCfg
			c.runner = shuttleengine.NewRunner(reed, claudeengine.NewFromConfig(shuttleCfg), reedGeom.AnchorPath, reedGeom.WorktreeRoot, shuttleCfg)
			c.strands = newReedStrandOps(reed)
			c.paths = orchPaths(location)
			c.stencilsDir = fabricengine.StencilsDir(location.HubPath)
			c.reedUp = func() error { _, err := reed.Up(); return err }
			c.starter = runnerSessionStarter{runner: c.runner}
			c.spawnWatcher = c.spawnWatcherProcess
			c.sleep = time.Sleep
			return nil
		},
	}

	parent.AddCommand(c.startCmd(), c.statusCmd(), c.refreshCmd(), c.distillCmd(), c.stopCmd(), c.watchCmd())
	return parent
}

// RunCLI is the public seam for the orch module CLI, delegating to clihelp.Execute for output capture.
func RunCLI(out io.Writer, args []string) int {
	return RunCLIIn("", out, args)
}

// RunCLIIn is RunCLI's cwd-carrying sibling: an empty cwd reads the process cwd, any other value seeds cwd into the execution context, because lyxcwd.WithCwd panics on an empty directory.
func RunCLIIn(cwd string, out io.Writer, args []string) int {
	if cwd == "" {
		return clihelp.Execute(Command(), out, args)
	}
	return clihelp.ExecuteIn(Command(), cwd, out, args)
}
