# Done-gate fix brief: fabric-clone-commit-module-configs

## Task context

Task: fabric: clone doesn't commit written module configs.
Worktree: `/home/knatte/Code/loomyard/wts/fabric-clone-commit-module-configs`.
Branch: `fabric-clone-commit-module-configs` (parent: `main`).
Full plan: `_mill/plan/00-overview.md` (read it for the batches and Shared Decisions).

All 3 batches are implemented and holistically approved (0 blocking, 0 nits).
`mill-go`'s Handoff then ran the task-wide `pipeline.done_gate` command as a final gate, and it failed.

## Important: this is NOT a regression this task's diff introduced

Unlike this agent's usual brief, this failure is not something the task's batches broke.
The plan's own `00-overview.md` already documents it under the Shared Decision
`preexisting-stencilcli-failure-blocks-done-gate`:

> `internal/stencilcli`'s `TestStencilCLI_ListAndValidate` fails deterministically on this branch's
> tip **before** any of this task's work, and nothing in this plan fixes it.
> The failure is a stencil-marker validation defect in `internal/stencilcli`, unrelated to fabric's
> clone path, and confirmed reproducible with the worktree clean at commit `2d98c05d` (i.e. it also
> fails on `main`).

The operator has now decided to fix it anyway, as a standalone fix, so the done gate can pass and
this task can be finalized.
Treat this exactly as you would any other real bug: investigate from first principles, fix it
properly, do not just make the assertion pass.

## The done_gate command

```
go test ./... && go test -tags integration ./...
```

## The failure output

```
--- FAIL: TestStencilCLI_ListAndValidate (0.24s)
    cli_integration_test.go:201: validate findings missing the expected warning finding for "loom-template-discussion"; got [map[marker:ZZZ_UNKNOWN_MARKER name:landing-template-conflict severity:error]]
FAIL
FAIL	github.com/Knatte18/loomyard/internal/stencilcli	0.922s
```

This was the only failing package in the entire `done_gate` run — every other package passed
(including `internal/fabriccli`, `internal/fabricengine`, `internal/hubforge`, `internal/preflight`,
`internal/preflightshed`, `internal/configengine` — this task's own touched packages).

## What to do

1. Investigate `internal/stencilcli`'s `TestStencilCLI_ListAndValidate` (in
   `internal/stencilcli/cli_integration_test.go` around line 201) and whatever stencil-marker
   validation logic it exercises.
   Understand why the "loom-template-discussion" warning finding is missing and why a
   `ZZZ_UNKNOWN_MARKER` / `landing-template-conflict` error finding is showing up instead.
2. Fix the actual site — production code, test fixture, or both — so the invariant holds.
   Prefer fixing the underlying construction/validation logic over patching the assertion.
3. Re-run the full `done_gate` command yourself (`go test ./... && go test -tags integration ./...`
   from the repo root) before reporting done — not just `internal/stencilcli`.
   A local fix that breaks something else elsewhere is not done.
4. Commit your fix(es) on the current branch (`fabric-clone-commit-module-configs`) via the
   `git-commit` skill (lint + codeguide-update per commit).
   Never push.
5. If `manifest/designs/` docs or `CONSTRAINTS.md` describe the invariant you touched, update them
   in the same commit per this repo's Documentation Lifecycle rule in `CLAUDE.md`.

## Test Integrity Guardrail

Never weaken, relax, exclude, downgrade, or delete test assertions, conformance checks, or allowlist
entries to make the gate pass.
Fix the code or the fixture properly; never gut coverage to go green.

## Shell conventions

Never use `sed`.
Use `Edit`/`Read`/`Write`, or `awk`/`grep`/plain `cat` for a genuine one-liner.

## Report

Final chat message: a concise summary of the root cause, the fix, the commit SHA(s), and explicit
confirmation that you re-ran the full `done_gate` command and it passed.
Do not paste full test output.
