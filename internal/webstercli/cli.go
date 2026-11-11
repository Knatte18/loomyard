// cli.go builds the cobra command tree for the webster module.
// It also holds the RunCLI seam, which adapts that tree to the standard io.Writer call contract.
// The parent "webster" command carries a PersistentPreRunE.
// It resolves cwd, runs one preflight.ResolveMode probe, and delegates to c.wire (wiring.go).
// c.wire selects hub or standalone mode and builds the whole engine stack for the mode that wins.
// It stores the resolved ingredients on websterCLI exactly once per invocation.
// A non-nil ResolveMode error means refuse.
// The pre-run aborts there, before c.wire is called, since refusal is a resolution verdict, not a wiring choice.
//
// The module holds no *lymcwd.Location.
// Every path it touches, including every _lym/plan and _lym/webster path, arrives as a websterengine.Geometry.
// hubgeom.WebsterGeometry(loc) supplies it in hub mode.
// standalonegeom.WebsterGeometry(target, stateDir) supplies it in standalone.
// No path is re-derived from a stored Location.
// Fabric is reached only through the lazy c.openFabric closure.
// That closure is nil in standalone mode, which has no fabric repo, and wiring never calls it in either mode.
//
// websterCLI stores two adapted views of the one constructed Runner.
// starter (websterengine.Starter) is webster's own local copy of the spawn seam.
// The cold-strand spawns of recover-batch, fix-verify and fix-plan consume it.
// masterStarter (websterengine.MasterStarter) sits behind the runnerMasterStarter adapter.
// The run verb's Master spawn consumes it.
// The seams differ because each verb needs its own narrow view of the same *shuttleengine.Runner.

package webstercli

import (
	"bytes"
	"errors"
	"fmt"
	"io"

	"github.com/Knatte18/loomyard/internal/batcher"
	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/output"
	"github.com/Knatte18/loomyard/internal/pauserecord"
	"github.com/Knatte18/loomyard/internal/planparser"
	"github.com/Knatte18/loomyard/internal/preflight"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/websterengine"
	"github.com/spf13/cobra"
)

// websterCLI is the receiver for every webster verb.
// It holds the ingredients that wire resolves for those verbs.
type websterCLI struct {
	// runner is the constructed shuttle Runner the two adapted seams below are derived from.
	runner *shuttleengine.Runner

	// starter and masterStarter are the two narrow seams webster's verbs spawn through.
	starter       websterengine.Starter
	masterStarter websterengine.MasterStarter

	// engines and reed are the constructed engine set and reed engine that record-batch and recover-batch use directly.
	// engines is the lookup that each recorded session is read through.
	engines websterengine.AuditEngines
	reed    shuttleengine.ReedOps

	// reedUp brings the standalone reed session up, idempotently.
	// Only wireStandalone sets it.
	// run calls it immediately before spawning Master.
	// recover-batch calls it immediately before spawning its cold recovery strand.
	// Standalone mode has no other way to a live session.
	// The hub-only `lym reed up` requires lymcwd.Resolve in its pre-run, so it cannot start standalone's session.
	// Its socket is lym-<hash8>, its state lies under the derived state directory, and it can only boot in-process.
	//
	// Only those two verbs call it, never wiring, so validate, status and await-batch boot no tmux server.
	// The rule is whether a verb starts an OS process of its own, not whether it writes state.
	// begin-batch and record-batch write state but only inject into or read around a pane Master already owns,
	// so a session they reach exists by construction whenever they are legitimately called.
	// It stays nil in hub mode, where bringing reed up is the operator's or loom's own act.
	reedUp func() error

	// planDirOverridden reports whether --plan-dir moved the plan off the mode's own default.
	// The default is <stateDir>/_lym/plan in standalone and the hub anchor's _lym/plan in hub mode.
	// Both wiring paths set it, and only the run verb reads it, to refuse spawning Master over a moved plan.
	// Master's in-pane verb invocations are flagless and resolve the default, so they could never see the override.
	// The Master stencil drives `lym webster begin-batch <NN>` flagless in both modes; only {{.plan_dir}} is templated.
	// Hub mode sets the field too, so run refuses over a moved plan that the in-pane begin-batch could not see.
	// planDirDefault holds that default path for the refusal's recourse text.
	planDirOverridden bool
	planDirDefault    string

	shuttleCfg shuttleengine.Config
	cfg        websterengine.Config
	roles      map[websterengine.Role]shuttleengine.ModelChoice

	// instructionFiles lists the instruction files Master's engine names, which count toward the start base; nil means none.
	// It looks the engine up when called, so a verb that never prices a partition is never refused over the master key.
	instructionFiles func() ([]string, error)

	// anchorRel is the hub-mode Location's anchor-relative path (loc.AnchorRel), and it is empty in standalone.
	// Fabric sync needs it to scope its pathspec with fabricengine.ScopedPathspec.
	// No other field supplies that path.
	anchorRel string

	// geom is the websterengine.Geometry that every verb's Deps construction reads.
	// It is hubgeom.WebsterGeometry(loc) in hub mode and standalonegeom.WebsterGeometry(target, stateDir) in standalone.
	// Every path this module touches is read from it.
	geom websterengine.Geometry
	// refMatcher is the injected fabric-reference class matcher that record-batch's and run's audits consult.
	// It is a real *fabricengine.RefScanner in hub mode.
	// It is built eagerly, because NewRefScanner only compiles a regexp and cannot fail.
	refMatcher websterengine.RefMatcher
	// openFabric is the lazy fabric-handle opener that the verbs' fabric commits go through.
	// It must not be opened during PersistentPreRunE, since fabricengine.Open stat-checks the paired sibling.
	// Opening it there would fail the pre-run in healthy but unwired locations.
	// Those are the locations that validate and status run in, and neither verb commits to the fabric.
	openFabric func() (*fabricengine.Fabric, error)
	// parentBranch lazily reads the pair's recorded parent branch from fabric's origin record.
	// The clean-parent-merge rule needs it; standalone leaves it nil, since no merge commit is accepted.
	parentBranch websterengine.ParentBranchFunc
	// runProcDir is the hub-level directory holding the owner records of a run's processes; wireHub sets it.
	// pairSlugRead lazily reads the worktree's pair slug from fabric's origin record; it stays nil in standalone.
	runProcDir   string
	pairSlugRead func() string

	// batcher is the load-time-resolved, config-selected batchifier.
	batcher batcher.Batcher

	// languageSkills are the hub's language skills the master and the recovery strand load after their own.
	// wireHub loads them once per process from the hub's language.yaml; wireStandalone leaves them empty.
	languageSkills []string

	// gateTiers are the hub's rendered tier and gate-list lines the fork and recovery prompts take.
	// wireHub and wireStandalone load them once per process from gate.yaml; an absent file renders none.
	gateTiers string

	// renameTools are the hub's structure-aware rename tools the fork and recovery prompts render.
	// wireHub loads them once per process from the hub's language.yaml; wireStandalone leaves them empty.
	renameTools []string

	// frictionDir is the absolute friction directory in hub mode, resolved tolerantly from loom's own config in wireHub.
	// It is empty in standalone.
	// A standalone webster run is not a loom run and has no friction directory.
	// It is the reason this package imports internal/loomengine at all.
	// The Master stencil drives the batch loop by shelling out to `lym webster` verbs as separate processes.
	// So the per-batch implementer fork's and recovery strand's prompts are composed here.
	// They are not composed in internal/loomcli.
	frictionDir string

	// pause is the pause geometry that the wiring resolves and hands to the runner.
	// wireHub and wireStandalone set it; the verify verb builds its hold from it, and batch verbs read the pause record.
	pause pauserecord.Geometry
	// masterStrand returns the calling Master strand's name and guid.
	// begin-batch and recover-batch claim them into a pause covering the pair; wireHub and wireStandalone set it.
	masterStrand func() (name, guid string, err error)

	// stencilsDirFlag, planDirFlag, and targetDirFlag hold the raw values of the three standalone-entry persistent flags.
	// Command() binds each one.
	// The wiring function reads each inside resolvePersistentPreRun.
	// An empty value means the flag was not passed.
	// The wiring function computes each mode's own default, rather than a zero-value fallback landing here.
	stencilsDirFlag string
	planDirFlag     string
	targetDirFlag   string
}

// runnerMasterStarter adapts *shuttleengine.Runner to websterengine.MasterStarter.
type runnerMasterStarter struct {
	runner *shuttleengine.Runner
}

// StartMaster implements websterengine.MasterStarter.
//
// It starts the run through Runner.StartGated, so the gate reaches the run before the caller blocks on it.
// A zero gate starts the run ungated.
func (s runnerMasterStarter) StartMaster(spec shuttleengine.Spec, gate shuttleengine.GateSpec) (websterengine.MasterHandle, error) {
	run, err := s.runner.StartGated(spec, gate)
	if err != nil {
		return nil, err
	}
	return run, nil
}

// resolvePersistentPreRun resolves the working directory and the preflight mode, then wires the engine stack.
// It calls preflight.ResolveMode, then delegates the mode decision and the stack construction to c.wire.
// c.wire stores the resolved ingredients on c.
// A ResolveMode error is a refusal handled here rather than in c.wire:
// it is a resolution verdict, not a wiring choice,
// so it is surfaced verbatim and aborts the pre-run before c.wire runs.
// It skips resolution when the group command itself runs:
// a bare listing or an unknown subcommand, both routed through clihelp.GroupRunE.
// Neither path needs a git repository.
func (c *websterCLI) resolvePersistentPreRun(cmd *cobra.Command, args []string) error {
	if cmd.Name() == "webster" {
		return nil
	}

	ctx := cmd.Context()
	out := cmd.OutOrStdout()

	cwd, err := lymcwd.CwdFrom(ctx)
	if err != nil {
		output.Err(out, err.Error())
		clihelp.Abort(ctx, 1)
		return nil
	}

	loc, mode, err := preflight.ResolveMode(cwd)
	if err != nil {
		output.Err(out, err.Error())
		clihelp.Abort(ctx, 1)
		return nil
	}

	if err := c.wire(loc, mode, cwd, c.stencilsDirFlag, c.planDirFlag, c.targetDirFlag); err != nil {
		var refusal configLoadRefusal
		if errors.As(err, &refusal) {
			c.refuseConfigLoad(cmd, args, refusal)
			return nil
		}
		output.Err(out, err.Error())
		clihelp.Abort(ctx, 1)
		return nil
	}
	return nil
}

// refuseConfigLoad prints refusal as an error envelope and aborts before the verb's body.
// It sets config_invalid: true only for a content failure, not for an unreadable file.
// It also writes the refusal's friction note itself, since a pre-run refusal never reaches noteRefusals.
func (c *websterCLI) refuseConfigLoad(cmd *cobra.Command, args []string, refusal configLoadRefusal) {
	var fields map[string]any
	if refusal.invalid {
		fields = map[string]any{"config_invalid": true}
	}
	var captured bytes.Buffer
	code := output.ErrFields(io.MultiWriter(cmd.OutOrStdout(), &captured), refusal.Error(), fields)
	clihelp.Abort(cmd.Context(), code)
	if c.frictionDir != "" {
		c.noteEnvelope(cmd, args, captured.Bytes())
	}
}

// Command returns the cobra command tree for the webster module.
//
// The parent "webster" command's PersistentPreRunE, c.resolvePersistentPreRun, resolves the working directory.
// It wires the engine stack into c.
// It skips resolution when the group command itself is invoked.
func Command() *cobra.Command {
	c := &websterCLI{}

	parent := &cobra.Command{
		Use:   "webster",
		Short: "drive a pinned plan-format plan through a long-lived Master session that forks one implementer per batch",
		Long: `webster takes a pinned plan-format plan (see contracts/specs/loom-plan-spec.md)
and drives it through a long-lived Master session that reads the plan once
and forks one implementer per batch in-session, bracketing each fork with
begin-batch and record-batch, until the plan is built or the run reports
stuck. A fork that reports stuck, or never reports, is escalated
to a cold recovery strand with recover-batch, a verify-gate failure the gate
escalates goes to a cold fixer strand with fix-verify, and a begin-batch plan
conflict goes to a cold plan fixer strand with fix-plan.

Reach for "lym webster run" to build an approved plan, "lym webster validate"
to lint one first, and "lym webster status" to see where a run stands.

webster runs in hub mode inside a lym hub worktree, and in standalone mode in
any other git checkout. --stencils-dir and --plan-dir override where it reads
the stencils and the plan, in both modes; --target-dir names the repository
to drive, in standalone mode only. In standalone mode run boots its own
private reed session before spawning Master.

Example (standalone, outside any lym hub):
  lym webster run --target-dir /path/to/repo`,
		// RunE lists the subcommands for a bare "lym webster" and emits a JSON error envelope for an unknown one.
		RunE:              clihelp.GroupRunE,
		PersistentPreRunE: c.resolvePersistentPreRun,
	}

	parent.PersistentFlags().StringVar(&c.stencilsDirFlag, "stencils-dir", "",
		"override the stencils directory read at call time (read-only in both modes; hub default: the hub's own stencils dir; standalone default: the derived state directory's _lym/stencils)")
	parent.PersistentFlags().StringVar(&c.planDirFlag, "plan-dir", "",
		"override the plan directory parsed at call time (read-only in both modes; hub default: the anchor's _lym/plan; standalone default: the derived state directory's _lym/plan)")
	parent.PersistentFlags().StringVar(&c.targetDirFlag, "target-dir", "",
		"standalone-only: the git repository webster drives Master and its forks against; defaults to the current directory, and either way is lifted to the containing repository's root; refused in hub mode, where the worktree is already the target")

	c.addVerbs(parent)

	return parent
}

// listInstructionFiles returns the instruction file names the start base counts, or nil when no lister is set.
func (c *websterCLI) listInstructionFiles() ([]string, error) {
	if c.instructionFiles == nil {
		return nil, nil
	}
	return c.instructionFiles()
}

// executionBatches returns the batches every verb runs for plan and the loaded state.
// Cards are sized from the task worktree.
// The start base is computed only for a nil state, the one case that forms a partition;
// a recorded partition is mapped without reading it.
func (c *websterCLI) executionBatches(plan *planparser.Plan, st *websterengine.State) ([]batcher.Batch, error) {
	var base batcher.StartBase
	if st == nil {
		files, err := c.listInstructionFiles()
		if err != nil {
			return nil, err
		}
		base, err = websterengine.MerriamBase(c.geom, files)
		if err != nil {
			return nil, fmt.Errorf("%w; way forward: transient, re-run the verb", err)
		}
	}
	return websterengine.ExecutionBatches(plan, st, c.batcher, batcher.DiskSizes(c.geom.WorktreeRoot), base)
}

// addVerbs registers every webster verb under parent.
// Every verb but `validate` and `verify` goes through noteRefusals, so a refusal it prints leaves a friction note.
// `validate` and `verify` are self-checks an agent iterates against.
// Their findings are expected rather than friction.
// A refusal that cobra raises before the verb's RunE stays unnoted.
// It fires before the friction directory is resolved:
// a flag-parse or argument-count error, and one raised in resolvePersistentPreRun.
// The bracket verbs also go through refuseOrphanedMaster, so a Master session whose run ended cannot drive them.
func (c *websterCLI) addVerbs(parent *cobra.Command) {
	parent.AddCommand(c.validateCmd())
	parent.AddCommand(c.verifyCmd())
	for _, verb := range []*cobra.Command{
		c.runCmd(),
		c.statusCmd(),
		c.refuseOrphanedMaster(c.beginBatchCmd()),
		c.refuseOrphanedMaster(c.awaitBatchCmd()),
		c.refuseOrphanedMaster(c.recordBatchCmd()),
		c.refuseOrphanedMaster(c.recoverBatchCmd()),
		c.refuseOrphanedMaster(c.fixVerifyCmd()),
		c.refuseOrphanedMaster(c.fixPlanCmd()),
		c.rebaselineCmd(),
		c.acceptAuditCmd(),
		c.restorePlanCmd(),
		c.resetCmd(),
	} {
		parent.AddCommand(c.noteRefusals(verb))
	}
}

// persistPlanFingerprintRebaseline saves st when the bracket verb that just failed had already
// re-baselined the plan-staleness fingerprint, and returns the save's own error.
//
// Both bracket verbs re-baseline State.PlanFingerprint as soon as a sanctioned plan rewrite lands on disk:
// begin-batch does so while canonicalizing a handle in ValidateDispatch;
// record-batch does so while binding a handle and repairing exact-tier drift.
// Either verb can then fail on a later step of the same call.
// The rewrite is a durable fact about the run, so the re-baseline must outlive the failed call.
// Discarding it would leave the plan on disk carrying the rewrite while state.json keeps the old fingerprint,
// and every later bracket verb would refuse that rewrite as a foreign edit.
//
// fingerprintBefore is the value read out of st immediately before the verb ran.
// An unchanged fingerprint means no rewrite happened, so nothing is written at all.
// That keeps a genuine foreign edit failing ErrFingerprintMismatch.
// Callers invoke this while still holding the state-mutation lease.
//
// It persists only the plan baseline, the fingerprint, the per-file hashes and the overview frame hash.
// It does not save the caller's whole in-memory State.
// The state written here is re-loaded from disk and carries only the new baseline.
// The caller's copy must not be saved whole:
// RecordBatch appends to State.SeenForkTranscripts before the step that can fail,
// and the save would mark transcripts consumed by a call that then failed.
// A resumed record-batch would then find no new transcripts and fail with ErrNoForkTranscripts.
// Re-loading under the still-held lease is safe by construction: nothing else may mutate state while the lease is held,
// so the reload differs from the caller's copy only by the mutations this function drops.
func persistPlanFingerprintRebaseline(geom websterengine.Geometry, st *websterengine.State, fingerprintBefore string) error {
	if st == nil || st.PlanFingerprint == fingerprintBefore {
		return nil
	}
	fresh, err := websterengine.LoadState(geom.WebsterDir, geom.ScratchDir)
	if err != nil {
		return fmt.Errorf("webster: reload state to re-baseline the plan fingerprint: %w", err)
	}
	if fresh == nil {
		return fmt.Errorf("webster: state.json disappeared before the plan-fingerprint re-baseline could be persisted; the plan on disk now carries webster's own rewrite with no state to record it")
	}
	fresh.PlanFingerprint = st.PlanFingerprint
	fresh.PlanFileHashes = st.PlanFileHashes
	fresh.PlanOverviewFrameHash = st.PlanOverviewFrameHash
	return websterengine.SaveState(geom.WebsterDir, geom.ScratchDir, fresh)
}

// RunCLI is the public seam for the webster module CLI.
//
// It delegates to clihelp.Execute with the cobra command tree.
// Out receives all output, including cobra's error text.
func RunCLI(out io.Writer, args []string) int {
	return RunCLIIn("", out, args)
}

// RunCLIIn runs the webster module CLI with cwd seeded into the execution context.
// An empty cwd reads the process working directory instead, through clihelp.Execute.
// The branch exists because lymcwd.WithCwd panics on an empty directory.
func RunCLIIn(cwd string, out io.Writer, args []string) int {
	if cwd == "" {
		return clihelp.Execute(Command(), out, args)
	}
	return clihelp.ExecuteIn(Command(), cwd, out, args)
}
