MILL_REVIEW_BEGIN
# Review: gitexec: decide whether RunGit should return a typed error carrying stderr

```yaml
verdict: REQUEST_CHANGES
reviewer_model: opusmedium
reviewer_self_id: Claude (Anthropic), Opus-class model
reviewed_file: _mill/discussion.md
date: 2026-08-10
```

## Findings

### [BLOCKING:design] Merge rule ignores the `(git exit %d)` fragment
**Section:** the-migration-is-a-two-message-merge-not-a-substitution / migration recipe
**Issue:** Nearly every exit-path message in the tree embeds the code as well as stderr — `fmt.Errorf("warp switch to branch %q failed (git exit %d): %s", branch, exitCode, strings.TrimSpace(switchStderr))` (`internal/fabricengine/checkout.go:95`; same shape at `remove.go:67`, `add.go:203`, `clone.go:528`, `weftwiring.go:116`, `reconcile.go:470`, `status.go:190`, `cleanup.go:243/282`) — and `gitexec.Run` deletes the `exitCode` binding the `%d` consumes, so the rule "%s-of-stderr becomes %w-of-error" leaves the fragment unfillable and duplicated against `GitError.Error()`'s own `exit <code>`.
**Fix:** State in the merge rule that the `(git exit %d)` fragment is dropped along with its argument, since `GitError` now carries the code.

### [BLOCKING:design] Merge rule silent on sentinel-returning exit paths
**Section:** the-migration-is-a-two-message-merge-not-a-substitution
**Issue:** The rule assumes both branches carry a *message*, but `internal/lyxcwd/lyxcwd.go:151` returns a bare sentinel (`return "", ErrNotAGitRepo`) while `:149` wraps it; `%w`-wrapping the `GitError` instead would break `errors.Is(err, lyxcwd.ErrNotAGitRepo)` at `internal/loomengine/preflight.go:46` and the exact-string assertion at `internal/lyxcwd/lyxcwd_test.go:127`.
**Fix:** Add a sentinel-preserving clause (sentinel identity survives; wrap as `fmt.Errorf("%w: %v", Sentinel, gitErr)` or leave bare) and state the raw-vs-checked disposition for each of the four named outside-fabric sites.

### [BLOCKING:consistency] Predicate shape list double-counts two sites
**Section:** The predicate-site inventory
**Issue:** `warpprobe.go:77` is the `RunGit` call whose comparison is `:81`, so listing both makes one site two and contradicts "`:81` … is the only predicate in the file"; likewise `weftwiring.go:90` (call at `:89-92`) and `weftwiring.go:96` (its `return exitCode == 0`) are one site listed in two lists — verified in source.
**Fix:** Key every inventory entry to one line-kind (call site, not comparison) and re-state the "6 rev-parse sites" / "four bare `return code == 0`" counts after de-duplication.

### [BLOCKING:scope] gitrepo discard count and its query disagree with the tree
**Section:** `gitrepo.run` — the second copy of the shape
**Issue:** The text claims six stderr-discarding `r.run` sites but names five and admits "one more" it cannot name; source shows five (`reset.go:18`, `pull.go:19`, `pull.go:33`, `push.go:133`, `ancestry.go:26`), and the stated query `grep -c '_, _, .*= r\.run('` returns four because `push.go:133` is `stdout, _, code, err`. "The remaining fifteen" is therefore also wrong (21−5=16).
**Fix:** Correct to five/sixteen and widen the query to match any `_` in the second binding position, since the verdict is to carry the query, not the count.

### [NIT:consistency] Superseded 48/15 left standing in the Q&A log
**Section:** Q&A log (orchestrator review entry)
**Issue:** One entry records "the correction is 48 error-constructing / 15 predicate" as a resolution, superseded by the round-1 entry's 51/12; a reader of the verdict draft could carry the stale pair forward.
**Fix:** Mark that entry as superseded by the 51/12 correction.

### [NIT:consistency] Lifecycle rationale quotes a line that says the opposite
**Section:** verdict-doc-lifecycle
**Issue:** The rationale cites the design doc's own status line, "Deleted once the verdict is recorded, wherever it lands" (verified at `manifest/designs/gitexec-error-shape.md:5`), as supporting keeping the doc alive as the recorded verdict — that line reads as delete-on-verdict.
**Fix:** Justify the retention on the Documentation Lifecycle rule alone, or note that the status line is being amended by this task.

## Verdict

REQUEST_CHANGES
Merge rule has two unhandled site classes; two inventories miscount against the tree.
MILL_REVIEW_END
