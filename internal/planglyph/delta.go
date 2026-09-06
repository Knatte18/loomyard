// delta.go implements Delta, this package's one and only call site of (*quarry.Repo).DeltaGit —
// the batch's own record-batch re-resolution boundary reads a real OS-process git spawn through
// this one function, so it is the one place that spawn needs to be logged and the one place its
// error needs to be folded into ErrQuarryUnavailable.

package planglyph

import (
	"fmt"

	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/quarry/quarry"
)

// Delta opens a quarry.Repo rooted at worktreeRoot and answers a whole-repository DeltaGit query
// between fromSHA and toSHA, via the fixed "." target: the scope guard (card 35) needs the
// whole-repo delta anyway, and narrowing the range would blind the one consumer that exists to
// catch out-of-scope changes.
//
// DeltaGit starts a real OS process (quarry's own internal/gitsrc runs exec.Command("git", ...)),
// so the spawn is logged at Info immediately before the call — a lifecycle spawn, per the
// Live-Substrate Spawn Observability invariant, not a spawn inside a polling probe. There is no
// teardown to log separately: DeltaGit waits internally and returns an answer, so the spawn's own
// log line is the whole of this function's observability obligation.
//
// A non-nil error is wrapped with ErrQuarryUnavailable, exactly as openRepo and resolveTargets
// wrap theirs, so a DeltaGit failure reads as the same infrastructure category as an Open or
// Resolve failure and can never be mistaken for an empty delta.
func Delta(worktreeRoot, fromSHA, toSHA string) (quarry.GitDeltaAnswer, error) {
	repo, err := openRepo(worktreeRoot)
	if err != nil {
		return quarry.GitDeltaAnswer{}, err
	}

	logger.Info("planglyph: spawning git for DeltaGit", "worktree_root", worktreeRoot, "from_sha", fromSHA, "to_sha", toSHA)
	answer, err := repo.DeltaGit(fromSHA, toSHA, ".")
	if err != nil {
		return quarry.GitDeltaAnswer{}, fmt.Errorf("%w: delta %s..%s: %v", ErrQuarryUnavailable, fromSHA, toSHA, err)
	}
	return answer, nil
}
