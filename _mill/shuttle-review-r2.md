# `shuttle` — independent review, round 2 (`opus-high-r2`)

> Clean-room round-2 review of `internal/shuttleengine` + `internal/shuttleengine/claudeengine` + `internal/shuttlecli`.
> Written per `_mill/shuttle-review-prompt.md`. Findings formed BEFORE reading any round-1 material.
> Merge bar: correctness in the NORMAL single-instance flow. No N×-concurrent sweep against this module.

## Substrate baseline (recorded before any driving)

- `claude` on PATH: `/home/knatte/.local/bin/claude`, version `2.1.226 (Claude Code)`, logged in.
- `tmux` on PATH: `/usr/bin/tmux`.
- `ps -eo comm | grep -cx 'tmux: server'` = **0** at start.
- `pgrep -c claude` = **20** at start — 14 `claude-desktop` (unrelated GUI app) + 6 `claude` agent sessions
  (pids 1472397, 1922201, 3145593, 3145825, 3187175, 3187599), all pre-dating this round.
  Teardown is judged against this exact pid set, not against zero.
- This worktree is anchored at its own root (no `.lyx-anchor`, `AnchorRel` = `"."`), and has NO `_lyx/`,
  so every live invocation below degraded to shuttle's embedded template
  (`configengine: degrading to embedded template … reason="absent _lyx/ directory"` in the trace log) —
  i.e. `poll_interval_ms: 500`, `liveness_every_n_polls: 10`, `run_timeout_min: 30`, `startup_timeout_s: 90`.
  That also means anchorPath == worktreeRoot here, so subpath-anchored geometry was exercised hermetically, not live.

## What was tested

### Hermetic gates, cold

- `go build ./...` → exit 0.
- `go vet ./internal/shuttleengine/... ./internal/shuttleengine/claudeengine/... ./internal/shuttlecli/...` → exit 0.
- `go vet -tags smoke ./internal/shuttlecli/...` → exit 0.
- `go test -count=1 ./internal/shuttleengine/... ./internal/shuttleengine/claudeengine/... ./internal/shuttlecli/... ./cmd/lyx/...` → all `ok`.

### Source-tree litter left by round 1's development (observation, NOT a finding)

`internal/shuttleengine/.lyx/shuttle/654443a5…/run.json` and `internal/shuttleengine/sub/dir/.lyx/shuttle/b74bcacb…/run.json`
were sitting in the worktree, invisible to `git status` because `.gitignore:15` ignores `.lyx/`.
Their `outputFiles` entry names `/tmp/TestNewRunner_RefusesUnusableToldPaths…/001/out.md`, and their paths
(`./.lyx/…` for an empty anchor, `./sub/dir/.lyx/…` for the relative anchor `sub/dir`) are exactly what the
PRE-R1-F3 code produced when told an empty/relative anchor: it resolved the run-dir root against the process
cwd, which under `go test` is the package directory.

I removed both trees and re-ran the full hermetic suite: **they do not regenerate**. This is dead residue from
round 1's red-phase, not a live defect — `validateToldPaths` now refuses both inputs before any directory is
created. Recorded because it is positive live evidence that R1-F3's fix closed the write path it was written to
close, not merely the error message. Not counted as a finding.

### CLI surface, driven directly against the dev binary (`./deploy-dev` at `cbbed01e`)

| scenario | observed |
| --- | --- |
| `shuttle run` outside a git repo, no `--prompt` | **TWO** JSON envelopes: `{"error":"not a git repository"}` then `{"error":"exactly one of --prompt or --prompt-file is required"}`, exit 1 → **R2-F1** |
| `shuttle run` outside a git repo, `--prompt` + `--prompt-file` | **TWO** envelopes again (`not a git repository`, then the mutual-exclusion error), exit 1 → **R2-F1** |
| `shuttle interrupt abc` outside a git repo | ONE envelope (`not a git repository`) — the verb guards `ShouldAbort` first, so `run` is the outlier |
| `shuttle send abc hi` outside a git repo | ONE envelope — same |
| bare `lyx shuttle` | subcommand listing, no repo required (PersistentPreRunE guard holds) |
| `lyx shuttle bogus` | `{"error":"unknown subcommand \"bogus\" for \"lyx shuttle\""}` |
| `run --output-file <existing>` | refused with the pre-existing-file message; no run dir, no strand |
| `run --timeout -5s` | refused with the negative-timeout message |
| `send abc ""` / `send abc $'a\nb'` | refused (empty/whitespace, multiline) — `validateSendText` holds |
| `interrupt deadbeef` | `shuttle: "deadbeef" is not a shuttle strand: shuttle: no run found for strand "deadbeef"` |
| `run --effort HIGH` | `shuttle: prepare run: claudeengine: invalid effort "HIGH"; …` — engine-side validation, run dir cleaned up |

### Live scenario L1 — happy path (1 real `claude`, `--model haiku`)

```
lyx reed up
LYX_TRACE=1 lyx shuttle run --model haiku \
  --prompt "Write exactly the single word DONE … to /tmp/r2-l1-out.txt … then stop." \
  --output-file /tmp/r2-l1-out.txt --timeout 6m
```

→ `{"ok":true,"outcome":"done","guid":"156a73e344f0b7ac127b73dda88d59b9","sessionId":"36867282-…","runDir":"…/.lyx/shuttle/7799c326…"}`,
`/tmp/r2-l1-out.txt` = `DONE`, `lyx reed status` shows zero strands, `.lyx/shuttle/` is empty (run dir removed).
Wall clock ≈ 10 s.

**R1-F10 re-verified live on my own scenario** — the trace log carries BOTH halves for this run's guid:

```
INFO msg="shuttle: run started"  runDir=…/7799c326… strandGUID=156a73e3… sessionID=36867282-… forkSubagents=false
INFO msg="shuttle: run finished" runDir=…/7799c326… strandGUID=156a73e3… sessionID=36867282-… outcome=done cleanedUp=true
```

**R1-F1 re-verified live** — the trace's `argv=` line records `shuttle run --model haiku …`; the pin reaches the real process.

### Large / malformed transcript behaviour (`AuditForks`)

Malformed input degrades gracefully by construction and I could not break it: `readTranscriptLines` skips blank
lines and every line that fails `json.Unmarshal`, so a truncated final line (abnormal session end), a binary
file, and a file with no newline at all are all absorbed silently. No error, no hang, no miscount beyond the
skipped lines. **Round 1's gap here is closed as "no defect" on the malformed axis.**

The SIZE axis is a different story. I replicated `readTranscriptLines` verbatim (`os.ReadFile` +
`strings.Split(string(data), "\n")` + decode-all-into-a-retained-slice) and measured it:

| input | lines | wall | peak RSS | total alloc |
| --- | --- | --- | --- | --- |
| synthetic 303 MiB | 150 000 | 1.50 s | **931 MiB** (3.07×) | 1406 MiB |
| REAL 83 MiB transcript (largest under `~/.claude/projects` on this host, of 3219 files / 1.4 GB) | 32 098 | 0.49 s | **178 MiB** (2.15×) | 294 MiB |

→ **R2-F5**.

### The `Wait` / reed-failure boundary — round 1's named residual, independently assessed

See the "Named residual" section below. Short version: **I confirm round 1's assessment**, with a refinement.

## Findings

Seven findings: 1 MEDIUM, 4 LOW, 2 NIT. No BLOCKING. None large enough to defer.

---

### R2-F1 — `lyx shuttle run` emits TWO JSON error envelopes when geometry resolution aborts and a flag is also bad

- **Where:** `internal/shuttlecli/run.go:76-99` (flag validation runs before the `clihelp.ShouldAbort` check).
- **Severity:** LOW. **Status:** CONFIRMED (live-reproduced, twice, above). **Size:** small.
- **Scenario:** stand outside a git repo (or in any worktree where `lyxcwd.Resolve`/`LoadConfig`/`ReedGeometry`
  fails) and run `lyx shuttle run --output-file /tmp/x`. `PersistentPreRunE` emits
  `{"error":"not a git repository","ok":false}` and calls `clihelp.Abort`, but `Abort` only records an exit
  code — cobra still runs `RunE`. `RunE`'s flag block deliberately sits AHEAD of its `ShouldAbort` guard, so it
  emits a SECOND envelope on the same invocation.
- **Why it matters, concretely:** the module's own contract is one envelope per invocation, and its own smoke
  tests consume it that way — `smoke_run_test.go` does `json.Unmarshal(out.Bytes(), &result)` on the whole
  buffer, which hard-fails on `{…}\n{…}` with `invalid character '{' after top-level value`. Any programmatic
  caller written the same way (webster, loom, a script) sees a parse error instead of the real, actionable
  cause. The operator additionally gets the SECONDARY problem reported after the primary one, with nothing
  saying which to fix first.
- **`run` is the only verb with this shape** — `interrupt` and `send` both check `ShouldAbort` first and were
  confirmed live to emit exactly one envelope, so this is a local slip, not a repo-wide pattern.
- **Suggested fix:** move the `ShouldAbort` check to the top of `runCmd`'s `RunE`, ahead of flag validation.
  The comment's stated intent ("a bad flag combination is reported as its own flag error rather than being
  swallowed by the abort's already-recorded exit code") is fully preserved: when `PersistentPreRunE` succeeded,
  `ShouldAbort` is false and the flag error is still emitted as its own envelope. Only the both-failed case
  changes, and there the pre-run failure is the one the operator must fix first anyway.

---

### R2-F2 — a fork-audit failure throws away the outcome the run actually reached

- **Where:** `internal/shuttleengine/wait.go:288-295` (`finalize`'s `AuditForks` error branch returns `run.identity()`).
- **Severity:** LOW. **Status:** CONFIRMED (traced; hermetic repro added in Job 2). **Size:** small.
- **Scenario:** a run with `Spec.ForkSubagents` true reaches `OutcomeDone` — every output file written, the file
  contract satisfied. `finalize` then calls `engine.AuditForks`, which fails for a reason that has nothing to do
  with the run's success (the parent transcript is absent because `~/.claude/projects/` was pruned, `$HOME` is
  unreadable, or `subagents/` errored with something other than `IsNotExist`). `finalize` returns
  `run.identity()` — a `Result` with `Outcome: ""`.
- **Why it matters:** `identity()` exists for the case Wait's own doc describes — "a mechanism failure that
  leaves NO classifiable outcome". Here a classification WAS reached, and the code comment on the very next line
  says so out loud ("The run itself SUCCEEDED"), yet the value handed back denies it. The caller now cannot tell
  a run that finished and merely failed its audit from a run that never classified at all — and since the
  audit-failure branch skips cleanup, the strand and run dir are still live either way, so the identity fields
  alone do not disambiguate. `shuttlecli/run.go`'s `identityFields` prints guid/sessionId/runDir and no
  `outcome`, so the operator loses the fact that the agent already did its work.
- **Suggested fix:** return `result` (already fully populated with `Outcome`, `SessionID`, `StrandGUID`,
  `RunDir`, `LastAssistantMessage`) alongside the error instead of `run.identity()`. Strictly more information;
  `ForkAudit` stays nil, which already means "not audited".

---

### R2-F3 — nothing fails when a smoke test stops pinning the cheap model (R1-F1's open regression gap)

- **Where:** `internal/shuttlecli/smoke_run_test.go:41` (`smokeClaudeModel`) and its four call sites across
  `smoke_run_test.go`, `smoke_interrupt_test.go`, `smoke_guardrail_test.go`.
- **Severity:** MEDIUM. **Status:** CONFIRMED (the absence is mechanical — no test references the constant as
  an assertion subject). **Size:** small.
- **I agree the gap is real, and it should be closed.** The `--model haiku` pin is an explicit, campaign-wide
  operator instruction with a direct money cost, and today it is enforced by exactly two things: the constant
  existing, and a human grepping for it. Deleting `"--model", smokeClaudeModel,` from any one call site leaves
  `go build`, `go vet`, `go vet -tags smoke` and the whole hermetic suite green, and the only symptom is a
  silently larger bill on the next `-tags smoke` run — the class of defect that is invisible precisely when it
  is costing something.
- **Scenario:** a future round adds a fifth smoke test, or edits `TestSmokeInterruptSendContinues`'s argument
  slice, and omits the pin. Nothing anywhere goes red.
- **Suggested fix:** an UNTAGGED source-scan guard in `internal/shuttlecli` — the idiom this repo already uses
  for exactly this class (`cmd/lyx/tierpurity_test.go`, `destructiveguard_test.go`, `checkedcall_test.go`).
  Walk `smoke_*_test.go` as TEXT (a source scan reads them regardless of their `//go:build smoke` tag, which is
  the whole reason this works from an untagged test), find every `RunCLI(`-style shuttle-run argument slice, and
  assert each one threads `smokeClaudeModel`. No process spawn, so the Test Tier Purity Invariant is satisfied
  with no allowlist entry; no `go env` call, so no scan-root resolution is needed — the test's own cwd IS the
  package directory.

---

### R2-F4 — `template.yaml`'s `run_dir` comment misstates the run-dir root's base AND its constructor

- **Where:** `internal/shuttleengine/template.yaml:1`.
- **Severity:** NIT. **Status:** CONFIRMED (contradicted by `rundir.go:48-56`). **Size:** small.
- **Text today:** `empty resolves to <worktree>/.lyx/shuttle via hubgeometry (never constructed here)`.
- **Both halves are wrong:**
  1. It resolves against the **anchor path**, not the worktree root — `runDirRoot` joins
     `anchorPath + DotLyxDirName + "shuttle"`, and `rundir.go`'s own comment is emphatic that BOTH branches
     share `anchorPath` deliberately "or a subpath-anchored repo would end up with two distinct run-dir roots".
     In a subpath-anchored worktree the two are different directories — which is the entire premise of
     `validateToldPaths`, whose doc comment states the same distinction correctly. So the shipped config file
     tells an operator the opposite of what the swap detector was built to protect.
  2. `hubgeom` constructs no run-dir root at all — it only converts a `*lyxcwd.Location` into a
     `reedengine.Geometry`. `runDirRoot` in this very package constructs it, so "never constructed here" is
     false about the package the file belongs to.
- **This is the operator-facing spelling of the exact confusion the module spends a whole validator guarding
  against**, which is why it is worth correcting rather than tolerating.
- **Suggested fix:** reword to name the anchor path and drop the hubgeom attribution, keeping the (correct and
  valuable) worktree-local warning that follows it.

---

### R2-F5 — `AuditForks` slurps each transcript whole, amplifying a large session ~2–3× in resident memory

- **Where:** `internal/shuttleengine/claudeengine/audit.go:142-161` (`readTranscriptLines`).
- **Severity:** LOW. **Status:** CONFIRMED by measurement (table above). **Size:** small.
- **Scenario:** `readTranscriptLines` does `os.ReadFile` (whole file → `[]byte`), then
  `strings.Split(string(data), "\n")` (a full COPY into a `string`, plus a `[]string` header per line, all
  alive simultaneously with the original `[]byte`), then decodes every line into a `[]transcriptLine` that is
  retained until the caller returns. Both callers — `auditParentTranscript` and `auditForkTranscript` — then
  iterate that slice exactly once and never index it again.
- **Measured:** a real 83 MiB transcript (the largest of 3219 under `~/.claude/projects` on this host) peaks at
  **178 MiB RSS / 294 MiB total alloc**; a 303 MiB one peaks at **931 MiB / 1406 MiB**.
- **Why it matters:** this runs inside `finalize`, i.e. inside whatever long-lived process owns the run —
  webster's Master calls `AuditForksIncremental` once per batch, for the whole life of a plan. A several-hundred-MiB
  spike per audit in a process that is also holding plan/state is a real availability risk, and it buys nothing:
  the whole file is materialised so it can be walked once, forward.
- **Suggested fix:** stream. Read with `bufio.Reader.ReadString('\n')` and hand each decoded line to a callback,
  dropping peak to O(longest line). Note `bufio.Scanner` is the WRONG tool here — its 64 KiB default token cap
  would turn a single long transcript line (a large tool result) from "handled" into "error", a regression;
  `ReadString` has no such cap. Behaviour must stay byte-identical otherwise: skip blank lines, skip
  unmarshal failures, and treat a final line with no trailing newline as a line (`ReadString` returns it with
  `io.EOF`, which must not be dropped).

---

### R2-F6 — `sendVerified` can double-deliver a `Send` whose earlier copy scrolled out of the pane viewport

- **Where:** `internal/shuttleengine/run.go:440-464`.
- **Severity:** LOW. **Status:** PLAUSIBLE (traced, not reproduced live — see honesty note). **Size:** small.
- **Scenario:** `sendVerified` captures a `baseline` count of the needle in the pane BEFORE sending, then
  declares success only when a later capture counts **strictly more** than that baseline. `reed.CapturePane` returns
  the pane's VISIBLE viewport, which is a fixed-height window over a scrolling TUI. So: send the same text twice
  in one session (a retry, an operator re-issuing the same nudge, `loom`'s repeated one-line pointers). The first
  copy is on screen, so `baseline` = 1. The send lands correctly — but by the time the poll runs, the agent has
  emitted enough output that the FIRST copy has scrolled off, so the count is 1, not 2. `1 > 1` is false, so all
  20 poll attempts fail, and the `try` loop **replays the whole `ComposeSend` choreography**, typing the text into
  the pane a second time. The agent receives the instruction twice.
- **Why the direction matters:** the verification is built to fail safe against a SWALLOWED send, and does; but
  its recovery action is a re-send, so a false negative is not a harmless retry — it is a duplicate turn.
- **Honesty note:** I did not reproduce this live. Doing so needs a pane whose viewport churns past a specific
  line between two captures within one 5-second window, which is timing I cannot force deterministically without
  a fake — hence PLAUSIBLE, not CONFIRMED. The code path is unambiguous on reading.
- **Suggested fix:** make the check monotone rather than differential — re-read the baseline on each poll and
  succeed when the needle is present at all if it was absent at baseline, or, when it was already present,
  accept a count `>=` baseline after the choreography has been played and the pane has settled. The minimal,
  lowest-risk form: treat a count that has DROPPED below the baseline as evidence of scroll rather than of
  non-delivery, and re-baseline instead of replaying.

---

### R2-F7 — `docs/overview.md`'s shuttle bullet says the `Agent` deny is unconditional; it is neither unconditional nor undocumentedly narrowed

- **Where:** `docs/overview.md:282`.
- **Severity:** NIT. **Status:** CONFIRMED (contradicted by `claudeengine/settings.go:90-108`). **Size:** small.
- **Text today:** "`PreToolUse` guardrails deny the in-process `Agent` tool **always**, and `AskUserQuestion`
  too when the run is autonomous".
- **Two ways it is false:** (1) the deny is emitted only when `cfg.ClaudeDenyAgentTool` is true — a documented,
  operator-settable `shuttle.yaml` key that the same bullet's own sibling sentence acknowledges exists for other
  knobs; (2) in a `ForkSubagents` run the deny is deliberately NARROWED — `settings.go` emits a grep-guarded hook
  that ALLOWS `"subagent_type":"fork"` through and denies only every other subagent type. "Always" reads as
  "there is no configuration in which an Agent call succeeds", which is exactly the thing fork mode exists to
  make untrue.
- **Suggested fix:** replace "always" with the two real conditions (on by default via `claude_deny_agent_tool`;
  narrowed to non-fork subagent types in a fork-authorized run), in one clause.

## Named residual — `Wait` cannot tell "reed is gone" from "reed refuses"

**I independently CONFIRM round 1's assessment: this is not closable from shuttle's side today.** But round 1's
stated reason ("reed exposes no sentinel") is not quite the whole reason, and the refinement matters for whoever
picks up the eventual reed change, so I am recording it rather than silently agreeing.

What I found by reading reed's current code rather than assuming:

- Both cases funnel through **one** function, `reedengine.requireSessionLocked` (`lifecycle.go:1112`), which
  every non-booting verb — `Status` included, i.e. exactly the call `Wait` makes — bottoms out in.
- The two outcomes are already **textually** distinct: `noSessionMessage` for "gone"
  (`no reed session; run "lyx reed up"`, or the N-strands-persisted variant), and
  `refuseLiveForeignSessionLocked` for "refuses" ("recorded against tmux session %q, which is STILL RUNNING…").
  So the information exists — it is simply only in prose. Matching on that prose from shuttle would couple
  shuttle to reed's message wording, which is strictly worse than the current honest ambiguity.
- **There IS a structural route, and it is the one I want on record as REJECTED rather than unnoticed.**
  `reedengine.LoadState` is exported and shuttle already calls it (`run.go:237`, the orphan sweep);
  `State.PaneGeneration.SessionName` is an exported field; and `reedengine.SessionName(worktreeRoot)` is an
  exported function shuttle could call with the `worktreeRoot` it is already told. Comparing those two would let
  shuttle distinguish the two cases with **no reed change at all**.
- I am not proposing it, and I do not think round 1 should have. It would make shuttle reconstruct reed's tmux
  session identity and re-derive reed's own foreign-session diagnosis — and shuttle's package doc draws that
  line explicitly: "nothing here names a tmux command … panes are reed's vocabulary, reached only through
  ReedOps." The orphan sweep's existing `LoadState` call is not a precedent for it: that reads
  `Strands[].GUID`, which is *strand* identity (shuttle's own vocabulary, the same guid `AddStrand` handed
  back), never *session/tmux* identity. Adding the second would convert a narrow, defensible reach into a
  general licence for shuttle to interpret reed's state file.
- So the correct fix stays where round 1 put it: a typed sentinel on reed's side
  (`errors.Is(err, reedengine.ErrForeignSession)` vs `ErrNoSession`), which is a reed change and out of this
  campaign. Recorded here as an out-of-campaign observation, not a shuttle finding.

Practical consequence for a merge decision: **none**. The current behaviour — return the error plus the run's
identity, never guess `OutcomeDied` — is the safe direction, and R1-F2 already made sure the caller keeps the
handles it needs.

## Re-verification of round 1's fixes under my own scenarios

| fix | how I re-checked it (not round 1's reproduction) | verdict |
| --- | --- | --- |
| R1-F1 model pin | live L1 with `--model haiku`; trace log's `argv=` line records the pin reaching the real process | holds (but see R2-F3 for the missing guard) |
| R1-F2 identity on failure | live L3 (reed torn down under an in-flight run) — see below | holds |
| R1-F3 told-path validation | live production wiring via L1; hermetic matrix of every `hubgeom.ReedGeometry` shape; plus the litter evidence above proving the pre-fix write path is gone | holds |
| R1-F5 unreadable `run.json` | live L4 (truncated `run.json` on a live run) — see below | holds |
| R1-F6 untracked-strand message | live `interrupt` against a guid whose strand reed no longer tracks — see below | holds |
| R1-F7 resume flag threading | see below | holds (hermetically); live repro assessed as impractical |
| R1-F10 teardown logging | live L1 trace log carries BOTH `run started` and `run finished` for my guid | holds |

## Scope assessment

(pending)

## Convergence assessment

(pending)
