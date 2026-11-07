# SANDBOX-REED-SUITE -- lym reed black-box agent suite

## What this is

A structured test-loop for exercising `lym reed` against a **live tmux server** in the sandbox Hub's Fabric repo.
The value here is partly **visual**: windows opening, panes staying put.
This is not an automated suite: an agent drives it, and an operator watches.

## Pre-conditions

Before starting a session:

1. **Deploy a fresh dev binary.**
   Run `deploy-dev` to build `lym.exe` from current source into `.dev-bin`.
   The suite resolves `.dev-bin` itself and prepends it to the agent's PATH.
   No PATH setup is needed, and production `lym` stays untouched.
   The fingerprint header's `Source: dev` line confirms the dev build is under test.
   The deployed binary is a snapshot: re-deploy after any source change you want to test.
   Once a pane exists, run these checks:
   - `command -v lym` (`where lym` on Windows) names the spawning binary, not the production install.
     This holds in the pane shell of a strand whose command does not start a fresh shell.
     A strand whose command starts a fresh shell re-runs the operator's rc.
     Examples are `--cmd bash` and the Bash tool of a plain `--cmd claude`.
     Its bare `lym` then follows that rc's `PATH`, while `LYM_BIN` still names the spawning binary.
   - `LYM_BIN` is set inside that pane and names the same binary.
   These are live-substrate facts no `go test` can prove, so they are pre-condition checks rather than assertions.
   See `SANDBOX-SHUTTLE-SUITE.md` for the agent-pane half.
   This suite's strands run operator-supplied commands and spawn no agent pane.
   A `claude`-resolution check therefore has nothing to exercise here.
2. **Materialize the hub.**
   Run `sandbox/build.cmd`, or `sandbox/build.cmd -reset` to start clean.
   The session cwd is the Hub's Fabric repo root.
3. **Live-tmux requirement.** tmux (or the Windows tmux port) on PATH and PowerShell 7 present.
   If tmux or pwsh is unavailable, **note that as the session outcome rather than treating it as a reed defect**.
   The `**Covers:** reed` tags satisfy the sandbox coverage guard (`sandbox_coverage_test.go`) even then.

## Black-box rule

**The agent under test works exclusively inside the Hub's Fabric repo (`lym-test-LYMHUB/lym-test`).
It tests `lym.exe` as a black box -- exactly as a real user with only the binary on PATH.
It must not look for, read, or reason about the lym source tree.
No peeking at `C:\Code\loomyard\` or any other path outside the Hub.**

Discover the command surface via `lym reed`, `lym reed <subcommand>`, and `lym reed <subcommand> --help` alone.
Do not use documentation outside the Hub.

### Controlled tmux exceptions

Two sanctioned deviations from the pure black-box rule:

- **(a) Direct `tmux -L <socket>` verbs** are allowed **only** for crash simulation and window/stray-state verification.
  `<socket>` is taken from `lym reed status` output.
  Its JSON result carries `session`, `socket`, and `strands[]` with `guid`/`name`/`paneId`/`live`.
- **(b) Scenario M7 (attach) is operator-assisted** -- see M7 below.

## Fingerprint header

The launcher prepends a "binary under test" fingerprint block when it copies this file into the Hub's Fabric repo.
The fingerprint records the absolute path, size, modification time, and a short SHA-256 of the `lym.exe` binary at launch.

The same fingerprint gives the report its provenance.
A separate fetch step, run after the session, stamps it into `meta.fingerprint` of the fetched `sandbox-report.json`.
A maintainer can then reproduce the exact binary that produced each finding.
The agent does not need to transcribe the fingerprint anywhere itself.

## How to run a scenario

For each scenario below:

- Read the **Goal** -- it names the task, not the commands.
  Discover the commands via `lym reed`, `lym reed <subcommand>`, and `--help` flags (M0 ethos).
- **Watch** what lym does.
  Note where it stalls, guesses wrong, or hits an error.
- Record the outcome per the verdict buckets: `OK` (worked) / `WARN` (rough edge) / `FAIL` (broke).

## Verdict key

- `OK`   -- completed without friction
- `WARN` -- completed but with confusion, awkward UX, or a non-fatal error
- `FAIL` -- did not complete;
  lym broke, panicked, or gave wrong output

## Capturing findings

After all scenarios are run, write **all** `WARN`/`FAIL` findings to `./sandbox-report.json` in the Fabric-repo cwd.
Use this exact schema.
**Always write the file, even when there are no `WARN`/`FAIL` findings**: `items` is then an empty array.

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
- `ref` is the scenario id (`M0`, `M1b`, and so on).
- `title` is a short one-line summary.
- `body` folds the detail, repro steps, and verdict into one markdown string.

Write only `source` and `items`.
A separate fetch step, run after the session, stamps `meta`, including the binary fingerprint.
Confine all free text to the `title`/`body` string fields so the JSON stays well-formed.

## Scenarios

### M0 -- Discovery

**Goal:** "You have `lym` on PATH and nothing else inside this repo.
Find out what `lym reed` can do and report the full command tree."

**Watch:** Does `lym reed` list every subcommand with an accurate `Short`? Does each `--help` explain itself?

**Verdict:** `OK` / `WARN` / `FAIL`

---

### M1 -- Pre-up ergonomics (remove still refuses)

**Goal:** "From a fresh state (no reed session running), try to remove a strand.
See how reed tells you what to do first."

**Watch:** `lym reed remove <guid>` must fail with the JSON-envelope error `no reed session; run "lym reed up"`.
That message is the `OK` outcome, not a finding.

**Verdict:** `OK` / `WARN` / `FAIL`

---

### M1b -- Add self-heals a cold worktree

**Goal:** "From a fresh state (no reed session running), add a strand.
See whether reed boots the session for you instead of refusing."

**Watch:** `lym reed add --cmd ...` against a fresh state boots this worktree's session, then adds the strand.
The boot has `up`'s own semantics: a bare substrate, with no persisted strand relaunched.
A session coming up and the strand landing is the `OK` outcome, not a finding.

**Verdict:** `OK` / `WARN` / `FAIL`

---

### M2 -- Up

**Covers:** reed

**Goal:** "Bring the reed overlay online for this worktree."

**Watch:** `lym reed up` boots the named server + this worktree's session;
a second `up` is a clean no-op (idempotent).
It runs no strand command (substrate-only).

**Verdict:** `OK` / `WARN` / `FAIL`

---

### M3 -- Add (visible)

**Goal:** "Add a visible strand running a long-lived command and confirm it shows up as a pane."

**Watch:** `lym reed add --cmd <long-running command>` returns an assigned `guid` + resolved `name`;
the pane exists in a window of its own.

**Verdict:** `OK` / `WARN` / `FAIL`

---

### M4 -- Status

**Goal:** "Ask reed what it currently knows about this session."

**Watch:** `lym reed status` reports `session`, `socket`, and the strand from M3 with `live: true`.

**Verdict:** `OK` / `WARN` / `FAIL`

---

### M5 -- Add hidden

**Goal:** "Add a strand that should not show up as a pane, then confirm reed still knows about it."

**Watch:** `lym reed add --anchor hidden --cmd ...` creates **no pane**;
`status` still lists the strand;
`lym reed resume` **skips** it: a hidden strand is pending, not dead, so there is still no pane after resume.
Surfacing a hidden strand is possible only through the engine API, since there is no `lym reed update` verb.
Do **not** attempt it; its absence is not a finding.

**Verdict:** `OK` / `WARN` / `FAIL`

---

### M6 -- Retired: top-band tiling removed with anchor:top

---

### M7 -- Attach (operator-assisted visual)

**Goal:** "Confirm the session's windows look right by having the operator attach and look."

**Watch:** The agent pauses and instructs the operator to run `lym reed attach` **in a second terminal**.
The operator confirms the window list visually and detaches.
The agent records the operator's verdict.
The agent session owns the current terminal, so it cannot demonstrate or observe the takeover itself.
`attach` is a documented envelope exception: pre-flight failures arrive as the JSON envelope.
A successful handover emits no JSON.
Neither behaviour is a finding.

**Verdict:** `OK` / `WARN` / `FAIL`

---

### M8 -- Resume semantics

**Goal:** "Confirm resume is a no-op when everything is already live.
Then confirm it recreates a pane that was killed out from under it."

**Watch:** `lym reed resume` with all strands live leaves them untouched (no double launch).
Then kill one strand's pane with `tmux -L <socket> kill-pane -t <paneId>` (controlled exception).
Take `paneId` from `status`.
Run `resume` again: the strand's pane is recreated and its stored resume/launch command replays.

**Verdict:** `OK` / `WARN` / `FAIL`

---

### M9 -- Crash-resume

**Goal:** "Simulate a server crash and confirm resume rebuilds everything."

**Watch:** `tmux -L <socket> kill-server` (controlled exception) simulates a server crash;
`lym reed resume` boots a fresh server + session and replays every non-hidden strand's command into a new pane.

**Verdict:** `OK` / `WARN` / `FAIL`

---

### M10 -- Recursive remove

**Goal:** "Try to remove a strand that has children, first without and then with `--recursive`."

**Watch:** Removing a strand that has children without `--recursive` fails with `strand has children, use --recursive`;
with `--recursive` the removal cascades over the subtree and the result JSON lists every removed strand.
"No stray state" also applies to `remove`.
Like `down`, it waits for the removed panes' whole process subtree to exit before returning.
Immediately after a `remove`, no leftover shell keeps the worktree directory busy:
the pre-`remove` `#{pane_pid}` values and their descendants are gone from `tasklist`.
`TestSmokeTeardown/RemoveReapsRemovedPaneChildProcesses` covers this headlessly.

**Verdict:** `OK` / `WARN` / `FAIL`

---

### M11 -- Down without stray state

**Goal:** "Tear the overlay down and confirm nothing is left behind."

**Watch:** `lym reed down` kills the server and clears the worktree's strand state.
`tmux -L <socket> ls` (controlled exception) confirms no server survives.
A follow-up `lym reed status` reports the friendly no-session error rather than stale strands.
"No stray state" means, concretely, that the instant `down` returns:
(a) **no tmux process names this socket.**
`down` force-reaps the server *and* its internal `__warm__` helper and confirms the socket is clear.
Both carry the worktree as their cwd, so a survivor would keep the worktree dir busy.
Check with `tasklist | findstr tmux` filtered to this `-L` socket, and expect zero.
(b) **no pane process subtree survives.**
`down` always reaps this session's whole pane process subtree before returning.
Check that the pre-`down` `#{pane_pid}` values and their descendants are gone from `tasklist`.
`TestSmokeTeardown/DownReapsPaneChildProcesses` and `TestSmokeTeardown/DownLeavesNoTmuxOnSocket` cover this headlessly.

> **Not a FAIL:** a `conhost.exe` (the OS ConPTY host tmux uses per pane) may linger with the worktree as its cwd.
> It usually exits on its own a beat later, but under heavy CPU saturation it can be orphaned and then holds the dir indefinitely.
> It is not a `#{pane_pid}` descendant, and reed does not reap it: an OS console host is not stray *agent* state.
> The smoke harness kills hub-holding conhosts itself (see `deferHubRelease`).
> A held worktree dir is therefore not by itself a reed leak.
> Only a surviving **tmux** process or a live **pane shell** is a `FAIL` here.

**Verdict:** `OK` / `WARN` / `FAIL`

---

### M12 -- Own-window survival under successive adds

**Goal:** "Add a parent strand, a child under it, then a second child under the parent, all with the default anchor.
Confirm every strand still has its own live pane."

**Watch:** After all three `add`s, `lym reed status` reports all three strands `live: true`.
`tmux -L <socket> list-panes -s` (controlled exception) shows three strand panes, each in a window of its own.
Selvage sits alone in the shell window beside them.
A pane count below three, an empty pane list, or a strand flipping to `live: false` on the next verb is a `FAIL`.
It means an add silently destroyed panes, which is not cosmetic.

**Verdict:** `OK` / `WARN` / `FAIL`

---

### M13 -- Add after removing the last strand

**Goal:** "Remove the session's only strand, then add a fresh one and prove its command runs."

**Watch:** `lym reed remove <guid>` on the sole strand succeeds.
The following `lym reed add --cmd <long-running command>` returns a guid.
The strand must read `live: true` in `status` **both immediately and again after one more verb** (e.g. `lym reed up`).
A strand that reads live once and then flips to `live: false` with an empty `paneId` is a `FAIL`.
It means its fresh split silently failed to produce a working pane, and its command never ran.

**Verdict:** `OK` / `WARN` / `FAIL`

---

### M14 -- Attach visual (operator or capture-assisted)

**Goal:** "Confirm `lym reed attach` renders the session's panes, not just that the command exits."

**Watch:** With the overlay up and at least one strand added, run `lym reed attach` from a **second terminal**.
It shows the strands' windows and the tmux status bar.
`Ctrl+b d` detaches cleanly, and the session keeps running (`lym reed status` still lists it).
This scenario is operator-assisted, like M7.
`TestSmokeLifecycle/AttachRendersInsideHarnessPane` covers it headlessly.
It drives attach inside a harness tmux pane and asserts the rendered content.
A `FAIL` here is therefore a visual/UX issue, not a correctness gap.

**Verdict:** `OK` / `WARN` / `FAIL`

---

### M15 -- Claude resume recall (needs a logged-in claude)

**Goal:** "Prove a real agent's conversation survives a crash.
Launch claude in a strand, give it a codeword, crash the server, resume, and confirm the codeword comes back."

**Watch:** `lym reed add --cmd "claude '...codeword...'" --resume-cmd "claude --continue"`;
after claude answers, `tmux -L <socket> kill-server` (controlled exception) simulates a crash;
`lym reed resume` replays `claude --continue` and the resumed pane recalls the codeword.
If `claude --continue` reports **"No conversation found"**, env hygiene failed to let the transcript persist.
That is a `FAIL`: it is the one Claude-adjacent thing reed owns.
Skip with a note if no claude is configured.
`TestSmokeClaudeResumeRecallsCodeword` covers this headlessly.

**Verdict:** `OK` / `WARN` / `FAIL`

### M16 -- Foreign pane in the reed session

**Goal:** "With the overlay up and **no strands added**, create a pane in the reed session behind reed's back.
Then run `lym reed up` again and prove the session is still usable."

**Watch:** Run `tmux -L <socket> split-window -t <session>` (controlled exception).
This simulates an operator-split or foreign pane.
The follow-up `lym reed up` must **not** destroy the session's pane set.
`tmux -L <socket> list-panes` must still show panes: an empty pane list means tmux wiped the window (`FAIL`).
That SAME `up`, not a subsequent `add`, **deterministically reaps** the foreign pane.
This is the documented "reed owns the session window" policy, not a finding.
The untracked reap fires from the alive Selvage this `up` boots.
A subsequent `lym reed add --cmd <long-running command>` must succeed and read `live: true` in `status`.
A "session has no panes to split" error means the session became a zero-pane husk (`FAIL`).
A *tracked* strand's pane disappearing instead of the foreign one is also a `FAIL`.
`TestSmokeLifecycle/UpWithOnlyForeignPanesKeepsSessionUsable` covers the `up` reap headlessly.
`TestSmokeLifecycle/ForeignPaneIsReapedNotAdoptedByAdd` is the faithful M16 regression.

**Verdict:** `OK` / `WARN` / `FAIL`

---

### M17 -- Cross-worktree scope (down in one worktree spares its sibling)

**Goal:** "Bring the overlay up and add a strand in THIS worktree.
Confirm a **sibling worktree on the same hub** can run its own reed session at the same time.
Then confirm that `down` in THIS worktree leaves the sibling's session, panes, and agents untouched."

**Watch:** The tmux server is **per-hub**: sibling worktrees under one hub share one server (one `-L <socket>`).
Each owns a distinct session, named after its worktree basename.
Running `lym reed up` and `lym reed add` in a second worktree on the hub must add a **second session on the same socket**.
The same single server must back it, with no duplicate server spawned.
`tmux -L <socket> ls` (controlled exception) lists both sessions, and `#{pid}` from each is identical.
The proof is `lym reed down` in the FIRST worktree.
It must kill **only its own session**, so `tmux -L <socket> has-session -t <this-session>` now fails.
The sibling's session **stays live**: `tmux -L <socket> has-session -t <sibling-session>` still succeeds.
The sibling's pane is still present and not `pane_dead`, and the shared server's `#{pid}` is unchanged.
The sibling's overlay dying because a `down` next door killed the shared server is a `FAIL`.
Only when the *last* session on the hub goes `down` is the server itself torn down (M11's stray-state guarantee).
Materializing a real second worktree is environment setup outside the reed surface.
The authoritative headless coverage is `TestSmokeTeardown/DownInOneWorktreeLeavesSiblingSessionAlive`.
It boots two clones on one hub and downs one.
It asserts the other's session, pane, and agent subtree stay live on the still-single shared server.
Treat this scenario as `OK` when that coverage holds and no sibling worktree is available to hand-drive.

**Verdict:** `OK` / `WARN` / `FAIL`

---

### M18 -- Retired: below-parent mother/child shrink removed with the shell-window stack

---

### M19 -- Always-on Selvage pane

**Goal:** "With the overlay up, find the always-on Selvage pane and confirm it is a genuine shell prompt.
Confirm the strand bar lives in the status bar instead.
Then prove Selvage survives everything the strand lifecycle throws at it.
That includes the removal of the session's last strand and the death of its own process."

**Watch:** After `lym reed up`, the session holds one pane beyond any strands: Selvage, physically **bottom**-most.
Its **visible content** must be a bare **shell prompt**.
A JSON error body, an empty row, or anything other than an idle shell prompt is a `FAIL`, not cosmetics.
The pane merely being *alive* is not enough.
Separately, `tmux -L <socket> display-message -p '#{status}'` (controlled exception) must read `2`.
The attached client (operator-assisted, as in M7) must show the `shell` button on line 1.
It must show the worktree's session on line 2.
A missing bar there is a `FAIL` in its own right.
`lym reed status` must never list Selvage as a strand, and `up`'s `strands` count must exclude it.
Then add a strand and remove it.
The session **survives** on Selvage alone.
A follow-up `add` still works, with Selvage still alone in the shell window.
A session with no permanent pane would die with its last strand;
that teardown is the footgun this feature exists to remove.
Finally, kill Selvage's own shell process (controlled exception).
Find its pid with `tmux -L <socket> list-panes -t "=<session>:" -F "#{pane_id} #{pane_pid}"`.
On POSIX use `kill -9`, since interactive shells ignore TERM.
Intermediate verbs (`add`/`remove`) must keep working, each strand keeping its own window.
A strand pane found in the shell window beside the corpse is a `FAIL`.
The next `lym reed up` must **heal** Selvage.
That means a fresh, alive, bottom-most pane with an idle shell prompt, and the corpse gone.
A `split Selvage pane: ... no space for new pane` error from `up` is the wedged-heal regression (`FAIL`).

**Verdict:** `OK` / `WARN` / `FAIL`

---

### M20 -- Refusal of a worktree name tmux would silently rewrite

**Goal:** "Prove reed refuses, up front and loudly, a worktree whose directory name tmux would silently rewrite.
The alternative is a hang that strands a session on the shared hub server."

**Watch:** Rename a worktree directory so its name carries one of tmux's three rewrite classes (controlled exception).
Run `git worktree repair` after each rename, and restore the name afterwards.
The classes are a `.` (e.g. `svc.v2`);
a `\` (e.g. `mv svc2 'svc\v2'`, POSIX hosts only, since `\` cannot appear in a directory name on Windows);
and a control character (e.g. a literal TAB via `mv svc3 "svc$(printf '\t')3"`).
`lym reed up` and every other reed verb must refuse IMMEDIATELY, sub-second with no 20s hang.
The refusal is a JSON error naming the offending character and the worktree directory to rename.
Afterwards `tmux -L <socket> ls` (controlled exception) must show NO new session squatting on the shared hub server.
This holds for the raw name and for a rewritten one (`svc_v2`, `svc\\v2`, `svc\t3`).
A hang, a "did not materialize" timeout, or any leftover session is a `FAIL`.

**Verdict:** `OK` / `WARN` / `FAIL`

---

### M21 -- Reed never touches the operator's default tmux socket

**Goal:** "Prove a full reed session never starts a server on the operator's own default tmux socket."

**Watch:** Before starting, note whether `tmux ls` (default socket, controlled exception) reports a server.
Run a normal cycle -- `up`, `add`, `status`, `resume`, `down`.
Afterwards `tmux ls` must report exactly what it did before.
If there was no default-socket server, there still is none ("no server running").
A new server on the default socket is a `FAIL`.

**Verdict:** `OK` / `WARN` / `FAIL`

---

### M22 -- Recovery from a scrubbed reed.json while the session is up

**Goal:** "Prove a lost `.lym/reed.json` does not permanently wedge a worktree whose tmux session is still running."

**Watch:** `up`, then `add` one strand, so a strand window exists beside the shell window.
Delete `.lym/reed.json` (or run `git clean -xdf` in the worktree) while leaving the session up.
`.lym` is never-tracked machine-local scratch, so this is a sanctioned operator action.
`lym reed up` must then SUCCEED and converge IMMEDIATELY, not one verb late.
The old Selvage pane and the now-untracked strand pane are both reaped in this same `up`.
The freshly rebuilt Selvage ends up alone, full-height, filling the whole window, with nothing above it.
That is tmux's own tiling rather than something reed computes, and it is the expected `OK` here, not a defect.
A subsequent `lym reed add` must run its command for real, not type it onto a pane that is already busy.
Check the pane, and check that the process exists.
An `up` that fails with `no space for new pane` is a `FAIL`.
So is a Selvage that does not end up bottom-most or an added strand whose command never runs.
So is a surviving OLD Selvage pane or a surviving ORPHANED strand pane left behind by that `up`.

**Verdict:** `OK` / `WARN` / `FAIL`

---

### M23 -- A stale reed.json is never mistaken for live strands

**Goal:** "Prove a `reed.json` older than the tmux session now running reports its strands honestly.
It must not claim a dead strand is alive."

**Watch:** `up`, `add` one strand, then copy `.lym/reed.json` somewhere safe.
`down`, then `up` again.
The server died, so tmux's pane ids restart at `%0`.
The saved file's ids now name panes belonging to the NEW session.
Copy the saved `reed.json` back over `.lym/reed.json`.
This is exactly what a backup tool, a `git stash pop`, or a hand-copied `.lym` does.
`lym reed status` must report the strand `live: false` with no pane id.
It must NOT report `live: true` against whatever pane happens to hold that id now.
`lym reed resume` must then rebuild it (`resumed` at least 1), and the strand's command must be running afterwards.
Check the process, not just the pane.
A `live: true` for a process that does not exist is a `FAIL`.
So is a `resumed: 0` that leaves the strand unrecovered.

**Verdict:** `OK` / `WARN` / `FAIL`

---

### M24 -- A renamed worktree refuses instead of double-launching

**Goal:** "Prove renaming a worktree while its session is up does not silently start a second copy of every strand.
The first copy must not be orphaned."

**Watch:** In a worktree, `up` and `add` one strand.
Rename the worktree directory while that session is still running (controlled exception).
Run `git worktree repair` afterwards, and restore the name when done.
`lym reed resume` (and `up`, `add`, `status`, and `attach`) in the renamed directory must REFUSE.
The refusal is a JSON error naming the old session, the socket, and the exact `kill-session` command that clears it.
`status` matters above all, since it is the verb an operator reaches for first.
A plain "no reed session; run `lym reed resume`" there is a `FAIL`.
It names a remedy that itself refuses, sending the operator round a loop.
`tmux -L <socket> ls` (controlled exception) must then show only the ORIGINAL session.
The refusal must not have deposited a second one.
The strand's command must still be running exactly once, not twice.
Running the `kill-session` the error names, then `lym reed resume` again, must succeed normally.
A silent success is a `FAIL`.
So is two copies of the strand process, a second session left on the socket, or a refusal the operator cannot escape.
After that successful `resume`, inspect the hub root: it must contain no directory named after the pre-rename worktree.
A stray directory there is a `FAIL`, since the point of the refusal is that nothing gets conjured under the old name.

**Verdict:** `OK` / `WARN` / `FAIL`

---

### M25 -- Down names the session it abandons

**Goal:** "Prove the one escape from the refusal that needs no raw tmux says out loud what it leaves behind."

**Watch:** Set up M24's state again: a worktree renamed while its session is up, with one strand running.
This time escape with `lym reed down` in the renamed directory instead of the `kill-session` M24 uses.
`down` must SUCCEED.
It is the only route out of the refusal for an operator who can run `lym` but not raw `tmux`.
Its JSON must carry `abandonedSession` naming the still-running old session.
`tmux -L <socket> ls` (controlled exception) must still show that session.
The strand's command must still be running.
`down` reports the abandonment and never kills a session it cannot prove is this worktree's own;
it may be a sibling worktree's live work.
An `ok: true` with no `abandonedSession` key is a `FAIL`.
`down` deletes `reed.json`, so that key is the last thing that ever names the orphan.
A `down` that kills the old session is also a `FAIL`.
Then run an ordinary `lym reed up` / `down` cycle in a normal worktree and confirm `abandonedSession` is ABSENT there.
The key must be signal, not noise.
Inspect the hub root as well, but not immediately: wait a few watchdog cycles first.
`down` leaves the abandoned session running, and checking too early would mask a daemon that resumed leaking.
Once that wait has elapsed, the hub root must contain no directory named after the pre-rename worktree.
A stray directory there is a `FAIL`.

**Verdict:** `OK` / `WARN` / `FAIL`

---

### M26 -- Retired: resize self-heal removed with the shell-window stack

---

### M27 -- Repeated add --if-absent is idempotent

**Covers:** reed

**Goal:** "Prove a repeated `add --if-absent` never stacks a duplicate strand.
This holds for each of the three reopen shapes."

**Watch:** `up`, then `add --if-absent --cmd <claude> --name claude --focus`.
Record the returned guid.
Run the identical `add --if-absent` a second time while that strand's pane is still alive.
It must add nothing and report the SAME guid, and `status` must show exactly one strand under that name.
Kill the strand's pane, not with `remove`: the strand must stay tracked, and only its pane must die.
Run the identical `add --if-absent` a third time.
It must relaunch the SAME guid rather than adding a second strand.
`status` must then report that guid `live: true`.
Finally, `add --if-absent` naming a HIDDEN strand (added with `--anchor hidden`) must add nothing.
It must report the hidden strand's own guid.
A second strand appearing under the same name at any point is a `FAIL`.
So is a guid that changes across the reopen or a `live: false` after the third add's relaunch.

**Verdict:** `OK` / `WARN` / `FAIL`

---

### M28 -- Switch steps a client through the sessions

**Covers:** reed

**Goal:** "Prove `lym reed switch --next` and `--prev` move an attached client to the neighbouring session.
Both must wrap at either end."

**Watch:** Bring two worktrees' sessions up, running `up` in each so one server holds both sessions.
Attach a client to the first (operator-assisted, as in M7).
Read the server's socket path with `tmux -L <socket> display -p '#{socket_path}'` (controlled exception).
Read the client's name with `tmux -L <socket> list-clients -F '#{client_name}'`.
Then run `lym reed switch --next --socket <path> --client <name> --tmux <tmux binary>`.
It must print nothing and exit 0.
The client must land on the neighbouring session, in the order the sessions were created.
Run `--next` again to see the client wrap to the first session, then `--prev` to wrap back.
Run it once with neither flag and once with both: each must exit non-zero with a JSON error.
A client that does not move is a `FAIL`.
So is a move to a non-neighbouring session, output on success, or a bare-flag run that exits 0.

**Verdict:** `OK` / `WARN` / `FAIL`

## Session log format

After running all scenarios, record a short session summary:

```
Date: <YYYY-MM-DD>
Binary fingerprint: <copy from the header above>

M0: <OK|WARN|FAIL> -- <one-line note if not OK>
M1: <OK|WARN|FAIL> -- <one-line note if not OK>
M1b: <OK|WARN|FAIL> -- <one-line note if not OK>
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
M27: <OK|WARN|FAIL> -- <one-line note if not OK>
M28: <OK|WARN|FAIL> -- <one-line note if not OK>

sandbox-report.json written: <count of WARN/FAIL items>
```

`./sandbox-report.json` must be written before the session ends, per the Capturing findings section above -- with `items: []` when every scenario was `OK`.

## Notes

- Warp/weft scenarios stay in `SANDBOX-CORE-SUITE.md`;
  this suite grows with reed (windows for clusters, daemon) -- add `M` scenarios here, not in the main suite.
