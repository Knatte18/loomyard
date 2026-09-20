// arm.go implements battencli's two exported resolution entry points -- Arm, which resolves cwd
// itself, and ArmAt, which takes an already-resolved *lyxcwd.Location and a resolved run-id and
// performs no lyxcwd.Resolve of its own -- plus the three-function split behind them: the
// unexported worker arm resolves cwd and delegates to armAt, armAt applies the non-prime refusal,
// refuses a self address, gates the auto-seed, and wires the whole engine stack exactly as
// resolvePersistentPreRun (cli.go) always has, and the resolution-free specFor fills a Spec from an
// already-wired receiver. Arm stays the single cwd-resolution entry point for batten's own subtree
// -- that rule survives unchanged there -- but it no longer describes the "lyx shed" path, which
// enters through ArmAt with the Location and run-id already in hand. See the overview's
// exported-Arm-is-the-single-resolution-entry-point and spec-fill-is-separable-from-resolution
// Shared Decisions.
//
// Arm carries no command-name guard of its own: the existing cmd.Name() == "batten" short-
// circuit stays in resolvePersistentPreRun, where it lets a bare group listing run without a git
// repository -- a bare "lyx shed" listing is shedcli's own equivalent guard, not this module's.
//
// arm's order is exactly: lyxcwd.Resolve, the non-prime refusal, run-id resolution from args, a
// self-address refusal, the verb branch (the gated auto-seed), then wire. The auto-seed runs here,
// ahead of wire, never inside battenPreRun: battenPreRun is a shedverbs.Spec hook called inside
// run's RunE, which is after arm has already called wire and built the whole Env, so seeding there
// would leave anything reading the seed at wiring time looking at a seed that does not exist yet on
// a first "lyx batten run <slug>".

package battencli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/Knatte18/loomyard/internal/battenrecipe"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/lock"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shedrun"
	"github.com/Knatte18/loomyard/internal/shedverbs"
	"github.com/Knatte18/loomyard/internal/state"
)

// resolveBattenRunID resolves the run-id verb addresses -- args[0] when present, shedrun.SelfRunID
// otherwise -- reporting explicit == true only when the operator actually supplied a positional
// argument.
//
// The explicit flag matters to refuseSelfAddress below: an omitted argument and an
// explicitly-typed "self" both resolve to the identical run-id value, but they are different
// operator mistakes and get different refusal text -- an omitted argument names the missing slug,
// while an explicit "self" names the reservation collision.
func resolveBattenRunID(args []string) (runID string, explicit bool) {
	if len(args) > 0 {
		return args[0], true
	}
	return shedrun.SelfRunID, false
}

// refuseSelfAddress refuses a self address outright, ahead of the auto-seed gate: prime hosts many
// slug-addressed batten runs, so "self" -- "this worktree's own primary run" -- has no meaning
// there.
//
// Two distinct cases reach here, worded differently. An omitted positional argument is the
// operator's mistake being an omitted slug argument, not a wrong one: without this refusal an
// argument-less "lyx batten run" would take the self default, fall through the gate, and write
// prime a seed with an empty params.slug. An explicitly-typed slug of "self" is a different
// mistake -- it collides with shedrun.SelfRunID's own reserved meaning -- caught here via
// shedrun.IsReserved because this is the one site a batten run-id derives from a Board slug;
// IsReserved does not catch the omitted-argument case on its own, since that check is scoped to
// run-ids derived from a Board slug and the omitted-argument default never named one.
func refuseSelfAddress(runID string, explicit bool) error {
	if !explicit {
		return fmt.Errorf(
			"battencli: no slug given; batten addresses a task worktree by slug, and prime has no run of its own to default to -- pass the slug: \"lyx batten run <slug>\"",
		)
	}
	if shedrun.IsReserved(runID) {
		return fmt.Errorf("battencli: slug %q is reserved for addressing prime's own run and cannot name a task", runID)
	}
	return nil
}

// battenAutoSeedVerbs is the set of verbs that seed a fresh run when no seed exists yet at the
// addressed run-id. Every other verb requires one and refuses instead: read-only verbs must not
// create state as a side effect of being asked a question, the same reasoning
// specFor's EnsureStatusLockDir: false already encodes for batten's status verb.
func battenAutoSeedVerbs(verb string) bool {
	switch verb {
	case "run", "step":
		return true
	default:
		return false
	}
}

// battenDriver returns flagVal, defaulting to shedrun.DriverGo when flagVal is empty -- the shape
// both --driver and --child-driver take when arm_seed_test.go drives armSeed directly against a
// zero-value receiver, and the shape "lyx batten run"/"lyx batten step" take once cli.go's own
// StringVar default ("go") has already filled the field.
func battenDriver(flagVal string) string {
	if flagVal == "" {
		return shedrun.DriverGo
	}
	return flagVal
}

// armSeed gates batten's auto-seed: it reads the seed at runID, does nothing when one already
// exists, writes a fresh one for "run" and "step" when absent, and refuses every other verb with
// shedrun.MissingSeedMessage naming "lyx batten run <slug>" as the remedy.
//
// Prime's seed carries recipe: "batten", never the Board task's own type: the two are different
// runs' recipes, and a prime seed carrying "loom" would make "lyx shed status <slug>" from prime arm
// loomcli against prime -- the wrong recipe against the wrong worktree. The child worktree's own
// recipe is chosen later, by battenshed's Seed-Child producer, from the Board task's type.
//
// The auto-seeded driver and child_driver come from c.driverFlag/c.childDriverFlag -- "run"'s and
// "step"'s own --driver/--child-driver flags -- but the two are validated differently, because they
// answer different questions. Batten's own driver flag names the process the operator typed, and
// batten has no bootstrap verb, so an "llm" value there is refused outright by
// refuseBattenOwnDriverLLM, worded as a statement about batten's own recipe rather than a pointer to
// a roadmap item. The child-driver flag names the driver the task worktree's own bootstrap will
// honour, which is a fact about that worktree's recipe, not batten's; it is validated through
// shedrun.ValidateDriver alone and nothing further -- the child's own recipe capability is checked
// when that child's seed is written (internal/shedcli's writeSeed), not here.
//
// The refusal carries no "kind" field, keeping the five-value step refusal-kind vocabulary closed:
// a missing run is not a sixth kind.
func (c *battenCLI) armSeed(location *lyxcwd.Location, runID, verb string) error {
	_, found, err := shedrun.ReadSeed(location, runID)
	if err != nil {
		return err
	}
	if found {
		return nil
	}

	if !battenAutoSeedVerbs(verb) {
		existing, listErr := shedrun.List(location)
		if listErr != nil {
			return listErr
		}
		return errors.New(shedrun.MissingSeedMessage("battencli", runID, existing, `run "lyx batten run <slug>" first`))
	}

	driver := battenDriver(c.driverFlag)
	if err := shedrun.ValidateDriver(driver); err != nil {
		return err
	}
	if err := refuseBattenOwnDriverLLM(driver); err != nil {
		return err
	}
	childDriver := battenDriver(c.childDriverFlag)
	if err := shedrun.ValidateDriver(childDriver); err != nil {
		return err
	}

	return shedrun.WriteSeed(location, runID, shedrun.Seed{
		Recipe: shedrun.RecipeBatten,
		Driver: driver,
		Params: map[string]string{
			"slug":         runID,
			"child_driver": childDriver,
		},
	})
}

// refuseBattenOwnDriverLLM refuses driver when it is shedrun.DriverLLM, reading BootstrapVerb
// directly from this package's own constant rather than reaching for the shed CLI's table: battencli
// cannot import internal/shedcli without an import cycle, which is the reason the bootstrap-verb
// capability is declared per module in the first place. The message is a statement about the
// recipe -- batten has no bootstrap verb, so it cannot be driven by an LLM -- never a pointer to a
// roadmap item.
func refuseBattenOwnDriverLLM(driver string) error {
	if driver != shedrun.DriverLLM {
		return nil
	}
	if BootstrapVerb != "" {
		return nil
	}
	return fmt.Errorf("battencli: batten has no bootstrap verb, so it cannot be driven by an LLM")
}

// arm resolves cwd into a *lyxcwd.Location, resolves the addressed run-id and whether it was
// explicitly typed -- args[0] when present, shedrun.SelfRunID and explicit == false otherwise --
// and delegates to armAt. It is batten's own subtree entry point: every "lyx batten <verb>"
// invocation reaches Arm, never ArmAt directly, so its own resolvePersistentPreRun keeps resolving
// cwd exactly once per invocation as it always has.
func (c *battenCLI) arm(cwd string, verb string, args []string) (shedverbs.Spec, error) {
	location, err := lyxcwd.Resolve(cwd)
	if err != nil {
		// lyxcwd.Resolve's error is already self-describing (it IS the "not a git repository"
		// sentinel); pass it through bare rather than doubling that same text on top of it.
		return shedverbs.Spec{}, err
	}

	runID, explicit := resolveBattenRunID(args)
	return c.armAt(location, runID, explicit, verb)
}

// armAt applies the non-prime refusal, refuses a self address, gates the auto-seed, wires the
// receiver, and returns the filled Spec. It performs no lyxcwd.Resolve of its own: location is
// already resolved, told rather than derived, by whichever caller holds it.
//
// explicit carries resolveBattenRunID's own distinction through to refuseSelfAddress: it is always
// true when called through the exported ArmAt, since that entry point receives an already-resolved
// runID rather than raw positional args and so cannot itself tell an omitted argument from an
// explicitly-typed "self" -- both collapse to the same runID value before ArmAt ever sees it. The
// "no slug given" wording is therefore reachable only through batten's own subtree (Arm); the "lyx
// shed" path's identical mistake reads the reserved-collision message instead, which still refuses,
// just with the other of the two existing texts.
//
// It is the single worker both arm (batten's own subtree, which resolves cwd first) and the
// exported ArmAt wrapper delegate to, so the *battenCLI whose fields the returned hooks close over
// is always the same value the caller holds: the pre-run's own c on the "lyx batten" path, the
// wrapper's freshly-constructed one on the "lyx shed" path.
func (c *battenCLI) armAt(location *lyxcwd.Location, runID string, explicit bool, verb string) (shedverbs.Spec, error) {
	primeName, primeNameErr := fabricengine.PrimeName(location)
	if refusalErr := refuseNonPrime(location.WorktreeName, primeName, primeNameErr); refusalErr != nil {
		return shedverbs.Spec{}, refusalErr
	}

	if refusalErr := refuseSelfAddress(runID, explicit); refusalErr != nil {
		return shedverbs.Spec{}, refusalErr
	}

	c.location = location
	c.slug = runID

	if err := c.armSeed(location, runID, verb); err != nil {
		return shedverbs.Spec{}, err
	}

	if err := c.wire(location, runID); err != nil {
		return shedverbs.Spec{}, err
	}

	return c.specFor(verb), nil
}

// specFor fills a Spec from c's already-wired fields, performing no resolution, no wire call, and
// no I/O of its own. internal/battencli's own untagged run_test.go and cli_test.go call this
// directly against a hand-populated receiver, so their leaf-command tests never spawn git.
func (c *battenCLI) specFor(verb string) shedverbs.Spec {
	spec := shedverbs.Spec{
		StatusPath:     c.shedPaths.StatusPath,
		LockPath:       c.shedPaths.LockPath,
		StatusLockPath: c.shedPaths.StatusLockPath,
		// batten's status and pause are both false here: status is read-only, so creating its
		// per-slug directory as a side effect of reading it would be a new, unasked-for write.
		EnsureStatusLockDir: false,
		StatusLabel:         "batten",
		DecodeErrPrefix:     "battencli:",
		RunBusyMessage:      fmt.Sprintf("battencli: another batten run already holds the run lock %q", c.shedPaths.LockPath),
		// StepBusyMessage mirrors RunBusyMessage's own wording: shed.Step's own busy detection (a
		// race landing after battenPreStep's early probe releases the lock) reports through this
		// told text rather than passthrough, and StepBusyKind stays inside the closed five-value
		// vocabulary.
		StepBusyMessage: fmt.Sprintf("battencli: another batten run already holds the run lock %q", c.shedPaths.LockPath),
		StepBusyKind:    shedverbs.KindBusy,
		AbsentStatus: shedverbs.AbsentDisposition{
			// A slug that has never run on this machine is a determined answer, not an error.
			Refuse: false,
		},
		PauseAbsentMessage: fmt.Sprintf("battencli: no status file at %s; there is nothing running to pause -- run \"lyx batten run <slug>\" first", c.shedPaths.StatusPath),
		Hooks: shedverbs.Hooks{
			PreRun:       c.battenPreRun,
			PostRun:      c.battenPostRun,
			PreStep:      c.battenPreStep,
			StatusExtras: c.battenStatusExtras,
		},
	}

	// BuildShed is batten's plain battenrecipe.New(c.env, c.shedPaths) for both "run" and "step":
	// unlike loom's own step arm (buildLoomShed), batten's wire performs no fabric-open/origin-read
	// work that a second construction would duplicate, so there is no reason for step to take a
	// different path than run does.
	if verb == "run" || verb == "step" {
		spec.BuildShed = func() (*shedengine.Shed, error) { return battenrecipe.New(c.env, c.shedPaths) }
	}

	return spec
}

// Arm is battencli's exported resolution entry point, for batten's own subtree: it constructs a
// fresh receiver and returns c.arm(cwd, verb, args), resolving cwd itself. A package-level Arm
// alone could not serve resolvePersistentPreRun, because wire is a method on the receiver and the
// hooks close over c.env, c.shedPaths and c.abandonedSession, and the pre-run's own c.location and
// c.slug assignments would stop happening.
func Arm(cwd string, verb string, args []string) (shedverbs.Spec, error) {
	c := &battenCLI{}
	return c.arm(cwd, verb, args)
}

// ArmAt is battencli's Location-taking resolution entry point, for internal/shedcli's "lyx shed"
// table: it constructs a fresh receiver and returns c.armAt(location, runID, true, verb),
// performing no lyxcwd.Resolve of its own. It exists because shedcli's own pre-run must read a seed
// before it knows which recipe to arm, a seed read is a path read, a path read needs an anchor, and
// resolving one twice per invocation to preserve Arm's own single signature would cost a second
// "git rev-parse" on every call. explicit is always true here; see armAt's own doc comment for why.
func ArmAt(location *lyxcwd.Location, verb string, runID string) (shedverbs.Spec, error) {
	c := &battenCLI{}
	return c.armAt(location, runID, true, verb)
}

// battenPreRun implements the PreRun hook for batten's spec: run's whole existing
// pre-flight, in today's order -- decode the status file, refuse a done slug, resume silently
// over every other found state, and seed a fresh status when absent -- each step's error becoming
// a returned error the generic body reports on the error envelope exactly as run's own inline
// handling did.
//
// This seed-when-absent behaviour is batten's alone and must not leak into the generic body:
// loom refuses in exactly the situation batten seeds here, because only "lyx loom start" may
// seed loom's own status file.
//
// It MkdirAlls the status lock's own ephemeral directory before the first read, mirroring
// battenPreStep's own MkdirAll(filepath.Dir(LockPath)) call: StatusPath is durable and
// StatusLockPath is ephemeral, the two no longer share a directory the way they did before this
// task's durable/ephemeral split, and state.ReadJSONStrict deliberately never creates one itself
// (see its own "no MkdirAll" contract) -- so on a run-id that has never stepped on this machine,
// the very first read here would otherwise fail to acquire the lock with a bare "no such file or
// directory", before the run ever reaches shedengine's own run-lock probe.
func (c *battenCLI) battenPreRun(ctx context.Context) error {
	if err := os.MkdirAll(filepath.Dir(c.shedPaths.StatusLockPath), 0o755); err != nil {
		return err
	}
	st, found, err := state.ReadJSONStrict[shedengine.Status](c.shedPaths.StatusPath, c.shedPaths.StatusLockPath)
	if err != nil {
		return errors.New("battencli: decode status file " + c.shedPaths.StatusPath + ": " + err.Error())
	}
	if found {
		switch st.State {
		case shedengine.StateDone:
			return fmt.Errorf(
				"battencli: %q has already completed; delete %s to run it again",
				c.slug, BattenDir(c.location, c.slug),
			)
		case shedengine.StateRunning, shedengine.StateBlocked, shedengine.StateFailed, shedengine.StatePaused:
			// Each of these resumes silently from the persisted current producer, with no
			// re-seed, no prompt, and no flag: the engine itself already resumes from blocked
			// and failed, and StateBlocked is the everyday path, since every operator-fixable
			// refusal in this task lands there.
		default:
			return fmt.Errorf("battencli: unrecognized status state %q", st.State)
		}
		return nil
	}

	// An absent status file is a fresh start: shedengine.Shed.Run refuses to walk from a status
	// file that does not exist yet (it never seeds one itself), so this hook seeds it here, at
	// the entry row, before the Shed ever reads it. The mutate closure is idempotent against a
	// concurrently-seeded file: it leaves an already-present status untouched rather than
	// overwriting it.
	return state.UpdateJSON(c.shedPaths.StatusPath, c.shedPaths.StatusLockPath, func(cur shedengine.Status, found bool) (shedengine.Status, error) {
		if found {
			return cur, nil
		}
		return shedengine.Status{
			CurrentProducer: battenrecipe.NameWorktreeCreate,
			State:           shedengine.StateRunning,
			History:         []shedengine.HistoryEntry{},
		}, nil
	})
}

// battenPreStep implements the PreStep hook for batten's spec, modelled on loomcli's own
// loomPreStep: a run-lock probe first, then the same work battenPreRun performs -- decode the
// status, refuse a done slug, resume silently over every other state, seed the status when absent.
//
// Without this hook, stepLocked's own read gate would hit an absent status and hard-error, which
// shedverbs/step.go reports as kind: "producer" -- the one kind ly-drive retries, so a fresh slug's
// first step would loop rather than seed.
//
// Its refusal-kind mapping stays inside the closed five: the run lock already held is
// shedverbs.KindBusy; a status decode failure or a failed status seed is shedverbs.KindUnseeded; any
// other pre-producer failure (the unrecognized-state default and the done-slug refusal alike) is
// shedverbs.KindBootstrap. shedverbs.KindOwnership is not used -- it exists for loom's own
// seeded-status-belongs-to-another-slug check, which has no batten analogue -- and
// shedverbs.KindProducer is shed.Step's own to emit, never this hook's.
func (c *battenCLI) battenPreStep(ctx context.Context) (string, error) {
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

	st, found, err := state.ReadJSONStrict[shedengine.Status](c.shedPaths.StatusPath, c.shedPaths.StatusLockPath)
	if err != nil {
		return shedverbs.KindUnseeded, errors.New("battencli: decode status file " + c.shedPaths.StatusPath + ": " + err.Error())
	}
	if found {
		switch st.State {
		case shedengine.StateDone:
			return shedverbs.KindBootstrap, fmt.Errorf(
				"battencli: %q has already completed; delete %s to run it again",
				c.slug, BattenDir(c.location, c.slug),
			)
		case shedengine.StateRunning, shedengine.StateBlocked, shedengine.StateFailed, shedengine.StatePaused:
			// Resumes silently, exactly as battenPreRun's own identical switch does.
		default:
			return shedverbs.KindBootstrap, fmt.Errorf("battencli: unrecognized status state %q", st.State)
		}
		return "", nil
	}

	if err := state.UpdateJSON(c.shedPaths.StatusPath, c.shedPaths.StatusLockPath, func(cur shedengine.Status, found bool) (shedengine.Status, error) {
		if found {
			return cur, nil
		}
		return shedengine.Status{
			CurrentProducer: battenrecipe.NameWorktreeCreate,
			State:           shedengine.StateRunning,
			History:         []shedengine.HistoryEntry{},
		}, nil
	}); err != nil {
		return shedverbs.KindUnseeded, err
	}

	return "", nil
}

// battenPostRun implements the PostRun hook for batten's spec: it returns the envelope's
// "abandonedSession" key only when c.abandonedSession is non-empty, preserving today's
// conditional emission.
//
// The run envelope deliberately carries neither a mutations array nor a partial bool: a run may
// perform zero, one, or two topology mutations at arbitrary points hours apart, so there is no
// coherent single array at run scope, and partial has no referent here. The mutation records are
// logged at Info by the wiring closures (wire.go) instead of discarded.
func (c *battenCLI) battenPostRun(ctx context.Context, result shedengine.Result, runErr error) map[string]any {
	if c.abandonedSession == "" {
		return nil
	}
	return map[string]any{"abandonedSession": c.abandonedSession}
}

// battenStatusExtras implements the StatusExtras hook for batten's spec: batten's own
// three found-envelope keys and no others. The generic body supplies current_producer, state,
// error and activity, which together with these three reproduce today's seven-key found-envelope
// exactly.
func (c *battenCLI) battenStatusExtras(st shedengine.Status) (map[string]any, error) {
	return map[string]any{
		"found":       true,
		"status_path": c.shedPaths.StatusPath,
		"history":     st.History,
	}, nil
}
