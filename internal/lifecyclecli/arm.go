// arm.go implements lifecyclecli's exported Arm resolution entry point, plus the two-function
// split behind it: the unexported worker arm resolves cwd, applies the non-prime refusal, reads
// the slug, and wires the whole engine stack exactly as resolvePersistentPreRun (cli.go) always
// has, and the resolution-free specFor fills a Spec from an already-wired receiver. See the
// overview's exported-Arm-is-the-single-resolution-entry-point and
// spec-fill-is-separable-from-resolution Shared Decisions.
//
// Arm carries no command-name guard of its own: the existing cmd.Name() == "lifecycle" short-
// circuit stays in resolvePersistentPreRun, where it lets a bare group listing run without a git
// repository -- a bare "lyx shed" listing is shedcli's own equivalent guard, not this module's.

package lifecyclecli

import (
	"context"
	"errors"
	"fmt"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/lifecyclerecipe"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shedverbs"
	"github.com/Knatte18/loomyard/internal/state"
)

// arm resolves cwd into a *lyxcwd.Location, applies the non-prime refusal, reads the slug from
// args, wires the receiver, and returns the filled Spec. It is the single worker both
// resolvePersistentPreRun and the exported Arm wrapper delegate to, so the *lifecycleCLI whose
// fields the returned hooks close over is always the same value the caller holds: the pre-run's
// own c on the "lyx lifecycle" path, the wrapper's freshly-constructed one on the "lyx shed" path.
func (c *lifecycleCLI) arm(cwd string, verb string, args []string) (shedverbs.Spec, error) {
	location, err := lyxcwd.Resolve(cwd)
	if err != nil {
		// lyxcwd.Resolve's error is already self-describing (it IS the "not a git repository"
		// sentinel); pass it through bare rather than doubling that same text on top of it.
		return shedverbs.Spec{}, err
	}

	primeName, primeNameErr := fabricengine.PrimeName(location)
	if refusalErr := refuseNonPrime(location.WorktreeName, primeName, primeNameErr); refusalErr != nil {
		return shedverbs.Spec{}, refusalErr
	}

	slug := ""
	if len(args) > 0 {
		slug = args[0]
	}
	c.location = location
	c.slug = slug

	if err := c.wire(location, slug); err != nil {
		return shedverbs.Spec{}, err
	}

	return c.specFor(verb), nil
}

// specFor fills a Spec from c's already-wired fields, performing no resolution, no wire call, and
// no I/O of its own. internal/lifecyclecli's own untagged run_test.go and cli_test.go call this
// directly against a hand-populated receiver, so their leaf-command tests never spawn git.
func (c *lifecycleCLI) specFor(verb string) shedverbs.Spec {
	spec := shedverbs.Spec{
		StatusPath:     c.shedPaths.StatusPath,
		LockPath:       c.shedPaths.LockPath,
		StatusLockPath: c.shedPaths.StatusLockPath,
		// lifecycle's status and pause are both false here: status is read-only, so creating its
		// per-slug directory as a side effect of reading it would be a new, unasked-for write.
		EnsureStatusLockDir: false,
		StatusLabel:         "lifecycle",
		DecodeErrPrefix:     "lifecyclecli:",
		RunBusyMessage:      fmt.Sprintf("lifecyclecli: another lifecycle run already holds the run lock %q", c.shedPaths.LockPath),
		// step has no lifecycle analogue, so both of its told fields stay at their zero values.
		StepBusyMessage: "",
		StepBusyKind:    "",
		AbsentStatus: shedverbs.AbsentDisposition{
			// A slug that has never run on this machine is a determined answer, not an error.
			Refuse: false,
		},
		PauseAbsentMessage: fmt.Sprintf("lifecyclecli: no status file at %s; there is nothing running to pause -- run \"lyx lifecycle run <slug>\" first", c.shedPaths.StatusPath),
		Hooks: shedverbs.Hooks{
			PreRun:       c.lifecyclePreRun,
			PostRun:      c.lifecyclePostRun,
			StatusExtras: c.lifecycleStatusExtras,
		},
	}

	if verb == "run" {
		spec.BuildShed = func() (*shedengine.Shed, error) { return lifecyclerecipe.New(c.env, c.shedPaths) }
	}

	return spec
}

// Arm is lifecyclecli's exported resolution entry point, for internal/shedcli's table: it
// constructs a fresh receiver and returns c.arm(cwd, verb, args). A package-level Arm alone could
// not serve resolvePersistentPreRun, because wire is a method on the receiver and the hooks close
// over c.env, c.shedPaths and c.abandonedSession, and the pre-run's own c.location and c.slug
// assignments would stop happening.
func Arm(cwd string, verb string, args []string) (shedverbs.Spec, error) {
	c := &lifecycleCLI{}
	return c.arm(cwd, verb, args)
}

// lifecyclePreRun implements the PreRun hook for lifecycle's spec: run's whole existing
// pre-flight, in today's order -- decode the status file, refuse a done slug, resume silently
// over every other found state, and seed a fresh status when absent -- each step's error becoming
// a returned error the generic body reports on the error envelope exactly as run's own inline
// handling did.
//
// This seed-when-absent behaviour is lifecycle's alone and must not leak into the generic body:
// loom refuses in exactly the situation lifecycle seeds here, because only "lyx loom start" may
// seed loom's own status file.
func (c *lifecycleCLI) lifecyclePreRun(ctx context.Context) error {
	st, found, err := state.ReadJSONStrict[shedengine.Status](c.shedPaths.StatusPath, c.shedPaths.StatusLockPath)
	if err != nil {
		return errors.New("lifecyclecli: decode status file " + c.shedPaths.StatusPath + ": " + err.Error())
	}
	if found {
		switch st.State {
		case shedengine.StateDone:
			return fmt.Errorf(
				"lifecyclecli: %q has already completed; delete %s to run it again",
				c.slug, LifecycleDir(c.location, c.slug),
			)
		case shedengine.StateRunning, shedengine.StateBlocked, shedengine.StateFailed, shedengine.StatePaused:
			// Each of these resumes silently from the persisted current producer, with no
			// re-seed, no prompt, and no flag: the engine itself already resumes from blocked
			// and failed, and StateBlocked is the everyday path, since every operator-fixable
			// refusal in this task lands there.
		default:
			return fmt.Errorf("lifecyclecli: unrecognized status state %q", st.State)
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
			CurrentProducer: lifecyclerecipe.NameWorktreeCreate,
			State:           shedengine.StateRunning,
			History:         []shedengine.HistoryEntry{},
		}, nil
	})
}

// lifecyclePostRun implements the PostRun hook for lifecycle's spec: it returns the envelope's
// "abandonedSession" key only when c.abandonedSession is non-empty, preserving today's
// conditional emission.
//
// The run envelope deliberately carries neither a mutations array nor a partial bool: a run may
// perform zero, one, or two topology mutations at arbitrary points hours apart, so there is no
// coherent single array at run scope, and partial has no referent here. The mutation records are
// logged at Info by the wiring closures (wire.go) instead of discarded.
func (c *lifecycleCLI) lifecyclePostRun(ctx context.Context, result shedengine.Result, runErr error) map[string]any {
	if c.abandonedSession == "" {
		return nil
	}
	return map[string]any{"abandonedSession": c.abandonedSession}
}

// lifecycleStatusExtras implements the StatusExtras hook for lifecycle's spec: lifecycle's own
// three found-envelope keys and no others. The generic body supplies current_producer, state,
// error and activity, which together with these three reproduce today's seven-key found-envelope
// exactly.
func (c *lifecycleCLI) lifecycleStatusExtras(st shedengine.Status) (map[string]any, error) {
	return map[string]any{
		"found":       true,
		"status_path": c.shedPaths.StatusPath,
		"history":     st.History,
	}, nil
}
