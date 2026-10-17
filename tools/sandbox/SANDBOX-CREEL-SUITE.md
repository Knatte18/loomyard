# SANDBOX-CREEL-SUITE -- lyx creel black-box agent suite

## What this is

A structured test-loop for exercising `lyx creel` against a **live tmux server and a logged-in claude** in the sandbox Hub's Fabric repo.
Like `SANDBOX-REED-SUITE.md`, the value here is partly **visual**: mail reminders and injected lines typed into a strand's pane, a message's events in the log.
Not an automated suite -- an agent drives it, an operator watches.

## Pre-conditions

Before starting a session:

1. **Deploy a fresh dev binary.**
   Run `deploy-dev` to build `lyx.exe` into `.dev-bin` as current source.
   The suite resolves `.dev-bin` itself and prepends it to the agent's PATH (the fingerprint header's `Source: dev` line confirms the dev build is under test) -- no PATH setup needed, and production `lyx` stays untouched.
   The deployed binary is a snapshot -- re-deploy after any source change you want to test.
   Once a pane exists, run these checks inside a spawned agent pane:
   - `command -v lyx` (`where lyx` on Windows) is not a check this suite makes: its interactive strands, the plain `--cmd claude` and `--cmd bash` strands, each start a fresh shell that re-runs the operator's rc.
     See `SANDBOX-REED-SUITE.md` for the pane-shell guarantee and `SANDBOX-SHUTTLE-SUITE.md` for the agent Bash tool check.
   - `LYX_BIN` is set inside that pane and names the same binary.
   - `claude` still resolves inside that pane.
     `claude` is looked up by bare name from the pane's PATH, so it is the property most exposed to any future change in how a pane's shell is started, and it is the one property no hermetic test can prove.
   `LYX_BIN` being set is the live-substrate fact no `go test` can prove, and both remaining checks are pre-condition checks rather than assertions.
2. **Materialize the hub.**
   Run `sandbox/build.cmd` (or `sandbox/build.cmd -reset` to start clean);
   the session cwd is the Hub's Fabric repo root, the same operating model as the main suite.
3. **Live-tmux and claude requirement.** tmux (or the Windows tmux port) on PATH, PowerShell 7, which reed's strands need,
   and a logged-in `claude` on PATH, since S9 adds a `claude` strand.
   If any of these is unavailable in the session, **note that as the session outcome rather than treating it as a creel defect** -- the `**Covers:** creel` tag on S9 satisfies the sandbox coverage guard (`sandbox_coverage_test.go`) regardless of runtime availability.
4. **Wired worktree required.** `lyx reed` requires a worktree wired by `lyx fabric clone`/`lyx fabric add`, and `lyx creel` reaches strands through reed.

## Black-box rule

**The agent under test works exclusively inside the Hub's Fabric repo (`lyx-test-LYXHUB/lyx-test`).
It tests `lyx.exe` as a black box -- exactly as a real user with only the binary on PATH.
It must not look for, read, or reason about the lyx source tree.
No peeking at `C:\Code\loomyard\` or any other path outside the Hub.**

Discovering the command surface is done via `lyx creel`, `lyx creel <subcommand>`, `lyx reed`, `lyx reed <subcommand>`, and `--help` flags alone -- not from documentation outside the Hub.

### Controlled creel exceptions

The creel scenarios (C1 to C13) need these non-black-box steps and nothing more:

- **Listing the tmux and lyxtemp socket directories read-only** (C1), to compare the sandbox hub's sockets with the live hub's.
- **Planting or editing a journal day file while the daemon is stopped** (C10), under the hub's creel state directory.
- **Running a process detached from a strand's pane** (C12), to check that a process outside the pane is not taken for the strand.
  The disagreeing-strand-name check of C12 is operator-assisted when the agent's harness refuses a command naming the strand variable: the agent asks the operator to run it in the pane and reports what the operator saw.
- **Pane content a scenario judges** (S9's typed reminder, C2's single reminder in the scrollback, C6's line not typed again, C7's restored prompt) is observed by the operator through `lyx reed attach` and reported back by the agent, as C12's operator-assisted check is.

## Fingerprint header

The launcher prepends a "binary under test" fingerprint block to this file when it copies it into the Hub's Fabric repo.
The fingerprint records the absolute path, file size, modification time, and a short SHA-256 of the `lyx.exe` binary at launch time.

The same fingerprint identifies the binary for the report's provenance: a separate fetch step (run after this session) stamps it into `meta.fingerprint` of the fetched `sandbox-report.json` so a maintainer can reproduce the exact binary that produced each finding.
The agent does not need to transcribe the fingerprint anywhere itself.

## How to run a scenario

For each scenario below:

- Read the **Goal** -- it names the task, not the commands.
  Discover the commands via `lyx creel`, `lyx creel <subcommand>`, `lyx reed`, and `--help` flags (S0 ethos).
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

Write only `source` and `items` -- a separate fetch step (run after the session) stamps `meta` (including the binary fingerprint).
Confine all free text to the `title`/`body` string fields so the JSON stays well-formed.

## Scenarios

### S9 -- Creel delivers mail and injects into a hand-added strand

**Covers:** creel

**Goal:** "Post a message to an interactive agent that was added by hand, watch its reminder get typed once the agent is idle, have the agent read the message, inject a line into it, and confirm the log shows the events without any body."

**Watch:** Add a strand with `lyx reed add --cmd claude` and let it reach its idle prompt.
`lyx creel post <strand name> "please say READ"` prints one JSON envelope with the message `id`, and the first creel verb of the hub starts its daemon by itself.
At the agent's next idle moment a one-line reminder naming the sender appears typed in its pane; the reminder is not repeated while the agent works.
Asking the agent to run `lyx creel read` prints the message with its sender and body, and a second `lyx creel read` prints that nothing is unread.
`lyx creel inject <strand name> "say INJECTED" --wait` types the line into the pane at idle and exits 0 with `"outcome":"injected"`.
`lyx creel log --id <id>` lists the message's events `posted`, each `reminded` event whose unread ids hold it, and `read`, oldest first, with their times and subject, and no event has a body field.
A reminder typed into a busy turn, a line typed twice, or a body field in the log is a real failure here.

**Verdict:** `OK` / `WARN` / `FAIL`

---

**Creel scenario fixtures.**
The creel scenarios C1 to C13 reuse the fixtures of the creel audit, which the agent sets up in the Hub as steps of its own:

- Bash strands, added with `lyx reed add --cmd bash`, stand in for plain shell agents.
- A fake idle pane that swallows input: a script that draws a ready marker and an empty input box and reads lines without echoing them.
- A fake input box that ignores Enter, so a typed line stays in the box.
- An orch-role copy of the idle pane, added with `--role orch`.

The agent stops the creel daemon only by replacing its binary, never by killing a process it did not start.

### C1 -- First verb and isolation

**Covers:** creel

**Goal:** "Run the first creel verb of the sandbox hub and confirm it shares no path with any other hub on the machine."

**Watch:** The first verb starts the hub's daemon.
The daemon socket name carries the sandbox hub's shortname and path hash, the sandbox reed `-L` key differs from the live hub's, and the daemon's binary is the sandbox build.
Listing the tmux and lyxtemp socket directories read-only shows the sandbox sockets beside the live ones and none shared.

**Verdict:** `FAIL` if any path is shared with the live hub.

---

### C2 -- Reminder timing

**Covers:** creel

**Goal:** "Post a message to a Claude strand that is busy with a long task, and confirm its reminder is typed after the turn and only once."

**Watch:** Busy a Claude strand with a long inject, then post mid-turn.
The reminder lands only after the turn ends and appears once in the pane's scrollback.

**Verdict:** `FAIL` on a reminder typed into a busy turn, or a duplicate.

---

### C3 -- Reminder backoff

**Covers:** creel

**Goal:** "Leave a message unread while the strand keeps answering its reminders, and confirm the reminders back off and a new message is reminded at once."

**Watch:** The strand answers reminders without reading.
`reminded` events in `lyx creel log` come about 5 and 10 minutes apart, and a new message is reminded at once.

**Verdict:** `WARN` if the timing drifts by more than 30 s.

---

### C4 -- Read sets and acknowledgement

**Covers:** creel

**Goal:** "Leave interleaved mail from several senders, read it in every way the verb offers, and confirm each read returns the right set and that mail whose output could not be written stays unread."

**Watch:** Interleave mail from EXTERNAL and a shell strand, then check plain `read`, `read --from`, `read --all` and `inbox`.
`lyx creel read > /dev/full` exits 1, writes a `creel:` line to stderr and leaves the mail unread, so a following `read` still returns it.

**Verdict:** `FAIL` on a wrong set or lost mail.

---

### C5 -- Reply chain

**Covers:** creel

**Goal:** "Wait for a reply to a posted message, answer it, forward a message on and answer that, and confirm every reply reaches the right sender and the refused cases are refused."

**Watch:** `post --wait-reply` is answered by `--reply`.
A forward followed by a reply goes back to the sender.
Forwarding to oneself, forwarding a message not held and forwarding one already read are refused.

**Verdict:** `FAIL` on a misrouted reply.

---

### C6 -- Injects

**Covers:** creel

**Goal:** "Inject a line into panes that take it, ignore Enter, or never go idle, and confirm each ends as it should without a line typed twice."

**Watch:** `inject --wait` into an idle pane gives `injected`.
A fake box that ignores Enter gives `unconfirmed`, and the line is not typed again once the box is cleared.
A short `--ttl` into a never-idle pane expires within a second of its TTL, and into a bash strand it names the stall `no-input-box`.

**Verdict:** `FAIL` on a retype or an early expiry.

---

### C7 -- Interrupt

**Covers:** creel

**Goal:** "Interrupt a Claude strand mid-turn, with and without an inject, and confirm the line lands without anyone clearing the box."

**Watch:** `interrupt` and `inject --interrupt` mid-turn.
Claude puts the interrupted prompt back in the input box, and creel clears it, so the inject lands with no hand-clearing.
A box holding the operator's own text is left alone, and the inject waits.

**Verdict:** `FAIL` if the inject waits behind the restored prompt.

---

### C8 -- Hold and pause

**Covers:** creel

**Goal:** "Hold a strand for one sender, send from another, then pause the hub and resume it, and confirm delivery follows the hold and the pause."

**Watch:** A privileged sender holds the strand, injects and mail from another sender are held back, and the owner's inject passes.
After `release` the held inject is typed and the mail reminded.
An `inject --wait` started after a draft was cleared under a hold prints no stall line and ends injected.
Then `lyx tender pause` freezes the TTL and holds delivery, and `lyx tender resume` delivers.

**Verdict:** `FAIL` on delivery during a hold or pause, or a TTL that runs while paused.

---

### C9 -- Orch

**Covers:** creel

**Goal:** "Send mail, an inject and an interrupt to an orch-role strand, and confirm nothing is typed into its pane."

**Watch:** Mail to the orch-role strand is stored with a `withheld` record, `inject` and `interrupt` are refused, and the orch's own `inbox` shows the mail as withheld.

**Verdict:** `FAIL` on any keystroke in an orch pane.

---

### C10 -- Daemon life and journal

**Covers:** creel

**Goal:** "Replace the binary under a running daemon, damage the journal in the ways a crash or an old day file would, and confirm the daemon restarts on the same pool and says why when it cannot start."

**Watch:** Replacing the binary ends the daemon within 2 s, and the next verb brings the same pool (a diff of `lyx creel log` before and after).
A planted 8-day-old day file is carried forward and deleted, and a torn tail is cut whether or not it ends in a newline, and a restart after the next append starts cleanly.
A malformed middle line, planted while the daemon is stopped, gives the verb an error naming the daemon log, which quotes the start's error line with the journal file and line.

**Verdict:** `FAIL` on a pool diff or lost unread mail.

---

### C11 -- Removed holder

**Covers:** creel

**Goal:** "Remove the strand that holds forwarded mail and mail someone waits on, and confirm each message steps back or ends undelivered and the reader is told it bounced."

**Watch:** Remove the strand holding forwarded mail and the one holding mail awaited with `--wait-reply`.
Within 60 s of the removal each message steps back one hop or ends `undelivered`, the waiter exits 5, and the reader of a bounced message sees `ended_unread_by` and `ended_unread_at` on it.

**Verdict:** `WARN` if the bounce is not shown.

---

### C12 -- Identity

**Covers:** creel

**Goal:** "Post from inside a pane, from a terminal, from a detached process and from a pane under another strand's name, and confirm each is who it should be or is refused."

**Watch:** Pane processes resolve to their own strand and a terminal to EXTERNAL.
A process detached from a strand's pane that keeps the strand name is refused.
A disagreeing strand name inside a pane is refused; this check is operator-assisted when the harness refuses a command naming the strand variable.

**Verdict:** `FAIL` on a spoof that succeeds.

---

### C13 -- Hand-added and stalled strands

**Covers:** creel

**Goal:** "Post to a Claude strand added by hand and to a strand whose window shows no input box, and confirm each receives its reminder or creel says why it cannot deliver."

**Watch:** A plain `lyx reed add --cmd claude` strand receives its reminder.
A `lyx reed add --cmd bash` strand, which reed places in a window of its own, shows no input box.
Its stall reason, `no-input-box`, shows as a `stalled` event in `lyx creel log` and as a progress line on stderr of `lyx creel inject --wait`.

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

`./sandbox-report.json` must be written before the session ends, per the Capturing findings section above -- with `items: []` when every scenario was `OK`.

## Notes

- Creel scenarios go here as `C` scenarios with a `**Covers:** creel` line;
  shuttle scenarios stay in `SANDBOX-SHUTTLE-SUITE.md`.
