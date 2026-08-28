MILL_REVIEW_BEGIN
# Review: reed: watchdog daemon

```yaml
verdict: REQUEST_CHANGES
reviewer_model: opusmedium
reviewer_self_id: claude-opus-5 (Anthropic Claude, Opus-class; exact build self-reported)
reviewed_file: _mill/discussion.md
date: 2026-08-28
```

## Findings

### [BLOCKING:design] Watch return contract vs. keepalive ownership
**Section:** `loop-body-lives-in-reedengine-...` + `watchdog-single-toggle-no-tunables`
**Issue:** `Engine.Watch(ctx) error` is paired with "the `--blocking` tail does nothing but call it" and "the loop never exits, never returns an error to the pane" — yet on `watchdog: off`/invalid the loop "declines to start and falls through to the plain block-forever keepalive", and the discussion never says whether `Watch` blocks in that case or returns to a tail that blocks; a literal implementation of "does nothing but call it" lets `reedcli/header.go`'s RunE fall through past `blockForever()` (`header.go:64-68`) and the keepalive pane exits.
**Fix:** State `Watch`'s return semantics explicitly — that it never returns while the pane must live (including the disabled case), or that the tail unconditionally calls `blockForever()` after it — and state that a non-nil return is logged only, never written to stdout/stderr (an `output.Err` there would paint over the rendered header, contradicting `header-blocking-tail-discards-logger-output`).

### [BLOCKING:design] Hook presence is not hook delivery; signal mode never demotes
**Section:** `hook-availability-decides-poll-fallback`
**Issue:** Mode selection rests entirely on a `show-hooks`-class presence read, and signal mode "never re-probes and never demotes"; on psmux — the exact platform the fallback exists for, where `set-hook`/`run-shell` are absent from `requiredSubcommands` and unverified — a hook that installs but never fires (or a `HookInstalled` match rule that matches any `window-resized` hook rather than reed's own command string for this session's signal path) pins the watcher in signal mode permanently with zero self-heal and no way back.
**Fix:** State the exact `HookInstalled` match rule (reed's own `run-shell -b` string targeting this worktree's signal path), and give signal mode one escape — demote to poll after a bounded signal-less interval, or declare Windows/psmux poll-only unconditionally rather than probe-decided.

### [NIT:consistency] "off stops the watcher outright" contradicts the take-effect boundary
**Section:** `hook-availability-decides-poll-fallback`, reverse-direction rationale
**Issue:** It justifies not handling hook removal with "`watchdog: off` … stops the watcher outright", while `watchdog-single-toggle-no-tunables` states `off` does **not** stop an already-running watcher until the next header-pane rebuild — so a signal-mode watcher does keep running against an unset hook.
**Fix:** Restate the rationale on its true grounds: the operator asked for `off`, so a signal-mode watcher going silent is the intended outcome, not an unhandled case.

### [NIT:design] Repeated hook install idempotency unstated
**Section:** `hook-set-in-pingeometryoptionslocked`
**Issue:** `pinGeometryOptionsLocked` runs on every `AttachArgv` pre-flight (`attach.go:80`) as well as at boot (`lifecycle.go:423`), so the hook is re-installed on every attach; the live-facts table probes firing but never repeat-install, and tmux hook options are arrays where append-vs-replace decides whether N attaches mean N `run-shell` spawns per resize.
**Fix:** State that the install is a replacing `set-hook` (never `-a`) and that repeated installs are idempotent, or record the probe that establishes it.

## Verdict

REQUEST_CHANGES
Two contract gaps: Watch/keepalive ownership, and signal-mode selection with no escape.
MILL_REVIEW_END
