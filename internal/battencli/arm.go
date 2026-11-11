// arm.go holds the entry points that resolve a batten run's location and wire its engine stack.
// Arm resolves the cwd itself, and ArmAt takes a location and run-id that the caller has already resolved.

package battencli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Knatte18/loomyard/internal/agentname"
	"github.com/Knatte18/loomyard/internal/battenrecipe"
	"github.com/Knatte18/loomyard/internal/battenshed"
	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/dobbyengine"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/hubgeom"
	"github.com/Knatte18/loomyard/internal/hubreconcile"
	"github.com/Knatte18/loomyard/internal/lock"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/machinename"
	"github.com/Knatte18/loomyard/internal/pauserecord"
	"github.com/Knatte18/loomyard/internal/runproc"
	"github.com/Knatte18/loomyard/internal/selfexec"
	"github.com/Knatte18/loomyard/internal/shedadapters"
	"github.com/Knatte18/loomyard/internal/sheddriver"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shedrun"
	"github.com/Knatte18/loomyard/internal/shedtransient"
	"github.com/Knatte18/loomyard/internal/shedverbs"
	"github.com/Knatte18/loomyard/internal/state"
)

// resolveBattenRunID returns the run-id a verb addresses: args[0] when present, shedrun.SelfRunID otherwise.
// It also reports whether the operator supplied the argument.
// The flag matters: an omitted argument and an explicit "self" resolve to one run-id but are refused differently.
func resolveBattenRunID(args []string) (runID string, explicit bool) {
	if len(args) > 0 {
		return args[0], true
	}
	return shedrun.SelfRunID, false
}

// refuseSelfAddress refuses a self address before the auto-seed gate runs.
// Prime hosts many slug-addressed batten runs, so "self" has no run of its own to name there.
//
// An omitted argument is refused as a missing slug.
// Without it, a bare "lym batten run" would take the self default and write prime a seed with an empty params.slug.
// A typed "self" collides with shedrun.SelfRunID; shedrun.IsReserved catches it here.
// IsReserved checks only run-ids derived from a Board slug, so it does not catch an omitted argument.
func refuseSelfAddress(runID string, explicit bool) error {
	if !explicit {
		return fmt.Errorf(
			"battencli: no slug given; batten addresses a task worktree by slug, and prime has no run of its own to default to -- pass the slug: \"lym batten run <slug>\"",
		)
	}
	if shedrun.IsReserved(runID) {
		return fmt.Errorf("battencli: slug %q is reserved for addressing prime's own run and cannot name a task", runID)
	}
	return nil
}

// battenAutoSeedVerbs reports whether verb seeds a fresh run when no seed exists at the addressed run-id.
// Every other verb requires an existing seed and refuses without one.
// Read-only verbs must not create a run as a side effect of being asked a question.
func battenAutoSeedVerbs(verb string) bool {
	switch verb {
	case "run", "step":
		return true
	default:
		return false
	}
}

// battenDriver returns flagVal, or shedrun.DriverGo when flagVal is empty.
// The CLI's flag default fills the field first, so only a direct call with a zero-value receiver sees it empty.
// Batten has no bootstrap verb, so go is its only drivable default.
func battenDriver(flagVal string) string {
	if flagVal == "" {
		return shedrun.DriverGo
	}
	return flagVal
}

// refuseAdoptedSeed refuses a seed at runID that does not describe the run the operator asked batten to drive.
// A found seed is authoritative and never overwritten here; a disagreement is refused, not silently adopted.
//
// A seed with another recipe would leave one run directory driven by two recipes, depending on the verb typed.
// A driver the operator typed cannot take effect on a seeded run, so it is refused rather than dropped.
// A driver left at its cobra default carries no intent to contradict, so it is ignored.
// A typed --child-driver is refused against a seed that records a different child driver or none.
// An untyped one passes a seed that records none: the child then takes its recipe's driver_kind, then shed.yaml's.
func refuseAdoptedSeed(seed shedrun.Seed, runID string, driverFlag string, driverSet bool, childDriverFlag string, childDriverSet bool) error {
	if seed.Recipe != shedrun.RecipeBatten {
		return fmt.Errorf(
			"battencli: run %q is already seeded with recipe %q, not %q; batten refuses to drive it -- drive it with \"lym shed run %s\", or delete its seed to re-seed it as a batten run",
			runID, seed.Recipe, shedrun.RecipeBatten, runID,
		)
	}
	if driverSet && driverFlag != seed.Driver {
		return fmt.Errorf(
			"battencli: run %q is already seeded with driver %q; --driver %q cannot change a seeded run's recorded driver",
			runID, seed.Driver, driverFlag,
		)
	}
	if childDriverSet {
		recorded, has := childDriverOf(seed)
		if !has {
			return fmt.Errorf(
				"battencli: run %q is already seeded with no child driver; --child-driver %q cannot be recorded on a seeded run -- delete the run's seed to re-seed it with the flag, or drop the flag to take the child recipe's driver_kind, then shed.yaml's",
				runID, childDriverFlag,
			)
		}
		if childDriverFlag != recorded {
			return fmt.Errorf(
				"battencli: run %q is already seeded with child driver %q; --child-driver %q cannot change a seeded run's recorded child driver",
				runID, recorded, childDriverFlag,
			)
		}
	}
	return nil
}

// childDriverOf reports the child driver the seed records and whether it records one.
// An absent or empty params.child_driver records none.
// The refusal and the child-driver choice both read through it, so they cannot disagree on what the seed records.
func childDriverOf(seed shedrun.Seed) (driver string, recorded bool) {
	if driver, ok := seed.Params["child_driver"]; ok && driver != "" {
		return driver, true
	}
	return "", false
}

// childDriverFrom returns the child driver that Seed-Child writes.
// It is the child_driver the prime seed records when it records one.
// Otherwise it returns the result of resolveKind, error included.
func childDriverFrom(seed shedrun.Seed, found bool, resolveKind func() (string, error)) (string, error) {
	if found {
		if driver, recorded := childDriverOf(seed); recorded {
			return driver, nil
		}
	}
	return resolveKind()
}

// childDriverKind resolves the driver of a child run from the config layers under dirs.
// It reads shed.yaml's driver_kind, else llm, regardless of the child's recipe.
func childDriverKind(dirs configengine.Dirs, _ string) (string, error) {
	shed, err := sheddriver.LoadConfig(dirs)
	if err != nil {
		return "", err
	}
	return sheddriver.ResolveDriverKind(shed, sheddriver.Override{}), nil
}

// anchorParam is the seed param that carries a beam project's anchor from its first pass to Worktree-Create.
const anchorParam = "anchor"

// armSeed gates batten's auto-seed at runID.
// A found seed that disagrees with this invocation is refused, and any other found seed is left untouched.
// A missing seed is written for "run" and "step" only.
// Every other verb refuses a missing seed with shedrun.MissingSeedMessage.
//
// The seed carries recipe "batten", never the Board task's own type.
// A seed with that type would make "lym shed status <slug>" from prime use the wrong recipe on the wrong worktree.
//
// The auto-seeded driver comes from --driver; child_driver comes from --child-driver only when it was typed.
// --driver names batten's own process, and batten has no bootstrap verb, so it refuses "llm" outright.
// --child-driver names the driver that the task worktree's bootstrap honours, not batten's own.
// Only shedrun.ValidateDriver checks it here.
// Whether the child's recipe can run that driver is checked when the child's seed is written.
//
// A typed flag is validated before the seed is read, because its verdict does not depend on the seed.
// The auto-seed path validates the value it writes, which for an untyped flag is cobra's default.
//
// Seeding happens before wiring, because wiring reads the seed and a first run has none until then.
//
// The refusal carries no "kind" field, which keeps the step refusal-kind vocabulary closed.
// A missing run is not a kind of its own.
//
// Each successful return registers the run as batten (registerSeededRun), whether it found the seed or wrote it.
// A refusal registers nothing.
func (c *battenCLI) armSeed(location *lymcwd.Location, runID, verb string) error {
	if c.driverFlagSet {
		if err := shedrun.ValidateDriver(c.driverFlag); err != nil {
			return err
		}
		if err := refuseBattenOwnDriverLLM(c.driverFlag); err != nil {
			return err
		}
	}
	if c.childDriverFlagSet {
		if err := shedrun.ValidateDriver(c.childDriverFlag); err != nil {
			return err
		}
	}

	seed, found, err := shedrun.ReadSeed(location, runID)
	if err != nil {
		return err
	}
	if found {
		if err := refuseAdoptedSeed(seed, runID, c.driverFlag, c.driverFlagSet, c.childDriverFlag, c.childDriverFlagSet); err != nil {
			return err
		}
		c.registerSeededRun(location, runID, verb)
		return nil
	}

	if !battenAutoSeedVerbs(verb) {
		existing, listErr := shedrun.List(location)
		if listErr != nil {
			return listErr
		}
		return errors.New(shedrun.MissingSeedMessage("battencli", runID, existing, `run "lym batten run <slug>" first`))
	}

	driver := battenDriver(c.driverFlag)
	if err := shedrun.ValidateDriver(driver); err != nil {
		return err
	}
	if err := refuseBattenOwnDriverLLM(driver); err != nil {
		return err
	}

	// The run-id is the slug; no param repeats it.
	// child_driver is recorded only when the operator typed it;
	// Seed-Child resolves the rest from the child recipe's driver_kind, then shed.yaml's.
	// anchor is recorded only by a first pass that names one, and Worktree-Create reads it back.
	var params map[string]string
	if c.childDriverFlagSet {
		params = map[string]string{"child_driver": c.childDriverFlag}
	}
	if c.firstPassAnchor != "" {
		if params == nil {
			params = map[string]string{}
		}
		params[anchorParam] = c.firstPassAnchor
	}
	// A cancel that began while the start was pulling the board writes no seed.
	if err := c.refuseCancelMarker(); err != nil {
		return err
	}
	if err := shedrun.WriteSeed(location, runID, shedrun.Seed{
		Recipe: shedrun.RecipeBatten,
		Driver: driver,
		Params: params,
	}); err != nil {
		return err
	}
	c.registerSeededRun(location, runID, verb)
	return nil
}

// registerSeededRun records the calling process as batten for runID in the hub's owner record, for verb "run" only.
// It runs once a seed agrees with this invocation or is written;
// a start refused before or at the seed registers nothing.
// A failed registration is logged and the run goes on.
func (c *battenCLI) registerSeededRun(location *lymcwd.Location, runID, verb string) {
	if verb != "run" {
		return
	}
	if err := runproc.Register(fabricengine.RunProcDir(location.HubPath), runID, runproc.SelfRecord(runproc.RoleBatten)); err != nil {
		logger.Warn("battencli: could not register the batten process", "slug", runID, "error", err.Error())
	}
}

// refuseBattenOwnDriverLLM refuses driver when it is shedrun.DriverLLM and BootstrapVerb is empty.
// Batten without a bootstrap verb cannot be driven by an LLM.
// BootstrapVerb is read from this package, because importing `internal/shedcli` here would create an import cycle.
func refuseBattenOwnDriverLLM(driver string) error {
	if driver != shedrun.DriverLLM {
		return nil
	}
	if BootstrapVerb != "" {
		return nil
	}
	return fmt.Errorf("battencli: batten has no bootstrap verb, so it cannot be driven by an LLM")
}

// arm resolves cwd into a *lymcwd.Location, then delegates to armAt with the run-id from args.
// The run-id is args[0] when present and counts as explicit; otherwise it is shedrun.SelfRunID and does not.
func (c *battenCLI) arm(cwd string, verb string, args []string) (shedverbs.Spec, error) {
	location, err := lymcwd.Resolve(cwd)
	if err != nil {
		// The error from lymcwd.Resolve already names the missing git repository, so it is returned bare.
		return shedverbs.Spec{}, err
	}

	// A run reconciles the hub's config after a binary change;
	// that must happen before the run is seeded and before the Worktree-Create row forks the pair from the prime.
	if verb == "run" {
		if geometry, inHub := hubgeom.ReconcileGeometry(location); inHub {
			hubreconcile.EnsureOrWarn(geometry, hubreconcile.Options{})
		}
	}

	runID, explicit := resolveBattenRunID(args)
	return c.armAt(location, runID, explicit, verb)
}

// armAt applies the prime-or-beam guard, refuses a self address, and gates the auto-seed.
// It wires the receiver over the prime's location and returns the filled Spec.
// It performs no resolution of its own, so location must already be resolved.
//
// explicit reports whether the run-id was typed rather than defaulted.
// A caller that receives its run-id already resolved always passes true.
//
// The hooks in the returned Spec close over the receiver, so the caller must keep that same receiver.
func (c *battenCLI) armAt(location *lymcwd.Location, runID string, explicit bool, verb string) (shedverbs.Spec, error) {
	// The guard admits the prime and a beam, and returns the prime's location, which every path below lies under.
	prime, refusalErr := ResolveBattenPrime(location)
	if refusalErr != nil {
		return shedverbs.Spec{}, refusalErr
	}

	if refusalErr := refuseSelfAddress(runID, explicit); refusalErr != nil {
		return shedverbs.Spec{}, refusalErr
	}
	if verb == "run" || verb == "step" || verb == "goto" {
		if refusalErr := refuseOwnPair(location, prime, runID, verb); refusalErr != nil {
			return shedverbs.Spec{}, refusalErr
		}
	}
	location = prime

	c.location = location
	c.slug = runID
	c.caller = agentname.CallerLabel(os.Getenv(agentname.StrandNameEnv))
	if c.owner.Machine == nil {
		c.owner = hubOwnerSeams(location)
	}
	if c.guard.Marker == nil {
		c.guard = hubStartGuard(location, runID)
	}
	if verb == "run" && c.readopt.Add == nil {
		c.readopt = hubReadoptSeams(location)
	}
	c.preDrive = c.preDriveCheck

	// The guard's reads are local and its refusals come before the board pull, the prime claim and every write below.
	if err := c.refuseAtStart(verb); err != nil {
		return shedverbs.Spec{}, err
	}

	// A start writes its seed below, so its owner check runs first.
	// A task another machine owns leaves no seed behind and no done motif run removed.
	if err := c.claimStart(verb); err != nil {
		return shedverbs.Spec{}, err
	}

	if err := c.armSeed(location, runID, verb); err != nil {
		return shedverbs.Spec{}, err
	}

	if err := c.wire(location, runID); err != nil {
		return shedverbs.Spec{}, err
	}

	if err := c.loadRouting(); err != nil {
		return shedverbs.Spec{}, err
	}

	return c.specFor(verb), nil
}

// loadRouting projects batten's recipe routing onto c.routing, with MaxBounces taken from c.shedPaths.
// Status then reports the bounce budget the engine enforces.
// It must run after wiring, which fills c.shedPaths, and it is the only place the routing can return an error.
func (c *battenCLI) loadRouting() error {
	routing, err := battenrecipe.Routing()
	if err != nil {
		return err
	}

	routing.MaxBounces = c.shedPaths.MaxBounces
	c.routing = routing

	return nil
}

// specFor fills a Spec from c's already-wired fields, with no resolution, no wire call and no I/O.
// Because it does no I/O, a test can call it on a hand-populated receiver without spawning git.
func (c *battenCLI) specFor(verb string) shedverbs.Spec {
	spec := shedverbs.Spec{
		StatusPath:     c.shedPaths.StatusPath,
		LockPath:       c.shedPaths.LockPath,
		StatusLockPath: c.shedPaths.StatusLockPath,

		MissingStatusWayForward: c.shedPaths.MissingStatusWayForward,
		PauseWayForward:         c.shedPaths.PauseWayForward,
		// The status file is durable, but its lock is ephemeral.
		// A machine that never stepped the slug, such as a fresh clone, has the file but not the lock's directory.
		// Creating that directory is not a write to the run.
		// It lets the read-only verbs read a status the pair carried here, which is why the status file is durable.
		EnsureStatusLockDir: true,
		StatusLabel:         "batten",
		DecodeErrPrefix:     "battencli:",
		RunBusyMessage:      fmt.Sprintf("battencli: another batten run already holds the run lock %q", c.shedPaths.LockPath),
		// StepBusyMessage uses the same wording as `RunBusyMessage`.
		// A busy detection in `shed.Step` can fire when a race lands after the pre-step probe releases the lock;
		// it reports this text rather than passing shed's own error through.
		// StepBusyKind keeps the refusal inside the closed refusal-kind vocabulary.
		StepBusyMessage: fmt.Sprintf("battencli: another batten run already holds the run lock %q", c.shedPaths.LockPath),
		StepBusyKind:    shedverbs.KindBusy,
		AbsentStatus: shedverbs.AbsentDisposition{
			// A slug that has never run on this machine is a determined answer, not an error.
			Refuse: false,
		},
		PauseAbsentMessage: fmt.Sprintf("battencli: no status file at %s; there is nothing running to pause -- run \"lym batten run <slug>\" first", c.shedPaths.StatusPath),
		Hooks: shedverbs.Hooks{
			PreRun:       c.battenPreRun,
			PreStep:      c.battenPreStep,
			StatusExtras: c.battenStatusExtras,
			PreGoto:      c.battenPreGoto,
		},
	}

	// BuildShed builds the shed with battenrecipe.New over c.env and c.shedPaths, for both "run" and "step".
	// c.wire performs no fabric-open or origin-read work that a second construction would duplicate;
	// run and step therefore share this construction.
	if verb == "run" || verb == "step" {
		// LockedCheck makes the shed's Run and Step call the pre-drive check once they hold the run lock.
		// That matters for a resume whose status was already present.
		spec.BuildShed = func() (*shedengine.Shed, error) {
			shed, err := battenrecipe.New(c.env, c.shedPaths)
			if err != nil {
				return nil, err
			}
			shed.LockedCheck = c.preDrive
			machineName := c.machineName
			if machineName == nil {
				machineName = machinename.Name
			}
			name, err := machineName()
			if err != nil {
				return nil, fmt.Errorf("battencli: this machine's name is unknown, so the run cannot record which machine it ran on: %w; way forward: give the machine a hostname, then re-run lym batten run %s", err, c.slug)
			}
			shed.Machine = name
			return shed, nil
		}
	}

	spec.Caller = c.caller
	// Batten carries no agent friction directory, so FrictionDir stays empty.
	if c.location != nil {
		spec.ScratchDir = shedrun.ScratchDir(c.location, c.slug)
		spec.StepsDir = shedrun.StepsDir(c.location, c.slug)
		spec.RunID = shedrun.ResolveRunID(c.location, c.slug)
	}
	spec.Routing = c.routing
	spec.Hooks.InterruptPolicyFor = battenInterruptPolicy
	spec.Hooks.ConsumeAttempt = c.battenConsumeAttempt
	if c.location != nil {
		spec.Loop = battenLoopSpec(c.location, c.slug)
	}

	return spec
}

// battenLoopSpec builds the loop arming of the batten run slug under location.
// It leaves IdleTimeout zero and Activity nil: Run-Shed waits idle by design, so the watchdog stays disarmed.
// A wedged step is ended only by the operator's pause or by ending the detached loop job.
// OnStart registers the loop process, and the keeper that starts it, in the hub's owner record under slug with the role loop;
// a failed registration is logged and the loop runs on.
func battenLoopSpec(location *lymcwd.Location, slug string) shedverbs.LoopSpec {
	executable, err := selfexec.LymBinary()
	if err != nil {
		logger.Warn("battencli: could not resolve the running binary, so the loop has no executable", "error", err.Error())
	}
	return shedverbs.LoopSpec{
		LockPath:     shedrun.LoopLock(location, slug),
		PIDPath:      shedrun.LoopPIDFile(location, slug),
		LogPath:      shedrun.LoopLog(location, slug),
		EnvelopePath: shedrun.LoopEnvelope(location, slug),
		JobName:      shedrun.LoopJobName(location, slug),
		Delivered:    func(loopID string) string { return shedrun.LoopDelivered(location, slug, loopID) },
		StopFiles: func(traceID string) (string, string) {
			return shedrun.StepTraceCopy(location, slug, traceID), shedrun.StepStderr(location, slug, traceID)
		},
		TraceFiles:    func(traceID string) ([]string, error) { return logger.TraceFilesFor(logger.TraceDir(), traceID) },
		LoopTraceFile: logger.TraceFile,
		Executable:    executable,
		OnStart: func() {
			if err := runproc.Register(fabricengine.RunProcDir(location.HubPath), slug, runproc.SelfRecord(runproc.RoleLoop)); err != nil {
				logger.Warn("battencli: could not register the loop process", "slug", slug, "error", err.Error())
			}
		},
	}
}

// battenInterruptPolicy returns the interrupt policy for a step that ended without an envelope at row.
// Run-Shed gets InterruptPolicyReinvoke, since its wait reattaches to the child run;
// that step is stepped again under the repair cap.
// Any other row, the empty name included, gets InterruptPolicyHandback.
func battenInterruptPolicy(row string) string {
	if row == battenrecipe.NameRunShed {
		return shedverbs.InterruptPolicyReinvoke
	}
	return shedverbs.InterruptPolicyHandback
}

// Arm resolves cwd, then returns the Spec for verb and args.
// It builds a fresh receiver for the call, and the Spec's hooks close over that receiver.
func Arm(cwd string, verb string, args []string) (shedverbs.Spec, error) {
	c := &battenCLI{}
	return c.arm(cwd, verb, args)
}

// ArmAt returns the Spec for verb, for a location the caller has already resolved and a runID.
// It builds a fresh receiver for the call and performs no lymcwd.Resolve of its own.
// The caller needs the location before it can read the seed that picks the recipe;
// resolving the location again here would cost a second `git rev-parse` on every call.
func ArmAt(location *lymcwd.Location, verb string, runID string) (shedverbs.Spec, error) {
	c := &battenCLI{}
	return c.armAt(location, runID, true, verb)
}

// checkRunStatus is the read-only pre-flight check of the `run` verb.
// It decodes the status file, refuses a done slug, and resumes silently from every other known state.
// It reports found false for an absent status file and never seeds one.
// A decode failure, an unrecognized state and a done slug come back as errors.
func (c *battenCLI) checkRunStatus() (found bool, err error) {
	st, found, err := state.ReadJSON[shedengine.Status](c.shedPaths.StatusPath, c.shedPaths.StatusLockPath)
	if err != nil {
		return false, errors.New("battencli: decode status file " + c.shedPaths.StatusPath + ": " + err.Error())
	}
	if !found {
		return false, nil
	}
	switch st.State {
	case shedengine.StateDone:
		return true, doneSlugRefusal(c.location, c.slug)
	case shedengine.StateRunning, shedengine.StateBlocked, shedengine.StateAwaiting, shedengine.StateFailed, shedengine.StatePaused:
		// Each of these resumes silently from the persisted current producer, with no re-seed, no prompt and no flag.
		// The engine already resumes from blocked and failed.
		// StateBlocked is the everyday path, since every operator-fixable refusal lands there.
		return true, nil
	default:
		return true, fmt.Errorf("battencli: unrecognized status state %q", st.State)
	}
}

// coveredRefusal is the refusal for a run that a live pause covers.
// A pass exits with its own code on it.
type coveredRefusal struct {
	err error
}

// Error implements error by returning the wrapped refusal's text unchanged.
func (r *coveredRefusal) Error() string { return r.err.Error() }

// Unwrap exposes the wrapped refusal to errors.Is and errors.As.
func (r *coveredRefusal) Unwrap() error { return r.err }

// continueRefusal is the refusal of a continue pass for a run that dobby may not continue.
// A pass exits with its own code on this refusal, and dobby reads that code as a full pass owed.
type continueRefusal struct {
	// reason says which condition of a continue pass the run does not meet.
	reason string
	// slug is the run, for the way forward.
	slug string
}

// Error implements error; its text names the unmet condition and the full pass that settles it.
func (r *continueRefusal) Error() string {
	return fmt.Sprintf("battencli: %q is not a run a continue pass may continue: %s; way forward: run `lym batten run %s`", r.slug, r.reason, r.slug)
}

// refuseContinuePass refuses a continue pass unless dobby may continue the run.
// Batten's status must read running at Run-Shed, and the task worktree must be present.
// The dobby state record of the parent worktree must name the run as driven, with no full pass owed.
// No origin-owed marker may stand.
// A gone task worktree owes a full pass, whose resume re-adopts a live pair.
// The dobby state record is read through the parent worktree's geometry.
// A record that is absent, or does not name the run, reads as a run that dobby does not drive.
func (c *battenCLI) refuseContinuePass() error {
	refuse := func(reason string) error { return &continueRefusal{reason: reason, slug: c.slug} }

	status, found, err := state.ReadJSON[shedengine.Status](c.shedPaths.StatusPath, c.shedPaths.StatusLockPath)
	if err != nil {
		return errors.New("battencli: decode status file " + c.shedPaths.StatusPath + ": " + err.Error())
	}
	if !found {
		return refuse("it has no status file")
	}
	if status.State != shedengine.StateRunning || status.CurrentProducer != battenrecipe.NameRunShed {
		return refuse(fmt.Sprintf("its status reads %s at %s, not running at %s", status.State, status.CurrentProducer, battenrecipe.NameRunShed))
	}
	if c.readopt.Gone != nil {
		gone, err := c.readopt.Gone(c.slug)
		if err != nil {
			return refuse(fmt.Sprintf("whether its task worktree is gone does not read: %v", err))
		}
		if gone {
			return refuse("its task worktree is gone")
		}
	}

	parent, err := parentWorktree(c.location, c.slug)
	if err != nil {
		return refuse(fmt.Sprintf("its parent worktree does not resolve: %v", err))
	}
	geometry, err := hubgeom.DobbyGeometry(c.location, parent)
	if err != nil {
		return refuse(fmt.Sprintf("its parent's dobby geometry does not resolve: %v", err))
	}
	dobbyState, _, err := dobbyengine.ReadState(geometry.StatePath)
	if err != nil {
		return refuse(fmt.Sprintf("its parent's dobby state does not read: %v", err))
	}
	run, driven := dobbyState.Runs[c.slug]
	if !driven {
		return refuse("its parent's dobby does not name it as a run it drives")
	}
	if run.FullPassOwed {
		return refuse("its parent's dobby owes it a full pass")
	}

	owed, err := OriginOwed(c.location, c.slug)
	if err != nil {
		return refuse(fmt.Sprintf("its origin-owed marker does not read: %v", err))
	}
	if owed {
		return refuse("a resume judged it without origin and origin has not been reached since")
	}
	return nil
}

// refuseCovered returns the refusal naming "lym tender resume" when a live pause entry covers this run's slug;
// it returns nil otherwise.
// A geometry or record that cannot be read refuses nothing and logs one Warn, so a corrupt file never blocks a run.
func (c *battenCLI) refuseCovered() error {
	geometry, err := hubgeom.PauseGeometry(c.location)
	if err != nil {
		logger.Warn("battencli: pause geometry unreadable, running despite a possible pause", "slug", c.slug, "cause", err)
		return nil
	}
	coverage, err := pauserecord.ReadCoverage(geometry.RecordPath, c.slug, time.Now())
	if err != nil {
		logger.Warn("battencli: pause record unreadable, running despite a possible pause", "slug", c.slug, "cause", err)
		return nil
	}
	if !coverage.Covered {
		return nil
	}
	return &coveredRefusal{err: pauserecord.CoveredError(coverage.Entry, c.slug, fmt.Sprintf(`run "lym batten run %s" again`, c.slug))}
}

// pausedExtras returns the status keys a live pause entry covering this run's slug adds:
// paused_since (the entry's time, RFC 3339) and paused_by (its scope).
// An uncovered run, a receiver without a location and an unreadable record add nothing;
// an unreadable geometry or record logs one Warn, so a status read never fails on the record.
func (c *battenCLI) pausedExtras() map[string]any {
	if c.location == nil {
		return nil
	}
	geometry, err := hubgeom.PauseGeometry(c.location)
	if err != nil {
		logger.Warn("battencli: pause geometry unreadable, status omits paused_since", "slug", c.slug, "cause", err)
		return nil
	}
	coverage, err := pauserecord.ReadCoverage(geometry.RecordPath, c.slug, time.Now())
	if err != nil {
		logger.Warn("battencli: pause record unreadable, status omits paused_since", "slug", c.slug, "cause", err)
		return nil
	}
	if !coverage.Covered {
		return nil
	}
	return map[string]any{
		"paused_since": coverage.Entry.PausedAt.Format(time.RFC3339),
		"paused_by":    coverage.Entry.Scope,
	}
}

// battenPreRun implements the PreRun hook for batten's spec:
// the pause or continue-pass refusal, then checkRunStatus, then seeding a fresh status when absent.
// Each step's error becomes a returned error, which the generic body reports on the error envelope.
// A found status file resumes only after the owner pre-flight (ownerPreflight) lets it;
// that pre-flight is the run-scope origin reconcile (reconcileResume) for every run but these:
// a run still at Worktree-Create, or at Worktree-Teardown whose pair is absent.
// A fresh start runs no reconcile, and armAt has already run its owner check.
// Once the owner pre-flight lets a found status resume, reopenWindow reopens the terminal window;
// it does so when no client is attached, unless --no-reopen is set.
// Under --continue-pass, refuseContinuePass runs first,
// and a run it lets through skips the pause refusal, the owner pre-flight and the reopening:
// the run is one dobby already drives, and the pass is not a resume.
//
// This seed-when-absent behaviour is batten's alone and must stay out of the generic body:
// loom refuses in exactly the situation batten seeds here, because only "lym loom start" may seed loom's status file.
func (c *battenCLI) battenPreRun(ctx context.Context) error {
	if c.continuePass {
		if err := c.refuseContinuePass(); err != nil {
			return err
		}
	} else if err := c.refuseCovered(); err != nil {
		return err
	}
	found, err := c.checkRunStatus()
	if err != nil {
		return err
	}
	if found {
		if c.continuePass {
			return nil
		}
		if err := c.preflightRunOwner(); err != nil {
			return err
		}
		if c.noReopen {
			return nil
		}
		status, _, err := state.ReadJSON[shedengine.Status](c.shedPaths.StatusPath, c.shedPaths.StatusLockPath)
		if err != nil {
			return errors.New("battencli: decode status file " + c.shedPaths.StatusPath + ": " + err.Error())
		}
		return c.reopenWindow(ctx, status)
	}

	// An absent status file is a fresh start:
	// shedengine.Shed.Run refuses to walk from a status file that does not exist yet (it never seeds one itself),
	// so this hook seeds it here, at the entry row, before the Shed ever reads it.
	// The mutate closure is idempotent against a concurrently-seeded file:
	// it leaves an already-present status untouched rather than overwriting it.
	// The pre-drive check and the marker clear run inside it, under the status lock,
	// so a refused seeding writes no status and clears no marker.
	return state.UpdateJSON(c.shedPaths.StatusPath, c.shedPaths.StatusLockPath, c.seedAbsentStatus)
}

// preDriveRefusal marks the refusal of the pre-drive check inside seedAbsentStatus,
// so a caller can tell it from a failed marker clear or write.
type preDriveRefusal struct{ error }

// Unwrap returns the check's refusal.
func (e preDriveRefusal) Unwrap() error { return e.error }

// seedAbsentStatus is the mutate closure of the status seeding: a found status stays as it is;
// otherwise the pre-drive check runs, the run markers are cleared, and the Worktree-Create running status is returned.
// A refusal of the check comes back as a preDriveRefusal with nothing cleared and nothing written.
func (c *battenCLI) seedAbsentStatus(cur shedengine.Status, found bool) (shedengine.Status, error) {
	if found {
		return cur, nil
	}
	if c.preDrive != nil {
		if err := c.preDrive(); err != nil {
			return cur, preDriveRefusal{err}
		}
	}
	if err := battenshed.ClearRunMarkers(c.env.ScratchDir); err != nil {
		return cur, err
	}
	return shedengine.Status{
		CurrentProducer: battenrecipe.NameWorktreeCreate,
		State:           shedengine.StateRunning,
		History:         []shedengine.HistoryEntry{},
	}, nil
}

// preDriveCheck re-reads the cancel marker and the seed:
// a run that passed the arm-time checks before a cancel began must still be refused.
// A present marker refuses with the cancel refusal,
// and an absent seed refuses naming the fresh start, since the run was cancelled or never started.
// It writes nothing.
func (c *battenCLI) preDriveCheck() error {
	if err := c.refuseCancelMarker(); err != nil {
		return err
	}
	_, found, err := shedrun.ReadSeed(c.location, c.slug)
	if err != nil {
		return err
	}
	if !found {
		return fmt.Errorf("battencli: %s has no seed, so its run was cancelled or never started; way forward: run %s and \"lym batten run %s\" for a fresh start", c.slug, releaseCommand(c.slug), c.slug)
	}
	return nil
}

// openWindowTimeout is the time the reopened Open-Window command may run, the default of any Command row.
const openWindowTimeout = 30 * time.Second

// reopenWindow runs the Open-Window row's command once for a resumed run past the row.
// It does so only when no client is attached to the task worktree's reed session:
// the window the first run opened is gone with that client.
// It opens nothing when batten.open_window is off, read through Env.Setting as the row's entry gate reads it;
// a forced run and a task's open_window: false apply as in the row.
// Otherwise it runs the batten.open_window_command command through a command producer.
// A status whose history holds no Open-Window entry opens nothing;
// so does one whose current producer is still at or before the row, and so does an attached client.
// A failed client check is warned about and opens nothing;
// the command's own failure is returned as an error.
func (c *battenCLI) reopenWindow(ctx context.Context, status shedengine.Status) error {
	if c.clientAttached == nil || !pastOpenWindow(status) {
		return nil
	}
	attached, err := c.clientAttached()
	if err != nil {
		logger.Warn("battencli: cannot tell whether a client is attached, not reopening the window", "slug", c.slug, "cause", err)
		return nil
	}
	if attached {
		return nil
	}
	value, err := c.env.Setting("batten", "open_window")
	if err != nil {
		return fmt.Errorf("battencli: reopen the window: read batten.open_window: %w; way forward: correct it, then %s", err, c.env.Resume)
	}
	enabled, isBool := value.(bool)
	if !isBool {
		return fmt.Errorf("battencli: reopen the window: batten.open_window holds %v, not a bool; way forward: correct it, then %s", value, c.env.Resume)
	}
	if !enabled {
		return nil
	}
	deps := c.env.Command
	deps.Setting = c.env.Setting
	deps.ExecutableSetting = c.env.ExecutableSetting
	row := shedadapters.CommandRow{
		CommandModule: "batten",
		CommandKey:    "open_window_command",
		Timeout:       openWindowTimeout,
	}
	if _, _, err := shedadapters.NewCommandProducer(battenrecipe.NameOpenWindow, row, deps).Call(ctx); err != nil {
		return fmt.Errorf("battencli: reopen the window: %w", err)
	}
	return nil
}

// pastOpenWindow reports whether status is past Open-Window.
// It holds an Open-Window history entry, and its current producer is a row after Open-Window.
func pastOpenWindow(status shedengine.Status) bool {
	switch status.CurrentProducer {
	case battenrecipe.NameWorktreeCreate, battenrecipe.NameSeedChild, battenrecipe.NameOpenWindow:
		return false
	}
	for _, entry := range status.History {
		if entry.Producer == battenrecipe.NameOpenWindow {
			return true
		}
	}
	return false
}

// doneSlugRefusal is the refusal of every start of a slug whose run has already completed.
// It names the whole abandon path, not the run directory alone:
// a torn-down pair keeps its task branch locally and on the remote.
// So deleting only the run directory leads straight into the create row's leftover-branch refusal on the re-run.
// It also names the other way forward: giving the new task a different slug.
func doneSlugRefusal(location *lymcwd.Location, slug string) error {
	return fmt.Errorf(
		"battencli: %q has already completed; to run it again, run \"lym batten cleanup --apply\" from the prime, which removes its run directory %s, and delete the task branch its torn-down pair left behind, locally and on the remote (\"lym fabric cleanup --apply --remote\" removes any orphaned sibling branch), or give the new task a different slug",
		slug, shedrun.RunDir(location, slug),
	)
}

// battenPreStep implements the PreStep hook for batten's spec.
// It probes the run lock first, then decodes the status:
// a done slug is refused, a known state other than done resumes silently, and an absent status is seeded.
//
// Without this hook, stepLocked's read gate would hit an absent status and hard-error.
// shedverbs/step.go reports that as kind: "producer", the one kind the loom driver retries,
// so a fresh slug's first step would loop rather than seed.
//
// Its refusal-kind mapping stays inside the closed set of shedverbs kinds.
// The run lock already held is shedverbs.KindBusy, and so is a seeding the pre-drive check refuses:
// the run is held by a cancel or no longer seeded, and the driver leaves it to its holder.
// A status decode failure, a failed marker clear or a failed status seed is shedverbs.KindUnseeded.
// Any other pre-producer failure is shedverbs.KindBootstrap:
// the unrecognized-state default and the done-slug refusal alike.
// shedverbs.KindOwnership is not used, since it reports a seeded status that belongs to another slug.
// Batten has no such check.
// shedverbs.KindProducer is shed.Step's own to emit, never this hook's.
//
// Every returned error passes through shedtransient.Mark, the bootstrap boundary of the Transient Stop Invariant.
// Only the owner pre-flight reaches a remote,
// so the mark is inert for every other sub-step.
func (c *battenCLI) battenPreStep(ctx context.Context) (string, error) {
	kind, err := c.battenPreStepUnmarked(ctx)
	return kind, shedtransient.Mark(err)
}

// battenPreStepUnmarked runs the pre-step work and returns its refusal kind with the error.
// The caller applies the transient mark.
func (c *battenCLI) battenPreStepUnmarked(ctx context.Context) (string, error) {
	if err := os.MkdirAll(filepath.Dir(c.shedPaths.LockPath), 0o755); err != nil {
		return shedverbs.KindBootstrap, err
	}
	probe, runLockFree, err := lock.TryAcquireWriteLock(c.shedPaths.LockPath)
	if err != nil {
		return shedverbs.KindBootstrap, err
	}
	if runLockFree {
		_ = probe.Release()
	} else {
		return shedverbs.KindBusy, fmt.Errorf("battencli: another batten run already holds the run lock %q", c.shedPaths.LockPath)
	}

	st, found, err := state.ReadJSON[shedengine.Status](c.shedPaths.StatusPath, c.shedPaths.StatusLockPath)
	if err != nil {
		return shedverbs.KindUnseeded, errors.New("battencli: decode status file " + c.shedPaths.StatusPath + ": " + err.Error())
	}
	if found {
		switch st.State {
		case shedengine.StateDone:
			return shedverbs.KindBootstrap, doneSlugRefusal(c.location, c.slug)
		case shedengine.StateRunning, shedengine.StateBlocked, shedengine.StateAwaiting, shedengine.StateFailed, shedengine.StatePaused:
			// These states resume silently; the owner pre-flight below judges them.
		default:
			return shedverbs.KindBootstrap, fmt.Errorf("battencli: unrecognized status state %q", st.State)
		}
		if err := c.ownerPreflight(true, st); err != nil {
			return shedverbs.KindBootstrap, err
		}
		return "", nil
	}

	if err := state.UpdateJSON(c.shedPaths.StatusPath, c.shedPaths.StatusLockPath, c.seedAbsentStatus); err != nil {
		var refused preDriveRefusal
		if errors.As(err, &refused) {
			return shedverbs.KindBusy, refused.error
		}
		return shedverbs.KindUnseeded, err
	}

	return "", nil
}

// maxStatusHistoryEntries caps the history entries the status envelope carries.
// History grows only when Run-Shed's wait returns:
// on a state change, a pause or a failed status stat, each its own entry.
// The cap still keeps a long run from burying the two ends that carry the run's meaning.
const maxStatusHistoryEntries = 20

// battenStatusExtras implements the StatusExtras hook for batten's spec:
// it returns batten's own found-envelope keys and no others.
// The generic body supplies current_producer, state, error and activity, plus history_length.
//
// History is bounded to the most recent maxStatusHistoryEntries entries, always alongside the true history_length,
// so a bounded view is never mistaken for a short one.
// A blocked run also carries the producer's own recorded stuck reason:
// Worktree-Create, Seed-Child and Worktree-Teardown carry no on_stuck, so their reason is the persisted error;
// this extra is kept for the Run-Shed block through the budget arm, whose error stays the fixed budget literal.
// A batten run stopped before its teardown while its child tapestry run is done also carries childDoneFlagKey:
// the flag names the step that closes it.
// A run a live pause covers also carries paused_since and paused_by.
// The slug's effective owner on the local Bolt rides along as owner_machine and owned_here;
// an unreadable board adds neither.
func (c *battenCLI) battenStatusExtras(st shedengine.Status) (map[string]any, error) {
	history, truncated := recentHistory(st.History)
	extras := map[string]any{
		"found":             true,
		"status_path":       c.shedPaths.StatusPath,
		"history":           history,
		"history_truncated": truncated,
	}
	scratchDir := BattenDir(c.location, c.slug)
	if st.State == shedengine.StateBlocked {
		if reason, found := readStuckReason(scratchDir, st.CurrentProducer); found {
			extras["stuck_reason"] = reason
		}
	}
	// Reported in every state: the operator usually asks after the run has finished;
	// the asking process is usually not the one that observed the shutdown.
	if session, found := readAbandonedSession(scratchDir); found {
		extras["abandonedSession"] = session
	}
	child, childFound := c.readChildStatus()
	if flag, flagged := haltedOverDoneChild(c.slug, st, child, childFound); flagged {
		extras[childDoneFlagKey] = flag
	}
	for key, value := range c.pausedExtras() {
		extras[key] = value
	}
	for key, value := range c.ownerExtras() {
		extras[key] = value
	}
	return extras, nil
}

// childDoneFlagKey is the status envelope key that flags a batten run stopped before its teardown:
// its child tapestry run is done.
const childDoneFlagKey = "child_done_batten_halted"

// haltedOverDoneChild returns the flag text and true when batten's own state st is failed, blocked or paused.
// It also requires that the child tapestry run status was found and is done.
// The flag names the batten run that closes the pair:
// resuming batten at the row it halted at lands the run through Land before Worktree-Teardown tears the pair down.
func haltedOverDoneChild(slug string, st, child shedengine.Status, childFound bool) (string, bool) {
	switch st.State {
	case shedengine.StateFailed, shedengine.StateBlocked, shedengine.StatePaused:
	default:
		return "", false
	}
	if !childFound || child.State != shedengine.StateDone {
		return "", false
	}
	return fmt.Sprintf("batten is %s while its child tapestry run is done; run \"lym batten run %s\" from the prime, which resumes batten at the row it halted at and lands the run through Land before Worktree-Teardown tears the pair down", st.State, slug), true
}

// readChildStatus reads the task worktree's own tapestry run status.
// Every failure to read it, an absent pair or status file included, answers found false;
// failing to read the child must never change batten's own status answer.
func (c *battenCLI) readChildStatus() (shedengine.Status, bool) {
	task, err := taskWorktreeLocation(c.location, c.slug)
	if err != nil {
		return shedengine.Status{}, false
	}
	child, found, err := state.ReadJSON[shedengine.Status](shedrun.StatusFile(task, shedrun.SelfRunID), shedrun.StatusLock(task, shedrun.SelfRunID))
	if err != nil || !found {
		return shedengine.Status{}, false
	}
	return child, true
}

// readAbandonedSession reads back the record that battenshed's teardown row writes when shutdown abandons a session.
// It reports found == false when there is none.
// The record outlives the process that observed the shutdown, so a later read sees the same fact.
func readAbandonedSession(scratchDir string) (string, bool) {
	if scratchDir == "" {
		return "", false
	}
	data, err := os.ReadFile(battenshed.AbandonedSessionFile(scratchDir))
	if err != nil {
		return "", false
	}
	session := strings.TrimSpace(string(data))
	if session == "" {
		return "", false
	}
	return session, true
}

// recentHistory returns the most recent `maxStatusHistoryEntries` entries of history.
// It reports whether anything was dropped.
// A nil or short history is returned as-is with truncated == false.
func recentHistory(history []shedengine.HistoryEntry) (recent []shedengine.HistoryEntry, truncated bool) {
	if len(history) <= maxStatusHistoryEntries {
		return history, false
	}
	return history[len(history)-maxStatusHistoryEntries:], true
}

// readStuckReason reads the one-line stuck-reason file that battenshed writes for producer under scratchDir.
// It reports found == false when there is none.
// A missing or unreadable file is simply not reported:
// failing to find why something is stuck must never change the answer to whether it is stuck.
func readStuckReason(scratchDir, producer string) (string, bool) {
	if scratchDir == "" || producer == "" {
		return "", false
	}
	data, err := os.ReadFile(battenshed.StuckReasonFile(scratchDir, producer))
	if err != nil {
		return "", false
	}
	reason := strings.TrimSpace(string(data))
	if reason == "" {
		return "", false
	}
	return reason, true
}
