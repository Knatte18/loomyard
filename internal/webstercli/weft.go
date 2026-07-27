// weft.go implements weftCommit, the package-local helper every webster verb
// that reaches a batch-boundary commit point calls to stage, commit, and
// push the webster artifacts it just wrote (state.json, a batch report,
// outcome.yaml) through the weft junction -- copied verbatim from
// buildercli's own weftCommit (internal/buildercli/weft.go), including its
// lock-exclusion rationale: lock files (run.lock, mutate.lock -- advisory
// OS locks) are machine-local runtime artifacts, not webster state, so
// committing them would leak runtime noise into durable weft history and
// materialize stale lock files on every other machine's weft pull. Adapted
// for webster with one addition buildercli has no analog for: webster's
// rendered fork prompts (_lyx/webster/prompts/*) are machine-local
// re-renderable artifacts (BeginBatch rewrites each batch's own the next
// time it begins) -- committing them would be weft noise and a
// cross-machine confusion, the same class of exclusion as the pause flag,
// so they are excluded here too.
package webstercli

import (
	"fmt"
	"path"
	"path/filepath"

	"github.com/Knatte18/loomyard/internal/builderengine"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/hubgeometry"
	"github.com/Knatte18/loomyard/internal/websterengine"
)

// websterWeftPathspec returns the scoped _lyx pathspec every webster weft
// commit stages under, with the machine-local runtime artifacts excluded:
// any *.lock file (run.lock, mutate.lock -- advisory OS locks), the pause
// flag (_lyx/webster/<websterengine.PauseFlagName> -- webster's own
// webster-local pause-flag mechanics, per the builder-is-frozen-copy-not-move
// decision), and every rendered fork prompt
// (_lyx/webster/prompts/*). All three are per-machine or purely-derived
// runtime state, never durable webster state, so committing them would leak
// runtime noise into weft history and materialize on every other machine's
// weft pull -- the pause flag in particular could read as a spurious pause
// request elsewhere (it is present on disk during record-batch's terminal
// commit whenever a pause raced the last in-flight batch). Extracted from
// weftCommit so the exclusion set is asserted directly by a unit test rather
// than only implicitly through a live commit.
//
// The exclusion set is deliberately NOT limited to webster's own artifacts.
// Webster and builder are two round-loop drivers sharing one _lyx tree, so a
// webster weft commit stages whatever builder happens to have left on disk,
// and builder's pause flag is the same class of machine-local state as
// webster's own. Leaving it in is not merely noise in one commit: once the
// flag is tracked, the module that OWNS it can never stage its own deletion
// -- that module's exclusion entry removes the path from `git add`'s
// consideration -- so the flag is pinned in weft HEAD, pushed, and
// materialized by every other machine's weft pull as a pause request nobody
// made. The *.lock exclusion below is already cross-module (a git pathspec
// "*" crosses "/", so "<base>/*.lock" catches builder's locks too); only the
// pause flag needed naming explicitly.
//
// Every exclusion is ANCHORED under the same scoped base the positive
// pathspec names, and spelled with forward slashes (a git pathspec is not an
// OS path). A leading-wildcard exclusion such as ":(exclude)*.lock" is NOT
// equivalent and must never be reintroduced: git classifies a pattern that
// begins with "*" and carries no further wildcard as a one-star pathspec,
// which then false-positive-matches every intermediate directory git has to
// descend through to reach a multi-segment positive pathspec. At a
// layout.RelPath of two or more segments that prunes the whole subtree, so
// `git add` stages nothing at all, the staged-diff check reports "nothing to
// commit", and the weft commit silently becomes a no-op with no error for the
// caller to see. Within the anchored base "*" still crosses "/", so
// "<base>/*.lock" keeps catching lock files at any depth beneath it.
//
// The ":(exclude)" entries are git pathspec magic, carried by fabricengine's
// CommitWeft pathspec parameter end-to-end through add, the staged-diff
// check, and the pathspec-scoped commit -- gitrepo's "plain relative paths"
// rule governs its own direct consumers, not CommitWeft callers.
func websterWeftPathspec(layout *hubgeometry.Layout) []string {
	base := weftPathspecBase(layout)
	return append(
		fabricengine.ScopedPathspec(layout.RelPath, []string{hubgeometry.LyxDirName}),
		":(exclude)"+base+"/*.lock",
		":(exclude)"+base+"/webster/"+websterengine.PauseFlagName,
		":(exclude)"+base+"/webster/prompts/*",
		":(exclude)"+base+"/builder/"+builderengine.PauseFlagName,
	)
}

// weftPathspecBase returns the scoped _lyx base every exclusion entry in
// websterWeftPathspec anchors under, as a git pathspec: layout.RelPath joined
// with the _lyx directory name using forward slashes, collapsing to plain
// "_lyx" at RelPath "." or "". It exists so the exclusions are anchored to
// exactly the base the positive ScopedPathspec entry names -- see
// websterWeftPathspec for why an unanchored, leading-wildcard exclusion
// silently empties the commit at a nested RelPath.
func weftPathspecBase(layout *hubgeometry.Layout) string {
	// path.Clean normalizes the empty RelPath to "." too, so the single "."
	// check covers both the worktree-root and the unset-RelPath spellings.
	rel := path.Clean(filepath.ToSlash(layout.RelPath))
	if rel == "." {
		return hubgeometry.LyxDirName
	}
	return rel + "/" + hubgeometry.LyxDirName
}

// weftCommit stages and commits every change under layout's scoped _lyx
// pathspec (excluding the machine-local *.lock files, both round-loop
// modules' pause flags, and the rendered fork prompts -- see
// websterWeftPathspec) through the weft
// junction, then pushes, using "webster: <label>" as the commit subject.
// Per fabric's CommitWeft contract, the commit also carries a
// "Warp-SHA: <host HEAD>" trailer in its own blank-line-separated paragraph
// and records a warp<->weft correspondence entry in the weft gitdir's
// index. It reports whether a commit was actually made (false when there
// was nothing staged) and any error from either the commit or the push
// step -- mirroring buildercli's weftCommit exactly.
func weftCommit(layout *hubgeometry.Layout, label string) (bool, error) {
	weftWorktree := layout.WeftWorktree()
	opts := fabricengine.EnvSyncOptions()
	pathspec := websterWeftPathspec(layout)

	// SkipGit is checked here, before fabricengine.New's stat-based path
	// validation, mirroring CommitWeft's own top-level short-circuit: the
	// CI/test bypass must never require a real weft worktree to exist on
	// disk, but New (unlike CommitWeft itself) validates both paths
	// unconditionally.
	var committed bool
	if !opts.SkipGit {
		f, err := fabricengine.New(layout.WorktreeRoot, weftWorktree)
		if err != nil {
			return false, err
		}
		// On error, committed is passed through rather than forced to false:
		// CommitWeft reports committed=true alongside a RecordCorrespondence
		// error precisely because the commit already exists on disk at that
		// point -- reporting false there would tell the caller no commit was
		// made about a commit that is real. The push is still skipped (the
		// next weft push sweeps it up), matching the pre-cutover error flow.
		if _, committed, err = f.CommitWeft(pathspec, fmt.Sprintf("webster: %s", label), opts); err != nil {
			return committed, err
		}
	}
	if err := fabricengine.PushWeftAt(weftWorktree, opts); err != nil {
		return committed, err
	}
	return committed, nil
}
