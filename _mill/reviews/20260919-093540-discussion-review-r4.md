MILL_REVIEW_BEGIN
# Review: reed: born-as-strand for loom start's operator attach

```yaml
duration_s: 121.0
verdict: REQUEST_CHANGES
reviewer_model: opus
reviewer_self_id: Anthropic Claude, Opus-class (environment reports claude-opus-5)
reviewed_file: _mill/discussion.md
date: 2026-09-19
```

## Findings

### [NIT:consistency] Technical context still orders the Shell() accessor
**Demoted-from:** BLOCKING
**Section:** § Technical context, "The engine surface already available" **Issue:** It states "The only missing piece is the configured shell: `ReedConfig.Shell` ... has no exported accessor. Add one in the style of the existing `TmuxPath()`", which directly contradicts the Scope bullet ("No new `reedengine` accessor ... `ReedConfig.Shell` stays unexported and untouched") and the `empty-cmd-leaves-the-panes-own-shell` decision; verified that `reedengine` today exposes `Socket`/`SessionName`/`TmuxPath` (`internal/reedengine/lock.go:51-61`) and no `Shell`. **Fix:** Delete/rewrite that paragraph to state no accessor is added, and reconcile the Scope bullet's "the command builder" in `bootstrap.go`, which is likewise vestigial once `Cmd` is `""`.

### [NIT:consistency] Testing asserts Focus==true and a shell Cmd
**Demoted-from:** BLOCKING
**Section:** § Testing, first Tier 1 bullet **Issue:** It requires asserting `Display.Focus == true` and "that `Cmd` is the shell string handed in rather than a composed command line" — both superseded values; the decisions pin `Focus: false` and `Cmd: ""`, and the later "Worth testing after all — the focus default" paragraph assumes the opposite of this bullet. **Fix:** Restate the bullet as `Focus == false`, `Cmd == ""`, and drop the "shell string handed in" parameter implied by it.

### [BLOCKING:design] Focus default fails on the re-entrant attach path
**Section:** § Decisions, `display-below-parent-focused-no-shrink` / `operator-strand-is-the-thing-born` **Issue:** The rationale only reasons about the fresh-bootstrap stack `[loom-status, operator]`; `orderStack` (`internal/reedengine/render/policy.go:113-133`) sorts strictly by chain depth, so on the explicitly supported re-entrant invocation (driver already alive, agent strands at depth ≥ 1 present) the depth-0 operator strand is never bottom-most and `focusTarget` lands the operator in an agent pane — contradicting "attaches with that pane focused". **Fix:** State the disposition for the re-attach case explicitly — accept landing in the agent pane as intended, or adopt rejected option (c) (`select-pane` on the operator pane in the attach chain) — rather than leaving it to a smoke test.

### [NIT:scope] start.go's own Long help text is outside the doc inventory
**Section:** § Scope, "Docs in the same commit" **Issue:** The inventory names `manifest/designs/loom.md`'s step list but not `internal/loomcli/start.go`'s `Long`, which enumerates the same four steps and says "--no-attach performs steps 1 through 3"; both the operator strand and the watchdog spawn change that user-visible list, and the alias registration shares it. **Fix:** Add the `startCmd` `Long` text to the same-commit update list.

## Verdict

REQUEST_CHANGES
Two superseded statements and an unaddressed focus case on the re-entrant attach path.
_Note: 2 finding(s) demoted from BLOCKING to NIT by the stage's blocking-class ceiling; current blocking_count is 1._
MILL_REVIEW_END
