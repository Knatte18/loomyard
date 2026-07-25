// syncweft.go — SyncWeft, the canonical synchronous coordinated weft-sync
// operation: commit-with-trailer, in-process push, and a post-push
// correspondence record. Unlike CommitWeft's own pre-push record (the
// detached CLI push path's self-correcting entry), SyncWeft re-reads the
// weft SHA after a successful push and records that instead, since a
// rebase-recovered push rewrites the local commit it replayed.

package fabricengine

import "fmt"

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
// record named; without the re-read, the index would point at an
// off-history commit. Synchronous by design — this is the path a caller
// willing to wait on a real push uses; the detached CLI path (a later
// batch) uses CommitWeft plus a spawned PushWeftAt instead.
func (f *Fabric) SyncWeft(message string, pathspec []string, opts SyncOptions) (SyncResult, error) {
	sha, committed, err := f.CommitWeft(pathspec, message, opts)
	if err != nil {
		return SyncResult{}, err
	}
	if !committed {
		return SyncResult{Committed: false}, nil
	}

	warpSHA, err := f.Warp.CurrentSHA()
	if err != nil {
		return SyncResult{}, fmt.Errorf("fabricengine: warp CurrentSHA: %w", err)
	}

	if opts.SkipGit || opts.SkipPush {
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

	if err := f.RecordCorrespondence(warpSHA, postPushSHA); err != nil {
		return SyncResult{}, err
	}

	return SyncResult{Committed: true, Pushed: true, WarpSHA: warpSHA, WeftSHA: postPushSHA}, nil
}
