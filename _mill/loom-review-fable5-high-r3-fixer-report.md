# loom fixer report — round 3 (fable5-high-r3)

Companion to `_mill/loom-review-fable5-high-r3.md`. Every finding fixed, all severities; two additional Mission-A findings (F-A3, F-A4) surfaced during post-fix live verification and were fixed in the same round.

## What was implemented (mission-tagged, commit per fix)

| Finding | Commit | Change | Tests added |
|---|---|---|---|
| F1+F1b [B] | `c2bcaa4e1` | `planglyph.resolvePass` excludes Rename pairs' New sides from the status policy via new `renameNewTargetSet` (mirroring the pure layer's Pairs.New rule); `targetCards` attributes one finding per card, not per occurrence | `TestResolvePass_FileRenameNewSideIsNotAFinding`, `TestTargetCards_IndexesACardOncePerTarget` |
| F3 [B] | `fb5339ec1` | `DoneChecks` gains `rename-not-done`: old side still resolving, or new side (handle-stripped) still missing, blocks record-batch; `planglyph/doc.go`'s canonical ID list updated | `TestDoneChecks_RenameLanded`, `TestDoneChecks_RenameSkipped`, `TestDoneChecks_FileRenameLanded` (integration tag) |
| F4 [B] | `fcd4d508f` | `CanonicalizeHandles` skips identity substitutions; `rewriteCardFile` guards `newLine != line`, honoring RewriteRefs' byte-identical-no-write contract | new subtest of `TestCanonicalizeHandles_ReportsWhetherItRewrote` |
| F5 [B] | `3c79210a7`, `a9d735541` | six stale format-4 doc comments in `planparser/plan.go`/`parse.go` corrected to format-5 | doc-only |
| F6 [B] | `848fce2df` | `DetectDrift(fullPlan, pending, ...)`: gate one reads the FULL plan's declared pairs (the recording batch is excluded from pending, so its own declared outcome could never match); refs/sweeps stay pending-scoped | `TestDetectDrift_GateOneSeesTheRecordingBatchOwnPair` (untagged, hand-built delta) |
| F2 [B] | `0d5f2dcf5` | `websterengine.Run` validates AFTER the state phase settles, via `ValidateDispatch` scoped by `completedCards`; explicit approval gate kept at entry; fingerprint restamped+saved post-validation | `TestRun_ResumeWithCompletedCreateCardIsNotRefused`, `TestRun_UnapprovedPlanRefused` (integration tag) |
| F-A1 [A] | `af56a2ce5` | webstercli+burlercli standalone wirings arm an in-process `reedUp` seam; the run verbs fire it (idempotent `reedengine.Up`) before any spawn — `lyx reed up` is hub-only and could never reach standalone's geometry. (Commit also carries F-A3's guard — same files, flagged here for the audit trail.) | `TestWire_ReedUpSeamPerMode` in both CLIs |
| F-A3 [A] | `af56a2ce5` | standalone `run` refuses a `--plan-dir` that moves the plan off the default (`<stateDir>/_lyx/plan`) — Master's in-pane verbs are flagless and resolve the default; every other verb keeps honoring the override | `TestWireStandalone_PlanDirOverrideMarksRunRefusal` |
| F-A4 [A] | `f78f998ba`, `69367f5df` | Master/fork/recovery prompts render the TOLD plan directory and integration-report path (new `plan_dir`/`integration_report_path` markers in `webster-template-master.md`; card pointers re-rooted); the display's relative base is the PANE's cwd (`Geometry.WorktreeRoot`) — the first cut keyed on AnchorRoot, which in standalone IS the state dir, and re-rendered the unreachable relative spelling (caught by the second live E2E attempt); hub renders byte-identical `_lyx/plan` | `TestMasterPlanDirDisplay`, `TestRenderCardPointers_ReRootsOntoPlanDirDisplay`; template marker tests extended |
| F-B7 [B] | `37cccedc5` | `loomengine.VerifySeedOwnership` + guards in `lyx loom run`/`drive`: a status file recording another task's slug refuses loudly with the reset recourse, instead of silently resuming the inherited run | `TestVerifySeedOwnership` (3 subtests) |
| docs | `4acbdea4d` | `docs/overview.md` webster section records the standalone run's reed bring-up, default-plan-dir requirement, and told-path prompts | link gate green |

No fix touched `NewRunner`'s containment assertion or any hub-mode spawn path semantics; the hub-prompt bytes are pinned unchanged by the display tests.

## Deliberately deferred

- **Full `--plan-dir` override support for standalone `run`** (propagating the override into Master's in-pane verb invocations, e.g. via pane environment) — a cross-cutting design decision touching reed's spawn environment and webstercli's flag precedence; F-A3's loud refusal with a working recourse covers the operator today. Recorded for the orchestrator to spin into its own task if wanted.
- **`lyx reed` standalone support** (attach/status against a derived-geometry session) — the operator can reach the session via raw `tmux -L lyx-<hash8> attach`; giving reedcli a standalone mode is its own module task. F-A1's in-process bring-up removes the blocking half.

## Verification

- Hermetic, after every fix and cold at the end (see below): `CGO_ENABLED=1 go build ./...`, `go vet` over the full expanded package set, `go test -count=5` over the same set + `cmd/lyx`, `go test -tags integration` over the seven-package set, whole-repo `go test ./...`.
- Live, per finding: F1 (file-rename plan now validates clean via standalone `lyx webster validate`), F4 (backdated card mtimes survive a validate), F-A3 (refusal envelope observed with the exact recourse text), F-A1/F-A4 (real standalone `lyx webster run`: reed session boots on socket `lyx-b1c70921`, Master orients against the told plan dir and opens batch brackets — full E2E transcript in the review report's What-was-tested), F2 (re-run of the completed standalone run re-reports done — see below).

### Additional fixes landed after the draft above

| Finding | Commit | Change | Tests |
|---|---|---|---|
| F-B7 (vocabulary) | `3b95e1e4c` | ownership guard's comments/error reworded off the bare fabric-side tokens (`TestEnforcement_FabricVocabulary` caught the first wording; green after) | enforcement gate |
| F-B8 [B] | `7f16dde7e` | `checkHandleMalformed` refuses a file-unit handle (language-gated `.go`-suffix rule) naming the package-directory fix; spec row 15 + Plan-Write stencil `<unit>` guidance updated | `TestCheckHandleMalformed_FileUnitHandle` |

### Final results

- **Live E2E (Mission A, decisive):** `lyx webster run --fresh --target-dir /home/knatte/Code/lyx-r3-standalone` → `{"ok":true,"outcome":"done","batches_done":2}` — the first completed standalone webster run ever. Target commits `00489fe`/`cefc499`, `go test ./greeter/` green, `BindHandles` collapsed the handle to `greeter#Farewell` across both cards. Immediate re-run (F2's live check, two landed Create cards) re-reported `done`.
- **Live hub run (Mission B):** full `lyx loom run` pipeline with two `Plan-Write`-authored Rename cards to `Finalize → done` (evidence in the review report).
- **Final cold gates at HEAD:** `go build ./...`, `go vet` (expanded set), `go test -count=5` (expanded set + cmd/lyx), `go test -tags integration` (seven packages), whole-repo `go test ./...` — all green. `golangci-lint` over the changed packages: only three pre-existing findings (present at the round's base commit), untouched.
- **Teardown:** standalone tmux server killed, hub reed session downed, sandbox repos committed-clean, zero substrate processes from this round remain.
- **Not live-verified:** F-B7's mismatch refusal (unit-tested; the live worktree's state now legitimately matches its own slug), burlercli's standalone reed bring-up (same seam as webstercli's, wiring-pinned; no standalone burler profile scenario was in this round's scope).
