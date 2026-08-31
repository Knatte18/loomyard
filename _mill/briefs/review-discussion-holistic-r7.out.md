MILL_REVIEW_BEGIN
# Review: Reed attach dot-fill render artifact on resize and cross-client mouse move

```yaml
verdict: REQUEST_CHANGES
reviewer_model: opusmedium
reviewer_self_id: Claude Opus 5 (Anthropic)
reviewed_file: _mill/discussion.md
date: 2026-08-31
```

## Findings

### [BLOCKING:design] Smoke scenarios' watchdog setting is unstated
**Section:** Testing → smoke tests / measurement gate
**Issue:** No scenario states which `watchdog` value the reed session boots with, and it is load-bearing in both directions: with `watchdog: on` the artifact self-heals in ~1 s (per `root-cause-model`) while the named predicate style, `pollPaneContains` (`internal/reedcli/smoke_test.go:687`), samples every 500 ms — so the control can miss and the treatment's "absent for the whole deadline" is partly satisfied by the watchdog heal rather than by the repaint entry; with `watchdog: off` the array is empty from boot and re-emptied by any degrading attach (this discussion's own `repaint-is-independent-of-watchdog`), so the treatment may run with no repaint entry installed at all.
**Fix:** State the `watchdog` value each scenario runs under, and state the predicate's sampling cadence relative to the ~1 s heal window rather than inheriting `pollPaneContains`' 500 ms by reference.

### [BLOCKING:design] Measured-body pinning is not writable as specified
**Section:** Testing → Measurement gate ("the accepted body string is then asserted, in a pure unit test, to equal the string the scenario measured")
**Issue:** For candidate 1 the body embeds `e.TmuxPath()`, `e.Socket()` and `exactSessionTarget(e.SessionName())`, which are the harness's values in the measuring scenario and different values in any unit test, so a byte-equality assertion against a recorded literal cannot hold; only candidate 2's constant body admits it.
**Fix:** Restate the anti-drift gate as "the builder, invoked with the same tmux path, socket and session the scenario used, reproduces the measured string byte-identically", or scope the literal pin to candidate 2.

### [NIT:consistency] "grep finds no list-clients" claim is false
**Section:** Technical context → "Nothing in reed currently knows about multiple clients"
**Issue:** `internal/reedengine/attachgeometry_integration_test.go:157` already issues `e.tmux.output("list-clients", "-t", exactSessionTarget(e.SessionName()))`; the grep claim as written is wrong, though the production-code claim holds.
**Fix:** Narrow the claim to production code and cite the existing test call, which independently corroborates the `exactSessionTarget` form chosen in `repaint-body-composition`.

### [NIT:design] Candidate 1's fragment needs two tmux invocations, not one
**Section:** `repaint-body-composition`
**Issue:** The four-source list describes the binary path and `-L` socket as embedded once, but the loop invokes tmux twice — `list-clients -F` to enumerate and `refresh-client -t` per line — each needing path plus socket, and only the former takes the session target.
**Fix:** Say both invocations carry the quoted path and `-L` socket, and that the session target applies to `list-clients` alone.

## Verdict

REQUEST_CHANGES
Two load-bearing test-strategy gaps: unstated watchdog setting and an unimplementable body-pinning assertion.
MILL_REVIEW_END
