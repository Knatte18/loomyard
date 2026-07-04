# MUX-SANDBOX-SUITE -- lyx mux black-box agent suite

## What this is

A structured test-loop for exercising `lyx mux` against a **live psmux server** in the
sandbox Hub host repo. Unlike the host/weft-centric main suite (`SANDBOX-SUITE.md`), the
value here is partly **visual**: panes popping up, layout holding. Not an automated
suite -- an agent drives it, an operator watches.

## Pre-conditions

Before starting a session:

1. **Deploy a fresh binary.** Run `deploy.cmd` so `lyx.exe` on PATH is current source.
   The deployed binary is a snapshot -- re-deploy after any source change you want to test.
2. **Materialize the hub.** Run `sandbox-build.cmd` (or `sandbox-build.cmd -reset`
   to start clean); the session cwd is the Hub host repo root, the same operating model
   as the main suite.
3. **Live-psmux requirement.** `psmux.exe` on PATH (installed at
   `C:\Code\tools\bin\psmux.exe`) and PowerShell 7 present. If psmux or pwsh is
   unavailable in the session, **note that as the session outcome rather than treating
   it as a mux defect** -- the `**Covers:** mux` tag on M2 satisfies the sandbox
   coverage guard (`sandbox_coverage_test.go`) regardless of runtime availability.

## Black-box rule

**The agent under test works exclusively inside the Hub host repo (`lyx-test-HUB/lyx-test`).
It tests `lyx.exe` as a black box -- exactly as a real user with only the binary on PATH.
It must not look for, read, or reason about the lyx source tree. No peeking at
`C:\Code\loomyard\` or any other path outside the Hub.**

Discovering the command surface is done via `lyx mux`, `lyx mux <subcommand>`, and
`lyx mux <subcommand> --help` alone -- not from documentation outside the Hub.

### Controlled psmux exceptions

Two sanctioned deviations from the pure black-box rule, mirroring the main suite's S6
controlled-exception note:

- **(a) Direct `psmux -L <socket>` verbs** (`kill-server`, `list-panes`, `ls`) are
  allowed **only** for crash simulation and layout/stray-state verification, where
  `<socket>` is taken from `lyx mux status` output (its JSON result carries `session`,
  `socket`, and `strands[]` with `guid`/`name`/`paneId`/`live`).
- **(b) Scenario M7 (attach) is operator-assisted** -- see M7 below.

## Fingerprint header

The launcher prepends a "binary under test" fingerprint block to this file when it
copies it into the Hub host repo. The fingerprint records the absolute path, file size,
modification time, and a short SHA-256 of the `lyx.exe` binary at launch time.

The same fingerprint identifies the binary for the report's provenance: a separate
fetch step (run after this session) stamps it into `meta.fingerprint` of the
fetched `sandbox-report.json` so a maintainer can reproduce the exact binary that
produced each finding. The agent does not need to transcribe the fingerprint
anywhere itself.

## How to run a scenario

For each scenario below:

- Read the **Goal** -- it names the task, not the commands. Discover the commands via
  `lyx mux`, `lyx mux <subcommand>`, and `--help` flags (M0 ethos).
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
      "ref": "M5",
      "title": "…",
      "body": "verdict: WARN\n\n…repro…"
    }
  ]
}
```

- `source` is the literal string `"sandbox-report"`.
- `items[]` holds only `WARN`/`FAIL` findings -- do not record `OK` scenarios here.
- `ref` is the scenario id (`M0`-`M15`).
- `title` is a short one-line summary.
- `body` folds the detail, repro steps, and verdict into one markdown string.

Write only `source` and `items` -- a separate fetch step (run after the session)
stamps `meta` (including the binary fingerprint). Confine all free text to the
`title`/`body` string fields so the JSON stays well-formed.

## Scenarios

### M0 -- Discovery

**Goal:** "You have `lyx` on PATH and nothing else inside this repo. Find out what
`lyx mux` can do and report the full command tree."

**Watch:** Does `lyx mux` list all subcommands (`up`, `add`, `remove`, `status`,
`attach`, `resume`, `down`) with accurate `Short`s? Does each `--help` explain itself?

**Verdict:** `OK` / `WARN` / `FAIL`

---

### M1 -- Pre-up ergonomics

**Goal:** "From a fresh state (no mux session running), try to add a strand and remove
one. See how mux tells you what to do first."

**Watch:** `lyx mux add --cmd ...` and `lyx mux remove <guid>` must fail with the
friendly JSON-envelope error `no mux session; run "lyx mux up"` -- that message is the
`OK` outcome, not a finding.

**Verdict:** `OK` / `WARN` / `FAIL`

---

### M2 -- Up

**Covers:** mux

**Goal:** "Bring the mux overlay online for this worktree."

**Watch:** `lyx mux up` boots the named server + this worktree's session; a second `up`
is a clean no-op (idempotent). It runs no strand command (substrate-only).

**Verdict:** `OK` / `WARN` / `FAIL`

---

### M3 -- Add (visible)

**Goal:** "Add a visible strand running a long-lived command and confirm it shows up as
a pane."

**Watch:** `lyx mux add --cmd <long-running command>` returns an assigned `guid` +
resolved `name`; the pane exists and the layout applies.

**Verdict:** `OK` / `WARN` / `FAIL`

---

### M4 -- Status

**Goal:** "Ask mux what it currently knows about this session."

**Watch:** `lyx mux status` reports `session`, `socket`, and the strand from M3 with
`live: true`.

**Verdict:** `OK` / `WARN` / `FAIL`

---

### M5 -- Add hidden

**Goal:** "Add a strand that should not show up as a pane, then confirm mux still knows
about it."

**Watch:** `lyx mux add --anchor hidden --cmd ...` creates **no pane**; `status` still
lists the strand; `lyx mux resume` **skips** it (hidden strands are pending, not dead --
still no pane after resume). Note explicitly: surfacing a hidden strand is
engine-API-only in v1 (there is no `lyx mux update` verb) -- do **not** attempt it, and
its absence is not a finding.

**Verdict:** `OK` / `WARN` / `FAIL`

---

### M6 -- Layout sanity (>=2 top, 0 stack)

**Goal:** "With at least two top-anchored strands and no below-parent strands, confirm
the layout tiles the full box with no torn/leftover rows."

**Watch:** The last top band stretches to tile the full box. Verify via
`psmux -L <socket> list-panes` geometry (controlled exception) or visually at M7.

**Verdict:** `OK` / `WARN` / `FAIL`

---

### M7 -- Attach (operator-assisted visual)

**Goal:** "Confirm the pane layout looks right by having the operator attach and look."

**Watch:** The agent pauses and instructs the operator to run `lyx mux attach` **in a
second terminal**, visually confirm the pane layout, then detach; the agent records the
operator's verdict. Rationale: the agent session owns the current terminal, so it cannot
demonstrate or observe the takeover itself. Also note: `attach` is a documented envelope
exception -- pre-flight failures come as the JSON envelope; a successful handover emits
no JSON. Neither behaviour is a finding.

**Verdict:** `OK` / `WARN` / `FAIL`

---

### M8 -- Resume semantics

**Goal:** "Confirm resume is a no-op when everything is already live, then confirm it
recreates a pane that was killed out from under it."

**Watch:** `lyx mux resume` with all strands live leaves them untouched (no double
launch). Then kill one strand's pane via `psmux -L <socket> kill-pane -t <paneId>`
(controlled exception; `paneId` from `status`) and run `resume` again: the strand's pane
is recreated and its stored resume/launch command replays.

**Verdict:** `OK` / `WARN` / `FAIL`

---

### M9 -- Crash-resume

**Goal:** "Simulate a server crash and confirm resume rebuilds everything."

**Watch:** `psmux -L <socket> kill-server` (controlled exception) simulates a server
crash; `lyx mux resume` boots a fresh server + session and replays every non-hidden
strand's command into a new pane.

**Verdict:** `OK` / `WARN` / `FAIL`

---

### M10 -- Recursive remove

**Goal:** "Try to remove a strand that has children, first without and then with
`--recursive`."

**Watch:** Removing a strand that has children without `--recursive` fails with
`strand has children, use --recursive`; with `--recursive` the removal cascades over the
subtree and the result JSON lists every removed strand. "No stray state" also applies to
`remove`: like `down`, it waits for the removed panes' whole process subtree to exit before
returning, so immediately after a `remove` no leftover shell keeps the worktree directory
busy (the pre-`remove` `#{pane_pid}` values and their descendants are gone from `tasklist`).
Covered headlessly by `TestSmokeRemoveReapsRemovedPaneChildProcesses`.

**Verdict:** `OK` / `WARN` / `FAIL`

---

### M11 -- Down without stray state

**Goal:** "Tear the overlay down and confirm nothing is left behind."

**Watch:** `lyx mux down` kills the server and clears the worktree's strand state;
`psmux -L <socket> ls` (controlled exception) confirms no server survives, and a
follow-up `lyx mux status` reports the friendly no-session error rather than stale
strands. "No stray state" means, concretely, that the instant `down` returns: (a) **no
psmux process names this socket** — `down` force-reaps the server *and* its internal
`__warm__` helper and confirms the socket is clear, because both carry the worktree as
their cwd and a survivor would keep the worktree dir busy (check with `tasklist | findstr
psmux` filtered to this `-L` socket — expect zero); and (b) **no pane process subtree
survives** — `down` always reaps this session's whole pane process subtree before
returning (checkable via the pre-`down` `#{pane_pid}` values and their descendants being
gone from `tasklist`). Covered headlessly by `TestSmokeDownReapsPaneChildProcesses` and
`TestSmokeDownLeavesNoPsmuxOnSocket`.

> **Not a FAIL:** a `conhost.exe` (the OS ConPTY host psmux uses per pane) may linger for a
> beat with the worktree as its cwd and then exit on its own. It is not a `#{pane_pid}`
> descendant and mux does not reap it — a dying OS console host is not stray *agent* state.
> Under heavy concurrent load this can briefly delay deleting the worktree dir; that is an OS
> teardown race, not a mux leak. Only a surviving **psmux** process or a live **pane shell**
> is a `FAIL` here.

**Verdict:** `OK` / `WARN` / `FAIL`

---

### M12 -- Layout survival under mixed adds

**Goal:** "Build a busy session -- two top-anchored strands, then a parent strand,
then a child under it -- and confirm every strand still has its own live pane."

**Watch:** After all four `add`s, `lyx mux status` reports all four strands
`live: true`, and `psmux -L <socket> list-panes` (controlled exception) shows exactly
four panes with sane geometry (the two top strands as compact bands, the child as the
dominant bottom pane). A pane count below four, an empty pane list, or a strand that
flips to `live: false` after the next verb means a split/apply silently destroyed
panes -- that is a `FAIL`, not cosmetics.

**Verdict:** `OK` / `WARN` / `FAIL`

---

### M13 -- Add after removing the last strand

**Goal:** "Remove the session's only strand, then add a fresh one and prove its
command actually runs."

**Watch:** `lyx mux remove <guid>` on the sole strand succeeds; the following
`lyx mux add --cmd <long-running command>` returns a guid, and the strand reads
`live: true` in `status` **both immediately and again after one more verb** (e.g.
`lyx mux up`) -- a strand that reads live once and then flips to `live: false` with an
empty `paneId` adopted a dead leftover pane and its command never ran (`FAIL`).

**Verdict:** `OK` / `WARN` / `FAIL`

---

### M14 -- Attach visual (operator or capture-assisted)

**Goal:** "Confirm `lyx mux attach` actually renders the session's panes, not just
that the command exits."

**Watch:** With the overlay up and at least one strand added, `lyx mux attach` from a
**second terminal** shows the strands' panes and the psmux status bar; `Ctrl+b d`
detaches cleanly and the session keeps running (`lyx mux status` still lists it).
Operator-assisted like M7. (The automated layer covers this headlessly:
`TestSmokeAttachRendersInsideHarnessPane` drives attach inside a harness psmux pane
and asserts the rendered content -- so a `FAIL` here is a visual/UX issue, not a
correctness gap.)

**Verdict:** `OK` / `WARN` / `FAIL`

---

### M15 -- Claude resume recall (needs a logged-in claude)

**Goal:** "Prove a real agent's conversation survives a crash: launch claude in a
strand, give it a codeword, crash the server, resume, and confirm the codeword comes
back."

**Watch:** `lyx mux add --cmd "claude '...codeword...'" --resume-cmd "claude
--continue"`; after claude answers, `psmux -L <socket> kill-server` (controlled
exception) simulates a crash; `lyx mux resume` replays `claude --continue` and the
resumed pane recalls the codeword. If `claude --continue` reports **"No conversation
found"**, env hygiene failed to let the transcript persist -- that is a `FAIL` (the one
Claude-adjacent thing mux owns). Skip with a note if no claude is configured. (Covered
headlessly by `TestSmokeClaudeResumeRecallsCodeword`.)

**Verdict:** `OK` / `WARN` / `FAIL`

## Session log format

After running all scenarios, record a short session summary:

```
Date: <YYYY-MM-DD>
Binary fingerprint: <copy from the header above>

M0: <OK|WARN|FAIL> -- <one-line note if not OK>
M1: <OK|WARN|FAIL> -- <one-line note if not OK>
M2: <OK|WARN|FAIL> -- <one-line note if not OK>
M3: <OK|WARN|FAIL> -- <one-line note if not OK>
M4: <OK|WARN|FAIL> -- <one-line note if not OK>
M5: <OK|WARN|FAIL> -- <one-line note if not OK>
M6: <OK|WARN|FAIL> -- <one-line note if not OK>
M7: <OK|WARN|FAIL> -- <one-line note if not OK>
M8: <OK|WARN|FAIL> -- <one-line note if not OK>
M9: <OK|WARN|FAIL> -- <one-line note if not OK>
M10: <OK|WARN|FAIL> -- <one-line note if not OK>
M11: <OK|WARN|FAIL> -- <one-line note if not OK>
M12: <OK|WARN|FAIL> -- <one-line note if not OK>
M13: <OK|WARN|FAIL> -- <one-line note if not OK>
M14: <OK|WARN|FAIL> -- <one-line note if not OK>
M15: <OK|WARN|FAIL> -- <one-line note if not OK>

sandbox-report.json written: <count of WARN/FAIL items>
```

`./sandbox-report.json` must be written before the session ends, per the Capturing
findings section above -- with `items: []` when every scenario was `OK`.

## Notes

- Host/weft scenarios stay in `SANDBOX-SUITE.md`; this suite grows with mux (windows
  for clusters, daemon) -- add `M` scenarios here, not in the main suite.
