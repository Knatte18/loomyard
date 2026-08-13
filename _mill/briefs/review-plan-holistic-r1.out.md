MILL_REVIEW_BEGIN
# Review: Unblock t.Parallel on hub-fixture tests that currently t.Chdir — holistic

```yaml
verdict: APPROVE
reviewer_model: sonnetxhigh
reviewer_self_id: Claude Sonnet 4.5 (Anthropic), self-assessed
reviewed_file: plan/
date: 2026-08-13
```

## Findings

### [NIT:consistency] Card 7 miscounts the rebase call sites as six
**Location:** batch 02-module-seams, card 7 **Issue:** "Pass the seam `cwd` as `base` at all six call sites" lists only five line numbers (:161, :163, :291, :293, :580) — verified against `internal/scoutcli/cli.go`, there are exactly five `parseQuery`/`inFileQuery` call sites outside the two function definitions, not six. **Fix:** Change "six call sites" to "five call sites"; the listed line numbers themselves are complete and correct.

### [NIT:consistency] Card 17 miscounts sibling `go env GOMOD` allowlist entries as four
**Location:** batch 03-test-migration-guard, card 17 **Issue:** "the same reason four sibling guards on that map already carry" — verified against `cmd/lyx/tierpurity_test.go`'s `allowedSpawners`, only three existing entries (`gitrepoboundary_test.go`, `boardguard_test.go`, `destructiveguard_test.go`) state the "resolves its scan root via `go env GOMOD`" reason. **Fix:** Change "four sibling guards" to "three sibling guards"; the instruction to add the new entry is otherwise correct.

## Verdict

APPROVE
Every checked line/function citation across all three batches verifies exactly against source; only two trivial miscounted-list NITs found.
MILL_REVIEW_END
