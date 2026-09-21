# batten review — sonnet-xhigh-r5

Round 5. Independent, clean-room review + fix of the `batten` module, per
`_mill/batten-review-prompt.md`. This file is built incrementally per that
prompt's "Log as you go" rule: observations and provisional findings are
appended as each scenario/command returns, before the final executive
summary and severity ordering are written.

Clean-room note: this review's own findings list (below) was written before
reading any prior round's `_mill/batten-review-*` material. Prior-round
context was read only afterward, to re-confirm previously-fixed behavior and
re-evaluate the two deferred items — never to shape the findings list itself.

## What was tested

### Static / code reading (Job 1, phase 1)

- Read `internal/battenshed/*.go` (create.go, seamchild.go, innerrun.go,
  teardown.go, ctx.go, deps.go, stuck.go, doc.go) and every corresponding
  `_test.go`, plus `seam_enforcement_test.go`.
- Read `internal/battenrecipe/*.go` (battenrecipe.go, names.go, doc.go) and
  its tests (recipe_test.go, coverage_guard_test.go, seam_enforcement_test.go;
  skimmed fixture_test.go).
- Read `internal/battencli/*.go` in full: arm.go, bootstrapverb.go,
  cli.go, commitstatus.go, paths.go, refusal.go, wire.go, and
  `lifecycle_integration_test.go` in full. Skimmed the remaining `_test.go`
  files' test-name inventory (arm_seed_test.go, cli_test.go,
  commitstatus_test.go, paths_test.go, refusal_test.go, run_test.go,
  step_test.go, wire_test.go) to confirm coverage shape; read
  `TestChildSpawnError` and the write-seed-failure tests in full.
- Read `contracts/recipes/batten-recipe.yaml`.
- Recovered and read the deleted design doc:
  `git show 8ac857ce1~1:manifest/designs/seeded-shed.md` in full.
- Read `docs/overview.md`'s batten entry (lines 369-380) and nested-Shed
  paragraph (lines 412-413).
- Read `CONSTRAINTS.md` in full (390 lines) — all invariants, not just the
  batten-named ones.
- Read `tools/sandbox/SANDBOX-FABRIC-SUITE.md`'s F22 section in full
  (lines 529-551).
- Diffed every non-batten file this campaign has touched since its branch
  point (`git diff --stat 29d9e6a42^..HEAD`, then full diffs on the
  interesting ones): `CONSTRAINTS.md`, `internal/fabricengine/fabric.go`
  + `doc.go` (RequireDrivableWorktree), `internal/loomcli/bootstrap_test.go`
  (driverFieldReadCarveOuts), `internal/boardcli/cli.go`,
  `internal/shedrecipe/entries_batten.go` (+test),
  `internal/reedengine/{spawn.go,doc.go,panebin.go,...}`,
  `internal/shell/{shell.go,posix.go,pwsh.go}` (the #017 Pane Binary
  Resolution merge). Grepped the whole campaign diff for stray corrupted
  unicode (curly quotes) as a cheap blast-radius smoke check.
- Grepped every production call site of `fabricengine.RequireWarpWorktree`
  and confirmed the only caller outside `fabricengine` itself is
  `internal/fabriccli` (an owner-set package per the Fabric Vocabulary
  Invariant) — `internal/battencli` uses the vocabulary-neutral
  `RequireDrivableWorktree` wrapper exclusively. No second non-owner
  bare-caller found (re-confirms F6[R4]'s fix is still complete).
- Delegated a fact-gathering-only (no-judgment) sub-task to a fork for
  boundary code I needed precise citations on rather than my own review
  judgment: `internal/shedengine`'s Stuck-vs-hard-error status handling,
  `plugins/ly/skills/ly-drive/SKILL.md`'s step loop, and — most
  importantly — `internal/loomcli/start.go`'s exact `--no-attach` blocking
  contract for both `driver: go` and `driver: llm`. Findings folded in
  below (see "Focus item 4" and "Boundary facts").

### Hermetic gate (baseline, before any fix)

- `go build ./...` — clean, no output.
- `go vet ./internal/battenshed/... ./internal/battencli/... ./internal/battenrecipe/...` — clean.
- `go test -count=5 ./internal/battenshed/... ./internal/battencli/... ./internal/battenrecipe/... ./cmd/lyx/...` — all `ok`.
- `go vet ./...` — clean (`VET_EXIT=0`).
- `go test -count=1 ./...` — all `ok`, zero FAIL/panic across every package in the repo (94-line summary, one `ok` line per package).

### Operational note: pre-existing scratchpad content

This session's scratchpad directory
(`.../c7ce4f6e-42d7-4add-825c-c14fded/scratchpad`) already contained
leftover shell scripts and logs from what its own `env.sh` comment names
"the batten r4 live-driving scenarios" (fixture-hub build/drive/teardown
scripts, `s-*.sh` scenario drivers, `success-run*.log`). This is not
`_mill/batten-review-*` material (the clean-room bar), so reading script
*names* to confirm disposal is not a clean-room violation, but I did not
read the narrative log content, to avoid priming my own findings. Verified
directly: the fixture hub directory these scripts built
(`scratchpad/hub`) no longer exists on disk (disposed), no stray `lyx`
process, no stray tmux socket, and the operator's real standing bench
(`~/Code/lyx-test-HUB`) is present and untouched. This round builds its own
fresh, separately-named fixture hub rather than reusing or trusting that
leftover state, and its own live-driving deliverable is independent of it.

(Live-driving scenarios continue below as they are run.)

## Findings (provisional — filled in during Job 1, ranked at the end)

Recorded as spotted; severity/CONFIRMED-vs-PLAUSIBLE finalized once live
verification is complete.
