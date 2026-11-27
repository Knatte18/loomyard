// runintents.go fills the seams of the pause and resume verbs.
// They record a run's pause intent.
// The intent lives in the run's batten record on the prime, which is committed and pushed with the run's other records.

package tendercli

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/Knatte18/loomyard/internal/agentname"
	"github.com/Knatte18/loomyard/internal/boardengine"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/hubgeom"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/machinename"
	"github.com/Knatte18/loomyard/internal/pairteardown"
	"github.com/Knatte18/loomyard/internal/runfsm"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shedrun"
)

// resumeAside is what a resume sets aside before the act rule decides.
// The resume lifts its own pause entry and drives nothing.
// A pause, a cancel under way and a shed pause therefore do not stop it, while a finished cancel still reads cancelled.
// It acts offline too, so an owed push does not stop it.
var resumeAside = runfsm.Aside{Pause: true, UnfinishedCancel: true, ShedPause: true, PushOwed: true}

// runIntents holds the seams one run's pause and resume verbs ask, over the hub's prime.
type runIntents struct {
	// Owner returns the effective owner and the adopt subject when another machine owns the run, empty strings otherwise.
	Owner func() (owner, subject string, err error)
	// Record writes the pause intent into the run's batten record, commits it and spawns the push of the records side.
	Record func() error
	// Verdict returns the act rule's answer over the run, with what a resume sets aside cleared.
	Verdict func() (runfsm.Verdict, error)
	// Clear removes the pause intent from the run's batten record, commits it and spawns the push of the records side.
	Clear func() error
}

// hubRunIntents fills the seams of the run of slug over the prime of the hub location belongs to.
func hubRunIntents(location *lymcwd.Location, slug string) (runIntents, error) {
	primeName, err := fabricengine.PrimeName(location)
	if err != nil {
		return runIntents{}, fmt.Errorf("resolve the hub's prime: %w", err)
	}
	prime, err := lymcwd.ResolveWorktree(filepath.Join(location.HubPath, primeName))
	if err != nil {
		return runIntents{}, fmt.Errorf("locate the hub's prime %q: %w", primeName, err)
	}
	by := os.Getenv(agentname.StrandNameEnv)
	if by == "" {
		by = agentname.External
	}
	return runIntents{
		Owner: func() (string, string, error) { return ownerElsewhere(prime, slug) },
		Record: func() error {
			intent := shedengine.PauseIntent{By: by, At: time.Now().Format(time.RFC3339)}
			return commitRunIntent(prime, slug, "tender pause: record the pause intent of "+slug, func() (bool, error) {
				return shedrun.WritePauseIntent(prime, slug, intent)
			})
		},
		Verdict: func() (runfsm.Verdict, error) {
			records, inputs, err := pairteardown.ReadRun(prime, slug)
			if err != nil {
				return runfsm.Verdict{}, err
			}
			_, verdict := runfsm.Judge(records, inputs, resumeAside)
			return verdict, nil
		},
		Clear: func() error {
			return commitRunIntent(prime, slug, "tender resume: clear the pause intent of "+slug, func() (bool, error) {
				return shedrun.ClearPauseIntent(prime, slug)
			})
		},
	}, nil
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
	here, err := machinename.Name()
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
	if !boardengine.OwnedElsewhere(effective, here) {
		return "", "", nil
	}
	subject, err = owners.AdoptSubject(slug, parentOf)
	if err != nil {
		return "", "", err
	}
	return effective, subject, nil
}

// commitRunIntent runs write over the run of slug, then commits the run's batten record under message.
// It spawns the push of the prime's records side, held under slug.
// A slug with no batten record writes and commits nothing.
// A push that fails to start is logged, since the next push carries the commit.
func commitRunIntent(prime *lymcwd.Location, slug, message string, write func() (found bool, err error)) error {
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
	if _, err := fabricengine.SpawnStatusPush(prime, slug); err != nil {
		logger.Warn("tendercli: the push of the pause intent did not start; the next push carries it", "slug", slug, "error", err)
	}
	return nil
}
