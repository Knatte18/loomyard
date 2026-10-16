# Creel stage 1 re-test, 2026-10-16

## Verdict

All eight defects from the 2026-10-15 audit (#657 to #664) are fixed in live use.
Ten of C1 to C13 are OK, and three (C4, C8, C10) are WARN for rough edges; none is FAIL.
One known gap remains: the llm-tier smoke test `TestSmokeCreelClaudeStrand` still fails with the same message as before, although a hand-added Claude strand now gets its reminder within a second in the sandbox.
The worst new finding is in the journal: a bad last line that ends in a newline is skipped but not cut, and the next start then refuses the journal until someone fixes it by hand.

## Environment

- Binary: `./deploy-dev` built `main` at `1cf2b2726` (which contains `27887027b`) into `.dev-bin/lyx`.
- Hub: `sandbox/posix/build.sh` (the Linux counterpart of `sandbox/build.cmd`) cloned `/home/knatte/Code/lyx-test-LYXHUB`, shortname `lyt`.
- Every sandbox command ran under `env -i` with a fixed PATH headed by `.dev-bin`, which drops the live `TMUX`, strand name and `LYX_BIN`.
- tmux: the sandbox socket was `lyx-lyx-test-LYXHUB-e3c6a01f`, the live one `lyx-loomyard-LYXHUB-40e21604`.
- Creel: the sandbox daemon used `/tmp/lyx-1000/creel-lyt-e3c6a01f.sock`, and no live creel socket existed at any point.
- Strands: alice (plain `claude`, stacked, 2 rows), eve (plain `claude`, own window), bob (`claude --model haiku`, own window), bash strands carol, dave, frank, gus and `orch-2` (role orch), and fake Claude-like panes sink, stuck, busy and an orch-role sink `lyt:orch`.
- The folder-trust dialog of the first Claude strand was answered by hand with tmux `send-keys`, standing in for the operator.
- Teardown: `lyx reed down`, the daemon stopped by redeploying `.dev-bin/lyx`, and the sandbox reed watchdog, which outlived `reed down` and respawned on the new binary, killed by pid after its command line was checked.
  No sandbox tmux server, socket or process remains.
  The hub directory `/home/knatte/Code/lyx-test-LYXHUB` is left in place with `lyx-test/sandbox-report.json` in it.
- The live hub was not touched: no window was opened in `loomyard`, and no creel, batten, loom, tender, orch or board verb ran against it.
- Gate runs: `lyx gate test --tags tmux ./internal/creelcli` passed; `lyx gate test --tags llm ./internal/creelcli` failed (see #657).

## Scenarios

| Scenario | Verdict | Evidence |
|---|---|---|
| C1 first verb and isolation | OK | The first `lyx creel inbox` started `.dev-bin/lyx creel daemon --hub-path /home/knatte/Code/lyx-test-LYXHUB` on `creel-lyt-e3c6a01f.sock`; the reed key `lyx-lyx-test-LYXHUB-e3c6a01f` differs from the live `lyx-loomyard-LYXHUB-40e21604`; no shared path. |
| C2 reminder timing | OK | Mail #22 was posted 8 s into bob's turn; `reminded` came at 08:37:36, after the turn ended, and appears once in scrollback. The turn was short because the user's hook refused a foreground `sleep` in the agent. |
| C3 backoff | OK | eve, which answers reminders without reading, was reminded at 08:37:01, step 1 at 08:42:05 (+5:04) and step 2 at 08:52:06 (+10:01); new mail #910 was reminded 0.5 s after posting. |
| C4 read sets and ack | WARN | Plain `read` returned EXTERNAL's #8 and #10 only, `--from lyt:carol` returned #9 and #11, `--all --from` narrowed to carol, `--all` returned the rest, and `inbox` counted per sender. `read > /dev/full` exited 1 and the mail stayed unread, but nothing was printed on stderr. |
| C5 reply chain | OK | `post --wait-reply` got dave's `--reply` (exit 0); carol's #17, forwarded by dave to frank, was answered by frank and the reply reached carol; self, already-read, not-held and unknown-receiver forwards were refused; `--timeout 3s` exited 4. |
| C6 injects | OK | `inject --wait` into sink gave `injected`; into stuck (Enter ignored) it gave `unconfirmed` after 30 s, the text was typed once, and nothing was retyped in 40 s after the box was cleared; a 30 s TTL into a never-idle pane expired at 31.4 s. |
| C7 interrupt | OK | `interrupt` mid-turn gave `sent:true`, and Claude's box showed `Ctrl+Y to paste deleted text`, so creel had cleared the restored prompt; `inject --interrupt` 2 s and 9 s into a turn landed in 3.0 s and 4.7 s; a box holding operator text was left alone, and the inject expired with `stall:"draft"`. |
| C8 hold and pause | WARN | Hold, renew and release worked, `orch-2`'s inject and mail were held back while the owner's passed, and orch-2's hold, interrupt and release were refused; `tender pause` held an inject (TTL 60 s) and mail for 103 s, `interrupt` was refused, and both arrived after `tender resume`. The owner's inject printed a draft stall from 37 s earlier as if current. |
| C9 orch | OK | Mail to `lyt:orch` and `lyt:orch-2` was journaled `withheld`; `inject` and `interrupt` were refused naming `lyx orch distill`/`refresh`; orch-2's `inbox` showed `withheld:true`, `notice:"creel-notices"`; the fake orch pane received no keystroke. |
| C10 daemon life and journal | WARN | A redeploy ended the daemon about 1.6 s later with the socket removed; the next verb started a new daemon, and the `log` diff showed only the shutdown `aged` record and a re-evaluated `stalled`; an 8-day-old day file was carried forward (unread #900 and inject #902) and deleted, the read #901 was dropped, ids went on at 903, and a torn tail with no newline was cut; a malformed middle line gave the verb the daemon log's quote of the file and line. See the new findings. |
| C11 removed holder | OK | After frank and gus were removed, the sweep took 80 s, not about 60: #900 from EXTERNAL went `undelivered`, forwarded #905 `returned` to dave, carol's #906 `returned` to carol, the waiter on #907 exited 5 with `outcome:"returned"`, and the readers saw `ended_unread_by`/`ended_unread_at`. |
| C12 identity | OK (partial) | Pane senders resolved to `lyt:carol`, `lyt:dave`, `lyt:frank`, `lyt:orch-2`, and a terminal to `lyt:EXTERNAL`; a `setsid -f` child of carol's pane was refused with `LYX_STRAND_NAME names lyt:carol but the calling process runs in no strand pane`. The disagreeing-name and other-hub-name checks were not run (see the last section). |
| C13 collapsed and hand-added | OK | Plain `--cmd claude` eve was reminded 0.5 s after posting despite its `❯ Try "..."` placeholder; collapsed alice (2 rows) got a `stalled` `too-short` event, and `inject --wait` printed `creel: lyt:alice is stalled: the pane is too short to show the input box; way forward: ...` on stderr and ended `expired` with `stall`, `stall_at` and `stall_reason`. |

## Status of #657 to #664

| Issue | Status | Evidence |
|---|---|---|
| #657 hand-added Claude never read idle | Fixed live; smoke test still fails | eve (`lyx reed add --cmd claude`, placeholder `❯ Try "fix lint errors"`) logged `reminded` 0.5 s after mail #1 was posted, and read it. `lyx gate test --tags llm ./internal/creelcli` still fails: `smoke_llm_test.go:51: the adapter never read the booted session idle` (60.7 s). The test prints no pane capture, so the cause is unknown; one candidate is Claude's folder-trust dialog in the test's temp hub, which the sandbox also showed on first boot. |
| #658 no delivery into a 3-row pane, no reason | Fixed | Journal `{"event":"stalled","to":"lyt:alice","reason":"too-short"}`; `inject --wait` stderr `creel: lyt:alice is stalled: the pane is too short to show the input box; way forward: give the strand's pane more rows ...`; envelope `"outcome":"expired","stall":"too-short"`. |
| #659 inject waits behind the restored prompt | Fixed | `inject --interrupt` 2 s into a turn: `{"id":27,"outcome":"injected"}` in 3.0 s; the interrupted prompt #26 never reached the transcript, and EARLY was answered. |
| #660 read acks when the write failed | Fixed | `read > /dev/full` exit 1; the following `inbox` still showed `lyt:EXTERNAL` 2 and `lyt:carol` 2, and the next `read` returned #8 and #10. |
| #661 daemon start failure invisible | Fixed | The verb's error now quotes the daemon log: `creel journal: .../2026-10-16.jsonl line 51 is not a valid record (invalid character 'h' ...); way forward: repair or remove that line by hand`. It still takes about 23 s to say so. |
| #662 TTL charged before post | Fixed | Inject #36 posted at 08:44:20.48 had a shutdown record of `consumed_s:8` at 08:44:29.35; new posts carry no `consumed_s`; after restarts the TTL counts only daemon uptime (25 s at 38.9 s wall time with 13 s down). |
| #663 returned message not marked | Fixed | `read --from lyt:carol` returned #905 with `"ended_unread_by":"lyt:frank","ended_unread_at":"2026-10-16T08:49:23Z"`. |
| #664 small inconsistencies | Fixed | `reminded` records carry `unread:[...]` and the journal has no `"id":0`; `inject --help` says "any other strand on its own strand only"; `--wait` envelopes carry `"sender":"lyt:EXTERNAL"`; `release lyt:nosuch` is refused with the names that exist. |

## New findings

1. **WARN, journal: a bad last line that ends in a newline is skipped, not cut, and later stops the daemon from starting.**
   Seen: with `{"event":"posted","at":"2026-10-16T08:45:1` plus a newline as the last line, the daemon logged `skipping a torn last line left by a crash ... line=101` and appended new records after it.
   At the next start that line sat in the middle of the file, and the daemon refused it: `line 101 is not a valid record (unexpected end of JSON input); way forward: repair or remove that line by hand`.
   Creel was then down until the line was removed by hand.
   Expected: the skipped line is cut, as a torn tail without a newline is (`cutting a torn last line ... bytes=42`), so a later start does not fail on it.
2. **WARN, `read` to an unwritable stdout fails silently.**
   Seen: `lyx creel read > /dev/full 2>err` exits 1 and `err` is empty.
   Expected: a short error on stderr, since the JSON error envelope cannot be written either.
3. **WARN, a stale stall is printed as current.**
   Seen: the owner's `inject --wait` into bob printed `lyt:bob is stalled: the input box holds a draft ... (at 10:39:55)` 37 s after the draft was cleared, then injected at once.
   Expected: no stall line once the box is empty, or one clearly marked as past.
4. **WARN, the verb waits about 23 s for a daemon that has already exited.**
   Seen: against a malformed journal the daemon died at once (daemon log 10:46:01.395), but `lyx creel inbox` returned its (now useful) error after 23 s.
   Expected: the error as soon as the started daemon has exited.
5. **WARN, carried-forward messages are invisible to `log`.**
   Seen: the journal holds `{"event":"carried","id":900,...}` and frank's `inbox` counts it, but `lyx creel log --id 900` returns `[]`.
   Expected: `log` shows a carried message, at least as a `carried` event.
6. **WARN, the removed-holder sweep took 80 s.**
   Seen: strands removed at 08:48:03, `returned`/`undelivered` at 08:49:23.
   Expected per C11: within about 60 s.
7. **WARN, suite pre-condition: a strand pane resolves `lyx` to the production binary.**
   Seen: in dave's pane `command -v lyx` names `/home/knatte/go/bin/lyx`, while `LYX_BIN` names `.dev-bin/lyx` and `claude` resolves.
   The pane's PATH begins `/home/knatte/go/bin:` because `~/.bashrc` prepends it, so a Claude strand that runs bare `lyx creel read` talks to the sandbox daemon through the production binary.
   Production (`origin/prod` = `831196a12`) contains the fixes, so no verdict above depends on this, but the suite's first check fails on this machine.
   I ran every pane command through `$LYX_BIN`.
8. **Minor, a bash strand's expired inject gives no reason.**
   Seen: `inject lyt:dave ... --ttl 10s --wait` ended `expired` with no `stall` field, while Claude-adapter panes now name theirs.
   Expected: a reason such as "not an agent pane", or the outcome documented.
9. **Minor, noise: every dev-build verb prints a `stencilstore: dev build does not refresh untouched stencils` WARN on stderr.**
10. **Minor: a stalled inject expired late.**
    A 20 s TTL into the too-short pane expired at 23.3 s; the never-idle pane's 30 s TTL expired on time at 31.4 s.

## Not run and why

- **The C12 disagreeing-strand-name and other-hub-name checks:** the harness refuses any command naming the strand variable, the suite makes this check operator-assisted, and no operator was available to this agent.
- **`.cmd`/`.exe` names:** the suite's `sandbox/build.cmd`, `deploy-dev` (`lyx.exe`) and `where lyx` have Linux counterparts (`sandbox/posix/build.sh`, `./deploy-dev`, `.dev-bin/lyx`, `command -v lyx`), which were used; none was missing.
- **The suite's launcher `sandbox/posix/shuttle-suite.sh`** was not used: it starts a separate agent session, and this audit drove the scenarios directly.
  So no fingerprint header was prepended, and `sandbox-report.json` has no `meta`.
- **The C2 long turn:** a real 45 s foreground turn could not be made, because the user's Claude hook refuses a foreground `sleep` inside the agent; the post still landed mid-turn.
- **The C13 3-row case:** the stacked Claude strand was squeezed to 2 rows, not 3, so the exact 3-row height was not tested; the 3-row stacked strand (carol) is bash.
- **Day rollover at midnight:** not forced live.
- **The S scenarios:** not run, being outside the creel scope.
