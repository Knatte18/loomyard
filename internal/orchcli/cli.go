// cli.go builds the cobra command tree for the orch module and wires its verbs to the engines they call.
// The parent "orch" command's PersistentPreRunE resolves the cwd and hands the location to wireCLI;
// wireCLI fills one receiver that every verb closes over, so no verb re-resolves geometry itself.
// BeamUp and BeamDown run the same wiring for a caller that is no orch verb.

package orchcli

import (
	"fmt"
	"io"
	"os"
	"time"

	"github.com/Knatte18/loomyard/internal/agentconfig"
	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/creelcli"
	"github.com/Knatte18/loomyard/internal/dobbyengine"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/hubgeom"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/orchengine"
	"github.com/Knatte18/loomyard/internal/output"
	"github.com/Knatte18/loomyard/internal/reedengine"
	"github.com/Knatte18/loomyard/internal/runproc"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/shuttleengines"
	"github.com/Knatte18/loomyard/internal/slackengine"
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

func newReedStrandOps(reed *reedengine.Engine) reedStrandOps {
	return reedStrandOps{status: reed.Status, remove: reed.RemoveStrand, add: reed.AddStrand, replace: reed.ReplaceStrand}
}

func (o reedStrandOps) AddStrand(spec reedengine.AddSpec) (reedengine.Strand, error) {
	return o.add(spec)
}

// ReplaceStrand replaces the strand guid with one built from spec, keeping its slot.
func (o reedStrandOps) ReplaceStrand(guid string, spec reedengine.AddSpec) (reedengine.Strand, error) {
	return o.replace(guid, spec)
}

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

// orchCLI is the receiver every orch verb hangs off, populated by wireCLI.
type orchCLI struct {
	location   *lymcwd.Location
	cfg        orchengine.Config
	shuttleCfg shuttleengine.Config
	// orchSkills is the orch role's skill list from agents.yaml, which the watcher re-sends when the orch reloads.
	orchSkills  []string
	engines     shuttleengine.EngineSet
	runner      *shuttleengine.Runner
	strands     strandOps
	paths       orchengine.Paths
	stencilsDir string
	// pauseRecordPath is the hub's pause record; status, refresh and distill read it to tell a request held by a hub pause.
	pauseRecordPath string

	reedUp  func() error
	starter sessionStarter
	// newCreel builds the creel seam the watcher and the bring-up call creel through, signing as the ORCHWATCH program.
	newCreel     func() (orchengine.Creel, error)
	spawnWatcher func() error
	// startLockWait is how long `stop` waits for start.lock before it refuses.
	startLockWait time.Duration
	sleep         func(time.Duration)
	// executable returns the path of the running lym binary, which the runs strand's command names.
	executable func() (string, error)
	// currentBuild identifies the running lym build, which tells a runs strand of this build from an older one.
	currentBuild func() (statuspane.BuildIdentity, error)
	// runsWindowTitle names the strand's window on add or replace: `beam: prime` in the prime, `beam: <slug>` in a beam;
	// with the beam's board handle once the board names one; a live strand of this build keeps its title until replaced.
	runsWindowTitle func() string
	// reconcile judges and moves the hub's branches against origin; a Prepare bring-up runs it before any strand decision.
	reconcile func() (fabricengine.OriginReport, error)
	// readBeamFacts reads the owner, standing anchor change and landing hold a bring-up refuses on; nil refuses nothing.
	readBeamFacts func() (beamFacts, error)
	// claimPrime records this machine as prime owner if the board has none; a Prepare bring-up calls it after reconcile.
	// claimed names this machine after a claim, lostTo the other machine when its claim won; nil skips the claim.
	claimPrime func() (claimed, lostTo string, err error)
	// pauseOwned pauses the pair of each run another machine owns in a Prepare bring-up; nil skips the pause.
	pauseOwned func(location *lymcwd.Location, slug string) error
	// startDobby starts this worktree's dobby when it is not live and reports the outcome; nil starts none.
	// Its failure never stops the bring-up: it becomes a warning.
	startDobby func() (dobbyengine.StartEntry, error)
	// startRelay starts the hub's Slack relay when it is down and reports the outcome; nil, as for a beam, starts none.
	// Its failure never stops the bring-up: it becomes a warning.
	startRelay func() (slackengine.StartOutcome, error)
	// dobbyLive reports whether this worktree's dobby holds its lock; nil leaves the dobby alone on a bring-down.
	dobbyLive func() (bool, error)
	// endDobby ends this worktree's dobby and the processes below it, and no other process of the beam.
	endDobby func() error
	// reapInherited waits on the children a re-exec'd watcher inherited, which `watch` calls before it starts anything.
	reapInherited func()
}

// Command returns the cobra command tree for the orch module.
func Command() *cobra.Command {
	return commandOf(&orchCLI{})
}

// commandOf returns the orch command tree built on c, so a test can read what PersistentPreRunE wires into c.
func commandOf(c *orchCLI) *cobra.Command {
	parent := &cobra.Command{
		Use:   "orch",
		Short: "run the hub orchestrator as a reed strand with automatic handoff",
		Long: `orch hosts the hub orchestrator as an interactive Claude session in a reed
strand of the hub's prime worktree or of a beam's. A detached watcher reads each
turn's context usage and, past a threshold and while the session is idle, has the
session write a handoff, clears it and resumes it from that handoff.
"lym beam start" brings the orchestrator up and "lym beam stop" takes it down.
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

			cwd, err := lymcwd.CwdFrom(ctx)
			if err != nil {
				return fail(err)
			}
			location, err := lymcwd.Resolve(cwd)
			if err != nil {
				return fail(err)
			}
			if err := wireCLI(c, location); err != nil {
				return fail(err)
			}
			return nil
		},
	}

	parent.AddCommand(c.statusCmd(), c.refreshCmd(), c.distillCmd(), c.withdrawCmd(), c.stopCmd(), c.rulesCmd(), c.watchCmd(), c.resumeContextCmd())
	return parent
}

// wireCLI fills c for the worktree at location: the config, the reed engine, the runner, the paths and every seam.
// It refuses a worktree that is neither the hub's prime nor a beam before it reads any config.
func wireCLI(c *orchCLI, location *lymcwd.Location) error {
	if err := fabricengine.RequireDrivableWorktree(location); err != nil {
		return fmt.Errorf("orch: lym orch runs from the hub's prime worktree or a beam: %w", err)
	}
	primeName, primeNameErr := fabricengine.PrimeName(location)
	beam := false
	if primeNameErr == nil && location.WorktreeName != primeName {
		origin, found, err := fabricengine.ReadOrigin(location)
		if err != nil {
			return fmt.Errorf("orch: read the origin record of %q to tell whether it is a beam: %w", location.WorktreeName, err)
		}
		beam = found && origin.Beam
	}
	if err := refuseNonPrime(location.WorktreeName, primeName, primeNameErr, beam); err != nil {
		return err
	}

	orchCfg, err := orchengine.LoadConfig(fabricengine.ConfigDirs(location), "orch")
	if err != nil {
		return err
	}
	shuttleCfg, err := shuttleengine.LoadConfig(fabricengine.ConfigDirs(location), "shuttle")
	if err != nil {
		return err
	}
	orchSkills, err := shuttleCfg.Agents.Skills(agentconfig.RoleOrch, nil)
	if err != nil {
		return err
	}
	reedCfg, err := reedengine.LoadConfig(fabricengine.ConfigDirs(location), "reed")
	if err != nil {
		return err
	}

	reedGeom, err := hubgeom.ReedGeometry(location)
	if err != nil {
		return err
	}
	reed := reedengine.New(reedCfg, reedGeom, shuttleengines.EnvScrub())

	c.location = location
	c.runsWindowTitle = func() string { return runsWindowTitleIn(location, beam) }
	c.cfg = orchCfg
	c.shuttleCfg = shuttleCfg
	c.orchSkills = orchSkills
	engines, err := shuttleengines.New(shuttleCfg, fabricengine.ConfigDirs(location))
	if err != nil {
		return err
	}
	c.engines = engines
	c.runner = shuttleengine.NewRunner(reed, engines, reedGeom.AnchorPath, reedGeom.WorktreeRoot, shuttleCfg)
	c.strands = newReedStrandOps(reed)
	c.paths = orchPaths(location)
	c.stencilsDir = fabricengine.StencilsDir(location.HubPath)
	pauseGeometry, err := hubgeom.PauseGeometry(location)
	if err != nil {
		return err
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
	c.reapInherited = reapInheritedChildren
	c.executable = os.Executable
	c.currentBuild = statuspane.CurrentBuild
	c.reconcile = func() (fabricengine.OriginReport, error) { return reconcileHub(location) }
	c.readBeamFacts = func() (beamFacts, error) { return readBeamFacts(location, beam) }
	c.claimPrime = func() (string, string, error) { return claimPrimeFor(location) }
	c.pauseOwned = installedOwnerPause()
	c.startDobby = func() (dobbyengine.StartEntry, error) { return ensureDobbies(location) }
	c.startRelay = relayStarter(location, beam)
	c.dobbyLive = func() (bool, error) {
		prime, err := resolvePrime(location)
		if err != nil {
			return false, err
		}
		geometry, err := hubgeom.DobbyGeometry(prime, location.WorktreeName)
		if err != nil {
			return false, err
		}
		return dobbyengine.Live(geometry)
	}
	owner := runproc.HubOwner
	if beam {
		owner = location.WorktreeName
	}
	c.endDobby = func() error {
		_, err := runproc.EndRole(fabricengine.RunProcDir(location.HubPath), owner, runproc.RoleDobby, os.Getpid())
		return err
	}
	return nil
}

// RunCLI runs the orch module CLI on args and writes its output to out.
func RunCLI(out io.Writer, args []string) int {
	return RunCLIIn("", out, args)
}

// RunCLIIn runs the orch module CLI on args with cwd as the working directory, writing its output to out.
// An empty cwd reads the process working directory instead, since the execution context panics on an empty directory.
func RunCLIIn(cwd string, out io.Writer, args []string) int {
	if cwd == "" {
		return clihelp.Execute(Command(), out, args)
	}
	return clihelp.ExecuteIn(Command(), cwd, out, args)
}
