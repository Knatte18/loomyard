# Batch: reed-if-absent

```yaml
task: "Launch ly-supervise and orchestrator via lyx reed add"
batch: "reed-if-absent"
number: 1
cards: 5
verify: go test ./internal/reedengine/... ./internal/reedcli/... && go test -tags integration ./internal/reedcli/... && go vet -tags smoke ./internal/reedcli/
depends-on: []
```

## Batch Scope

This batch delivers the `--if-absent` flag on `lyx reed add` end to end: the engine-side decision table, the flag that reaches it, and the three test tiers that cover it.
It is one batch because the four branch rows, the flag that selects them, and the tests that pin them are one contract — splitting the classifier from the branch that performs it would leave a half-wired engine between two commits.
The external interface the other batches consume is the flag's spelling and its idempotence guarantee: batch 2 writes `lyx reed add --if-absent --cmd <claude> --name claude --focus` into the generated VS Code task, and batch 3 documents the convention.
Batch-local decision beyond the overview's shared set: the classifier returns an index into the caller's own `[]Strand` rather than a copy of the strand, so the relaunch branch can take a pointer into the live state slice the way `Resume` does.

## Cards

### Card 1: pure `--if-absent` classifier over the strand table

- **Context:**
  - `internal/reedengine/state.go`
  - `internal/reedengine/apply.go`
  - `internal/reedengine/lifecycle.go`
  - `internal/reedengine/render/types.go`
  - `internal/reedengine/lock_test.go`
- **Edits:**
  - `internal/reedengine/strand.go`
  - `internal/reedengine/strand_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Add to `internal/reedengine/strand.go` an unexported `ifAbsentDecision` type with exactly four constants — `ifAbsentAdd`, `ifAbsentNoOpAlive`, `ifAbsentRelaunch`, `ifAbsentNoOpHidden` — and a pure function `classifyIfAbsent(strands []Strand, name string, aliveIDs map[string]bool) (ifAbsentDecision, int)`.
  The second return value is the index into `strands` the decision names, and is `-1` for `ifAbsentAdd`.
  Build two sets internally, named so the rows cannot disagree: `matched` is every strand whose `Name` equals `name`, hidden ones included;
  `candidates` is `matched` minus every strand whose `Display.Anchor` equals `render.AnchorHidden`, in persisted (slice) order.
  The four rows are mutually exclusive and exhaustive: `matched` empty returns `ifAbsentAdd` with `-1`;
  `candidates` holding at least one alive strand returns `ifAbsentNoOpAlive` with the index of the **first alive** candidate in persisted order;
  `candidates` non-empty with none alive returns `ifAbsentRelaunch` with the index of the **first** candidate in persisted order;
  `matched` non-empty with `candidates` empty returns `ifAbsentNoOpHidden` with the index of the **first matched** strand in persisted order.
  A candidate is alive exactly when `s.PaneID != "" && aliveIDs[s.PaneID]` — both halves, copying `planResumeLaunches` in `internal/reedengine/lifecycle.go`, whose own comment records why the empty-`PaneID` half carries its weight after a server reboot clears every binding.
  Give the function a doc comment naming `aliveIDSet` in `internal/reedengine/apply.go` as the set its caller must build, and saying why `liveIDSet` is the wrong one (a dead-but-present pane would read as live, so the case most worth recovering would be the one `--if-absent` refused to fix).
  In `internal/reedengine/strand_test.go`, add a table-driven test per row plus the cases the discussion's Testing section enumerates for this function: a candidate with an empty `PaneID` classifies as not-alive and yields `ifAbsentRelaunch`;
  a candidate bound to a pane present in the pane list but absent from the alive set yields `ifAbsentRelaunch`;
  two candidates where the second is the alive one selects the second;
  two candidates where neither is alive selects the first;
  a hidden strand sharing a name with a not-alive visible one yields `ifAbsentRelaunch` against the visible one.
  Use the existing `newTestEngine` fixture pattern from `internal/reedengine/lock_test.go` only where an engine is genuinely needed — this function takes no receiver, so the table cases construct `[]Strand` values directly.
- **Commit:** `feat(reedengine): classify the four --if-absent branch rows`

### Card 2: `AddSpec.IfAbsent` and the branch in `AddStrand`

- **Context:**
  - `internal/reedengine/state.go`
  - `internal/reedengine/spawn.go`
  - `internal/reedengine/apply.go`
  - `internal/reedengine/lifecycle.go`
  - `internal/reedengine/render/types.go`
  - `internal/reedengine/template_posix.yaml`
  - `internal/reedengine/template_windows.yaml`
- **Edits:**
  - `internal/reedengine/strand.go`
  - `internal/reedengine/strand_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Add an `IfAbsent bool` field to `AddSpec` in `internal/reedengine/strand.go`, documented as opt-in: when false, `AddStrand` behaves byte-for-byte as it does today.
  Add a pure `validateIfAbsent(spec AddSpec) error` in the same file that returns an error naming the requirement — the message must state that `--if-absent` requires `--name` — when `spec.IfAbsent` is true and `spec.NameOverride` is empty, and `nil` otherwise.
  The reason belongs in its doc comment: `resolveStrandName` falls back to `guid[:8]` and the shipped templates in `internal/reedengine/template_posix.yaml` and `internal/reedengine/template_windows.yaml` carry `<SHORT_GUID>`, minted fresh per invocation, so a templated name can never match an existing strand and `--if-absent` would stack a duplicate on every reopen.
  Call `validateIfAbsent` as the first statement inside `AddStrand`'s `withOpLock` closure, ahead of `requireSessionLocked`, so the rejection precedes any state load.
  Then, still inside the closure and after `loadOrInitStateLocked`, branch when `spec.IfAbsent` is true: list the session's panes through the same `e.tmux.listPanes(e.SessionName())` call `Status` uses, build the alive set with `aliveIDSet`, and call `classifyIfAbsent(st.Strands, spec.NameOverride, aliveIDs)`.
  On `ifAbsentAdd`, fall through to today's `addStrandLocked` path unchanged.
  On `ifAbsentNoOpAlive` and on `ifAbsentNoOpHidden`, return the strand at the returned index as the result and mutate nothing: no `SaveState`, no `reconcileApplyPersistLocked`, no write to `Cmd`, `ResumeCmd`, `Parent` or `Display`.
  On `ifAbsentRelaunch`, take a pointer to `st.Strands[i]`, compute the launch command as the stored `ResumeCmd` falling back to the stored `Cmd` — the identical fallback `Resume` applies in `internal/reedengine/lifecycle.go` — and call `e.launchStrandLocked(st, &st.Strands[i], launchCmd)`, then `SaveState(e.stateDir(), st)` immediately after the launch succeeds and before `reconcileApplyPersistLocked`, matching the order `AddStrand` and `Resume` both already use so a pane whose binding is unpersisted is never reaped as untracked.
  The relaunch branch logs its spawn at `Info` via `logger.Info` before returning, carrying the socket, session, strand guid and strand name, per CONSTRAINTS.md's Live-Substrate Spawn Observability invariant;
  the two no-op branches log nothing at `Info`, since they start no process.
  This `Info` log is a deliberate, scoped exception rather than an emerging inconsistency, and the implementer records that in a comment beside the call: the three sibling paths through `launchStrandLocked` — ordinary `AddStrand`, `UpdateStrand`'s hidden-to-visible surface, and each per-strand replay inside `Resume` — carry no equivalent per-strand `Info` log today, relying on the generic tmux `Debug` trace instead.
  The discussion for this task settled the question for this branch alone;
  widening or narrowing the sibling paths' logging is separate work and stays out of this plan.
  Every branch returns a `Strand` carrying a non-empty `GUID` and `Name`, the hidden row included.
  In `internal/reedengine/strand_test.go`, add cases for `validateIfAbsent` (rejected with the requirement named when `NameOverride` is empty and `IfAbsent` is true;
  accepted otherwise, including the `IfAbsent` false case with no name), and drive both no-op branches through the engine helpers so the no-mutation guarantee is pinned at the engine-call level, one case per branch.
  For the alive-candidate no-op: `Display` is untouched even when the incoming spec carries `Focus: true`, and `Cmd`/`ResumeCmd`/`Parent` stay as persisted even when the spec supplies different ones.
  For the hidden-only no-op: the same four assertions hold, and additionally the strand count is unchanged — nothing was added — and the returned strand is the hidden one, carrying a non-empty `GUID` and `Name`.
  Keep the tmux-facing relaunch out of this untagged file — card 5 covers it.
- **Commit:** `feat(reedengine): branch AddStrand on the --if-absent decision`

### Card 3: the `--if-absent` CLI flag and its sandbox scenario

- **Context:**
  - `internal/reedengine/strand.go`
  - `internal/reedcli/cli.go`
  - `internal/reedcli/status.go`
- **Edits:**
  - `internal/reedcli/add.go`
  - `tools/sandbox/SANDBOX-REED-SUITE.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `internal/reedcli/add.go`, register a `--if-absent` bool flag defaulting to `false` with a one-line usage string saying it makes a repeated `add` idempotent by matching `--name` against this worktree's strands, and map it onto the new `AddSpec.IfAbsent` field.
  The flag adds no validation of its own in this file: the `--name` requirement is the engine's, surfaced through the existing `output.Err(out, err.Error())` path that already handles every `AddStrand` error.
  Keep `MarkFlagRequired("cmd")` exactly as it is — `--cmd` stays required under `--if-absent` too, because it is what the absent-name branch launches, and relaxing it would make the common first-open case fail.
  Extend the command's `Long` text with a short paragraph naming the flag, the fact that it requires `--name`, and that a matched strand is left as persisted rather than rewritten from this invocation's flags.
  Leave the success envelope shape untouched: every branch still prints `guid` and `name` through `output.Ok`.
  In `tools/sandbox/SANDBOX-REED-SUITE.md`, add scenario `M27 -- Repeated add --if-absent is idempotent` in the file's existing **Goal:** / **Watch:** / **Verdict:** shape, placed after `M26` and before the report-template section, and add its `M27: <OK|WARN|FAIL> -- <one-line note if not OK>` row to the report template block at the end of the file.
  The **Watch:** text must name the three reopen shapes the flag covers — a second `add --if-absent` while the strand is alive adds nothing and reports the same guid;
  a third after the strand's pane has died relaunches that same guid rather than adding a second strand;
  and an `add --if-absent` naming a hidden strand adds nothing and reports the hidden strand's own guid.
- **Commit:** `feat(reedcli): add --if-absent to lyx reed add`

### Card 4: integration coverage for the CLI envelope

- **Context:**
  - `internal/reedcli/add.go`
  - `internal/reedcli/cli.go`
  - `internal/reedengine/strand.go`
- **Edits:**
  - `internal/reedcli/cli_integration_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Add two tests to `internal/reedcli/cli_integration_test.go`, following the file's existing `hubforge.NewHub` plus `RunCLIIn` shape and its `TestRunCLI_AddNotUp_FriendlyError` case in particular.
  The first asserts that `add --if-absent --cmd <anything>` with no `--name`, run against a fixture hub with no session up, exits non-zero and emits a JSON error envelope whose message names the `--name` requirement — specifically **not** the `no reed session; run "lyx reed up"` message, which is what proves the rejection precedes the state load.
  The second asserts that `add --if-absent --name claude` with no `--cmd` still fails on the missing required `--cmd`, so the flag relaxes nothing.
  Both stay in this integration-tagged file rather than the untagged `internal/reedcli/cli_test.go`, because `hubforge.NewHub` is barred from untagged files by CONSTRAINTS.md's Test Tier Purity Invariant.
- **Commit:** `test(reedcli): cover the --if-absent rejection envelopes`

### Card 5: live-server smoke coverage for the reopen scenario

- **Context:**
  - `internal/reedcli/smoke_resume_test.go`
  - `internal/reedcli/smoke_lifecycle_test.go`
  - `internal/reedcli/smoke_test.go`
  - `internal/reedcli/testmain_test.go`
  - `internal/reedcli/add.go`
  - `internal/reedengine/strand.go`
- **Edits:** none
- **Creates:**
  - `internal/reedcli/smoke_ifabsent_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Create `internal/reedcli/smoke_ifabsent_test.go` carrying the `//go:build smoke` constraint and following the conventions of `internal/reedcli/smoke_resume_test.go`: `tmuxBinaryPath(t)`, `hubforge.NewHub(t, ".")`, `deferHubRelease`, `t.Chdir(h.PrimeWorktree())`, and a `t.Cleanup` that runs `down`.
  A new file rather than an addition to an existing one, matching the package's one-smoke-file-per-concern convention.
  The test walks the real reopen sequence: `up`;
  `add --if-absent --name claude --cmd <long-running command>`;
  a second identical `add --if-absent`, after which `status` must report exactly one strand carrying the same guid the first add returned;
  then kill that strand's pane and issue a third `add --if-absent`, after which `status` must report that same guid with `live: true` and still exactly one strand.
  Assert the third call against liveness, never against a pane merely existing: tmux keeps a session's sole pane on screen after its process dies, so a check for pane presence alone would pass without the relaunch ever happening.
  Read liveness from the `live` field of the `status` envelope, which is already built from the alive-not-merely-present set.
- **Commit:** `test(reedcli): smoke the repeated --if-absent reopen`

## Batch Tests

`verify:` runs three gates, each scoped to what this batch touches.
`go test ./internal/reedengine/... ./internal/reedcli/...` runs the untagged tier — the classifier table and `validateIfAbsent` cases added to `internal/reedengine/strand_test.go` by cards 1 and 2, alongside the existing tests in both packages that must keep passing.
`go test -tags integration ./internal/reedcli/...` runs card 4's two envelope assertions in `internal/reedcli/cli_integration_test.go`, which need a fixture hub and therefore a tag.
`go vet -tags smoke ./internal/reedcli/` compiles card 5's `internal/reedcli/smoke_ifabsent_test.go` without needing a live tmux server in CI;
the smoke tier itself is operator-run, as every other `smoke_*_test.go` in that package already is.
Package scoping rather than a repo-wide `go test ./...` is deliberate: only these two packages change here, and the repo-wide sweep is already the configured done gate, which runs once at task end.
