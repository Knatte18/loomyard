MILL_REVIEW_BEGIN
# Review: reed: attach doesn't reconcile session geometry with the terminal

```yaml
duration_s: 132.0
verdict: REQUEST_CHANGES
reviewer_model: opus
reviewer_self_id: Claude (Anthropic), Opus-class model
reviewed_file: _mill/discussion.md
date: 2026-08-26
```

## Findings

### [BLOCKING:design] `status off` pin is global; exit 0 is not proof
**Section:** `### geometry-options-pinned-at-boot-and-attach`
**Issue:** The pin is spelled `set-option -g status off` and the design treats a successful call as proof the post-attach window height equals the client's rows, but `status` is a session option: a session-scoped value (settable from the very `~/.tmux.conf` this decision names as the threat model, and loaded at the server start that creates reed's own session) is not overridden by a global set, and `set-option` still exits 0 — so the "failed pin suppresses the chain" safeguard never fires while the told box is off by one row, contradicting the claim that pinning "removes the only remaining way an operator's config can break the fix".
**Fix:** State the option scope explicitly (session-targeted pin, or `-g` plus a session-level set) or name a verification step for the effective value — `display-message -p '#{status}'` uses an already-required subcommand, unlike the `show-options` alternative the Rejected list turns down.

### [NIT:design] Op-lock acquisition mode for `AttachArgv` unstated
**Section:** `### engine-owns-the-attach-argv` / `### the-build-vs-apply-window-is-accepted-and-safe`
**Issue:** The plan is built "under the op lock", but `withOpLock` (`internal/reedengine/lock.go:79`) is non-reentrant and `lock.AcquireWriteLock` blocks with no timeout — the discussion never says whether `AttachArgv` blocks like the existing `Status()` pre-flight or try-locks and degrades, and "no engine-side failure ever blocks the attach" does not cover a wait.
**Fix:** State that `AttachArgv` acquires the op lock the same blocking way `Status()` already does in this pre-flight (or that it try-locks and degrades to the bare argv), so a plan writer neither invents a nested acquisition nor a new timeout.

## Verdict

REQUEST_CHANGES
One unstated option-scope premise underpins the told box; everything else is decided.
MILL_REVIEW_END
