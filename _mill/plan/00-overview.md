# Plan: reed: AddStrand and attach self-heal a cold worktree

```yaml
task: 'reed: AddStrand and attach self-heal a cold worktree'
slug: 'reed-cold-worktree-selfheal'
approved: true
started: '20260919-050951'
parent: 'main'
root: ""
verify: null
discussion_sha: b8bbaa85a790e4055f1193a11329bb24a9984a0e
```

## Prior failure

- Holistic review round 1 fix: `go test -tags smoke ./internal/reedcli/` (batch 5, tagged-tests) fails deterministically on `TestSmokeClaudeResumeRecallsCodeword` when run from inside a nested Claude Code session — the test's own doc comment names this exact failure mode (no new claude transcript persisted+stabilized because a nested `claude` invocation stops writing transcripts). Reproduced twice by the fixer session, unaffected by either finding fixed that round.
- Resolution: a third reproduction from the top-level orchestrator session (not a subagent) failed identically, and a code-diff audit confirmed the env-hygiene mechanism (`reedengine.CleanClaudeEnv`) and every helper the test relies on are byte-identical to `main` — this task's diff never touches that path. The test is a manual/human-operated real-subscription check (per its own doc comment), not CI-shaped, and cannot run inside any Claude-Code-ancestored process regardless of code correctness. Batch 5's `verify:` (both here and in `05-tagged-tests.md`) now `-skip`s only that one test by name; every other smoke/integration assertion this batch and the plan's regression-signal intent depend on still runs.
- Done-gate failure: `go test -tags integration ./...` failed deterministically on `internal/reedcli/cli_integration_test.go`'s pre-existing `TestRunCLI_AddNotUp_FriendlyError`, which still pinned the refused-when-cold `add` behaviour this task deliberately removes (exit 1 plus the "no reed session" error).
  No batch touched that file, and none of the five batches' `verify:` commands ran `-tags integration` against `internal/reedcli` (batch 5 ran it against `internal/reedengine` only), so the stale assertion surfaced only at the repo-wide done gate.
- Resolution: the test is rewritten as `TestRunCLI_AddNotUp_SelfHealsAndSucceeds`, the integration-tier twin of the smoke-tier headline scenario — exit 0, a guid-and-name envelope, and a following `status` succeeding against the booted session with the strand live — with a `down` cleanup so the booted session never outlives the test;
  `TestRunCLI_AddIfAbsentNoName_RejectsBeforeSessionCheck`'s comment no longer cites the retired scenario and the test now also asserts that the rejection boots nothing.
  The `remove` and `status` refusal tests in the same file are unchanged, since only `add` and `attach` self-heal.

## Batch Index

_The fenced yaml block below is the authoritative DAG mill-go reads to schedule batches.
Every batch lives at `NN-<batch-slug>.md` in this directory and is mirrored as one entry here._

```yaml
batches:
  - number: 1
    name: engine-seam
    file: 01-engine-seam.md
    depends-on: []
    verify: go test ./internal/reedengine/
  - number: 2
    name: attach-preflight
    file: 02-attach-preflight.md
    depends-on: [1]
    verify: go test ./internal/reedcli/ ./internal/reedengine/
  - number: 3
    name: comment-sweep
    file: 03-comment-sweep.md
    depends-on: [1, 2]
    verify: go test ./internal/reedengine/ ./internal/burlercli/ ./internal/webstercli/ ./internal/vscode/ ./cmd/lyx/
  - number: 4
    name: docs-and-suites
    file: 04-docs-and-suites.md
    depends-on: [1, 2]
    verify: go test ./internal/lyxcwd/ ./cmd/lyx/
  - number: 5
    name: tagged-tests
    file: 05-tagged-tests.md
    depends-on: [1, 2]
    verify: go test -tags smoke ./internal/reedcli/ -skip '^TestSmokeClaudeResumeRecallsCodeword$' && go test -tags integration ./internal/reedengine/
```

## Shared Decisions

### Decision: the seam is `ensureSessionLocked`, never `Up()`

- **Decision:** the self-heal entry point is a new narrow helper pair — unexported `ensureSessionLocked() (bool, error)` and its exported `withOpLock` wrapper `EnsureSession() (bool, error)` — which probes session liveness first and returns `(false, nil)` immediately when the session is already usable.
  Only when it is not does it delegate to `upLocked()`.
  Routing a warm call through `Up()`/`upLocked()` is banned.
- **Rationale:** `upLocked`'s tail reaches `planReconcile`, which adds every live non-exempt pane to `untrackedPanesToKill` whenever the header is alive, so a warm `attach` through it would destroy a pane an operator hand-split on a verb that is read-only today.
  Separately, `ensureServerAndSessionLocked` runs its whole pre-tmux config-validation block *before* its already-up early return, so a `mouse:` typo would refuse `attach` against a perfectly healthy live session.
  Probing liveness first removes both: when there is nothing to boot there is nothing to validate, nothing to reconcile, and nothing to write.
- **Applies to:** all batches

### Decision: the warm path changes only by adding probe round trips

- **Decision:** a `reed add` or `reed attach` against a live session holding at least one pane performs no reconcile, no layout apply, no `SaveState`, and no config validation it did not already perform, and keeps every refusal it performs today.
  `add` gains one round trip (`list-panes`), `attach` gains two (`has-session`, `list-panes`).
- **Rationale:** this is a hard boundary on the task, not a nice-to-have.
  It is what makes the change strictly additive rather than a silent rewrite of two verbs' failure modes.
- **Applies to:** all batches

### Decision: one shared liveness predicate, never two copies

- **Decision:** the "session up **and** holding at least one pane" question is answered by exactly one new unexported helper, `sessionSubstrateLocked() (up bool, usable bool, err error)`.
  `ensureServerAndSessionLocked` and `ensureSessionLocked` both read it;
  neither restates the `len(live) > 0` condition itself.
- **Rationale:** a session that exists but holds zero panes is broken substrate that can never host a strand, which is why `ensureServerAndSessionLocked` kills the husk and re-boots.
  An early return on bare `has-session` would skip that repair and leave `add` failing forever.
  Two copies of the condition can silently diverge, and the husk bug is exactly what a divergence would reintroduce.
- **Applies to:** engine-seam

### Decision: `booted` means a session was actually created

- **Decision:** `booted` is never "the cold branch was taken".
  `upLocked` returns `(UpResult, booted bool, error)`, passing `ensureServerAndSessionLocked`'s own flag straight out;
  `Up()` discards it;
  `ensureSessionLocked` returns it verbatim rather than hardcoding `true` on the delegate path.
- **Rationale:** `ensureSessionLocked` can probe, find no live session, delegate — and `ensureServerAndSessionLocked` probes again and finds the session up by then, returning `booted == false`.
  A sibling `lyx reed up` in another terminal is the realistic trigger; the op lock serialises only reed's own callers.
  A hardcoded `true` would make the attribution log claim a spawn that never happened, which is the one thing that log exists to report.
- **Applies to:** engine-seam, attach-preflight, tagged-tests

### Decision: exactly one new exported engine method

- **Decision:** `EnsureSession()` is the only exported-surface addition.
  `UpResult`, `Up()`, `Status()`, `AttachArgv()` and `AddStrand()`'s signatures are all unchanged, and `reedcli`'s `up` verb keeps its current envelope keys.
  No `UpResult.Booted` field.
- **Rationale:** `manifest/designs/reed-fabric-standalone-api.md` freezes `*Engine`'s exported surface by count and by name, so every addition has a documentation cost.
  The helper the CLI already has to call is the natural carrier of the boot signal, so no extra surface is needed.
- **Applies to:** engine-seam, docs-and-suites

### Decision: attach keeps its `Status()` call

- **Decision:** `internal/reedcli/attach.go`'s pre-flight becomes `EnsureSession()` **then** `Status()`.
  The existing `Status()` call is kept, not replaced, and the ordering is load-bearing.
- **Rationale:** `Status()` reaches `loadOrInitStateLocked`, so today's warm attach refuses when `reed.json` is corrupt and when its persisted bindings were minted against a still-live session under another name.
  `EnsureSession`'s early return reads no state at all, and `AttachArgv` degrades to the bare argv by contract rather than erroring, so dropping `Status()` would silently delete two refusals.
  `Status()` first would refuse before anything booted, reinstating the exact bug this task removes.
- **Applies to:** attach-preflight, tagged-tests

### Decision: corrupt `reed.json` boots then fails, and that residue is pinned

- **Decision:** on a cold worktree whose `reed.json` is unreadable, the self-healing `add`/`attach` boot the session first and then fail with `LoadState`'s corrupt-file diagnosis, leaving a bare session behind.
  No pre-boot readability refusal is added anywhere.
  A smoke test asserts both halves — the failure text and the residue.
- **Rationale:** this matches `lyx reed up`'s behaviour today exactly, and the premise of the whole task is that the self-healing verbs do what `up` does.
  Fixing it for `add`/`attach` alone would diverge the very paths this task is unifying.
  Pinning it records the accepted posture so a later change to it fails loudly instead of passing silently.
- **Applies to:** engine-seam, tagged-tests

### Decision: the zero-pane-husk smoke test is dropped, not weakened

- **Decision:** no smoke test is written for the zero-pane husk repair.
  The predicate's correctness is argued from `ensureServerAndSessionLocked`'s own documented reasoning and the shared-helper requirement above, not from a test.
- **Rationale:** no existing fixture in the repo builds a zero-pane session, and `listPanes` returns an error rather than an empty slice when tmux cannot list at all.
  The discussion sanctions dropping the test rather than weakening the predicate to fit, and the repair path itself is pre-existing and unchanged by this task.
  Leaving the predicate untested is acceptable; changing it to something testable is not.
- **Applies to:** tagged-tests

### Decision: `done_gate` is left as configured, and lint stays out of it

- **Decision:** `pipeline.done_gate` keeps its current value, `go test ./... && go test -tags integration ./...`.
  `golangci-lint run` is deliberately NOT added.
- **Rationale:** the candidate lint command was run against this worktree's tip before planning and exits non-zero on pre-existing debt unrelated to this task (`cmd/lyx/drift_test.go:36`'s `S1025`, among others).
  Adding it would make every future task in this hub depend on that debt being cleared first.
- **Applies to:** all batches

### Decision: an untagged test that relied on `AddStrand` failing fast is reworked by intent

- **Decision:** the repo-wide grep for `AddStrand` in `_test.go` files was run at planning time and yielded exactly one affected hit: `internal/burlercli/wiring_test.go`'s `TestWireStandalone_RunnerReachesPublicEntryPointWithoutToldPathError`.
  Every other hit is either build-tagged (`smoke`/`integration`) or drives a fake implementing reed's interface rather than a real engine.
  The affected test is reworked to reach its told-path assertion without spawning, never retagged to the `smoke` tier.
- **Rationale:** that test's intent is the told-path/wiring assertion; reed's refusal is only the cheap way it stopped early.
  Retagging would trade a Test Tier Purity breach for a slower tier-1 suite and hide the intent change.
- **Applies to:** comment-sweep

### Decision: comments a change falsifies are rewritten in the same commit

- **Decision:** every comment that states the invalidated "requires a live session" / "fails fast without spawning" / "the only pre-flight that can abort" premise is rewritten, and the calls those comments sit on are kept.
  This covers `internal/vscode/config.go`, both `internal/webstercli` sites, `internal/reedengine/spawn.go`'s caller-route enumeration, `internal/reedengine/doc.go`, and `internal/reedengine/strand.go`'s three strand-op doc comments.
- **Rationale:** all three redundant explicit boots become redundant, none becomes wrong, and each is more legible as an explicit early envelope-reportable boot than as one buried inside a later `AddStrand`.
  A comment that misstates why its code exists is worse than a redundant call, and it is what the next reader will trust.
- **Applies to:** comment-sweep, docs-and-suites

## All Files Touched

- `docs/overview.md`
- `internal/burlercli/wiring_test.go`
- `internal/reedcli/attach.go`
- `internal/reedcli/cli_integration_test.go`
- `internal/reedcli/smoke_coldstart_test.go`
- `internal/reedcli/smoke_staterecovery_test.go`
- `internal/reedcli/smoke_warmpath_test.go`
- `internal/reedengine/attach.go`
- `internal/reedengine/doc.go`
- `internal/reedengine/ensuresession_integration_test.go`
- `internal/reedengine/lifecycle.go`
- `internal/reedengine/spawn.go`
- `internal/reedengine/strand.go`
- `internal/reedengine/strand_test.go`
- `internal/vscode/config.go`
- `internal/webstercli/recoverbatch.go`
- `internal/webstercli/run.go`
- `manifest/designs/reed-fabric-standalone-api.md`
- `manifest/designs/worktree-lifecycle-shed-producers.md`
- `manifest/roadmap.md`
- `tools/sandbox/SANDBOX-REED-SUITE.md`
- `tools/sandbox/SANDBOX-SHUTTLE-SUITE.md`
- `tools/sandbox/SANDBOX-WEBSTER-SUITE.md`
