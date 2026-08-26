# Batch: cli-convergence

```yaml
task: 'reed: attach doesn''t reconcile session geometry with the terminal'
batch: 'cli-convergence'
number: 3
cards: 4
verify: go build ./... && go test ./internal/reedcli/... ./internal/loomcli/... ./cmd/lyx/...
depends-on: [2]
```

## Batch Scope

This batch converges the two duplicated attach-argv builders onto the one the engine now owns.
It adds `golang.org/x/term` as a new direct module requirement, teaches both CLI pre-flights to read the operator's own terminal size and hand it to `Engine.AttachArgv`, and deletes `internal/loomcli`'s copied builder along with both builders' tests.

After this batch the observable behaviour of `lyx reed attach` and `lyx loom run` changes for the first time in this task: on a real TTY the handover argv carries a chained `select-layout` sized to that terminal.
Nothing downstream consumes a new interface — batch 4 only documents and proves what this batch ships.

Batch-local decisions:
- The terminal-size read and the `AttachArgv` call both sit in the **pre-flight** half of each `RunE`, above the terminal handover — but neither reports on the JSON envelope.
  Each degrades to today's behaviour and logs a warning instead.
  The existing `Status()` call stays the one pre-flight step that can abort with an envelope error, so the CLI/Cobra Invariant's "one envelope per invocation" property is unchanged.
- `internal/loomcli` already imports `internal/reedengine`, so calling the engine's builder adds no import edge.

## Cards

### Card 8: add golang.org/x/term as a direct module requirement

- **Context:**
  - `internal/fslink/fslink_windows.go`
- **Edits:**
  - `go.mod`
  - `go.sum`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Add `golang.org/x/term` to the FIRST (direct) require block in `go.mod`, alphabetically beside the existing `golang.org/x/sys v0.45.0` line.
  This is a new direct requirement, not a promotion from indirect: `go.mod` has no `golang.org/x/term` line in either require block today, and no Go source in the repo imports it — only `go.sum` carries it, as a transitive artefact.
  Run `go get golang.org/x/term@v0.42.0` to add it, pinning the version `go.sum` already records with both an `h1:` module hash and a `/go.mod` hash, so no new checksum has to be fetched.
  If the module proxy is unreachable, add the require line by hand and run `go mod download golang.org/x/term` followed by `go mod tidy`;
  do not upgrade `golang.org/x/sys` as a side effect, and do not let `go mod tidy` rewrite unrelated lines.
  Confirm afterwards that `go build ./...` still succeeds and that the second (indirect) require block is unchanged.
  The dependency is a two-function wrapper (`GetSize`, `IsTerminal`) maintained by the Go team over `golang.org/x/sys`, which is already a direct dependency — so this edge adds no new transitive subtree, only a name in `go.mod`.
- **Commit:** `build: add golang.org/x/term as a direct requirement for the terminal-size read`

### Card 9: lyx reed attach reads its terminal size and calls the engine builder

- **Context:**
  - `internal/reedengine/attach.go`
  - `internal/reedcli/cli.go`
  - `go.mod`
- **Edits:**
  - `internal/reedcli/attach.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Delete the package-level `attachArgv` function from `internal/reedcli/attach.go` entirely.

  In the `RunE`, after the existing `c.eng.Status()` pre-flight and before the `exec.Command` handover, read the operator's terminal size with `golang.org/x/term`'s `GetSize` against `os.Stdout`'s file descriptor — `term.GetSize(int(os.Stdout.Fd()))`.
  On an error (piped output, no controlling terminal), do not report on the envelope and do not abort: log via `logger.Warn` that no terminal size is available and use `0, 0`, which `AttachArgv` answers with the bare argv — exactly today's behaviour, so nothing regresses on a non-TTY.
  Then build the command as `exec.Command(c.eng.TmuxPath(), c.eng.AttachArgv(cols, rows)...)`, leaving the `Stdin`/`Stdout`/`Stderr` wiring and the exit-code propagation in the handover tail unchanged.

  Update the command's `Long` so it stays accurate under the CLI/Cobra Invariant's help-accuracy obligation: state that the handover also asks tmux to apply a layout computed for this terminal's own size, and that when no terminal size is readable the attach proceeds exactly as before.
  Keep the existing `Short`, the existing `Example:` block, and the `clihelp.ShouldAbort` check as the `RunE`'s first statement.

  Update the file-header comment to record that the argv now comes from `internal/reedengine`, and that the size read and the builder call are pre-flight steps which nonetheless never write to the envelope — each degrades and warns instead, so the `Status()` call remains the only pre-flight step that can abort with an envelope error.
- **Commit:** `feat(reedcli): hand the terminal size to the engine's attach argv builder`

### Card 10: lyx loom run drops its copied builder and calls the engine's

- **Context:**
  - `internal/reedengine/attach.go`
  - `internal/reedcli/attach.go`
  - `internal/loomcli/cli.go`
  - `go.mod`
- **Edits:**
  - `internal/loomcli/bootstrap.go`
  - `internal/loomcli/run.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Delete the `attachArgv` function at the end of `internal/loomcli/bootstrap.go` — the verbatim copy of the reedcli builder — and drop the `internal/reedengine` import from that file only if nothing else there still uses it (`findStatusStrand` and `resolveStatusStrandAction` both reference `reedengine.StrandStatus`, so the import almost certainly stays).
  Update `bootstrap.go`'s file-header comment: it currently says the file holds "the two command-string builders the bootstrap composes over", which is one builder after this change.

  In `internal/loomcli/run.go`, in step 7's terminal-handover block, read the terminal size with `term.GetSize(int(os.Stdout.Fd()))` after the existing `c.reed.Status()` call and before `exec.Command`, degrading to `0, 0` with a `logger.Warn` on error exactly as `internal/reedcli/attach.go` does, and build the command as `exec.Command(c.reed.TmuxPath(), c.reed.AttachArgv(cols, rows)...)`.
  Leave the `bootstrapLock.Release()` ordering, the `Status()` envelope abort, the stdio wiring and the exit-code propagation exactly as they are;
  this card adds no new fallible step that reports on the envelope, so step 7 keeps its interactive-handoff exception unchanged.
  Do not touch the driver-spawn handshake, `awaitRunLock`, or `dispositionForHandshake`.
- **Commit:** `refactor(loomcli): drop the duplicated attach argv in favour of the engine builder`

### Card 11: retire both deleted builders' tests

- **Context:**
  - `internal/loomcli/bootstrap.go`
  - `internal/reedcli/attach.go`
- **Edits:**
  - `internal/reedcli/cli_test.go`
  - `internal/loomcli/bootstrap_test.go`
  - `internal/reedengine/attach_test.go`
  - `internal/reedengine/windowsize_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Delete `TestAttachArgv` from `internal/reedcli/cli_test.go` and `TestAttachArgv` from `internal/loomcli/bootstrap_test.go`.
  Both pin the bare `-L <socket> attach-session -t =<session>` argv of functions that no longer exist, so they stop compiling with card 9 and card 10;
  their coverage is not lost, it moved into `internal/reedengine/attach_test.go`'s no-size and no-layout cases, which assert exactly that argv.
  Keep both files themselves — every other test in each stays.
  Update `internal/reedcli/cli_test.go`'s file-header comment, which currently names "the built attach invocation" as one of the three things the file covers, and remove any now-unused import each file is left with.
  Do not delete or weaken any other test in either file.

  Verify surfaced a pre-existing defect from batch 2: `internal/reedengine/attach_test.go`'s and `internal/reedengine/windowsize_test.go`'s file-header comments each document, in prose, that the file drives its cases "through TmuxCmd's execHook seam" with "no exec.Command" — but `cmd/lyx/tierpurity_test.go` and `cmd/lyx/hermeticenv_test.go` scan for the banned token as a raw substring, including inside comments, so the literal string "exec.Command" in prose trips both guards even though neither file's actual test code spawns a process.
  Reword each occurrence of the literal substring `exec.Command` in both files' header comments to an equivalent description that avoids the token (for example, "no live tmux server, no external process spawn") — a wording fix only, no test behavior changes.
- **Commit:** `test: retire the two attach-argv tests whose builders the engine replaced`

## Batch Tests

`verify: go build ./... && go test ./internal/reedcli/... ./internal/loomcli/... ./cmd/lyx/...`

The `go build ./...` half is the cheap whole-repo compile gate this batch specifically needs: card 8 edits `go.mod`, and cards 9-11 delete two package-level functions, so a stale reference anywhere in the tree must fail loudly rather than hide behind a scoped test run.

The test half is scoped to the three packages this batch can affect:
- `internal/reedcli/...` covers the edited `attach.go` and `cli_test.go` (the remaining `RunCLI` no-args, unknown-subcommand and not-a-git-repo cases), plus `cli_integration_test.go`.
- `internal/loomcli/...` covers the edited `bootstrap.go`/`run.go`/`bootstrap_test.go`, plus `wiring_test.go`, `parity_test.go` and `validate_test.go`, which exercise the same command tree.
- `cmd/lyx/...` is not incidental: the CLI/Cobra Invariant's guards live there, and this batch changes a command's `Long` and both packages' import sets.
  `helptree_test.go`, `drift_test.go`, `registration_test.go`, `longlist_test.go` and `seamsignature_test.go` must all stay green with no command added or renamed, and `tierpurity_test.go` must confirm no untagged test file picked up a banned spawn token.

`internal/reedengine/...` is deliberately not re-run here — batch 2's verify already covered it and this batch changes nothing under it.
The `smoke`-tagged suites in `internal/reedcli` and `internal/loomcli` are not run either: they need a real hub and a real multiplexer, and the tier-2 proof for this task is the `integration`-tagged test batch 4 adds.
