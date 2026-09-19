# Batch: loom run --no-attach

```yaml
task: "Worktree spawn/teardown as Shed producers"
batch: "loom run --no-attach"
number: 4
cards: 2
verify: go test ./internal/loomcli/...
depends-on: []
```

## Batch Scope

This batch adds one flag to one existing verb: `lyx loom run --no-attach` performs every step the verb performs today through the bootstrap handshake, then returns instead of handing the terminal to tmux.
It is independent of batches 1 through 3 — it touches `internal/loomcli` alone and nothing in the lifecycle packages — and is a dependency of batch 5, whose spawn closure runs exactly this command as a child process it waits for.

It is one batch because the change is a single flag, a single pure predicate, and the tests for both.
The external interface batch 5 consumes is the flag's spelling and its guarantee: the process returns once its own handshake confirms a detached driver took the run lock.

Batch-local decision: the attach decision is extracted into a pure predicate in `internal/loomcli/bootstrap.go` rather than written inline in the verb body, matching that file's own stated purpose — every piece there is a pure function or takes injected seams, so the verb body stays assembly over judgment that is already under test.

## Cards

### Card 19: the --no-attach flag and its predicate

- **Context:**
  - `internal/loomcli/cli.go`
  - `internal/loomcli/sharedbootstrap.go`
  - `CONSTRAINTS.md`
- **Edits:**
  - `internal/loomcli/run.go`
  - `internal/loomcli/bootstrap.go`
  - `internal/loomcli/bootstrap_test.go`
  - `internal/loomcli/cli_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `internal/loomcli/bootstrap.go` add `func mustAttach(noAttach bool) bool`, returning `!noAttach`, the twin of the existing `mustSpawnDriver` predicate in that same file, with a doc comment stating the terminal handover is the CLI/Cobra Invariant's narrow interactive-handoff exception and that skipping it removes an exception rather than adding one.
  In `internal/loomcli/run.go`, declare a `noAttachFlag` bool alongside the existing `parentFlag`, register it with `cmd.Flags().BoolVar` under the name `no-attach` and a usage string saying the verb performs every bootstrap step and returns once the driver has taken the run lock, instead of handing the terminal to the session.
  In the `RunE` body, after the step that releases the bootstrap lock at the top of the terminal-handover tail and before the call that reads the reed session's status, return `nil` when `mustAttach(noAttachFlag)` reports false.
  Nothing before that point changes: the parent resolution, the seed and its commit, the bootstrap lock, the status strand, the driver spawn, and the handshake all run byte-identically whether or not the flag is set, because the child this flag exists for depends on exactly those steps having happened.
  Extend `internal/loomcli/bootstrap_test.go` with a table covering `mustAttach` in both directions.
  Extend `internal/loomcli/cli_test.go`'s existing alias-flag assertion so it checks the alias command exposes `no-attach` as well as `parent`, since the alias takes the subtree's own `runCmd` unchanged and would otherwise silently diverge, and add an assertion that the `run` verb under the `loom` parent registers the `no-attach` flag.
  Do not change the long help text's four-step description beyond adding one sentence naming what the flag skips.
- **Commit:** `feat(loomcli): add --no-attach to the run verb`

### Card 20: the byte-identical-without-the-flag proof

- **Context:**
  - `internal/loomcli/run.go`
  - `internal/loomcli/bootstrap.go`
  - `internal/loomcli/cli_test.go`
  - `internal/loomcli/parity_test.go`
- **Edits:**
  - `internal/loomcli/bootstrap_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Add a test asserting the flag's default is false, so an invocation that does not pass it takes today's attach path unchanged — the regression this flag most plausibly causes is a silently flipped default, and nothing else in the package would catch it.
  Assert it against the built command tree's own flag lookup rather than against the package variable, so the assertion covers registration and default together.
  Pair it with an assertion that `mustAttach` over that same default reports true, which is what ties the registered default to the branch it controls.
  Keep both cases in `internal/loomcli/bootstrap_test.go` alongside the predicate they concern, and keep them untagged: they read a cobra flag and call a pure function, spawning nothing.
- **Commit:** `test(loomcli): pin the --no-attach default so today's attach path is unchanged`

## Batch Tests

`verify:` runs `go test ./internal/loomcli/...`, covering `bootstrap_test.go` and `cli_test.go`, the two files this batch edits, alongside the package's other untagged tests that share the command tree.
The batch adds no `integration`- or `smoke`-tagged test, and that is a decision rather than a gap.
The tail this flag skips is the CLI/Cobra Invariant's registered interactive-handoff exception: it hands the operator's stdio to a `tmux attach-session` child, which no tier in this repo drives today and which therefore has no existing coverage for a new test to extend.
What is testable is what this batch tests — the predicate that selects the branch, and the registered default that keeps every existing invocation on today's path.
Batch 5's `integration`-tagged end-to-end stubs at the `LoomRunDeps` field level with a no-op spawn, so it deliberately never launches this verb as a real child either.

