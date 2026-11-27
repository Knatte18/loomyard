// registrysetter.go holds the machine registry's setter, the only writer of board.json's `machines` map.
//
// Registry.Check judges a registration against the registry and refuses a clash with the way forward;
// Registry.apply sets this machine's entry; RegisterMachine makes the pull-first Bolt commit and pushes it.

package boardengine

import (
	"errors"
	"fmt"
	"maps"
	"path/filepath"
	"slices"
	"time"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/fabriclayout"
	"github.com/Knatte18/loomyard/internal/machinename"
)

// Registration is what a machine registers: its identity and the former names it drops from its entry.
type Registration struct {
	// Identity is the machine's id, current name, former names and forgotten names.
	Identity machinename.Identity
	// Forget lists the former names `lym machine name --forget-former` drops from this machine's entry.
	Forget []string
}

// RegistrationError is the refusal of a registration or of a missing machine file directory.
type RegistrationError struct {
	// Reason says what clashes.
	Reason string
	// Machine is the registered machine's current name the registration clashes with, empty when none is involved.
	Machine string
	// WayForward says what the operator does next.
	WayForward string
}

// Error reads `machine registration refused: <reason>; way forward: <way forward>`.
func (e *RegistrationError) Error() string {
	return fmt.Sprintf("machine registration refused: %s; way forward: %s", e.Reason, e.WayForward)
}

// sortedIDs returns the registry's machine ids in order, so a refusal always names the same machine.
func (r Registry) sortedIDs() []string {
	return slices.Sorted(maps.Keys(r))
}

// Check refuses a registration that would take a name another registered machine holds, or that a copied file makes.
// The refusal is a *RegistrationError naming the clashing machine and the way forward.
// A registration that passes may still change nothing; apply reports that.
func (r Registry) Check(reg Registration) error {
	identity := reg.Identity
	if identity.ID == "" {
		return &RegistrationError{
			Reason:     "the registration carries no machine id",
			WayForward: "take the identity from boardengine.EnsureIdentity, which always returns one",
		}
	}
	if entry, registered := r[identity.ID]; registered && entry.Name != identity.Name &&
		!slices.Contains(identity.Former, entry.Name) && !slices.Contains(identity.Forgotten, entry.Name) {
		return &RegistrationError{
			Reason: fmt.Sprintf("the registry holds this machine id as %q, which is not this machine's name %q or a name it replaced: "+
				"the machine file was copied from another machine", entry.Name, identity.Name),
			Machine:    entry.Name,
			WayForward: "run `lym machine name <new>` on this machine",
		}
	}
	for _, id := range r.sortedIDs() {
		if id == identity.ID {
			continue
		}
		other := r[id]
		for _, name := range identity.Former {
			if name == other.Name && !slices.Contains(reg.Forget, name) {
				return &RegistrationError{
					Reason:     fmt.Sprintf("this machine's former name %q is the current name of machine %q", name, other.Name),
					Machine:    other.Name,
					WayForward: fmt.Sprintf("run `lym machine name --forget-former %s` on this machine", name),
				}
			}
		}
		if identity.Name == other.Name || slices.Contains(other.Former, identity.Name) {
			return &RegistrationError{
				Reason:     fmt.Sprintf("this machine's name %q is a name of machine %q", identity.Name, other.Name),
				Machine:    other.Name,
				WayForward: "run `lym machine name <new>` on this machine",
			}
		}
	}
	return nil
}

// apply returns the registry with this registration's entry set, and whether anything changed.
// The entry takes the registration's current name.
// Its former names are the union of the entry's, the registration's and the old current name when that changed.
// The current name and reg.Forget are removed from them.
// It never overwrites another machine's entry; Check runs first.
func (r Registry) apply(reg Registration) (Registry, bool) {
	identity := reg.Identity
	next := maps.Clone(r)
	if next == nil {
		next = Registry{}
	}
	previous, registered := r[identity.ID]

	candidates := slices.Concat(previous.Former, identity.Former)
	if registered && previous.Name != "" {
		candidates = append(candidates, previous.Name)
	}
	var former []string
	for _, name := range candidates {
		if name == "" || name == identity.Name || slices.Contains(reg.Forget, name) || slices.Contains(former, name) {
			continue
		}
		former = append(former, name)
	}

	next[identity.ID] = MachineEntry{Name: identity.Name, Former: former, Unknown: previous.Unknown}
	changed := !registered || previous.Name != identity.Name || !slices.Equal(previous.Former, former)
	return next, changed
}

// RegisterMachine registers reg in the board at boardDir, the only write of its `machines` map besides an owner write.
// It makes one pull-first Bolt commit, judging reg with Registry.Check against the registry the pull brought.
// It pushes the commit before it returns.
// It reports whether it wrote: a registration the registry already holds writes and pushes nothing.
// A refusal is a *RegistrationError and writes nothing.
// An unreachable origin returns ErrOwnerOriginUnreachable, wrapped with its detail.
// BOARD_SKIP_GIT writes board.json with no pull, commit or push, and BOARD_SKIP_PUSH commits with no pull or push.
func RegisterMachine(boardDir string, reg Registration) (bool, error) {
	wrote := false
	write := func() ([]string, error) {
		store := NewStore(boardDir)
		if err := store.Load(); err != nil {
			return nil, err
		}
		if err := store.machines.Check(reg); err != nil {
			return nil, err
		}
		next, changed := store.machines.apply(reg)
		if !changed {
			return nil, nil
		}
		store.machines = next
		if err := store.Save(); err != nil {
			return nil, fmt.Errorf("save store: %w", err)
		}
		rendered, err := renderOwnerWrite(boardDir, store.Tasks(), store.primeOwner)
		if err != nil {
			return nil, err
		}
		wrote = true
		return append([]string{boardFile}, rendered...), nil
	}
	message := fmt.Sprintf("board: register machine %s", reg.Identity.Name)
	if err := commitBoardWriteAndPush(boardDir, message, write, nil); err != nil {
		return false, err
	}
	return wrote, nil
}

// commitBoardWriteAndPush runs write as one pull-first Bolt commit with message and pushes it synchronously.
// It is the owner write's commit path for a write that has no retry hook of its own.
// beforePush runs once between the commit and the first push, and a nil one runs nothing.
// An unreachable origin returns ErrOwnerOriginUnreachable, wrapped with its detail;
// a pull that cannot bring the board up to date and a push that stays deferred are refused with their way forward.
func commitBoardWriteAndPush(boardDir, message string, write func() ([]string, error), beforePush func()) error {
	cfg := ApplySkipEnv(Config{Path: boardDir})
	opts := fabricengine.SyncOptions{SkipGit: cfg.SkipGit, SkipPush: cfg.SkipPush}
	bolt := fabricengine.NewBolt(boardDir)

	if cfg.SkipGit || cfg.SkipPush {
		_, _, err := bolt.CommitWritten(message, write, opts)
		return err
	}
	rec := fabricengine.NewMutations(filepath.Dir(boardDir))
	result, err := bolt.PullThenCommitWritten([]fabricengine.BoltWrite{{Message: message, Write: write}}, rec)
	if err != nil {
		return err
	}
	switch result.Skipped {
	case "":
	case fabriclayout.BoltSkipFetchFailed:
		return fmt.Errorf("%w: %s", ErrOwnerOriginUnreachable, result.SkipDetail)
	default:
		return errors.New("board: the write was refused, since the board could not be brought up to date: " + result.SkipDetail)
	}
	if beforePush != nil {
		beforePush()
	}
	return pushOwnerCommit(bolt, opts, rec, time.Sleep)
}
