# batten — fixer report, round opus-medium-r4

Companion to `_mill/batten-review-opus-medium-r4.md`.
All five findings fixed, one commit each, none pushed.

## Implemented

| Finding | Severity | Commit | Change |
|---|---|---|---|
| F1 | BLOCKING | `39496b045` | `fabricengine.PairComplete` also requires the pair's origin record (found, non-empty `ParentBranch`). A create killed between junction wiring and `WriteOrigin` now gets the incomplete-pair refusal instead of a false done. `docs/overview.md` describes the stronger check. |
| F2 | MEDIUM | `0396b6b76` | `incompletePairRemedy` names `lyx fabric remove --force <slug>` (removes whatever part of the pair `Add` reached, portal/launchers/sibling included) and `git branch -D <branch_prefix><slug>`. The create closure loads the fabric config before the probe so the prefix is known. Overview updated. |
| F3 | MEDIUM | `8ccf2692e` | New `fabricengine.Topology.RemovePairBranch(l, slug)`: refuses while the pair's other side is on disk, otherwise deletes the other-side branch locally when present and on origin when present (gated helpers shared with `Cleanup`), and returns a failed remote deletion as an error. The teardown Remove closure calls it in its "pair already gone" arm and turns `RemoveResult.RemoteBranchError` into a resumable stuck reason. fabric `doc.go`, overview and sandbox F22 updated. |
| F4 | NIT | `1c2de6cec` | `createRefusal` / `doneSlugRefusal` text and doc comments, and the overview line, say a torn-down pair keeps its task branch locally and on the remote (not "both remote copies"). |
| F5 | NIT | `a93f803fb` | `internal/shedcli/table.go` `BootstrapVerb` comment no longer claims nothing reads the field. |

## Tests added or changed (each proven to fail without its fix)

- F1: `TestBattenIntegration_CreateRow_PairWithoutOriginRecordIsIncomplete` (real hub, `AddPair`, origin record removed). With `fabric.go` stashed it failed at `CreateWorktree() error = nil`.
- F2: `TestBattenIntegration_CreateRow_IncompletePairRemedyWorksVerbatimOnAPrefixedHub` replaces the old other-side-leftover test.
  On a `branch_prefix: r4/` hub it builds task worktree + sibling + portal, asserts the remedy text, runs the remedy through `Topology.Remove(force)` + `git branch -D r4/<slug>`, and asserts the resumed create succeeds.
  `TestBattenIntegration_CreateRow_IncompletePairRefusesRatherThanSkippingAdd` now wants the new remedy.
  With `wire.go` stashed, both failed on the remedy text.
- F3: `TestBattenIntegration_Teardown_AlreadyGonePairFinishesItsBranchDeletion` (subtests `LocalAndRemoteLeft`, `RemoteOnlyLeft`), `TestBattenIntegration_Teardown_FailedRemoteDeletionHaltsResumably`, `TestBattenIntegration_Teardown_AlreadyGonePairWithNoBranchIsDone`, and `TestRemovePairBranch_DeletesLocalAndRemoteAndRefusesALivePair` (fabricengine).
  With `wire.go` stashed, the first two failed (all three subtests/tests).
  The Tier 1 tests `TestWire_TeardownIsIdempotentAgainstAnAlreadyRemovedWorktree` and `TestWire_TeardownRefusesAHalfTornPair` dropped their already-gone Remove assertions, because that arm now reaches git; the integration tests above cover it (Test Tier Purity).
- F4: `step_test.go` / `run_test.go` done-slug assertions now pin "task branch" / "locally and on the remote".
- Every new timing-free integration test passed under `-count=5`.

## Commands and results (final source `a93f803fb`)

- `go build ./...`, `go vet ./...`: clean.
- `go test -count=1 ./...`: every package ok (after each fix and at the end).
- `go test -count=5 ./internal/battenshed/... ./internal/battencli/... ./internal/battenrecipe/... ./internal/shedrun/... ./cmd/lyx/...`: ok.
- `go test -tags integration -count=1 ./internal/battencli/... ./internal/shedcli/... ./internal/fabricengine/...`: ok.
- `go test -tags integration -count=5 -run 'Teardown_|CreateRow_|RemovePairBranch' ./internal/battencli/ ./internal/fabricengine/`: ok.
- `go test -count=1 ./tools/sandbox/` (sandbox coverage): ok.

## Live re-verification (redeployed with `./deploy-dev` after each fix)

- F1: `kb8` killed at A8 → resume blocked at Worktree-Create (was: done + dead-end seed). After the F2 remedy, the resumed create and Seed-Child wrote `params.parent: main`.
- F2: `kb8` (default prefix) and `pb5` (A5, `r4/` prefix) — remedy followed verbatim from prime → create done.
- F3: `kb8` killed at R6 → resume done with the other-side branch gone locally and remotely. `pfull` killed at R5 → same result. `ka11` killed at R4, then prune → same result. `pb5` with the weft origin broken → blocked, naming the remote copy; origin restored → done, branch gone from the remote.
- Re-entry on the final source: `kc12` killed at A12 → create done, Seed-Child done. Then killed at R8 → teardown done, nothing left.
- Full run on the final source: `lyx batten run pfin --child-driver go` on the prefixed hub, untyped task → exit 0, `outcome: done`.
  Both child halves had empty `git status --porcelain` at the moment the child turned done.
  Afterwards `r4/pfin` was on the warp remote, `r4/pfin-weft` on neither side, and prime `main` printed `hello`.

## Deferred

None of this round's findings. The campaign's standing deferred items were re-evaluated in the review report; nothing in them changed.
`--child-driver llm` was not driven (operator-accepted `ly` plugin gap). Windows was not touched (unreachable from this host).

## Changed files

- `internal/fabricengine/fabric.go`, `internal/fabricengine/cleanup.go`, `internal/fabricengine/doc.go`, `internal/fabricengine/cleanupremote_integration_test.go`
- `internal/battencli/wire.go`, `internal/battencli/arm.go`, `internal/battencli/wire_test.go`, `internal/battencli/lifecycle_integration_test.go`, `internal/battencli/step_test.go`, `internal/battencli/run_test.go`
- `internal/shedcli/table.go`
- `docs/overview.md`, `tools/sandbox/SANDBOX-FABRIC-SUITE.md` (F22)

## Teardown

See the review report's Teardown section: both fixture hubs deleted, no fixture processes left, no issues filed, bench untouched.

## Verdict

MERGEABLE.
The normal single-instance flow reached done twice, the second time on the final source and a prefixed hub.
Every finding is fixed with a sabotage-proven test and re-driven live.
