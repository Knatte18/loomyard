# SANDBOX-REED-WATCH-SUITE -- lyx reed live-observation suite

## What this is

A structured, **non-destructive** single-session walkthrough of `lyx reed`, built to be watched
continuously from one attach window instead of followed scenario-by-scenario the way
`SANDBOX-REED-SUITE.md` is. That suite deliberately covers crash/kill/rename/corruption
scenarios -- real defects hide there, but they are hard for an operator to follow visually, since
half of them tear the session down or kill a process out from under it. This suite is the calm
counterpart: no scenario here kills a server, kills a pane, renames a worktree, corrupts state, or
tears the session down. The operator attaches once, near the start, and stays attached through
every remaining step, watching the pane layout evolve live -- strands appearing, a parent
collapsing as a child appears, a real (cheap) Claude Code agent visibly working in its own pane,
a live terminal resize reflowing everything in front of them. Not an automated suite -- an agent
drives it, an operator watches continuously, without the visual whiplash of the destructive suite.

This suite intentionally never runs `lyx reed down`. Unlike every other reed-touching sandbox
suite, its `suiteSpec` sets `reedTeardown: false` -- the whole point is that the session stays up
and attached after the driving agent's own turn ends, so the operator can keep exploring by hand.
See "Session end" below.

## Pre-conditions

Before starting a session:

1. **Deploy a fresh dev binary.**
   Run `deploy-dev` to build `lyx.exe` into `.dev-bin` as current source.
   The suite resolves `.dev-bin` itself and prepends it to the agent's PATH (the fingerprint header's `Source: dev` line confirms the dev build is under test) -- no PATH setup needed, and production `lyx` stays untouched.
   The deployed binary is a snapshot -- re-deploy after any source change you want to test.
2. **Materialize the hub.**
   Run `sandbox/build.cmd` (or `sandbox/build.cmd -reset` to start clean);
   the session cwd is the Hub's Fabric repo root, the same operating model as the main suite.
3. **Live-tmux requirement.** tmux (or the Windows tmux port) on PATH and PowerShell 7 present.
4. **A logged-in `claude` on PATH**, since W1 spawns a real, live Claude Code agent inside a
   reed pane -- not a placeholder command. Skip W1 with a note if none is configured.

## Black-box rule

**The agent under test works exclusively inside the Hub's Fabric repo (`lyx-test-HUB/lyx-test`).
It tests `lyx.exe` as a black box -- exactly as a real user with only the binary on PATH.
It must not look for, read, or reason about the lyx source tree.
No peeking at `C:\Code\loomyard\` or any other path outside the Hub.**

Discovering the command surface is done via `lyx reed`, `lyx reed <subcommand>`, and `lyx reed <subcommand> --help` alone -- not from documentation outside the Hub.

There is no controlled-tmux-exception clause in this suite: unlike `SANDBOX-REED-SUITE.md`, no
scenario here ever calls raw `tmux` directly (no `kill-pane`, no `kill-server`, no `split-window`).
Every check goes through `lyx reed status`/`lyx reed attach` alone.

## Fingerprint header

The launcher prepends a "binary under test" fingerprint block to this file when it copies it into the Hub's Fabric repo.
The fingerprint records the absolute path, file size, modification time, and a short SHA-256 of the `lyx.exe` binary at launch time.

The same fingerprint identifies the binary for the report's provenance: a separate fetch step (run after this session) stamps it into `meta.fingerprint` of the fetched `sandbox-report.json` so a maintainer can reproduce the exact binary that produced each finding.
The agent does not need to transcribe the fingerprint anywhere itself.

## How to run a scenario

For each scenario below:

- Read the **Goal** -- it names the task, not the commands.
  Discover the commands via `lyx reed`, `lyx reed <subcommand>`, and `--help` flags, except where the Watch section names an exact flag because precision matters for reproducibility.
- **Watch** what lyx does.
  Note where it stalls, guesses wrong, or hits an error.
- Record the outcome per the verdict buckets: `OK` (worked) / `WARN` (rough edge) / `FAIL` (broke).

## Verdict key

- `OK`   -- completed without friction
- `WARN` -- completed but with confusion, awkward UX, or a non-fatal error
- `FAIL` -- did not complete;
  lyx broke, panicked, or gave wrong output

## Capturing findings

After all scenarios are run, write **all** `WARN`/`FAIL` findings to `./sandbox-report.json` (in the Fabric-repo cwd) on this exact schema.
**Always write the file, even when there are zero `WARN`/`FAIL` findings** -- in that case `items` is an empty array.

```json
{
  "source": "sandbox-report",
  "items": [
    {
      "ref": "W3",
      "title": "…",
      "body": "verdict: WARN\n\n…repro…"
    }
  ]
}
```

- `source` is the literal string `"sandbox-report"`.
- `items[]` holds only `WARN`/`FAIL` findings -- do not record `OK` scenarios here.
- `ref` is the scenario id (`W0`-`W5`).
- `title` is a short one-line summary.
- `body` folds the detail, repro steps, and verdict into one markdown string.

Write only `source` and `items` -- a separate fetch step (run after the session) stamps `meta` (including the binary fingerprint).
Confine all free text to the `title`/`body` string fields so the JSON stays well-formed.

## Scenarios

### W0 -- Attach and stay

**Goal:** "Bring the reed overlay online, then have the operator attach in a second terminal and
stay attached through every scenario that follows -- nothing later in this suite should require a
fresh attach."

**Watch:** `lyx reed up` boots cleanly with no strands yet.
The agent then pauses and instructs the operator to run `lyx reed attach` **in a second terminal**
now, before any strand exists, and to keep that terminal open and visible for the remainder of the
session rather than detaching between scenarios.
Rationale: the agent session owns the current terminal, so it cannot demonstrate or observe the
takeover itself (same rationale as `SANDBOX-REED-SUITE.md`'s M7).

**Verdict:** `OK` / `WARN` / `FAIL`

---

### W1 -- A cheap live agent doing visible work

**Covers:** reed

**Goal:** "Add a strand running a real, cheap Claude Code agent (Haiku) on a small, verifiable
task, and confirm the operator can watch it actually work in the attached window -- not a
placeholder `sleep`."

**Watch:** `lyx reed add --cmd 'claude --model haiku --dangerously-skip-permissions "write a
one-file Go program that prints a distinctive, short string, then run it with go run and show the
output"'` returns a guid;
the pane appears in the attach window and the operator watches Haiku's output scroll live,
ending with the program's real output visible in the pane.
`--dangerously-skip-permissions` on this nested call is required, not optional -- this pane has no
one present to approve a tool-use prompt, and an unattended Claude session stalled on a permission
prompt is indistinguishable from a hang.
This is only safe because the strand runs inside the disposable Hub warp repo, the same reason the
suite's own driving agent is launched the same way (see `suite.go`'s `launchAgent`).
A pane that never produces visible output, stalls waiting on a permission prompt, or exits
immediately with no output is a `FAIL`.
Keep this strand's guid -- W3 adds a child under it.

**Verdict:** `OK` / `WARN` / `FAIL`

---

### W2 -- A hidden strand alongside a visible one

**Goal:** "Add a strand that produces no pane, confirm reed still tracks it, and confirm nothing
already in the attach window was disturbed."

**Watch:** `lyx reed add --anchor hidden --cmd <a plain long-running command>` creates **no** new
pane -- the operator confirms this by eye in the attach window, not just by reading JSON --
while `lyx reed status` still lists the strand.
The W1 pane keeps running untouched.

**Verdict:** `OK` / `WARN` / `FAIL`

---

### W3 -- A child strand collapses its parent live

**Covers:** reed

**Goal:** "Add a child strand under the W1 Haiku strand and watch the parent visibly collapse to a
compact strip in real time while the child takes the bulk of the window."

**Watch:** `lyx reed add --parent <W1-guid> --cmd <a plain long-running command>` -- both strands
read `live: true` in `status`, and the operator watches, in the attach window, the parent's pane
shrink to `collapsed_strip_rows` as the split happens, with the child's pane taking the rest of
the window.
A parent that does not visibly shrink, a child pane that never appears, or either strand flipping
to `live: false` is a `FAIL`.

**Verdict:** `OK` / `WARN` / `FAIL`

---

### W4 -- Status while watching

**Goal:** "Check status and correlate it against what the operator can see in the attach window."

**Watch:** `lyx reed status` lists every strand added so far (W1, W2, W3) with `live`/`paneId`
values that match what the operator observes: three panes visible for W1+W3 (W2 stays paneless),
and the parent/child geometry from W3 still holding.

**Verdict:** `OK` / `WARN` / `FAIL`

---

### W5 -- Live resize, both directions (operator-assisted visual)

**Covers:** reed

**Goal:** "Prove a live terminal resize re-applies the layout on its own, in both directions, using
the same attach window from W0 -- no new attach needed."

**Watch:** With the operator still attached from W0, have them **drag the terminal window
larger** and confirm the layout re-applies within about a second, with no `lyx` command run.
Then have them **drag it smaller** and confirm the same.
The shrink direction is the non-negotiable half of this scenario -- it is the one SIGWINCH misses
entirely, and a watcher that only self-heals on growth must be reported as a `FAIL` here, not a
`WARN`.
Confirm the cursor did NOT jump to another pane across either resize, and that a pane focused
before the resize stays focused after it.
Rationale: the agent session owns the current terminal, so it cannot demonstrate or observe a live
client resize itself.

The confirmed mechanism, if dots briefly appear during either drag: they are tmux's own padding or stale paint in the region of a client's terminal the window does not cover, never reed content,
and both the resize trigger here and the cross-client trigger below are the same underlying mismatch between a client's terminal size and the window's size.
No repaint entry shipped for this task -- see `internal/reedengine/doc.go`'s decision log (the Measurement record and the bullets that follow it) for which candidates were tried and why both were rejected -- so expect the roughly one-second smear the watchdog's own round trip heals, not a brief flicker, on each drag.

Also try the cross-client trigger, since the original finding could not explain it on its own: attach a second `lyx reed attach` client of a **different** size to the same session from another terminal,
then move the pointer into or type into that second window -- no click required -- and watch the *first* client fill with dots.
When the observed client is larger than the client that just became most-recently-used, those dots are correct tmux behavior for uncovered real estate and are **expected** -- report the scenario `OK` for that case rather than filing it again.
reed now logs a `logger.Warn` at attach time naming any already-attached client whose size differs from the one attaching, which is the searchable trace for this exact condition.

**Verdict:** `OK` / `WARN` / `FAIL`

## Session end

**No `down` is run as part of this suite, on purpose.**
Once W5 finishes, the reed session is left live and attached so the operator can keep exploring by
hand afterward -- add more strands, watch Haiku work longer, resize again.
This is the one difference from every other reed-touching sandbox suite: `runSuite`'s automatic
post-session `lyx reed down` (see `suite.go`'s `reedTeardown` field) is deliberately turned off for
this suite's `suiteSpec`, specifically so the session survives past the driving agent's own turn
ending.
Tear it down yourself with `lyx reed down` when you are done exploring -- an orphaned tmux server
otherwise holds handles inside the Hub and blocks the next `sandbox/build.cmd -reset`.

After running all scenarios, record a short session summary:

```
Date: <YYYY-MM-DD>
Binary fingerprint: <copy from the header above>

W0: <OK|WARN|FAIL> -- <one-line note if not OK>
W1: <OK|WARN|FAIL> -- <one-line note if not OK>
W2: <OK|WARN|FAIL> -- <one-line note if not OK>
W3: <OK|WARN|FAIL> -- <one-line note if not OK>
W4: <OK|WARN|FAIL> -- <one-line note if not OK>
W5: <OK|WARN|FAIL> -- <one-line note if not OK>

sandbox-report.json written: <count of WARN/FAIL items>
```

`./sandbox-report.json` must be written before the session ends, per the Capturing findings section above -- with `items: []` when every scenario was `OK`.

## Notes

- This suite exists alongside, not instead of, `SANDBOX-REED-SUITE.md` -- that suite's
  crash/kill/rename/corruption scenarios (M8-M11, M15-M17, M19-M25) still need their own run;
  this one only replaces the *experience* of watching the calm subset (M2-M5, M12-M14, M18, M26)
  with one continuous attach session plus a real live agent, never the coverage.
- Grow this suite with more calm, visually-interesting scenarios (more nesting depth, more
  concurrent strands) -- never with anything that kills a process or tears the session down; that
  belongs in `SANDBOX-REED-SUITE.md` instead.
