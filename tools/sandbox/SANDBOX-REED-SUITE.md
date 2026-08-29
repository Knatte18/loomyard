# SANDBOX-REED-SUITE -- lyx reed black-box agent suite

## What this is

A structured test-loop for exercising `lyx reed` against a **live tmux server** in the sandbox Hub's Fabric repo.
Unlike the warp/weft-centric main suite (`SANDBOX-CORE-SUITE.md`), the value here is partly **visual**: panes popping up, layout holding.
Not an automated suite -- an agent drives it, an operator watches.

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
   If tmux or pwsh is unavailable in the session, **note that as the session outcome rather than treating it as a reed defect** -- the `**Covers:** reed` tag on M2 satisfies the sandbox coverage guard (`sandbox_coverage_test.go`) regardless of runtime availability.

## Black-box rule

**The agent under test works exclusively inside the Hub's Fabric repo (`lyx-test-HUB/lyx-test`).
It tests `lyx.exe` as a black box -- exactly as a real user with only the binary on PATH.
It must not look for, read, or reason about the lyx source tree.
No peeking at `C:\Code\loomyard\` or any other path outside the Hub.**

Discovering the command surface is done via `lyx reed`, `lyx reed <subcommand>`, and `lyx reed <subcommand> --help` alone -- not from documentation outside the Hub.

### Controlled tmux exceptions

Two sanctioned deviations from the pure black-box rule, mirroring the main suite's S6 controlled-exception note:

- **(a) Direct `tmux -L <socket>` verbs** (`kill-server`, `list-panes`, `ls`) are allowed **only** for crash simulation and layout/stray-state verification, where `<socket>` is taken from `lyx reed status` output (its JSON result carries `session`, `socket`, and `strands[]` with `guid`/`name`/`paneId`/`live`).
- **(b) Scenario M7 (attach) is operator-assisted** -- see M7 below.

## Fingerprint header

The launcher prepends a "binary under test" fingerprint block to this file when it copies it into the Hub's Fabric repo.
The fingerprint records the absolute path, file size, modification time, and a short SHA-256 of the `lyx.exe` binary at launch time.

The same fingerprint identifies the binary for the report's provenance: a separate fetch step (run after this session) stamps it into `meta.fingerprint` of the fetched `sandbox-report.json` so a maintainer can reproduce the exact binary that produced each finding.
The agent does not need to transcribe the fingerprint anywhere itself.

## How to run a scenario

For each scenario below:

- Read the **Goal** -- it names the task, not the commands.
  Discover the commands via `lyx reed`, `lyx reed <subcommand>`, and `--help` flags (M0 ethos).
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
      "ref": "M5",
      "title": "…",
      "body": "verdict: WARN\n\n…repro…"
    }
  ]
}
```

- `source` is the literal string `"sandbox-report"`.
- `items[]` holds only `WARN`/`FAIL` findings -- do not record `OK` scenarios here.
- `ref` is the scenario id (`M0`-`M22`).
- `title` is a short one-line summary.
- `body` folds the detail, repro steps, and verdict into one markdown string.

Write only `source` and `items` -- a separate fetch step (run after the session) stamps `meta` (including the binary fingerprint).
Confine all free text to the `title`/`body` string fields so the JSON stays well-formed.

## Scenarios

### M0 -- Discovery

**Goal:** "You have `lyx` on PATH and nothing else inside this repo.
Find out what `lyx reed` can do and report the full command tree."

**Watch:** Does `lyx reed` list all subcommands (`up`, `add`, `remove`, `status`, `attach`, `resume`, `down`) with accurate `Short`s? Does each `--help` explain itself?

**Verdict:** `OK` / `WARN` / `FAIL`

---

### M1 -- Pre-up ergonomics

**Goal:** "From a fresh state (no reed session running), try to add a strand and remove one.
See how reed tells you what to do first."

**Watch:** `lyx reed add --cmd ...` and `lyx reed remove <guid>` must fail with the friendly JSON-envelope error `no reed session; run "lyx reed up"` -- that message is the `OK` outcome, not a finding.

**Verdict:** `OK` / `WARN` / `FAIL`

---

### M2 -- Up

**Covers:** reed

**Goal:** "Bring the reed overlay online for this worktree."

**Watch:** `lyx reed up` boots the named server + this worktree's session;
a second `up` is a clean no-op (idempotent).
It runs no strand command (substrate-only).

**Verdict:** `OK` / `WARN` / `FAIL`

---

### M3 -- Add (visible)

**Goal:** "Add a visible strand running a long-lived command and confirm it shows up as a pane."

**Watch:** `lyx reed add --cmd <long-running command>` returns an assigned `guid` + resolved `name`;
the pane exists and the layout applies.

**Verdict:** `OK` / `WARN` / `FAIL`

---

### M4 -- Status

**Goal:** "Ask reed what it currently knows about this session."

**Watch:** `lyx reed status` reports `session`, `socket`, and the strand from M3 with `live: true`.

**Verdict:** `OK` / `WARN` / `FAIL`

---

### M5 -- Add hidden

**Goal:** "Add a strand that should not show up as a pane, then confirm reed still knows about it."

**Watch:** `lyx reed add --anchor hidden --cmd ...` creates **no pane**;
`status` still lists the strand;
`lyx reed resume` **skips** it (hidden strands are pending, not dead -- still no pane after resume).
Note explicitly: surfacing a hidden strand is engine-API-only in v1 (there is no `lyx reed update` verb) -- do **not** attempt it,
and its absence is not a finding.

**Verdict:** `OK` / `WARN` / `FAIL`

---

### M6 -- Retired: top-band tiling removed with anchor:top

---

### M7 -- Attach (operator-assisted visual)

**Goal:** "Confirm the pane layout looks right by having the operator attach and look."

**Watch:** The agent pauses and instructs the operator to run `lyx reed attach` **in a second terminal**, visually confirm the pane layout, then detach;
the agent records the operator's verdict.
Rationale: the agent session owns the current terminal, so it cannot demonstrate or observe the takeover itself.
Also note: `attach` is a documented envelope exception -- pre-flight failures come as the JSON envelope;
a successful handover emits no JSON.
Neither behaviour is a finding.

**Verdict:** `OK` / `WARN` / `FAIL`

---

### M8 -- Resume semantics

**Goal:** "Confirm resume is a no-op when everything is already live, then confirm it recreates a pane that was killed out from under it."

**Watch:** `lyx reed resume` with all strands live leaves them untouched (no double launch).
Then kill one strand's pane via `tmux -L <socket> kill-pane -t <paneId>` (controlled exception;
`paneId` from `status`) and run `resume` again: the strand's pane is recreated and its stored resume/launch command replays.

**Verdict:** `OK` / `WARN` / `FAIL`

---

### M9 -- Crash-resume

**Goal:** "Simulate a server crash and confirm resume rebuilds everything."

**Watch:** `tmux -L <socket> kill-server` (controlled exception) simulates a server crash;
`lyx reed resume` boots a fresh server + session and replays every non-hidden strand's command into a new pane.

**Verdict:** `OK` / `WARN` / `FAIL`

---

### M10 -- Recursive remove

**Goal:** "Try to remove a strand that has children, first without and then with `--recursive`."

**Watch:** Removing a strand that has children without `--recursive` fails with `strand has children, use --recursive`;
with `--recursive` the removal cascades over the subtree and the result JSON lists every removed strand.
"No stray state" also applies to `remove`: like `down`, it waits for the removed panes' whole process subtree to exit before returning, so immediately after a `remove` no leftover shell keeps the worktree directory busy (the pre-`remove` `#{pane_pid}` values and their descendants are gone from `tasklist`).
Covered headlessly by `TestSmokeRemoveReapsRemovedPaneChildProcesses`.

**Verdict:** `OK` / `WARN` / `FAIL`

---

### M11 -- Down without stray state

**Goal:** "Tear the overlay down and confirm nothing is left behind."

**Watch:** `lyx reed down` kills the server and clears the worktree's strand state; `tmux -L <socket> ls` (controlled exception) confirms no server survives, and a follow-up `lyx reed status` reports the friendly no-session error rather than stale strands. "No stray state" means, concretely, that the instant `down` returns: (a) **no tmux process names this socket** — `down` force-reaps the server *and* its internal `__warm__` helper and confirms the socket is clear, because both carry the worktree as their cwd and a survivor would keep the worktree dir busy (check with `tasklist | findstr tmux` filtered to this `-L` socket — expect zero); and (b) **no pane process subtree survives** — `down` always reaps this session's whole pane process subtree before returning (checkable via the pre-`down` `#{pane_pid}` values and their descendants being gone from `tasklist`). Covered headlessly by `TestSmokeDownReapsPaneChildProcesses` and `TestSmokeDownLeavesNoTmuxOnSocket`.

> **Not a FAIL:** a `conhost.exe` (the OS ConPTY host tmux uses per pane) may linger with the worktree as its cwd — usually it exits on its own a beat later, but under heavy CPU saturation it can be orphaned and then holds the dir indefinitely. It is not a `#{pane_pid}` descendant and reed does not reap it — an OS console host is not stray *agent* state (the smoke harness kills hub-holding conhosts itself; see `deferHubRelease`). A held worktree dir is therefore not by itself a reed leak. Only a surviving **tmux** process or a live **pane shell** is a `FAIL` here.

**Verdict:** `OK` / `WARN` / `FAIL`

---

### M12 -- Layout survival under mixed adds

**Goal:** "Build a busy below-parent-only session -- a parent strand, a child under it, then a second child under the parent -- and confirm every strand still has its own live pane."

**Watch:** After all three `add`s, `lyx reed status` reports all three strands `live: true`, and `tmux -L <socket> list-panes` (controlled exception) shows exactly three panes with sane geometry: the parent shrunk to a `collapsed_strip_rows` strip once a child exists, and the deepest child dominant (the tallest, bottom-most pane).
A pane count below three, an empty pane list,
or a strand that flips to `live: false` after the next verb means a split/apply silently destroyed panes -- that is a `FAIL`, not cosmetics.

**Verdict:** `OK` / `WARN` / `FAIL`

---

### M13 -- Add after removing the last strand

**Goal:** "Remove the session's only strand, then add a fresh one and prove its command actually runs."

**Watch:** `lyx reed remove <guid>` on the sole strand succeeds;
the following `lyx reed add --cmd <long-running command>` returns a guid,
and the strand reads `live: true` in `status` **both immediately and again after one more verb** (e.g. `lyx reed up`) -- a strand that reads live once and then flips to `live: false` with an empty `paneId` means its fresh split silently failed to produce a working pane and its command never ran (`FAIL`).

**Verdict:** `OK` / `WARN` / `FAIL`

---

### M14 -- Attach visual (operator or capture-assisted)

**Goal:** "Confirm `lyx reed attach` actually renders the session's panes, not just that the command exits."

**Watch:** With the overlay up and at least one strand added, `lyx reed attach` from a **second terminal** shows the strands' panes and the tmux status bar;
`Ctrl+b d` detaches cleanly and the session keeps running (`lyx reed status` still lists it).
Operator-assisted like M7. (The automated layer covers this headlessly: `TestSmokeAttachRendersInsideHarnessTmuxPane` drives attach inside a harness tmux pane and asserts the rendered content -- so a `FAIL` here is a visual/UX issue, not a correctness gap.)

**Verdict:** `OK` / `WARN` / `FAIL`

---

### M15 -- Claude resume recall (needs a logged-in claude)

**Goal:** "Prove a real agent's conversation survives a crash: launch claude in a strand, give it a codeword, crash the server, resume, and confirm the codeword comes back."

**Watch:** `lyx reed add --cmd "claude '...codeword...'" --resume-cmd "claude --continue"`;
after claude answers, `tmux -L <socket> kill-server` (controlled exception) simulates a crash;
`lyx reed resume` replays `claude --continue` and the resumed pane recalls the codeword.
If `claude --continue` reports **"No conversation found"**, env hygiene failed to let the transcript persist -- that is a `FAIL` (the one Claude-adjacent thing reed owns).
Skip with a note if no claude is configured. (Covered headlessly by `TestSmokeClaudeResumeRecallsCodeword`.)

**Verdict:** `OK` / `WARN` / `FAIL`

### M16 -- Foreign pane in the reed session

**Goal:** "With the overlay up and **no strands added**, create a pane in the reed session behind reed's back, then run `lyx reed up` again and prove the session is still usable."

**Watch:** `tmux -L <socket> split-window -t <session>` (controlled exception) simulates an operator-split/foreign pane.
The follow-up `lyx reed up` must **not** destroy the session's pane set (`tmux -L <socket> list-panes` still shows panes — an empty pane list means an empty layout was applied and tmux wiped the window: `FAIL`), and that SAME `up` -- not a subsequent `add` -- is what **deterministically reaps** the foreign pane (the documented "reed owns the session window" policy, not a finding), since the untracked reap now fires from the alive header this `up` boots.
A subsequent `lyx reed add --cmd <long-running command>` must succeed and read `live: true` in `status` (a "session has no panes to split" error means the session became a zero-pane husk: `FAIL`) — what would be a `FAIL` is a *tracked* strand's pane disappearing instead of the foreign one.
Covered headlessly by `TestSmokeUpWithOnlyForeignPanesKeepsSessionUsable` (the `up` reap) and `TestSmokeForeignPaneIsReapedNotAdoptedByAdd` (the faithful M16 regression).

**Verdict:** `OK` / `WARN` / `FAIL`

---

### M17 -- Cross-worktree scope (down in one worktree spares its sibling)

**Goal:** "With the overlay up and a strand added in THIS worktree, confirm a **sibling worktree on the same hub** can run its own reed session at the same time,
and that tearing THIS worktree's overlay `down` leaves the sibling's session, panes, and agents untouched."

**Watch:** The tmux server is **per-hub** -- sibling worktrees under one hub share one server (one `-L <socket>`) but each owns a distinct session (named after its worktree basename).
So a second worktree on the same hub that runs `lyx reed up` + `lyx reed add` must appear as a **second session on the same socket, backed by the same single server** (no duplicate server spawned): `tmux -L <socket> ls` (controlled exception) lists both sessions, and `#{pid}` from each is identical.
The proof is what happens on `lyx reed down` in the FIRST worktree: it must kill **only its own session** (`tmux -L <socket> has-session -t <this-session>` now fails) while the sibling's session **stays live** -- `tmux -L <socket> has-session -t <sibling-session>` still succeeds, its pane is still present and not `pane_dead`,
and the shared server's `#{pid}` is unchanged.
The sibling's overlay dying because a `down` next door killed the shared server is a `FAIL`. (Only when the *last* session on the hub goes `down` is the server itself torn down -- M11's stray-state guarantee.)
Materializing a real second worktree is environment setup outside the reed surface, so the authoritative headless coverage is `TestSmokeDownInOneWorktreeLeavesSiblingSessionAlive`, which boots two clones on one hub, downs one, and asserts the other's session/pane/agent-subtree stay live on the still-single shared server;
treat this scenario as `OK` when that coverage holds and no sibling worktree is available to hand-drive.

**Verdict:** `OK` / `WARN` / `FAIL`

---

### M18 -- Below-parent mother/child shrink (operator-assisted visual)

**Goal:** "Add a below-parent *root* 'mother' strand running a plain, non-TUI status-line placeholder command (no `--anchor top` anywhere), confirm it renders full height while it has no live child, then add a Claude Code child under it via `--parent` and confirm the mother collapses to a compact strip while the child takes the bulk of the window."

**Watch:** With only the mother strand live, `lyx reed status` reports it `live: true` and `tmux -L <socket> list-panes` (controlled exception) shows its pane at full box height -- a childless below-parent mother rendering full-height is intended (not a bug to file).
After `lyx reed add --parent <mother-guid> --cmd claude ...` adds the Claude Code child, both strands read `live: true`, the mother's pane collapses to `collapsed_strip_rows`,
and the child's pane gets the rest of the window.
Confirm the mother's plain-text status line stays legible at the collapsed height -- this is the scenario the removed `TopBandRows` band-height override existed to protect against box-drawing-TUI corruption at a fixed 1-row band;
a plain-text line has no such corruption risk.

**Verdict:** `OK` / `WARN` / `FAIL`

---

### M19 -- Always-on header pane (operator console)

**Goal:** "With the overlay up, find the always-on header pane, confirm it actually shows its rendered text, and prove it survives everything the strand lifecycle throws at it — including the removal of the session's last strand and the death of its own process."

**Watch:** After `lyx reed up`, the session holds one extra pane beyond any strands: the header, physically topmost, whose **visible content** is the rendered header line (default template: `hub: <hub path>`) — a JSON error body, a bare shell prompt,
or an empty row where the text should be is a `FAIL`, not cosmetics (the pane merely being *alive* is not enough). `lyx reed status` must never list the header as a strand, and `up`'s `strands` count must exclude it.
Then: add a strand, remove it — the session **survives** on the header alone (pre-header reed destroyed the session with its last pane;
that teardown is the footgun this feature exists to remove) and a follow-up `add` still works, with the header back at its configured `height_rows` (default 1) and still topmost.
Finally kill the header's own process (`tmux -L <socket> list-panes -t "=<session>:" -F "#{pane_id} #{pane_pid}"` to find it, then kill that pid — controlled exception;
on POSIX use `kill -9`, interactive shells ignore TERM): intermediate verbs (`add`/`remove`) must keep working with sane pane geometry (a strand squeezed to 1 row means a stale header cell scrambled the layout: `FAIL`),
and the next `lyx reed up` must **heal** the header — a fresh, alive, topmost pane showing the rendered text again, with the corpse gone.
A `split header pane: ... no space for new pane` error from `up` is the wedged-heal regression: `FAIL`.

**Verdict:** `OK` / `WARN` / `FAIL`

---

### M20 -- Refusal of a worktree name tmux would silently rewrite

**Goal:** "Prove reed refuses, up front and loudly, a worktree whose directory name tmux would silently rewrite -- instead of hanging and stranding a session on the shared hub server."

**Watch:** Rename a worktree directory so its name carries one of tmux's three rewrite classes (`git worktree repair` after each;
controlled exception, restore the name afterwards): a `.` (e.g. `svc.v2`), a `\` (e.g. `mv svc2 'svc\v2'`, POSIX hosts only -- on Windows `\` cannot appear in a directory name), or a control character (e.g. a literal TAB via `mv svc3 "svc$(printf '\t')3"`).
`lyx reed up` (and every other reed verb) must refuse IMMEDIATELY (sub-second, no 20s hang) with a JSON error naming the offending character and the worktree directory to rename.
Afterwards `tmux -L <socket> ls` (controlled exception) must show NO new session -- neither the raw name nor a rewritten one (`svc_v2`, `svc\\v2`, `svc\t3`) -- squatting on the shared hub server.
A hang, a "did not materialize" timeout, or any leftover session is a `FAIL`.

**Verdict:** `OK` / `WARN` / `FAIL`

---

### M21 -- Reed never touches the operator's default tmux socket

**Goal:** "Prove a full reed session never starts a server on the operator's own default tmux socket."

**Watch:** Before starting, note whether `tmux ls` (default socket, controlled exception) reports a server.
Run a normal cycle -- `up`, `add`, `status`, `resume`, `down`.
Afterwards `tmux ls` must report exactly what it did before: if there was no default-socket server, there still is none ("no server running").
A new server on the default socket (the capability probe's historical leak, R2-F6) is a `FAIL`.

**Verdict:** `OK` / `WARN` / `FAIL`

---

### M22 -- Recovery from a scrubbed reed.json while the session is up

**Goal:** "Prove a lost `.lyx/reed.json` does not permanently wedge a worktree whose tmux session is still running."

**Watch:** `up`, then `add` one strand, so the one-row header band at the top of the window is actually laid out.
Delete `.lyx/reed.json` (or run `git clean -xdf` in the worktree -- `.lyx` is never-tracked machine-local scratch, so this is a sanctioned operator action) while leaving the session up.
`lyx reed up` must then SUCCEED and converge IMMEDIATELY, not one verb late: the old header and the now-untracked strand pane are both reaped in this same `up`, and the freshly rebuilt header ends up alone, full-height, at the very top of the window, with nothing below it -- a full-height header with an empty strand stack is the expected `OK` here, not a defect.
A subsequent `lyx reed add` must run its command for real (check the pane, and check the process exists), not type it onto a pane that is already busy.
An `up` that fails with `no space for new pane`, a header that does not end up topmost, or an added strand whose command never runs is a `FAIL` -- and so now is a surviving OLD header pane or a surviving ORPHANED strand pane left behind by that `up`.

**Verdict:** `OK` / `WARN` / `FAIL`

---

### M23 -- A stale reed.json is never mistaken for live strands

**Goal:** "Prove a `reed.json` older than the tmux session now running reports its strands honestly, instead of claiming a dead strand is alive."

**Watch:** `up`, `add` one strand, then copy `.lyx/reed.json` somewhere safe.
`down`, then `up` again -- the server died, so tmux's pane ids restart at `%0` and the saved file's ids now name panes belonging to the NEW session.
Copy the saved `reed.json` back over `.lyx/reed.json` (this is exactly what a backup tool, a `git stash pop`, or a hand-copied `.lyx` does).
`lyx reed status` must report the strand `live: false` with no pane id -- NOT `live: true` against whatever pane happens to hold that id now.
`lyx reed resume` must then rebuild it (`resumed` at least 1) and the strand's command must genuinely be running afterwards (check the process, not just the pane).
A `live: true` for a process that does not exist, or a `resumed: 0` that leaves the strand unrecovered, is a `FAIL`.

**Verdict:** `OK` / `WARN` / `FAIL`

---

### M24 -- A renamed worktree refuses instead of double-launching

**Goal:** "Prove renaming a worktree while its session is up does not silently start a second copy of every strand and orphan the first."

**Watch:** In a worktree, `up` and `add` one strand.
Rename the worktree directory while that session is still running (`git worktree repair` afterwards;
controlled exception, restore the name when done).
`lyx reed resume` (and `up`, and `add`, and `status`, and `attach`) in the renamed directory must REFUSE, with a JSON error naming the old session, the socket, and the exact `kill-session` command that clears it.
`status` above all: it is the verb an operator reaches for first, and a plain "no reed session; run `lyx reed resume`" there is a `FAIL` -- it names a remedy that itself refuses, sending the operator round a loop.
`tmux -L <socket> ls` (controlled exception) must then show only the ORIGINAL session -- the refusal must not have deposited a second one -- and the strand's command must still be running exactly once, not twice.
Running the `kill-session` the error names, then `lyx reed resume` again, must succeed normally.
A silent success, two copies of the strand process, a second session left on the socket, or a refusal the operator cannot escape is a `FAIL`.

**Verdict:** `OK` / `WARN` / `FAIL`

---

### M25 -- Down names the session it abandons

**Goal:** "Prove the one escape from the refusal that needs no raw tmux says out loud what it leaves behind."

**Watch:** Set up M24's state again (a worktree renamed while its session is up, with one strand running), but this time escape with `lyx reed down` in the renamed directory instead of the `kill-session` M24 uses.
`down` must SUCCEED -- it is the only route out of the refusal for an operator who can run `lyx` but not raw `tmux` -- and its JSON must carry `abandonedSession` naming the still-running old session.
`tmux -L <socket> ls` (controlled exception) must still show that session, and the strand's command must still be running: `down` reports the abandonment, it never kills a session it cannot prove is this worktree's own (it may be a sibling worktree's live work).
An `ok: true` with no `abandonedSession` key is a `FAIL` -- `down` deletes `reed.json`, so that key is the last thing that ever names the orphan.
A `down` that kills the old session is also a `FAIL`.
Then run an ordinary `lyx reed up` / `down` cycle in a normal worktree and confirm `abandonedSession` is ABSENT there -- the key must be signal, not noise.

**Verdict:** `OK` / `WARN` / `FAIL`

---

### M26 -- Resize self-heal (operator-assisted visual)

**Covers:** reed

**Goal:** "Prove a live terminal resize re-applies the planned layout on its own, in both directions, with no `lyx` command run."

**Watch:** The agent pauses and instructs the operator to attach with `lyx reed attach` **in a second terminal** against a session holding a header pane and at least two strands.
Confirm the header is exactly `header.height_rows` tall and the strand budgets look right.
Then have the operator **drag the terminal window larger** and confirm the layout re-applies within about a second, with no `lyx` command run.
Then have the operator **drag it smaller** and confirm the same.
The shrink direction is the non-negotiable half of this scenario -- it is the one SIGWINCH misses entirely, and a watcher that only self-heals on growth must be reported as a `FAIL` here, not a `WARN`.
A header that grows past its configured row count, or a bottom strand squeezed below `min_full_rows`, is also a `FAIL`.
The operator must also confirm the cursor did NOT jump to another pane across either resize (the focus-steal regression), and that typing into a pane before a resize leaves that same pane focused after it.
Rationale: the agent session owns the current terminal, so it cannot demonstrate or observe a live client resize itself.

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
M16: <OK|WARN|FAIL> -- <one-line note if not OK>
M17: <OK|WARN|FAIL> -- <one-line note if not OK>
M18: <OK|WARN|FAIL> -- <one-line note if not OK>
M19: <OK|WARN|FAIL> -- <one-line note if not OK>
M20: <OK|WARN|FAIL> -- <one-line note if not OK>
M21: <OK|WARN|FAIL> -- <one-line note if not OK>
M22: <OK|WARN|FAIL> -- <one-line note if not OK>
M23: <OK|WARN|FAIL> -- <one-line note if not OK>
M24: <OK|WARN|FAIL> -- <one-line note if not OK>
M25: <OK|WARN|FAIL> -- <one-line note if not OK>
M26: <OK|WARN|FAIL> -- <one-line note if not OK>

sandbox-report.json written: <count of WARN/FAIL items>
```

`./sandbox-report.json` must be written before the session ends, per the Capturing findings section above -- with `items: []` when every scenario was `OK`.

## Notes

- Warp/weft scenarios stay in `SANDBOX-CORE-SUITE.md`;
  this suite grows with reed (windows for clusters, daemon) -- add `M` scenarios here, not in the main suite.
