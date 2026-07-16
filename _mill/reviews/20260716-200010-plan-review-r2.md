MILL_REVIEW_BEGIN
# Review: Fork-based cluster review in burler — holistic

```yaml
verdict: REQUEST_CHANGES
reviewer_model: opushigh
reviewed_file: plan/
date: 2026-07-16
```

## Findings

### [BLOCKING] AuditForks widens Engine; 5 other doubles unpatched
**Location:** Batch 2, Card 5
**Issue:** Adding `AuditForks` to `shuttleengine.Engine` breaks five external compile-time implementers Card 5 does not touch — `specCapturingEngine` (shuttlecli/cli_test.go:169), `fakeEngine` (builderengine/poll_test.go:139), `spawnFakeEngine` (builderengine/spawn_test.go:115), `pollFakeEngine` (buildercli/poll_test.go:52), `spawnFakeEngine` (buildercli/spawnbatch_test.go:84); batch 2's scoped verify (`./internal/shuttleengine/...`) hides it until batch 5's `go test ./...` fails to compile.
**Fix:** Card 5 must add an `AuditForks` stub to all five doubles and list those files in Edits + `All Files Touched` (or split AuditForks off the core Engine interface).

### [LOW] template.go marker-count doc goes stale
**Location:** Batch 4, Card 9
**Issue:** Card 9 adds a 9th top-level marker (`{{.cluster_rules}}`) but only updates the markdown template's header comment; `template.go`'s own doc ("static prose around eight top-level markers") and prompt.go's header ("eight marker values") are not in Card 9's Edits, so `template.go` is left stating a stale count.
**Fix:** Add `internal/burlerengine/template.go` to Card 9's Edits and update both marker-count comments alongside prompt.go's.

## Verdict

REQUEST_CHANGES
One BLOCKING interface-widening gap breaks five unpatched Engine doubles at the terminal `go test ./...`.
MILL_REVIEW_END
