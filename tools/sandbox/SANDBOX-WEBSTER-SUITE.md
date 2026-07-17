# SANDBOX-WEBSTER-SUITE -- lyx webster black-box suite

## What this is

A structured test-loop for exercising `lyx webster` against a **live tmux server and a
logged-in claude** in the sandbox Hub host repo, mirroring `SANDBOX-BUILDER-SUITE.md`'s own
operating model. `webster` is builder's fork-based sibling: instead of spawning a fresh
mux/tmux strand per batch, one long-lived **Master** session reads the codebase and the
whole plan once, then forks one implementer per batch in-session (Claude Code's Agent tool,
`subagent_type: "fork"`) -- no `spawn-batch`/`poll` verbs exist here; Master itself brackets
each fork with `begin-batch`/`await-batch`/`record-batch` calls (forks are BACKGROUNDED
agents on current Claude Code -- the Agent call returns immediately, so Master long-polls
`await-batch` for the batch report instead of relying on a synchronous fork return, and
never ends its turn while a batch is open). This suite is deliberately narrow: two
scenarios, not builder's nine, because webster's Go-level mechanics (fingerprinting,
`--fresh` archiving, chain rollback, pause) are the SAME imported `builderengine` code
builder's own suite already exercises (per discussion.md's `reuse-by-import` decision) --
what is genuinely new here is the fork loop itself (W1) and the one load-bearing,
previously-unvalidated mechanism unique to webster: Go-driven `/model` pane injection for
oversized batches (W2).

## Pre-conditions

Before starting a session:

1. **Deploy a fresh binary.** Run `deploy.cmd` so `lyx.exe` on PATH is current source.
   The deployed binary is a snapshot -- re-deploy after any source change you want to test.
2. **Materialize the hub.** Run `sandbox-build.cmd` (or `sandbox-build.cmd -reset`
   to start clean); the session cwd is the Hub host repo root, the same operating model
   as the main suite.
3. **Live-tmux and claude requirement.** tmux (or the Windows tmux port) on PATH, PowerShell 7, and a logged-in `claude` on PATH.
   If any of these is unavailable in the session, **note that as the session outcome
   rather than treating it as a webster defect** -- the `**Covers:** webster` tag on
   W1 satisfies the sandbox coverage guard (`sandbox_coverage_test.go`) regardless of
   runtime availability.
4. **`lyx init` first.** `lyx webster` requires an initialized worktree
   (`_lyx/config/webster.yaml`, plus `shuttle.yaml`/`mux.yaml` since webster branches off
   shuttle directly) exactly like `lyx shuttle`/`lyx burler`/`lyx builder` do.
5. **`lyx mux up` before any spawn.** `webster run` spawns the Master session through
   shuttle into an existing mux session and does not boot one itself; without it the
   spawn fails loud with `no mux session; run "lyx mux up"`.
6. **Attached interactive terminal.** Launch `sandbox-webster-suite.cmd` from a real,
   attached console -- never redirected, backgrounded, or detached. Without a TTY the
   driving claude session cannot idle between turns waiting for notifications, so the
   process ends as soon as a turn ends and the remaining scenarios are silently abandoned.
   The launcher prints a warning when it detects non-console stdio.

## Black-box rule

**The agent under test works exclusively inside the Hub host repo (`lyx-test-HUB/lyx-test`).
It tests `lyx.exe` as a black box -- exactly as a real user with only the binary on PATH.
It must not look for, read, or reason about the lyx source tree. No peeking at
`C:\Code\loomyard\` or any other path outside the Hub.**

Discovering the command surface is done via `lyx webster --help` and `lyx webster
<subcommand> --help` alone -- not from documentation outside the Hub. The plan file(s) under
`_lyx/plan/` are the one artifact the agent must construct itself per each scenario's Goal
below; `docs/modules/plan-format.md`'s worked example (available via `lyx webster validate
--help` or by reasoning from validation error messages) is the reference for the file shape.
Keep every scenario's plan cards trivial -- e.g. "create `resultN.md` containing the single
line `OK`" -- so a real fork finishes each batch in one card, one commit, fast.

### Controlled tmux exceptions

One sanctioned deviation from the pure black-box rule, mirroring the mux/shuttle/burler/
builder suites' own controlled-exception note:

- **Direct `tmux -L <socket> list-panes`/`ls`** is allowed only to confirm Master's own
  strand exists (or was cleaned up), where `<socket>` is read from `lyx mux status` output
  -- this is also how W1 confirms no EXTRA strand appears per batch (a fork is not a new
  strand; there is exactly one implementer-bearing strand, Master's own, for the whole run).

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
  `lyx webster`, `lyx webster <subcommand>`, and `--help` flags (S0 ethos).
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
      "ref": "W2b",
      "title": "…",
      "body": "verdict: WARN\n\n…repro…"
    }
  ]
}
```

- `source` is the literal string `"sandbox-report"`.
- `items[]` holds only `WARN`/`FAIL` findings -- do not record `OK` scenarios here.
- `ref` is the scenario id (`W1`, or `W2a`/`W2b`/`W2c` for W2's three separately-verdicted
  assertions).
- `title` is a short one-line summary.
- `body` folds the detail, repro steps, and verdict into one markdown string.

Write only `source` and `items` -- a separate fetch step (run after the session) stamps
`meta` (including the binary fingerprint). Confine all free text to the `title`/`body`
string fields so the JSON stays well-formed.

## Scenarios

### W1 -- Happy path (full `run`, in-session forks)

**Covers:** webster

**Goal:** Pin a tiny two-batch plan whose cards each just write one fixed-content file, run
`lyx webster run`, and confirm it drives itself end-to-end -- via in-session Agent-tool
forks, not new mux strands -- to a `"outcome":"done"` outcome with both batches' cards
committed.

**Watch:** `lyx webster run` blocks until the run reaches a terminal outcome; the printed
JSON envelope reports `"outcome":"done"` with `batches_done: 2`. Confirm **one fork per
batch, no extra mux strands during batches**: `lyx mux status` shows exactly one
implementer-bearing strand for the entire run (Master's own) -- never a second strand
appearing and disappearing per batch the way builder's separate implementer strands do.
Confirm **per-batch weft commits landing**: `state.json` committed at each batch's
`begin-batch` (start-SHA + batch entry durable before the fork), and the batch report plus
`state.json` committed at each batch's `record-batch` (the main per-batch sync) -- inspect
the weft's own commit log for both. Confirm **digest envelopes from `record-batch`**: each
batch's fork-return is followed by a `record-batch` call whose JSON response carries the
pinned digest fields (`batch`, `status`, `tests`, `files_changed`, `dirty` -- the same
terse, prose-free shape builder's `poll` emits, never raw report prose). Confirm **a valid
`summary.md`** at exit: `_lyx/webster/summary.md` exists, its first line is `# <title>`,
and the rest is a non-empty narrative -- alongside `_lyx/webster/outcome.yaml`. Afterward,
Master's pane/run dir is cleaned up (no leftover strand; `lyx mux status` no longer lists
it).

**Verdict:** `OK` / `WARN` / `FAIL`

---

### W2 -- `/model` injection validation (the escalation-vs-fallback decider)

**Covers:** webster

**Goal:** Pin a plan with one `oversized: true` batch and drive the run far enough that
`begin-batch` for that batch fires its `/model` pane-injection choreography (see
discussion.md's `oversized-model-escalation` decision) **while the `begin-batch` Bash
subprocess call is still the foreground tool call executing inside Master's own pane** --
capture the pane's state at the moment the injection races the still-running subprocess,
not after that tool call has already returned. This is the load-bearing, previously
UNVALIDATED production timing: `/model` is a local Claude Code CLI command expected to
apply to subsequent API calls within Master's single long agentic turn, but whether
pane-injected keys actually reach the TUI input while a foreground subprocess is mid-flight
-- rather than merely in a quiet, idle pane -- had never been confirmed before this
scenario. **State plainly in the session log: this scenario's verdict decides whether the
oversized-batch model-escalation feature stays enabled, or permanently degrades to its
documented fallback** (`oversized:` stays accepted for plan-format compatibility but has no
spawn effect in webster).

**Watch**, recorded as **three separately-verdicted assertions** -- do not fold them into
one OK/WARN/FAIL; a miss on (a) alone is benign, a hit on (b) is dangerous regardless of
what (a) or (c) showed:

- **(a) Model switch takes effect.** The injected `/model <target>` keystrokes reach
  Claude's TUI input and the session's model actually switches for subsequent calls within
  the same turn (observe the pane directly, or a subsequent call's behavior, for the
  model change). **A miss here is the BENIGN failure mode**: `oversized:` keeps its
  plan-format compatibility but has no spawn effect in webster -- this IS the documented
  fallback, not a defect to fix.
- **(b) No corruption of the foreground call.** The injected keystrokes do **not** leak
  into the running foreground subprocess's own stdin/output, and that Bash tool call's own
  result is not corrupted by the injection racing it. **A hit here (corruption) is the
  DANGEROUS failure mode** -- it unconditionally triggers the fallback regardless of what
  (a) showed, since a corrupted tool result is worse than no escalation at all.
- **(c) Fork-transcript flush timing.** By the time the fork has COMPLETED (its report
  file has landed -- the moment `await-batch` returns `{"report": true}` and Master calls
  `record-batch`), the fork's `subagents/<id>.jsonl` transcript file already exists on
  disk (under the session's `~/.claude/projects/<encoded-cwd>/<sessionID>/subagents/`
  directory) -- the incremental per-batch audit's transcript-count-before-report-presence
  check (`record-batch`) depends on this flush having already happened by then, not
  merely by session end. (Forks are backgrounded on current Claude Code, so "the Agent
  call returning" is the spawn acknowledgment, not completion.)

**Verdict:** `OK` / `WARN` / `FAIL` for EACH of (a), (b), (c) independently; record all
three in the session log and name whichever one(s) failed.

## Session log format

After running all scenarios, record a short session summary:

```
Date: <YYYY-MM-DD>
Binary fingerprint: <copy from the header above>

W1:  <OK|WARN|FAIL> -- <one-line note if not OK>
W2a: <OK|WARN|FAIL> -- <one-line note if not OK>
W2b: <OK|WARN|FAIL> -- <one-line note if not OK>
W2c: <OK|WARN|FAIL> -- <one-line note if not OK>

sandbox-report.json written: <count of WARN/FAIL items>
```

`./sandbox-report.json` must be written before the session ends, per the Capturing findings
section above -- with `items: []` when every scenario was `OK`.

## Teardown

After the session summary is recorded and `./sandbox-report.json` is written, run `lyx mux
down` to tear down the tmux session/server the scenarios booted. An orphaned tmux server
holds open handles inside the Hub host repo and blocks the next `sandbox-build.cmd -reset`.
The launcher also runs `lyx mux down` itself after the session ends (deterministic backstop),
but run it here anyway -- defense-in-depth, and it keeps the Hub clean while the session is
still open for inspection.

## Notes

- Host/weft scenarios stay in `SANDBOX-CORE-SUITE.md`, mux/tmux scenarios stay in
  `SANDBOX-MUX-SUITE.md`, shuttle black-box agent scenarios stay in
  `SANDBOX-SHUTTLE-SUITE.md`, burler's own review+fix round scenarios stay in
  `SANDBOX-BURLER-SUITE.md`, perch's gate-loop scenarios stay in `SANDBOX-PERCH-SUITE.md`,
  builder's batch-loop scenarios stay in `SANDBOX-BUILDER-SUITE.md`; this suite holds only
  webster's fork-loop and model-escalation scenarios -- add `W` scenarios here, not in any
  other suite.
- This suite is a FLOOR, not a ceiling, and deliberately narrower than builder's own: the
  run-level mechanics (fingerprinting, `--fresh` archiving, chain rollback, pause,
  `run.lock` contention) are the SAME imported `builderengine` code builder's own suite
  (`SANDBOX-BUILDER-SUITE.md`'s B2-B9) already exercises against webster's own dirs, per
  the reuse-by-import decision -- duplicating those scenarios here would test the same Go
  code twice under a different module name. What is genuinely webster-specific is the fork
  loop itself (W1) and the one load-bearing, previously-unvalidated mechanism unique to
  webster: Go-driven `/model` pane injection (W2). Neither scenario proves implementer
  quality or plan-format content richness -- those are a normal code review's and `perch`'s
  job respectively.
