# SANDBOX-SHUTTLE-SUITE -- lym shuttle black-box agent suite

## What this is

A structured test loop that exercises `lym shuttle` against a **live tmux server and a logged-in claude** in the sandbox Hub's Fabric repo.
Like `SANDBOX-REED-SUITE.md`, it is partly **visual**: a strand's pane does real agent work, and an outcome comes back.
It is not an automated suite: an agent drives it, and an operator watches.

## Pre-conditions

Before starting a session:

1. **Deploy a fresh dev binary.**
   Run `deploy-dev` to build `lym.exe` into `.dev-bin` as current source.
   The suite resolves `.dev-bin` itself and prepends it to the agent's PATH, so no PATH setup is needed and production `lym` stays untouched.
   The fingerprint header's `Source: dev` line confirms the dev build is under test.
   The deployed binary is a snapshot: re-deploy after any source change you want to test.
   Once a pane exists, run these checks inside a spawned agent pane:
   - `command -v lym` (`where lym` on Windows), run from the spawned agent's Bash tool, names the spawning binary (`LYM_BIN`).
     It must not name the production install, even on a machine whose shell rc prepends another directory holding a `lym`.
   - `LYM_BIN` is set inside that pane and names the same binary.
   - `claude` still resolves inside that pane.
     `claude` is looked up by bare name from the pane's PATH, so it is the property most exposed to a change in how a pane's shell is started.
   All three are live-substrate facts no `go test` can prove, which is why they are pre-condition checks rather than assertions.
2. **Materialize the hub.**
   Run `sandbox/build.cmd` (or `sandbox/build.cmd -reset` to start clean);
   the session cwd is the Hub's Fabric repo root, the same operating model as the main suite.
3. **Live-tmux and claude requirement.**
   The session needs tmux (or the Windows tmux port) on PATH, PowerShell 7, and a logged-in `claude` on PATH.
   If any of these is unavailable, **note that as the session outcome rather than treating it as a shuttle defect**.
   The `**Covers:** shuttle` tag on S1 satisfies the sandbox coverage guard (`sandbox_coverage_test.go`) regardless of runtime availability.
4. **Wired worktree required.**
   `lym shuttle` requires a worktree wired by `lym fabric clone`/`lym fabric add`, exactly like `lym reed`.
   Wiring materializes `_lym/config/shuttle.yaml` and `reed.yaml` eagerly.

## Black-box rule

**The agent under test works exclusively inside the Hub's Fabric repo (`lym-test-LYMHUB/lym-test`).
It tests `lym.exe` as a black box -- exactly as a real user with only the binary on PATH.
It must not look for, read, or reason about the lym source tree.
No peeking at `C:\Code\loomyard\` or any other path outside the Hub.**

Discovering the command surface is done via `lym shuttle`, `lym shuttle <subcommand>`, and `lym shuttle <subcommand> --help` alone -- not from documentation outside the Hub.

### Controlled tmux exceptions

Sanctioned deviations from the pure black-box rule, as in the reed suite's controlled-exception note:

- **Direct `tmux -L <socket> list-panes`/`ls`** is allowed only to confirm a strand's pane exists (or was cleaned up).
  `<socket>` is read from the shuttle run's strand guid, cross-referenced against `lym reed status` output.
- **Scenario S2's operator attach** is operator-assisted: see S2 below.

## Fingerprint header

The launcher prepends a "binary under test" fingerprint block to this file when it copies the file into the Hub's Fabric repo.
The fingerprint records the absolute path, file size, modification time, and a short SHA-256 of the `lym.exe` binary at launch time.

The same fingerprint gives the report its provenance.
A separate fetch step, run after the session, stamps it into `meta.fingerprint` of the fetched `sandbox-report.json`,
so a maintainer can reproduce the exact binary that produced each finding.
The agent does not transcribe the fingerprint anywhere.

## How to run a scenario

For each scenario below:

- Read the **Goal** -- it names the task, not the commands.
  Discover the commands via `lym shuttle`, `lym shuttle <subcommand>`, and `--help` flags (S0 ethos).
- **Watch** what lym does.
  Note where it stalls, guesses wrong, or hits an error.
- Record the outcome per the verdict buckets: `OK` (worked) / `WARN` (rough edge) / `FAIL` (broke).

## Verdict key

- `OK`   -- completed without friction
- `WARN` -- completed but with confusion, awkward UX, or a non-fatal error
- `FAIL` -- did not complete;
  lym broke, panicked, or gave wrong output

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
- `ref` is the scenario id (`S1`-`S8`).
- `title` is a short one-line summary.
- `body` folds the detail, repro steps, and verdict into one markdown string.

Write only `source` and `items` -- a separate fetch step (run after the session) stamps `meta` (including the binary fingerprint).
Confine all free text to the `title`/`body` string fields so the JSON stays well-formed.

## Scenarios

### S1 -- Autonomous happy path

**Covers:** shuttle

**Goal:** "Ask a shuttle agent to write a specific file, and confirm the run reports `done` once it does."

**Watch:**
Run `lym shuttle run --prompt "write the single line OK into result.md and nothing else" --output-file result.md`.
It starts a strand, visible as a pane and confirmable via `lym reed status` or `tmux -L <socket> list-panes`.
The command blocks until the agent finishes.
The printed JSON envelope reports `"outcome":"done"` with a `sessionId` and `guid`, and `result.md` exists with the expected content.
Afterward the strand's pane and run directory are cleaned up: no leftover pane, and `lym reed status` no longer lists the guid.

**Verdict:** `OK` / `WARN` / `FAIL`

---

### S2 -- Hold path (operator-assisted)

**Covers:** shuttle

**Goal:** "Give a shuttle agent a task it cannot complete without a decision only the operator can make, and confirm the run keeps waiting at the agent's question.
Then let the operator answer it directly in the pane or by `lym creel post`, and confirm the run ends `done`."

**Watch:**
Run `lym shuttle run --prompt "before writing decision.md, stop and ask me which of two options you should pick — do not guess" --output-file decision.md --interactive`.
It does not return when the agent ends its turn with its question: the run is held.
The strand and its pane are still alive: `lym reed status` still lists the guid, and `decision.md` does not exist yet.
The agent then instructs the operator to attach (`lym reed attach` in a second terminal, per the reed suite's M7/M14 pattern),
and to answer the question in the pane or by `lym creel post <strand> "<answer>"`.
Confirm that the agent continues and eventually writes `decision.md`, and that the original `lym shuttle run` returns `"outcome":"done"`.

**Verdict:** `OK` / `WARN` / `FAIL`

---

### S3 -- Interrupt and post (cross-terminal poke)

**Covers:** shuttle

**Goal:** "Start a long-running shuttle agent from one terminal.
From a second terminal, interrupt its current turn and post it a one-line update, and confirm the agent continues from the new instruction."

**Watch:** Start a long-running run in one terminal, for example:
`lym shuttle run --prompt "count slowly to a very large number out loud, one number per line, before writing done.md" --output-file done.md`.
From a second terminal, note the strand name (via `lym reed list`) and run `lym creel interrupt <strand>`.
The agent's current turn stops without killing its pane or session: `lym reed status` still shows it `live: true`.
Then run `lym creel post <strand> "stop counting and write done.md right away"`, a single-line update only.
The property to verify is that `done.md` eventually appears with the redirected content, and that the first terminal's envelope reports `"outcome":"done"`.
The interrupted turn's own Stop is a hold, and only the redirect turn's completion ends the run.
Any other outcome (or a missing or wrong `done.md`) is a real failure here.

**Verdict:** `OK` / `WARN` / `FAIL`

---

### S4 -- One envelope per invocation, even when two things are wrong at once

**Covers:** shuttle

**Goal:** "Make a `lym shuttle run` invocation fail for TWO independent reasons at once.
Confirm it still answers with exactly one JSON object, naming the one to fix first."

**Watch:** From a directory that is NOT a git repository (e.g. a fresh `mkdir` under the system temp dir), run `lym shuttle run --output-file out.md`.
Geometry resolution fails AND no `--prompt`/`--prompt-file` was given, so both the pre-flight and the flag check have something to say.
The command must print **exactly one** JSON object, and it must be the pre-flight one (`"error":"not a git repository"`).
That is the problem the operator has to fix before the flag error can be evaluated.
Two objects on one invocation is a FAIL, not a cosmetic issue:
a caller that parses the output as a single JSON object gets a parse error instead of the real cause.
Repeat with `--prompt a --prompt-file b --output-file out.md` (mutually exclusive flags) for the same result.
Then, from inside a real lym worktree, run `lym shuttle run --output-file out.md` and confirm the flag error IS reported on its own there:
suppressing it only when the pre-flight already failed is the point, and suppressing it always would be a different bug.
This scenario starts no agent and costs no provider tokens.

**Verdict:** `OK` / `WARN` / `FAIL`

---

### S5 -- Reed's bookkeeping is reset under a run that is still working

**Covers:** shuttle

**Goal:** "Take reed's strand table away from underneath a live shuttle run.
Confirm shuttle says it does not know rather than declaring the agent dead, and that it does not delete the live run's directory."

**Watch:** Start a run that stays inside ONE turn for several minutes, e.g.
`lym shuttle run --prompt "run python3 -c 'import time; time.sleep(100); print(1)' in the foreground three times in a single reply, then write done.md" --output-file done.md --model haiku`.
Wait until `lym reed status` shows the strand `live: true` and the pane visibly shows the agent working.
The pane shows a running tool call with a rising elapsed counter; do not proceed while it is still booting.
Then, from a second terminal, delete reed's state file: `rm <worktree>/.lym/reed.json`.
This is not vandalism: it is the second remedy reed's own corrupt-state error recommends (`delete <path> by hand to keep the session for now`),
and it is what a `git clean -xdf` in the worktree does.

Three things must hold:

1. The blocked `lym shuttle run` must return `"ok":false` with an error saying reed no longer tracks the strand and that the agent may still be working.
   It must **never** return `"ok":true` with `"outcome":"died"`.
   A `died` envelope here is a FAIL: it tells an unattended caller to respawn.
   The respawn puts a second agent on the worktree while the first keeps running unreachably.
2. The envelope must still carry `guid`, `sessionId` and `runDir`, so the operator can reach the pane that is still running.
3. Confirm the agent is still alive afterward: `tmux -L <socket> capture-pane` on its pane shows the same turn still progressing.
   This is the point of the verdict in (1).

Then, with `reed.json` still absent, start a SECOND run in the same worktree.
Use a cheap prompt, e.g. `--model haiku --prompt "write ok" --output-file second.md`.
`add strand` self-heals rather than refusing, so this second run boots a fresh session and spawns a real second agent;
**this step costs real tokens**.
Confirm the FIRST run's directory under `<worktree>/.lym/shuttle/` is **still there** once the second run has started.
A missing run directory is a FAIL.
It holds the `events.jsonl` the live agent's Stop hook is still appending to.
It also holds the `run.json` without which `lym creel interrupt` and `lym creel post` can no longer find the running agent.

Restore with `lym reed up`, which writes a fresh `reed.json`, and tear the session down with `lym reed down`.

**Verdict:** `OK` / `WARN` / `FAIL`

---

### S6 -- Re-sending the same text into a pane that has scrolled

**Covers:** shuttle

**Goal:** "Send one instruction twice into a busy agent pane, with the first copy scrolled to the top of the viewport.
Confirm the second send is delivered exactly once and reported as delivered."

**Watch:** Start a run with `--keep-pane` and a trivial prompt, for example:
`lym shuttle run --prompt "write READY into ready.txt" --output-file ready.txt --keep-pane --model haiku`.
It leaves one live, idle agent pane to drive.
Note the strand name from `lym reed list`.

Send an instruction whose ANSWER is long enough to fill the pane, for example:
`lym creel post <strand> "reply with the numbers 1 to 38 one per line and nothing else"`.
Wait for the reply to finish, then look at the pane with `tmux -L <socket> capture-pane -p -t <paneId>`.
The line echoing your instruction must have been pushed to within a line or two of the TOP of the viewport, still visible.
That is the setup; if it scrolled off entirely, send it once more and re-check.

Now send the SAME instruction again, timing it while the earlier copy is still visible at the top.
Two things must hold:

1. The command answers `"ok":true` within about a second.
   An `"ok":false` "the send was NOT delivered" here is a FAIL; the giveaway is that it takes ~11 s to say it.
2. The pane must show the instruction accepted exactly ONCE more, not twice.
   A second, duplicate copy of your instruction (and a second answer to it) is the same FAIL seen from the other side:
   the delivery check gave up and re-typed a message the agent had already received.

The earlier copy scrolling off as the new one arrives leaves the occurrence COUNT unchanged.
That is indistinguishable from nothing arriving unless position is taken into account.
Tear down with `lym reed remove <guid>` and `lym reed down`.

**Verdict:** `OK` / `WARN` / `FAIL`

---

### S7 -- Reed drops the pane binding of a run that is still working

**Covers:** shuttle

**Goal:** "Make reed decide this worktree's pane bindings are stale while a shuttle run is mid-turn.
Confirm shuttle says it can no longer address the agent rather than declaring it dead."

**Watch:** This is S5's sibling: there reed forgets the strand, here reed keeps the strand but forgets its PANE.
Start the same kind of run as in S5, one that stays inside ONE turn for several minutes.
Wait until the pane visibly shows the agent working.

Then make the persisted pane generation stale.
Reed's own doc describes this state as "a reed.json older than the session now running"; it is reachable in the wild from a restored backup or a copied `.lym`.
In `<anchor>/.lym/reed.json`, change the `paneGeneration.created` value to any older number.
Write the file atomically: write a temp file beside it and rename it over the original, so reed never reads a half-written file.

Reed then logs `persisted pane bindings were minted against a different tmux session incarnation, clearing them`.
`lym reed status` reports the strand with an empty `paneId` and `live: false`, while `tmux list-panes` still shows its pane alive with the agent inside it.

Two things must hold:

1. The blocked `lym shuttle run` must return `"ok":false` saying reed holds no pane binding for the strand and that the agent may still be working.
   It must **never** return `"ok":true` with `"outcome":"died"`.
   A `died` envelope here is the same FAIL as S5's, for the same reason: it tells an unattended caller to respawn onto a worktree whose agent is still running.
2. `lym creel interrupt <strand>` must refuse.
   Its message must NOT claim the run reached a terminal outcome or that its pane died, because neither is true here.

Restore the original `created` value to make the binding usable again.
`lym reed status` shows the strand `live: true` on its pane once more, which also proves the agent was alive throughout.
Then run `lym reed remove <guid>` and `lym reed down`.

**Verdict:** `OK` / `WARN` / `FAIL`

---

### S8 -- Session state of a running agent

**Covers:** shuttle

**Goal:** "Start an interactive shuttle agent.
Read what `lym shuttle state` says its session is doing, while it works and after it asks its question.
Narrow the list to the one run, and confirm every printed state matches what the pane shows."

**Watch:** Start the S2 run:
`lym shuttle run --prompt "before writing decision.md, stop and ask me which of two options you should pick — do not guess" --output-file decision.md --interactive`.
From a second terminal, `lym shuttle state` prints one JSON envelope.
Its `runs` lists the run with its `strand`, `guid`, `state`, `cause`, `detail`, `outstanding`, `since` and `history`.
While the pane shows the agent working, the run reads `busy` with cause `turn`.
After the agent ends its turn with its question, the run reads `asking` with cause `awaiting-input`.
That is because the run is interactive and `decision.md` does not exist yet.
Its `since` is the time the turn ended.
`lym shuttle state --strand <strand name or guid>` lists only that run.
`--strand no-such-strand` prints an empty `runs` list with `"ok":true`.
Reading the state changes nothing: the pane, the run and `lym reed status` are the same before and after.
A state the pane contradicts (for instance `idle-done` while the agent still works) is a real failure here.
The state model is in shadow mode, so a mismatch is a finding about the reading, never about the run's own behavior.

**Verdict:** `OK` / `WARN` / `FAIL`

## Session log format

After running all scenarios, record a short session summary:

```
Date: <YYYY-MM-DD>
Binary fingerprint: <copy from the header above>

S1: <OK|WARN|FAIL> -- <one-line note if not OK>
S2: <OK|WARN|FAIL> -- <one-line note if not OK>
S3: <OK|WARN|FAIL> -- <one-line note if not OK>
S4: <OK|WARN|FAIL> -- <one-line note if not OK>
S5: <OK|WARN|FAIL> -- <one-line note if not OK>
S6: <OK|WARN|FAIL> -- <one-line note if not OK>
S7: <OK|WARN|FAIL> -- <one-line note if not OK>
S8: <OK|WARN|FAIL> -- <one-line note if not OK>

sandbox-report.json written: <count of WARN/FAIL items>
```

`./sandbox-report.json` must be written before the session ends, per the Capturing findings section above.
When every scenario was `OK`, it holds `items: []`.

## Notes

- Warp/weft scenarios belong in `SANDBOX-CORE-SUITE.md`, and reed/tmux scenarios in `SANDBOX-REED-SUITE.md`.
  Add shuttle scenarios here as `S` scenarios, not in either other suite.
- Creel scenarios live in `SANDBOX-CREEL-SUITE.md`.
