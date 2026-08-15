MILL_REVIEW_BEGIN
# Review: Shed: outer phase-FSM skeleton

```yaml
verdict: REQUEST_CHANGES
reviewer_model: opusmedium
reviewer_self_id: Claude (Anthropic), Opus-class model
reviewed_file: _mill/discussion.md
date: 2026-08-15
```

## Findings

### [BLOCKING:design] Routing undefined for a non-Done/Stuck Outcome
**Section:** `outcome-string-typed` / step-6 routing (and `shed.md:63-73`)
**Issue:** `Outcome` is an open `string` type, so an adapter can return `Outcome("")`, `"approved"`, or a typo with a nil error; the loop's routing covers only `error`, `Stuck`, and `Done`, and no decision says what happens to anything else — `shed.md:27` still describes the contract as "done / approved / stuck / blocked", four values, while its Go block (`shed.md:80-84`) declares two.
**Fix:** Pin the disposition of an unrecognised `Outcome` (hard error with `state: "failed"` is the only choice consistent with "never guess a status"), add it to the `ShedProducer` contract obligations beside the cancellation rule, and name the `shed.md:27` prose as a reconciliation edit.

### [BLOCKING:design] Validation rule set omits empty `Name` and nil `Producer`
**Section:** `validate-at-run-top`
**Issue:** The enumerated pre-loop checks (empty list, duplicate `Name`, dangling `OnStuck`, negative `MaxBounces`, same lock paths, empty paths) do not reject a `ProducerDef` with `Name: ""` or `Producer: nil`; an empty `Name` is semantically ambiguous because `OnStuck: ""` is the "escalate to human" sentinel (`shed.md:104`), and it also matches the zero-value `current_producer` an incomplete seed would carry, so step 2 would silently look it up and run it; a nil `Producer` panics at step 4 rather than failing loud.
**Fix:** Add both to the validation list and to the TDD validation table (line 343), stating that `Name` must be non-empty precisely because `""` collides with `OnStuck`'s sentinel and with a zero-value `current_producer`.

### [NIT:consistency] `Result` shape on the already-`done` short-circuit
**Section:** `terminal-state-on-completion-and-rerun` vs. `run-entrypoint-result` / `timestamps-and-result-history-scope`
**Issue:** The short-circuit is specified as returning `Result{Outcome: RunDone}`, but `HaltedProducer` is defined as "the producer `current_producer` named when `Run` returned" and `Result.History` as "the full persisted history as it stands when `Run` returns" — read literally the two rules disagree, so a second `Run`'s `Result` may or may not match the first's.
**Fix:** State explicitly that the short-circuit fills `HaltedProducer` and `History` from the file just read (writing nothing), or that both are deliberately empty there.

## Verdict

REQUEST_CHANGES
Two unpinned failure modes: unrecognised `Outcome` routing and incomplete producer-list validation.
MILL_REVIEW_END
