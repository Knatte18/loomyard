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

(final gate outputs and the E2E terminal envelope appended below once the last live run lands)
