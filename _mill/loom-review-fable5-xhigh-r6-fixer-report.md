# loom review — round 6 fixer report (fable5-xhigh-r6)

Companion to `_mill/loom-review-fable5-xhigh-r6.md`. What was implemented, what was deferred, the exact verification, and the changed files.

## Findings and disposition

| ID | Severity | Status | Commit |
|----|----------|--------|--------|
| F1 | MEDIUM | FIXED | `d52352305` |

One finding, one fix. No NIT/LOW findings were recorded, so none were deferred. Nothing required an operator decision or an external-dependency change, so the deferred section is empty.

## F1 — Attach discards a satisfied file contract for a running record (FIXED)

- **Defect:** `internal/shuttleengine/attach.go`'s `dispositionCandidate` classified a `run.json` whose persisted `Outcome` is still `runOutcomeRunning`, and whose every declared output file is already on disk, as `verdictRespawnEligible` whenever its pane was dead/untracked/binding-cleared — so `Attach` reported not-found and `SingleLLMProducer` archived the finished output files and re-ran the whole LLM step. The fifth instance of the campaign's file-contract-first defect shape, and the first outside `wait.go`.
- **Fix:** added a top-of-function guard in `dispositionCandidate`: `if c.state.Outcome == runOutcomeRunning && allOutputFilesExist(spec.OutputFiles) { return verdictAttachable }`, placed before the liveness dispatch. The reconstructed run's own `Wait` then harvests the satisfied contract as `OutcomeDone` through the not-tracked/not-live branches rounds 4-5 already hardened, and `finalize` cleans up. Gating on `runOutcomeRunning` keeps the harvest unreachable from a `Discussion-Validate` bounce (a bounce leaves no `running` run.json — `finalize` removed the completed run's dir), so the crash-versus-bounce ping-pong is not reopened.
- **Tests added/changed** (`internal/shuttleengine/attach_test.go`):
  - `TestAttach_RunningRecordSatisfiedFileContract_HarvestsNotRespawn` (NEW) — the running-record + satisfied-contract harvest, across untracked / dead-pane / binding-cleared strands at young and old dir ages; asserts `found==true`, `Outcome==OutcomeDone`, and run-dir cleanup. Verified load-bearing: reverting the `attach.go` change fails all four subtests.
  - `TestAttach_RunningRecordUnsatisfiedFileContract_RespawnsOrErrors` (NEW) — the other side: an unsatisfied contract still respawns (dead pane, old untracked) or errors (young untracked), so the harvest is gated on a genuinely satisfied contract, not the run.json's mere presence.
  - `TestAttach_LeftoverOutputFilesExist` (REMOVED / REPLACED) — this test pinned the exact buggy behavior (a `running` record with existing files asserted `found==false`); replaced by the two tests above.
  - `TestAttach_DeadPane` doc comment tightened to state its no-satisfied-contract precondition explicitly.
- **Docs** (`manifest/designs/loom.md`, same commit): crash-recovery step 2 now records the file-contract-first harvest at `Attach`'s entry and names the round; the crash-versus-bounce section rewritten to explain the `running` run.json as the signal present in the crash case and absent in the bounce, and why the harvest is therefore safe where a naive file-existence check was not.

## Verification (exact commands + results)

Hermetic (all green, before and after the fix):
- `go build ./...` -> exit 0.
- `go vet ./internal/loomengine/... ./internal/loomcli/... ./internal/loomshed/... ./internal/planparser/... ./internal/planglyph/... ./internal/websterengine/... ./internal/webstercli/... ./internal/shuttleengine/...` -> exit 0.
- `go test -count=5 ./internal/loomengine/... ./internal/loomcli/... ./internal/loomshed/... ./internal/planparser/... ./internal/planglyph/... ./internal/websterengine/... ./internal/webstercli/... ./internal/shuttleengine/... ./cmd/lyx/...` -> all ok.
- `go test ./...` (full repo) -> exit 0. Confirms nothing downstream of `shuttleengine` (burler, webster, shed adapters) regressed.
- `go test -tags smoke ./internal/loomcli/... -run Smoke -count=1` -> ok (14.8s), all 13 smoke tests pass.
- `golangci-lint run ./internal/shuttleengine/...` -> exit 0.

Fix-is-load-bearing check:
- Reverted the `attach.go` guard in a scratch copy, ran `TestAttach_RunningRecordSatisfiedFileContract_HarvestsNotRespawn` -> all four subtests FAIL (`found = false; want true`). Restored, re-ran -> ok. The regression test is not vacuous.

Live driving (real built binary, rebuilt from the fixed source):
- Rebuilt `lyx` with `CGO_ENABLED=1 go build -o <scratch>/lyx ./cmd/lyx`.
- Re-staged the F1 scenario in the hand-built fabric hub (`.../crucwarp-HUB/crucible-live`): status `Discussion-Write/running`, reed down, a run dir with `outcome:running`/`started:true` naming both discussion files, both files written with a sentinel.
- `lyx loom drive`: history gained `Discussion-Write:done` and advanced to `Discussion-Validate` (which then bounced on the deliberately-incomplete sentinel content — legitimate, not the bug). Before the fix, the same scenario produced no `Discussion-Write` history entry (files archived, fresh respawn died). The harvest is confirmed live.

## Substrate teardown

- reed session for the live hub torn down (`lyx reed down`); `pgrep -f tmux` confirmed zero stray tmux processes at the end.
- Scratch fixtures (the standalone `fx` plan repo and the `crucwarp-HUB` fabric hub) live under the session scratchpad and are disposable.

## Not fixed (with reasons)

- The `AddStrand`/`run.json` "Accepted residual" (`internal/shuttleengine/run.go`'s `Start`): re-evaluated, three-round judgment holds, no new angle found — a two-independent-stores gap not closable by reordering, and the detection mitigation requires parsing `Launch.Cmd`, which `Launch`'s contract forbids. Correctly left documented. This is a design-tradeoff residual, not a fixable defect this round.
- The fabric weft-branch-naming bug (round 5's incidental find): out of loom's scope, tripped over again while building the hub fixture, worked around (created a `main-weft` branch), not fixed — not this campaign's job.
- Windows-specific path behavior: unreachable from this Linux host; named, never-executed gap.
