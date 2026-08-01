MILL_REVIEW_BEGIN
# Review: fabric: warp-rebase / remote-reconcile recovery — holistic

```yaml
verdict: REQUEST_CHANGES
reviewer_model: sonnetxhigh
reviewer_self_id: Claude Sonnet 4.5 (Claude Opus/Sonnet family, best-effort self-assessment)
reviewed_file: plan/
date: 2026-08-01
```

## Findings

### [BLOCKING] Card 7 missing index.go from Context
**Location:** 02-fabric-pull.md, Card 7 (PullResult, PartialPullError, and sentinel errors)
**Issue:** Requirements says the two new sentinel errors must match "the `ErrStaleSHA`/`ErrNoCorrespondence` style" — both are defined (with their full doc comments) in `internal/fabricengine/index.go`, which is not listed in Card 7's `Context:` (only `commit.go`, `revert.go`, `fabric.go`). `revert.go` merely *uses* the two sentinels, it does not define them.
**Fix:** Add `internal/fabricengine/index.go` to Card 7's `Context:` list.

### [BLOCKING] Card 12 leaves a stale, directly-contradicted SHAExists claim unfixed
**Location:** 04-docs-sandbox.md, Card 12, part (2); target file `manifest/designs/fabric-unified-view.md`
**Issue:** Card 12 only directs fixing the slice-6 line ("Detection (`SHAExists`)", line 113). It misses the more prominent claim two sections earlier — "## Warp-rebase and remote-reconcile" — line 79: "**Detection is already honest and shipped.** After a warp rewrite, weft's `Warp-SHA` trailers point at warp SHAs that no longer exist; `SHAExists` catches this rather than trusting a dead reference." This is flatly false and is exactly the misconception the plan's own "reachability, never object-existence" Shared Decision exists to correct (`git fetch` never prunes objects, so `SHAExists` reports `true` post-fetch on a rebased-away commit — detection never fires). Landing the doc update without fixing this leaves a load-bearing, actively-misleading claim in the same file being edited in the same commit.
**Fix:** Extend Card 12 part (2) to also correct (or strike) line 79's "Detection is already honest and shipped" paragraph in `fabric-unified-view.md`.

### [NIT] Card 9's git-log record-separator placement likely misaligns SHA/paths
**Location:** 02-fabric-pull.md, Card 9 (PATTERN residue enumeration)
**Issue:** The card directs `git log --name-only --format=<sep-format> ...`, reusing `scanWarpSHATrailers`'s idiom of a *trailing* record separator. With `--name-only`, git appends each commit's changed-file list as separate lines *after* the format output, so a trailing record separator lands between a commit's SHA and its own file list, not between commits — the opposite of what `scanWarpSHATrailers` achieves (which has no `--name-only` component). As literally described this would misassign paths to the wrong commit.
**Fix:** Note that the record separator likely needs to *lead* each commit's block (not trail it) when combined with `--name-only`; call this out explicitly, or leave it to the implementer with a pointer to verify empirically (card 10's integration test would catch the failure either way).

### [NIT] PATTERN residue pathspec is not RelPath-scoped
**Location:** 02-fabric-pull.md, Card 9
**Issue:** The card scopes `git log -- <patternDir>` to a bare `hubgeometry.PatternDirName` ("_pattern") at the weft repo root. `hubgeometry.Layout.WeftPatternDir()` shows the real on-disk location is `RelPath/_pattern`, and fabricengine's own doc.go notes multiple hubs at different `RelPath` depths can share one weft checkout. For a subpath-anchored hub this pathspec would silently match nothing. This mirrors the existing "relpath-is-dot-for-slice-2" simplification already accepted in `Fabric.Commit`, so it may be intentional scope, but the plan doesn't say so.
**Fix:** Either scope the pathspec through the resolved `Layout.RelPath` or add a one-line note documenting the RelPath-blind limitation, consistent with the existing precedent.

### [NIT] Card 3's gogit.go comment fix perpetuates a pre-existing inaccuracy
**Location:** 01-gitrepo-primitives.md, Card 3
**Issue:** `internal/gitrepo/gogit.go`'s locking-discipline comment lists `hasUnpushed` among the go-git object-lookup methods routed through `lookupObjectRetrying`. This is already wrong today — `hasUnpushed`/`HasUnpushed` is CLI-bound (`r.run`), took no go-git handle, and is excluded from that locking discipline entirely (confirmed by the boundary guard's pinned `r.run`-bound set). Card 3 only asks for a case-only rename here, leaving the substantive error uncorrected.
**Fix:** Either drop `HasUnpushed` from that parenthetical list entirely, or note the fix should be content-level, not just case-level.

### [NIT] Fetch has no explicit error-path test
**Location:** 01-gitrepo-primitives.md, Card 1
**Issue:** `Pull` has existing no-remote-configured / diverged-refusal error-path tests; Card 1 only requires one positive-path scenario for `Fetch` (`fetch_integration_test.go`), despite Fetch following "Pull's exact error style" (no-stderr-leak, wrapped spawn error).
**Fix:** Add a small error-path case (e.g., no remote configured) mirroring `TestPull_NoRemoteConfigured_ErrorNamesRepoPath`.

## Verdict

REQUEST_CHANGES
Two Context-completeness/doc-accuracy BLOCKING gaps (Card 7, Card 12); design and sequencing otherwise sound.
MILL_REVIEW_END
