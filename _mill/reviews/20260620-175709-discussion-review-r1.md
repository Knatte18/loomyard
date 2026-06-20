MILL_REVIEW_BEGIN
# Review: Optimise and slim the test suite

```yaml
verdict: GAPS_FOUND
reviewer_model: opushigh
reviewed_file: _mill/discussion.md
date: 2026-06-20
```

## Findings

### [GAP] CLI does not currently read WEFT_SKIP_* env vars
**Section:** Decisions › parallelism via layered env→param
**Issue:** The decision says the "CLI push/sync subcommand … keep reading the env vars and pass them through as the option," but `internal/weft/cli.go` (lines 66, 106, 113, 117, 123, 129) calls `Commit`/`Push`/`Pull` with no env read at all — the env check lives *inside* the functions today. After moving the env read out, every CLI call site must be newly taught to read env and pass the option; this is a new code change the discussion frames as a no-op ("keep").
**Fix:** State that the CLI dispatcher (cli.go) gains the env→option mapping at each Commit/Push/Pull call site (it has none now), not merely "keeps" it.

### [GAP] pushWeftBranch refactor target unspecified
**Section:** Decisions › parallelism via env→param; Technical context
**Issue:** The Add path env read is in `pushWeftBranch` (`internal/worktree/weft.go:208`), not a `Commit`/`Push`/`Pull` function; the decision text only names the three sync.go functions plus "the weft path in worktree/weft.go" without specifying whether `pushWeftBranch` takes the new option and where its caller (`Add`) maps env→option for parallel `TestAdd` cases.
**Fix:** Name `pushWeftBranch` explicitly, give its new signature, and state which caller maps env→option so the paired-Add tests can pass `skipPush` without `t.Setenv`.

### [NOTE] Bare-remote isolation mechanism left as alternatives
**Section:** Decisions › fixture-amortisation
**Issue:** Remote isolation is described as "one `git remote set-url`, or copy the bare and rewrite" — two approaches offered without a pick; `git remote set-url` reintroduces a per-test git spawn, partly undercutting the zero-spawn goal.
**Fix:** Choose one (copy-bare + rewrite to preserve zero per-test git spawns) and state it.

### [NOTE] Equivalence guardrail capture method undefined
**Section:** Testing › Equivalence guardrail
**Issue:** "record the full list of test names and pass result per package" names no mechanism (e.g. `go test -list`, JSON output) for capturing/diffing before vs after, leaving the guardrail's rigor to interpretation.
**Fix:** Name the capture command (e.g. `go test -tags integration -list '.*'` snapshot diffed pre/post).

## Verdict
GAPS_FOUND
Two CLI/Add env-mapping migration points are under-specified; fixture isolation and guardrail mechanics need a pick.
MILL_REVIEW_END
