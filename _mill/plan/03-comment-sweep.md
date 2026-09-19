# Batch: comment-sweep

```yaml
task: 'reed: AddStrand and attach self-heal a cold worktree'
batch: 'comment-sweep'
number: 3
cards: 3
verify: go test ./internal/reedengine/ ./internal/burlercli/ ./internal/webstercli/ ./internal/vscode/ ./cmd/lyx/
depends-on: [1, 2]
```

## Batch Scope

This batch reconciles every Go comment the seam change falsifies, and reworks the one untagged test that relied on `AddStrand` failing fast without spawning.
It is one batch because all three cards share a single premise — the invalidated "requires a live session" claim — and no card changes behaviour.
The three redundant explicit boots the task deliberately keeps are kept here; only their stated justifications move.

Batch-local decision: the comment sweep is grep-driven, not file-list-driven.
Each card names the hits confirmed at planning time and requires the implementer to re-run the grep against the tree it actually sees, because the set can drift.

## Cards

### Card 6: reconcile `internal/reedengine`'s own production comments

- **Context:**
  - `internal/reedengine/lifecycle.go`
  - `internal/reedengine/lock.go`
  - `internal/reedengine/generation.go`
  - `internal/reedengine/reconcile.go`
- **Edits:**
  - `internal/reedengine/doc.go`
  - `internal/reedengine/spawn.go`
  - `internal/reedengine/strand.go`
  - `internal/reedengine/attach.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Grep all of `internal/reedengine`'s production files — not just `internal/reedengine/doc.go` — for `requireSessionLocked`, `AddStrand` and `AttachArgv`, and reconcile every comment hit whose claim the seam change falsifies.
  The four confirmed hits below are the known set, not the whole set; treat a hit the grep surfaces that is not listed here as in scope for this card.

  In `internal/reedengine/spawn.go`, `loadOrInitStateLocked`'s doc comment enumerates its callers by route, claiming every op other than the two booting verbs arrives via `requireSessionLocked`.
  `AddStrand` now arrives via `ensureSessionLocked`, so the enumeration is wrong.
  Rewrite it to name all three routes, keeping the sentence's purpose — that the generation probe always has a session to ask about — intact.

  In `internal/reedengine/doc.go`, the last-pane-fate bullet describes a subsequent add as one that calls `requireSessionLocked` and never re-boots, which is the inference that makes its psmux observation load-bearing.
  Rewrite it so the observation survives: on psmux the session survived the last-pane kill, which is why the later add found a live session rather than booting one.
  Revisit the surrounding lifecycle grammar in the same pass and correct any sentence that now describes add or attach as non-booting verbs.

  In `internal/reedengine/strand.go`, `UpdateStrand`'s and `RemoveStrand`'s doc comments both describe their pre-flight by cross-reference to `AddStrand`.
  That cross-reference is now false for both.
  Re-point each at `Status` instead, and state explicitly that these two verbs keep `requireSessionLocked` and do not self-heal.
  `AddStrand`'s own doc comment was already rewritten in batch 1 and must not be rewritten again here.

  In `internal/reedengine/attach.go`, `AttachArgv`'s code is unchanged, but its file header and doc comments describe a world in which the builder's caller has not booted anything.
  Add a sentence acknowledging that `internal/reedcli`'s attach pre-flight now boots the session before this builder runs, while keeping both of the builder's own documented contracts stated as they are: it returns no error by contract, and it is read-only with respect to `.lyx/reed.json`.
  Do not add a boot to this builder.
- **Commit:** `docs(reedengine): reconcile comments the AddStrand self-heal falsifies`

### Card 7: reconcile the downstream callers' comments

- **Context:**
  - `internal/reedengine/lifecycle.go`
  - `internal/reedengine/strand.go`
- **Edits:**
  - `internal/vscode/config.go`
  - `internal/webstercli/run.go`
  - `internal/webstercli/recoverbatch.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  In `internal/vscode/config.go`, the generated task chain's ordering rationale argues its safety on the premise that `AddStrand` pre-flights `requireSessionLocked` and attach pre-flights `Status`, so a failed `reed up` row ends with no strand, no pane, and no bare agent.
  That premise is now false: the failed row is followed by an add row that will attempt its own boot and fail the same way.
  Rewrite the comment so it states the net operator outcome that still holds — still no strand — on the new mechanism rather than the old one.
  The generated task chain itself keeps its `reed up` row: removing it would leave every existing worktree on the old chain while newly-generated ones differ, so the codebase would carry both shapes for no gain.

  In `internal/webstercli/run.go`, the standalone boot's comment asserts that pre-fix the spawn died on the no-session error with an impossible recourse.
  In `internal/webstercli/recoverbatch.go`, the equivalent comment asserts that recover-batch spawns a cold recovery strand through `AddStrand`, which requires a live session.
  Both premises are falsified verbatim.
  Rewrite both to say the call is now a deliberate early, explicit boot chosen for envelope-reportable failure and placement control, not the only thing standing between a standalone spawn and a dead end.

  Keep both boot calls and both call sites' positions.
  The one in `internal/webstercli/recoverbatch.go` is deliberately placed after its bad-batch, unparseable-plan and absent-run refusals so a rejected call boots no substrate — an ordering property the self-heal does not provide, since the boot now happens wherever `AddStrand` is called.
  Preserve the paragraph making that argument and keep it as the stated reason the call stays.
- **Commit:** `docs(vscode,webstercli): re-ground the redundant reed boots on their real reasons`

### Card 8: rework the untagged wiring test that relied on a fast refusal

- **Context:**
  - `internal/burlercli/wiring.go`
  - `internal/burlercli/cli.go`
  - `internal/cliwire/standalone.go`
  - `internal/configengine/config.go`
  - `internal/reedengine/config.go`
  - `internal/reedengine/lifecycle.go`
  - `internal/reedengine/strand.go`
  - `internal/reedengine/contract_integration_test.go`
  - `internal/shuttleengine/run.go`
- **Edits:**
  - `internal/burlercli/wiring_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  `TestWireStandalone_RunnerReachesPublicEntryPointWithoutToldPathError` drives a real engine through the burler engine's run entry point and relies on reed refusing before anything spawns.
  After batch 1 that call boots a real tmux server from an untagged test.
  Rework it so it reaches the same assertion without spawning, and do not move it to the `smoke` tier.

  The test's intent is unchanged and must stay: `wireStandalone` builds its runner via `NewDetachedRunner` rather than `NewRunner`, so the runner's held told-path verdict is nil and the public entry point reaches reed rather than returning a containment refusal.
  Keep the existing two assertions — the returned error is non-nil, and it names neither constructor.

  Make the boot impossible by pointing reed's configured multiplexer binary at a path that does not exist, so `sessionSubstrateLocked`'s own session probe fails at process lookup and nothing is ever spawned.
  Reach that by calling the CLI's wire method twice: the first call resolves the standalone state directory onto the receiver and spawns nothing, since `wireStandalone` assigns its boot closure without executing it.
  Between the two calls, seed a reed config under that state directory, writing `ConfigTemplate`'s bytes to the path `ConfigFile` returns for the reed module with the multiplexer key's value replaced by a path under the test's own temp directory that is never created.
  Create the config directory with a recursive, already-exists-tolerant call rather than the single-level `os.Mkdir` that `seedReedConfig` uses in `internal/reedengine/contract_integration_test.go`: that helper assumes a bare temp directory, whereas the first wire call has already created this state directory's `_lyx` tree — `ResolveStandalone` seeds the standalone stencils and specs directories beneath it, and `internal/stencilstore/`'s reconcile creates their parents on the way — so transplanting the single-level form verbatim fails on an existing directory before the config is ever written.
  Write the whole template rather than a single-key fragment, since the degrading config loader resolves a present file rather than merging it over the template.
  Then call wire again on a fresh CLI receiver and drive the same entry point the test drives today.

  Rewrite the test's doc comment.
  Its current closing sentences state that the test reaches no live reed session because `requireSessionLocked` fails fast, and spawns no process.
  The replacement must say that `AddStrand` now self-heals, that the test therefore pins its configured multiplexer binary out of existence to keep the assertion spawn-free, and that this is what keeps an untagged test inside the Test Tier Purity Invariant.
- **Commit:** `test(burlercli): keep the told-path wiring assertion spawn-free after the reed self-heal`

## Batch Tests

`verify` runs the untagged tier of every package this batch edits plus `cmd/lyx`, whose repo-wide guards are the ones a comment sweep can trip.
`internal/burlercli` is the only package here with a behavioural assertion at stake: card 8's rework must still fail for the right reason, and must do so without spawning.
`internal/vscode` covers the generated task-file fixture assertions, `internal/webstercli` and `internal/reedengine` are compile-and-regression gates for the files whose comments move, and `cmd/lyx` carries the AST and substring guards — `cmd/lyx/spawnobservability_test.go`, `cmd/lyx/tierpurity_test.go`, `cmd/lyx/hermeticenv_test.go` — that a comment edit or a new test-file construct can break.
The scope is deliberately per-package rather than the whole module: no card in this batch changes behaviour outside the five packages named.
