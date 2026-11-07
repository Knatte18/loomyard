# SANDBOX-REED-WATCH-SUITE -- lym reed live-observation suite

## What this is

A structured, **non-destructive** single-session walkthrough of `lym reed`, built to be watched continuously from one attach window.
`SANDBOX-REED-SUITE.md` covers crash, kill, rename and corruption scenarios;
real defects hide there, but an operator cannot follow them visually,
because many tear the session down or kill a process out from under it.
This suite is the calm counterpart:
no scenario kills a server, kills a pane, renames a worktree, corrupts state, or tears the session down.
The operator attaches once, near the start, and stays attached through every remaining step.
They watch the window list evolve live:
strands appear in windows of their own, and a real (cheap) Claude Code agent works in its own window.
An agent drives the suite while an operator watches; it is not an automated suite.

This suite never runs `lym reed down`.
Its `suiteSpec` sets `reedTeardown: false`,
so the session stays up and attached after the driving agent's turn ends, and the operator can keep exploring by hand.
See "Session end" below.

## Pre-conditions

Before starting a session:

1. **Deploy a fresh dev binary.**
   Run `deploy-dev` to build `lym.exe` into `.dev-bin` as current source.
   The suite resolves `.dev-bin` itself and prepends it to the agent's PATH, so no PATH setup is needed and production `lym` stays untouched.
   The fingerprint header's `Source: dev` line confirms the dev build is under test.
   The deployed binary is a snapshot -- re-deploy after any source change you want to test.
2. **Materialize the hub.**
   Run `sandbox/build.cmd` (or `sandbox/build.cmd -reset` to start clean);
   the session cwd is the Hub's Fabric repo root, the same operating model as the main suite.
3. **Live-tmux requirement.** tmux (or the Windows tmux port) on PATH and PowerShell 7 present.
4. **A logged-in `claude` on PATH**, since W1 spawns a real, live Claude Code agent inside a reed pane, not a placeholder command.
   Skip W1 with a note if none is configured.

## Black-box rule

**The agent under test works exclusively inside the Hub's Fabric repo (`lym-test-LYMHUB/lym-test`).
It tests `lym.exe` as a black box -- exactly as a real user with only the binary on PATH.
It must not look for, read, or reason about the lym source tree.
No peeking at `C:\Code\loomyard\` or any other path outside the Hub.**

Discover the command surface through `lym reed`, `lym reed <subcommand>`, and `lym reed <subcommand> --help` alone.
Documentation outside the Hub is off limits.

This suite has no controlled-tmux exception:
no scenario calls raw `tmux` directly (no `kill-pane`, no `kill-server`, no `split-window`).
Every check goes through `lym reed status` and `lym reed attach` alone.

## Fingerprint header

The launcher prepends a "binary under test" fingerprint block to this file when it copies it into the Hub's Fabric repo.
The fingerprint records the absolute path, file size, modification time,
and a short SHA-256 of the `lym.exe` binary at launch time.

The same fingerprint identifies the binary for the report's provenance.
A separate fetch step, run after this session, stamps it into `meta.fingerprint` of the fetched `sandbox-report.json`,
so a maintainer can reproduce the exact binary that produced each finding.
The agent does not need to transcribe the fingerprint anywhere itself.

## How to run a scenario

For each scenario below:

- Read the **Goal** -- it names the task, not the commands.
  Discover the commands via `lym reed`, `lym reed <subcommand>`, and `--help` flags,
  except where the Watch section names an exact flag for reproducibility.
- **Watch** what lym does.
  Note where it stalls, guesses wrong, or hits an error.
- Record the outcome per the verdict buckets: `OK` (worked) / `WARN` (rough edge) / `FAIL` (broke).

## Verdict key

- `OK`   -- completed without friction
- `WARN` -- completed but with confusion, awkward UX, or a non-fatal error
- `FAIL` -- did not complete;
  lym broke, panicked, or gave wrong output

## Capturing findings

After all scenarios are run, write **all** `WARN`/`FAIL` findings to `./sandbox-report.json` in the Fabric-repo cwd,
on this exact schema.
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

Write only `source` and `items`.
A separate fetch step, run after the session, stamps `meta`, including the binary fingerprint.
Confine all free text to the `title`/`body` string fields so the JSON stays well-formed.

## Scenarios

### W0 -- Attach and stay

**Goal:** "Bring the reed overlay online, then have the operator attach in a second terminal and stay attached through every scenario that follows -- nothing later in this suite should require a fresh attach."

**Watch:** `lym reed up` boots cleanly with no strands yet.
The agent then pauses and instructs the operator to run `lym reed attach` **in a second terminal** now, before any strand exists.
The operator keeps that terminal open and visible for the rest of the session rather than detaching between scenarios.
The agent session owns the current terminal, so it cannot demonstrate or observe the takeover itself;
`SANDBOX-REED-SUITE.md`'s M7 has the same constraint.

**Verdict:** `OK` / `WARN` / `FAIL`

---

### W1 -- A cheap live agent doing visible work

**Covers:** reed

**Goal:** "Add a strand running a real, cheap Claude Code agent (Haiku) on a small, verifiable task, and confirm the operator can watch it work in the attached window -- not a placeholder `sleep`."

**Watch:** `lym reed add --cmd 'claude --model haiku --dangerously-skip-permissions "write a
one-file Go program that prints a distinctive, short string, then run it with go run and show the
output"'` returns a guid;
the strand's own window appears in the attach window's status bar, the operator switches to it, and watches Haiku's output scroll live,
ending with the program's real output visible in the pane.
`--dangerously-skip-permissions` on this nested call is required:
no one is present in this pane to approve a tool-use prompt,
and an unattended Claude session stalled on a permission prompt is indistinguishable from a hang.
This is safe because the strand runs inside the disposable Hub warp repo.
The suite's driving agent is launched the same way, by `launchAgent` in `suite.go`.
A pane that never produces visible output, stalls waiting on a permission prompt, or exits immediately with no output is a `FAIL`.
Keep this strand's guid -- W4 correlates it against `status`.

**Verdict:** `OK` / `WARN` / `FAIL`

---

### W2 -- A hidden strand alongside a visible one

**Goal:** "Add a strand that produces no pane, confirm reed still tracks it, and confirm nothing already in the attach window was disturbed."

**Watch:** `lym reed add --anchor hidden --cmd <a plain long-running command>` creates **no** new pane;
the operator confirms this by eye in the attach window, not just by reading JSON.
`lym reed status` still lists the strand.
The W1 pane keeps running untouched.

**Verdict:** `OK` / `WARN` / `FAIL`

---

### W3 -- Retired: the live parent collapse is removed with the shell-window stack

---

### W4 -- Status while watching

**Goal:** "Check status and correlate it against what the operator can see in the attach window."

**Watch:** `lym reed status` lists every strand added so far (W1, W2) with `live`/`paneId` values that match what the operator observes:
a window of its own for W1 (W2 stays paneless).

**Verdict:** `OK` / `WARN` / `FAIL`

---

### W5 -- Retired: the live resize self-heal is removed with the shell-window stack

## Session end

**This suite runs no `down`.**
Once W4 finishes, the reed session is left live and attached so the operator can keep exploring by hand:
add more strands, watch Haiku work longer.
Other reed-touching sandbox suites end with an automatic `lym reed down` from `runSuite`, gated by the `reedTeardown` field in `suite.go`;
this suite's `suiteSpec` turns it off so the session survives the driving agent's turn.
Tear it down yourself with `lym reed down` when you are done exploring;
an orphaned tmux server otherwise holds handles inside the Hub and blocks the next `sandbox/build.cmd -reset`.

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

`./sandbox-report.json` must be written before the session ends, as the Capturing findings section describes.
Write `items: []` when every scenario was `OK`.

## Notes

- This suite complements `SANDBOX-REED-SUITE.md` and does not replace it.
  That suite's crash, kill, rename and corruption scenarios still need their own run;
  this one changes how the calm subset is watched, as one continuous attach session plus a real live agent, not the coverage.
- Grow this suite with more calm, visually interesting scenarios, such as more nesting depth or more concurrent strands.
  Anything that kills a process or tears the session down belongs in `SANDBOX-REED-SUITE.md`.
