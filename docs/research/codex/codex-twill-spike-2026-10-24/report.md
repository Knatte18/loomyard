# Codex twill spike: a fan of forks with inherited context

Spike of 2026-10-24, the second Codex spike, after [the first hands-on spike](../codex-spike-2026-10-24/report.md).
Codex CLI 0.164.0 (`codex-tui`), ChatGPT sign-in (plan `plus`), Linux, tmux 3.6.
Nothing was built.

The question: can twill, a fan of forks that inherit the parent's conversation, run on Codex?
The answer: yes, with the caveats in [What twill's directive would need to say](#what-twills-directive-would-need-to-say-on-codex) and [What blocks it](#what-blocks-it).
A Codex subagent spawned with `fork_turns: "all"` carries the parent's whole history, runs in parallel with its siblings, and reports back into the parent's context.
The default cap is three children at once, two of the twenty sessions that asked for a fan got none, and the child's task text is encrypted in every record.

## Setup

- Codex ran only in a private tmux server (`tmux -L codex-twill`, 140x50), in a throwaway git repository under the task worktree's `.scratch/`, launched with `-a never -s workspace-write -m gpt-6-luna -c model_reasoning_effort=low`.
- Codex had its own `CODEX_HOME` under `.scratch/`: a `config.toml` holding a `[projects."<repo root>"]` table with `trust_level = "trusted"`, and a symlink to `~/.codex/auth.json`.
  The trust prompt did not show.
  Codex added two `[tui]` entries to that private `config.toml` itself (`screen_reader_detection_done` and a `model_availability_nux` table).
- `~/.codex/config.toml`, `~/.codex/rules/` and `~/.codex/sessions/` were not touched: their modification times are unchanged and no rollout of this spike sits in `~/.codex/sessions/`.
  The private home had no allow rules, so the sandbox held for every command.
- Each session got its prompt by bracketed paste followed by a separate Enter, as the first spike found necessary.
  A script waited for the turn to end (no `esc to interrupt` line and a `Worked for` line, twice in a row), then sent `/quit`.
- An earlier pass of ten sessions ran with a poll script that sometimes ended a run before its turn did, so the runs overlapped; the counts below leave that pass out.
  Its first session, which ran alone, supplies the timing table under Parallelism.
- Every number below comes from the sessions' rollouts (`~/.codex`-style JSONL, here under the private home) or from the pane.
  The sample is small and the model is the cheapest one, so counts are indicative.
- The sessions fall into these groups; "fan prompt" is a one-line prompt asking for subagents that each write `out/aN.txt` and a parent that merges them into `out/merged.txt`.

| Group | Sessions | Fan prompt |
|---|---|---|
| A | 5, 4 subagents | names `fork_turns all` |
| B | 3, 4 subagents | says "each must know everything you know", names no parameter |
| C | 3, 4 subagents | silent about context and parameter |
| N | 2, 4 subagents | names `fork_turns none` |
| G | 2, 4 subagents, two turns | turn 1 puts a 14k-token file and a codeword in context, turn 2 is the fan with `all` (G1) or `none` (G2) |
| E8 | 1, 8 subagents | names `fork_turns all`, default cap |
| H8 | 1, 8 subagents | names `fork_turns all`, cap raised, hooks recording |
| D | 3, 4 subagents | a `PreToolUse` hook is configured; D1 prompts `none` under a guard that requires `all`, D3 prompts silent under the same guard, D2 ran silent without any hook by a mistake in the launch line |

## Summary

| Question | Result |
|---|---|
| Parallel? | Yes. Children overlap; the default allows three at once (the model is told "4 available concurrency slots, including you") |
| Cap | A fourth concurrent spawn returns `collab spawn failed: agent thread limit reached`; `-c agents.max_concurrent_threads_per_session=8` gave nine slots and eight children ran |
| How the parent waits | `wait_agent(timeout_ms)` returns `Wait completed.` with no agent named; each child's result arrives as an `agent_message` item `Message Type: FINAL_ANSWER … Sender: /root/aN` in the parent's context |
| Does a prompt get `fork_turns: "all"`? | Of 81 `spawn_agent` calls in 20 sessions, 67 said `all` and 14 said `none`; every `none` followed a prompt that named `none`, and every prompt that named `all` or said nothing got `all` |
| Does a prompt get a fan at all? | Two of 20 sessions spawned nothing though asked to (A4, B2): the parent wrote the files itself |
| Cost of an inherited context | A forked child's first call reads the parent's context plus about 7.5k tokens, whatever that context holds |
| Audit without hooks | The child's rollout names its parent, its path and its fork; replay in it proves the inheritance; the task text is encrypted |
| Hooks | `SessionStart`, `UserPromptSubmit` and `Stop` fire for the parent only; `PreToolUse` fires for children too, with the parent's `session_id` and the child's `transcript_path` |

## Parallelism

Four subagents, default cap, group A first session (times from the parent's and children's rollouts, seconds within one minute):

| Event | Parent spawns | Child runs |
|---|---|---|
| a1 | 04.87 | 04.9 to 07.6 |
| a2 | 06.00 | 06.0 to 09.8 |
| a3 | 06.99 | 07.0 to 09.8 |
| a4 | 08.26 | 08.3 to 11.3 |

- The model issues one `spawn_agent` per step, so spawns sit 1.0 to 2.3 seconds apart across all sessions; there is no single call that spawns a fan.
  A child starts the moment its spawn returns, and `spawn_agent` returns `{"task_name":"/root/a1"}` at once.
- A child's run takes about 3 to 6 seconds for a one-line file write, so the children overlap in pairs and triples, not in four.
- The parent's whole turn took 10.3 to 16.7 seconds in the 4-agent sessions without a second turn.

### The cap

The parent's developer message reads: "There are 4 available concurrency slots, meaning that up to 4 agents can be active at once, including you."
So three children run at once.
The fourth spawn failed with `collab spawn failed: agent thread limit reached` in three of the fifteen 4-agent sessions at the default cap (A1, A2 and N1).
The model retried after a child finished, and all three retries succeeded.

Eight subagents at the default cap (E8) hit the limit twice, on `a4` and `a8`.
The model retried `a8` after waiting but never retried `a4`; it wrote `a4`'s file itself, with the line `agent 4 knows secret unknown`.
Before spawning, the model also said in a text message that the collaboration instructions forbid spawning unless the user asks, noted that the user had asked, and went on.

With `-c agents.max_concurrent_threads_per_session=8` (H8) the developer message read "9 available concurrency slots", all eight spawns succeeded without a failure, and up to four children ran at once.
The key is the only knob found; the binary names it three times.

## How the parent waits and learns each child is done

From the parent's rollout in the G1 session, abridged:

```text
05:17.153  function_call_output  {"task_name":"/root/a4"}
05:17.155  agent_message         Message Type: FINAL_ANSWER Task name: /root Sender: /root/a1 Payload: Wrote `out/a1.txt`.
05:17.943  function_call         wait_agent {"timeout_ms":30000}
05:18.873  function_call_output  {"message":"Wait completed.","timed_out":false}
05:18.875  agent_message         Message Type: FINAL_ANSWER … Sender: /root/a2 Payload: Wrote `out/a2.txt`.
```

- A child's last assistant message comes back to the parent as an `agent_message` item with `Message Type: FINAL_ANSWER`, the child's task name as `Sender` and the message as `Payload`.
  The item arrives on its own, even before the parent calls `wait_agent`.
  A `inter_agent_communication_metadata` line sits beside it.
- `wait_agent` takes a `timeout_ms` and returns `Wait completed.` as soon as any message is pending.
  It names no agent, so the parent counts `FINAL_ANSWER` items.
  The model chose 30,000 to 120,000 ms; the developer message tells it to "prefer longer waits (minutes) to avoid busy polling".
  A wait that found nothing printed `No agents completed yet` in the pane, and the model waited again.
- Each parent wait cycle is a model call that re-sends the parent's whole context (see Cost).
- `list_agents` returns the live agents with a status such as `{"completed":"Created out/a5.txt …"}`; in E8 it did not list the children that had finished earlier.
- The pane shows `Started /root/a1`, `Completed /root/a1`, `Waiting for agents` and `Finished waiting`.
- The tool set also has `send_message`, `followup_task` and `interrupt_agent`, which a twill had no use for here.

## Does a prompt reliably get `fork_turns: "all"`?

`spawn_agent`'s own description says `fork_turns` "Defaults to `all`; use `none` to start without parent history. Only `all` and `none` are supported."
All 81 calls in the spike still carried an explicit `fork_turns`.

| Prompt | Sessions | Sessions that spawned | Calls with `all` | Calls with `none` |
|---|---|---|---|---|
| A, names `all` (with G1, E8, H8) | 8 | 7 | all 39 | 0 |
| B, "know everything you know" | 3 | 2 | all 8 | 0 |
| C, silent (with D2, D3) | 5 | 5 | all 20 | 0 |
| N, names `none` (with G2, D1) | 4 | 4 | 0 | all 14 |

(D1's one call was denied by the guard below; the table counts it.)

- Naming the value gets that value every time it spawns at all.
  Saying nothing got `all` in all five sessions, which matches the tool's default.
  The first spike's `none` pick came from a one-subagent probe that asked a question; this fan prompt never produced it unprompted.
- The risk is not the value but a missing fan: A4 (named `all`) and B2 spawned nothing, and the parent wrote all four files with one shell command.
  The developer message says: "Do not spawn sub-agents unless the user or applicable AGENTS.md/skill instructions explicitly ask for sub-agents, delegation, or parallel agent work."
  A directive has to ask for them in so many words.
- A `PreToolUse` hook can enforce the value.
  The tool's name in the payload is `collaborationspawn_agent` and `tool_input` holds `task_name`, `fork_turns` and `message`.
  A guard that printed the `permissionDecision: "deny"` shape of the first spike for any `fork_turns` other than `all` blocked D1's `none` call; the pane showed `Blocked by hook` and the model reported the guard and spawned nothing.
  The same guard allowed the four `all` calls of D3.
- Whether a child inherited anything is shown by its rollout (next sections) and by a probe the answer of which depends on the history alone.
  The first probe, "write the secret number as you know it", is weak: in N2 the four `none` children wrote the number, presumably because the parent put it in the task message, and in 4 of the 36 `all` children of groups A to C the child wrote `unknown` though its rollout held the prompt.
  The second probe, the codeword given in turn 1 and barred from the task message, was clean: G1's four `all` children wrote `pelican` and G2's four `none` children wrote `unknown`.

## The token cost of an inherited context

A child's rollout holds its own `token_count` events only (the parent's are not replayed), so `info.total_token_usage` of a child is the child's cost.
Input tokens below include the cached ones.

| Session | Parent context at the spawn | Child's first call, input (cached) | Child's two calls, input |
|---|---|---|---|
| Group N, `none` | 14.1k to 14.4k | 14,278 to 14,284 (0 to 14,080) | 28,636 to 28,693 |
| Group A to C, `all` | 14.1k to 14.4k | 21,540 to 22,228 (11,392 to 14,592) | 43,152 to 44,538 |
| G2, `none`, 14k-token file in context | 28.3k to 28.6k | 14,284 (6,912 to 14,080) | 43,132 and 43,227 for the two children with a third call |
| G1, `all`, 14k-token file in context | 28.3k to 28.6k | 35,752 to 36,067 (28,032 to 28,416) | 71,584 to 72,004 |

- A forked child's first call costs the parent's context plus about 7.5k tokens, with a 14k-token file in the parent's context as well as without: the child's first-call input minus the input of the parent call that issued its spawn was 7,457 to 7,545 in G1 and in the first A session.
  The extra is not the history: the rollout's replay ends with a developer note ("You are now a newly spawned child agent. The preceding conversation is inherited parent context."), a role message, a tools update of about 29,000 characters, a world-state record and the task message.
- The inherited part is the cheap part when the parent's prefix is cached: in G1 the first call read 28k of its 35.7k tokens from the cache.
  The 7.5k extra is the part sent fresh, and the cache hit on the inherited part varied (the cached figure of the small sessions ran from 11,392 to 14,592).
- Each later child's first call is larger by 75 to 105 tokens per earlier `spawn_agent` call in the parent's history: G1's four children read 35,752, 35,857, 35,962 and 36,067.
- Totals of one fan, parent plus four children:

| Session | Input | Cached | Output |
|---|---|---|---|
| A1, small context, `all` | 334,456 | 285,952 | 854 |
| A5, small context, `all` | 318,239 | 275,584 | 744 |
| N2, small context, `none` | 244,528 | 202,112 | 724 |
| G1, 14k file, `all` | 638,589 | 565,760 | 937 |
| G2, 14k file, `none` | 532,440 | 483,584 | 1,211 |

  The parent is about half of a fan's input: 159,899 against 174,557 in A1, and 314,800 against 323,789 in G1.
  It re-sends its context on every model call, and each spawn, wait and `FINAL_ANSWER` is one; in G1 the parent made twelve calls, most of them at 28k to 29k input.
- The four `none` children cost less than the four `all` children, by 11k input tokens each on average at the small context (N2 against A5) and by 49k with the file (G2 against G1), and they lost the codeword.

## What the rollouts show for auditing without hooks

A child writes its own rollout `rollout-<time>-<child id>.jsonl` beside the parent's in `sessions/YYYY/MM/DD/`.

- Its first `session_meta` holds `forked_from_id` (the parent's id) and `source.subagent.thread_spawn` with `parent_thread_id`, `depth` (1), `agent_path` (`/root/a1`) and an `agent_nickname`; the rollout name carries the child's own id.
  A second `session_meta` line follows it: the parent's, replayed.
  A reader that copies a rollout elsewhere should drop both, which hold account ids.
- The inherited history is replayed in full and verbatim: the developer messages, the tools, the parent's user prompt and its tool calls, all stamped with the fork instant.
  Presence of the parent's prompt in the child's rollout proves `all`, and its absence proves `none` (checked in every session of groups A to C and N).
  The `token_count` events are not replayed.
- The replay ends with a synthetic `turn_aborted` event with reason `interrupted`, which marks where the fork cut the parent's turn.
  A reader that counts interrupts must skip a child's first one.
- The child's own turn follows: `task_started`, an `agent_message` of `Message Type: NEW_TASK`, its tool calls, `token_count` events and `task_complete`, whose `last_agent_message` is the text the parent receives as `FINAL_ANSWER`.
  So a child's completion and result can be read from its rollout without any hook.
- The task text is encrypted.
  The `message` argument of `spawn_agent` in the parent's rollout, the `NEW_TASK` payload in the child's rollout and the `message` in the `PreToolUse` payload are all `gAAAAA…` ciphertext.
  An audit sees that a spawn happened, with what `fork_turns`, and what came back, but not what the child was told.
  The parent's assistant text before the call is readable.
- The parent's rollout shows each spawn (`function_call` `spawn_agent`, with `task_name` and `fork_turns`), its outcome (`{"task_name":"/root/a1"}` or the failure string), each wait and each `FINAL_ANSWER`.
- Rollout sizes: about 150 KB for a parent and 175 KB for each of its children at the small context, 208 to 216 KB with the 14k-token file, since each child stores the replay again.
  The 141 rollouts of the whole spike, both passes, took 23 MB.

### Hooks

Configured as in the first spike (`hooks.<Event>` with `--dangerously-bypass-hook-trust`), H8 recorded 23 payloads for an eight-child fan:

| Event | Payloads | Whose |
|---|---|---|
| `SessionStart` | 1 | parent |
| `UserPromptSubmit` | 1 | parent |
| `Stop` | 1 | parent, at the end of its turn |
| `PreToolUse` | 20 | 8 `spawn_agent`, 3 `wait_agent`, 8 child `Bash` writes, 1 parent `Bash` merge |

- The first spike's "no hook fired for the subagent" holds for `SessionStart`, `UserPromptSubmit` and `Stop`, and needs a correction for `PreToolUse`.
- A child's `PreToolUse` payload carries the parent's `session_id`.
  Its `transcript_path` names the child's rollout and its `turn_id` differs from the parent's, so a hook tells the child's tool calls from the parent's by `transcript_path`, not by `session_id`.
- So a shell guard written as a `PreToolUse` hook covers the children as well as the parent, and `spawn_agent` itself passes through the hook.

## What twill's directive would need to say on Codex

A twill directive written for Claude Code forks would, on Codex, need to say:

- Spawn the sub-agents explicitly and now: "Spawn N sub-agents with `spawn_agent`, in parallel." Without an explicit ask the model may do the work itself (A4, B2).
- Pass `fork_turns: "all"` on every call, and put nothing the history already holds into the task `message`.
  Never set `model` or `reasoning_effort` on these calls: Codex tells the model that full-history forks inherit both and do not accept overrides, and an override needs `fork_turns: "none"`.
- Give each child its own output file and no shared one: all agents share one directory and edits are visible at once.
- Wait until every child has answered: call `wait_agent` with a long `timeout_ms` (minutes) and repeat until one `FINAL_ANSWER` per child has arrived, counting them, since a wait names no agent.
- On `agent thread limit reached`, wait for a child to finish and spawn that child again; never write the child's file in the parent (E8's `a4`).
  Or launch with a raised `agents.max_concurrent_threads_per_session` and say the fan size.
- Merge only after the last `FINAL_ANSWER`, from the files or the payloads.
- Tell the children not to spawn sub-agents of their own (the developer note allows it).

On the launch side, a `codexengine` would:

- carry a `PreToolUse` guard that denies `collaborationspawn_agent` unless `fork_turns` is `all`, as D1 and D3 showed;
- read child completion from the child's rollout `task_complete` or the parent's `FINAL_ANSWER` items, and not from `Stop`;
- key child tool calls by `transcript_path`.

## What blocks it

Nothing found blocks a twill fan outright.
These limits shape it:

- **Three children at once by default.** A twill of more than three forks needs the config key, set through `-c` on the launch line (which the first spike found switches Codex to embedded mode and adds a startup warning), or a model that retries after the limit error.
- **No per-fork model.** A twill that gives lenses different models cannot do it with `fork_turns: "all"`; it would lose the inherited context.
- **No isolation.** Children share the parent's working directory and sandbox.
- **Opaque task text.** The brief a child got cannot be read back from the rollouts or the hooks.
  A twill that audits its prompts would have to keep its own copy of each brief.
- **A fan is a request, not a guarantee.** In 2 of 20 sessions the model did the work itself, so the engine has to check that the expected number of child rollouts and `FINAL_ANSWER` items exist, and re-prompt when they do not.
- **Cost of waiting.** Each wait cycle and each arriving result is a parent model call that re-sends the parent's context; the parent is about half of a fan's input tokens (G1: 314,800 against 323,789 for the four children).
- **The first-spike gaps remain.** No `Stop` for an interrupted turn, and the trust prompt for a fresh worktree unless the config holds the entry.

## Not tested

- More than four forks past the cap beyond two eight-subagent runs, and a cap of other values.
- Plan-quota cost: the `rate_limits` percentages in the rollouts were not read.
- `fork_turns` other than `all` and `none` (the tool accepts only these two).
- Children spawning their own children (`depth` above 1), `send_message` and `followup_task`.
- A parent interrupted while children run, and a child interrupted.
- A model above `gpt-6-luna` at `low`; the model's fan behavior may differ.
- Whether the shared `auth.json` symlink survives a token refresh; none happened in the spike's hour.
