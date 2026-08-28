MILL_REVIEW_BEGIN
# Review: reed: resume/down leak lock directories at the stale pre-rename session-name path

```yaml
duration_s: 158.0
verdict: REQUEST_CHANGES
reviewer_model: opusmedium
reviewer_self_id: Claude (Anthropic), Opus-class model
reviewed_file: _mill/discussion.md
date: 2026-08-28
```

## Findings

### [BLOCKING:design] Standalone geometry teller is never considered
**Section:** `### the-anchor-must-pre-exist-but-dot-lyx-need-not`, Technical context
**Issue:** `internal/standalonegeom/reedgeom.go:19-23` is a second `reedengine.Geometry` teller where `AnchorPath = stateDir` (a derived `standalonestate.Derive` path, not a worktree), wired in `burlercli/wiring.go:157` and `webstercli/wiring.go:153`; per CONSTRAINTS `Derive` creates nothing on disk, and no production `MkdirAll` of `stateDir` exists in either wiring (only `stencilstore.Reconcile`, skipped when `--stencils-dir` is passed), so `withOpLock`'s existing `MkdirAll` is what materializes it today — the new refusal would break standalone first-run and emit "this worktree was renamed" for a path that is not a worktree.
**Fix:** State the disposition for standalone mode explicitly: either the anchor-exists precondition holds there too (and name who creates `stateDir` first), or the refusal is scoped/worded so standalone first-run is unaffected.

### [NIT:consistency] "No set-hook install site anywhere" is false
**Demoted-from:** BLOCKING
**Section:** `### window-resized-hook-has-no-install-site`
**Issue:** `resizePinHookArgvs`/`installResizePinsLocked` (`internal/reedengine/windowsize.go:197-237`) issue `set-hook -w -t <window> window-resized "resize-pane …"` and are called on every successful apply (`apply.go:235`) and attach (`attach.go:144`) — the same window-scoped option `hookInstalledLocked` reads back (`reapply.go:59`), using the literal string rather than the const. The conclusion (poll-only forever) survives, because the probe demands an exact match on `resizeHookCommand`, but the stated inventory does not.
**Fix:** Correct the rationale to "the signal-`touch` hook (`resizeHookCommand`) has no install site, while the pin array occupies the same `window-resized` option", and note that contention when recording the follow-up.

### [BLOCKING:design] Stat-error semantics vs. permanent watchdog park undecided
**Section:** `### refuse-at-the-op-lock-chokepoint` / `### the-watch-loop-terminates-when-its-anchor-is-gone`
**Issue:** The predicate is described only as "does not exist" / "proven gone"; nothing says whether a non-`ErrNotExist` `Stat` failure (EACCES, EIO, a stalled network mount) maps to the same sentinel. Since that sentinel makes `watchLoop` park permanently, a transient stat error silently kills a healthy session's self-heal with one `Warn`.
**Fix:** State that only `errors.Is(err, fs.ErrNotExist)` (plus the not-a-directory case) yields the terminal sentinel, and that any other stat error is a retryable failure.

### [NIT:decision] Not-a-directory case has no stated diagnosis
**Section:** `### the-error-names-the-vanished-path-and-the-remedy`, Testing
**Issue:** Testing requires refusal when `AnchorPath` is a regular file, but the error decision covers only the "no longer exists" wording, leaving the message for that case to the plan writer.
**Fix:** Say whether it shares the vanished-path message or gets its own.

## Verdict

REQUEST_CHANGES
Standalone anchor semantics, a false hook-inventory claim, and undecided stat-error handling.
_Note: 1 finding(s) demoted from BLOCKING to NIT by the stage's blocking-class ceiling; current blocking_count is 2._
MILL_REVIEW_END
