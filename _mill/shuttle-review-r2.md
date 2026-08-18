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

### Live scenario L3 — races and mechanism failures against ONE live run (1 real `claude`, `--model haiku`)

One 110-second run (`sleep 110` then write the output file), guid `c412d49cc5ce5f4706529ecc904bc802`, probed four ways:

| probe | observed |
| --- | --- |
| **P1 — genuine race:** `shuttle interrupt <guid>` and `shuttle send <guid> "…"` launched as two SEPARATE background processes, no ordering | both returned `{"ok":true}`; no error, no crash, no dropped keys. reed's `withOpLock` serialises each individual `send-keys`, so the two choreographies interleave at step granularity but never corrupt each other. Ordering between them is nondeterministic by construction — see the residual note below |
| **P2 — truncate `run.json` to zero bytes under the live run, then `interrupt`** | `…no run found for strand "c412…", but 1 run directory under …/.lyx/shuttle could not be read — if this guid names a run that is still live, its run.json is damaged rather than absent, and its agent is still in its pane…` → **R1-F5 holds** |
| **P2b — same but with non-empty INVALID json (`{ not json`)** | identical message → R1-F5 covers both damage shapes, not just truncation |
| **restore `run.json`, `interrupt` again** | `{"ok":true,"action":"interrupt"}` — the scan recovers cleanly |
| **P3 — `lyx reed down` under the in-flight run** | the blocked `run` returned `{"error":"shuttle: reed status failed 2 times consecutively: reed status: no reed session; run \"lyx reed up\"","guid":"c412d49c…","runDir":"…/be139a36…","sessionId":"caaf377a-…"}` → **R1-F2 holds**: all three identity fields present, `Outcome` correctly absent, run dir left on disk |
| **P4 — `reed up` again, then `interrupt` the now-untracked guid** | `shuttle: strand "c412…" is not tracked by reed — either its run completed and was cleaned up, or reed's strand table was reset under it (a reed remove/down, or a lost or rebuilt reed.json); check "lyx reed status"` → **R1-F6 holds** |

`ps -eo comm | grep -cx 'tmux: server'` = 0 after teardown.

### Live scenario L6 — resume wiring, prepared artifacts, orphan sweep (1 real `claude`, `--model haiku --effort low`)

**R1-F7 verified against the PRODUCTION wiring with no second `claude` process.** reed persists verbatim the
`ResumeCmd` string shuttle handed `AddStrand`, so reading `.lyx/reed.json` during a live run proves the
threading end-to-end without triggering an actual resume:

```
cmd      : 'claude' "$(cat '…/prompt.md')" --session-id '9b7e1057-…' --settings '…/settings.json' --model 'haiku' --effort 'low' --dangerously-skip-permissions
resumeCmd: 'claude' --resume '9b7e1057-…'                            --settings '…/settings.json' --model 'haiku' --effort 'low' --dangerously-skip-permissions
```

Model, effort and permission mode all ride the resume line. I judge a full live resume repro unnecessary given
this: the only thing it would add is proof that reed replays the string, which is reed's contract, not shuttle's.

Prepared artifacts inspected on disk: `prompt.md`, `settings.json`, `run.json` (0600), `run.json.lock`.
`settings.json` carried exactly the expected autonomous shape — the `Stop` append hook, the `Agent` deny, and the
`AskUserQuestion` deny — with the events path single-quoted.

**Outcome was `asking`**, and this is where a doc claim broke (see R2-F10):

```
{"outcome":"asking","lastAssistantMessage":"Waiting for the 60-second sleep to complete...","guid":"d9e3079a…", …}
```

The last message is a status update, not a question. The strand stayed live and the run dir was kept, both
correct for a non-`done` outcome.

### Orphan sweep, driven at its real boundary (no `claude` spawned)

An invalid `--effort` reaches `Start`'s sweep and then fails at `Prepare`, which gave me a free sweep trigger.
With `startup_timeout_s: 90` ⇒ `minAge` = 180 s:

| dir | age at sweep | strand tracked? | result |
| --- | --- | --- | --- |
| `be139a36…` (left behind by L3's mechanism failure) | 236 s | no | **swept** |
| `ff14566e…` (L6's kept `asking` dir) | 53 s | no | **kept** (age guard) |

Also confirmed: the run dir this failing `Start` created for itself was removed on the `Prepare` error, and the
sweep proceeded correctly with an ABSENT `reed.json` (see the assessed-not-a-finding note below).
An earlier attempt at ~180 s exactly did NOT sweep — the boundary behaves as `age >= minAge`, as documented, with
no off-by-one.

### `guid` reuse across a crash/restart — checked, and it cannot happen

The prompt asks whether `sweepOrphans` can keep or sweep the wrong dir because reed reused a guid across a
restart. It cannot: `reedengine.newGUID()` has exactly ONE call site in the whole package,
`strand.go:160` inside `AddStrand`. `lyx reed resume` rebuilds panes for the EXISTING strand records and mints
no new guids, so a resumed strand keeps the guid the run dir's `run.json` recorded, and the sweep's membership
test stays correct across a down/resume cycle. No finding.

### The `Wait` / reed-failure boundary — round 1's named residual, independently assessed

See the "Named residual" section below. Short version: **I confirm round 1's assessment**, with a refinement.

## Findings

Eleven findings: 2 MEDIUM, 7 LOW, 2 NIT. **No BLOCKING.**
Ten are small and are fixed inline in Job 2. **One — R2-F11 — is LARGE and is named NOT-FIXED-THIS-ROUND**;
it was discovered during Job 2 while fixing R2-F6, and is recorded below with the same detail as the rest.

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

---

### R2-F8 — five live-substrate log sites are still on the bare `log` package and never reach the durable trace sink

- **Where:** `internal/shuttleengine/run.go:196, 239, 253` and `internal/shuttleengine/wait.go:240, 249`.
- **Severity:** LOW. **Status:** CONFIRMED — reproduced live, with zero `claude` spawned. **Size:** small.
- **What the invariant requires:** CONSTRAINTS.md's *Live-Substrate Spawn Observability* — "Any code path that
  starts a real OS process on behalf of a round/strand/session … logs the spawn and **its teardown** via
  `internal/logger` — `logger.Info` for normal spawn/teardown events, **`logger.Warn` for a retry or a teardown
  that did not confirm clean**. The durable Info+ trace-file sink captures these regardless of verbosity."
- **The five sites, each mapped onto the invariant's own words:**
  1. `run.go:196` — `RemoveStrand` failed while tearing down a strand whose **pane and `claude` process are
     already launching** (the save-run-state cleanup path). This is literally "a teardown that did not confirm
     clean", and it is the exact twin of `finalize`'s `cleanup: remove strand failed`, which R1-F10 already put
     on `logger.Warn`.
  2. `wait.go:240` — `CapturePane` failed during the startup probe, logged as "non-fatal, **retrying**". The
     invariant names a retry explicitly.
  3. `wait.go:249` — dismissing the trust prompt failed: a key play into a live pane during the spawn window
     that did not confirm it landed.
  4. `run.go:239` / 5. `run.go:253` — the orphan sweep could not load reed state / could not complete. This is
     the deferred teardown of previous runs' live-substrate residue, and its sibling in `finalize`
     (`cleanup: remove run dir failed`) is already on `logger.Warn`.
- **Proof it actually loses the record.** I corrupted `.lyx/reed.json` and ran `lyx shuttle run` — `Start`
  reaches the sweep, hits the `LoadState` failure, and then fails at `AddStrand` **before any `claude` is
  spawned**, so this cost nothing. The sweep message appeared on stderr in the stdlib `log` format:
  ```
  2026/08/18 15:28:16 shuttle: orphan sweep: load reed state failed, skipping this sweep (non-fatal, new run proceeds): reed state file …/.lyx/reed.json is unreadable: …
  ```
  and the trace file for that same invocation (`trace=ee5ebc6d2df5e6cc`) contains **only**:
  ```
  command=./.dev-bin/lyx argv=shuttle run … trace=ee5ebc6d2df5e6cc pid=3224619 …
  INFO msg="configengine: degrading to embedded template" trace=ee5ebc6d2df5e6cc module=shuttle …
  INFO msg="configengine: degrading to embedded template" trace=ee5ebc6d2df5e6cc module=reed …
  ```
  Nothing hijacks the stdlib logger anywhere in the repo (`grep` for `log.SetOutput`/`log.Default` finds no
  production site), so the message is gone from the durable record, and it carries no `trace=` correlation id
  either — it cannot even be joined back to its invocation after the fact.
- **Why it matters:** the durable trace file is the only post-hoc record of what a live-substrate run did. The
  five surviving sites are precisely the *unhappy* paths — a strand that would not tear down, a pane that would
  not capture, a sweep that skipped — i.e. the ones an operator goes looking for, and the only ones that are
  invisible. R1-F10 fixed this class in `finalize` and left its five siblings in the same two files behind.
- **Suggested fix:** move all five onto `internal/logger` (`logger.Warn`, matching `finalize`'s already-migrated
  siblings and the invariant's wording), with structured key/value fields rather than `%v` interpolation. Both
  files already import `internal/logger`; the `"log"` import drops out of both.

---

### R2-F9 — the startup deadline is enforced on only ONE of the four paths through the not-yet-started window

- **Where:** `internal/shuttleengine/wait.go:214-257` (`checkLivenessTick`).
- **Severity:** MEDIUM. **Status:** CONFIRMED by tracing (not reproduced live — forcing a stuck trust prompt
  against a real, already-trusted `claude` is not something I can do without a fake). **Size:** small.
- **The shape.** `Wait` computes `startupDeadline = now + StartupTimeoutS` and hands it to every
  `checkLivenessTick`. Inside, once the strand is confirmed live and `*started` is still false, there are four
  ways out, and only ONE consults the deadline:

  | path | consults `startupDeadline`? |
  | --- | --- |
  | `CapturePane` errors → `log.Printf` + `return "", nil` | **no** |
  | `Startup(capture) == StartupReady` → `*started = true` | n/a (started) |
  | `Startup(capture) == StartupTrustPrompt` → play `TrustDismissSequence` | **no** |
  | `Startup(capture) == StartupPending` | yes → `OutcomeDied` |

- **Scenario A (trust prompt that will not dismiss).** `claudeengine.Startup` classifies
  `StartupTrustPrompt` on the whitespace-stripped needles `trustthisfolder` / `filesinthisfolder`. Any pane
  content matching those keeps returning `StartupTrustPrompt` forever — a trust dialog whose Enter dismissal
  does not take (a variant layout, a second confirmation step, a provider UI change), or a false positive where
  the launched process printed matching text. Each liveness tick replays `Enter` and returns `""`. The startup
  deadline is never consulted, so instead of dying at `startup_timeout_s` (90 s), the run burns all the way to
  `run_timeout_min` (**30 minutes** by default) before `Wait`'s own deadline check classifies `OutcomeTimeout`.
- **Scenario B (pane that cannot be captured).** `CapturePane` failing every tick — a wedged tmux server, a
  pane id that no longer resolves — takes the first row: log, return `""`, no deadline check. Same 30-minute
  burn. Note this is *different* from the strand going not-live, which is caught earlier and correctly
  classified.
- **Why it matters, concretely:** the config key's own shipped documentation promises otherwise —
  `template.yaml`: `startup_timeout_s: 90  # seconds allowed for the claude process to start and reach a live
  pane; 0 is NOT "unlimited" or "disabled" — it fast-fails startup as died immediately`. The whole point of a
  startup window is to fail a launch that never came up in seconds rather than minutes. Instead, two of the
  three failure paths through that window get 20× the budget, and the outcome is misclassified as `timeout`
  (which the docs define as "wall-clock Timeout elapsed before output files written", i.e. the agent was
  working) rather than `died` (which is what actually happened — it never started). A caller escalating on
  `died` vs retrying on `timeout` makes the wrong decision, on top of the stalled orchestrator and the 30
  minutes of a real, paid `claude` pane sitting on a dialog.
- **Suggested fix:** lift the deadline check out of the `StartupPending` arm to cover the whole not-yet-started
  window — check it once whenever `!*started` and the tick did not reach `StartupReady`, including on the
  capture-error path. The `StartupPending` behaviour is unchanged; the two silent paths gain the guard they
  were always meant to have.

---

### R2-F10 — `asking` is documented in three places as "the agent asked a question"; shuttle never inspects the message, and live it was a status update

- **Where:** `internal/shuttleengine/engine.go:16-17` (`OutcomeAsking`'s doc), `docs/overview.md:282`, and
  `internal/shuttlecli/run.go`'s `run` `Long` help text.
- **Severity:** LOW. **Status:** CONFIRMED live (L6). **Size:** small.
- **What the three docs say:**
  - `engine.go`: "OutcomeAsking: agent ended turn without writing output files, **last message reads as a question**."
  - `docs/overview.md`: "`asking` means the agent ended its turn **with a question** instead."
  - `run --help`: "`asking` means the agent ended its turn **with a question** instead."
- **What the code does.** `pollEventsTick` classifies `OutcomeAsking` for the last parsed event whenever
  `allOutputFilesExist` is false — with **no inspection of `Message` whatsoever**. `wait.go`'s own comment is
  explicit that this is deliberate: "an `EventStop` with no output files and an `EventAsk` with no output files
  both classify `OutcomeAsking` identically — Kind only selects Message's source". So `asking` actually means
  "ended a turn without satisfying the file contract", for any reason at all.
- **Live evidence (L6).** The agent ended its turn mid-task and shuttle returned:
  ```
  {"outcome":"asking","lastAssistantMessage":"Waiting for the 60-second sleep to complete...","guid":"d9e3079a…"}
  ```
  That is a progress report, not a question. Nothing was asked.
- **Why it matters:** `asking` is the module's escalation channel, and `LastAssistantMessage` is what a
  downstream consumer (burler, perch, an operator) reads to decide what to do. Documenting it as "a question"
  tells that consumer to expect an interrogative and to treat the field as one, when in practice it receives
  whatever the agent happened to say when it stopped early. The behaviour is right; three docs describe a
  narrower behaviour than the one shipped, and the widest-read of them (`--help`, `docs/overview.md`) are the
  least hedged.
- **Suggested fix:** state the real rule in all three — the agent ended its turn without satisfying the file
  contract, *typically* because it is asking or is blocked, with `LastAssistantMessage` carrying whatever it
  last said. This is a wording fix only; no behaviour changes.

---

### R2-F11 — **NOT-FIXED-THIS-ROUND** — a count over a scrolling viewport cannot soundly verify pane delivery

- **Where:** `internal/shuttleengine/run.go`'s `sendVerified`.
- **Severity:** LOW. **Status:** PLAUSIBLE (traced; not reproducible live for the reason in R2-F6).
- **SIZE: LARGE — named, not fixed.** Reason: closing it means REPLACING the delivery signal, not
  adjusting it, and the signal it replaces is a live-proven guarantee (`sendVerified`'s
  swallowed-send detection was earned against a real Claude TUI, and its "the provider TUI swallowed
  the input" path is the reason `Send` does not report a silent no-op as success). Getting a
  replacement wrong regresses that in the direction that fails silently. It needs its own design step
  and live validation against a real TUI, which is a task rather than an inline crucible fix.
- **Discovered during Job 2**, while writing R2-F6's fix and its test: recorded here rather than
  quietly shipping a fix that reads as if it closed the whole class.
- **What R2-F6's fix DOES close:** a baseline inflated by occurrences that later scroll away. The
  count falling BELOW the baseline is now read as scrolling and re-baselines, so the poll can succeed
  and no replay is issued. Sabotage-proved.
- **What it does NOT close:** the case where one occurrence scrolls off the top *as* the delivered
  one arrives at the bottom. The count is then unchanged — neither `>` nor `<` the baseline — so
  every poll fails and the choreography is replayed into a pane that already received it. This is
  reachable whenever the viewport is full and the earlier copy sits at its top, which is ordinary in
  a busy pane.
- **Why no small fix exists.** The information the check needs is not in a count: `CapturePane`
  returns the visible viewport with no scrollback and no cursor/position context, so "one left as one
  arrived" and "nothing arrived" are genuinely indistinguishable from two counts alone. A sound
  signal has to use something else — the needle's POSITION relative to the end of the capture (the
  delivered copy is always newest, hence lowest), a reed-side scrollback capture, or a
  provider-specific echo marker. Each of those is a real design choice with its own failure modes.
- **Suggested direction for the task:** prefer a position-aware check (needle present at or below the
  region that was the capture's tail at baseline time) over a count, and validate it against a real
  Claude TUI under a genuinely churning pane before trusting it; keep the swallowed-send failure path
  exactly as it is.

## Assessed and deliberately NOT recorded as findings

Named explicitly so a later round can see they were examined rather than missed.

- **The orphan sweep runs with an EMPTY live-guid set when `reed.json` is ABSENT**, while an *unreadable*
  `reed.json` skips the sweep entirely (`run.go:236-241`). The asymmetry is real, and reed's own corrupt-state
  error text actively recommends the dangerous action ("delete `…/.lyx/reed.json` by hand to keep the session
  (its panes and their processes keep running, untracked)") — after which a later `Start` sweeps every aged run
  dir belonging to those still-live agents, including `--keep-pane` diagnosis dirs. **Not a finding**, because
  absence is overwhelmingly the *normal* meaning "nothing is live": `lyx reed down` removes the file (verified —
  `.lyx/reed.json` was gone after every `down` I ran), so blocking the sweep on absence would break the ordinary
  post-`down` cleanup path in exchange for guarding an escape hatch reed already warns loses tracking. The age
  guard is the correct and sufficient protection here.
- **`Send`/`Interrupt` ordering between two processes is nondeterministic** (P1). reed's `withOpLock` makes each
  individual `send-keys` atomic but nothing sequences one verb's whole choreography against another's. This is
  inherent to a shared-pane transport with no shuttle-side run lock, both verbs returned success, and no
  corruption was observed. A run-scoped lock would be a design change, not a bugfix.
- **`buildSettings`' `Stop` hook is `cat >> events.jsonl`**, so two hook processes firing at once could
  interleave partial lines. `readEventsFrom` only ever consumes up to the last complete `\n` and `ParseEvents`
  skips unparseable lines, so the failure mode is a skipped event, not corruption — and Claude fires `Stop` once
  per turn end. Not worth a change.
- **`readEventsFrom` never recovers if `run.offset` exceeds the file size** (events file truncated or replaced
  under a live run): `Seek` past EOF succeeds, `ReadAll` returns empty, and the run silently stops classifying
  from events until the liveness or timeout path catches it. I could not construct a realistic trigger — the
  only writer is the `Stop` hook, which appends, and a removed run dir makes the hook fail rather than restart
  the file at offset 0. Recorded as a watch item, not a finding.

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

## Scope assessment — plan-promised vs shipped

I found **nothing deferred that should be v1, and nothing shipped beyond scope.** Round 1's scope verdict holds
under a second, independent lens.

- **Shipped and promised:** the four outcomes, the file contract, the `Stop`-hook events channel, the live
  `AskUserQuestion` marker hook, the `PreToolUse` guardrails, the engine seam with Claude as the only v1
  implementation, `run`/`interrupt`/`send`, the run-dir lifecycle with an age-guarded orphan sweep, per-run
  `Model`/`Effort`/`Version`, and the fork audit. All present, all exercised above.
- **`Spec.Version`, `Spec.ForkSubagents` and `Spec.KeepPane`'s programmatic-only status.** `Version` and
  `ForkSubagents` have no CLI flag; `KeepPane` does. `docs/overview.md:282` documents the `Version` decision
  explicitly ("no CLI flag — consumers drive it via the model-spec notation's `version=` param"), and
  `ForkSubagents` is a burler/webster-driven capability, not an operator verb. Deliberate, documented, correct
  — **not** a gap.
- **`Engine.ModelSwitchSequence` and `Engine.AuditForksIncremental` have no caller inside shuttle.** Both are
  consumed by `internal/websterengine` (`beginbatch.go:243`, `recordbatch.go:102`). Seam surface carried for a
  downstream consumer, not dead code.
- **`posix.go`'s Windows gap** is named in this round's out-of-scope list and was read, not driven; it is
  correct on inspection (drive-rooted only, UNC and relative rejected).
- The **Shuttle Provider-Seam Invariant** holds: no Claude specific appears outside `claudeengine`
  (`seam_enforcement_test.go` covers the import half; I read the rest and found no leakage — the closest call
  is `claudeengine`'s use of `shuttleengine.PosixPath`, which is provider-invariant by construction).
- The **Durable-vs-Ephemeral State Invariant** holds: every run artifact lives under `.lyx/shuttle/`, verified
  on disk in L6.
- The **Live-Substrate Spawn Observability** invariant is where round 1's R1-F10 left work behind — see R2-F8.
  That is the one invariant this module does not currently fully honour.

## Convergence assessment

**shuttle is converging, but it has not converged.** Ten findings is more than the "may plausibly find little"
this round was primed for, and I want to be honest about what that does and does not mean.

- **Nothing I found is a defect class.** There is no analogue of reed's tmux-identity or state-loss families —
  no BLOCKING, no data-loss path, no correctness bug in the normal single-instance flow. Every live scenario I
  drove reached the right outcome, and every one of round 1's seven re-checkable fixes held under scenarios
  round 1 did not run.
- **The findings cluster in three shallow bands**, which is itself the convergence signal:
  1. *Unhappy-path polish* (R2-F1 double envelope, R2-F2 dropped outcome, R2-F9 startup deadline) — three
     independent instances of the same underlying habit: the happy path is carefully reasoned and the
     failure path is one branch less carefully reasoned. R2-F9 is the one with real operational cost.
  2. *Observability and regression-guard debt* (R2-F8, R2-F3) — both are round-1 work that stopped one step
     short: R1-F10 migrated two log sites and left five, R1-F1 fixed four call sites and left them unguarded.
     Neither is a new defect; both are the tail of an existing fix.
  3. *Documentation drift* (R2-F4, R2-F7, R2-F10) — three places where the prose promises something narrower
     or different from what ships. All three were found by driving, not by reading, which is why round 1's
     method did not reach them.
- **What that predicts for round 3.** I expect the correctness surface to be close to exhausted: I spent this
  round specifically hunting the areas round 1 could not reach (real races, mechanism failures under a live
  agent, transcript scale, the startup window) and the only genuinely new *behavioural* finding was R2-F9. A
  third round is worth running for the fresh lens, but I would set expectations at "one or two small
  behavioural findings, plus doc drift" rather than another ten.
- **Two things I could NOT verify, stated plainly:**
  - **Subpath-anchored geometry, live.** This worktree is anchored at its own root (`AnchorRel` = `"."`), so
    `anchorPath == worktreeRoot` in every live scenario. `validateToldPaths`' swap detector, and the
    anchor-vs-worktree distinction R2-F4 is about, were exercised hermetically across every
    `hubgeom.ReedGeometry` shape but never against a real subpath-anchored hub with a live agent in it. A round
    that can build such a fixture should drive it.
  - **A live resume.** R1-F7 was verified through the persisted `resumeCmd` in `reed.json` (production wiring,
    exact string) rather than by triggering a real reed resume, which would have cost a second `claude`
    process for a fact that is reed's contract rather than shuttle's.
  - **R2-F6** is marked PLAUSIBLE, not CONFIRMED, for the reason given in the finding: I cannot force a
    viewport scroll at a specific instant without a fake.
- **Merge bar verdict:** the normal single-instance flow is correct. Every finding is small and in-module;
  none blocks a merge on its own, and all ten are fixed in Job 2.
