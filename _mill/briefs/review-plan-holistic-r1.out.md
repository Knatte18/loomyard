MILL_REVIEW_BEGIN
# Review: Build modelspec - the model-spec parser + registry — holistic

```yaml
verdict: REQUEST_CHANGES
reviewer_model: opushigh
reviewed_file: plan/
date: 2026-07-11
```

## Findings

### [BLOCKING] Seed-only materialize can't be byte-identical
**Location:** Batch 2, Card 9 (absent-file branch + test (a))
**Issue:** Card 9 routes the SeedOnly+absent case through the existing `yamlengine.Reconcile` path and claims it "yields the template verbatim," then test (a) asserts the written file is "byte-identical to `modelspec.ConfigTemplate()`." Reconcile round-trips through `yaml.Node`→`yaml.Marshal`, which re-indents nested `defaults:` to yaml.v3's default 4-space, drops blank lines between aliases, and can relocate the header/`# zephyr` example comments — so byte-identity fails and the operator-facing annotated seed (the whole point of a seed-only module) is degraded.
**Fix:** For `SeedOnly && absent`, write `m.Template()` verbatim via `fsx.AtomicWriteBytes` (report all keys in `Added`, `Applied=true`) instead of falling through to `Reconcile`; then test (a)'s byte-identity assertion is valid and the annotated seed reaches the operator intact.

### [NIT] modelspec.ConfigTemplate referenced but not in Context
**Location:** Batch 2, Cards 8 and 9
**Issue:** Both cards use `modelspec.ConfigTemplate` but `internal/modelspec` is absent from their `Context:`/`Edits:` (mitigated: the `func() string` signature is stated inline, and it is a batch-1 cross-dependency).
**Fix:** Add `internal/modelspec/template.go` to each card's `Context:` for read-access completeness.

## Verdict

REQUEST_CHANGES
Seed-only absent materialization must write the template verbatim, not via Reconcile.
MILL_REVIEW_END
