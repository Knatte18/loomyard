// runintents.go fills the seams of the pause and resume verbs.
// They record a run's pause intent.
// The intent lives in the run's batten record on the prime, which is committed and pushed with the run's other records.

package tendercli

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"time"

	"github.com/Knatte18/loomyard/internal/agentname"
	"github.com/Knatte18/loomyard/internal/boardengine"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/hubgeom"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/machinename"
	"github.com/Knatte18/loomyard/internal/pairteardown"
	"github.com/Knatte18/loomyard/internal/pauserecord"
	"github.com/Knatte18/loomyard/internal/runfsm"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shedrun"
	"github.com/Knatte18/loomyard/internal/tenderengine"
)

// resumeAside is what a resume sets aside before the act rule decides.
// The resume lifts its own pause entry and drives nothing.
// A pause, a cancel under way and a shed pause therefore do not stop it, while a finished cancel still reads cancelled.
// It acts offline too, so an owed push does not stop it.
// It sets the adopt hold aside as well, since the resume's own hold rules decide what a hold leaves standing.
var resumeAside = runfsm.Aside{Pause: true, UnfinishedCancel: true, ShedPause: true, PushOwed: true, Hold: true}

// runIntents holds the seams one run's pause and resume verbs ask, over the hub's prime.
type runIntents struct {
	// Owner returns the effective owner and the adopt subject when another machine owns the run, empty strings otherwise.
	Owner func() (owner, subject string, err error)
	// Record writes the pause intent into the run's batten record, commits it, and pushes the records side and waits for it.
	Record func() error
	// Verdict returns the act rule's answer over the run, with what a resume sets aside cleared.
	Verdict func() (runfsm.Verdict, error)
	// Clear removes the pause intent from the run's batten record, commits it, and pushes the records side and waits for it.
	Clear func() error
}

// hubRunIntents fills the seams of the run of slug over the prime of the hub location belongs to.
func hubRunIntents(location *lymcwd.Location, slug string) (runIntents, error) {
	prime, err := hubPrime(location)
	if err != nil {
		return runIntents{}, err
	}
	return runIntents{
		Owner:  func() (string, string, error) { return ownerElsewhere(prime, slug) },
		Record: func() error { return recordPauseIntent(prime, slug) },
		Verdict: func() (runfsm.Verdict, error) {
			records, inputs, err := pairteardown.ReadRunPulled(prime, slug)
			if err != nil {
				return runfsm.Verdict{}, err
			}
			_, verdict := runfsm.Judge(records, inputs, resumeAside)
			return verdict, nil
		},
		Clear: func() error {
			return commitRunIntent(prime, slug, "tender resume: clear the pause intent of "+slug, os.Stderr, func() (bool, error) {
				return shedrun.ClearPauseIntent(prime, slug)
			})
		},
	}, nil
}

// hubPrime resolves the prime of the hub location belongs to.
func hubPrime(location *lymcwd.Location) (*lymcwd.Location, error) {
	primeName, err := fabricengine.PrimeName(location)
	if err != nil {
		return nil, fmt.Errorf("resolve the hub's prime: %w", err)
	}
	prime, err := lymcwd.ResolveWorktree(filepath.Join(location.HubPath, primeName))
	if err != nil {
		return nil, fmt.Errorf("locate the hub's prime %q: %w", primeName, err)
	}
	return prime, nil
}

// recordPauseIntent records a pause intent on the batten record of slug on the prime.
// It pushes the record, by the calling strand.
func recordPauseIntent(prime *lymcwd.Location, slug string) error {
	by := os.Getenv(agentname.StrandNameEnv)
	if by == "" {
		by = agentname.External
	}
	intent := shedengine.PauseIntent{By: by, At: time.Now().Format(time.RFC3339)}
	return commitRunIntent(prime, slug, "tender pause: record the pause intent of "+slug, os.Stderr, func() (bool, error) {
		return shedrun.WritePauseIntent(prime, slug, intent)
	})
}

// PauseIntentEntries writes this machine's pause entries from the pause intents of the runs of slugs.
// It reads them in the hub location belongs to.
// A slug whose batten record on the prime holds a pause intent and whose scope has no live entry is paused.
// It is paused under the default caller and the pause grace, so its local processes hold as the intent's run does.
// A slug with no intent, and a slug whose scope already has a live entry, writes nothing.
// A failed read or pause of one slug is joined into the returned error and the other slugs go on.
func PauseIntentEntries(location *lymcwd.Location, slugs []string) error {
	if len(slugs) == 0 {
		return nil
	}
	prime, err := hubPrime(location)
	if err != nil {
		return err
	}
	geometry, err := hubgeom.PauseGeometry(location)
	if err != nil {
		return err
	}
	deps, _, err := HubDeps(location, os.Getenv(agentname.StrandNameEnv))
	if err != nil {
		return err
	}
	var failures []error
	for _, slug := range slugs {
		if err := pauseFromIntent(prime, deps, geometry.RecordPath, slug); err != nil {
			failures = append(failures, fmt.Errorf("pause entry of %s from its pause intent: %w", slug, err))
		}
	}
	return errors.Join(failures...)
}

// pauseFromIntent pauses slug when its batten record holds a pause intent.
// It pauses only when the scope has no live entry in the pause record at recordPath.
func pauseFromIntent(prime *lymcwd.Location, deps tenderengine.Deps, recordPath, slug string) error {
	status, found, err := pairteardown.ReadBattenRecord(prime, slug)
	if err != nil {
		return err
	}
	if !found || status.PauseIntent == nil {
		return nil
	}
	record, err := pauserecord.Read(recordPath)
	if err != nil {
		return err
	}
	if slices.ContainsFunc(record.Live, func(entry pauserecord.Entry) bool { return entry.Scope == slug }) {
		return nil
	}
	_, err = tenderengine.Pause(deps, tenderengine.PauseRequest{Slug: slug, Grace: pauseGrace})
	return err
}

// ownerElsewhere returns the owner and the adopt subject of slug when another machine owns it.
// It pulls the Bolt first and reads the local Bolt when origin is unreachable.
// Any other failed pull is returned with the way forward of bringing the board level.
// A hub with no Bolt checkout has no owners.
func ownerElsewhere(prime *lymcwd.Location, slug string) (owner, subject string, err error) {
	boltDir := fabricengine.BoltDir(prime.HubPath)
	if _, err := os.Stat(boltDir); errors.Is(err, os.ErrNotExist) {
		return "", "", nil
	}
	if err := boardengine.PullBolt(boltDir); err != nil && !errors.Is(err, boardengine.ErrOwnerOriginUnreachable) {
		return "", "", fmt.Errorf("pause: read who owns %s: %w; way forward: bring the board level as it names, then run the verb again", slug, err)
	}
	here, err := machinename.ReadIdentity()
	if err != nil {
		return "", "", err
	}
	owners, err := boardengine.ReadOwners(fabricengine.BoardDir(prime.HubPath))
	if err != nil {
		return "", "", err
	}
	parentOf := func(entry string) (string, error) { return hubgeom.ParentOf(prime, entry) }
	effective, err := owners.Effective(slug, parentOf)
	if err != nil {
		return "", "", err
	}
	if !boardengine.OwnedElsewhere(effective, owners.Registry, here) {
		return "", "", nil
	}
	subject, err = owners.AdoptSubject(slug, parentOf)
	if err != nil {
		return "", "", err
	}
	return effective, subject, nil
}

// boltHoldState reads the adopt hold the hub's Bolt records, after a pull.
// An unreachable origin reads the local Bolt.
// Any other failed pull is returned naming its detail.
// A hub with no Bolt checkout records no hold.
// The fix task is the one an adopt-fix hold stands over, and only while no other hold holds it.
func boltHoldState(hubPath string) (tenderengine.BoltHoldState, error) {
	boltDir := fabricengine.BoltDir(hubPath)
	if _, err := os.Stat(boltDir); errors.Is(err, os.ErrNotExist) {
		return tenderengine.BoltHoldState{}, nil
	}
	if err := boardengine.PullBolt(boltDir); err != nil && !errors.Is(err, boardengine.ErrOwnerOriginUnreachable) {
		return tenderengine.BoltHoldState{}, fmt.Errorf("the board could not be pulled: %w", err)
	}
	owners, err := boardengine.ReadOwners(fabricengine.BoardDir(hubPath))
	if err != nil {
		return tenderengine.BoltHoldState{}, err
	}
	if len(owners.Holds) == 0 {
		return tenderengine.BoltHoldState{}, nil
	}
	state := tenderengine.BoltHoldState{Standing: true, Machine: owners.Holds[0].Machine}
	if fixTask := owners.Holds[0].FixTask; fixTask != "" {
		if _, held := owners.HoldOf(fixTask); !held {
			state.FixTask = fixTask
		}
	}
	return state, nil
}

// commitRunIntent runs write over the run of slug, then commits the run's batten record under message.
// It then pushes the prime's records side, held under slug, and waits for the push.
// A slug with no batten record writes and commits nothing.
// A push that is owed is answered on stderr and leaves the commit for the next push of the prime's records branch.
func commitRunIntent(prime *lymcwd.Location, slug, message string, stderr io.Writer, write func() (found bool, err error)) error {
	if _, err := os.Stat(shedrun.StatusFile(prime, slug)); errors.Is(err, os.ErrNotExist) {
		return nil
	}
	active, err := fabricengine.MergeStateActive(prime)
	if err != nil {
		return fmt.Errorf("read the prime's records merge state: %w", err)
	}
	if active {
		return errors.New("a merge is in progress on the prime's records side; conclude or abort that merge there")
	}
	if err := os.MkdirAll(filepath.Dir(shedrun.StatusLock(prime, slug)), 0o755); err != nil {
		return err
	}
	found, err := write()
	if err != nil || !found {
		return err
	}
	_, committed, err := fabricengine.CommitAnchoredPaths(fabricengine.NewMutations(""), prime, []string{shedrun.StatusRel(prime, slug)}, message, fabricengine.EnvSyncOptions())
	if err != nil || !committed {
		return err
	}
	_, err = pairteardown.PushRun(prime, prime, slug, pairteardown.RunPush{LockWait: fabricengine.StatusPushLockWait})
	return owedIntentPush(slug, err, stderr)
}

// owedIntentPush answers the error of a run intent's synchronous push.
// No error answers nil.
// A push that is owed writes one line to stderr naming slug, the push's error and how the intent lands, and answers nil:
// the verb has acted here already, and it owes its push.
// Any other push error is returned, so the verb fails naming it.
func owedIntentPush(slug string, err error, stderr io.Writer) error {
	if err == nil {
		return nil
	}
	if !fabricengine.IsPushOwed(err) {
		return err
	}
	fmt.Fprintf(stderr, "tender: the push of the intent of %s is owed: %v; the intent lands with the prime's records branch's next push, or run `lym fabric push` from the prime\n", slug, err)
	return nil
}
