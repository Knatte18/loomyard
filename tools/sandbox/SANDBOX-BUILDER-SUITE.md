# SANDBOX-BUILDER-SUITE -- lyx builder black-box suite

## What this is

A structured test-loop for exercising `lyx builder` against a **live psmux server and a
logged-in claude** in the sandbox Hub host repo. Like `SANDBOX-SHUTTLE-SUITE.md` and
`SANDBOX-BURLER-SUITE.md`, the value here is partly **visual**: a batch's implementer doing
real work in a pane, a digest coming back, a pause landing cleanly at a batch boundary. Not
an automated suite -- an agent drives it, an operator watches.

`builder` drives a pinned plan-format v2 plan through implementer sessions, batch by batch,
until the plan is built (see `docs/modules/builder.md`). `lyx builder run` spawns its own
**long-lived LLM orchestrator session** that autonomously calls `spawn-batch`/`poll`/`pause`
itself; `spawn-batch`, `poll`, and `pause` are also directly invocable `lyx builder`
subcommands, which is how scenarios B2-B7 below isolate the Go-level state-machine mechanics
(timeout classification, lock contention, pause discipline, fingerprint archiving,
stuck->recovery report archiving, the in-flight guard and dead-respawn reclaim) without
depending on the orchestrator LLM's own cooperation with adversarial timing. Scenarios are
deliberately trivial (a plan whose cards just write a fixed-content file) so the assertions
are about the mechanics, not about whether an implementer's actual coding judgment is good --
that is `perch`'s and a normal code review's job, not this suite's.

## Pre-conditions

Before starting a session:

1. **Deploy a fresh binary.** Run `deploy.cmd` so `lyx.exe` on PATH is current source.
   The deployed binary is a snapshot -- re-deploy after any source change you want to test.
2. **Materialize the hub.** Run `sandbox-build.cmd` (or `sandbox-build.cmd -reset`
   to start clean); the session cwd is the Hub host repo root, the same operating model
   as the main suite.
3. **Live-psmux and claude requirement.** `psmux.exe` on PATH (installed at
   `C:\Code\tools\bin\psmux.exe`), PowerShell 7, and a logged-in `claude` on PATH.
   If any of these is unavailable in the session, **note that as the session outcome
   rather than treating it as a builder defect** -- the `**Covers:** builder` tag on
   B1 satisfies the sandbox coverage guard (`sandbox_coverage_test.go`) regardless of
   runtime availability.
4. **`lyx init` first.** `lyx builder` requires an initialized worktree
   (`_lyx/config/builder.yaml`, plus `shuttle.yaml`/`mux.yaml` since builder branches off
   shuttle directly) exactly like `lyx shuttle`/`lyx burler` do.
5. **Attached interactive terminal.** Launch `sandbox-builder-suite.cmd` from a real,
   attached console -- never redirected, backgrounded, or detached. Without a TTY the
   driving claude session cannot idle between turns waiting for notifications, so the
   process ends as soon as a turn ends and the remaining scenarios are silently abandoned.
   The launcher prints a warning when it detects non-console stdio.

## Black-box rule

**The agent under test works exclusively inside the Hub host repo (`lyx-test-HUB/lyx-test`).
It tests `lyx.exe` as a black box -- exactly as a real user with only the binary on PATH.
It must not look for, read, or reason about the lyx source tree. No peeking at
`C:\Code\loomyard\` or any other path outside the Hub.**

Discovering the command surface is done via `lyx builder --help` and `lyx builder
<subcommand> --help` alone -- not from documentation outside the Hub. The plan file(s) under
`_lyx/plan/` are the one artifact the agent must construct itself per each scenario's Goal
below; `docs/modules/plan-format.md`'s worked example (available via `lyx builder validate
--help` or by reasoning from validation error messages) is the reference for the file shape.
Keep every scenario's plan cards trivial -- e.g. "create `resultN.md` containing the single
line `OK`" -- so a real implementer session finishes in one card, one commit, fast.

### Controlled psmux exceptions

One sanctioned deviation from the pure black-box rule, mirroring the mux/shuttle/burler
suites' own controlled-exception note:

- **Direct `psmux -L <socket> list-panes`/`ls`** is allowed only to confirm a strand's pane
  exists (or was cleaned up), where `<socket>` is read from `lyx mux status` output.
- **A second terminal** is required for B4 (run-lock contention) -- start the first `lyx
  builder run`/`spawn-batch` in terminal A, the contending call in terminal B, while A is
  still in flight.

## Fingerprint header

The launcher prepends a "binary under test" fingerprint block to this file when it copies it
into the Hub host repo. The fingerprint records the absolute path, file size, modification
time, and a short SHA-256 of the `lyx.exe` binary at launch time.

The same fingerprint identifies the binary for the report's provenance: a separate fetch step
(run after this session) stamps it into `meta.fingerprint` of the fetched
`sandbox-report.json` so a maintainer can reproduce the exact binary that produced each
finding. The agent does not need to transcribe the fingerprint anywhere itself.

## How to run a scenario

For each scenario below:

- Read the **Goal** -- it names the task, not the commands. Discover the commands via
  `lyx builder`, `lyx builder <subcommand>`, and `--help` flags (S0 ethos).
- **Watch** what lyx does. Note where it stalls, guesses wrong, or hits an error.
- Record the outcome per the verdict buckets: `OK` (worked) / `WARN` (rough edge) /
  `FAIL` (broke).

## Verdict key

- `OK`   -- completed without friction
- `WARN` -- completed but with confusion, awkward UX, or a non-fatal error
- `FAIL` -- did not complete; lyx broke, panicked, or gave wrong output

## Capturing findings

After all scenarios are run, write **all** `WARN`/`FAIL` findings to `./sandbox-report.json`
(in the host-repo cwd) on this exact schema. **Always write the file, even when there are
zero `WARN`/`FAIL` findings** -- in that case `items` is an empty array.

```json
{
  "source": "sandbox-report",
  "items": [
    {
      "ref": "B2",
      "title": "…",
      "body": "verdict: WARN\n\n…repro…"
    }
  ]
}
```

- `source` is the literal string `"sandbox-report"`.
- `items[]` holds only `WARN`/`FAIL` findings -- do not record `OK` scenarios here.
- `ref` is the scenario id (`B1`-`B9`).
- `title` is a short one-line summary.
- `body` folds the detail, repro steps, and verdict into one markdown string.

Write only `source` and `items` -- a separate fetch step (run after the session) stamps
`meta` (including the binary fingerprint). Confine all free text to the `title`/`body`
string fields so the JSON stays well-formed.

## Scenarios

### B1 -- Autonomous happy path (full `run`, real orchestrator LLM)

**Covers:** builder

**Goal:** "Pin a tiny two-batch plan whose cards each just write one fixed-content file, run
`lyx builder run`, and confirm it drives itself end-to-end to a `done` outcome with both
batches' cards committed and both weft-commit boundaries honored."

**Watch:** `lyx builder run` blocks until the run reaches a terminal outcome; the printed
JSON envelope reports `"outcome":"done"` with `batches_done: 2`. Each batch spawned a real
implementer pane (visible via `lyx mux status` while running), and each batch's card(s)
landed as its own commit with the `NN.C: <what>` subject convention. `lyx builder status`
after completion shows both batches `done`/`tests: green`. Confirm the three weft-commit
points actually fired (not just the exit-time backstop): `state.json` was committed at each
`spawn-batch`, the batch report was committed at each terminal `poll` classification, per
`docs/modules/builder.md`'s "three weft-commit points". Afterward, both batches' panes/run
dirs are cleaned up (no leftover pane, `lyx mux status` no longer lists either guid).

**Verdict:** `OK` / `WARN` / `FAIL`

---

### B2 -- `poll`'s dead/timeout classification

**Covers:** builder

**Goal:** "Configure a very short `batch_timeout_min`, spawn a batch whose implementer never
satisfies the file contract (e.g. give it a card whose instructions it cannot complete, or
interrupt its pane before it reports), and confirm `poll` classifies it terminal `dead` with
`dead_reason: timeout` -- not `running` forever, and not misclassified as `asking`/`died`."

**Watch:** `lyx builder spawn-batch <NN>` returns immediately once the strand is registered.
Poll with `lyx builder poll --wait <duration>` (or repeated short polls) before the timeout
window: confirm `status: running` with a growing `elapsed_s`. After `batch_timeout_min`
minutes have elapsed with no report file and the mux strand still nominally present, confirm
the NEXT poll classifies `status: dead`, `dead_reason: timeout`. Separately (a second run of
this scenario, or a variant), end the implementer's pane/session directly (e.g. via `psmux`
against its socket) before it reports, and confirm poll instead (or additionally) exercises
the `died` branch. Confirm the pane/run dir is kept for diagnosis on any `dead` classification
(not auto-cleaned), per the doc's stated discipline.

**Verdict:** `OK` / `WARN` / `FAIL`

---

### B3 -- Pause is a boundary check, not an interrupt

**Covers:** builder

**Goal:** "Start a batch, then call `lyx builder pause` while that batch is still in flight;
confirm the in-flight batch is NOT killed and finishes normally, and that it is the NEXT
`spawn-batch` call that actually refuses with `{\"paused\": true}`. Then confirm `lyx builder
run` clears the pause flag at its own entry and resumes."

**Watch:** `lyx builder spawn-batch 01` (a plan with at least 2 batches), then immediately
`lyx builder pause` from a second terminal while batch 01's implementer pane is still
visibly active. Poll batch 01 to completion (`lyx builder poll --wait ...`): confirm it
reaches its own normal terminal state (`done`/`stuck`), never interrupted by the pause. Then
attempt `lyx builder spawn-batch 02`: confirm it refuses with `{"paused": true}` and spawns
nothing (no new pane, no `state.json` mutation for batch 02). Finally run `lyx builder run`:
confirm it clears the pause flag at entry and batch 02 proceeds normally to completion.

**Verdict:** `OK` / `WARN` / `FAIL`

---

### B4 -- `run.lock` contention

**Covers:** builder

**Goal:** "Start `lyx builder run` in one terminal against a plan with at least two batches;
while it is still holding the run-level lock, attempt a second `lyx builder run` against the
SAME worktree from a second terminal. Confirm the loser fails fast with a run-busy error and
that `state.json` is never corrupted or double-written."

**Watch:** Terminal A: `lyx builder run` (blocks). Terminal B, while A is still running:
`lyx builder run` again. Confirm B's command exits immediately with a clear run-busy error
and touches no state -- inspect `state.json`'s mtime/content immediately before and after
B's failed attempt to confirm it is byte-for-byte unchanged. Let A finish normally to a
terminal outcome; confirm A's own exit-time backstop weft-commit still fired (the doc's
claim is that the LOSER skips its own backstop commit, not the winner). Note: a manual
`spawn-batch` from terminal B is deliberately NOT run-lock-refused -- the orchestrator's own
`spawn-batch` calls run under the winner's lock, so a lock check there would deadlock every
normal run. A `spawn-batch` fired while A's current batch is mid-flight is instead refused
by the in-flight guard (B7); one fired between A's batches is structurally indistinguishable
from the orchestrator's own call and is not refused.

**Verdict:** `OK` / `WARN` / `FAIL`

---

### B5 -- Fingerprint mismatch, `--fresh`, and stale-outcome archiving

**Covers:** builder

**Goal:** "Start a run, let one batch complete, then edit a plan `*.md` file's content. Run
`lyx builder run` again (no `--fresh`) and confirm a hard refusal naming both fingerprints.
Then run with `--fresh` and confirm `state.json` and the reports dir are ARCHIVED (never
deleted) with a timestamp suffix, and the run re-inits cleanly."

**Watch:** After batch 01 completes, edit any plan `*.md` file's body text (a no-op content
change is enough to shift the fingerprint). `lyx builder run` (no `--fresh`): confirm it
refuses, and the error names the old and new fingerprints and points at `run --fresh`; confirm
`state.json` is untouched. `lyx builder run --fresh`: confirm `state.json` is renamed to
`state-<timestamp>.json` (present alongside the fresh one, not deleted) and the reports dir is
renamed to `<reports-dir>-<timestamp>` with a fresh empty reports dir created; the run then
proceeds from batch 01 again with a new `RunGUID`. If you can complete this scenario twice
within the same wall-clock second (scripted back-to-back), confirm the second archive gets a
numeric `-1` suffix instead of clobbering the first. Separately, confirm stale `outcome.yaml`
archiving: after a run reaches a terminal outcome, start a fresh `lyx builder run` (e.g. after
a `--fresh` reset) and confirm the prior `outcome.yaml` is renamed
`outcome-<UTC-compact-timestamp>.yaml` (not overwritten) before the new run's own outcome is
ever written.

**Verdict:** `OK` / `WARN` / `FAIL`

---

### B6 -- Stuck -> recovery ladder (single-batch, non-chain)

**Covers:** builder

**Goal:** "Pin a plan whose batch 01 is guaranteed to report `stuck` (e.g. a `verify:`
command that always fails, so the implementer exhausts `self_fix_cap` and reports
`status: stuck`). Drive it to the stuck classification, then confirm
`lyx builder spawn-batch 01 --role recovery` is NOT refused for the pre-existing stuck
report -- it must ARCHIVE that report and spawn a fresh recovery implementer."

**Watch:** `lyx builder spawn-batch 01` then `lyx builder poll --wait <duration>` until it
returns `status: "stuck"` with a `stuck_reason`; confirm `reports/01-<slug>.yaml` (status:
stuck) is on disk and was weft-committed. Now run `lyx builder spawn-batch 01 --role
recovery`: confirm it succeeds (does NOT return `batch report already exists`), that the
prior report was RENAMED to `01-<slug>-<UTC-compact-timestamp>.yaml` (archived, not deleted
-- the prior stuck judgment stays on disk), that the live `01-<slug>.yaml` path is free, and
that a real recovery-role implementer pane spawned (visible via `lyx mux status`, role
`recovery`). Let the recovery session run and confirm it writes its own fresh
`01-<slug>.yaml`, which the next `poll` distills normally. This is the exact stuck->recovery
escalation the orchestrator drives autonomously in B1; B6 isolates it at the Go verb level so
it is verifiable without depending on the orchestrator LLM choosing to escalate. (A stuck
CHAIN member instead restarts the whole chain via `--restart-chain`, covered by the chain
mechanics, not here.)

**Verdict:** `OK` / `WARN` / `FAIL`

### B7 -- In-flight guard and dead-respawn substrate reclaim

**Covers:** builder

**Goal:** "With a batch's implementer genuinely mid-flight, confirm `lyx builder spawn-batch`
of ANY batch is refused with a batch-already-in-flight error (never a silent double-spawn).
Then let a batch classify `dead` (timeout or died) with its pane kept alive, and confirm the
respawn of that SAME batch succeeds by re-claiming the kept substrate: the kept strand is
stopped and any late report the orphan wrote after its classification is ARCHIVED (never
deleted, never refused on)."

**Watch:** `lyx builder spawn-batch 01` (a slow batch -- e.g. a card instructing one long
blocking `sleep`), then immediately `lyx builder spawn-batch 02` from a second terminal:
confirm it refuses naming the in-flight batch and its strand, pointing at `lyx builder poll`,
and spawns nothing (`lyx mux status` still lists exactly one implementer strand). Poll batch
01 past a short `batch_timeout_min` to its `dead`/`timeout` classification (pane kept live,
per B2). If the orphan then finishes and writes its report late, confirm
`lyx builder spawn-batch 01` is still NOT refused: the late report is renamed with the
UTC-compact archive suffix and the kept strand is gone from `lyx mux status` before the
fresh implementer spawns. Also confirm the B1 happy path's cleanup half: after every
`done`/`stuck` classification the batch's pane is released (no leftover implementer strands
accumulate across a run), while every `dead` classification keeps its pane for diagnosis
until a respawn re-claims it.

**Verdict:** `OK` / `WARN` / `FAIL`

### B8 -- Chain restart from a non-lowest member restarts from the bottom

**Covers:** builder

**Goal:** "Pin a small deferred-verify chain (e.g. batch 01 `verify: deferred`
`chain-end: 02`, batch 02 the chain end that runs the real `verify:`). Drive batch 01 to
`done` so the chain-start SHA is recorded and batch 01's card commit lands, then invoke
`lyx builder spawn-batch 02 --restart-chain` -- naming the chain END, not the lowest
member. Confirm the reset rolls the host repo back to the recorded chain-start SHA AND
that the batch actually spawned is the chain's LOWEST member (01), never the named end
(02) on a tree missing batch 01's just-discarded work."

**Watch:** `lyx builder spawn-batch 01` then `lyx builder poll --wait <duration>` until it
returns `status: "done"` `tests: "skipped"` (a deferred-verify intermediate reports
skipped); confirm batch 01's card commit is on `HEAD` and `state.json`'s
`chainStartShas` records the pre-01 SHA. Now run `lyx builder spawn-batch 02
--restart-chain`: confirm the returned `batch_name` is the LOWEST member (`01-...`), not
`02-...`; confirm `git rev-parse HEAD` equals the recorded chain-start SHA (batch 01's
commit and its files rolled back); and confirm `state.json`'s `currentBatch` is the lowest
member and every chain member's stale `BatchState`/report was cleared. Spawning the named
end (02) directly on the rolled-back tree -- silently skipping batch 01 -- is a `FAIL`
(the round opus-r3 defect this scenario pins): the chain-recovery mechanism must re-run
from the bottom regardless of which member the caller names.

**Verdict:** `OK` / `WARN` / `FAIL`

### B9 -- Run-entry substrate reclaim (orphaned orchestrator, --fresh with a live implementer)

**Covers:** builder

**Goal:** "Prove `run` reclaims a superseded run's live agents instead of double-driving.
First: start `lyx builder run`, hard-kill the run PROCESS (not the panes) while a batch is
mid-flight, confirm the orchestrator pane is still live and still driving, then re-run
`lyx builder run` and confirm the OLD orchestrator strand is stopped before the fresh one
spawns -- never two live orchestrators at once. Second: with a batch's implementer genuinely
mid-flight, edit a plan `*.md` file and run `lyx builder run --fresh`; confirm the superseded
implementer's strand is stopped before state/reports are archived -- its late report must
never land in the fresh run's reports dir."

**Watch:** Part 1: `lyx builder run` (terminal A), then kill that process (e.g. `taskkill
/PID <pid> /F`) while `lyx mux status` shows the orchestrator and an implementer live.
Confirm the orchestrator pane survives the kill (it is a detached psmux pane) and keeps
calling builder verbs. Re-run `lyx builder run`: confirm `lyx mux status` never shows two
live `orchestrator:` strands -- the recorded one is stopped at run entry (state.json's
`orchestratorStrand`), then the fresh one spawns, and the resumed run completes normally.
Part 2: spawn a slow batch (a card with one long blocking wait), edit any plan file while
it is in flight, then `lyx builder run --fresh`: confirm the old implementer strand is gone
from `lyx mux status` before the fresh orchestrator's first spawn-batch, that `state.json`/
reports were archived with the timestamp suffix as in B5, and that the fresh run's batch is
implemented by its OWN implementer (the fresh run's outcome reflects the EDITED plan's
content, not the superseded card's). A second live orchestrator, a superseded implementer
surviving the archive, or a fresh `done` whose file content matches the superseded plan is
a `FAIL` (the round fable-r4 defects this scenario pins).

**Verdict:** `OK` / `WARN` / `FAIL`

## Session log format

After running all scenarios, record a short session summary:

```
Date: <YYYY-MM-DD>
Binary fingerprint: <copy from the header above>

B1: <OK|WARN|FAIL> -- <one-line note if not OK>
B2: <OK|WARN|FAIL> -- <one-line note if not OK>
B3: <OK|WARN|FAIL> -- <one-line note if not OK>
B4: <OK|WARN|FAIL> -- <one-line note if not OK>
B5: <OK|WARN|FAIL> -- <one-line note if not OK>
B6: <OK|WARN|FAIL> -- <one-line note if not OK>
B7: <OK|WARN|FAIL> -- <one-line note if not OK>
B8: <OK|WARN|FAIL> -- <one-line note if not OK>
B9: <OK|WARN|FAIL> -- <one-line note if not OK>

sandbox-report.json written: <count of WARN/FAIL items>
```

`./sandbox-report.json` must be written before the session ends, per the Capturing findings
section above -- with `items: []` when every scenario was `OK`.

## Teardown

After the session summary is recorded and `./sandbox-report.json` is written, run `lyx mux
down` to tear down the psmux session/server the scenarios booted. An orphaned psmux server
holds open handles inside the Hub host repo and blocks the next `sandbox-build.cmd -reset`.
The launcher also runs `lyx mux down` itself after the session ends (deterministic backstop),
but run it here anyway -- defense-in-depth, and it keeps the Hub clean while the session is
still open for inspection.

## Notes

- Host/weft scenarios stay in `SANDBOX-CORE-SUITE.md`, mux/psmux scenarios stay in
  `SANDBOX-MUX-SUITE.md`, shuttle black-box agent scenarios stay in
  `SANDBOX-SHUTTLE-SUITE.md`, burler's own review+fix round scenarios stay in
  `SANDBOX-BURLER-SUITE.md`, perch's gate-loop scenarios stay in `SANDBOX-PERCH-SUITE.md`;
  this suite holds only builder's batch-loop scenarios -- add `B` scenarios here, not in any
  other suite.
- This suite is a FLOOR, not a ceiling. It proves the mechanics (state machine, locking,
  timing classification, archiving discipline), never implementer quality or plan-format
  content richness -- those are a normal code review's and `perch`'s job respectively.
