// syncweft.go — SyncWeft, the canonical synchronous coordinated weft-sync
// operation: commit-with-trailer, in-process push, and a post-push
// correspondence record. Unlike CommitWeft's own pre-push record (the
// detached CLI push path's self-correcting entry), SyncWeft re-reads the
// weft SHA after a successful push and records that instead, since a
// rebase-recovered push rewrites the local commit it replayed. The warp SHA
// it reports and records is parsed back out of the commit's own Warp-SHA
// trailer — never re-read from warp HEAD — so the index entry agrees with
// the trailer by construction.

package fabricengine

import (
	"fmt"

	"github.com/Knatte18/loomyard/internal/gitexec"
)

// SyncResult reports what SyncWeft actually did: whether a commit was made,
// whether it was pushed, and the warp/weft SHAs the sync ended up at. JSON
// tags are snake_case for fabriccli's envelope (a later batch) to serialize
// directly.
type SyncResult struct {
	Committed bool   `json:"committed"`
	Pushed    bool   `json:"pushed"`
	WarpSHA   string `json:"warp_sha"`
	WeftSHA   string `json:"weft_sha"`
}

// SyncWeft is fabric's canonical coordinated weft-sync operation: it commits
// pathspec-scoped weft changes with a Warp-SHA trailer (via CommitWeft,
// which also records the pre-push correspondence), and — when something was
// committed and neither opts.SkipGit nor opts.SkipPush is set — pushes it
// in-process and re-records the correspondence against the post-push weft
// SHA. The re-read matters because a push that recovers via rebase (gitrepo's
// documented Push contract) rewrites the local commit CommitWeft's pre-push
// record named. The warp SHA in the result and the post-push record is
// parsed out of the weft commit's own Warp-SHA trailer rather than re-read
// from warp HEAD: warp can advance between CommitWeft's read and a second
// read, and the trailer — the sole source of truth for correspondence — must
// never disagree with the index entry recorded beside it. Synchronous by
// design — this is the path a caller willing to wait on a real push uses;
// the detached CLI path (a later batch) uses CommitWeft plus a spawned
// PushWeftAt instead.
func (f *Fabric) SyncWeft(message string, pathspec []string, opts SyncOptions) (SyncResult, error) {
	sha, committed, err := f.CommitWeft(pathspec, message, opts)
	if err != nil {
		return SyncResult{}, err
	}
	if !committed {
		return SyncResult{Committed: false}, nil
	}

	if opts.SkipGit || opts.SkipPush {
		warpSHA, err := f.warpSHAFromTrailer(sha)
		if err != nil {
			return SyncResult{}, err
		}
		return SyncResult{Committed: true, Pushed: false, WarpSHA: warpSHA, WeftSHA: sha}, nil
	}

	if err := f.Weft.Push(); err != nil {
		return SyncResult{}, err
	}

	// Re-read: a rebase-recovered push rewrites the local commit sha named
	// above, so the SHA recorded here must come from after the push, never
	// from CommitWeft's return value.
	postPushSHA, err := f.Weft.CurrentSHA()
	if err != nil {
		return SyncResult{}, fmt.Errorf("fabricengine: weft CurrentSHA after push: %w", err)
	}

	// The warp SHA comes from the pushed commit's own trailer (a rebase
	// replay preserves the message verbatim), so the record below and the
	// trailer can never disagree.
	warpSHA, err := f.warpSHAFromTrailer(postPushSHA)
	if err != nil {
		return SyncResult{}, err
	}

	if err := f.RecordCorrespondence(warpSHA, postPushSHA); err != nil {
		return SyncResult{}, err
	}

	return SyncResult{Committed: true, Pushed: true, WarpSHA: warpSHA, WeftSHA: postPushSHA}, nil
}

// warpSHAFromTrailer parses the Warp-SHA trailer value out of the weft
// commit weftSHA's own message. SyncWeft uses it to report and record the
// warp SHA the commit actually names, instead of re-reading warp HEAD and
// risking a value the trailer disagrees with. A weft commit SyncWeft just
// produced always carries the trailer (CommitWeft wrote it), so a missing
// trailer is a genuine invariant violation, not a tolerable state.
func (f *Fabric) warpSHAFromTrailer(weftSHA string) (string, error) {
	stdout, stderr, code, err := gitexec.RunGit([]string{"show", "-s", "--format=%B", weftSHA}, f.weftPath)
	if err != nil {
		return "", fmt.Errorf("fabricengine: read weft commit message: %w", err)
	}
	if code != 0 {
		return "", fmt.Errorf("fabricengine: git show -s %s in %s: %s", weftSHA, f.weftPath, stderr)
	}

	warpSHA, ok := parseWarpSHATrailer(stdout)
	if !ok {
		return "", fmt.Errorf("fabricengine: weft commit %s carries no %s trailer", weftSHA, WarpSHATrailerKey)
	}
	return warpSHA, nil
}
