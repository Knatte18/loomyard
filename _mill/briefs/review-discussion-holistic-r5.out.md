MILL_REVIEW_BEGIN
# Review: reed: attach's layout computation scales header pane height with terminal height

```yaml
verdict: REQUEST_CHANGES
reviewer_model: opusmedium
reviewer_self_id: Claude (Anthropic), Opus-class model
reviewed_file: _mill/discussion.md
date: 2026-08-28
```

## Findings

### [BLOCKING:scope] Multiplexer contract surface gains two verbs, undisposed
**Section:** `doc-work-is-additions-at-three-named-sites` / Constraints
**Issue:** `set-hook` and `resize-pane` appear nowhere in `internal/` today (grep) and are absent from both `doc.go`'s "Subcommand set" paragraph and `requiredSubcommands` (`probe.go`); `doc.go`'s closing sentence — "requiredSubcommands (probe.go) did not grow for any of this … add no capability-probe change and **no new psmux risk**" — becomes false once this task ships two unprobed verbs against a multiplexer whose support is admitted unverified, so the decision's premise "nothing in the existing geometry comments is false" does not hold and the three named sites are an incomplete inventory.
**Fix:** Name the "Subcommand set" paragraph (and the closing psmux-risk sentence) as a fourth doc site, and state a disposition for `contract_integration_test.go`'s `TestMultiplexerContract` — the documented canary for exactly this wire surface — either a new case for `set-hook`/`window-resized`/`resize-pane` or an explicit "not covered, because non-fatal".

### [NIT:design] Engine-side seam for obtaining pins is unspecified
**Section:** `pins-come-from-render-policy-not-raw-config`
**Issue:** The decision fixes the `render`-side entry point and rules out a fourth `Rules` return value, but never says how the engine reaches it — `planLayout` (`apply.go`) owns the `toRenderStrands`/present-set filtering and the `HeaderPaneID` blanking the zero-pin case depends on, so a plan writer may either grow `planLayout`'s returns or add a second mapping site that can silently diverge from it.
**Fix:** State that the pins are produced from the same mapping `planLayout` performs (one call site, blanked header id included), leaving only the signature shape to the plan.

## Verdict

REQUEST_CHANGES
Doc/contract inventory omits the new tmux verbs and the contract-test canary.
MILL_REVIEW_END
