MILL_REVIEW_BEGIN
# Review: loom: Plan-Write/Plan-Validate approval deadlock (F7)

```yaml
verdict: REQUEST_CHANGES
reviewer_model: opusmedium
reviewer_self_id: Claude (Opus-class), Anthropic
reviewed_file: _mill/discussion.md
date: 2026-08-26
```

## Findings

### [BLOCKING:design] Idempotence rests on a resume path that cannot happen
**Section:** Technical context → Idempotence
**Issue:** "`Bouncer.settle` can be reached more than once for one generation on a resume" is false against `bouncer.go:182-193` — an APPROVED verdict on disk at `Call` entry is archived and `n` reset to 0, so the approved branch of `settle` is once-only per generation; the discussion's own Pipeline paragraph states this correctly, so the two sections contradict each other.
**Fix:** Restate the idempotence rationale on the true ground (a re-approved later generation re-running `SetApproved`), and state the disposition of the real recovery path below.

### [BLOCKING:design] No stated behaviour for a failed Approve/Commit on resume
**Section:** Decisions → approve-failure-is-an-error-not-a-stuck
**Issue:** An error from `settle` persists the run failed; `run.go:95` re-calls the same producer on resume, which then archives the APPROVED run dir and spends a **whole new LLM review generation** — and if `Approve` succeeded but `Commit` failed, that new generation judges a plan already carrying an uncommitted `approved: true`. Neither cost is acknowledged.
**Fix:** Decide and record whether that re-review is accepted as-is, or whether the settle seams need a retry/short-circuit, before the plan is written.

### [BLOCKING:consistency] "Four call sites keep their present behaviour" is wrong twice
**Section:** Decisions → validate-splits-into-two-named-functions (rationale)
**Issue:** Only three production `planparser.Validate` call sites exist outside `loomshed` (`websterengine/runlevel.go:332`, `webstercli/validate.go:74`, `loomcli/validate.go:97`), not four; and `loomcli/validate.go` does **not** keep its present behaviour — the next decision changes its default to `ValidateFormat`.
**Fix:** Correct the count to three and scope the "unchanged by default" claim to the two webster call sites.

### [BLOCKING:decision] Planparser invariant extension left as "should be read as"
**Section:** Constraints → Planparser Sole-Parser Invariant
**Issue:** The shipped entry is parse-only wording; the discussion says it "should be read as covering both after this task" but the Scope's in-commit doc list (line 41) names only the Gate Self-Check Parity entry, so the sole-writer property lands as an unwritten reading.
**Fix:** Put an explicit `CONSTRAINTS.md` Planparser Sole-Parser Invariant edit (sole parser **and** sole writer of the plan format) in the Scope's in-commit doc list.

### [BLOCKING:design] Keystone routing assertion has no stated mechanism
**Section:** Testing → Recipe-level wiring
**Issue:** The assertion "`Plan-Bouncer`'s `on_done` reaches `Batchifier` through `Plan-Revalidate` with no bounce" is named as the one test that would have caught F7, but reaching an APPROVED settle needs a verdict/ledger/report set in the run dir plus a satisfied `Env.Shuttle` and `NewBouncer`'s eager rubric-stencil probe — none of which the discussion says how to supply without an LLM run.
**Fix:** Name the mechanism (stub shuttle, or hand-seeded `round-N-review.md`/verdict/ledger files plus a stencils fixture) and the test tier it lands in.

### [NIT:consistency] Format-check count is off by one
**Section:** Decisions → validate-splits-into-two-named-functions
**Issue:** "the fifteen format checks, `checkIndexFileConsistency` through `checkCommitSubjectMismatch`, plus `format-unrecognized`" — that range is fourteen checks (`validate.go:60-73`); the format set is fifteen IDs including `format-unrecognized`, sixteen with `plan-unapproved`.
**Fix:** Say "the fourteen checks … plus `format-unrecognized`, fifteen IDs in all".

### [NIT:decision] Plan stencil's Step 5 self-check block has no disposition
**Section:** Decisions → stencil-and-writer-doc-keep-approved-false
**Issue:** Only the "future review gate" clause (`loom-template-plan.md:80`) is dispositioned; Step 5 (`:154-164`, "The verb takes no arguments", "re-run it until it exits 0") is the writer-facing half of the same deadlock and is never mentioned.
**Fix:** State explicitly that Step 5 stays verbatim (the new default makes it satisfiable) or name the wording change.

### [NIT:design] Mode parameter type left unspecified
**Section:** Decisions → planvalidate-row-mode
**Issue:** "`NewPlanValidate` takes a mode" does not say whether that is a `requireApproved bool` or a named type, and the constructor is pinned by `loomrecipe/shape_test.go` and `loomcli/parity_test.go`.
**Fix:** Name the parameter and its type in the decision.

## Verdict

REQUEST_CHANGES
Sound core design; four blocking gaps in recovery behaviour, call-site claims, invariant scope, and the keystone test.
MILL_REVIEW_END
