MILL_REVIEW_BEGIN
# Review: Shed-generic watchdog for ly-drive and loom's CLI verbs

```yaml
verdict: REQUEST_CHANGES
reviewer_model: opusmedium
reviewer_self_id: claude-opus-4-class (self-assessed; brief dictates "opusmedium")
reviewed_file: _mill/discussion.md
date: 2026-09-19
```

## Findings

### [BLOCKING:design] Generic `status` envelope never decided
**Section:** `envelope-contracts-move-with-the-verbs`, `pause-and-watch-generalize`, Testing
**Issue:** The Decision fixes `step`'s ten keys and `run`'s four, but never the `status` envelope, and the two shipped ones are incompatible: `loomcli/status.go` emits `current_producer`/`state`/`error`/`pause_requested`/`activity`/`history_length`/`slug`/`parent`/`interrupt_policy` and *errors* on an absent file ("run \"lyx loom start\" first"), while `lifecyclecli/status.go` emits `found`/`status_path`/`current_producer`/`state`/`error`/`activity`/`history` and treats absent as success — and no hook in the four-hook set covers loom's `st.Product` → `loomengine.Status` decode or its `interrupt_policy` key.
**Fix:** State the generic `status` envelope contract and the hook/spec mechanism carrying loom's product-payload and policy keys and its absent-file refusal.

### [BLOCKING:consistency] Testing contradicts loom's status surface
**Section:** Testing → `internal/shedcli`
**Issue:** "`status`: found and not-found both as success envelopes" directly contradicts `existing-subtrees-keep-their-surface`, since `lyx loom status` today returns an error envelope for not-found (`status.go:115-118`).
**Fix:** Reconcile — either the not-found disposition is told per arming spec, or the test expectation is corrected to match the preserved loom behaviour.

### [BLOCKING:design] PreRun vs Shed-construction ordering unfixed
**Section:** `optional-hooks-for-module-specific-work`, `named-recipe-table-arming`
**Issue:** The arming function is said to supply "the filled `shedrecipe.Env`", but `loomcli/run.go:125` assigns `c.env.Landing = landingDeps(...)` inside the pre-flight, after arming and immediately before `loomrecipe.New` — so a Shed built at arming time would carry a nil `Landing`.
**Fix:** State explicitly that `NewShed` is called after `PreRun` returns (or that the spec carries a Shed *constructor* rather than a built Shed).

### [BLOCKING:design] `lyx shed status|pause --recipe loom` breaks lightweight wiring
**Section:** `named-recipe-table-arming`, `generic-package-resolves-nothing`, `pause-and-watch-generalize` note
**Issue:** `loomcli.verbUsesLightweightWiring` keys on `cmd.Name()` so `status`/`pause` skip config load and engine construction (`cli.go:143,156-170`), but the shed table binds arming to the *recipe name* only, verb-blind, so `lyx shed status --recipe loom` would run the full `wire()` — the exact broken-config hazard that path exists to avoid, and a parity divergence from `lyx loom status`.
**Fix:** Say how the shed subtree's arming selects the lightweight path per verb, or that `status`/`pause` arm from a paths-only spec.

### [BLOCKING:consistency] Interactive-handoff list claimed unchanged
**Section:** Constraints → CLI / Cobra Invariant
**Issue:** "The interactive-handoff exception list is unchanged" is false once `status --watch` generalizes: `lyx shed status --watch` (and `lyx lifecycle status --watch`) are new never-exiting commands, and the CONSTRAINTS.md list names only `lyx loom status --watch`; relatedly, `lifecyclecli` gains `--watch`/`--interval` (it has neither today), a surface widening absent from the two agreed exceptions in Testing → Regression.
**Fix:** Name the new exception-list entries and add lifecycle's `--watch`/`--interval` to the enumerated surface changes.

### [NIT:scope] `--interval` omitted from the generic flag set
**Section:** `flags-and-args-belong-to-the-arming-module`
**Issue:** "only the flags the generic bodies themselves read (`status`'s `--watch`)" omits `--interval`, which the generic watch body reads for its own sleep (`status.go:132,166`).
**Fix:** List `--interval` alongside `--watch`.

### [NIT:consistency] `shedcli` is not a Cobra-invariant deviation
**Section:** Constraints → CLI / Cobra Invariant
**Issue:** The text says the "deviations list" needs updating for `shedcli`, but `shedcli` → `internal/shedengine` conforms to the `<module>cli` naming rule; only the "twelve of thirteen" count moves.
**Fix:** Drop the deviations-list claim, keep the count update.

## Verdict

REQUEST_CHANGES
Status envelope, hook/build ordering, lightweight-wiring arming, and handoff-list claims need resolving.
MILL_REVIEW_END
