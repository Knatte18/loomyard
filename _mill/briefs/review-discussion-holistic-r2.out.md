MILL_REVIEW_BEGIN
# Review: reed: pane reap isn't applied consistently across up/add's mutating paths

```yaml
verdict: REQUEST_CHANGES
reviewer_model: opusmedium
reviewer_self_id: Claude, Opus-class Anthropic model (exact version not self-verifiable)
reviewed_file: _mill/discussion.md
date: 2026-08-28
```

## Findings

### [BLOCKING:consistency] Non-regression claim misstates when the reap fires
**Section:** Testing → Smoke → Non-regression **Issue:** The discussion says `TestSmokeUpWithOnlyForeignPanesKeepsSessionUsable` "must keep passing unchanged" and that the initial+foreign panes are reaped "during the add" — but under `anyBoundPresent || headerPresent` the reap fires on that test's **second `up`** (header present, zero strands), so the test's own stated premise (`smoke_lifecycle_test.go:218` "the foreign panes survive an up"; `:255` "Every pane must survive it") becomes false while the loose `len(panes) == 0` assertion still passes. **Fix:** State that this test's premise/comment must be rewritten to the new post-`up` pane count and that its `up` assertion is tightened, rather than declaring it unchanged.

### [BLOCKING:design] Corpse header as anchor on the launch path is undecided
**Section:** Decisions → reap-gate-accepts-the-header-as-survival-anchor / reap-before-allocate **Issue:** Presence-not-aliveness is justified by "a header corpse is separately healed by `ensureHeaderPaneLocked` on the next `up`/`resume`", but the new chokepoint makes the gate fire from `add`/`update`/`remove`, which never run `ensureHeaderPaneLocked` — so a dead-but-present header can authorize reaping the session's only *alive* pane, after which `planPaneTarget` falls through to splitting the corpse itself (`spawn.go:68-84`). **Fix:** State the disposition — either require an alive anchor when reconcile runs from `launchStrandLocked`, or explicitly accept split-off-a-corpse as the designed outcome.

### [NIT:decision] Reap log line: optional distinction vs mandatory identifiability
**Section:** Decisions → the-reap-logs-what-it-destroys **Issue:** "Whether the line distinguishes dead-pane kills from untracked-pane kills … is mill-plan's call" sits beside "the untracked reap must be identifiable in the log", and `planReconcile` returns one merged `panesToKill` slice (`reconcile.go:14`), so the Testing section's "Info line naming those pane ids" for untracked kills is only satisfiable by some distinction. **Fix:** Say the distinction is required and leave only its mechanism (two lists vs `reason` key) to mill-plan.

### [NIT:scope] Adoption-describing comments outside the file list
**Section:** Scope → In **Issue:** `strand.go:497` ("planPaneTarget never adopts a corpse") is in neither In nor Out, and `doc.go`'s load-bearing bullet at `doc.go:164-174` ("planPaneTarget must never adopt such a corpse") and `spawn.go:3` ("create (or adopt)") are adoption claims the In list describes only as reap-policy/header paragraphs. **Fix:** Name the adoption-describing comments explicitly as in-scope doc surface.

## Verdict

REQUEST_CHANGES
Two decisions rest on inaccurate premises about existing test behaviour and header-corpse anchoring.
MILL_REVIEW_END
