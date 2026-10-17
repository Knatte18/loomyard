# Creel stage 1 re-test, 2026-10-17

## Verdict

All of C1 to C13 are OK on `main` at `58a7feec7`, which holds `bf1df3859` (the eight re-test fixes) and `9306d886a` (reed down ends its watchdog; agents resolve lyx to the spawning binary).
The three WARN scenarios of 2026-10-16 (C4, C8, C10) now pass, and none is WARN or FAIL.
Every new finding of the 2026-10-16 report is fixed live, with three exceptions.
The dev-build `stencilstore` warning noise is still there and was never filed.
The llm smoke test (#678) and the C12 strand-name checks could not be checked in this run.
Three new defects were found and filed, all minor: a success envelope that cannot be written is lost with exit 0 (#699), the stderr fallback doubles its prefix (#700), and `log --id` leaves out reminders (#701).

## Environment

- Binary: `./deploy-dev` built `main` at `58a7feec7` into `.dev-bin/lyx`.
- Hub: `sandbox/posix/build.sh -reset` re-cloned `/home/knatte/Code/lyx-test-LYXHUB`, shortname `lyt`.
- Every sandbox command ran under `env -i` with HOME, USER, LANG, TERM and a fixed PATH headed by `.dev-bin`, which drops the live `TMUX`, strand name and `LYX_BIN`.
- tmux: the sandbox socket was `lyx-lyx-test-LYXHUB-e3c6a01f`, the live one `lyx-loomyard-LYXHUB-40e21604`.
- Creel: the sandbox daemon used `/tmp/lyx-1000/creel-lyt-e3c6a01f.sock`, and no live creel socket existed at any point.
- Strands, all in their own windows because reed no longer stacks panes (`654cecd6a`):
  - Claude strands: eve (plain `claude`), bob and alice (`claude --model haiku`).
  - Bash strands: carol, dave, frank, gus and `orch-2` (role orch).
  - Fake Claude-like panes: sink (Enter submits), stuck (Enter ignored), busy (always shows `esc to interrupt`) and an orch-role sink `lyt:orch`.
- No folder-trust dialog appeared, since the hub path was already trusted.
- Pane commands ran as `$LYX_BIN creel ...`, typed with tmux `send-keys` on the sandbox socket only.
- For the 3-row case of C13, alice was squeezed by splitting a 46-row `cat` pane under her on the sandbox socket; the pane was removed afterwards.
- Teardown:
  - `lyx reed down` ended the sandbox tmux server, and the sandbox reed watchdog exited with it, so no kill was needed.
  - The creel daemon was stopped by redeploying `.dev-bin/lyx`, and that redeploy revived no watchdog.
  - No sandbox tmux server, socket or process remains.
  - The hub directory `/home/knatte/Code/lyx-test-LYXHUB` is left in place with `lyx-test/sandbox-report.json` in it.
- The live hub was not touched: no window was opened in `loomyard`, nothing ran on its socket, and its watchdog was left alone.
  No creel, batten, loom, tender, orch or board verb ran against it.
- Gate runs:
  - `lyx gate test --tags tmux ./internal/creelcli` passed (1.7 s).
  - `lyx gate test ./internal/creelengine ./internal/creelcli` passed; creelcli has no untagged tests.
  - `lyx gate test --tags integration ./internal/creelengine ./internal/creelcli` passed as an extra run.
  - The llm tier was not run.

## Scenarios

| Scenario | Verdict | Evidence |
|---|---|---|
| C1 first verb and isolation | OK | The first `lyx creel inbox` started `.dev-bin/lyx creel daemon --hub-path /home/knatte/Code/lyx-test-LYXHUB` on `creel-lyt-e3c6a01f.sock`; the reed key `lyx-lyx-test-LYXHUB-e3c6a01f` differs from the live `lyx-loomyard-LYXHUB-40e21604`; no shared path. |
| C2 reminder timing | OK | Mail #4 was posted 4 s into bob's 81 s turn; `reminded` came at 09:55:14, about 3 s after the turn ended, and appears once in scrollback; bob then read it. |
| C3 backoff | OK | eve, told to answer reminders without reading, was reminded at 09:53:33.9, step 1 at 09:58:38.2 (+5:04) and step 2 at 10:08:39.4 (+10:01); new mail #44 was reminded 0.5 s after posting, with `unread:[2,44]`. |
| C4 read sets and ack | OK | Plain `read` returned EXTERNAL's #5 and #7, `--from lyt:carol` #6 and #8, `--all --from lyt:carol` #10 and #12, `--all` #9 and #11, then `nothing unread`; `inbox` counted 2 and 2 per sender. `read > /dev/full` exited 1 with `creel: creel read: could not write the messages, so they stay unread: ...` on stderr, and the mail stayed unread. |
| C5 reply chain | OK | `post --wait-reply` got dave's `--reply` (exit 0, `outcome:"reply"`); carol's #16, forwarded by dave to frank, was answered by frank and the reply reached carol; self, unknown-receiver, not-held and already-read forwards were refused; `--timeout 3s` exited 4 after 3.0 s. |
| C6 injects | OK | `inject --wait` into sink gave `injected` in 0.56 s; into stuck it gave `unconfirmed` (exit 2) after 30.6 s, the text was typed once, and nothing was retyped in 48 s after the box was cleared; a 20 s TTL into busy expired at 20.2 s; a 10 s TTL into bash strand dave expired at 10.2 s with `stall:"no-input-box"` and a stderr progress line. |
| C7 interrupt | OK | `interrupt` mid-turn gave `sent:true` and left the box empty; `inject --interrupt` 2 s and 9 s into a turn landed in 3.0 s each, and at 9 s Claude showed `Ctrl+Y to paste deleted text`, so creel had cleared the restored prompt; a box holding operator text was left alone, the turn was not interrupted, and the inject expired at its 25 s TTL with `stall:"draft"`. |
| C8 hold and pause | OK | EXTERNAL's hold on bob, and its renewal, worked; orch-2's inject and mail were held back while the owner's passed; orch-2's hold, interrupt and release were refused; after `release` orch-2's inject was typed and its mail reminded. An owner's `inject --wait` 39 s after a draft was cleared printed no stall line and ended `injected`. `tender pause` held an inject (TTL 60 s) and mail to sink for 111 s and refused `interrupt`; both arrived 2 s after `tender resume`. |
| C9 orch | OK | Mail to `lyt:orch` and `lyt:orch-2` was journaled `withheld` with `notice:"creel-notices"`; `inject` and `interrupt` were refused naming `lyx orch distill`/`refresh`; orch-2's `inbox` showed `withheld:true`; the fake orch pane received no keystroke. |
| C10 daemon life and journal | OK | A redeploy ended the daemon 1.4 s after the binary changed, with the socket removed; the next verb started a new daemon in 1.04 s and the `log` diff was empty. An 8-day-old day file was carried forward (unread #900, inject #902) and deleted, the read #901 was dropped, ids went on at 903, and `log --id 900` shows the `carried` event. A torn tail ending in a newline was cut (`bytes=40`), and the restart after the next append started cleanly; a torn line followed by two blank lines, a `[1]` line and a wrongly typed record were cut too. A malformed middle line gave the verb, in 0.02 s, the daemon log's quote of file and line 50. |
| C11 removed holder | OK | frank and gus were removed at 10:04:10, and the sweep settled at 10:04:29 (19 s): forwarded #38 `returned` to dave, carol's #39 `returned` to carol, EXTERNAL's #40 and #18 `undelivered`, the waiter on #41 exited 5 with `outcome:"returned"`, and the readers saw `ended_unread_by`/`ended_unread_at`. |
| C12 identity | OK (partial) | Pane senders resolved to `lyt:carol`, `lyt:dave`, `lyt:frank`, `lyt:orch-2`, and a terminal to `lyt:EXTERNAL`; a `setsid -f` child of carol's pane was refused with `LYX_STRAND_NAME names lyt:carol but the calling process runs in no strand pane`. The disagreeing-name and other-hub-name checks were not run (see the last section). |
| C13 collapsed and hand-added | OK | Plain `--cmd claude` eve took an inject in 0.5 s and was reminded 0.5 s after posting despite its `❯ Try "fix lint errors"` placeholder; alice squeezed to 3 rows got a `stalled` `too-short` event, `inject --wait` printed the too-short line with its way forward on stderr and ended `expired` at 20.1 s with `stall`, `stall_at` and `stall_reason`; once unsqueezed she was reminded 8 s later. |

## Status of earlier findings

| 2026-10-16 finding | Issue | Status | Evidence |
|---|---|---|---|
| 1 bad last line with newline skipped, not cut | #677 | Fixed | `cutting a torn last line ... bytes=40` at the start that skipped it; the next start after an append was clean. |
| 2 `read` to unwritable stdout silent | #681 | Fixed | stderr `creel: creel read: could not write the messages, so they stay unread: write /dev/stdout: no space left on device`, exit 1; the success paths of other verbs are still silent (#699). |
| 3 stale stall printed as current | #682 | Fixed | No stall line 39 s after the draft was cleared; with the draft still standing 34 s after the last reading, a new waiter was told the stall afresh (`at 12:12:16`) and its `expired` named it. |
| 4 verb waits 23 s for an exited daemon | #683 | Fixed | The malformed-journal error came after 0.02 s; six concurrent verbs against no daemon all succeeded within 1.2 s, five daemons started and one served. |
| 5 carried messages invisible to `log` | #684 | Fixed | `log --id 900` and `--id 902` return a `carried` event with its kind. |
| 6 removed-holder sweep took 80 s | #685 | Fixed | Settled 19 s after the removal. |
| 7 strand pane resolves `lyx` to production | #680 | Fixed for agents; bash panes as documented | bob's Bash tool printed `.dev-bin/lyx` for `command -v lyx`; in bash strand dave `command -v lyx` still names `/home/knatte/go/bin/lyx` because `~/.bashrc` prepends it, which `9306d886a` documents as a bound and the suite no longer checks. |
| 8 bash strand's expired inject gives no reason | #686 | Fixed | `stall:"no-input-box"` with a stall reason and a stderr progress line. |
| 9 `stencilstore` dev-build WARN on every verb | none | Open, unfiled | Every verb still prints `stencilstore: dev build does not refresh untouched stencils ...` on stderr. |
| 10 a stalled inject expired late | #686 | Fixed | TTLs of 10, 20, 20 and 25 s expired at 10.2, 20.2, 20.1 and 25.2 s. |
| llm smoke test `TestSmokeCreelClaudeStrand` | #678 | Not checked | The llm tier is the operator's to run. |
| Reed watchdog outlives `reed down` | #679 | Fixed | The sandbox watchdog was gone 2 s after `reed down`, and the redeploy that stopped creel revived none. |

## New findings

1. **WARN, a success envelope that cannot be written is lost with exit 0 (#699).**
   Seen: `post`, `inject` and `log` run from a terminal, and `inbox` run from a strand pane, each with stdout on `/dev/full`, exit 0 with nothing on stderr.
   The post (#907) and the inject were carried out, but their ids never reached the caller.
   Cause: the shared success path in `internal/creelcli/cli.go` (`output.Ok(out, res.fields)`) drops the write error.
   Expected: a `creel: ...` line on stderr and a non-zero exit, as for a refusal, naming the id for post and inject.
2. **Minor, the stderr fallback doubles its prefix (#700).**
   Seen: `lyx creel inbox > /dev/full` prints `creel: creel: an EXTERNAL caller holds no mail; ...`, and an unknown receiver prints `creel: creel: unknown receiver ...`.
   Expected: one `creel:` prefix.
3. **Minor, `log --id` leaves out reminders (#701).**
   Seen: `log --id 2` returned only `posted`, though three `reminded` records list id 2 in `unread`; `log --to lyt:eve` shows them.
   Expected: `--id` includes a `reminded` event whose `unread` lists the id, or its help says it does not.

Observations not filed:

- An EXTERNAL waiter's `returned` answer (C11, #41) carries no `ended_unread_by`/`ended_unread_at`, which only the reader's `read` shows; the suite asks this of readers only.
- In the 2-s `inject --interrupt` of C7, the interrupted prompt stayed in alice's transcript, so the model saw both prompts and ignored the injected one.
  Claude Code made that choice, and creel's line landed in 3.0 s as required.
- The torn-tail repair now cuts any unparseable last line of the newest file, including a complete, newline-terminated record of a known event with a wrongly typed field; `bf1df3859` chose this on purpose.

## Not run and why

- **The C12 disagreeing-strand-name and other-hub-name checks:** the harness's hook refuses any command naming the strand variable, and the suite makes this check operator-assisted; no operator was available.
- **The llm tier (`lyx gate test --tags llm ./internal/creelcli`):** the operator runs it by hand, so #678 stays unproven.
- **The suite's launcher `sandbox/posix/creel-suite.sh`:** it starts a separate agent session, and this re-test drove the scenarios directly.
  So no fingerprint header was prepended, and `sandbox-report.json` has no `meta`.
- **A stacked strand:** reed now gives every strand its own window, so the C13 3-row pane was made by splitting alice's window by hand.
- **Day rollover at midnight:** not forced live.
- **The S scenarios:** not run, being outside the creel scope.
