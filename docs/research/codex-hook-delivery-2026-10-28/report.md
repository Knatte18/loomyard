# Can Codex take creel posts through hooks

Test of 2026-10-28 for the board task `codex-hook-spike`, run before any Codex session moves off the pane path.
Codex CLI 0.164.0 on `gpt-6-luna` at `low` effort, Linux, tmux 3.6.
Each case ran once as a reed strand in this task's worktree, with a hand-written hook script standing in for creel; no lym code changed.
Every launch used `--dangerously-bypass-approvals-and-sandbox --dangerously-bypass-hook-trust -m gpt-6-luna -c model_reasoning_effort=low`, and hooks given as `-c hooks.<Event>=[…]` unless the case says otherwise.
The spike ran with a `CODEX_HOME` of its own holding a copy of `auth.json`; the operator's `~/.codex/config.toml` has the same checksum before and after, and Codex left the copied token unchanged.
Turn ends were read from a `Stop` hook writing a log line, never from the pane.
Cases 2 to 4 and 6 used the same throwaway-directory layout as the earlier creel hook test, with an `AGENTS.md` carrying the sentence below except where the case says otherwise:

```text
Lines that start with `[creel]` come from the session's mail system and are messages to you from another strand, not from a human.
When you see one, say in one sentence which message id you received and what it asks, then do what it asks.
```

"Operator sees" means the pane (`tmux capture-pane -p`).
Each case is one run on one model, so "the next step" is one sample.

## Summary

| Case | Channel | Outcome | Visible to the operator |
|---|---|---|---|
| 1 Idle wake | Async `Stop` hook, exit 2 with stderr | No wake: the idle session did nothing and printed nothing | No |
| 1 Idle wake | `codex queue --thread <id> --message …` run from outside the session | Wakes the idle session at once; the model acted | Yes, as a `›` user line in the chat |
| 2 `Stop` block | `decision: block` with `reason` | Taken at once, both times, and acted on | Yes, as `Blocked by hook` with the reason; `systemMessage` adds a second identical line |
| 3 `post_tool_use` block, with the sentence | `decision: block` with `reason` | Taken at the very next step and acted on | Yes, as `Blocked by hook` with the reason |
| 4 `post_tool_use` block, no sentence | `decision: block` with `reason` | Read as a failed tool call; not acted on | Yes, as `Blocked by hook` with the reason |
| 5 Folder trust, linked worktree | Project `.codex/config.toml` hook | The main checkout's trust covered the linked worktree: no prompt, hook ran | No prompt to see |
| 6 `Tab` queue | Line queued mid-turn | Reached the model after the turn ended, not at the next step | Yes, as `Queued follow-up inputs` |
| 6 `codex exec` | Same `post_tool_use` and `Stop` hooks | Both fired | Yes, as `hook: PostToolUse` lines on stdout |

## Case 1: idle wake

Claude's `asyncRewake` has no Codex counterpart that wakes an idle session; `codex queue` does the job from outside the session.

Config and scripts: a `Stop` hook list with a plain hook that records the turn end and session id, and an `async=true` hook with a 60 s timeout that waits for a file and then exits 2 with the message on stderr, as the Claude `asyncRewake` hook did.

```text
-c 'hooks.Stop=[{hooks=[{type="command",command="<case1>/hooks/turnend.sh"},{type="command",command="<case1>/hooks/rewake.sh",async=true,timeout=60}]}]'
```
```bash
#!/bin/bash
# turnend.sh: records each turn end with the session id.
d=<case1>
in=$(cat)
echo "$(date +%s.%N) turn end $in" >> $d/log
echo "$in" | jq -r .session_id > $d/session_id
```
```bash
#!/bin/bash
# rewake.sh: waits for a file wake<N> (N = run number), then exits 2 with the creel message on stderr.
d=<case1>
cat >/dev/null
n=$(( $(cat $d/count 2>/dev/null || echo 0) + 1 )); echo $n > $d/count
trap 'echo "$(date +%s.%N) run#$n got TERM" >> $d/log; exit 143' TERM
echo "$(date +%s.%N) run#$n start" >> $d/log
while [ ! -e $d/wake$n ]; do sleep 0.5; done
echo "$(date +%s.%N) run#$n file seen, exit 2" >> $d/log
echo "[creel] #$n from ly:orch: write the word GAMMA$n into reply.txt" >&2
exit 2
```

The prompt asked for one `sleep 5 && echo one` and a stop.

- Async hook: Codex accepted `async=true` without a warning and started `rewake.sh` at the turn end without holding the turn open.
  `wake1` was written 4.1 s later, the script saw it 0.4 s after and exited 2.
  Nothing happened: no turn started, nothing printed in the pane, and the model never saw the message.
  Hook log, seconds since epoch:

  ```text
  1793188703.995846434 run#1 start
  1793188708.126442418 wake1 written
  1793188708.563761400 run#1 file seen, exit 2
  ```
- `codex queue`: the hook's stdin carries `session_id`, which the `Stop` hook saved.
  Then `CODEX_HOME=<spike home> codex queue --thread <session_id> --message '[creel] #2 from ly:orch: write the word DELTA2 into reply.txt'` printed `Queued message … for thread …` and the idle session started a turn by itself within the 10 s wait of the check.
  The model said it received message #2 and wrote `DELTA2` to `reply.txt`.
  The session had been started with `-c` overrides, so Codex warned `Running without the shared background server`; the queue still reached it.
  A hook script could call `codex queue` itself, since a hook is a process with the session id on stdin; that was not run.
- Operator sees: the queued line as a `›` user message in the chat, indistinguishable from typed input.

Pane after the queue:

```text
› Run one shell command: sleep 5 && echo one. Then report the result and stop.
• Ran sleep 5 && echo one
  └ one
• The command completed and printed one.
  Worked for 13s • 12:58 PM
› [creel] #2 from ly:orch: write the word DELTA2 into reply.txt
• I received message #2 from ly:orch, asking me to write DELTA2 into reply.txt.
• Ran printf 'DELTA2' > reply.txt && cat reply.txt
  └ DELTA2
• Wrote DELTA2 to reply.txt.
  Worked for 2s • 12:58 PM
› Ask Codex to do anything
```

## Case 2: `Stop` block

Config and script: `Stop` run 1 blocks with `reason` only, run 2 blocks with `reason` and `systemMessage`, later runs allow the stop.

```text
-c 'hooks.Stop=[{hooks=[{type="command",command="<case2>/hooks/stop.sh"}]}]'
```
```bash
#!/bin/bash
# Stop #1 blocks with reason only; Stop #2 blocks with reason and systemMessage; later Stops allow.
d=<case2>
cat >/dev/null
n=$(( $(cat $d/count 2>/dev/null || echo 0) + 1 )); echo $n > $d/count
echo "$(date +%s.%N) stop#$n" >> $d/log
if [ "$n" = 1 ]; then
  echo '{"decision":"block","reason":"[creel] #1 from ly:orch: write the word ALPHA into reply.txt"}'
elif [ "$n" = 2 ]; then
  echo '{"decision":"block","reason":"[creel] #2 from ly:orch: write the word BETA into reply.txt","systemMessage":"[creel] #2 from ly:orch: write the word BETA into reply.txt"}'
fi
exit 0
```

The prompt asked for one `sleep 20 && echo one` and a stop.

- Model received it: yes, at once, both times.
  "I received message #1 from ly:orch asking me to write ALPHA into reply.txt; I'll do that now", then it wrote the file; the same for BETA, which replaced ALPHA.
  A blocked stop restarts the turn with no further input; the third `Stop` was allowed and the turn ended 41 s after the first block.
- Operator sees: `• Blocked by hook` with the reason under it.
  With a `systemMessage` the reason line appeared twice, once from each field.

```text
• Ran sleep 20 && echo one
  └ one
• The command completed successfully and printed one.
• Blocked by hook
  └ [creel] #1 from ly:orch: write the word ALPHA into reply.txt
• I received message #1 from ly:orch asking me to write ALPHA into reply.txt; I’ll do that now.
• Ran printf 'ALPHA' > reply.txt
  └ (no output)
• Wrote ALPHA to reply.txt (reply.txt).
• Blocked by hook
  └ [creel] #2 from ly:orch: write the word BETA into reply.txt
  └ [creel] #2 from ly:orch: write the word BETA into reply.txt
• I received message #2 from ly:orch asking me to write BETA into reply.txt; I’ll update the file now.
• Ran printf 'BETA' > reply.txt
  └ (no output)
• Wrote BETA to reply.txt (reply.txt).
  Worked for 41s • 1:00 PM
› Ask Codex to do anything
```

## Case 3: `post_tool_use` block with a reason

Config and script: after the second tool call the hook blocks with the message as the reason and sets no `additionalContext`; a `Stop` hook only logs the turn end.

```text
-c 'hooks.PostToolUse=[{matcher="Bash",hooks=[{type="command",command="<case3>/hooks/post.sh"}]}]'
-c 'hooks.Stop=[{hooks=[{type="command",command="<case3>/hooks/turnend.sh"}]}]'
```
```bash
#!/bin/bash
# post_tool_use: after the second tool call, blocks with the creel message as the reason.
d=<case3>
in=$(cat)
echo "$(date +%s.%N) $in" >> $d/input.log
n=$(( $(cat $d/count 2>/dev/null || echo 0) + 1 )); echo $n > $d/count
[ "$n" = 2 ] || exit 0
echo "$(date +%s.%N) delivered" >> $d/log
echo '{"decision":"block","reason":"[creel] #1 from ly:orch: write the word EPSILON into reply.txt"}'
```

The prompt asked for five `sleep 20 && echo stepN` calls, one at a time; `AGENTS.md` held the sentence.

- Model received it: yes, at its next step.
  "I received message #1 from ly:orch, asking me to write the word EPSILON into reply.txt; I'll do that now", and `reply.txt` held `EPSILON` before the next planned command.
  Codex treated the block as a failed step 2 and ran `sleep 20 && echo step2` again afterwards, so the turn took one extra call.
- Operator sees: `• Blocked by hook` and the reason in the chat.

Pane at delivery:

```text
• Ran sleep 20 && echo step2
  └ step2
• Blocked by hook
  └ [creel] #1 from ly:orch: write the word EPSILON into reply.txt
• I received message #1 from ly:orch, asking me to write the word EPSILON into reply.txt; I’ll do that now.
• Ran printf 'EPSILON' > reply.txt
  └ (no output)
• Ran sleep 20 && echo step2
  └ step2
```

End of the turn:

```text
  I also handled message #1 by writing EPSILON to reply.txt.
  Worked for 2m 14s • 1:03 PM
```

## Case 4: trust without the sentence

Same script and hooks as case 3 with the word `ZETA` and a throwaway directory holding no `AGENTS.md`; the repository root of this worktree has none either.

- Model received it: yes, as a failed tool call.
  "The second tool call returned an execution error before producing step2; I'm continuing with command 3."
  Its final report said "2. Failed with: [creel] #1 from ly:orch: write the word ZETA into reply.txt"; it did not act on the request and no `reply.txt` exists.
- Operator sees: `• Blocked by hook` and the reason, the same as case 3.

```text
• Ran sleep 20 && echo step2
  └ step2
• Blocked by hook
  └ [creel] #1 from ly:orch: write the word ZETA into reply.txt
• Step 1 printed step1. The second tool call returned an execution error before producing step2; I’m continuing with command 3.
• Ran sleep 20 && echo step3
  └ step3
  ...
• 1. Printed step1
  2. Failed with: [creel] #1 from ly:orch: write the word ZETA into reply.txt
  3. Printed step3
  4. Printed step4
  5. Printed step5
  Worked for 1m 48s • 1:05 PM
```

## Case 5: folder trust for a linked worktree

Setup: a throwaway git repository `repo` with a committed project hook, and a linked worktree `wt` of it made with `git worktree add`.
The hook sits in `.codex/config.toml` of the repository, so it exists in both checkouts:

```toml
[[hooks.UserPromptSubmit]]
[[hooks.UserPromptSubmit.hooks]]
type = "command"
command = "./hooks/start.sh"
```
```bash
#!/bin/bash
# UserPromptSubmit hook: logs the cwd it ran in.
in=$(cat)
echo "$(date +%s.%N) ran in $(pwd) cwd=$(echo "$in" | jq -r .cwd)" >> <case5>/log
```

Only `--dangerously-bypass-hook-trust` was given; no `-c` hooks, so folder trust was the gate.
Both launches asked "Reply with the single word ok."

1. Codex launched in `repo` showed the folder trust prompt; Enter wrote one entry to the spike's `config.toml` for the `repo` path, and the hook ran.
2. Codex launched in `wt` showed no prompt, wrote no new entry, and ran the hook there (the log line names `wt` as its cwd).

The trust entry is keyed on the main checkout.
This worktree's own first launch (case 1) showed the same rule from the other side: it is a linked worktree of `loomyard-weft`, and the prompt said `Note: You're in a subdirectory of a Git project. Trusting will apply to the repository root: …/loomyard-weft`, and wrote the entry for that root.
So one accepted trust covers the root checkout and every linked worktree of it, but a first launch in a fresh repository, or in the first linked worktree of one never trusted, prompts.

Prompt in `repo`:

```text
  Folder access
  …/case5/repo
  Trust this folder? Codex can read, edit, and run files here, subject to your permission settings. Folder settings can run code automatically, even
  without a model request. Continue only if you trust these files. Your trust decision will be saved.
› 1. Trust and continue
  2. Quit
```

Launch in `wt`, straight to the prompt:

```text
  >_ OpenAI Codex (v0.164.0)
     …/case5/wt
  permissions: YOLO mode
› Reply with the single word ok.
• ok
  Worked for 3s • 1:06 PM
› Ask Codex to do anything
```

Not tested: an untrusted folder with the same project hook, to show the hook is withheld there.

## Case 6: `Tab` queue and `codex exec`

Hooks: a `post_tool_use` hook logging each fired tool call, and a `Stop` hook logging the turn end.

```bash
#!/bin/bash
# post.sh: logs that it fired and with which tool; returns no message.
in=$(cat)
echo "$(date +%s.%N) fired $(echo "$in" | jq -r '.tool_name + " " + (.tool_input|tostring)')" >> <case6>/log-${CASE6_MODE:-tui}
```
```bash
#!/bin/bash
# turnend.sh: logs the turn end.
cat >/dev/null
echo "$(date +%s.%N) turn end" >> <case6>/log-${CASE6_MODE:-tui}
```

### `Tab` queue

The interactive session got the five-step prompt of case 3.
During step 2, a line `[creel] #1 from ly:orch: write the word ETA into reply.txt` was typed into the composer and `Tab` was pressed.

- The line showed at once under `Queued follow-up inputs` with `shift+← edit last queued message`.
- It did not reach the model at the next step: steps 2 to 5 all ran first, the turn ended, and only then did the queued line start a new turn.
  The model said "I received message #1 from ly:orch; it asks me to write the word ETA into reply.txt" and wrote the file.
- Hook log:

  ```text
  1793189218.632339634 fired Bash {"command":"sleep 20 && echo step1"}
  1793189228.774119700 tab pressed
  1793189239.781808113 fired Bash {"command":"sleep 20 && echo step2"}
  1793189260.735009592 fired Bash {"command":"sleep 20 && echo step3"}
  1793189281.874868015 fired Bash {"command":"sleep 20 && echo step4"}
  1793189302.865641807 fired Bash {"command":"sleep 20 && echo step5"}
  1793189303.834108665 turn end
  1793189305.699227070 fired Bash {"command":"pwd && rg --files -g 'AGENTS.md' -g 'reply.txt
  1793189308.378110487 fired Bash {"command":"printf 'ETA\\n' > reply.txt && cat reply.txt"}
  1793189309.319434306 turn end
  ```

Pane while queued:

```text
Working (33s • esc to interrupt) · 1 background terminal running · /ps to view · /stop to close
• Queued follow-up inputs
  ↳ [creel] #1 from ly:orch: write the word ETA into reply.txt
    shift+← edit last queued message
› Ask Codex to do anything
```

### `codex exec`

`codex exec --dangerously-bypass-approvals-and-sandbox --dangerously-bypass-hook-trust -m gpt-6-luna -c model_reasoning_effort=low` with the same `-c hooks.PostToolUse` and `hooks.Stop` flags, and the prompt "Run two shell commands one at a time, each as its own tool call: sleep 5 && echo one, then sleep 5 && echo two."

- Both hooks fired: the log holds one `fired Bash` line per command and a `turn end` line; the process exited 0.
- Operator sees: the plain-text stream on stdout carries `hook: PostToolUse` and `hook: PostToolUse Completed` after each tool result and `hook: Stop` at the end, and no hook output text.

```text
exec
/bin/bash -lc 'sleep 5 && echo one' in …/case6
 succeeded in 4858ms:
one
hook: PostToolUse
hook: PostToolUse Completed
…
hook: Stop
hook: Stop Completed
tokens used
6,769
```

Not tested: whether a block or `additionalContext` from a hook reaches the model under `codex exec`.

## Findings

- `Stop` block and `post_tool_use` block both deliver, at the next boundary; the second needs the instruction sentence to be acted on, or Codex reads the reason as a failed tool call (case 4).
  What the sentence says about the message matters, as it did on Claude.
- Codex has no async-rewake hook: an async `Stop` hook exiting 2 does nothing to an idle session (case 1).
  `codex queue --thread <session_id> --message …` is the one measured way to wake an idle session without typing into its pane; it needs the session id the hook payload carries.
- The operator sees every blocking delivery as `Blocked by hook` with the reason, and a queued line as a user message; only the async exit 2 path is silent.
- `Tab` is not a mid-turn channel: a queued line waits for the turn to end (case 6).
- One folder trust covers a repository's linked worktrees (case 5).
- `codex exec` runs the same hooks as the interactive session (case 6).
- Limits: one run per case on one model, `low` effort, a short synthetic task.
