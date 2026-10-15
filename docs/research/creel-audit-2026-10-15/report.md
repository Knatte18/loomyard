# Creel stage 1 audit, 2026-10-15

An audit of what `creel-core` (`993ff3fc5`) built, run by an orch agent in a closed sandbox hub, per board note `creel-test`.
It changed no code; its defects are filed as issues #657 to #664 and taken onto the board as notes.

Seven defects and one batch of small inconsistencies were found.
The worst two hit end-to-end delivery: a Claude strand added by hand is never reached, and that is also why the llm-tier smoke test fails.
Every other check worked, and three of the four gate runs passed.

## Environment and isolation

- **Binary:** built from `main` at e131c7625 into the agent's scratchpad, not the production `lyx`.
- **Repos:** a throwaway local pair, two bare repos in the scratchpad with no GitHub remote.
- **Hub:** `lyx fabric clone --shortname ctest` made `scratchpad/hubs/warp-LYXHUB`.
- **Clean environment:** every sandbox command ran with `env -i` and a fixed PATH, dropping the inherited live-hub variables (`TMUX`, the strand name, `LYX_BIN`).
- **tmux:** the sandbox server was `/tmp/tmux-1000/lyx-warp-LYXHUB-41ca7726`, the live one `/tmp/tmux-1000/lyx-loomyard-LYXHUB-40e21604`; both were compared read-only before the first creel verb.
- **Creel socket:** the sandbox used `/tmp/lyx-1000/creel-ctest-41ca7726.sock`. The live hub (shortname `ly`) had no creel socket or daemon running at any point.
- **Smoke tests:** their helpers were read first. `tmuxkit.Main` gives them a private `TMUX_TMPDIR`, `hubforge` a temp hub, and the daemon runs in process, so they are isolated.
- **No contact with the live hub:** the session `loomyard` was never touched, no windows were opened, and no verb ran against it. `tender pause` ran only in the sandbox.
- **Teardown:** `lyx reed down`, the creel daemon stopped by replacing its binary, and the sandbox reed watchdog killed by pid after checking its command line. The hub, remotes, seed and scratch binaries were then removed; no sandbox process or socket remained.
- **Strands used:** two Claude Haiku strands, alice (default settings) and bob, both stacked in the prime's window, and eve in its own window. Bob and eve ran with `promptSuggestionEnabled:false`.
- **Test fixtures:** bash strands carol, dave and `orch-2`; a fake idle pane that swallows input (sink, and an orch-role copy `ctest:orch`); a fake input box that ignores Enter (stuck, stuck2).

## Checks

- **FAIL**: a strand added with plain `lyx reed add --cmd claude` is never read idle. Claude's greyed placeholder (`❯ Try "edit <filepath> to..."`) counts as a draft, so nothing was typed in 20+ minutes. With suggestions switched off, delivery works within a second. (#657)
- **FAIL**: a strand whose pane reed has squeezed to 3 rows (any stacked strand but the bottom one) gets nothing. The "too short" signal is dropped, no reason is shown, and the inject runs out its TTL and is journaled `expired`. (#658)
- **FAIL**: after `interrupt`, or `inject --interrupt` 9 s into a turn, Claude puts the interrupted prompt back in its input box. Creel then reads the strand as busy, so the queued inject waits until the box is cleared by hand or the TTL ends. (#659)
- **FAIL**: `lyx creel read > /dev/full` exits 0 and marks the mail read, so the message is lost. The write error is ignored before the acknowledgement. (#660)
- **WARN**: when the daemon cannot start (a malformed journal line), verbs say only "no daemon answers … after 6 connects" after about 30 s. The real error, naming the file and line, shows only when the hidden daemon is run in the foreground. (#661)
- **WARN**: a new inject is charged TTL for time before it was posted, for example `consumed_s:3` 30 ms after posting, and 28 s after 23 s. (#662)
- **WARN**: a message returned by the sweep reads with no mark that it bounced or from whom. (#663)
- **WARN**: several small inconsistencies (#664):
  - `log` shows `reminded` events with `"id":0` and no message ids.
  - `inject --help` says no plain strand may inject, yet injecting into its own pane works, as `doc.go` says.
  - The `--wait` verbs print an empty `sender`.
  - `release` on an unknown name answers ok.
- **OK** `read` sets in `Store.Read`, with mail #3 to #8:
  - Plain `read` returned the oldest sender's two messages only.
  - `--from ctest:dave` returned dave's two messages.
  - `--all --from` narrowed to that sender; `--all` returned the rest.
  - `inbox` counted per sender in name order and marked nothing read.
  - The live sequence (inbox, then a plain read of the oldest sender) is consistent. The plan-review finding `read-sets-inbox-sequence-ambiguous` was not found, so this is judged against `doc.go`.
- **OK** `post`, `--reply`, `--wait-reply`, `--timeout`:
  - A reply reached the waiting EXTERNAL sender.
  - A timeout exits 4.
  - A reply to EXTERNAL with no one waiting is journaled `undelivered`.
  - A message removed from under a waiter exits 5 with `returned`.
- **OK** `forward`: the path is recorded. Forwarding to yourself, to an unknown name, a message you don't hold, or one already read is refused.
- **OK** `inject --wait` gives `injected` (exit 0) or `unconfirmed` (exit 2). The TTL ceiling of 1 h is enforced. A 30 s self-inject into a bash pane expired on time. Bob's 600 s inject expired after the paused stretch was excluded.
- **OK** an unlanded inject is journaled `sending` then `unconfirmed`, and was not typed again during 40 s with the box empty.
- **OK** a reminder was typed only after the busy turn ended, appeared once per occurrence (3 in eve's scrollback), and backed off: 16:13:25, then 16:18:29 (step 1), then 16:28:33 (step 2). A new unread set was reminded at once. Subjects are cut at 80 runes and `«»` inside them are neutralised.
- **OK** orch strand: mail is stored with a `withheld` record and nothing was typed. `inject` and `interrupt` are refused. An orch caller's `inbox` shows `withheld:true` with the notice `creel-notices`.
- **OK** `hold`/`release`:
  - Another sender's inject and mail were held back while the owner's inject passed.
  - Holding again renewed the hold.
  - Another sender's hold, interrupt or release was refused.
  - After `release`, the held inject was typed and the mail reminded.
- **OK** `interrupt` on a running turn reported `sent:true`; on an idle pane `sent:false`. Refused under a pause, a hold, and for an orch.
- **OK** `lyx tender pause` held both the inject and the reminder, froze a 60 s TTL across 112 s, and refused `interrupt`. After `tender resume` both were delivered.
- **OK** the daemon is started by the first verb. It survives a killed `read --wait`, and that mail stays unread.
- **OK** replacing the binary ends the daemon in about 2 s with the socket removed, and the next verb starts the new one.
- **OK** after a restart the journal folds to the same pool: the `log` was identical apart from one TTL record written at shutdown. The reminder backoff and TTL accounting carried across two restarts.
- **OK** retention and journal repair, tested with a crafted 2026-10-01 day file:
  - Its unread mail and queued inject were carried forward, its read mail dropped, and the file deleted.
  - New ids continued at 903, above the crafted ids.
  - A torn last line was cut off on start, and an unknown event name was ignored.
  - Day rollover itself could not be forced live; unit tests cover it.
- **OK** removed holders: within about 52 s, mail from EXTERNAL was journaled `undelivered`, a strand sender's mail went back to that sender, and a forwarded message went back to the last forwarder.
- **OK** identity:
  - Panes resolve to their own strand names by process ancestry.
  - A plain terminal posts as `ctest:EXTERNAL`.
  - A process detached from carol's pane but still carrying its strand name is refused.
  - A process with another hub's strand name in its environment posts as EXTERNAL.
- **WARN**: the live check that a process inside one pane claiming another strand's name is refused was not run, since the agent's harness refuses commands that name the strand variable; it rests on the unit case `Linux_DisagreeingStrandNameIsRefused`.

## Gate runs

| Run | Result |
|---|---|
| `lyx gate test --tags tmux ./internal/creelcli` | PASS; `TestSmokeCreelFakeProviderPane` ran (1.78 s) |
| `lyx gate test --tags llm ./internal/creelcli` | FAIL twice: `TestSmokeCreelClaudeStrand` stops at `smoke_llm_test.go:51: the adapter never read the booted session idle` (same cause as #657) |
| `lyx gate test ./...` | PASS, 158 packages |
| `lyx gate test --tags integration ./...` | PASS, 158 packages |

## Issues filed

| Issue | Board note | Defect |
|---|---|---|
| #657 | `creel-hand-added-claude-idle` | Hand-added Claude strand never read idle; llm smoke test fails |
| #658 | `creel-collapsed-pane-delivery` | No delivery into a 3-row pane, no reason shown |
| #659 | `creel-interrupt-restored-prompt` | `inject --interrupt` stalls behind the restored prompt |
| #660 | `creel-read-ack-on-write-error` | `read` marks mail read when writing it out failed |
| #661 | `creel-daemon-start-error` | Daemon start failure is invisible to verbs |
| #662 | `creel-inject-ttl-before-post` | New inject charged TTL from before it was posted |
| #663 | `creel-read-returned-mark` | `read` does not show a returned message bounced |
| #664 | `creel-output-inconsistencies` | Log, help, sender field and `release` inconsistencies |

## Proposed creel sandbox suite

Each scenario gives what to watch and its verdict; none is written into the sandbox suite yet.

- **C1, first verb and isolation.** Watch: the socket name carries the sandbox shortname and hash, the sandbox reed `-L` key differs from the live one, and the daemon's binary is the sandbox build. Verdict: FAIL if any path is shared with the live hub.
- **C2, reminder timing.** Watch: busy a Claude strand with a long inject, post mid-turn; the reminder lands only after the turn and appears once in scrollback. Verdict: FAIL on a reminder typed into a busy turn, or a duplicate.
- **C3, backoff.** Watch: the strand answers reminders without reading; `reminded` steps come about 5 and 10 minutes apart, and a new message is reminded at once. Verdict: WARN if timing drifts by more than 30 s.
- **C4, read sets and ack.** Watch: interleaved mail from EXTERNAL and a shell strand; check plain read, `--from`, `--all` and `inbox`; `read > /dev/full` leaves the mail unread. Verdict: FAIL on a wrong set or lost mail.
- **C5, reply chain.** Watch: `post --wait-reply` answered by `--reply`; a forward then reply goes back to the sender; refusals for self, not-held and already-read. Verdict: FAIL on a misrouted reply.
- **C6, injects.** Watch: `--wait` gives injected; a fake box that ignores Enter gives unconfirmed and no retype once it is cleared; a short `--ttl` into a never-idle pane expires on time. Verdict: FAIL on a retype or early expiry.
- **C7, interrupt.** Watch: `interrupt` and `inject --interrupt` mid-turn; the line lands without any hand-clearing of the box. Verdict: FAIL if the inject waits behind the restored prompt.
- **C8, hold and pause.** Watch: hold by one privileged sender, inject and mail from another, the owner's inject passes; release; then `tender pause` freezes the TTL and `resume` delivers. Verdict: FAIL on delivery during a hold or pause, or a TTL that runs while paused.
- **C9, orch.** Watch: mail to an orch-role strand gets `withheld`; inject and interrupt are refused; the orch's own `inbox` shows withheld. Verdict: FAIL on any keystroke in an orch pane.
- **C10, daemon life and journal.** Watch: replace the binary and the daemon exits within 2 s; the next verb brings the same pool (diff of `log`); a planted 8-day-old day file is carried forward and deleted; a torn tail is cut; a malformed middle line gives a reason the verb can see. Verdict: FAIL on a pool diff or lost unread mail.
- **C11, removed holder.** Watch: remove the strand holding forwarded mail and mail awaited with `--wait-reply`; within about 60 s each message steps back one hop or ends `undelivered`, the waiter exits 5, and the reader sees that it bounced. Verdict: WARN if the bounce is not shown.
- **C12, identity.** Watch: pane processes resolve to their strand, a terminal to EXTERNAL; a detached process keeping its strand name is refused; a disagreeing strand name inside a pane is refused. Verdict: FAIL on a spoof that succeeds.
- **C13, collapsed and hand-added strands.** Watch: a plain `--cmd claude` strand and a stacked 3-row strand both receive a reminder, or creel names why it cannot deliver. Verdict: FAIL on a silent stall.
