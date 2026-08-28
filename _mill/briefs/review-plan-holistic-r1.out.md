MILL_REVIEW_BEGIN
# Review: reed: attach's layout computation scales header pane height with terminal height — holistic

```yaml
verdict: REQUEST_CHANGES
reviewer_model: sonnetxhigh
reviewer_self_id: claude-sonnet-5 (Sonnet 5)
reviewed_file: plan/
date: 2026-08-28
```

## Findings

### [BLOCKING:design] Stale resize-pin hook survives applyLayoutLocked's guard-skip paths
**Location:** batch 2, cards 5/6; Shared Decision `install-points-are-two-named-statements-no-guard-moves`
**Issue:** Decision `the-clear-is-unconditional-including-zero-pins` only covers reaching the install statement with zero pins (blanked header, no strip); it never addresses the `len(live) < 2` / `!anyPlacedStrand` guard-skip paths, which leave a previously-installed hook's array entries (naming specific, now-stale pane ids/heights) installed with no clear. E.g. add one strand under a header, then remove it — `live` drops to 1 (header only), `applyLayoutLocked` returns at the guard before the new statement, and the header's earlier fixed-height pin stays armed; a later client resize fires `resize-pane -t <header> -y 1` against what is now the window's sole pane. Every other subtle tmux behavior claim in this plan is "verified live" — this reachable case has no such verification either way.
**Fix:** Either verify live that `resize-pane` on a window's sole pane no-ops/harmlessly errors (matching the array-independence property already relied on), or have the guard-skip paths also issue the unconditional clear so no stale entry can outlive the layout it was computed for.

### [BLOCKING:design] Card 9's "restore window-size to its prior value" names no mechanism
**Location:** batch 3, card 9 (`TestMultiplexerContract` new section)
**Issue:** The card pins the window to `window-size manual` for the fire-order sub-case, then says to "restore window-size to its prior value" — no prior readback is specified anywhere in the requirement, and no existing test in this package captures-then-restores a tmux option, so there is no established idiom to fall back on. Per the closed Requirements-specificity criterion this is vague prose without a named function/mechanism (read back via `display-message` first vs. `set-option -u` to unset).
**Fix:** Name the exact mechanism — either read `#{window-size}` via `display-message` before the `manual` pin and `set-option -w` it back afterward, or explicitly call for `set-option -uw -t <target> window-size` to unset the override.

### [NIT:scope] A few Requirements reference identifiers from files outside that card's Context/Edits
**Location:** batch 2, cards 3 (`LivePane`, declared in `parse.go`), 4/5/6 (`e.SessionName()`/`e.Socket()`, declared in `lock.go`), 6 (`render.Box`, declared in `render/types.go`)
**Issue:** Per the Context-completeness rule the implementer may only read files in `Context:`/`Edits:`; none of `parse.go`/`lock.go` are listed for these cards. Practical risk is low since each symbol is already used unchanged, with the same fields/semantics, inside the very file each card edits.
**Fix:** Add `parse.go` to card 3's Context and `lock.go`/`render/types.go` to cards 4–6's Context for completeness, even though cold-start risk here is minimal.

### [NIT:scope] Card 7 says "extend the existing hermetic fixtures" for apply_test.go, but no call-recording fixture exists there yet
**Location:** batch 2, card 7
**Issue:** `attach_test.go` has `attachRecorder`/`newAttachHook` to record call sequence; `apply_test.go` today has no analogous recorder — tests there either use no hook or a single-purpose closure. "Extend" undersells that a new recording seam must be built from scratch for the sequence-discriminating assertions this card asks for.
**Fix:** Reword to say a new recorder is added to `apply_test.go`, modeled on `attach_test.go`'s (which card 7 already has in its Edits and can pattern-match against).

## Verdict

REQUEST_CHANGES
Two BLOCKING gaps: an unverified stale-hook edge case on guard-skip, and an underspecified option-restore mechanism in card 9.
MILL_REVIEW_END
