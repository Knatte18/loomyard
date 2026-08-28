MILL_REVIEW_BEGIN
# Review: reed: watchdog daemon

```yaml
verdict: REQUEST_CHANGES
reviewer_model: opusmedium
reviewer_self_id: Claude (Anthropic), Opus-class model
reviewed_file: _mill/discussion.md
date: 2026-08-28
```

## Findings

### [BLOCKING:design] Unattended re-apply steals operator pane focus
**Section:** `reapply-layout-is-a-new-public-engine-op` **Issue:** `applyLayoutLocked` ends with `select-pane -t focus` (`apply.go:162`), and `focus` comes from reed state's `Display.Focus` flag via `render/focus.go`, not from the live active pane — so every resize would yank the operator's cursor into the reed-declared focus strand, which is harmless for an operator-invoked op but not for a watcher firing mid-typing. **Fix:** State a disposition: either the new op skips the `select-pane` half (and say what that costs), or it preserves and restores the live active pane, or focus-stealing is explicitly accepted with rationale.

### [BLOCKING:design] select-layout's own effect on `window-resized` never probed
**Section:** `window-resized-is-the-event-source` / Live tmux facts **Issue:** The probe table establishes hook ordering only for a *client* resize; `apply.go`'s comment (lines 136–140) documents that a detached `select-layout` can GROW the window to fit cells, which changes window geometry and would plausibly re-fire `window-resized` — the exact self-trigger loop `window-layout-changed` was rejected for. **Fix:** Add a live probe of whether reed's own `select-layout` (attached and detached) fires `window-resized`, and state the loop-breaking rule if it does.

### [BLOCKING:design] `watchdog: off` scope is undefined
**Section:** `watchdog-single-toggle-no-tunables` / `hook-set-in-pingeometryoptionslocked` **Issue:** The toggle is read by the header-pane process, but the hook install lives in `pinGeometryOptionsLocked`, which knows nothing of it; the discussion never says whether `off` also suppresses the hook install, whether an already-installed hook is unset when the operator flips to `off`, or whether `off` also disables the poll fallback. **Fix:** State exactly what `off` disables (loop only vs. loop + install vs. loop + install + unset) and what happens to a hook already set on a live session.

### [BLOCKING:design] Blocking op-lock acquisition is outside the retry contract
**Section:** `watch-loop-failures-are-never-fatal` **Issue:** `withOpLock` uses `lock.AcquireWriteLock` (`lock.go:21`), which blocks with no timeout (the repo records an 11027ms observed wait); a re-apply stuck behind a long `up`/`add` is neither an "attempt" nor a "failure", so the N-attempts-with-escalating-delay contract does not describe it, and tier-1 assertion 7 cannot exercise it. **Fix:** Decide the watcher's lock discipline — try-lock-and-defer-to-next-tick, or blocking with a stated deadline — and state it as part of the failure policy.

### [NIT:design] Hook-availability mode is decided once, never revisited
**Section:** `hook-availability-decides-poll-fallback` **Issue:** The startup `show-hooks` probe fixes the mode for the header process's whole life, yet the design's own migration story installs the hook later, at attach pre-flight (`attach.go:80`) — a watcher started on a hook-less already-up session stays in permanent poll mode even after the hook appears. **Fix:** Say whether the mode is re-probed (and on what trigger) or is deliberately one-shot for the process lifetime.

### [NIT:scope] Hook command string's shell seam left conditional
**Section:** Constraints (Shell Mechanics Seam) **Issue:** `shell.Shell` exposes only `Quote`/`Invoke`/`ReadFile`/`WithEnv` (`shell/shell.go:13`) — no file-touch primitive — and the discussion states adding one only conditionally ("if a file-touch primitive is needed"); it also never says which dialect builds the string, given `run-shell` is executed by the tmux server's own shell rather than the pane shell the seam models. **Fix:** Commit to adding the primitive (or not) and name the dialect selection rule for the hook string.

### [NIT:decision] Invalid `watchdog` value has no stated disposition
**Section:** `watchdog-single-toggle-no-tunables` **Issue:** The tier-1 plan tests a "garbage" value, but the decision never says whether it follows `mouseOption`'s precedent (`mouse.go:22`, which hard-errors the boot) or degrades silently to `on` — and a header-pane-only key failing `Up` is a real behaviour choice. **Fix:** State the invalid-value behaviour and where it is validated.

## Verdict

REQUEST_CHANGES
Focus-steal, self-trigger, toggle scope, and lock-blocking behaviour are undecided.
MILL_REVIEW_END
