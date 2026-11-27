// handover.go pushes a run's records synchronously.
// When origin records another machine's takeover of the run, it hands the run over.
// Its processes end, its local tips are archived and its local pair is removed.

package pairteardown

import (
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/Knatte18/loomyard/internal/boardengine"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/fabriclayout"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/machinename"
	"github.com/Knatte18/loomyard/internal/shedengine"
)

// RunPush carries what a run's synchronous push takes beyond the run it names.
type RunPush struct {
	// LockWait bounds the wait for the push lock, and fabriclayout.LockWaitUnbounded blocks.
	LockWait time.Duration
	// TeardownWait bounds the wait for the pair's teardown lock; a wait that runs out owes the push.
	TeardownWait time.Duration
	// OwedUnrecorded returns an owed push's error with no failed-push record, for a caller that retries the push itself.
	OwedUnrecorded bool
	// InsidePair says the caller runs inside the run's task pair.
	// A hand-over then leaves the pair for the parent's dobby to remove.
	InsidePair bool
	// Parent is the parent pair a Finalize merged the run into, nil otherwise.
	Parent *lymcwd.Location
	// PrimeRecords says the run's unpushed transitions sit in the prime's records.
	// A hand-over then archives the prime's local records tip under an owner archive tag, before its hand-over mark.
	PrimeRecords bool
}

// ThisMachine returns the names that read as this machine in the hub at hubPath.
// It serves the takeover read of a rejected records push.
// It reads this machine's identity and the registry of the hub's local Bolt,
// so a name recorded under any of the names of the machine's registry entry reads as this machine.
func ThisMachine(hubPath string) (fabriclayout.MachineNames, error) {
	here, err := machinename.ReadIdentity()
	if err != nil {
		return fabriclayout.MachineNames{}, err
	}
	owners, err := boardengine.ReadOwners(fabriclayout.BoardDir(hubPath))
	if err != nil {
		return fabriclayout.MachineNames{}, err
	}
	return machineNamesOf(here, owners.Registry), nil
}

// machineNamesOf returns the names of here that read as this machine under registry.
// Current is the current name of here.
// Former holds the former names of here, and the current and former names of the registry entry under the id of here.
// It omits the current name and duplicates.
// A former name another registered machine holds reads as that machine, so it is left out.
func machineNamesOf(here machinename.Identity, registry boardengine.Registry) fabriclayout.MachineNames {
	candidates := slices.Clone(here.Former)
	if entry, registered := registry[here.ID]; registered {
		candidates = append(candidates, entry.Name)
		candidates = append(candidates, entry.Former...)
	}
	var former []string
	for _, name := range candidates {
		if name != "" && name != here.Name && !slices.Contains(former, name) && registry.ReadsAsHere(name, here) {
			former = append(former, name)
		}
	}
	return fabriclayout.MachineNames{Current: here.Name, Former: former}
}

// PushRun pushes the pair at l synchronously for the run slug, in the hub of prime.
// It serves every synchronous push of a run's records: the background pusher's, a teardown's and a catch-up's.
// A rejected records push of a pair that meets another machine's takeover entry for the run hands the run over.
// It then returns that hand-over's error.
// A rejected push of the prime's records can meet the takeover entries of many batten runs.
// Each such run is handed over, with its unpushed prime commits archived and its local pair removed,
// and the push runs again.
// The run's commits are then left out of every later push.
// The push stops once it returns anything but such an entry.
// A push that names a run among the handed-over ones returns an error wrapping fabricengine.ErrHandedOver.
// A failed hand-over pushes nothing.
// It returns an error wrapping fabricengine.ErrTakeoverPassOwed that names the run and the step.
// An error wrapping fabricengine.ErrHandedOver from the push itself passes through unchanged:
// this machine's hand-over mark already names the entry's machine, so nothing more is pushed.
func PushRun(prime, l *lymcwd.Location, slug string, push RunPush) (fabriclayout.PushResult, error) {
	here, err := ThisMachine(prime.HubPath)
	if err != nil {
		return fabriclayout.PushResult{}, err
	}
	pushPair := func() (fabriclayout.PushResult, error) {
		return fabricengine.PushPairAnchored(l, fabricengine.EnvSyncOptions(), fabriclayout.PairPush{
			LockWait:       push.LockWait,
			TeardownWait:   push.TeardownWait,
			RunSlug:        slug,
			Here:           here,
			OwedUnrecorded: push.OwedUnrecorded,
		})
	}
	result, err := pushPair()
	var takeover *fabricengine.TakeoverError
	if errors.As(err, &takeover) {
		return result, HandOver(prime, slug, takeover.Entry, push)
	}

	var namedHandOver error
	for {
		var takeovers *fabricengine.TakeoversError
		if !errors.As(err, &takeovers) {
			break
		}
		for _, pending := range takeovers.Takeovers {
			handOverErr := HandOver(prime, pending.Slug, pending.Entry, RunPush{PrimeRecords: true})
			if !errors.Is(handOverErr, fabricengine.ErrHandedOver) {
				return result, fmt.Errorf("%w: %w", fabricengine.ErrTakeoverPassOwed, handOverErr)
			}
			if pending.Slug == slug {
				namedHandOver = handOverErr
			}
		}
		result, err = pushPair()
	}
	if err == nil && namedHandOver != nil {
		return result, namedHandOver
	}
	return result, err
}

// HandOver hands the run slug over to the machine of entry.
// It ends the processes this machine runs for the run.
// It then hands the run's git state over through fabricengine.HandOverTips.
// That pushes the owed code tip and archives the local tips.
// The pause and cancel intents origin lacks are named in the notice.
// Unless push.InsidePair, it then removes the local pair, whose uncommitted files the archive holds by then;
// inside the pair the removal is left to the parent's dobby.
// It returns an error wrapping fabricengine.ErrHandedOver naming the new owner, so its caller stops.
// A failed step returns that step's error instead, naming the run and the step.
// The next rejected push then hands over again.
func HandOver(prime *lymcwd.Location, slug string, entry shedengine.Takeover, push RunPush) error {
	if err := EndSubjectProcesses(prime, slug, false); err != nil {
		return fmt.Errorf("pairteardown: hand over the run %q: end its processes: %w", slug, err)
	}

	local, _, err := ReadBattenRecord(prime, slug)
	if err != nil {
		return fmt.Errorf("pairteardown: hand over the run %q: read its local batten record: %w", slug, err)
	}
	origin, _, err := fabricengine.OriginBattenStatus(prime, slug)
	if err != nil {
		return fmt.Errorf("pairteardown: hand over the run %q: read its batten record on origin: %w", slug, err)
	}

	td, err := New(prime)
	if err != nil {
		return err
	}
	gone, err := td.TaskWorktreeGone(slug)
	if err != nil {
		return fmt.Errorf("pairteardown: hand over the run %q: look for its pair: %w", slug, err)
	}
	var pair *lymcwd.Location
	if !gone {
		if pair, err = td.taskLocation(slug); err != nil {
			return fmt.Errorf("pairteardown: hand over the run %q: resolve its pair: %w", slug, err)
		}
	}
	request := fabricengine.HandOverRequest{
		Slug:           slug,
		Pair:           pair,
		Parent:         push.Parent,
		Entry:          entry,
		DroppedIntents: droppedIntents(local, origin),
	}
	if push.PrimeRecords {
		request.PrimeRecords = prime
	}
	_, err = fabricengine.HandOverTips(prime, request)
	if err != nil {
		return fmt.Errorf("pairteardown: hand over the run %q: archive its tips: %w", slug, err)
	}

	if pair == nil {
		return fmt.Errorf("%w: machine %q holds the run %q now", fabricengine.ErrHandedOver, entry.Machine, slug)
	}
	if push.InsidePair {
		return fmt.Errorf("%w: machine %q holds the run %q now; its pair is left standing for its parent's dobby to remove", fabricengine.ErrHandedOver, entry.Machine, slug)
	}
	if _, err := td.RemovePair(Request{Slug: slug, Local: true, Force: true, Verdict: fabriclayout.EntryOwnedElsewhere}); err != nil {
		return fmt.Errorf("pairteardown: hand over the run %q: remove its local pair: %w", slug, err)
	}
	return fmt.Errorf("%w: machine %q holds the run %q now", fabricengine.ErrHandedOver, entry.Machine, slug)
}

// HandOverOwnedElsewhere hands the run slug over to owner, the machine that owns it now, from this machine's side.
// With no takeover entry on the run's batten record on origin, it first pushes the run's owed commits.
// Those are the prime's records and then the run's pair when it stands here,
// so this machine's transitions reach origin before it lets go.
// A push that meets a takeover entry hands the run over inside PushRun, which ends the call with nil.
// It then ends this machine's processes for the run, archives the local tips and removes the local pair through HandOver.
// A run past its parent-side merge is handed over the same way, so its owner finishes the Finalize.
// It pushes only commits origin lacks, never force-pushes and writes no board.
// A failed push or hand-over step returns an error naming the run and the step.
// Nothing is archived before a failed push.
func HandOverOwnedElsewhere(prime *lymcwd.Location, slug, owner string) error {
	origin, _, err := fabricengine.OriginBattenStatus(prime, slug)
	if err != nil {
		return fmt.Errorf("pairteardown: hand over the run %q to %q: read its batten record on origin: %w", slug, owner, err)
	}
	if origin.Takeover == nil {
		if err := pushOwedRunCommits(prime, slug); err != nil {
			if errors.Is(err, fabricengine.ErrHandedOver) {
				return nil
			}
			return fmt.Errorf("pairteardown: hand over the run %q to %q: push its owed commits: %w", slug, owner, err)
		}
	}
	if err := HandOver(prime, slug, shedengine.Takeover{Machine: owner}, RunPush{}); err != nil && !errors.Is(err, fabricengine.ErrHandedOver) {
		return err
	}
	return nil
}

// pushOwedRunCommits pushes the prime's records and then the pair of slug when it stands here, records side first.
func pushOwedRunCommits(prime *lymcwd.Location, slug string) error {
	push := RunPush{LockWait: fabriclayout.StatusPushLockWait}
	if _, err := PushRun(prime, prime, slug, push); err != nil {
		return err
	}
	td, err := New(prime)
	if err != nil {
		return err
	}
	gone, err := td.TaskWorktreeGone(slug)
	if err != nil || gone {
		return err
	}
	pair, err := td.taskLocation(slug)
	if err != nil {
		return err
	}
	_, err = PushRun(prime, pair, slug, push)
	return err
}

// droppedIntents names the pause intent and unfinished cancel intent the local batten record holds and origin's lacks.
// Such an intent never reached origin, so the machine that took the run over cannot see it.
func droppedIntents(local, origin shedengine.Status) []string {
	var dropped []string
	if local.PauseIntent != nil && origin.PauseIntent == nil {
		dropped = append(dropped, "pause intent")
	}
	if local.Cancel != nil && local.Cancel.Finish == nil && origin.Cancel == nil {
		dropped = append(dropped, "cancel intent")
	}
	return dropped
}
