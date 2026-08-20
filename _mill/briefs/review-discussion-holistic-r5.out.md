MILL_REVIEW_BEGIN
# Review: Bouncer: the generic review-gate producer

```yaml
verdict: REQUEST_CHANGES
reviewer_model: opusmedium
reviewer_self_id: Claude (Opus-class, Anthropic) — exact build not self-verifiable
reviewed_file: _mill/discussion.md
date: 2026-08-20
```

## Findings

### [BLOCKING:design] Nothing tells the judge which artifact it reviews
**Section:** "The constructor surface" (`BouncerConfig` table) **Issue:** The table is declared to carry "exactly these fields" and none of them names the artifact under review, yet the design elsewhere requires the LLM to read "the rubric *and the artifact*" at seed time (§Seed call) and to judge a report *about* something; `RunDir`+`ReportName` name only the round producer's own report, and a review target such as a discussion or plan file need not live under `RunDir`. **Fix:** Decide and pin how the subject artifact reaches both spawns — a told `ArtifactPath` (or path list) field on `BouncerConfig`, or an explicit statement that the rubric stencil alone names it — and add it to the constructor validation and pointer/testing tables.

### [BLOCKING:design] Template marker set is never enumerated
**Section:** "Stencils" / "Technical context — the judge spawn pattern" **Issue:** Only the `rubric` marker and a previous-ledger marker are named, but `stencil.Fill` has no conditionals and errors on any unresolved marker (verified: `internal/stencil/stencil.go:20-27`, `treadleengine/judge.go:62-68,110`), so the exact marker set of `bouncer-template-seed.md` and `bouncer-template-judge.md` — round, report path, the three output paths, previous ledger, rubric — is a decision the plan cannot derive; relatedly, whether the report reaches the prompt as a *path* or as inlined *content* is stated both ways (§Four modes line "read the report ..." vs. Testing "a valid prior ledger's **path** reaches the prompt"). **Fix:** Pin the complete marker map for each template, state path-vs-content for the report and the previous ledger, and pin the literal `Spec.Role`/`Spec.Round` values (`Round` is a `string` field, `spec.go:72-73`).

### [BLOCKING:design] Round-1 focus file: existence-keyed, unlike `judged(N)`
**Section:** "Four modes" / "Focus-file synthesis" **Issue:** The re-bounce branch fires on `round-1-focus.md` merely *existing*, while `judged(N)` deliberately requires a *parse* for exactly the stated reason that a truncated artifact must not be mistaken for a real one; a crash between the seed spawn writing a malformed focus file and the seed fallback firing therefore leaves a permanently unparseable round-1 input that re-bounces until the budget is spent — the failure path 2 exists to prevent, and the crash-recoverability property replay is praised for. **Fix:** Decide explicitly whether the re-bounce discriminator is "present" or "present and parses" (and, if the latter, which path repairs it), and record the rationale either way.

### [NIT:scope] Report-read failure absent from the failure enumeration
**Section:** "Failure posture" **Issue:** The degradation list enumerates stencil, fill, `Run`, outcome, verdict and ledger failures but not an unreadable/empty report file for round `N`, which `ResolveRound` proved existed only by stat; the Testing section's degradation enumeration ("enumerate them individually") likewise omits it. **Fix:** Add the report-read failure to both the degradation list and the per-path test enumeration, or state that it is deliberately covered by the catch-all.

### [NIT:consistency] "File existence alone" contradicts `judged(N)`
**Section:** Scope bullet 2 / "Four modes" heading / r2 Q&A **Issue:** Scope and the section heading say the four branches are told apart "by file existence alone/only", but `judged(N)` requires both files to *parse*; the r2 Q&A entry also states the predicate as verdict+ledger present plus *verdict* parsing, omitting the ledger parse the body and the r3 entry require, without the "Superseded" annotation this file uses elsewhere. **Fix:** Reword the two "existence alone" phrasings to "existence plus parse", and annotate the r2 entry as refined by r3.

## Verdict

REQUEST_CHANGES
Three unresolved decisions: review target, template markers, round-1 focus discriminator.
MILL_REVIEW_END
