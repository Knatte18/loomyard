# SANDBOX-CREEL-SUITE -- lym creel black-box agent suite

## What this is

A structured test-loop that exercises `lym creel` in the sandbox Hub's Fabric repo.
It runs against a **live tmux server and a logged-in claude**.
Like `SANDBOX-REED-SUITE.md`, the suite is partly **visual**:
the operator sees mail reminders and injected lines typed into a strand's pane, and a message's events in the log.
The suite is not automated: an agent drives it and an operator watches.
Every strand the suite adds comes from `lym reed add` and has no provider, so it keeps the pane path.
That is why its typed notices and reminders hold here.
A Claude session shuttle launches gets its mail through its hooks instead, which no scenario of this suite drives.

## Pre-conditions

Before starting a session:

1. **Deploy a fresh dev binary.**
   Run `deploy-dev` to build `lym.exe` into `.dev-bin` as current source.
   The suite resolves `.dev-bin` itself and prepends it to the agent's PATH, so no PATH setup is needed.
   Production `lym` stays untouched.
   The fingerprint header's `Source: dev` line confirms the dev build is under test.
   The deployed binary is a snapshot -- re-deploy after any source change you want to test.
   Once a pane exists, run these checks inside a spawned agent pane:
   - `command -v lym` (`where lym` on Windows) is not checked here:
     the interactive `--cmd claude` and `--cmd bash` strands each start a fresh shell that re-runs the operator's rc.
     See `SANDBOX-REED-SUITE.md` for the pane-shell guarantee and `SANDBOX-SHUTTLE-SUITE.md` for the agent Bash tool check.
   - `LYM_BIN` is set inside that pane and names the same binary.
   - `claude` still resolves inside that pane.
     `claude` is looked up by bare name from the pane's PATH,
     so a change in how a pane's shell is started exposes it most.
   No hermetic test proves either `LYM_BIN` or `claude`, so both are pre-condition checks rather than assertions.
2. **Materialize the hub.**
   Run `sandbox/build.cmd` (or `sandbox/build.cmd -reset` to start clean);
   the session cwd is the Hub's Fabric repo root, the same operating model as the main suite.
3. **Live-tmux and claude requirement.**
   The session needs tmux (or the Windows tmux port) on PATH and PowerShell 7 for reed's strands.
   It also needs a logged-in `claude` on PATH for S9's `claude` strand.
   If any of these is unavailable, **note that as the session outcome rather than treating it as a creel defect**.
   The `**Covers:** creel` tag on S9 satisfies the coverage guard in `sandbox_coverage_test.go` whatever the runtime.
4. **Wired worktree required.**
   `lym reed` requires a worktree wired by `lym fabric clone` or `lym fabric add`.
   `lym creel` reaches strands through reed.

## Black-box rule

**The agent under test works exclusively inside the Hub's Fabric repo (`lym-test-LYMHUB/lym-test`).
It tests `lym.exe` as a black box -- exactly as a real user with only the binary on PATH.
It must not look for, read, or reason about the lym source tree.
No peeking at `C:\Code\loomyard\` or any other path outside the Hub.**

The agent discovers the command surface from these alone, never from documentation outside the Hub:
`lym creel`, `lym creel <subcommand>`, `lym reed`, `lym reed <subcommand>` and `--help` flags.

### Controlled creel exceptions

The creel scenarios (C1 to C13) need only these non-black-box steps:

- **Listing the tmux and lymtemp socket directories read-only** (C1).
  It compares the sandbox hub's sockets with the live hub's.
- **Planting or editing a journal day file while the daemon is stopped** (C10), under the hub's creel state directory.
- **Running a process detached from a strand's pane** (C12).
  It checks that a process outside the pane is not taken for the strand.
  C12's disagreeing-strand-name check is operator-assisted when the harness refuses a command naming the strand variable:
  the agent asks the operator to run it in the pane and reports what the operator saw.
- **Pane content a scenario judges** is observed by the operator through `lym reed attach`.
  The agent reports it back.
  That covers S9's typed reminder, C2's single reminder in the scrollback, C6's line not typed again,
  and C7's restored prompt.

## Fingerprint header

The launcher prepends a "binary under test" fingerprint block when it copies this file into the Hub's Fabric repo.
The fingerprint records the `lym.exe` binary's absolute path, size, modification time and a short SHA-256 at launch.

The same fingerprint gives the report its provenance:
a separate fetch step run after the session stamps it into `meta.fingerprint` of the fetched `sandbox-report.json`.
A maintainer can then reproduce the exact binary behind each finding.
The agent does not transcribe the fingerprint anywhere.

## How to run a scenario

For each scenario below:

- Read the **Goal** -- it names the task, not the commands.
  Discover the commands via `lym creel`, `lym creel <subcommand>`, `lym reed`, and `--help` flags.
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
The file follows this exact schema.
**Always write the file, even when there are zero `WARN`/`FAIL` findings** -- in that case `items` is an empty array.

```json
{
  "source": "sandbox-report",
  "items": [
    {
      "ref": "C2",
      "title": "…",
      "body": "verdict: WARN\n\n…repro…"
    }
  ]
}
```

- `source` is the literal string `"sandbox-report"`.
- `items[]` holds only `WARN`/`FAIL` findings -- do not record `OK` scenarios here.
- `ref` is the scenario id (`S9`, `C1`-`C13`).
- `title` is a short one-line summary.
- `body` folds the detail, repro steps, and verdict into one markdown string.

Write only `source` and `items`;
a separate fetch step run after the session stamps `meta`, including the binary fingerprint.
Confine all free text to the `title`/`body` string fields so the JSON stays well-formed.

## Scenarios

### S9 -- Creel delivers mail and injects into a hand-added strand

**Covers:** creel

**Goal:** "Post a message to an interactive agent added by hand, once from the terminal and once from another agent.
Watch the terminal's message carried in full and the agent's message reminded once the receiver is idle.
Have the receiver read the agent's message and take an injected line, then confirm the log shows no body."

**Watch:** Add a strand with `lym reed add --cmd claude` and let it reach its idle prompt.
Add a shell strand with `lym reed add --cmd bash`.
`lym creel post <strand name> "please say CARRIED"` from the operator's shell prints one JSON envelope.
The envelope carries the message `id`.
The first creel verb of the hub starts its daemon by itself.
The terminal is EXTERNAL, so its message is carried:
at the agent's next idle moment this line appears typed in the agent's pane:
`[creel] #<id> from "<shortname>:EXTERNAL": please say CARRIED`.
The message is read with no `lym creel read`.
`lym creel post <strand name> "please say READ"` run in the shell strand's pane is agent mail:
at the agent's next idle moment a one-line reminder naming the shell strand appears typed in its pane.
The reminder is not repeated while the agent works.
Asking the agent to run `lym creel read` prints plain text.
The first line holds the message's id, its sender and the reply command, and the body follows below.
A second `lym creel read` prints `nothing unread`.
`lym creel inject <strand name> "say INJECTED" --wait` types the line into the pane at idle.
It exits 0 with `"outcome":"injected"`.
`lym creel log --id <id>` lists the message's events, oldest first, with their times and subject:
`posted`, each `reminded` event whose unread ids hold it, and `read`.
No event has a body field.
A reminder typed into a busy turn, a line typed twice, or a body field in the log is a failure here.

**Verdict:** `OK` / `WARN` / `FAIL`

---

**Creel scenario fixtures.**
The creel scenarios C1 to C13 reuse the fixtures of the creel audit.
The agent sets them up in the Hub as steps of its own:

- Bash strands, added with `lym reed add --cmd bash`, stand in for plain shell agents.
- A fake idle pane that swallows input.
  It is a script that draws a ready marker and an empty input box, then reads lines without echoing them.
- A fake input box that ignores Enter, so a typed line stays in the box.
- An orch-role copy of the idle pane, added with `--role orch`.

The agent stops the creel daemon only by replacing its binary, never by killing a process it did not start.

### C1 -- First verb and isolation

**Covers:** creel

**Goal:** "Run the first creel verb of the sandbox hub and confirm it shares no path with any other hub."

**Watch:** The first verb starts the hub's daemon.
The daemon socket name carries the sandbox hub's shortname and path hash.
The sandbox reed `-L` key differs from the live hub's.
The daemon's binary is the sandbox build.
Listing the tmux and lymtemp socket directories read-only shows the sandbox sockets beside the live ones and none shared.

**Verdict:** `FAIL` if any path is shared with the live hub.

---

### C2 -- Reminder timing

**Covers:** creel

**Goal:** "Post a message to a Claude strand busy with a long task.
Confirm its reminder is typed after the turn, and only once."

**Watch:** Busy a Claude strand with a long inject, then post mid-turn from a shell strand's pane.
Only an agent strand's mail is reminded.
The reminder lands only after the turn ends and appears once in the pane's scrollback.

**Verdict:** `FAIL` on a reminder typed into a busy turn, or a duplicate.

---

### C3 -- Reminder backoff

**Covers:** creel

**Goal:** "Leave a message unread while the strand keeps answering its reminders.
Confirm the reminders back off and a new message is reminded at once."

**Watch:** The message comes from a shell strand's pane, and the strand answers reminders without reading.
`reminded` events in `lym creel log` come about 5 and 10 minutes apart, and a new message is reminded at once.

**Verdict:** `WARN` if the timing drifts by more than 30 s.

---

### C4 -- Read sets and acknowledgement

**Covers:** creel

**Goal:** "Leave interleaved mail from several senders and read it in every way the verb offers.
Confirm each read returns the right set.
Confirm mail whose output could not be written stays unread."

**Watch:** Interleave mail from EXTERNAL and a shell strand to a busy receiver.
The receiver stays busy so the EXTERNAL mail is not carried before the reads.
Plain `read` returns every sender's mail at once.
`read --from` narrows it to one sender.
`read --envelope` prints the same set as one JSON line.
Check `inbox` too.
`lym creel read > /dev/full` exits 1, writes a `creel:` line to stderr and leaves the mail unread.
A following `read` still returns it.

**Verdict:** `FAIL` on a wrong set or lost mail.

---

### C5 -- Reply chain

**Covers:** creel

**Goal:** "Wait for a reply to a posted message, answer it, then forward a message on and answer that.
Confirm every reply reaches the right sender and the refused cases are refused."

**Watch:** `post --wait-reply` is answered by `--reply`.
A forward followed by a reply goes back to the sender.
Forwarding to oneself, forwarding a message not held and forwarding one already read are refused.

**Verdict:** `FAIL` on a misrouted reply.

---

### C6 -- Injects

**Covers:** creel

**Goal:** "Inject a line into panes that take it, ignore Enter, or never go idle.
Confirm each ends as it should, with no line typed twice."

**Watch:** `inject --wait` into an idle pane gives `injected`.
A fake box that ignores Enter gives `unconfirmed`, and the line is not typed again once the box is cleared.
A short `--ttl` into a never-idle pane expires within a second of its TTL.
Into a bash strand it names the stall `no-input-box`.

**Verdict:** `FAIL` on a retype or an early expiry.

---

### C7 -- Interrupt

**Covers:** creel

**Goal:** "Interrupt a Claude strand mid-turn, with and without an inject.
Confirm the line lands without anyone clearing the box."

**Watch:** `interrupt` and `inject --interrupt` mid-turn.
Claude puts the interrupted prompt back in the input box, and creel clears it,
so the inject lands with no hand-clearing.
A box holding the operator's own text is left alone, and the inject waits.

**Verdict:** `FAIL` if the inject waits behind the restored prompt.

---

### C8 -- Hold and pause

**Covers:** creel

**Goal:** "Hold a strand for one sender and send from another, then pause the hub and resume it.
Confirm delivery follows the hold and the pause."

**Watch:** A privileged sender holds the strand.
Injects and mail from another sender are held back, and the owner's inject passes.
After `release` the held inject is typed and the mail reminded.
An `inject --wait` started after a draft was cleared under a hold prints no stall line and ends injected.
Then `lym tender pause` freezes the TTL and holds delivery, and `lym tender resume` delivers.

**Verdict:** `FAIL` on delivery during a hold or pause, or a TTL that runs while paused.

---

### C9 -- Orch

**Covers:** creel

**Goal:** "Send mail, an inject, an interrupt and a hold to an orch-role strand.
Confirm creel delivers the first three and refuses the hold."

**Watch:** Mail to the orch-role strand is reminded in its pane when it is idle.
A text `inject` and an `interrupt` from EXTERNAL are accepted.
A `hold` on it from EXTERNAL or from another orch strand is refused.
The refusal says only a program may hold an orch strand.
The orch's own `inbox` shows plain unread counts and no `withheld` or `notice` key.

**Verdict:** `FAIL` on a hold accepted from a non-program, or on a keystroke in an orch pane that no creel verb caused.

---

### C10 -- Daemon life and journal

**Covers:** creel

**Goal:** "Replace the binary under a running daemon, then damage the journal as a crash or an old day file would.
Confirm the daemon restarts on the same pool and says why when it cannot start."

**Watch:** Replacing the binary ends the daemon within 2 s.
The next verb brings the same pool, as a diff of `lym creel log` before and after shows.
A planted 8-day-old day file is carried forward and deleted.
A torn tail is cut whether or not it ends in a newline.
A restart after the next append starts cleanly.
A malformed middle line, planted while the daemon is stopped, gives the verb an error naming the daemon log.
The daemon log quotes the start's error line with the journal file and line.

**Verdict:** `FAIL` on a pool diff or lost unread mail.

---

### C11 -- Removed holder

**Covers:** creel

**Goal:** "Remove the strand that holds forwarded mail and mail someone waits on.
Confirm each message steps back or ends undelivered, and the reader is told it bounced."

**Watch:** Remove the strand holding forwarded mail and the one holding mail awaited with `--wait-reply`.
Within 60 s of the removal each message steps back one hop or ends `undelivered`, and the waiter exits 5.
The reader of a bounced message sees `ended_unread_by` and `ended_unread_at` on it.

**Verdict:** `WARN` if the bounce is not shown.

---

### C12 -- Identity

**Covers:** creel

**Goal:** "Post from a pane, from a terminal, from a detached process and from a pane under another strand's name.
Confirm each is who it should be or is refused."

**Watch:** Pane processes resolve to their own strand and a terminal to EXTERNAL.
A process detached from a strand's pane that keeps the strand name is refused.
A disagreeing strand name inside a pane is refused;
this check is operator-assisted when the harness refuses a command naming the strand variable.

**Verdict:** `FAIL` on a spoof that succeeds.

---

### C13 -- Hand-added and stalled strands

**Covers:** creel

**Goal:** "Post to a Claude strand added by hand and to a strand whose window shows no input box.
Confirm each receives its line or creel says why it cannot deliver."

**Watch:** A plain `lym reed add --cmd claude` strand receives the line carrying a post from the operator's shell.
A `lym reed add --cmd bash` strand, which reed places in a window of its own, shows no input box.
Its stall reason, `no-input-box`, shows as a `stalled` event in `lym creel log`.
It also shows as a progress line on stderr of `lym creel inject --wait`.

**Verdict:** `FAIL` on a silent stall.

## Session log format

After running all scenarios, record a short session summary:

```
Date: <YYYY-MM-DD>
Binary fingerprint: <copy from the header above>

S9: <OK|WARN|FAIL> -- <one-line note if not OK>
C1: <OK|WARN|FAIL> -- <one-line note if not OK>
C2: <OK|WARN|FAIL> -- <one-line note if not OK>
C3: <OK|WARN|FAIL> -- <one-line note if not OK>
C4: <OK|WARN|FAIL> -- <one-line note if not OK>
C5: <OK|WARN|FAIL> -- <one-line note if not OK>
C6: <OK|WARN|FAIL> -- <one-line note if not OK>
C7: <OK|WARN|FAIL> -- <one-line note if not OK>
C8: <OK|WARN|FAIL> -- <one-line note if not OK>
C9: <OK|WARN|FAIL> -- <one-line note if not OK>
C10: <OK|WARN|FAIL> -- <one-line note if not OK>
C11: <OK|WARN|FAIL> -- <one-line note if not OK>
C12: <OK|WARN|FAIL> -- <one-line note if not OK>
C13: <OK|WARN|FAIL> -- <one-line note if not OK>

sandbox-report.json written: <count of WARN/FAIL items>
```

`./sandbox-report.json` must be written before the session ends, as the Capturing findings section says.
When every scenario was `OK`, it holds `items: []`.

## Notes

- Creel scenarios go here as `C` scenarios with a `**Covers:** creel` line;
  shuttle scenarios stay in `SANDBOX-SHUTTLE-SUITE.md`.
