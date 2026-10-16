# SANDBOX-SHUTTLE-SUITE -- lyx shuttle black-box agent suite

## What this is

A structured test-loop for exercising `lyx shuttle` against a **live tmux server and a logged-in claude** in the sandbox Hub's Fabric repo.
Like `SANDBOX-REED-SUITE.md`, the value here is partly **visual**: a strand's pane doing real agent work, an outcome coming back.
Not an automated suite -- an agent drives it, an operator watches.

## Pre-conditions

Before starting a session:

1. **Deploy a fresh dev binary.**
   Run `deploy-dev` to build `lyx.exe` into `.dev-bin` as current source.
   The suite resolves `.dev-bin` itself and prepends it to the agent's PATH (the fingerprint header's `Source: dev` line confirms the dev build is under test) -- no PATH setup needed, and production `lyx` stays untouched.
   The deployed binary is a snapshot -- re-deploy after any source change you want to test.
   Once a pane exists, run these checks inside a spawned agent pane:
   - `command -v lyx` (`where lyx` on Windows) names the spawning binary, not the production install.
   - `LYX_BIN` is set inside that pane and names the same binary.
   - `claude` still resolves inside that pane.
     `claude` is looked up by bare name from the pane's PATH, so it is the property most exposed to any future change in how a pane's shell is started, and it is the one property no hermetic test can prove.
   The first two are also live-substrate facts no `go test` can prove, which is why all three are pre-condition checks rather than assertions.
2. **Materialize the hub.**
   Run `sandbox/build.cmd` (or `sandbox/build.cmd -reset` to start clean);
   the session cwd is the Hub's Fabric repo root, the same operating model as the main suite.
3. **Live-tmux and claude requirement.** tmux (or the Windows tmux port) on PATH, PowerShell 7,
   and a logged-in `claude` on PATH.
   If any of these is unavailable in the session, **note that as the session outcome rather than treating it as a shuttle defect** -- the `**Covers:** shuttle` tag on S1 satisfies the sandbox coverage guard (`sandbox_coverage_test.go`) regardless of runtime availability.
4. **Wired worktree required.** `lyx shuttle` requires a worktree wired by `lyx fabric clone`/`lyx fabric add` -- which materializes `_lyx/config/shuttle.yaml` and `reed.yaml` eagerly -- exactly like `lyx reed` does.

## Black-box rule

**The agent under test works exclusively inside the Hub's Fabric repo (`lyx-test-LYXHUB/lyx-test`).
It tests `lyx.exe` as a black box -- exactly as a real user with only the binary on PATH.
It must not look for, read, or reason about the lyx source tree.
No peeking at `C:\Code\loomyard\` or any other path outside the Hub.**

Discovering the command surface is done via `lyx shuttle`, `lyx shuttle <subcommand>`, and `lyx shuttle <subcommand> --help` alone -- not from documentation outside the Hub.

### Controlled tmux exceptions

One sanctioned deviation from the pure black-box rule, mirroring the reed suite's own controlled-exception note:

- **Direct `tmux -L <socket> list-panes`/`ls`** is allowed only to confirm a strand's pane exists (or was cleaned up), where `<socket>` is read from the shuttle run's strand guid cross-referenced against `lyx reed status` output.
- **Scenario S2's operator attach** is operator-assisted -- see S2 below.

### Controlled creel exceptions

The creel scenarios (C1 to C13) need these non-black-box steps and nothing more:

- **Listing the tmux and lyxtemp socket directories read-only** (C1), to compare the sandbox hub's sockets with the live hub's.
- **Planting or editing a journal day file while the daemon is stopped** (C10), under the hub's creel state directory.
- **Running a process detached from a strand's pane** (C12), to check that a process outside the pane is not taken for the strand.
  The disagreeing-strand-name check of C12 is operator-assisted when the agent's harness refuses a command naming the strand variable: the agent asks the operator to run it in the pane and reports what the operator saw.

## Fingerprint header

The launcher prepends a "binary under test" fingerprint block to this file when it copies it into the Hub's Fabric repo.
The fingerprint records the absolute path, file size, modification time, and a short SHA-256 of the `lyx.exe` binary at launch time.

The same fingerprint identifies the binary for the report's provenance: a separate fetch step (run after this session) stamps it into `meta.fingerprint` of the fetched `sandbox-report.json` so a maintainer can reproduce the exact binary that produced each finding.
The agent does not need to transcribe the fingerprint anywhere itself.

## How to run a scenario

For each scenario below:

- Read the **Goal** -- it names the task, not the commands.
  Discover the commands via `lyx shuttle`, `lyx shuttle <subcommand>`, and `--help` flags (S0 ethos).
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
      "ref": "S2",
      "title": "…",
      "body": "verdict: WARN\n\n…repro…"
    }
  ]
}
```

- `source` is the literal string `"sandbox-report"`.
- `items[]` holds only `WARN`/`FAIL` findings -- do not record `OK` scenarios here.
- `ref` is the scenario id (`S1`-`S9`, `C1`-`C13`).
- `title` is a short one-line summary.
- `body` folds the detail, repro steps, and verdict into one markdown string.

Write only `source` and `items` -- a separate fetch step (run after the session) stamps `meta` (including the binary fingerprint).
Confine all free text to the `title`/`body` string fields so the JSON stays well-formed.

## Scenarios

### S1 -- Autonomous happy path

**Covers:** shuttle

**Goal:** "Ask a shuttle agent to write a specific file, and confirm the run reports `done` once it does."

**Watch:** `lyx shuttle run --prompt "write the single line OK into result.md and nothing else" --output-file result.md` starts a strand (visible as a pane, confirmable via `lyx reed status` or `tmux -L <socket> list-panes`);
the command blocks until the agent finishes;
the printed JSON envelope reports `"outcome":"done"` with a `sessionId` and `guid`;
`result.md` exists with the expected content;
and afterward the strand's pane and run directory are cleaned up (no leftover pane, `lyx reed status` no longer lists the guid).

**Verdict:** `OK` / `WARN` / `FAIL`

---

### S2 -- Hold path (operator-assisted)

**Covers:** shuttle

**Goal:** "Give a shuttle agent a task it cannot complete without a decision only the operator can make, and confirm the run keeps waiting at the agent's question, then let the operator answer it directly in the pane or by `lyx shuttle send`, and confirm the run ends `done`."

**Watch:** `lyx shuttle run --prompt "before writing decision.md, stop and ask me which of two options you should pick — do not guess" --output-file decision.md --interactive` does not return when the agent ends its turn with its question: the run is held;
the strand and its pane are still alive (`lyx reed status` still lists the guid;
`decision.md` does not exist yet).
The agent then instructs the operator to attach (`lyx reed attach` in a second terminal, per the reed suite's M7/M14 pattern), answer the question in the pane or by `lyx shuttle send <guid> "<answer>"`, and confirm the agent continues, eventually writes `decision.md`, and the original `lyx shuttle run` returns `"outcome":"done"`.

**Verdict:** `OK` / `WARN` / `FAIL`

---

### S3 -- Interrupt and send (cross-terminal poke)

**Covers:** shuttle

**Goal:** "Start a long-running shuttle agent from one terminal, then from a second terminal interrupt its current turn and send it a one-line update, and confirm the agent continues from the new instruction."

**Watch:** Start a long-running run in one terminal, e.g. `lyx shuttle run --prompt "count slowly to a very large number out loud, one number per line, before writing done.md" --output-file done.md`.
From a second terminal, note the `guid` (via `lyx reed status`) and run `lyx shuttle interrupt <guid>` -- the agent's current turn stops without killing its pane or session (`lyx reed status` still shows it `live: true`).
Then run `lyx shuttle send <guid> "stop counting and write done.md right away"` -- a single-line update only.
The deterministic property to verify is that `done.md` eventually appears with the redirected content: the first terminal's envelope reports `"outcome":"done"`, because the interrupted turn's own Stop is a hold and only the redirect turn's completion ends the run.
Any other outcome (or a missing/wrong `done.md`) is a real failure here.
Sending multiline text must be rejected outright (a "must be a single line" error), not silently truncated or mis-submitted.

**Verdict:** `OK` / `WARN` / `FAIL`

---

### S4 -- One envelope per invocation, even when two things are wrong at once

**Covers:** shuttle

**Goal:** "Make a `lyx shuttle run` invocation fail for TWO independent reasons at the same time, and confirm it still answers with exactly one JSON object naming the one to fix first."

**Watch:** From a directory that is NOT a git repository (e.g. a fresh `mkdir` under the system temp dir), run `lyx shuttle run --output-file out.md` -- geometry resolution fails AND no `--prompt`/`--prompt-file` was given, so both the pre-flight and the flag check have something to say.
The command must print **exactly one** JSON object, and it must be the pre-flight one (`"error":"not a git repository"`), because that is the problem the operator has to fix before the flag error can even be evaluated.
Two objects on one invocation is a FAIL, not a cosmetic issue: a caller that reads the output with a single-object JSON parse gets a parse error instead of the real cause.
Repeat with `--prompt a --prompt-file b --output-file out.md` (mutually exclusive flags) for the same result.
Then, from inside a real lyx worktree, run `lyx shuttle run --output-file out.md` and confirm the flag error IS reported on its own there -- suppressing it only when the pre-flight already failed is the point, suppressing it always would be a different bug.
This scenario starts no agent and costs no provider tokens.

**Verdict:** `OK` / `WARN` / `FAIL`

---

### S5 -- Reed's bookkeeping is reset under a run that is still working

**Covers:** shuttle

**Goal:** "Take reed's strand table away from underneath a live shuttle run, and confirm shuttle says it does not know rather than declaring the agent dead -- and that it does not delete the live run's directory."

**Watch:** Start a run that stays inside ONE turn for several minutes, e.g.
`lyx shuttle run --prompt "run python3 -c 'import time; time.sleep(100); print(1)' in the foreground three times in a single reply, then write done.md" --output-file done.md --model haiku`.
Wait until `lyx reed status` shows the strand `live: true` and the pane visibly shows the agent working (a running tool call with a rising elapsed counter -- do not proceed while it is still booting).
Then, from a second terminal, delete reed's state file: `rm <worktree>/.lyx/reed.json`.
This is not vandalism -- it is verbatim the second remedy reed's own corrupt-state error recommends ("delete it by hand to keep the session"), and it is what a `git clean -xdf` in the worktree does.

Three things must hold, and each was a real defect before:

1. The blocked `lyx shuttle run` must return `"ok":false` with an error saying reed no longer tracks the strand and that the agent may still be working -- **never** `"ok":true` with `"outcome":"died"`. A `died` envelope here is a FAIL: it tells an unattended caller to respawn, which puts a second agent on the worktree while the first keeps running unreachably.
2. The envelope must still carry `guid`, `sessionId` and `runDir`, so the operator can reach the pane that is still running.
3. Confirm the agent really is still alive afterward (`tmux -L <socket> capture-pane` on its pane shows the same turn still progressing) -- the whole point of the verdict in (1).

Then, with `reed.json` still absent, start a SECOND run in the same worktree with a deliberately cheap prompt (e.g. `--model haiku --prompt "write ok" --output-file second.md`): `add strand` now self-heals rather than refusing, so this second run boots a fresh session and spawns a real second agent -- **this step now costs real tokens**, unlike the no-session refusal it used to hit. Confirm the FIRST run's directory under `<worktree>/.lyx/shuttle/` is **still there** once the second run has started. A missing run directory is a FAIL: it holds the `events.jsonl` the live agent's Stop hook is still appending to, and the `run.json` without which `lyx shuttle interrupt/send <guid>` can no longer find the running agent at all.

Restore with `lyx reed up` (which writes a fresh `reed.json`) and `lyx reed down` to tear the session down.

**Verdict:** `OK` / `WARN` / `FAIL`

---

### S6 -- Re-sending the same text into a pane that has scrolled

**Covers:** shuttle

**Goal:** "Send one instruction twice into a busy agent pane, with the first copy scrolled to the very top of the viewport, and confirm the second send is delivered exactly once and reported as delivered."

**Watch:** Start a run with `--keep-pane` and a trivial prompt (e.g. `lyx shuttle run --prompt "write READY into ready.txt" --output-file ready.txt --keep-pane --model haiku`), which leaves one live, idle agent pane to drive.
Note the `guid` from `lyx reed status`.

Send an instruction whose ANSWER is long enough to fill the pane, e.g. `lyx shuttle send <guid> "reply with the numbers 1 to 38 one per line and nothing else"`.
Wait for the reply to finish, then look at the pane (`tmux -L <socket> capture-pane -p -t <paneId>`): the line echoing your instruction must have been pushed to within a line or two of the TOP of the viewport, still visible.
That is the setup -- if it scrolled off entirely, send it once more and re-check.

Now send the SAME instruction again, timing it while the earlier copy is still visible at the top.
Two things must hold:

1. The command answers `{"ok":true,"action":"send",...}` within about a second. An `"ok":false` "the send was NOT delivered" here is a FAIL -- and the giveaway is that it takes ~11 s to say it.
2. The pane must show the instruction accepted exactly ONCE more, not twice. A second, duplicate copy of your instruction (and a second answer to it) is the same FAIL seen from the other side: the delivery check gave up and re-typed a message the agent had already received.

Both halves failed before this was fixed, because the earlier copy scrolling off as the new one arrived left the occurrence COUNT unchanged, which is indistinguishable from nothing arriving unless position is taken into account.
Tear down with `lyx reed remove <guid>` and `lyx reed down`.

**Verdict:** `OK` / `WARN` / `FAIL`

---

### S7 -- Reed drops the pane binding of a run that is still working

**Covers:** shuttle

**Goal:** "Make reed decide this worktree's pane bindings are stale while a shuttle run is mid-turn, and confirm shuttle says it can no longer address the agent rather than declaring it dead."

**Watch:** This is S5's sibling: there reed forgets the strand, here reed keeps the strand but forgets its PANE.
Start the same kind of run that stays inside ONE turn for several minutes (see S5) and wait until the pane visibly shows the agent working.

Then make the persisted pane generation stale, which is the state reed's own doc describes as "a reed.json older than the session now running" -- reachable in the wild from a restored backup or a copied `.lyx`.
In `<anchor>/.lyx/reed.json`, change the `paneGeneration.created` value to any older number, writing the file atomically (write a temp file beside it and rename it over the original, so reed never reads a half-written file).

Reed then logs `persisted pane bindings were minted against a different tmux session incarnation, clearing them`, and `lyx reed status` reports the strand with an empty `paneId` and `live: false` -- while `tmux list-panes` still shows its pane alive with the agent inside it.

Two things must hold:

1. The blocked `lyx shuttle run` must return `"ok":false` saying reed holds no pane binding for the strand and that the agent may still be working -- **never** `"ok":true` with `"outcome":"died"`. A `died` envelope here is the same FAIL as S5's, for the same reason: it tells an unattended caller to respawn onto a worktree whose agent is still running.
2. `lyx shuttle interrupt <guid>` must refuse, and its message must NOT claim the run reached a terminal outcome or that its pane died -- neither is true here.

Restore the original `created` value to make the binding usable again (`lyx reed status` will show the strand `live: true` on its pane once more, which also proves the agent was alive throughout), then `lyx reed remove <guid>` and `lyx reed down`.

**Verdict:** `OK` / `WARN` / `FAIL`

---

### S8 -- Session state of a running agent

**Covers:** shuttle

**Goal:** "Start an interactive shuttle agent, read what `lyx shuttle state` says its session is doing while it works and after it asks its question, narrow the list to the one run, and confirm every printed state matches what the pane shows."

**Watch:** Start the S2 run (`lyx shuttle run --prompt "before writing decision.md, stop and ask me which of two options you should pick — do not guess" --output-file decision.md --interactive`).
From a second terminal, `lyx shuttle state` prints one JSON envelope whose `runs` lists the run with its `strand`, `guid`, `state`, `cause`, `detail`, `outstanding`, `since` and `history`.
While the pane shows the agent working, the run reads `busy` with cause `turn`.
After the agent ends its turn with its question, the run reads `asking` with cause `awaiting-input`, because the run is interactive and `decision.md` does not exist yet, and `since` is the time the turn ended.
`lyx shuttle state --strand <strand name or guid>` lists only that run, and `--strand no-such-strand` prints an empty `runs` list with `"ok":true`.
Reading the state changes nothing: the pane, the run and `lyx reed status` are the same before and after.
A state the pane contradicts (for instance `idle-done` while the agent still works) is a real failure here.
The state model is in shadow mode, so a mismatch is a finding about the reading, never a reason the run itself misbehaved.

**Verdict:** `OK` / `WARN` / `FAIL`

---

### S9 -- Creel delivers mail and injects into a hand-added strand

**Covers:** creel

**Goal:** "Post a message to an interactive agent that was added by hand, watch its reminder get typed once the agent is idle, have the agent read the message, inject a line into it, and confirm the log shows the events without any body."

**Watch:** Add a strand with `lyx reed add --cmd claude` and let it reach its idle prompt.
`lyx creel post <strand name> "please say READ"` prints one JSON envelope with the message `id`, and the first creel verb of the hub starts its daemon by itself.
At the agent's next idle moment a one-line reminder naming the sender appears typed in its pane; the reminder is not repeated while the agent works.
Asking the agent to run `lyx creel read` prints the message with its sender and body, and a second `lyx creel read` prints that nothing is unread.
`lyx creel inject <strand name> "say INJECTED" --wait` types the line into the pane at idle and exits 0 with `"outcome":"injected"`.
`lyx creel log --id <id>` lists the message's events (`posted`, `read`) with their times and subject, and no event has a body field.
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
`lyx creel read > /dev/full` exits non-zero and leaves the mail unread, so a following `read` still returns it.

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
A short `--ttl` into a never-idle pane expires on time.

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
A planted 8-day-old day file is carried forward and deleted, and a torn tail is cut.
A malformed middle line, planted while the daemon is stopped, gives the verb an error naming the daemon log, which quotes the start's error line with the journal file and line.

**Verdict:** `FAIL` on a pool diff or lost unread mail.

---

### C11 -- Removed holder

**Covers:** creel

**Goal:** "Remove the strand that holds forwarded mail and mail someone waits on, and confirm each message steps back or ends undelivered and the reader is told it bounced."

**Watch:** Remove the strand holding forwarded mail and the one holding mail awaited with `--wait-reply`.
Within about 60 s each message steps back one hop or ends `undelivered`, the waiter exits 5, and the reader of a bounced message sees `ended_unread_by` and `ended_unread_at` on it.

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

### C13 -- Collapsed and hand-added strands

**Covers:** creel

**Goal:** "Post to a Claude strand added by hand and to one squeezed into a stacked 3-row pane, and confirm each receives its reminder or creel says why it cannot deliver."

**Watch:** A plain `lyx reed add --cmd claude` strand receives its reminder.
A stacked 3-row strand's stall reason shows as a `stalled` event in `lyx creel log` and as a progress line on stderr of `lyx creel inject --wait`.

**Verdict:** `FAIL` on a silent stall.

## Session log format

After running all scenarios, record a short session summary:

```
Date: <YYYY-MM-DD>
Binary fingerprint: <copy from the header above>

S1: <OK|WARN|FAIL> -- <one-line note if not OK>
S2: <OK|WARN|FAIL> -- <one-line note if not OK>
S3: <OK|WARN|FAIL> -- <one-line note if not OK>
S8: <OK|WARN|FAIL> -- <one-line note if not OK>
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

- Warp/weft scenarios stay in `SANDBOX-CORE-SUITE.md`, reed/tmux scenarios stay in `SANDBOX-REED-SUITE.md`;
  this suite grows with shuttle (a second engine, cluster reviews) -- add `S` scenarios here, not in either other suite.
- Creel scenarios go here as `C` scenarios beside the shuttle `S` ones, with a `**Covers:** creel` line.
