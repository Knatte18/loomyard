# `shuttle` — fixer report, round 2 (`opus-high-r2`)

Companion to `_mill/shuttle-review-r2.md`. Job 2 of the round: every SMALL finding fixed, one commit each,
nothing pushed.

## Outcome

| | count |
| --- | --- |
| findings recorded | 11 |
| fixed inline | **10** |
| named NOT-FIXED-THIS-ROUND (too large) | **1** (R2-F11) |
| deferred for any other reason | **0** |

Severities: 2 MEDIUM, 7 LOW, 2 NIT. No BLOCKING.

## Fix log — one commit per finding

| finding | sev | commit | what changed |
| --- | --- | --- | --- |
| R2-F9 | MEDIUM | `e6ea9d05` | `wait.go`: startup deadline lifted out of the `StartupPending` arm into `classifyStartupWindow`, called from every not-yet-ready exit |
| R2-F2 | LOW | `41a70c3d` | `wait.go`: fork-audit failure returns the classified `Result`, not `run.identity()`; `run.go`'s `Result.ForkAudit` doc corrected |
| R2-F8 | LOW | `ec205fee` | `run.go`/`wait.go`: five `log.Printf` sites → `logger.Warn`; `"log"` import dropped from both; `CONSTRAINTS.md` invariant extended |
| R2-F1 | LOW | `bbe5636b` | `shuttlecli/run.go`: `ShouldAbort` moved to the top of `RunE`; `CONSTRAINTS.md` CLI/Cobra Invariant gains the one-envelope rule |
| R2-F5 | LOW | `354946c1` | `claudeengine/audit.go`: `readTranscriptLines` → streaming `forEachTranscriptLine`; both callers converted |
| R2-F6 | LOW | `cdb22bf8` | `run.go`: `sendVerified` re-baselines when the needle count falls below the baseline |
| R2-F3 | MEDIUM | `934abdb9` | new `shuttlecli/modelpin_test.go`: untagged source-scan guard over the smoke suite's four real-`claude` spawn sites |
| R2-F4, R2-F7, R2-F10 | NIT, NIT, LOW | `22461927` | `template.yaml`, `docs/overview.md`, `engine.go`, `shuttlecli/run.go` help text |

Review-report commits: `cbbed01e`, `b5c0bf3b`, `9921268a`, `86451f81`. This report: final commit.

## Every fix carries a test, and every test was sabotage-proved

The proof procedure for each: revert the production change (only), confirm the new test fails **at its intended
assertion**, restore, confirm green. Where a fix had more than one way to regress, each way was proved
separately.

| finding | test added | sabotage-proved by | it failed with |
| --- | --- | --- | --- |
| R2-F9 | `TestRun_Wait_StartupDeadline_BindsEveryNotReadyPath` (3 subtests: trust prompt that never clears, capture fails every probe, still booting) | restoring the deadline check to the `StartupPending` arm only | the two silent paths ran to the 10-minute run deadline and classified `timeout`, not `died` |
| R2-F2 | `TestRun_Wait_ForkAuditFailure_KeepsTheClassifiedOutcome` | `return result` → `return run.identity()` | `Outcome = ""; want "done"` |
| R2-F8 | `TestRunner_Start_StrandTeardownFailure_LogsThroughLogger` + `TestShuttleengine_LiveSubstrateLoggingGoesThroughLogger` (source scan) | one `logger.Warn` → `log.Printf` | behavioural test lost the message from the captured logger; source scan named `run.go` and the banned token |
| R2-F1 | `TestRunCLI_Run_PreRunAbort_EmitsExactlyOneEnvelope` (3 subtests) + `parseSingleEnvelope` helper now used by `TestRunCLI_Run_FlagValidation` too | moving `ShouldAbort` back below the flag checks | a second JSON envelope followed the first |
| R2-F5 | `TestForEachTranscriptLine_StreamsRatherThanMaterialising`, `TestForEachTranscriptLine_EdgeCases` (4 subtests), `TestAuditForkTranscript_UnterminatedFinalLineStillCounted` | **(a)** restoring the `os.ReadFile`+`strings.Split` original, **(b)** replacing it with a `bufio.Scanner` port | (a) peak live heap 1.17× the file vs the 0.5× limit; (b) `bufio.Scanner: token too long` on both oversized-line cases |
| R2-F6 | `TestRun_Send_BaselineOccurrencesScrolledAway_NoDuplicateDelivery` | removing the re-baseline branch | `sent text never appeared in the pane after 2 attempt(s)` — i.e. the spurious replay |
| R2-F3 | `TestSmokeSuite_EveryRealClaudeSpawnPinsTheModel`, `TestSmokeSuite_ModelPinConstantIsDeclaredAndNonEmpty` | **five ways**: drop the pin from a `RunCLI` site; drop it from the `Spec` literal; empty the constant; rename the constant away; break a spawn site so the recogniser misses it | each named its own case; the site-count check caught the vacuous-pass case (`recognised 3 … want 4`) |
| R2-F4, R2-F7, R2-F10 | docs only — verified by reading the shipped artifacts back (`lyx shuttle run --help` re-rendered from the deployed binary) | n/a | n/a |

Two of these tests were built from measurements rather than guesses:

- **R2-F5's memory threshold** is 0.5× the file size, chosen after measuring both implementations. Streaming
  peaks at **0.02×** on the fixture; the materialising original cannot get below ~1.17× (it holds the raw
  bytes and a full string copy simultaneously). Roughly two orders of magnitude of margin on the passing side
  and a proved failure on the other, so the assertion does not depend on allocator or GC detail.
  `runtime.GC()` before each sample is what makes it deterministic — it measures what the walk still HOLDS,
  not what it has cumulatively allocated (`TotalAlloc` does not separate the two implementations at all: 3.14×
  streaming vs 4.64× materialising, which is why the obvious metric was rejected).
- **R2-F3's `wantSpawnSiteCount = 4`** exists so a recogniser that silently stops matching fails loudly rather
  than passing vacuously. Same reasoning as the `scanned == 0` guard in R2-F8's source scan.

## Verification

### Hermetic, after every fix and again at the end

- `go build ./...` — clean.
- `go vet ./internal/shuttleengine/... ./internal/shuttleengine/claudeengine/... ./internal/shuttlecli/...` — clean.
- `go vet -tags smoke ./internal/shuttlecli/...` — clean.
- `go test -count=5 ./internal/shuttleengine/... ./internal/shuttleengine/claudeengine/... ./internal/shuttlecli/... ./cmd/lyx/...` — all `ok`.
- **`go test ./...` (whole repo) — clean**, which is what confirms the `CONSTRAINTS.md` and `docs/overview.md`
  edits did not trip the markdown-link, vocabulary-walk, help-tree, or tier-purity guards.
- `gofmt -l internal/` reports only two files this round never touched
  (`internal/lyxcwd/docslink_test.go`, `internal/shell/posix.go`) — pre-existing, confirmed via `git status`.

### Live substrate, after the fixes, redeployed via `./deploy-dev`

All four smoke tests, run **one at a time, by exact name**, each spawning exactly one real `claude` on
`--model haiku`:

| test | result | wall |
| --- | --- | --- |
| `TestSmokeShuttleRunWritesOutputAndCleans` | PASS | 13.87 s |
| `TestSmokeInterruptSendContinues` | PASS (`outcome=done`) | 18.84 s |
| `TestSmokeGuardrailDeniesAgentTool` | PASS | 20.80 s |
| `TestSmokeGuardrailAskingSurfacesQuestion` | PASS | 11.45 s |

`ps -eo comm | grep -cx 'tmux: server'` = 0 after each.

Direct driving of the rebuilt binary:

- **Happy path re-driven end to end** — `lyx shuttle run --model haiku …` → `{"ok":true,"outcome":"done",…}`,
  output file correct, strand removed, run dir removed. This is also what proves R2-F9 did not make a normal
  startup fail early: five real launches (this plus the four smoke tests) all reached `StartupReady` well
  inside the window.
- **R2-F1 re-driven** — the two-envelope reproduction from the review now emits exactly one, and the third leg
  (a flag error from inside a real worktree, where no abort happened) still reports the flag error on its own.
- **R2-F8 re-driven** — the same zero-`claude` reproduction (corrupt `.lyx/reed.json`, then `lyx shuttle run`,
  which fails at `AddStrand` before any spawn) now puts the sweep-skip message **in the trace file**, at WARN,
  with its `trace=` correlation id and structured `anchorPath`/`error` fields. Before the fix that same
  message existed only on stderr and was absent from the trace file for the identical invocation.
- **New sandbox scenario S4 driven, all three legs** — see below.

### Sandbox suite

`tools/sandbox/SANDBOX-SHUTTLE-SUITE.md` gains **S4 — "One envelope per invocation, even when two things are
wrong at once"**, the black-box form of R2-F1. It is deliberately a zero-token scenario: it starts no agent.
It includes the third leg that keeps the fix honest — inside a real worktree the flag error must still be
reported on its own, because suppressing it *always* would be a different bug. All three legs driven live
against the deployed binary; `cmd/lyx/sandbox_coverage_test.go` stays green.

R2-F9, R2-F5, R2-F6 and R2-F2 got no sandbox scenario: each needs a fault an operator cannot induce from a
shell (a trust prompt that will not dismiss, a several-hundred-MiB transcript, a specific viewport scroll, a
broken `~/.claude/projects`). They are pinned by the hermetic tests above instead.

## Docs updated in the same commit as the change they document

- `CONSTRAINTS.md` — **Live-Substrate Spawn Observability** gains `internal/shuttleengine/wait.go` to its
  instrumented-call-sites list and a clause stating that a live-substrate module's operational messages all
  belong on `internal/logger`, not only its spawn/teardown lines (with `shuttleengine`'s own source scan named
  as its machine check and every other module named as a review obligation). **CLI / Cobra Invariant** gains
  the one-envelope-per-invocation rule and the `ShouldAbort`-first placement it depends on.
- `docs/overview.md` — shuttle bullet: `asking`'s real meaning, and the `Agent` deny's two real conditions.
- `internal/shuttleengine/engine.go` — `OutcomeAsking`'s doc comment.
- `internal/shuttleengine/run.go` — `Result.ForkAudit`'s doc comment.
- `internal/shuttleengine/template.yaml` — `run_dir`'s comment.
- `internal/shuttlecli/run.go` — the `run` verb's `Long` help text, re-rendered from the deployed binary and
  read back.
- `manifest/roadmap.md` — deliberately **not** touched: this is a hardening pass, which the repo's task-completion
  rule excludes from the roadmap.

## NOT-FIXED-THIS-ROUND

**R2-F11 — a count over a scrolling viewport cannot soundly verify pane delivery.** LOW severity, LARGE size.
Full detail in the review report. Summary of why it is not an inline fix: closing it means REPLACING
`sendVerified`'s delivery signal rather than adjusting it, and the signal being replaced is a live-proven
guarantee whose failure direction is silent. It needs a design step and live validation against a real Claude
TUI. R2-F6's shipped fix closes the reachable half (a baseline inflated by occurrences that later scroll away);
the residual is the exact one-off-as-one-arrives case, which two counts genuinely cannot distinguish from
non-delivery. Recommended for its own mill-wiki task.

## Teardown

Measured against the baseline recorded at the top of the review report, not against zero.

- `ps -eo comm | grep -cx 'tmux: server'` = **0**.
- `claude` processes: **19**, all from the pre-existing baseline pid set (`1472397`, `1922201`, `3145593`,
  `3187175`, `3187599`, plus 14 unrelated `claude-desktop`). One baseline session (`3145825`) exited on its own
  during the round. **Zero processes created by this round survive.**
- `.lyx/shuttle/` empty; `.lyx/reed.json` absent (cleared by `lyx reed down`).
- No source-tree litter under `internal/shuttleengine` or `internal/shuttlecli`.
- Nothing pushed. All work is commits on `reed-shuttle-crucible-hardening`.

## What I could not verify, stated plainly

- **Subpath-anchored geometry, live.** This worktree is anchored at its own root, so `anchorPath ==
  worktreeRoot` in every live scenario. R2-F4's correction is about exactly the case that never differed here;
  it is verified against the code and the hermetic matrix, not against a live subpath-anchored hub.
- **R2-F9's two failure modes, live.** Forcing a trust prompt that will not dismiss, or a pane that fails every
  capture, against a real already-trusted `claude` is not something I could arrange. The fix and its three
  subtests are hermetic; what live driving proves is only the other direction — that five real launches still
  start normally.
- **R2-F6/R2-F11's viewport scroll, live.** Named in the finding: the timing cannot be forced deterministically
  without a fake.
- **Windows.** Linux host; `posix.go`'s Windows path was read, not driven, per the round's scope.
