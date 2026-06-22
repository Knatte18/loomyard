MILL_REVIEW_BEGIN
# Review: Optimise and slim the test suite

```yaml
verdict: GAPS_FOUND
reviewer_model: opushigh
reviewed_file: _mill/discussion.md
date: 2026-06-20
```

## Findings

### [GAP] Add env→option contract is self-contradictory
**Section:** Decisions › parallelism via layered env→param; Technical context
**Issue:** The discussion says `Add` "maps env→option when it calls `pushWeftBranch`" (i.e. `Add` itself reads env) yet also that "tests call ... `Add` with the option passed directly — no `t.Setenv`" (i.e. `Add`'s signature gains the option) — these are mutually exclusive: if `Add` reads env internally, parallel tests must still `t.Setenv`. Verified `func (w *Worktree) Add(l *paths.Layout, slug string)` (add.go:59) has no env read today.
**Fix:** State explicitly whether `Add`'s signature gains a skipPush/skipGit parameter (option), and if so, who reads env at the edge — see next finding.

### [GAP] Unlisted env→option call site at worktree/cli.go:90
**Section:** Technical context › "Call sites that gain a NEW env→option read"
**Issue:** That list names only `weft/cli.go` and `add.go`, but if `Add` gains an explicit option, its sole production caller `w.Add(l, slug)` at `internal/worktree/cli.go:90` must also map env→option — otherwise the CLI loses the `WEFT_SKIP_PUSH`/`WEFT_SKIP_GIT` contract on the real Add path. This call site is omitted.
**Fix:** Add `worktree/cli.go` (the `w.Add` call) to the list of call sites gaining a new env→option read.

### [NOTE] Inconsistent build-tag treatment of equivalent helper files
**Section:** Decisions › build-tag gating › Explicit classification
**Issue:** worktree `helpers_test.go`/`testhelpers_test.go` are tagged `integration`, but paths `helpers_test.go` is left untagged with rationale "no git itself" — yet both define `newTestRepo`/`mustRun` that spawn git identically (verified paths/helpers_test.go:42-71). The rationale is misleading (the funcs do spawn git; they just aren't invoked at load) and the treatment differs for equivalent files.
**Fix:** State the real criterion (helper definitions don't execute, so an unused untagged helper compiles) and apply it consistently, noting these helpers migrate to `lyxtest` regardless.

## Verdict
GAPS_FOUND
The Add env→option refactor is contradictory and omits the worktree/cli.go call site.
MILL_REVIEW_END
