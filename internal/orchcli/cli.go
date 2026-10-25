// cli.go builds the cobra command tree for the orch module and the RunCLI seam that wires it into the standard io.Writer-based call contract.
// The parent "orch" command's PersistentPreRunE resolves cwd -> prime-or-beam check -> orch, shuttle and reed config -> reed engine -> claude engine -> shuttleengine.Runner -> paths exactly once per invocation, into a receiver every verb closes over, so no verb re-resolves geometry itself.

package orchcli

import (
	"fmt"
	"io"
	"os"
	"time"

	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/creelcli"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/hubgeom"
	"github.com/Knatte18/loomyard/internal/hubreconcile"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/orchengine"
	"github.com/Knatte18/loomyard/internal/output"
	"github.com/Knatte18/loomyard/internal/reedengine"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/shuttleengines"
	"github.com/Knatte18/loomyard/internal/statuspane"
	"github.com/spf13/cobra"
)

// strandOps is the slice of reed the verbs read, so untagged tests substitute a fake.
type strandOps interface {
	Strands() ([]reedengine.StrandStatus, error)
	RemoveStrand(guid string) error
	AddStrand(spec reedengine.AddSpec) (reedengine.Strand, error)
	ReplaceStrand(guid string, spec reedengine.AddSpec) (reedengine.Strand, error)
}

// reedStrandOps adapts *reedengine.Engine to strandOps.
type reedStrandOps struct {
	status  func() (reedengine.StatusResult, error)
	remove  func(guid string, recursive bool) (reedengine.Removed, error)
	add     func(spec reedengine.AddSpec) (reedengine.Strand, error)
	replace func(guid string, spec reedengine.AddSpec) (reedengine.Strand, error)
}

// newReedStrandOps wraps reed's own Status, RemoveStrand, AddStrand and ReplaceStrand.
func newReedStrandOps(reed *reedengine.Engine) reedStrandOps {
	return reedStrandOps{status: reed.Status, remove: reed.RemoveStrand, add: reed.AddStrand, replace: reed.ReplaceStrand}
}

// AddStrand adds a strand to reed's session.
func (o reedStrandOps) AddStrand(spec reedengine.AddSpec) (reedengine.Strand, error) {
	return o.add(spec)
}

// ReplaceStrand replaces the strand guid with one built from spec, keeping its slot.
func (o reedStrandOps) ReplaceStrand(guid string, spec reedengine.AddSpec) (reedengine.Strand, error) {
	return o.replace(guid, spec)
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
	engines     shuttleengine.EngineSet
	runner      *shuttleengine.Runner
	strands     strandOps
	paths       orchengine.Paths
	stencilsDir string
	// pauseRecordPath is the hub's pause record, which `status`, `refresh` and `distill` read to tell a request held by a hub pause.
	pauseRecordPath string

	reedUp  func() error
	starter sessionStarter
	// newCreel builds the creel seam the watcher and `start` call creel through, signing as the ORCHWATCH program.
	newCreel     func() (orchengine.Creel, error)
	spawnWatcher func() error
	// startLockWait is how long `stop` waits for start.lock before it refuses.
	startLockWait time.Duration
	sleep         func(time.Duration)
	// indexRunner returns the output of `<bin> help index`.
	indexRunner func(bin string) (string, error)
	// executable returns the path of the running lyx binary, which the runs strand's command names.
	executable func() (string, error)
	// currentBuild identifies the running lyx build, which tells a runs strand of this build from an older one.
	currentBuild func() (statuspane.BuildIdentity, error)
	// runsWindowTitle names the runs strand's window: `beam: prime` in the prime, `beam: <slug>` in a beam.
	runsWindowTitle string
	// reconcile judges and moves the hub's branches against origin, which `start` runs before it decides anything about the strand.
	reconcile func() (fabricengine.OriginReport, error)
	// beamStart refuses a start in a beam that another machine owns or that its landing holds, which `start` calls before it creates anything; nil in the prime, and in a hand-built harness, skips it.
	beamStart func() error
	// claimPrime records this machine as the hub's prime owner when the board records none, which `start` calls first after its reconcile.
	// claimed names this machine after a claim, lostTo the other machine when its claim won; nil skips the claim, as in a beam.
	claimPrime func() (claimed, lostTo string, err error)
	// pauseOwned pauses the pair of a run another machine owns, which `start` calls for each owned_elsewhere entry of its reconcile; nil skips the pause.
	pauseOwned func(location *lyxcwd.Location, slug string) error
	// startDobbies starts the dobby of the prime and of every beam this machine owns, which `start` calls first of all; nil starts none, as in a hand-built harness.
	// Its failure never stops the start: it becomes a warning.
	startDobbies func() error
}

// Command returns the cobra command tree for the orch module.
func Command() *cobra.Command {
	return commandOf(&orchCLI{})
}

// commandOf returns the orch command tree whose verbs hang off c, so a test can read what PersistentPreRunE wires into c.
func commandOf(c *orchCLI) *cobra.Command {
	parent := &cobra.Command{
		Use:   "orch",
		Short: "run the hub orchestrator as a reed strand with automatic handoff",
		Long: `orch hosts the hub orchestrator as an interactive Claude session in a reed
strand of the hub's prime worktree or of a beam's. A detached watcher reads each
turn's context usage and, past a threshold and while the session is idle, has the
session write a handoff, clears it and resumes it from that handoff.
Every verb runs from the hub's prime worktree or from a beam.`,
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
				return fail(fmt.Errorf("orch: lyx orch runs from the hub's prime worktree or a beam: %w", err))
			}
			primeName, primeNameErr := fabricengine.PrimeName(location)
			beam := false
			if primeNameErr == nil && location.WorktreeName != primeName {
				origin, found, err := fabricengine.ReadOrigin(location)
				if err != nil {
					return fail(fmt.Errorf("orch: read the origin record of %q to tell whether it is a beam: %w", location.WorktreeName, err))
				}
				beam = found && origin.Beam
			}
			if err := refuseNonPrime(location.WorktreeName, primeName, primeNameErr, beam); err != nil {
				return fail(err)
			}

			// Only start reconciles, and before any module config loads, so a retired key cannot refuse the verb ahead of the reconcile that removes it.
			if cmd.Name() == "start" {
				if geometry, inHub := hubgeom.ReconcileGeometry(location); inHub {
					hubreconcile.EnsureOrWarn(geometry, hubreconcile.Options{})
				}
			}

			orchCfg, err := orchengine.LoadConfig(fabricengine.ConfigDirs(location), "orch")
			if err != nil {
				return fail(err)
			}
			shuttleCfg, err := shuttleengine.LoadConfig(fabricengine.ConfigDirs(location), "shuttle")
			if err != nil {
				return fail(err)
			}
			reedCfg, err := reedengine.LoadConfig(fabricengine.ConfigDirs(location), "reed")
			if err != nil {
				return fail(err)
			}

			reedGeom, err := hubgeom.ReedGeometry(location)
			if err != nil {
				return fail(err)
			}
			reed := reedengine.New(reedCfg, reedGeom, shuttleengines.EnvScrub())

			c.location = location
			c.runsWindowTitle = runsWindowTitleOf(location.WorktreeName, beam)
			c.cfg = orchCfg
			c.shuttleCfg = shuttleCfg
			engines, err := shuttleengines.New(shuttleCfg, fabricengine.ConfigDirs(location))
			if err != nil {
				return fail(err)
			}
			c.engines = engines
			c.runner = shuttleengine.NewRunner(reed, engines, reedGeom.AnchorPath, reedGeom.WorktreeRoot, shuttleCfg)
			c.strands = newReedStrandOps(reed)
			c.paths = orchPaths(location)
			c.stencilsDir = fabricengine.StencilsDir(location.HubPath)
			pauseGeometry, err := hubgeom.PauseGeometry(location)
			if err != nil {
				return fail(err)
			}
			c.pauseRecordPath = pauseGeometry.RecordPath
			c.reedUp = func() error { _, err := reed.Up(); return err }
			c.starter = runnerSessionStarter{runner: c.runner}
			c.newCreel = func() (orchengine.Creel, error) {
				client, err := creelcli.NewProgramClient(location, orchWatchProgram)
				if err != nil {
					return nil, err
				}
				return creelSeam{client: client}, nil
			}
			c.spawnWatcher = c.spawnWatcherProcess
			c.startLockWait = stopStartLockWait
			c.sleep = time.Sleep
			c.indexRunner = runHelpIndex
			c.executable = os.Executable
			c.currentBuild = statuspane.CurrentBuild
			c.reconcile = func() (fabricengine.OriginReport, error) { return reconcileHub(location) }
			if beam {
				c.beamStart = func() error { return checkBeamStart(location) }
			} else {
				c.claimPrime = func() (string, string, error) { return claimPrimeFor(location) }
			}
			c.pauseOwned = installedOwnerPause()
			c.startDobbies = func() error { return ensureDobbies(location) }
			return nil
		},
	}

	parent.AddCommand(c.startCmd(), c.statusCmd(), c.refreshCmd(), c.distillCmd(), c.withdrawCmd(), c.stopCmd(), c.rulesCmd(), c.watchCmd(), c.resumeContextCmd(), c.cleanupCmd())
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
