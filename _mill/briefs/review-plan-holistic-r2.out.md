MILL_REVIEW_BEGIN
# Review: gitexec: add the checked entry point and migrate the call sites — holistic

```yaml
verdict: REQUEST_CHANGES
reviewer_model: sonnetxhigh
reviewer_self_id: claude-sonnet-5 (Sonnet 5, tool-use mode)
reviewed_file: plan/
date: 2026-08-13
```

## Findings

### [BLOCKING:design] Card 20's "six wrong-string paths" premise is stale against add.go's actual source
**Location:** batch 04-fabric-destroy-caller-files, Card 20 (add.go)
**Issue:** The card states "Six paths in this file report the fixed, wrong cause `"cwd is not a valid git worktree"` for failures unrelated to the cwd" and instructs leaving those strings unchanged. `internal/fabricengine/add.go`'s current source contains no occurrence of that string anywhere — every error path already carries git's own stderr (e.g. `"create warp worktree %q for branch %q failed (git exit %d): %s"`). `add_rollback_adopt_test.go`'s own comment confirms this in the past tense ("Six paths in Add **used to** answer every RunGit failure with..."), i.e. the defect this card describes was already fixed in an earlier slice, and `TestAdd_GitFailureCarriesGitsOwnReason` now asserts the wrong string is absent.
**Fix:** Drop the stale six-wrong-string-paths clause from Card 20 (and the identical claim in `_mill/discussion.md` and `manifest/designs/gitexec-error-shape.md` if those are still consulted) — the card's plain "migrate each RunGit site to `gitexec.Run` under the normal classification rules" instruction is sufficient and doesn't need this now-false justification.

### [NIT:consistency] "deliberate-discards-migrate-as-discards" overstates how many sites carry `//nolint:errcheck`
**Location:** `00-overview.md`, Shared Decision `deliberate-discards-migrate-as-discards`
**Issue:** The decision says the four best-effort `worktree prune` discard sites (`prune.go` ×2, `reconcile.go`, `remove.go`) each have a `//nolint:errcheck` comment to delete. In the actual source only `prune.go`'s two sites carry that comment; the `reconcile.go` and `remove.go` sites already discard via `_, _, _, _ = gitexec.RunGit(...)` with no such comment. The per-batch cards (batch 4 Card 22, batch 5 Card 26) are worded correctly (Card 22 says "delete both", i.e. two, matching `prune.go`), so this doesn't mislead the implementer, but the Shared Decision's summary is inaccurate.
**Fix:** Reword the Shared Decision to say only `prune.go`'s two sites carry a `//nolint:errcheck` comment to delete; the `reconcile.go`/`remove.go` sites already discard without one and just need their own why-comment added.

## Verdict

REQUEST_CHANGES
Card 20's factual premise about add.go no longer matches the source; everything else checked out precisely against the codebase.
MILL_REVIEW_END
