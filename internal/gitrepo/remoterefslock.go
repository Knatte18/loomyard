// remoterefslock.go holds the remote-refs lock: one lock per repository, in its common git dir, that every lym git call writing the repository's remote-tracking refs holds while git runs.
// A push's transfer writes none and runs outside it; only the push's tracking-ref write that follows takes it.

package gitrepo

import (
	"fmt"
	"path/filepath"

	"github.com/Knatte18/loomyard/internal/lock"
	"github.com/Knatte18/loomyard/internal/pauserecord"
)

// RemoteRefsLockFileName names the remote-refs lock file in a repository's common git dir.
// A fetch and a push both write the remote-tracking refs every worktree of the repository shares, so two such calls racing on one ref make the loser fail with git's "incorrect old value provided" or "cannot lock ref".
const RemoteRefsLockFileName = "lym-remote-refs.lock"

// remoteRefsGateOp names the in-flight operation the pause gate marks while a process wants or holds the remote-refs lock.
const remoteRefsGateOp = "remote-refs"

// HoldRemoteRefs takes the remote-refs lock of the repository this checkout belongs to, waiting while another holder has it, and returns its release.
// It passes the process's pause gate before it takes the lock and stays admitted until the release,
// so a pause never holds a git call made under the lock while other processes wait on it.
// A holder that reads origin and then judges the tracking refs sees no fetch and no push's tracking-ref write in between.
// The lock is not reentrant: a holder runs no fetch, pull or push through this package, since each takes the lock itself.
func (r *Repo) HoldRemoteRefs() (release func() error, err error) {
	commonDir, err := r.CommonDir()
	if err != nil {
		return nil, fmt.Errorf("gitrepo: hold the remote-refs lock of %s: %w", r.path, err)
	}
	leaveGate := pauserecord.Admit(remoteRefsGateOp)
	held, err := lock.AcquireWriteLock(filepath.Join(commonDir, RemoteRefsLockFileName))
	if err != nil {
		leaveGate()
		return nil, fmt.Errorf("gitrepo: hold the remote-refs lock of %s: %w", r.path, err)
	}
	return func() error {
		err := held.Release()
		leaveGate()
		return err
	}, nil
}

// withRemoteRefs runs call under the remote-refs lock, releasing it once call returns.
func (r *Repo) withRemoteRefs(call func() error) error {
	release, err := r.HoldRemoteRefs()
	if err != nil {
		return err
	}
	defer func() { _ = release() }()
	return call()
}

// FetchWithArgs runs `git fetch` with args under the remote-refs lock and returns git's stdout.
// It is the fetch for a caller naming its own remote and refspecs; a non-zero exit returns *gitexec.GitError.
func (r *Repo) FetchWithArgs(args ...string) (stdout string, err error) {
	err = r.withRemoteRefs(func() error {
		stdout, err = r.runChecked(append([]string{"fetch"}, args...)...)
		return err
	})
	return stdout, err
}
