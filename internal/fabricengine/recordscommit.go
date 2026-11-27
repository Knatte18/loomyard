// recordscommit.go guards the records commits of a one-repository hub.
// A records commit there lands on the worktree's own code branch, beside the agent's work and other git operations.
// The file holds the retry over a held `index.lock`,
// and the refusal of a records commit in a prime that is off its branch.

package fabricengine

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/Knatte18/loomyard/internal/dotgit"
	"github.com/Knatte18/loomyard/internal/gitexec"
	"github.com/Knatte18/loomyard/internal/gitrepo"
)

const (
	// indexLockRetryBound is how long a records commit waits out a held `index.lock` before it fails.
	indexLockRetryBound = 30 * time.Second
	// indexLockFirstDelay is the first sleep between attempts; each later sleep doubles it.
	indexLockFirstDelay = 50 * time.Millisecond
	// heldIndexLockMessage is the text git's refusal to take a held `index.lock` carries.
	heldIndexLockMessage = "index.lock': File exists"
)

// retryHeldIndexLock runs commit and returns its error.
// While the error names a held `index.lock`, it sleeps and runs commit again.
// The first sleep is 50ms and each later one doubles it.
// It stops once the slept time reaches bound and returns the last error.
// An error that does not name a held `index.lock` is returned at once.
// It sums the slept time and reads no clock, so a caller drives it with a fake sleep.
func retryHeldIndexLock(bound time.Duration, sleep func(time.Duration), commit func() error) error {
	var slept time.Duration
	delay := indexLockFirstDelay
	for {
		err := commit()
		if err == nil || !strings.Contains(err.Error(), heldIndexLockMessage) || slept >= bound {
			return err
		}
		sleep(delay)
		slept += delay
		delay *= 2
	}
}

// ErrPrimeBranch refuses a records commit in the prime of a one-repository hub.
// It applies while the prime is off its recorded branch.
// The prime's records live on the branch its git config names, so a commit on any other branch would put them there.
type ErrPrimeBranch struct {
	// Worktree is the prime's path.
	Worktree string
	// Recorded is the branch the prime's config names, empty when it names none.
	Recorded string
	// CheckedOut is the branch checked out in the prime, empty for a detached HEAD.
	CheckedOut string
}

// Error implements the error interface, naming the way forward.
func (e *ErrPrimeBranch) Error() string {
	if e.Recorded == "" {
		return fmt.Sprintf("the prime at %s records no branch in %s, so its records cannot be committed; way forward: run `git config %s <branch>` in the prime, naming the branch its records live on, then run the step again",
			e.Worktree, PrimeBranchConfigKey, PrimeBranchConfigKey)
	}
	checkedOut := e.CheckedOut
	if checkedOut == "" {
		checkedOut = "a detached HEAD"
	}
	return fmt.Sprintf("the prime at %s has %s checked out, but its records live on %s; way forward: switch the prime back to %s with `git switch %s`, or `git switch --merge %s` when git refuses to carry the uncommitted records, then run the step again",
		e.Worktree, checkedOut, e.Recorded, e.Recorded, e.Recorded, e.Recorded)
}

// IsPrimeBranchRefusal reports whether err is, or wraps, a refusal of a records commit in a prime that is off its branch.
func IsPrimeBranchRefusal(err error) bool {
	var refusal *ErrPrimeBranch
	return errors.As(err, &refusal)
}

// refuseOffPrimeBranch returns an *ErrPrimeBranch when the prime of a one-repository hub is not on its recorded branch.
// It applies only to the main worktree of a one-repository hub's repository.
// A task worktree and any worktree of a two-repository hub, a records prime included, is never refused.
// It reads and mutates nothing, so a refused records commit leaves the records uncommitted on disk.
func refuseOffPrimeBranch(worktree string) error {
	if !oneRepositoryAt(worktree) {
		return nil
	}
	geometry, err := dotgit.Read(worktree)
	if err != nil {
		return fmt.Errorf("fabricengine: read the git directory of %s: %w", worktree, err)
	}
	if !sameDirectory(worktree, filepath.Dir(geometry.CommonDir)) {
		return nil
	}

	recorded, err := gitexec.Run([]string{"config", "--get", PrimeBranchConfigKey}, worktree)
	var gitErr *gitexec.GitError
	switch {
	case errors.As(err, &gitErr) && gitErr.ExitCode == 1:
		recorded = ""
	case err != nil:
		return fmt.Errorf("fabricengine: read %s in %s: %w", PrimeBranchConfigKey, worktree, err)
	}
	recorded = strings.TrimSpace(recorded)

	checkedOut, detached, err := gitrepo.New(worktree).HeadRef()
	if err != nil {
		return fmt.Errorf("fabricengine: read the checked-out branch of %s: %w", worktree, err)
	}
	if detached {
		checkedOut = ""
	}
	if recorded != "" && recorded == checkedOut {
		return nil
	}
	return &ErrPrimeBranch{Worktree: worktree, Recorded: recorded, CheckedOut: checkedOut}
}

// commitMissingAnchorMarker commits the marker a worktree lacks, as the records commit of an existing worktree.
// It refuses first in a prime that is off its recorded branch, and waits out a held `index.lock`.
// A held-lock failure commits nothing, so the re-run commits the marker once.
func commitMissingAnchorMarker(rec *Mutations, weftPath, anchor, subject string) error {
	if err := refuseOffPrimeBranch(weftPath); err != nil {
		return err
	}
	return retryHeldIndexLock(indexLockRetryBound, time.Sleep, func() error {
		_, err := commitAnchorMarker(rec, weftPath, anchor, subject)
		return err
	})
}
