I have completed my verification. The plan is well-grounded against source; my findings center on Context-completeness gaps for same-package sibling files, one intra-batch compile-order cycle, and one unrecorded decision divergence.

MILL_REVIEW_BEGIN
# Review: Build perch - the review gate loop — holistic

```yaml
verdict: REQUEST_CHANGES
reviewer_model: opushigh
reviewed_file: plan/
date: 2026-07-08
```

## Findings

### [BLOCKING] Card 9 Context omits judgeverdict.go and template.go
**Location:** Batch 3 / Card 9
**Issue:** judge.go's `runCircling`/`runMilestone`/`runTriage` reference `ParseJudgeVerdict`/`ParseTriageVerdict`, the verdict constants (`JudgeProgressing`, `JudgeContinue`, `TriageRetry`, `JudgeVerdict`, `TriageVerdict`), and the embedded prompt templates — all from `internal/perchengine/judgeverdict.go` (card 8) and `internal/perchengine/template.go` (card 7), neither of which is in card 9's Context. The parser signatures and embed var names are non-guessable.
**Fix:** Add `internal/perchengine/judgeverdict.go` and `internal/perchengine/template.go` to card 9's Context.

### [MEDIUM] Card 13 Context omits profile.go and roundfiles.go
**Location:** Batch 4 / Card 13
**Issue:** run_test.go constructs `perchengine.Profile` values (Gate/GateMode/RoundCaps) and asserts round-token artifact paths (`round-3b-review.md`), but `internal/perchengine/profile.go` and `roundfiles.go` are not in Context — the implementer cannot see Profile's fields or the token scheme without cold-start exploration.
**Fix:** Add `internal/perchengine/profile.go` and `internal/perchengine/roundfiles.go` to card 13's Context.

### [MEDIUM] Cards 10 and 11 are mutually dependent; neither compiles alone
**Location:** Batch 4 / Cards 10, 11
**Issue:** card 10's engine.go `New` names `execGateCommand` (defined in card 11's gate.go), while card 11's gate.go references the `CommandRunner` type (defined in card 10's engine.go). This is a two-file compile cycle, so per-card atomicity ("no forward deps", each commit builds) cannot hold by reordering. gate.go is also absent from card 10's Context.
**Fix:** Have `New` store `opts.RunCommand` and let `Run`/gate substitute the `execGateCommand` default (so engine.go need not name it), or explicitly note cards 10+11 co-compile; add gate.go to card 10 Context.

### [MEDIUM] Card 15 drops the discussion's --timeout flag without a Shared Decision
**Location:** Batch 5 / Card 15
**Issue:** discussion decisions "Command tree v1" and "Run-tuning v1" specify `lyx perch run ... [--timeout]` overriding the profile's per-round tuning; card 15 states `--timeout` is "NOT a perch flag in v1" (round timeouts ride shuttle config) and Profile carries no burler-round Timeout. This is a genuine divergence not recorded in `## Shared Decisions`.
**Fix:** Either add the `--timeout` flag + a Profile round-timeout field, or record the drop as an explicit Shared Decision in 00-overview.md.

### [NIT] Card 6 Context omits profile.go for the Profile type
**Location:** Batch 2 / Card 6
**Issue:** `ProfileHash(p Profile)` and `loadOrInitState`/`DeriveRunID` reference the `Profile` type from profile.go (card 4), which is not in card 6's Context (only result.go and state.go). Only `json.Marshal` is needed, so the gap is minor.
**Fix:** Add `internal/perchengine/profile.go` to card 6's Context.

## Verdict

REQUEST_CHANGES
Sound plan; fix the card 9 Context gap and the smaller Context/ordering/decision-record issues.
MILL_REVIEW_END
