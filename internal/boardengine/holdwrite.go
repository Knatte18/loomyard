// holdwrite.go holds the adopt hold's setters.
// They are the plain hold write, the one-commit adopt write, and the fix and clear setters.
//
// WriteHold appends one hold in a pull-first Bolt commit and pushes it before it returns.
// WriteAdopt writes the hold, the owners and this machine's registry entry in one such commit.
// FixHold and ClearHold turn this machine's hold into an adopt-fix hold and remove it, each pushed before it returns.

package boardengine

import (
	"fmt"
	"maps"
	"reflect"
	"slices"
	"strings"
	"time"

	"github.com/Knatte18/loomyard/internal/machinename"
)

// holdSetterBeforePushHook runs once between the commit and the first push of FixHold and ClearHold; nil in production.
// A test sets it to plant another clone's write there, and so runs serially.
var holdSetterBeforePushHook func()

// holdWaitWayForward is the way forward of a refusal over another machine's hold, worded so an agent may read it.
const holdWaitWayForward = "wait for that machine's adopt to finish, and if that machine has stopped, ask the operator"

// HoldWrite reports what WriteHold did.
type HoldWrite struct {
	// Wrote reports whether the hold was appended and pushed.
	Wrote bool
	// Standing is the hold of another machine that stands instead, nil when none does.
	Standing *Hold
}

// WriteHold appends hold to the board at boardDir in one pull-first Bolt commit and pushes it before it returns.
// After the pull it judges the holds that stand.
// A hold of another machine stands, so nothing is written and it is returned in Standing.
// A hold of the same machine with the same kind and subjects stands too, so nothing is written, and Standing is nil.
// A hold of the same machine with another kind or other subjects is appended beside the standing one.
// A re-run of a cut write is therefore idempotent, and a hold another machine pushed meanwhile wins.
// The machines are compared through the registry the pull brought,
// so a hold under a former name reads as the same machine.
// A kind that is neither HoldKindAdopt nor HoldKindAdoptFix, or an adopt-fix hold with no FixTask, is refused.
// An unreachable origin returns ErrOwnerOriginUnreachable, wrapped with its detail.
// BOARD_SKIP_GIT writes board.json with no pull, commit or push, and BOARD_SKIP_PUSH commits with no pull or push.
func WriteHold(boardDir string, hold Hold) (HoldWrite, error) {
	if hold.Kind != HoldKindAdopt && hold.Kind != HoldKindAdoptFix {
		return HoldWrite{}, fmt.Errorf("board: the hold's Kind is %q, and a hold is %q or %q", hold.Kind, HoldKindAdopt, HoldKindAdoptFix)
	}
	if hold.Kind == HoldKindAdoptFix && hold.FixTask == "" {
		return HoldWrite{}, fmt.Errorf("board: an %q hold names its fix task in FixTask, and this one has none", HoldKindAdoptFix)
	}

	var result HoldWrite
	write := func() ([]string, error) {
		store := NewStore(boardDir)
		if err := store.Load(); err != nil {
			return nil, err
		}
		repeated := false
		for _, standing := range store.holds {
			if !store.machines.sameMachine(standing.Machine, hold.Machine) {
				result.Standing = &standing
				return nil, nil
			}
			if standing.Kind == hold.Kind && slices.Equal(standing.Subjects, hold.Subjects) {
				repeated = true
			}
		}
		if repeated {
			return nil, nil
		}
		store.holds = append(store.holds, hold)
		paths, err := saveAndRenderOwnerWrite(boardDir, store)
		if err != nil {
			return nil, err
		}
		result.Wrote = true
		return paths, nil
	}
	message := fmt.Sprintf("board: %s hold by %s", hold.Kind, hold.Machine)
	if err := commitBoardWriteAndPush(boardDir, message, write, nil); err != nil {
		return HoldWrite{}, err
	}
	return result, nil
}

// saveAndRenderOwnerWrite saves store and renders its board, and returns the board-relative paths the write changed.
func saveAndRenderOwnerWrite(boardDir string, store *Store) ([]string, error) {
	if err := store.Save(); err != nil {
		return nil, fmt.Errorf("save store: %w", err)
	}
	rendered, err := renderOwnerWrite(boardDir, store.Tasks(), store.primeOwner)
	if err != nil {
		return nil, err
	}
	return append([]string{boardFile}, rendered...), nil
}

// sameMachine reports whether the recorded names a and b name one machine.
// Two names an entry resolves to the same id name one machine; other names name one only when they are equal.
func (r Registry) sameMachine(a, b string) bool {
	if a == b {
		return true
	}
	idA, _, foundA := r.Resolve(a)
	idB, _, foundB := r.Resolve(b)
	return foundA && foundB && idA == idB
}

// HoldRefusal is the refusal of an adopt or of a hold setter over a hold that stands or that was lost.
type HoldRefusal struct {
	// Reason says which hold stands and why the write is refused.
	Reason string
	// Machine is the other machine whose hold stands, empty when none does.
	Machine string
	// OwnStands reports whether this machine's hold stood when the setter re-read the board after its push.
	OwnStands bool
	// WayForward says what the caller does next.
	WayForward string
}

// Error reads `adopt hold refused: <reason>; way forward: <way forward>`.
func (e *HoldRefusal) Error() string {
	return fmt.Sprintf("adopt hold refused: %s; way forward: %s", e.Reason, e.WayForward)
}

// holdSubjectNames returns the subjects a hold covers, in the hold's order.
func holdSubjectNames(subjects []HoldSubject) []string {
	names := make([]string, len(subjects))
	for i, subject := range subjects {
		names[i] = subject.Subject
	}
	return names
}

// sameSubjectSet reports whether a and b name the same subjects, order ignored.
func sameSubjectSet(a, b []string) bool {
	return slices.Equal(slices.Sorted(slices.Values(a)), slices.Sorted(slices.Values(b)))
}

// otherMachineRefusal refuses a write over the hold of another machine.
// The refusal names that machine and when its hold was written.
func otherMachineRefusal(standing Hold, ownStands bool) *HoldRefusal {
	return &HoldRefusal{
		Reason:     fmt.Sprintf("machine %q holds the hub for an adopt since %s", standing.Machine, standing.At.Format(time.RFC3339)),
		Machine:    standing.Machine,
		OwnStands:  ownStands,
		WayForward: holdWaitWayForward,
	}
}

// adoptAgainWayForward is the way forward that writes the hold again and checks again.
func adoptAgainWayForward(subjects []string) string {
	return "run `lym adopt " + strings.Join(subjects, " ") + "`"
}

// AdoptRequest describes the one-commit write of `lym adopt`.
type AdoptRequest struct {
	// Machine is this machine's name, the owner and the hold's machine to record.
	Machine string
	// Identity is this machine's identity, registered in the same commit after a check against the registry the pull brought.
	// A refused registration returns its *RegistrationError with nothing written.
	Identity machinename.Identity
	// Slugs names the beam subjects adopted.
	Slugs []string
	// Prime reports whether the root is a subject.
	Prime bool
	// TakeHold moves another machine's hold over the same subjects to this machine.
	// It moves only such a hold, skips no check and lets no run go on.
	TakeHold bool
	// ParentOf gives an entry's parent slug, "" for the prime, as a read judges it; nil reads every parent as the prime.
	ParentOf func(slug string) (string, error)
	// Now is the time the hold is written at.
	Now time.Time
	// BeforePush runs once between the commit and the first push; nil in production.
	BeforePush func()
}

// AdoptWrite reports what WriteAdopt left standing.
type AdoptWrite struct {
	// Hold is the hold that stands after the push.
	Hold Hold
	// Kept reports that this machine's hold over the same subjects stood already and was left as it was.
	Kept bool
	// Recorded lists the subjects whose owner reads as this machine after the push, OwnerWritePrime for the root.
	Recorded []string
}

// adoptHoldPlan is how a WriteAdopt settles the hold: kept as it stands, replaced at index, or appended when index is -1.
type adoptHoldPlan struct {
	hold  Hold
	index int
	kept  bool
}

// adoptSubjects returns the subjects req adopts: its beam slugs, then RootSubject when the prime is one.
func adoptSubjects(req AdoptRequest) []string {
	subjects := slices.Clone(req.Slugs)
	if req.Prime {
		subjects = append(subjects, RootSubject)
	}
	return subjects
}

// planAdoptHold judges the holds that stand over the subjects req adopts and returns how the hold is settled.
// registry is the registry as it stands once this machine is registered.
func planAdoptHold(store *Store, registry Registry, req AdoptRequest, subjects []string) (adoptHoldPlan, error) {
	for i, standing := range store.holds {
		if registry.ReadsAsHere(standing.Machine, req.Identity) {
			continue
		}
		if !req.TakeHold {
			return adoptHoldPlan{}, otherMachineRefusal(standing, false)
		}
		held := holdSubjectNames(standing.Subjects)
		if !sameSubjectSet(held, subjects) {
			return adoptHoldPlan{}, &HoldRefusal{
				Reason:     fmt.Sprintf("machine %q holds the hub for an adopt of %s, not of the subjects adopted now", standing.Machine, strings.Join(held, " ")),
				Machine:    standing.Machine,
				WayForward: "run `lym adopt --take-hold " + strings.Join(held, " ") + "`",
			}
		}
		rewritten := standing
		rewritten.Machine, rewritten.At = req.Machine, req.Now
		if standing.Kind == HoldKindAdoptFix && standing.FixMachine == "" {
			rewritten.FixMachine = standing.Machine
		}
		return adoptHoldPlan{hold: rewritten, index: i}, nil
	}
	for i, standing := range store.holds {
		if sameSubjectSet(holdSubjectNames(standing.Subjects), subjects) {
			return adoptHoldPlan{hold: standing, index: i, kept: true}, nil
		}
	}
	if len(store.holds) > 0 {
		held := holdSubjectNames(store.holds[0].Subjects)
		return adoptHoldPlan{}, &HoldRefusal{
			Reason:     fmt.Sprintf("this machine's hold covers %s, not the subjects adopted now", strings.Join(held, " ")),
			WayForward: adoptAgainWayForward(held) + " first",
		}
	}

	hold := Hold{Kind: HoldKindAdopt, Machine: req.Machine, At: req.Now}
	for _, subject := range subjects {
		prior := store.primeOwner
		if subject != RootSubject {
			effective, err := store.effectiveOwner(subject)
			if err != nil {
				return adoptHoldPlan{}, fmt.Errorf("entry %q: resolve its owner: %w", subject, err)
			}
			prior = effective
		}
		hold.Subjects = append(hold.Subjects, HoldSubject{Subject: subject, PriorOwner: prior})
	}
	return adoptHoldPlan{hold: hold, index: -1}, nil
}

// readHoldsAfterPush reads the holds of the local board at boardDir after a push.
// own holds the holds that read as here, and other the first hold of another machine, nil when none stands.
func readHoldsAfterPush(boardDir string, here machinename.Identity) (own []Hold, other *Hold, owners Owners, err error) {
	owners, err = ReadOwners(boardDir)
	if err != nil {
		return nil, nil, Owners{}, err
	}
	for _, hold := range owners.Holds {
		switch {
		case owners.Registry.ReadsAsHere(hold.Machine, here):
			own = append(own, hold)
		case other == nil:
			standing := hold
			other = &standing
		}
	}
	return own, other, owners, nil
}

// WriteAdopt writes the adopt in one pull-first Bolt commit and pushes it before it returns:
// the hold, the owner of each subject, and this machine's registry entry.
// Over the store the pull brought, with no hold standing it appends an adopt hold naming each subject's prior owner.
// This machine's hold over the same subjects is kept as it stands, its kind, fix task and try counts included.
// This machine's hold over other subjects, an overlapping set included, is refused, naming its subjects.
// Another machine's hold is refused, naming that machine and when it wrote the hold.
// With TakeHold that machine's hold over the same subjects is rewritten to name this machine.
// It keeps kind, fix task, try counts and prior owners; over other subjects it is refused.
// A rejected push replays the commit onto origin's tip, which refuses it whole over a hold another machine wrote.
// After the push it re-reads the local board.
// It refuses, naming the winner, when a subject's owner or the hold reads as another machine.
// An unreachable origin returns ErrOwnerOriginUnreachable, wrapped with its detail.
// BOARD_SKIP_GIT writes board.json with no pull, commit or push, and BOARD_SKIP_PUSH commits with no pull or push.
func WriteAdopt(boardDir string, req AdoptRequest) (AdoptWrite, error) {
	subjects := adoptSubjects(req)
	var written []string
	var kept bool
	write := func() ([]string, error) {
		store := NewStore(boardDir)
		store.parentOf = req.ParentOf
		if err := store.Load(); err != nil {
			return nil, err
		}
		registration := Registration{Identity: req.Identity}
		if err := store.machines.Check(registration); err != nil {
			return nil, err
		}
		registry, _ := store.machines.apply(registration)
		plan, err := planAdoptHold(store, registry, req, subjects)
		if err != nil {
			return nil, err
		}
		kept = plan.kept

		ownerRequest := OwnerRequest{Machine: req.Machine, Identity: req.Identity, Slugs: req.Slugs, Prime: req.Prime, Takeover: true, ParentOf: req.ParentOf}
		var ownerPaths []string
		written, ownerPaths, err = writeOwners(boardDir, ownerRequest, map[string]string{})
		if err != nil {
			return nil, err
		}
		if plan.kept {
			return ownerPaths, nil
		}

		store = NewStore(boardDir)
		store.parentOf = req.ParentOf
		if err := store.Load(); err != nil {
			return nil, err
		}
		if plan.index >= 0 {
			store.holds[plan.index] = plan.hold
		} else {
			store.holds = append(store.holds, plan.hold)
		}
		paths, err := saveAndRenderOwnerWrite(boardDir, store)
		if err != nil {
			return nil, err
		}
		return slices.Compact(slices.Sorted(slices.Values(slices.Concat(paths, ownerPaths)))), nil
	}
	message := fmt.Sprintf("board: adopt by %s of %s", req.Machine, strings.Join(subjects, ", "))
	if err := commitBoardWriteAndPush(boardDir, message, write, req.BeforePush); err != nil {
		return AdoptWrite{}, err
	}

	own, other, owners, err := readHoldsAfterPush(boardDir, req.Identity)
	if err != nil {
		return AdoptWrite{}, err
	}
	if other != nil {
		return AdoptWrite{}, otherMachineRefusal(*other, len(own) > 0)
	}
	for _, subject := range written {
		standing := owners.Prime
		if subject != OwnerWritePrime {
			standing = owners.Entries[subject].Owner
		}
		if OwnedElsewhere(standing, owners.Registry, req.Identity) {
			return AdoptWrite{}, &HoldRefusal{
				Reason:     fmt.Sprintf("machine %q owns %s after the push", standing, subject),
				Machine:    standing,
				OwnStands:  len(own) > 0,
				WayForward: holdWaitWayForward,
			}
		}
	}
	for _, hold := range own {
		if sameSubjectSet(holdSubjectNames(hold.Subjects), subjects) {
			return AdoptWrite{Hold: hold, Kept: kept, Recorded: written}, nil
		}
	}
	return AdoptWrite{}, &HoldRefusal{
		Reason:     "this machine's hold no longer stands after the push",
		OwnStands:  len(own) > 0,
		WayForward: adoptAgainWayForward(subjects),
	}
}

// FixHold rewrites this machine's hold to kind adopt-fix naming fixTask, with FixMachine this machine's name.
// Each subject's Tries comes from tries, and a subject absent from it keeps its own.
// It pushes in one pull-first Bolt commit and re-reads the local board after the push:
// it returns the hold only when this machine's hold stands as written.
// No hold standing is refused with the adopt that writes one.
// A hold of another machine is refused naming that machine.
// A replay that lost the change, or one a take-hold refused, is refused the same way after the push.
// An unreachable origin returns ErrOwnerOriginUnreachable, wrapped with its detail.
func FixHold(boardDir string, here machinename.Identity, fixTask string, tries map[string]int) (Hold, error) {
	subjects := slices.Sorted(maps.Keys(tries))
	var want Hold
	write := func() ([]string, error) {
		store := NewStore(boardDir)
		if err := store.Load(); err != nil {
			return nil, err
		}
		own := -1
		for i, standing := range store.holds {
			if !store.machines.ReadsAsHere(standing.Machine, here) {
				return nil, otherMachineRefusal(standing, false)
			}
			if own < 0 {
				own = i
			}
		}
		if own < 0 {
			return nil, &HoldRefusal{Reason: "no hold of this machine stands", WayForward: adoptAgainWayForward(subjects)}
		}
		want = store.holds[own]
		want.Kind, want.FixTask, want.FixMachine = HoldKindAdoptFix, fixTask, here.Name
		want.Subjects = slices.Clone(want.Subjects)
		for i, subject := range want.Subjects {
			if count, counted := tries[subject.Subject]; counted {
				want.Subjects[i].Tries = count
			}
		}
		if reflect.DeepEqual(store.holds[own], want) {
			return nil, nil
		}
		store.holds[own] = want
		return saveAndRenderOwnerWrite(boardDir, store)
	}
	message := fmt.Sprintf("board: adopt-fix hold by %s for %s", here.Name, fixTask)
	if err := commitBoardWriteAndPush(boardDir, message, write, holdSetterBeforePushHook); err != nil {
		return Hold{}, err
	}

	own, other, _, err := readHoldsAfterPush(boardDir, here)
	if err != nil {
		return Hold{}, err
	}
	if other != nil {
		return Hold{}, otherMachineRefusal(*other, len(own) > 0)
	}
	for _, hold := range own {
		if hold.Kind == want.Kind && hold.FixTask == want.FixTask && hold.FixMachine == want.FixMachine && slices.Equal(hold.Subjects, want.Subjects) {
			return hold, nil
		}
	}
	return Hold{}, &HoldRefusal{
		Reason:     fmt.Sprintf("the %s hold of this machine does not stand as written after the push", HoldKindAdoptFix),
		OwnStands:  len(own) > 0,
		WayForward: adoptAgainWayForward(holdSubjectNames(want.Subjects)),
	}
}

// ClearHold removes this machine's hold in one pull-first Bolt commit, pushes it, and reports whether a hold stood.
// It returns false, writing nothing, when no hold stands.
// A hold of another machine is left and refused, naming that machine.
// It re-reads the local board after the push and refuses when a hold of this machine, or another machine's, still stands:
// a replay that lost the removal, or one a take-hold refused.
// An unreachable origin returns ErrOwnerOriginUnreachable, wrapped with its detail.
func ClearHold(boardDir string, here machinename.Identity) (bool, error) {
	var cleared bool
	var standingSubjects []string
	write := func() ([]string, error) {
		store := NewStore(boardDir)
		if err := store.Load(); err != nil {
			return nil, err
		}
		for _, standing := range store.holds {
			if !store.machines.ReadsAsHere(standing.Machine, here) {
				return nil, otherMachineRefusal(standing, false)
			}
		}
		if len(store.holds) == 0 {
			return nil, nil
		}
		standingSubjects = holdSubjectNames(store.holds[0].Subjects)
		store.holds = nil
		paths, err := saveAndRenderOwnerWrite(boardDir, store)
		cleared = err == nil
		return paths, err
	}
	message := fmt.Sprintf("board: clear hold by %s", here.Name)
	if err := commitBoardWriteAndPush(boardDir, message, write, holdSetterBeforePushHook); err != nil {
		return false, err
	}
	if !cleared {
		return false, nil
	}

	own, other, _, err := readHoldsAfterPush(boardDir, here)
	if err != nil {
		return false, err
	}
	if other != nil {
		return false, otherMachineRefusal(*other, len(own) > 0)
	}
	if len(own) > 0 {
		return false, &HoldRefusal{
			Reason:     "this machine's hold still stands after the push",
			OwnStands:  true,
			WayForward: adoptAgainWayForward(standingSubjects),
		}
	}
	return true, nil
}
