# Batch: tagged-tests

```yaml
task: 'reed: AddStrand and attach self-heal a cold worktree'
batch: 'tagged-tests'
number: 5
cards: 4
verify: go test -tags smoke ./internal/reedcli/ -skip '^TestSmokeClaudeResumeRecallsCodeword$' && go test -tags integration ./internal/reedengine/
depends-on: [1, 2]
```

## Batch Scope

This batch is the whole real-tmux tier for the change: the cold-path smoke suite that proves the headline scenario, the warm-path smoke suite that proves the seam did not widen, the extension of the existing foreign-session recovery test, and the integration-tagged assertion on the boot flag itself.
It is one batch because every card drives a real multiplexer against a forged hub and shares the same fixture vocabulary, and because the cold and warm halves are only meaningful read against each other.

Batch-local decision: the warm-path cards are the sharpest tests in the suite and are not optional.
Each one fails loudly if the pre-flight is ever routed back through the boot path — the untracked-pane test because the boot path's reconcile would kill that pane, the config-error test because the boot path validates before its already-up early return, and the state-write test because the boot path persists.
They are the executable form of the task's hard boundary that a warm call changes only by adding probe round trips.

Per the overview's decision on the zero-pane husk, no test is written for that repair path: no fixture in the repo reliably constructs a session holding no panes, and weakening the predicate to make one testable is explicitly out of bounds.

## Cards

### Card 11: cold-path smoke suite

- **Context:**
  - `internal/reedcli/smoke_test.go`
  - `internal/reedcli/smoke_staterecovery_test.go`
  - `internal/reedcli/smoke_ifabsent_test.go`
  - `internal/reedcli/attach.go`
  - `internal/reedengine/strand.go`
  - `internal/reedengine/lifecycle.go`
  - `internal/reedengine/state.go`
- **Edits:** none
- **Creates:**
  - `internal/reedcli/smoke_coldstart_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Create a `smoke`-tagged file following the fixture pattern the package's existing smoke tests use — a `hubforge`-built hub, the `deferHubRelease` cleanup, and the `RunCLIIn` seam driven with an explicit cwd rather than a process-wide directory change.
  Reuse the package's existing helpers rather than writing new fixture machinery: `tmuxBinaryPath`, `materializeSibling`, `socketAndSessionIn`, `addStrandIn`, `listPaneLines`, `sessionAlive`, `paneIDForStrandIn`, and `smokeReapLaunchCmd`.
  On a freshly forged worktree the status verb still refuses, so a cold test that needs this worktree's socket and session name before any boot must derive them from `reedengine`'s own exported `ServerName` and `SessionName` free functions rather than from `socketAndSessionIn`.

  Write five tests.

  Cold add is the headline scenario: on a worktree never brought up, with no persisted state file, the add verb exits zero, its envelope carries a guid, and the multiplexer afterwards lists this worktree's session on the hub socket.

  Cold add builds the same substrate an explicit boot plus add would: after the cold add, the session holds the header pane as well as the strand's own pane, proving the delegate path ran the whole boot body rather than only spawning a server.

  Cold attach: on a worktree never brought up, the attach verb run without a controlling terminal must fail with the multiplexer's own terminal error rather than with a no-session JSON envelope, and the session must exist on the socket afterwards.
  The distinction between those two failure modes is the whole assertion — assert on both the absence of the no-session text and the presence of the session.

  Cold add with a persisted strand table: seed a state file naming two strands, leave the server dead, and run the add verb with the `--if-absent` flag for one of those names.
  It must boot and relaunch exactly that strand — one pane for it, not two entries under one name — and must not relaunch the other.
  This is the observable consequence of up semantics rather than resume semantics.

  Cold add on an unreadable state file: on a worktree never brought up whose `.lyx/reed.json` holds truncated bytes, the add verb exits non-zero with the state loader's corrupt-file diagnosis, naming the file and the teardown verb, **and** a session now exists on the socket.
  Both halves are the assertion.
  Comment the second half as the deliberately accepted residue of matching the boot verb's own behaviour, so a later change to that posture fails this test loudly instead of passing silently.
- **Commit:** `test(reedcli): pin the cold-worktree self-heal scenarios end to end`

### Card 12: warm-path smoke suite

- **Context:**
  - `internal/reedcli/smoke_test.go`
  - `internal/reedcli/smoke_lifecycle_test.go`
  - `internal/reedcli/smoke_staterecovery_test.go`
  - `internal/reedcli/attach.go`
  - `internal/reedengine/lifecycle.go`
  - `internal/reedengine/reconcile.go`
  - `internal/reedengine/config.go`
  - `internal/configengine/config.go`
- **Edits:** none
- **Creates:**
  - `internal/reedcli/smoke_warmpath_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Create a second `smoke`-tagged file using the same fixture vocabulary card 11 uses, holding the five warm-path assertions.
  Every test here boots the session explicitly first, so the pre-flight under test takes its early return.

  Warm attach does not reap an operator's pane: bring the session up, add a strand, then create an untracked pane directly through the multiplexer by splitting the session outside reed's bookkeeping, then run the attach verb.
  The untracked pane must still be alive afterwards.
  Comment this test as the one that fails loudly if the pre-flight is ever routed back through the boot path, whose reconcile would add that pane to its kill list.

  Warm attach survives a config error: bring the session up, then write an invalid value for the mouse or watchdog key into this worktree's reed config, then run the attach verb against the still-healthy live session.
  It must not refuse with that config error.
  Assert the cold direction too — same bad config, no session — where the refusal must still fire.
  Both directions together are what pin that the liveness probe precedes the boot path's validation block.

  Warm attach writes no state: capture the modification time and bytes of `.lyx/reed.json` before a warm attach and assert both are unchanged afterwards.

  Warm attach still refuses on an unreadable state file: with a live session, make `.lyx/reed.json` unparseable underneath it, then run the attach verb.
  It must abort on the JSON envelope with the state loader's diagnosis, exactly as it does today.
  Comment this test as the one that fails if the status call is ever dropped from the pre-flight.

  Warm add is unchanged: bring the session up then add a strand, and assert one session, one header pane, one strand.

  Where a test needs to rewrite this worktree's reed config, seed it the way the engine's own integration fixture does — write the full config template with the one key's value replaced, into the config path the config engine resolves for the reed module under this worktree's anchor, rather than a single-key fragment.
- **Commit:** `test(reedcli): pin that the warm add and attach paths gain only probe round trips`

### Card 13: extend the existing foreign-session recovery test

- **Context:**
  - `internal/reedcli/smoke_test.go`
  - `internal/reedengine/generation.go`
  - `internal/reedengine/lifecycle.go`
  - `internal/reedengine/strand.go`
- **Edits:**
  - `internal/reedcli/smoke_staterecovery_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  `TestSmokeDiagnosticVerbsNameTheOrphanSessionRatherThanPointingAtResume` already drives the renamed-worktree fixture and loops the status, attach and add verbs against it, asserting each names the orphan session.
  Extend it rather than writing a parallel test that duplicates the fixture.

  Keep all three verbs and all three existing assertions: the refusal must survive for every one of them.
  Add one new assertion inside the same loop — after each verb refuses, no session for the renamed worktree may exist on the hub socket.
  This is the property that matters most now: the two self-healing verbs reach a boot path and must still refuse *before* it deposits any substrate.
  Derive the renamed worktree's own session name from `reedengine`'s exported `SessionName` free function, since the status verb cannot report it once the worktree is renamed.

  Add the copied-state fixture as a second case: copy the original worktree's state file into a sibling worktree of the same hub while the original session is still live, then run the same three verbs there with the same two assertions.
  Either fold it into this test as a sub-case or add it as an adjacent test in the same file reusing the same helpers.
  Do not build a third fixture shape.

  Rewrite the test's framing comment.
  It currently says every verb that goes through `requireSessionLocked` rather than through a boot must report the same diagnosis, which is false for two of the three verbs now.
  The replacement must state the stronger property the test actually pins: attach and add now reach a boot path, still refuse, and still deposit nothing.
- **Commit:** `test(reedcli): pin that the foreign-session refusal survives the self-heal and leaves no residue`

### Card 14: boot-attribution integration test

- **Context:**
  - `internal/reedengine/contract_integration_test.go`
  - `internal/reedengine/logcapture_test.go`
  - `internal/reedengine/lifecycle.go`
  - `internal/reedengine/strand.go`
  - `internal/reedengine/lock_test.go`
- **Edits:** none
- **Creates:**
  - `internal/reedengine/ensuresession_integration_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Create an `integration`-tagged file in `internal/reedengine` following `contract_integration_test.go`'s pattern: its own scratch socket so it can never collide with a real hub server, the `seedReedConfig` recipe for the on-disk config, and the same self-skip when the configured multiplexer binary is absent.
  Both halves of this assertion need a real server, so there is no hermetic half to split off and none of it belongs in the untagged tier.

  Assert `EnsureSession`'s booted return directly: true against a worktree whose session has never been created, and false when called again against the live session it just created.
  That return value is the primitive both attribution log lines derive from, so pinning it covers both call sites at once.

  Add one assertion on `AddStrand`'s own log line, using the package's existing `captureLogOutput` helper, which sets both the logger output and its verbosity because neither alone captures at `Info`.
  A cold `AddStrand` must emit the attribution line; a second `AddStrand` against the now-live session must not.
  Do not replicate a logger-capture helper into `internal/reedcli` for the attach line: the booted assertion above already pins the condition that line is guarded on, and a second copy of that helper costs more than the assertion is worth.

  Tear down every session and server this file creates, including on the failure paths, so a failed run leaves nothing on the scratch socket.
- **Commit:** `test(reedengine): pin EnsureSession's booted return and the add self-heal log line`

## Batch Tests

`verify: go test -tags smoke ./internal/reedcli/ -skip '^TestSmokeClaudeResumeRecallsCodeword$' && go test -tags integration ./internal/reedengine/` runs both tagged tiers this batch writes into, and both are required: cards 11–13 are `smoke`-tagged and live in `internal/reedcli`, card 14 is `integration`-tagged and lives in `internal/reedengine`.
A single-tag command would silently skip whichever half it did not name, since a build tag excludes the file from compilation entirely rather than failing.
Each half is scoped to the one package that tier's new files land in.

Running the whole `smoke` tier of `internal/reedcli` rather than only the new files is deliberate and is the regression signal the discussion asks for: any existing test in that package asserting the no-session refusal from the add verb or the attach verb specifically is the behaviour being removed and must surface here, while any asserting the same refusal from status, remove, reapply or the send/capture ops must keep passing untouched.
A failure in that second group means the change leaked past its scope.
Both halves need a real multiplexer on the machine; the untagged tiers batches 1 through 4 run remain the offline gate.

`-skip '^TestSmokeClaudeResumeRecallsCodeword$'` excludes one pre-existing, unmodified test that this batch neither touches nor regresses: it launches a real `claude` subprocess and asserts transcript persistence across a crash+resume, which needs a logged-in `claude` CLI and a real subscription session, and is structurally unrunnable from *any* execution context that itself already sits inside a Claude Code session (an ancestor `claude` process), automated or interactive — confirmed by three independent reproductions during this task's own mill-go run (two nested implementer/fixer subagents and the top-level orchestrator itself, all three deterministic, zero relation to this batch's diff). It is a manual/human-operated verification, not a CI-shaped one; run it by hand from a plain (non-Claude-Code) terminal when validating a reed release.
