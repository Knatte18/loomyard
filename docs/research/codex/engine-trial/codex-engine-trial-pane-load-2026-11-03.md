# Codex engine trial: pane, sends, creel by the pane path, and load

Project `codex-engine-trial`, parts 5 and 7, run on 2026-11-05 against real Codex sessions launched through `lym shuttle`, as `internal/shuttleengine/codexengine/doc.go` and `internal/creelengine/doc.go` describe them.
Codex CLI 0.164.0, model `luna[medium]` (`gpt-6-luna`), every session in this task's worktree.
Verdict: cases 1, 2, 3 and 5 match the docs, with the notes below; case 4 does not, in two places; case 6 was not run.
Three lym faults are filed as board notes B3, B4 and B5.

## Method

- A held session is `lym shuttle run --model 'luna[medium]' --interactive --keep-pane` with an output file that is never written, so the run stays open at its composer; `lym shuttle state` lists it as `asking` between turns.
- `lym shuttle open --model 'luna[medium]'` gives a session with no run record.
- `lym shuttle` has no verb for a send, an idle probe or an interrupt, so a scratch program under `.scratch/` built `shuttleengine.NewRunner` as `internal/creelcli` does and called `Runner.Send`, `Runner.SessionIdle` and `Runner.Interrupt`. These are the calls creel's delivery makes.
- A creel post from EXTERNAL needs a process outside every strand pane: the post ran from `env -i … setsid -f bash -c 'lym creel post …'`.
- Turn counts and prompt texts come from each run's `events.jsonl` (UserPromptSubmit and Stop payloads) and the rollout under `~/.codex/sessions`.

## Case 1: pane readings

`lym shuttle state` reads a run's files alone, so it never reads the pane; the pane readings reach lym through `Runner.SessionIdle`, which reads the hook-derived readiness for a run and the pane for a strand with no run record.

| Reading | Where | Seen |
|---|---|---|
| Composer | held run at its composer | `lym shuttle state` `asking/awaiting-input`; `SessionIdle` `Idle:true`, `NoTurn:true`; pane showed `› Ask Codex to do anything` |
| Busy line | the same run, turn `timeout 45 tail -f /dev/null` | state `busy/turn` at 8, 16 and 24 s; `SessionIdle` `Idle:false`, reason `a turn is running`; pane showed `Working (3s • esc to interrupt) · 1 background terminal running` |
| Folder-trust prompt | `codex` in an untrusted git folder under the lym Codex home, in a reed pane | `Codex.Startup` on the capture returned the trust-prompt state; `TrustDismissSequence` returned `Enter` with the caret on `1. Trust and continue` and `Up`, `Enter` with the caret on option 2; pressing `Up`, `Enter` accepted the prompt and reached the composer |

The trust prompt cannot arise in a lym launch, since `Prepare` trusts the roots, and the pane's folder was not a run, so it was read by feeding the real capture to the engine, not through `lym shuttle state`.
An opened session showed the composer and the busy line in the pane, but its idle probe read `NoInputBox` each time: see Faults, B3.

Matches the docs for a run. The opened session does not.

## Case 2: sends

Held run, five sends of each kind through `Runner.Send`, one at a time, each after the session read idle.

- A 4,000-character send (a 60-character instruction and filler): all five landed whole. The five UserPromptSubmit payloads hold the text at exactly 4,000 characters and the five turns replied `long-1` to `long-5`.
- A 40-line send: `Runner.Send` refused all five before typing anything, `text must be a single line; multiline updates ride the file contract (write a file, Send a one-line pointer to it)`. The rule is in `validateSendText`; the creel inject is single-line too. So a 40-line send is not reachable through lym's guarded send.
- As a supplement, the same five 40-line texts were pasted with `tmux paste-buffer -p` (bracketed paste) and the Enter pressed a second later. All five landed whole: each UserPromptSubmit payload holds 40 lines and the replies were `lines-1` to `lines-5`. This tests Codex's paste, not lym's guard.

Matches the docs for the single-line send; the 40-line case is refused by design.

## Case 3: exit

A held run, `/exit` typed into the pane.
The pane ended with Codex's resume hint and `Pane is dead (status 0)`; `lym reed list` read the strand `live:false`; the run ended `died` and its strand was removed.
The `Send` that typed `/exit` into the earlier held run returned `the send did not land … box unreadable`, since the composer left the screen.
`lym shuttle state` read `asking` up to the end: see Faults, B4.

Matches the docs: a Codex exit reads as a dead pane.

## Case 4: creel by the pane path

| Step | Seen |
|---|---|
| Program notice typed in full while idle | The EXTERNAL post `external notice beta: reply with the word seen` (id 659) was typed into the held run's composer as `› [creel] #659 from "ly:EXTERNAL": external notice beta: reply with the word seen` and the session answered `seen`. |
| Agent post arrives as a reminder | A post from this task's strand (id 660, expects a reply) was typed as `[creel] 1 unread: #660 from "ly:codex-trial-pane-load:darn" — run lym creel read`, with no sender text. |
| `lym creel read` and `post --reply` from the sandbox | Both failed inside the session. `read` printed `creel: no daemon answers on /tmp/lym-1000/creel-ly-92073c9b.sock after starting one and 6 connects: dial unix …: connect: operation not permitted; daemon log …/_bolt/.lym/creel/daemon.log: … read-only file system`; `post --reply 660` printed the same. The daemon was running. |
| Notice to an opened session | The post (id 656) was never typed; `lym creel log` shows a `stalled` record, reason `no-input-box`, a second after the post: see Faults, B3. |

Matches the docs for the notice and the reminder. Does not match for the sandbox: the session cannot reach the daemon, so it cannot read or answer mail (B5).
After the strand was removed, message 660 went back to its sender, as the removed-holder sweep describes.

## Case 5: load

Three held runs started together, each driven by one script through ten turns (`timeout 5 tail -f /dev/null`, then a one-line reply), at the same time.
Turn 4 ran `timeout 60 …` and was interrupted four seconds in through `Runner.Interrupt`; turn 7 ran `timeout 25 …` and a second send was made four seconds in.

| Session | Prompts | Stops | Interrupt | Send during the busy turn |
|---|---|---|---|---|
| load1 | 12 | 11 | state `idle-stalled` after it, `turn_aborted` in the rollout | returned `sent` after 27 s, when the turn ended; turn `load1-extra` ran after it |
| load2 | 12 | 11 | the same | `sent` after 26 s |
| load3 | 12 | 11 | the same | `sent` after 27 s |

The 12 prompts are the launch pointer, ten turns and the extra send; the 11 Stops lack the interrupted turn 4.
Every other turn read `busy` four seconds in and `asking` once it ended, and every reply matched its turn.
One turn of load2 showed `Working (0s …)` for about 40 s before it ran, a slow model start; the script waited it out and no state read wrong.
Each session's rollout holds the abort of turn 4 twice (two lines for one `turn_aborted`).

Matches the docs: the turn ends, the interrupt and the guarded send read right under three sessions at once. The send waits for the idle composer and does not interrupt.

## Case 6: end to end

Not run.
It needs a throwaway sandbox hub, which `lym fabric clone` makes, and that verb is refused from a task session by design.
The orch will put it to the operator as a separate step.

## Faults

- B3 `opened-codex-never-idle` (shuttle, creel, codex): `Runner.SessionIdle` resolves the engine of a strand with no run record to the hub default, so an opened Codex session reads `NoInputBox` and creel types nothing into it, though `Runner.Send` picks the engine by the strand's provider tag.
- B4 `codex-state-asking-after-exit` (shuttle, codex): `lym shuttle state` reads a Codex run `asking` after Codex exited, while the loop ends it `died`.
- B5 `codex-sandbox-no-creel-socket` (codex, creel, shuttle): the `workspace-write` sandbox refuses the connect to the creel socket and makes the Bolt `.lym/creel` read-only, so a Codex strand cannot run `lym creel read` or `post --reply`.

## Observed, not faults

- `lym shuttle run` and `lym shuttle open` give no way to send, probe or interrupt; the cases used a scratch program over the runner.
- A turn that asks for `sleep 40` was refused by the model, which read the prompt's machine-sleep notice as covering it; `timeout N tail -f /dev/null` was used for a busy turn instead.
