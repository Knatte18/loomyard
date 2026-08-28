MILL_REVIEW_BEGIN
# Review: reed: pane reap isn't applied consistently across up/add's mutating paths

```yaml
duration_s: 160.0
verdict: REQUEST_CHANGES
reviewer_model: opusmedium
reviewer_self_id: Claude (Anthropic), Opus-class model
reviewed_file: _mill/discussion.md
date: 2026-08-28
```

## Findings

### [BLOCKING:design] Adoption-prose inventory has no sweep method
**Section:** Scope → In ("Adoption-describing comments elsewhere") **Issue:** The doc-surface inventory is hand-listed (spawn.go header, strand.go:497, doc.go bullets) with the rule "neither may be left asserting a behaviour that no longer exists", but no enumeration method — and it already misses `reconcile.go:129` and `doc.go:279`, both asserting *"planPaneTarget never adopts or splits the header"*, with `clearConflictingPaneBindings` simultaneously listed under **Out**. **Fix:** State a sweep for adoption prose the way the test sweep is stated (e.g. `grep -rn "adopt" internal/reedengine/*.go`) and a disposition rule, rather than a fixed list a plan writer must trust.

### [NIT:consistency] Smoke pid assertions contradict kill-pane-only
**Demoted-from:** BLOCKING
**Section:** Testing → Smoke (M16 and M22 regressions) vs Decision `untracked-reap-stays-kill-pane-only` **Issue:** Both regressions assert a pid is dead immediately after the verb returns ("the recorded foreign pid is no longer alive"; "the old strand's process is gone"), but the reap deliberately issues `kill-pane` only — no `descendantClosurePIDs`, no `reapPaneChildren` wait — and reed's own `RemoveStrand`/`Down` comments state pane subtrees die *asynchronously* and that the process holding the worktree can be a deeper descendant. M22's "the old strand's process" is a `send-keys`-launched child, not `#{pane_pid}`. **Fix:** Say which pid each test captures and how it waits (bounded poll on `#{pane_pid}` only), or state that descendant liveness is explicitly not asserted.

### [NIT:scope] RemoveStrand's tail behaviour changes while listed Out
**Section:** Scope → Out **Issue:** "`RemoveStrand`, `Down` … logic untouched" is true of the code, but the one-rule-for-every-call-site gate makes `RemoveStrand`'s tail reconcile (`strand.go:513`) newly reap untracked alive panes once the last strand is gone — a behaviour change a plan writer could read Out as excluding. **Fix:** Note in Out that Remove's code is untouched but its tail reconcile inherits the new gate.

## Verdict

REQUEST_CHANGES
Two blockers: unmethodical adoption-prose inventory, and smoke pid assertions contradicting the kill-pane-only decision.
_Note: 1 finding(s) demoted from BLOCKING to NIT by the stage's blocking-class ceiling; current blocking_count is 1._
MILL_REVIEW_END
