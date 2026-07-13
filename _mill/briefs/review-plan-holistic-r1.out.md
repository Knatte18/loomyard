MILL_REVIEW_BEGIN
# Review: Speed up git-fixture tests: bench, analyse, hardlink — holistic

```yaml
verdict: APPROVE
reviewer_model: opushigh
reviewed_file: plan/
date: 2026-07-13
```

## Findings

### [NIT] allowedNonHermetic guard-file entry semantics ambiguous
**Location:** Batch 3, Card 5
**Issue:** `cmd/lyx/hermeticenv_test.go` is added to `allowedNonHermetic`, but the guard is package-granular ("every git-spawning *package* must have HermeticGitEnv"); discussion says cmd/lyx is "not allowlisted", and cmd/lyx already passes (real TestMain in card 9 + the guard file's own `HermeticGitEnv` token), so the entry is redundant and, if read as a *package* exemption, would silently drop cmd/lyx's e2e git tests from the requirement.
**Fix:** Card 5 should state the entry excludes only that file from the git-spawn *scan* (mirroring tierpurity's per-file `spawnerAllowed`), never exempting the cmd/lyx package.

### [NIT] Full-hermetic breakage has no verify gate or fallback in-plan
**Location:** Batch 3 (cards 3/5) → Batch 4, Card 12
**Issue:** Discussion decision `neutral-global-config-contents` warns `GIT_CONFIG_NOSYSTEM=1` may break a test depending on system config (Git-for-Windows autocrlf) and prescribes an additive `GIT_CONFIG_COUNT` fallback; no batch verify runs the full Tier 2 suite — the first full run is card 12's *manual* `testtiming -full` — and the fallback contingency is not carried into any card.
**Fix:** Note the fallback in card 3 (or card 12) as the contingency if the full run goes red.

### [NIT] Card 10 forward-references a doc created in batch 4
**Location:** Batch 3, Card 10
**Issue:** The CONSTRAINTS.md/`doc.go` text points at `docs/benchmarks/fixture-copy.md`, which is not created until batch 4 card 12; the link dangles between batch 3 and 4 (final merged state is consistent).
**Fix:** Acceptable as-is; optionally note the target lands in batch 4.

### [NIT] Card 4 Context omits helper source files
**Location:** Batch 2, Card 4
**Issue:** Requirements reference `MustRun`, `CopyHostHub`, and `HermeticGitEnv`; only the edited `lyxtest_test.go` is accessible (it demonstrates the first two), while `HermeticGitEnv` (from card 3's `hermetic.go`) and the helper signatures in `lyxtest.go` are not in Context.
**Fix:** Add `internal/lyxtest/lyxtest.go` and `internal/lyxtest/hermetic.go` to card 4 Context.

## Verdict

APPROVE
Package set, DAG, sequencing, and decision alignment all verified; only clarity NITs remain.
MILL_REVIEW_END
