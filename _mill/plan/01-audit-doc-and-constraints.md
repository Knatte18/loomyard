# Batch: audit-doc-and-constraints

```yaml
task: "Audit internal/logger coverage across spawn/hard-error paths"
batch: "audit-doc-and-constraints"
number: 1
cards: 2
verify: go test ./internal/lyxcwd/ -run TestEnforcement_MarkdownLinks
depends-on: []
```

## Batch Scope

This batch delivers the survey half of the task: the enumerated audit table at `manifest/designs/logger-coverage.md`, and the CONSTRAINTS.md prose sharpening of the **Live-Substrate Spawn Observability** invariant that the survey's verdicts justify.
It is one batch and the DAG root because `_mill/discussion.md`'s `deliverable-shape` decision sequences the work document-first: every later batch's code change is derived from a verdict recorded here, and the audit table's exclusion reasons are what stop the next reader re-litigating `gitexec`.
The external interface later batches consume is the verdict set itself — batches 2, 3 and 4 implement the `add` rows, and batch 5's guard allowlist carries one entry per `blocked` and `excluded` row that lies inside its walk scope.

Batch-local decision differing from `## Shared Decisions`: nothing in this batch is Go source, so the `log-line-style` and `level-policy` decisions do not bind any card here — they are what the document *records*, not what it applies.

## Cards

### Card 1: Write the logger-coverage audit document

- **Context:**
  - `_mill/discussion.md`
  - `manifest/designs/review-finding-classification.md`
  - `manifest/designs/curation-triage.md`
  - `CLAUDE.md`
  - `internal/shedadapters/singlellm.go`
  - `internal/shedadapters/burler.go`
  - `internal/shedadapters/bouncer.go`
  - `internal/burlerengine/engine.go`
  - `internal/treadleengine/run.go`
  - `internal/treadleengine/judge.go`
  - `internal/treadleengine/targeting.go`
  - `internal/treadleengine/gate.go`
  - `internal/mergeresolve/mergeresolve.go`
  - `internal/websterengine/runlevel.go`
  - `internal/websterengine/integration.go`
  - `internal/boardengine/spawn.go`
  - `internal/configengine/edit.go`
  - `internal/vscode/launch_linux.go`
  - `internal/vscode/launch_windows.go`
  - `internal/reedengine/proctree_windows.go`
  - `internal/reedengine/lifecycle.go`
  - `internal/reedengine/overlay.go`
  - `internal/reedcli/attach.go`
  - `internal/reedengine/attach.go`
  - `internal/reedengine/doc.go`
  - `internal/githubclient/doc.go`
  - `internal/loomcli/run.go`
  - `internal/fabricengine/spawn.go`
  - `internal/gitexec/gitexec.go`
  - `internal/gitkit/gitkit.go`
  - `internal/githubclient/token.go`
  - `internal/selfreportengine/selfreport.go`
  - `internal/landingshed/publish.go`
  - `internal/hubforge/hub.go`
  - `cmd/testtiming/main.go`
  - `tools/deploy/main.go`
  - `tools/sandbox/main.go`
  - `tools/sandbox/suite.go`
- **Edits:** none
- **Creates:**
  - `manifest/designs/logger-coverage.md`
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Create `manifest/designs/logger-coverage.md`, a cross-cutting design document in the shape of the existing non-per-module documents in that directory (`review-finding-classification.md`, `curation-triage.md`).
  It must contain, in this order:

  1. A short framing section stating what the document is (the enumerated coverage survey behind the **Live-Substrate Spawn Observability** invariant) and why it exists (the durable trace file written by `internal/logger` is the raw material for bug reporting, and it only works if the events that matter reach it).
  2. A **Selectors** section stating both mechanical selectors verbatim, so a later reader can re-run them rather than trust the tables.
     The spawn selector: every `*ast.CallExpr` in a production (non-`_test.go`) `.go` file under `internal/` and `cmd/` whose `Fun` is an `*ast.SelectorExpr` whose `X` matches the file's own `os/exec` import name and whose `Sel.Name` is `Command` or `CommandContext`.
     The hard-error selector: every `*ast.SwitchStmt` any of whose `case` expressions is a selector whose `X` matches the file's `shuttleengine` import name and whose `Sel.Name` begins `Outcome`, plus every `*ast.BinaryExpr` with `==`/`!=` where either operand is such a selector.
     State that both are AST rather than grep because a doc-comment mention is not a call, and name the three files that hit the `exec.Command` substring with zero real calls: `internal/githubclient/doc.go`, `internal/reedengine/doc.go`, `internal/reedengine/attach.go`.
  3. A **Spawn sites** table with one row per site the spawn selector yields, columns `Site`, `Calls`, `Waits?`, `Verdict`.
     The verdict vocabulary is exactly `covered`, `add`, `excluded (<reason>)`, `blocked (<reason>)`.
     Rows, with the verdicts this task implements: `internal/reedengine/lifecycle.go` 1 covered; `internal/reedengine/overlay.go` 2 covered; `internal/reedcli/attach.go` 1 covered; `internal/loomcli/run.go` 2 covered; `internal/fabricengine/spawn.go` 1 covered; `internal/websterengine/integration.go` 1 waits via `Run` add (`Info` spawn + teardown); `internal/treadleengine/gate.go` 1 waits via `CombinedOutput` add (`Info` spawn + teardown); `internal/configengine/edit.go` 1 waits via `Run` add (`Info` spawn + teardown); `internal/boardengine/spawn.go` 1 detached add (`Info` spawn only, `Warn` on `Start` failure); `internal/vscode/launch_linux.go` 1 detached add (same shape); `internal/vscode/launch_windows.go` 1 detached add (same shape); `internal/reedengine/proctree_windows.go` 2 waits via `Output` add (`Debug` only); `internal/gitexec/gitexec.go` 1 blocked (import cycle); `internal/gitkit/gitkit.go` 3 blocked (gitkit Leaf Invariant); `internal/githubclient/token.go` 1 blocked (GitHub Auth leaf allowlist); `internal/hubforge/hub.go` 1 excluded (test-fixture builder); `cmd/testtiming/main.go` 1 excluded (test-timing harness); `tools/deploy/main.go` and `tools/sandbox/*` 7 excluded (dev tooling, outside the walk).
     Follow each table row's verdict with prose only where the verdict needs a reason the cell cannot hold.
  4. A **Detached spawns are spawn-only** subsection stating that `internal/boardengine/spawn.go` and both `internal/vscode` launchers call `Start` and never `Wait`, so there is no teardown event to observe and demanding a teardown log at those sites would be unsatisfiable.
  5. A **Hard-error-return sites** table with columns `Site`, `Non-Done handling`, `Verdict`, one row per hard-error selector yield: `internal/shedadapters/singlellm.go` `mapOutcome` — `Asking`→`Stuck`, `Died`/`Timeout`→error, `default`→error — **add** (`Warn` on `Died`/`Timeout` and on `default`); `internal/websterengine/runlevel.go` `Run`'s Master outcome switch — `Asking`/`Died`/`Timeout`→typed errors, `default`→error — **add** (`Warn` on all four non-`Done` branches); `internal/mergeresolve/mergeresolve.go` `Resolve`'s `!= OutcomeDone` branch → `abortAndStuck` — **add** (`Warn` before `abortAndStuck`); `internal/shedadapters/burler.go` two switches — retry then respawn — covered; `internal/shedadapters/bouncer.go` two comparisons — seed run and judge run did not complete — covered; `internal/treadleengine/run.go` two comparisons — retry on `Died`/`Timeout` — covered; `internal/treadleengine/judge.go` two comparisons — degrade to default verdict — covered; `internal/treadleengine/targeting.go` one comparison — degrade to no seed — covered; `internal/burlerengine/engine.go` one comparison — returns `result, nil` — excluded (a normal loop event, not a hard error; the caller branches).
     Cite sites by file and enclosing function, never by line number.
  6. An **Enforcement asymmetry** section stating plainly that the spawn table is enforced by a tree-wide guard test under `cmd/lyx/` while the hard-error table is document-only, and giving the reason: a new unlogged `exec.Command` is nearly always a real miss, whereas a new outcome-switch branch may legitimately return normally for the caller to branch on (`internal/burlerengine/engine.go` does exactly that), so the same file-level check would fire on correct code.
     State the accepted cost explicitly — the hard-error table will rot and the next author adding an outcome switch will not be told to log it — and name a branch-return-behaviour guard as the obvious follow-up.
  7. An **Untested log lines** section recording that the four `Warn` lines added to `internal/websterengine/runlevel.go`'s Master outcome switch land without a direct test, and why: the switch is inline in `Run` downstream of `SaveState`, the mutation-lease release, and `handle.Wait()`, and the only existing driver is a tagged external fixture that offers no seam for forcing a chosen shuttle outcome; building one would be new production structure in an additive logging change.
  8. A **Structural blocks** section, one subsection per blocked site, each recording the concrete reason: the `gitexec → logger → lyxcwd → gitexec` cycle (with the note that `gitexec.Run` already returns a `*GitError` carrying args, dir, exit code and stderr, so the diagnostic is reconstructable at the caller); the gitkit Leaf Invariant's pinned import list; and the GitHub Auth Invariant's leaf allowlist, together with the note that GitHub failures are logged one layer up in both production callers, `internal/selfreportengine/selfreport.go` and `internal/landingshed/publish.go`.

  Refer to every source file by a backticked path, never by an inline markdown link — see the `audit-doc-uses-backticked-paths` shared decision.
  Write prose with semantic line breaks per `CLAUDE.md`: one sentence per line, breaking inside a long sentence at an internal independent-clause boundary; table cells stay on one line.
- **Commit:** `docs(manifest): add logger-coverage audit document`

### Card 2: Sharpen the Live-Substrate Spawn Observability invariant prose

- **Context:**
  - `_mill/discussion.md`
  - `CLAUDE.md`
- **Edits:**
  - `CONSTRAINTS.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  In `CONSTRAINTS.md`, replace the body of the existing `## Live-Substrate Spawn Observability` section — the single prose sentence beginning "Any code path starting a real OS process for a round/strand/session" together with its two existing sub-bullets — with exactly this text, keeping the `## Live-Substrate Spawn Observability` heading itself unchanged:

```markdown
## Live-Substrate Spawn Observability

Every code path reachable from a `lyx` command that starts a real OS process logs its spawn via `internal/logger`, and logs its teardown wherever it waits for one — `Info` for a lifecycle spawn, `Debug` for a spawn inside a polling probe.

- A detached spawn (`Start` with no `Wait`) logs the spawn alone; there is no teardown to observe.
- A site structurally barred from importing `internal/logger` is exempt, and carries a written reason wherever the exemption is recorded.
- Test-fixture machinery, standalone harnesses, and dev tooling under `tools/` are outside this rule, not exemptions to it.
- Never re-exec `os.Executable()` under `go test`.
- A retry loop around a real spawn caps attempt COUNT, not only elapsed time.
```

  Change nothing else in `CONSTRAINTS.md` — no other section, no heading, no link.
  Do not name `cmd/lyx/spawnobservability_test.go`, do not describe the guard's allowlist, and do not add a pointer to `manifest/designs/logger-coverage.md`; the file's own opening blurb declares it is not a test-coverage index, and commit `d66cefe5` stripped exactly those references out.
  After editing, re-read the section and confirm it names no test file.
- **Commit:** `docs(constraints): sharpen Live-Substrate Spawn Observability scope`

## Batch Tests

`verify: go test ./internal/lyxcwd/ -run TestEnforcement_MarkdownLinks` runs the Markdown Link Integrity guard (`internal/lyxcwd/docslink_test.go`), which is the only automated check either card can break: it walks every `.md` file under `manifest/` and `docs/` and fails on an inline link whose file part or `#anchor` does not resolve.
Card 1 creates a new file under `manifest/` and card 2 edits prose in a file the guard does not scan (`CONSTRAINTS.md` is at the repo root, outside the `manifest/`/`docs/` walk) but which other scanned files may link into by anchor — the guard covers that direction too, and the heading is deliberately left unchanged so no existing anchor breaks.

The batch has no other runnable surface: it adds no Go code, so no package test is affected.
The module-wide `verify:` in the overview (`go build ./... && GOOS=windows go build ./...`) still runs at the batch boundary and is expected to be a no-op here.

Neither card is a TDD candidate — both produce prose, and the guard is a link check, not a content check.
The content correctness of the audit tables is what plan review and code review evaluate; the mechanical check that the tables match the tree is batch 5's guard, which fails on exactly the sites the audit marks `add` until batch 3 lands.
