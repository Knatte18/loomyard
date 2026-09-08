# Done-gate failure brief

**Task:** Unify webster/burler CLI wiring into a shared module
**Worktree:** `/home/knatte/Code/loomyard/wts/unify-webster-burler-wiring`
**Plan overview:** `/home/knatte/Code/loomyard/wts/unify-webster-burler-wiring/_mill/plan/00-overview.md` (read this for the full plan, Shared Decisions, and "All Files Touched" list)

## The `done_gate` command

```
go test ./... && go test -tags integration ./...
```

## Failure output

The untagged half (`go test ./...`) passes cleanly. The tagged half fails:

```
$ go test -tags integration -run TestRootHookWritesTraceFileOnNonZeroExit -v ./cmd/lyx/...
=== RUN   TestRootHookWritesTraceFileOnNonZeroExit
    main_integration_test.go:155: ReadDir("/tmp/TestRootHookWritesTraceFileOnNonZeroExit793633235/002/.lyx/logs"): open /tmp/TestRootHookWritesTraceFileOnNonZeroExit793633235/002/.lyx/logs: no such file or directory; lyx output: {"error":"unknown command \"bogus-subcommand\" for \"lyx\"","ok":false}
--- FAIL: TestRootHookWritesTraceFileOnNonZeroExit (1.13s)
FAIL
FAIL	github.com/Knatte18/loomyard/cmd/lyx	1.136s
FAIL
```

## Important: this failure pre-dates the task's diff

Before dispatching you, the Builder verified this via `git archive` of the parent branch
(`crucible-loom-glyph-hardening`) into a clean scratch snapshot and ran the same test there —
**it fails identically on the parent branch**, with an unmodified `cmd/lyx/main_integration_test.go`.
This task's own diff touches only `cmd/lyx/prerunlogging_test.go` (a 9-line test-only change) under
`cmd/lyx/` — see the plan's "All Files Touched" list, which does not include
`main_integration_test.go` or any root-hook/trace-file production code.

So this is very likely a pre-existing failure unrelated to this task's diff, not a regression the
plan's batches missed. Investigate from first principles per your own instructions, but weigh this
evidence: if you confirm the failure is unrelated to webster/burler CLI wiring and pre-exists on the
parent branch, the correct fix is whatever makes the root-hook/trace-file invariant hold in general
(same standard as any other done-gate fix) — do not weaken the test to make it pass, and do not treat
"pre-existing" as a reason to skip investigating; the gate must pass regardless of blame.
