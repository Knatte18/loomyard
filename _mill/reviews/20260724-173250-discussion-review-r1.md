MILL_REVIEW_BEGIN
# Review: gitrepo: generic, repo-agnostic git primitives

```yaml
verdict: GAPS_FOUND
reviewer_model: opushigh
reviewed_file: _mill/discussion.md
date: 2026-07-24
```

## Findings

### [GAP] Coalescing pusher has no API shape
**Section:** Decisions › Push surface
**Issue:** Every other operation gets an explicit signature, but the coalescing pusher is described only behaviourally — no name, no signature, and no statement of whether the explicit file set is fixed at construction or passed per call.
**Fix:** Pin the coalescing pusher's method/type name and signature (how it receives the file set + commit message), since it is the central board-replacement deliverable a plan writer must implement.

### [GAP] .gitignore commit path conflicts with "explicit file set, never add -A"
**Section:** Decisions › Lock ownership / Push surface
**Issue:** The lock file is "auto-added to .gitignore," but board makes that effective by committing .gitignore via `add -A` (sync.go `ensureLockfilesIgnored` + `commitDirty`); with explicit-only staging the .gitignore change is never staged, leaving it undefined whether/how it is committed — and arguably unnecessary since explicit staging never picks up the lock file anyway.
**Fix:** Specify whether the coalescing pusher commits .gitignore (and via what path) or whether the auto-gitignore is dropped as redundant under explicit staging.

### [NOTE] Push rebase-retry trigger set narrower than sync.go's
**Section:** Decisions › Push surface / Technical context
**Issue:** The Push decision mirrors `git.go`'s two triggers (`non-fast-forward`/`rejected`), but board's `sync.go:pushUnpushed` also retries on `fetch first`; to "fully replace board's sync.go" the coalescing pusher must include it.
**Fix:** State the retry trigger-string set matches sync.go (include `fetch first`), not just git.go's pair.

### [NOTE] Snapshot push / plain Push behaviour with no remote unaddressed
**Section:** Decisions › Snapshot tracking / Push surface
**Issue:** `SetSnapshotSHA` and `Push` assume a configured remote, but `gitrepo` is repo-agnostic and `New(path)` validates nothing; a remote-less repo's failure mode is unspecified.
**Fix:** Note the expected behaviour (error surfaces) when no remote/upstream is configured.

## Verdict

GAPS_FOUND
Two undefined interactions (coalescing pusher API, .gitignore commit path) need resolution before planning.
MILL_REVIEW_END
