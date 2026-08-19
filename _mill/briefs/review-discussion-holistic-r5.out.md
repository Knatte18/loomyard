MILL_REVIEW_BEGIN
# Review: landing: Publish + Finalize producers

```yaml
verdict: REQUEST_CHANGES
reviewer_model: opusmedium
reviewer_self_id: Claude (Opus-class, Anthropic)
reviewed_file: _mill/discussion.md
date: 2026-08-19
```

## Findings

### [BLOCKING:design] "Stuck with a distinct message" has no carrier
**Section:** `publish-resume-reads-pr-state`, `publish-repo-resolution`, `fabric-handles-are-injected-closures`, Testing
**Issue:** `shedengine.ShedProducer.Call` returns only `(Outcome, OutputPointer, error)` (`internal/shedengine/producer.go:31`) and `run.go:190-200` persists the fixed reason `"stuck with no OnStuck target"` with `StateBlocked` — a returned error is `StateFailed` instead, so nothing carries a producer-supplied stuck message anywhere; yet the discussion specifies four distinguishable `Stuck` messages (PR open vs closed-unmerged, absent/unparseable `origin`, no live parent pair, `worktree dirty` verbatim) and lists tests asserting they are distinguishable.
**Fix:** Name the carrier explicitly — `logger.Warn` as `internal/shedadapters/singlellm.go:104` already does for its non-Done outcomes, and/or a `OutputPointer.Path` report file — and restate the affected tests against that carrier rather than against a nonexistent reason field.

### [BLOCKING:scope] PushWarpAt's documented precondition is not in scope
**Section:** `publish-repo-resolution` ("`Publish` pushes the warp branch")
**Issue:** `internal/fabricengine/spawn.go:81-88` states `PushWarpAt` has **no** production caller and that wiring it into a live path leaves `gitrepo.PushLockFileName` (`.gitrepo-push.lock`) as untracked residue in the operator's warp repo — "Any future caller must seed the warp-side exclude first"; `seedGitExclude` (`junction.go:673`) seeds junction names only, unlike `seedWeftArtifactExcludes` (`weftgit.go:85`) which does list `gitrepo.PushLockFileName`. The Scope section adds no such work item.
**Fix:** Add the warp-side exclude seeding (or an explicitly-justified alternative push path) to Scope, and update `spawn.go`'s "no production caller" doc comment in the same commit.

### [BLOCKING:design] PushWarpAt's skip-gating and rebase-retry semantics unaddressed
**Section:** `publish-repo-resolution`
**Issue:** The decision passes an unspecified `opts` — `PushWarpAt` returns an empty result and nil error when `opts.SkipGit || opts.SkipPush` (`spawn.go:93-95`), so a skip silently yields a PR created for an unpushed branch (422), not the promised `Stuck`; and `PushCoalesced` → `pushWithRebaseRetry` (`push.go:43-83`) runs `git pull --rebase` on a rejected push, rewriting the warp task branch's SHAs while the weft side is not rebased.
**Fix:** State where `SyncOptions` comes from and what a skip means (refuse/`Stuck`/`Done`), and decide whether the rebase-retry path is acceptable for a warp/weft pair or must be avoided.

### [NIT:consistency] ScratchDir duplicates a value Deps already carries
**Section:** `told-values-via-landingshed-deps` / `mergeresolve-drives-shuttle-directly`
**Issue:** `Deps` carries both `AnchorPath` and `ScratchDir` (= `<AnchorPath>/.lyx/landing`), against the "no derived paths, no redundant near-duplicates" discipline the discussion itself imports from `loomshed.Deps`.
**Fix:** State why the near-duplicate is deliberate here (told-geometry beats derivation), or drop one field.

## Verdict

REQUEST_CHANGES
Stuck messaging has no seam; warp push adds an undischarged precondition and unspecified semantics.
MILL_REVIEW_END
