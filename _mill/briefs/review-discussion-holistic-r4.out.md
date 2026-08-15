MILL_REVIEW_BEGIN
# Review: Shed: outer phase-FSM skeleton

```yaml
verdict: REQUEST_CHANGES
reviewer_model: opusmedium
reviewer_self_id: Claude (Anthropic) — Opus-class model, exact version not self-verifiable
reviewed_file: _mill/discussion.md
date: 2026-08-15
```

## Findings

### [BLOCKING:design] Persist can create the status file Shed swore never to seed
**Section:** `reread-and-merge-persist` / Scope "No seeding"
**Issue:** `state.UpdateJSON` treats a missing file as non-error — it calls `mutate(zero, found=false)` and then writes the result (`internal/state/state.go:104`, `:122-132`), so if the status file is deleted or its dir replaced between step 1's read and step 5's persist, Shed silently *creates* a status file, contradicting "Shed never creates a status file" and the `no-seeding-hard-error-on-missing` decision.
**Fix:** Decide and state what the mutate function does on `found=false` — almost certainly return an error so `UpdateJSON` aborts without writing — and add it to the persist-failure test list.

### [BLOCKING:design] Cancellation during a producer call routes to `failed`, not `paused`
**Section:** `ctx-cancellation-as-pause` vs. `state-and-error-fields`
**Issue:** `ctx` is only checked at the top of an iteration, but a real producer handed a cancelled `ctx` returns `context.Canceled` from `Call`, which step 6's error branch writes as `state: "failed"` with a non-nil `Run` error — the exact "misrepresents an intentional stop as something broken" outcome the decision rejects, and the "one clean-stop path, not two" claim then fails in the common Ctrl-C case.
**Fix:** Pin whether `Run` classifies a producer error as a pause when `ctx.Err() != nil` (or `errors.Is(err, context.Canceled/DeadlineExceeded)`), and add a loop scenario for "producer returns ctx.Err() after cancellation".

### [NIT:design] `activity.last` format is unpinned but asserted
**Section:** `activity-mechanical-fill` / Testing "The `activity` fill rule"
**Issue:** `last` is "the most recent history entry's producer + outcome formatted for a human" — no exact format, yet a TDD-candidate test is to "assert the three composed fields", which needs one.
**Fix:** Either pin the exact composed string or say the test asserts substring containment of producer and outcome.

### [NIT:design] `RunOutcome`'s zero value is not a legal value
**Section:** `run-entrypoint-result`
**Issue:** On the hard-error paths (validation, busy lock, missing/incoherent status, persist failure) the returned `Result` is unspecified; its `Outcome` would be `""`, which is not one of the three constants.
**Fix:** State in the doc comment that `Result` is meaningless unless `error` is nil, matching treadle's "branch on Outcome" discipline already carried over.

## Verdict

REQUEST_CHANGES
Two unresolved interactions: persist-on-missing-file seeding, and cancellation surfacing as failure.
MILL_REVIEW_END
