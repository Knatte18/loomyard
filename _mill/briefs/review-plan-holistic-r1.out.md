MILL_REVIEW_BEGIN
# Review: fabric: unify warp + weft into one git-coordination module — holistic

```yaml
verdict: REQUEST_CHANGES
reviewer_model: opushigh
reviewed_file: plan/
date: 2026-07-25
```

## Findings

### [BLOCKING] Card 22 Context missing lyxtest.go
**Location:** batch 5 / card 22
**Issue:** Requirements prescribe `index_integration_test.go` using `lyxtest.CopyWeft` (defined in `internal/lyxtest/lyxtest.go`), but that file is absent from card 22's `Context:` (unlike cards 20/21/25/26 which list it) — a Context-completeness gap forcing cold-start exploration.
**Fix:** Add `internal/lyxtest/lyxtest.go` to card 22's `Context:`.

### [BLOCKING] CONSTRAINTS amendment lands after first weft-touching card
**Location:** batch 5 / cards 22–23
**Issue:** Batch scope states the Weft Git Invariant amendment ships "in the same commit as the first weft-touching code," but card 22 (index git-wiring) already runs raw git against the weft worktree (`git rev-parse --git-dir`, `git log` trailer scan) via gitexec in fabricengine, one commit before card 23 holds the amendment — fabricengine is unsanctioned by the un-amended invariant at card 22.
**Fix:** Move the CONSTRAINTS.md amendment into card 22, or reorder so card 23's weftgit precedes card 22, or explicitly scope card 22's reads as excluded.

### [NIT] Card 3 Context lacks lyxtest for HermeticGitEnv
**Location:** batch 2 / card 3
**Issue:** `testmain_test.go` requirement calls `lyxtest.HermeticGitEnv()` (defined in `internal/lyxtest/hermetic.go`), not in `Context:`; mitigated only because `internal/warpengine/testmain_test.go` (in Context) shows the call form.
**Fix:** Add `internal/lyxtest/hermetic.go` to card 3's `Context:`.

## Verdict

REQUEST_CHANGES
Two card-level fixes: card 22 Context gap and the CONSTRAINTS amendment ordering; DAG, numbering, and decisions otherwise sound.
MILL_REVIEW_END
