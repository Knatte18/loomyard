// arm.go implements loomcli's exported Arm resolution entry point, plus the two-function split
// behind it: the unexported worker arm resolves cwd and wires the whole engine stack exactly as
// resolvePersistentPreRun (cli.go) always has, and the resolution-free specFor fills a Spec from
// an already-wired receiver. See the overview's exported-Arm-is-the-single-resolution-entry-point
// and spec-fill-is-separable-from-resolution Shared Decisions.
//
// Arm carries no command-name guard of its own: the existing cmd.Name() == "loom" short-circuit
// stays in resolvePersistentPreRun, where it lets a bare group listing run without a git
// repository -- a bare "lyx shed" listing is shedcli's own equivalent guard, not this module's.
//
// arm's --parent disposition differs by entry point, and that difference is real rather than an
// oversight: "lyx shed step --recipe loom" registers no --parent flag of its own, so c.parentFlag
// is the empty string there -- exactly the value "lyx loom step" passes when the operator omits
// the flag, so the two paths agree and the provenance record is simply never written from the
// shed path.
//
// arm also resolves the run-id every one of loom's four generic verbs addresses -- args[0] when
// present, shedrun.SelfRunID otherwise -- and, for those four verbs alone, refuses when no seed
// exists at that run-id. "lyx loom start" is not a generic verb and never reaches this check: it is
// the one site that writes a seed, per the batch's loom's-run-and-step-do-not-auto-seed decision.
// See genericShedVerb's own doc comment for the exact four-verb set.

package loomcli

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/friction"
	"github.com/Knatte18/loomyard/internal/frictionengine"
	"github.com/Knatte18/loomyard/internal/lock"
	"github.com/Knatte18/loomyard/internal/loomengine"
	"github.com/Knatte18/loomyard/internal/loomrecipe"
	"github.com/Knatte18/loomyard/internal/loomshed"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/selfreportengine"
	"github.com/Knatte18/loomyard/internal/shedadapters"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shedrun"
	"github.com/Knatte18/loomyard/internal/shedverbs"
)

// genericShedVerb reports whether verb is one of the four verbs shedverbs.Verbs drives -- run,
// step, status, pause -- as opposed to loom's own hand-written verbs (start,
// validate-discussion, validate-plan), which never reach arm's seed-presence check below.
func genericShedVerb(verb string) bool {
	switch verb {
	case "run", "step", "status", "pause":
		return true
	default:
		return false
	}
}

// resolveRunID resolves the run-id verb addresses -- args[0] when present, shedrun.SelfRunID
// otherwise -- and records it on c before either wiring call, unconditionally for every verb, not
// only the four generic ones, so wireLightweight and wire (wiring.go) can read c.runID back to
// build every shedrun.* path, in place of a hardcoded shedrun.SelfRunID.
//
// For the four generic verbs alone (genericShedVerb), it also refuses when no seed exists at that
// run-id: ReadSeed answers found == false purely from seed.json's own absence, so a status file
// present with no seed beside it takes the same refusal. "lyx loom start" is not a generic verb and
// never reaches this refusal -- it is the one site that writes a seed, per the batch's
// loom's-run-and-step-do-not-auto-seed decision.
//
// It performs no I/O beyond shedrun.ReadSeed and shedrun.List, both pure filesystem reads under
// location's own AnchorPath -- no git spawn -- so a test can drive it directly against a hand-built
// *lyxcwd.Location with no real git repository behind it, which is what lets arm_seed_test.go stay
// Tier 1 despite arm itself needing lyxcwd.Resolve's real git spawn.
func (c *loomCLI) resolveRunID(location *lyxcwd.Location, verb string, args []string) error {
	runID := shedrun.SelfRunID
	if len(args) > 0 {
		runID = args[0]
	}
	c.runID = runID

	if !genericShedVerb(verb) {
		return nil
	}

	_, found, err := shedrun.ReadSeed(location, runID)
	if err != nil {
		return err
	}
	if found {
		return nil
	}

	existing, err := shedrun.List(location)
	if err != nil {
		return err
	}
	// Rendered through shedrun.MissingSeedMessage, the shared renderer batch 1 card 3 declares, so
	// this refusal words identically to battencli's own. The rendered text carries no "kind" field:
	// it surfaces through resolvePersistentPreRun's own output.Err(out, err.Error()) path, which is
	// kind-less by construction, never through step's own output.ErrFields envelope -- this refusal
	// fires before PreStep ever runs.
	return errors.New(shedrun.MissingSeedMessage("loom", runID, existing, `run "lyx loom start" first to bootstrap this task`))
}

// arm resolves cwd into a *lyxcwd.Location, resolves and records the addressed run-id, refuses
// when one of the four generic verbs addresses a run-id with no seed, wires the receiver's whole
// engine stack -- through wireLightweight when verb is one of the lightweight verbs, through the
// full wire otherwise -- and returns the filled Spec. It is the single worker both
// resolvePersistentPreRun and the exported Arm wrapper delegate to, so the *loomCLI whose fields
// the returned hooks close over is always the same value the caller holds: the pre-run's own c on
// the "lyx loom" path, the wrapper's freshly-constructed one on the "lyx shed" path.
//
// The seed-presence refusal runs before either wiring call -- before wireLightweight or wire have
// touched the filesystem at all -- so a refusing run/step writes nothing to disk.
func (c *loomCLI) arm(cwd string, verb string, args []string) (shedverbs.Spec, error) {
	location, err := lyxcwd.Resolve(cwd)
	if err != nil {
		// lyxcwd.Resolve's error is already self-describing (it IS the "not a git repository"
		// sentinel); pass it through bare rather than doubling that same text on top of it.
		return shedverbs.Spec{}, err
	}

	if err := c.resolveRunID(location, verb, args); err != nil {
		return shedverbs.Spec{}, err
	}

	if verbUsesLightweightWiring(verb) {
		c.wireLightweight(location, cwd)
	} else if err := c.wire(location, cwd); err != nil {
		return shedverbs.Spec{}, err
	}

	return c.specFor(verb), nil
}

// specFor fills a Spec from c's already-wired fields, performing no resolution, no wire call, and
// no I/O of its own. internal/loomcli's own untagged cli_test.go, status_test.go and step_test.go
// call this directly against a hand-populated receiver, so their leaf-command tests never spawn
// git.
func (c *loomCLI) specFor(verb string) shedverbs.Spec {
	spec := shedverbs.Spec{
		StatusPath:          c.shedPaths.StatusPath,
		LockPath:            c.shedPaths.LockPath,
		StatusLockPath:      c.shedPaths.StatusLockPath,
		EnsureStatusLockDir: true,
		StatusLabel:         "loom",
		DecodeErrPrefix:     "loom:",
		// Both busy messages are passthrough: loom's run reports the bare ErrShedBusy sentinel
		// text as an ordinary error envelope today, and step reports err.Error() verbatim
		// alongside its own kind.
		RunBusyMessage:  "",
		StepBusyMessage: "",
		StepBusyKind:    shedverbs.KindBusy,
		AbsentStatus: shedverbs.AbsentDisposition{
			Refuse:        true,
			RefuseMessage: "loom: no status file at " + c.shedPaths.StatusPath + "; run \"lyx loom start\" first to bootstrap this task",
		},
		PauseAbsentMessage: "loom: no status file at " + c.shedPaths.StatusPath + "; there is nothing running to pause -- run \"lyx loom start\" first to bootstrap this task",
		Hooks: shedverbs.Hooks{
			PreRun:             c.loomPreRun,
			PostRun:            c.loomPostRun,
			PreStep:            c.loomPreStep,
			PostStep:           c.loomPostStep,
			InterruptPolicyFor: loomshed.InterruptPolicyFor,
			StatusExtras:       c.loomStatusExtras,
		},
	}

	// BuildShed is filled per verb, never left generic: step drives c.buildLoomShed, which
	// already performs the whole fabricengine.Open/CurrentBranch/OriginURL/ReadOrigin/
	// resolveLandingParent block and the c.env.Landing assignment, while run performs that same
	// block inline in loomPreRun and then calls loomrecipe.New directly -- giving run a
	// buildLoomShed-backed BuildShed would open the fabric and read origin twice where it does so
	// once today. status and pause leave BuildShed nil, since neither ever calls it.
	switch verb {
	case "step":
		spec.BuildShed = c.buildLoomShed
	case "run":
		spec.BuildShed = func() (*shedengine.Shed, error) { return loomrecipe.New(c.env, c.shedPaths) }
	}

	return spec
}

// Arm is loomcli's exported resolution entry point, for internal/shedcli's table: it constructs a
// fresh receiver via newLoomCLI and returns c.arm(cwd, verb, args). A package-level Arm alone
// could not serve resolvePersistentPreRun, because that pre-run must wire its own c: start.go
// reads thirteen receiver fields and validate.go reads four more, so arming a throwaway receiver
// and assigning only *c.spec would break start, validate-discussion and validate-plan, none of
// which is a shedverbs verb and none of which reads c.spec.
func Arm(cwd string, verb string, args []string) (shedverbs.Spec, error) {
	c := newLoomCLI()
	return c.arm(cwd, verb, args)
}

// loomPreRun implements the PreRun hook for loom's spec: run's whole existing pre-flight, in
// today's order, each step's error becoming a returned error the generic body reports on the
// error envelope exactly as run's own inline handling did.
func (c *loomCLI) loomPreRun(ctx context.Context) error {
	// The status-file existence refusal, naming "lyx loom start" as the remedy: only that verb
	// may seed, because only it owns the commit-before-precondition ordering the bootstrap needs.
	if _, err := os.Stat(c.shedPaths.StatusPath); err != nil {
		return errors.New("loom: no status file at " + c.shedPaths.StatusPath + "; run \"lyx loom start\" first to bootstrap this task")
	}
	// The status file must be THIS task's own, for the same reason "lyx loom start" checks: a
	// worktree forked from a task worktree inherits the old task's `_lyx` state, and the phase
	// machine would silently resume the inherited run under the wrong slug (crucible round
	// fable5-high-r3, F-B7).
	if err := loomengine.VerifySeedOwnership(c.shedPaths.StatusPath, c.shedPaths.StatusLockPath, seedSlug(c.location.WorktreeName)); err != nil {
		return err
	}

	// Observed here, next to the read VerifySeedOwnership just performed, and guarded on the
	// knob directly: a disabled run must not pay for a lock probe and an extra status decode on
	// every run. Carried on the receiver rather than returned, since PreRun returns no map of its
	// own -- loomPostRun reads it back.
	c.entryObservation = observeEntry(c.cfg.Selfreport, c.shedPaths.LockPath, c.shedPaths.StatusPath, c.shedPaths.StatusLockPath, loomengine.LoomStepHandoff(c.location), loomengine.LoomStepHandoffLock(c.location))

	// Ensure the reed substrate before the first producer call: the rows beneath run spawn
	// agents into reed panes, so without a live session the run gets several producers deep and
	// then hard-errors. Up is idempotent and is what "lyx loom start" already calls at its own
	// step 4.
	if _, err := c.reed.Up(); err != nil {
		return err
	}

	handle, err := fabricengine.Open(c.location)
	if err != nil {
		return err
	}
	taskBranch, err := handle.CurrentBranch()
	if err != nil {
		return err
	}
	originURL, err := handle.OriginURL()
	if err != nil {
		// scalar-read-errors-refuse-or-defer-by-consumer: only Publish reads OriginURL, and only
		// when a pull request is actually required, so an unusable origin URL passes through as
		// an empty string rather than refusing run itself.
		originURL = ""
	}
	recorded, found, err := fabricengine.ReadOrigin(c.location)
	if err != nil {
		return err
	}
	parentBranch, err := resolveLandingParent(recorded, found, taskBranch)
	if err != nil {
		return err
	}
	syncOpts := fabricengine.EnvSyncOptions()
	pushBranch := func() error {
		_, err := handle.PushBranch(syncOpts)
		return err
	}
	c.env.Landing = landingDeps(
		c.location,
		c.runDeps.Geom,
		taskBranch,
		originURL,
		parentBranch,
		syncOpts.SkipPush,
		pushBranch,
		c.registry,
		c.runner,
		c.landingCfg,
	)

	// Ensure the friction directory before the run starts, and never clear it here: run requires
	// an already-seeded task (VerifySeedOwnership above), so a run-only invocation is by
	// definition a resume.
	friction.EnsureDir(c.frictionDir)

	return nil
}

// loomPostRun implements the PostRun hook for loom's spec: it fires detectAndFileAnomalies
// unconditionally -- including on the hard-error arm, which is why PostRun itself runs
// unconditionally -- and then returns the envelope's "friction" key, valued
// frictionengine.StatusSkipped when reflection does not fire. The key is returned unconditionally
// on the success path so it is never silently dropped from a RunPaused envelope.
func (c *loomCLI) loomPostRun(ctx context.Context, result shedengine.Result, runErr error) map[string]any {
	detectAndFileAnomalies(selfreportDeps{
		Ctx:            ctx,
		Selfreport:     c.cfg.Selfreport,
		Entry:          c.entryObservation,
		StatusPath:     c.shedPaths.StatusPath,
		StatusLockPath: c.shedPaths.StatusLockPath,
		MarkerPath:     loomengine.LoomSelfreportFiled(c.location),
		MarkerLockPath: loomengine.LoomSelfreportFiledLock(c.location),
		RunErr:         runErr,
		IsLedgerPath:   shedadapters.IsLedgerPath,
		ReadLedger:     shedadapters.ReadLedger,
		FileIssue:      selfreportengine.CreateIssue,
	})

	frictionStatus := frictionengine.StatusSkipped
	if shouldReflectFriction(c.frictionDir, result.Outcome) {
		frictionStatus = c.reflectFriction()
	}
	return map[string]any{"friction": frictionStatus}
}

// loomPreStep implements the PreStep hook for loom's spec: the early run-lock probe, then step's
// bootstrap, then the status-strand ensure -- today's stepCmd body, in today's order, each
// returned error paired with its refusal kind.
func (c *loomCLI) loomPreStep(ctx context.Context) (string, error) {
	// The MkdirAll is part of the probe rather than an accident of ordering: the run lock lives
	// in the ephemeral tree, internal/lock opens with O_CREATE but never creates a parent, and
	// "start" creates that same directory at its own step 4 before its own step-5 probe -- so
	// hoisting only the probe would run it on a fresh worktree whose parent directory does not
	// exist. Treating a missing-parent error as "lock free" is explicitly rejected, because it
	// would convert a filesystem fault into a false green light on the one check guarding
	// against a second driver.
	if err := os.MkdirAll(filepath.Dir(c.shedPaths.LockPath), 0o755); err != nil {
		return shedverbs.KindBootstrap, err
	}
	// Without this early probe, step would seed, commit, bring up reed, and churn the status
	// strand against a task a live driver owns.
	probe, runLockFree, err := lock.TryAcquireWriteLock(c.shedPaths.LockPath)
	if err != nil {
		return shedverbs.KindBootstrap, err
	}
	if runLockFree {
		_ = probe.Release()
	} else {
		return shedverbs.KindBusy, errors.New("loom: a driver already holds the run lock; run \"lyx loom pause\" to request a pause at the next producer boundary")
	}

	slug := seedSlug(c.location.WorktreeName)
	_, stage, err := c.seedAndCommitBootstrap(slug, c.parentFlag)
	if err != nil {
		return stepKindForBootstrapStage(stage), err
	}

	bootstrapLockPath := loomengine.LoomBootstrapLock(c.location)
	if err := os.MkdirAll(filepath.Dir(bootstrapLockPath), 0o755); err != nil {
		return shedverbs.KindBootstrap, err
	}
	bootstrapLock, err := lock.AcquireWriteLock(bootstrapLockPath)
	if err != nil {
		return shedverbs.KindBootstrap, err
	}
	if err := c.ensureStatusStrand(); err != nil {
		_ = bootstrapLock.Release()
		return shedverbs.KindBootstrap, err
	}
	// Released immediately, before the producer call: the lock must never be held across a
	// minutes-long LLM row.
	_ = bootstrapLock.Release()

	return "", nil
}

// loomPostStep implements the PostStep hook for loom's spec: it records the clean-handoff marker
// after a successful shed.Step and before the envelope is written -- a completed step's persisted
// aftermath is byte-identical to a mid-run driver death, and this marker is the one thing letting
// the next run's entry observation tell the two apart, so its call site can move neither above the
// Step call nor below the envelope.
func (c *loomCLI) loomPostStep(res shedengine.StepResult) {
	recordStepHandoff(loomengine.LoomStepHandoff(c.location), loomengine.LoomStepHandoffLock(c.location), len(res.History), res.State)
}

// loomStatusExtras implements the StatusExtras hook for loom's spec: loom's own five status keys,
// decoded from st.Product only when non-empty, and no others.
func (c *loomCLI) loomStatusExtras(st shedengine.Status) (map[string]any, error) {
	var product loomengine.Status
	if len(st.Product) > 0 {
		if err := json.Unmarshal(st.Product, &product); err != nil {
			// Returned verbatim, with no re-prefixing: the generic body reports the hook's
			// whole string as-is.
			return nil, errors.New("loom: decode status file " + c.shedPaths.StatusPath + "'s product payload: " + err.Error())
		}
	}
	return map[string]any{
		"pause_requested":  st.PauseRequested,
		"history_length":   len(st.History),
		"slug":             product.Slug,
		"parent":           product.Parent,
		"interrupt_policy": loomshed.InterruptPolicyFor(st.CurrentProducer),
	}, nil
}
