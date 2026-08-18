# shuttle — fixer report, round 1 (`opus-medium-r1`)

Job 2 of `_mill/shuttle-review-prompt.md`, fixing every finding recorded in `_mill/shuttle-review-r1.md`.
The review report was complete and committed (`fc10c795`) before any production or test file was touched.

**All 10 findings fixed. Nothing deferred.**
Commit-per-fix, on `reed-shuttle-crucible-hardening`, never pushed.

## Fixes, in landing order

| Finding | Severity | Commit | Files changed | Verified by |
| --- | --- | --- | --- | --- |
| R1-F8 stale package doc | NIT | `7d3cd491` | `internal/shuttleengine/doc.go` | build/vet/test |
| R1-F9 fork-hook comments describe a guard that does not exist | NIT | `7fa47ef8` | `internal/shuttleengine/claudeengine/settings.go` | build/vet/test |
| R1-F4 fixture passes one dir as both told paths | LOW | `e0f5107e` | `internal/shuttleengine/run_test.go` | `-count=3` on `TestRunner_Start*` |
| R1-F6 untracked-strand message asserts one cause | LOW | `e30f2786` | `run.go`, `run_test.go` | new assertion in `TestRun_InterruptAndSend_RefuseDeadOrUntrackedStrand` |
| R1-F5 unreadable `run.json` reported as "not a shuttle strand" | LOW | `fc1dc45f` | `rundir.go`, `rundir_test.go` | new `TestFindRunByStrand_MissNamesUnreadableDirs` **+ live re-drive** |
| R1-F10 teardown not logged through `internal/logger` | LOW | `6e1cccf3` | `wait.go`, `wait_test.go` | new `TestRun_Wait_LogsTeardownThroughLogger`, **proven to fail without the fix** |
| R1-F7 resume line drops every launch flag | LOW | `7951c86a` | `claudeengine/command.go`, `claudeengine.go`, `command_test.go` | extended `TestBuildResumeCmd` (4 cases) |
| R1-F2 mechanism failure discards the run's identity | MEDIUM | `342482ed` | `wait.go`, `shuttlecli/run.go`, `wait_test.go`, `cli_test.go` | 3 new tests **+ live re-drive of the exact reproduction** |
| R1-F3 told anchor/worktree pair validated nowhere | MEDIUM | `b68cfd21` | `run.go`, `doc.go`, `run_test.go`, `run_inject_test.go`, `shuttlecli/cli_test.go` | new `TestNewRunner_RefusesUnusableToldPaths` (5 cases) + `TestNewRunner_AcceptsHubGeometryShapes` **+ live re-drive** |
| R1-F1 smoke suite pins no model | MEDIUM | `f16eaba7` | `smoke_run_test.go`, `smoke_guardrail_test.go`, `smoke_interrupt_test.go` | **all four smoke tests re-run live on `haiku`** |

## What each fix does, where it is not obvious from the finding

- **R1-F3** adds `validateToldPaths`, computed once in `NewRunner` and stored as `Runner.toldErr`, returned by `Start`/`Interrupt`/`Send`/`Inject`.
  The constructor stays total (no signature change, so all five production call sites and every downstream test are untouched).
  Three clauses: non-empty, both absolute, and `anchorPath` inside-or-equal `worktreeRoot`.
  The third is the swap detector rather than a geometric preference — `hubgeom.ReedGeometry` reads both fields off one resolved `*lyxcwd.Location`, so `AnchorPath == WorktreeRoot/AnchorRel` always holds and a swap is exactly what violates it.
  Equality is allowed, since `AnchorRel` is `"."` for a worktree anchored at its own root.
  Two pre-existing fixtures (`newInjectTestRunner`, `newInterruptTestRun`) and one in `shuttlecli/cli_test.go` used two unrelated `t.TempDir()` calls — distinct, as R1-F4's rule requires, but not in the relation real geometry has;
  they now use a real subpath anchor, which satisfies both.
- **R1-F2** adds `(*Run).identity()` and returns it from all three of `Wait`'s no-classification exits, plus `output.ErrFields` wiring in the `run` verb so the operator sees `guid`/`sessionId`/`runDir` on a failure envelope.
  `identityFields` returns `nil` before a strand exists, so a flag or validation error is not decorated with three empty strings.
  The doc comment records explicitly why this is NOT reclassified as `OutcomeDied`, so a later round does not "finish the job" by guessing.
- **R1-F7** changes `buildResumeCmd`'s signature to take `model`, `effort`, and `interactive`. It is unexported with one call site.
- **R1-F10** uses `logger.SetOutput` + `logger.SetVerbosity(1)` as the test seam (restored via `t.Cleanup`).

## Verification

Hermetic, green after every commit and at the end:

- `go build ./...`
- `go vet ./internal/shuttleengine/... ./internal/shuttleengine/claudeengine/... ./internal/shuttlecli/...`
- `go vet -tags smoke ./internal/shuttlecli/...` and `go build -tags smoke ./...` (the smoke files are not covered by the untagged vet)
- `go test -count=5 ./internal/shuttleengine/... ./internal/shuttleengine/claudeengine/... ./internal/shuttlecli/... ./cmd/lyx/...` — all four ok
- `go test ./...` (whole repo, once, at the end) — no failures

Live re-drives against the redeployed `./deploy-dev` snapshot (`./deploy-dev` was re-run after every source change that a live check depended on):

| Fix | Live check | Result |
| --- | --- | --- |
| R1-F5 | Hand-seeded a damaged `run.json` beside a healthy one, `lyx shuttle interrupt` | Error now names `1 run directory … could not be read` and warns the agent may still be in its pane; removing the damaged dir restores the plain miss (no false hedging) |
| R1-F2 | Re-ran the exact reproduction — `lyx reed down` under an in-flight run | Envelope now carries `guid`, `sessionId`, `runDir` alongside the error |
| R1-F3 | `lyx shuttle run` through the real `shuttlecli` wiring | `outcome:"done"`, file written, strand and run dir cleaned — production's `reedGeom.AnchorPath`/`reedGeom.WorktreeRoot` pair passes validation |
| R1-F1 | All four smoke tests, one at a time, by exact name, foreground | `TestSmokeShuttleRunWritesOutputAndCleans` PASS 11.0s · `TestSmokeGuardrailDeniesAgentTool` PASS 21.7s · `TestSmokeGuardrailAskingSurfacesQuestion` PASS 11.0s · `TestSmokeInterruptSendContinues` PASS 19.2s (`outcome=done`) |

Live-substrate discipline held throughout: every real `claude` was `--model haiku`, one process at a time, foreground and waited on;
no bare `-run Smoke` sweep;
no N×-concurrent run.

## Teardown

`ps -eo comm | grep -cx 'tmux: server'` = **0**.
`pgrep -a -x claude` lists only the operator's own long-lived sonnet/opus sessions — **zero stray `haiku` processes**.
`lyx reed down` run, `.lyx/shuttle` emptied, the fabricated foreign-session `reed.json` and its live fixture session removed.
`git status --porcelain` clean.

## Docs updated in the same change as the behaviour they document

- `internal/shuttleengine/doc.go` — the stale hermeticity paragraph (R1-F8) and a new sentence recording that "told" does not mean unchecked (R1-F3).
- `internal/shuttleengine/claudeengine/settings.go` — three comments (R1-F9).
- Godoc on `Wait`, `finalize`, `findRunByStrand`, `buildResumeCmd`, `NewRunner`/`validateToldPaths` carries each fix's rationale.

Nothing here moves scope or adds a cross-cutting invariant, so `docs/overview.md`, `CONSTRAINTS.md`, and `manifest/roadmap.md` are deliberately untouched.
R1-F10 makes shuttle COMPLY with an existing `CONSTRAINTS.md` invariant (Live-Substrate Spawn Observability) rather than changing it.

`tools/sandbox/SANDBOX-SHUTTLE-SUITE.md` needed no extension: every behaviour these fixes touch is either already covered by a tagged scenario or is a hermetic/CLI-envelope property with no visual dimension, and `sandbox_coverage_test.go` stays green.

## Deferred

None.

## Named residuals for the next round (recorded, not fixed — see the review's Observations section)

- **`Wait` still cannot distinguish "reed is gone" from "reed refuses"** and therefore cannot classify a lost session as `OutcomeDied`. Blocked on reed exposing a typed no-session sentinel, which is a reed change and out of this campaign. R1-F2 mitigates the consequence (the caller keeps its handles) without guessing.
- **The orphan sweep trusts a `reed.json` reed itself refuses to act on**, so a copied `.lyx` can sweep this worktree's kept diagnosis dirs. Inherent to the sweep's contract, already documented in `template.yaml`'s `run_dir` comment, and closing it needs the same reed API.
- **`posix.go` carries a Windows verification gap** — read for correctness, not drivable on this Linux host.
