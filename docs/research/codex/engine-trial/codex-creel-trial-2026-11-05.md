# Codex engine trial: creel mail through the Codex hooks

Task `codex-creel-trial`, project `codex-engine-trial`, run on 2026-11-05 in the sandbox hub `lyt`, against real Codex sessions launched through `lym shuttle run` with the installed `lym` built from the landed `codex-creel-hooks`.
Codex CLI 0.164.0, model `luna[medium]` (`gpt-6-luna`), sandbox `workspace-write` with network access, every session an interactive held run (`--interactive --keep-pane`) with an output file that is never written.
Verdict: all five cases match the design, with one fault, filed as board note B2, and one first attempt that does not count.

## Method

- Two held runs, `lyt:creeltrial-a` and `lyt:creeltrial-b`, started together.
  A turn that holds a tool call open is `timeout 20 tail -f /dev/null`; a turn the trial wants without tools is a one-word reply.
- Every post came from outside any strand, so its sender is `lyt:EXTERNAL`, except the case 4 agent post, which a session made itself.
- Times come from the creel journal (`lym creel log`: `posted` and `read` events), each run's `events.jsonl` (hook stamps) and each session's rollout under the Codex home, where a mail shows as a user message or a `HookPrompt` item with its own timestamp.
- Proof that the hooks were on the launch: each Codex process's command line held the `lym creel hook-read codex` and `lym creel hook-wait codex` commands.
- The operator typed nothing in `lyt`.

## First attempt, not counted

The first session ran on the previous `lym` binary: the landed build was installed one second after that launch started, so its Codex command line held the recording hooks and the activity touch and no creel command.
Its mail reached it only through the pane path, as a typed line after its turn ended.
It was removed, and every case below ran on sessions launched after the install.
A trial that starts right after an install should check the launch line for `hook-read` before posting.

## Case 1: mail while the session runs tools

A woken turn ran three `timeout 20 tail -f /dev/null` calls.
Mail 2 was posted at 15:13:58.56 during the first call.

| Step | Time |
|---|---|
| First tool call ended | 15:14:22.56 |
| Mail 2 marked read, through `PostToolUse` | 15:14:22.63 |
| Final reply | `done-wake1 mail2-seen` |

The mail waited for the end of the running tool call, then arrived 0.07 s after it, before the second call, and the model acted on it.

A burst of three mails to each of the two sessions during one call (case 5) arrived the same way, all three at the call's end.

## Case 2: mail during the last step of a turn

A woken turn ran one 10 s tool call, then was told to write a long list.
Mail 3 was posted at 15:15:38.82, as the list was being written.

| Step | Time |
|---|---|
| Mail 3 posted | 15:15:38.82 |
| Delivered through `Stop` as a `HookPrompt` item | 15:15:51.45 |
| Final reply of the continued turn | 15:15:55.74, `mail3-seen` |

The `Stop` hook of the continued turn carried `stop_hook_active` true, and `Stop` recorded no turn end for the turn that took the mail.
The session went on to answer it in the same turn.

## Case 3: mail to an idle session

Time from post to the mail's user message in the session's rollout, one mail to an idle session each:

| Mail | Post to arrival |
|---|---|
| MAIL-1 (#13) | 0.2 s |
| WAKE-1 (#14) | 3.5 s |
| WAKE-2 (#16) | 2.8 s |
| PING-4 from B (#19) | 3.5 s |
| NOTICE-4 (#21) | 7.2 s |
| A-IDLE-1 (#22, in a burst) | 8.2 s |
| TASK-4 to B (#18) | 8.8 s |

The waiter marks the mail read within 0.4 s of the post every time; the turn starts later, through `codex queue`, 0.2 to 8.8 s after the post.
The model acted on every wake that asked for a tool call or a reply word.
One did not: MAIL-1 asked that the final reply start with `mail1-seen`, and the session replied `done-a` again, so a wake that names no task is a weak instruction to this model.

## Case 4: a notice and an agent's mail that expects a reply

| Step | Seen |
|---|---|
| Notice with the reply expected, from EXTERNAL (#21) | The session answered `seen4` in its own reply text, as the trust sentence says for EXTERNAL, and ran no `lym creel post`. |
| B ran `lym creel post lyt:creeltrial-a --expect-reply …` from inside its sandbox (#19) | Posted at 15:16:30.47 with `expect_reply` set; A had it as `#19 from "lyt:creeltrial-b", expects a reply (lym creel post --reply 19 "<text>")`. |
| A answered | `lym creel post --reply 19 "pong4"` from inside its sandbox, posted at 15:16:37.44 as reply #20, 6.7 s after the post. |
| B received the reply | Read at 15:16:37.75; B's turn started 7.4 s after the reply was posted, with `[creel] #20 from "lyt:creeltrial-a": pong4`. |

Both sessions read, posted and replied with `lym creel` inside `workspace-write` with network access; no refusal came up.
A notice from a lym program (a `<shortname>:LOOM` sender) was not posted: no trial session may impersonate one, so the program case rests on the EXTERNAL notice above.

## Case 5: several mails in a row and two sessions at once

| Variant | Mails | Seen |
|---|---|---|
| Both idle | A-IDLE-1 to 3 to A, B-IDLE-1 to 3 to B, interleaved within 0.6 s | A: the first woke it, the other two arrived through `Stop` 10 s later in one `HookPrompt` pair, the reply was `A-IDLE-2` and `A-IDLE-3`. B: see Faults. All six tags were answered. |
| Both mid-tool | A-BUSY-1 to 3 and B-BUSY-1 to 3, interleaved, during a 25 s call each | A read all three at 15:19:02.14, B all three at 15:18:57.64, each at the end of its own call; each reply listed its own tags in order. |

No session received a tag of the other: A's final list held only `MAIL-1`…`A-BUSY-3` and B's only `TASK-4`, `pong4`, `B-IDLE-2`, `B-IDLE-3`, `B-IDLE-1`, `WAKE-5`, `B-BUSY-1` to `B-BUSY-3`.

## Faults

- B2 `codex-idle-wake-extra-reminder` (codex, creel, shuttle): with a wake queued and its mail read, two more posts to the same idle Codex strand typed the pane-path line `[creel] Mail is waiting for you. End this turn now; …` into B's composer.
  That line started a turn of its own at 15:17:34.65, the `Stop` of that turn delivered the two mails at 15:17:37.86, and the queued first mail reached the model only at 15:17:42.70, 16.9 s after its post, where A got its first mail after 8.2 s.
  No mail was lost; the cost is a wasted model turn and the delay.

## Observed, not faults

- `lym creel log` stamps a mail `read` when the waiter or hook hands it over, not when the model starts the turn, so its `read` time is the delivery start.
- A model told to run six tool calls skipped them and replied at once in one session; the trial used three calls on a wake that named the calls as a task, which it followed.
- A session's `permission_mode` in the hook payloads reads `bypassPermissions` while the launch line sets `-s workspace-write`; the rollout's `sandbox_policy` reads `workspace-write`.
