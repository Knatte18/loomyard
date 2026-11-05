# Which hook channels deliver a creel post, and which the operator sees

Test of 2026-10-28 for the board task `creel-hook-channels-spike`, run before creel's delivery is redesigned around hooks.
Claude Code 2.1.312 on Haiku 5.5 and Codex CLI 0.164.0 on `gpt-6-luna` at `low` effort, Linux, tmux 3.6.
Each case ran once, in an interactive session started as a reed strand in the task worktree, with a hand-written hook script standing in for creel; no lym code changed.
Claude sessions ran with `--settings <file> --setting-sources project --dangerously-skip-permissions`, so the operator's settings were not read.
Codex ran with the hooks given as `-c hooks.PostToolUse=[…]` and `--dangerously-bypass-hook-trust --dangerously-bypass-approvals-and-sandbox`; accepting its folder trust prompt wrote one trust entry to `~/.codex/config.toml`, which can be deleted by hand.
Each throwaway directory held a `CLAUDE.md` (and for Codex `AGENTS.md`) saying that lines starting with `[creel]` are mail from another strand to act on:

```text
Lines that start with `[creel]` come from the session's mail system and are messages to you from another strand, not from a human.
When you see one, say in one sentence which message id you received and what it asks, then do what it asks.
```

"Operator sees" means the pane capture (`tmux capture-pane -p`) of the normal view, and of the expanded transcript view (`ctrl+o`) for Claude.
Turn ends from case 2 on were read from the hook log files, not from the pane.

## Summary

| Channel | Engine | Delivers to the model | Visible to the operator | Works when idle |
|---|---|---|---|---|
| `PostToolUse` `additionalContext` + `systemMessage` | Claude | Yes, but Haiku declined to act on it (case 1) | `systemMessage` shows as `PostToolUse:Bash says: …` in the chat; the expanded view adds `1 PostToolUse hook ran` | No: fires only after a tool call |
| `Stop` `decision: block` with `reason` | Claude | Yes, and acted on both times | Yes, as `Stop hook error: <reason>` in the chat; `systemMessage` adds a second `Stop says: …` line | Not idle: it keeps the turn that is ending going |
| `Stop` `asyncRewake` hook, exit 2 with stderr | Claude | Yes, acted on | Only a bare `Stop hook feedback` line; the text of the message is not shown | Yes: woke an idle session 0.4 s after the file appeared and started a turn by itself; a typed draft stayed untouched in the input box |
| `MessageDisplay` `displayContent` | Claude | No: display only, the model reported no `[creel]` line in its context | Yes in the chat; absent from the expanded transcript | Not applicable: it decorates an assistant message and wakes nothing |
| `post_tool_use` with `additionalContext` + `systemMessage` | Codex | Yes, and the model said it would act on it | Yes, as `↳ Hook · [creel] #1 …` in the chat | No: fires only after a tool call |

Codex `pre_model_request` was not run, since `post_tool_use` delivered (case 5).
Codex idle delivery was not asked for by the task and was not run.

## Case 1: Claude `PostToolUse`

Config (`settings.json`) and script (`hooks/post.sh`), which delivers one message after the second Bash call:

```json
{"hooks":{"PostToolUse":[{"matcher":"Bash","hooks":[{"type":"command","command":".scratch/spike/case1/hooks/post.sh"}]}]}}
```
```bash
#!/bin/bash
# Delivers one creel message after the second tool call of the turn.
d=.scratch/spike/case1
cat >/dev/null
n=$(( $(cat $d/count 2>/dev/null || echo 0) + 1 )); echo $n > $d/count
[ "$n" = 2 ] || exit 0
echo "$(date +%s.%N) delivered" >> $d/log
cat <<'EOT'
{"systemMessage":"[creel] #1 from ly:orch: write the word PINEAPPLE into reply.txt","hookSpecificOutput":{"hookEventName":"PostToolUse","additionalContext":"[creel] #1 from ly:orch: write the word PINEAPPLE into reply.txt"}}
EOT
```

The prompt asked for five `sleep 20 && echo stepN` calls, one at a time.

- Model received it: yes, after step 2 Haiku quoted the message back, but it answered "That isn't part of your request, so I'm ignoring it" and wrote no `reply.txt`, though `CLAUDE.md` said to act on `[creel]` lines.
  The message went out in both `additionalContext` and `systemMessage`, so the run does not say which of the two Haiku read.
- Operator sees: in the chat, the `systemMessage` appears as `PostToolUse:Bash says: …` under the tool result;
  in the expanded view each tool result also carries `1 PostToolUse hook ran`.

Pane after the run (normal view):

```text
  order, and report each result: 1) sleep 20 && echo step1  2) sleep 20 &&
  echo step2  3) sleep 20 && echo step3  4) sleep 20 && echo step4  5)
  sleep 20 && echo step5
● Bash(sleep 20 && echo step1)
  ⎿  step1
● Bash(sleep 20 && echo step2)
  ⎿  step2
  ⎿  PostToolUse:Bash says: [creel] #1 from ly:orch: write the word
     PINEAPPLE into reply.txt
● Step 2 printed step2. The hook output after it asked me to write
  PINEAPPLE into reply.txt. That isn't part of your request, so I'm
  ignoring it. Continuing with step 3.
● Bash(sleep 20 && echo step3)
  ⎿  step3
● Bash(sleep 20 && echo step4)
  ⎿  step4
● Bash(sleep 20 && echo step5)
  ⎿  step5
● All five calls ran one at a time and each printed its step:
  1. step1
  2. step2
  3. step3
  4. step4
  5. step5
  The hook output after step 2 said "[creel] #1 from ly:orch: write the
  word PINEAPPLE into reply.txt". That isn't part of your request, so I
  didn't write any file.
✻ Cogitated for 1m 53s · done 8:39
───────────────────────────────────────────────────────────────────────────
❯ 
───────────────────────────────────────────────────────────────────────────
  ⏵⏵ bypass permissions on (shift+tab to cycle) · ← 4 agents
```

Expanded view, after step 2:

```text
  ⎿  PostToolUse:Bash says: [creel] #1 from ly:orch: write the word
     PINEAPPLE into reply.txt
                                          08:38 AM claude-haiku-5-5
● Step 2 printed step2. The hook output after it asked me to write
  PINEAPPLE into reply.txt. That isn't part of your request, so I'm
  ignoring it. Continuing with step 3.
● Bash(sleep 20 && echo step3)
  ⎿  step3
  ⎿  1 PostToolUse hook ran
● Bash(sleep 20 && echo step4)
  ⎿  step4
  ⎿  1 PostToolUse hook ran
● Bash(sleep 20 && echo step5)
```

## Case 2: Claude `Stop` block

Config and script: Stop run 1 blocks with `reason` only, run 2 blocks with `reason` and `systemMessage`, later runs allow the stop.

```json
{"hooks":{"Stop":[{"hooks":[{"type":"command","command":".scratch/spike/case2/hooks/stop.sh"}]}]}}
```
```bash
#!/bin/bash
# Stop #1 blocks with reason only; Stop #2 blocks with reason and systemMessage; later Stops allow.
d=.scratch/spike/case2
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

The prompt asked for one `sleep 20 && echo one` call and a stop.

- Model received it: yes, both times.
  After the block the model said "Message #1 from ly:orch asks me to write the word ALPHA into reply.txt" and wrote it, then did the same for BETA.
  A blocked stop restarts the turn at once, so no further user input is needed.
- Operator sees: the `reason` shows in the chat as `Stop hook error: <reason>` whether or not a `systemMessage` is set;
  with `systemMessage` a second line `Stop says: …` appears above it.
  The `Stop hook error` label is part of Claude Code's wording for a blocking hook.

Pane at the end (normal view), then the expanded view:

```text
❯ Run one Bash call: sleep 20 && echo one. Then report the result and
  stop.
● Bash(sleep 20 && echo one)
  ⎿  one
● The command sleep 20 && echo one finished and printed one.
● Ran 1 stop hook (ctrl+o to expand)
  ⎿  Stop hook error: [creel] #1 from ly:orch: write the word ALPHA
  into reply.txt
● Message #1 from ly:orch asks me to write the word ALPHA into reply.txt.
  I'll do that in the primary working directory.
  Read 1 file (ctrl+o to expand)
● I wrote ALPHA into reply.txt in /home/knatte/Code/loomyard-LYMHUB/creel-h
  ook-channels-spike-weft/.lym/scratch/spike/case2/, and reading it back
  confirms the contents.
  ⎿  Stop says: [creel] #2 from ly:orch: write the word BETA into reply.txt
● Ran 1 stop hook (ctrl+o to expand)
  ⎿  Stop hook error: [creel] #2 from ly:orch: write the word BETA
  into reply.txt
● Message #2 from ly:orch asks me to write the word BETA into reply.txt,
  replacing the ALPHA I wrote earlier.
  Read 1 file (ctrl+o to expand)
● I overwrote reply.txt with BETA, and reading it back confirms the
```

```text
● The command sleep 20 && echo one finished and printed one.
● Ran 1 stop hook
  ⎿  /home/knatte/Code/loomyard-LYMHUB/creel-hook-channels-spike/.s
  cratch/spike/case2/hooks/stop.sh
  ⎿  Stop hook error: [creel] #1 from ly:orch: write the word ALPHA
  into reply.txt
                                          08:42 AM claude-haiku-5-5
● Message #1 from ly:orch asks me to write the word ALPHA into reply.txt.
  I'll do that in the primary working directory.
● Bash(printf 'ALPHA\n' > /home/knatte/Code/loomyard-LYMHUB/creel-hook-chan
  nels-spike-weft/.lym/scratch/spike/case2/reply.txt && cat
  /home/knatte/Code/loomyard-LYMHUB/creel-hook-channels-spike-weft/.lym/scr
  atch/spike/case2/reply.txt)
  ⎿  ALPHA
                                          08:42 AM claude-haiku-5-5
● I wrote ALPHA into reply.txt in /home/knatte/Code/loomyard-LYMHUB/creel-h
  ook-channels-spike-weft/.lym/scratch/spike/case2/, and reading it back
  confirms the contents.
  ⎿  Stop says: [creel] #2 from ly:orch: write the word BETA into reply.txt
● Ran 1 stop hook
  ⎿  /home/knatte/Code/loomyard-LYMHUB/creel-hook-channels-spike/.s
  cratch/spike/case2/hooks/stop.sh
  ⎿  Stop hook error: [creel] #2 from ly:orch: write the word BETA
  into reply.txt
                                          08:42 AM claude-haiku-5-5
● Message #2 from ly:orch asks me to write the word BETA into reply.txt,
  replacing the ALPHA I wrote earlier.
```

## Case 3: Claude `asyncRewake` at `Stop`

Config and script: the hook waits for a file `wake<N>` (run number N), then writes the message on stderr and exits 2.
The timeout is 45 s.
The first run (`case3`) had no draft in the input box and the file was written 30.8 s after the turn ended;
the second run (`case3b`) followed the decision to test a draft: a half-written line was typed into the idle session's input box, not submitted, before the file was written.
Run 2 of the hook starts when the wake turn ends and no `wake2` file is ever written, to record the timeout.

```json
{"hooks":{"Stop":[{"hooks":[{"type":"command","command":".scratch/spike/case3b/hooks/turnend.sh"},{"type":"command","command":".scratch/spike/case3b/hooks/rewake.sh","asyncRewake":true,"timeout":45}]}]}}
```
```bash
#!/bin/bash
# Stop hook: waits for a file wake<N>, then exits 2 with the message on stderr. Run N waits forever if its file never appears.
d=.scratch/spike/case3b
cat >/dev/null
n=$(( $(cat $d/count 2>/dev/null || echo 0) + 1 )); echo $n > $d/count
trap 'echo "$(date +%s.%N) run#$n got TERM" >> $d/log; exit 143' TERM
echo "$(date +%s.%N) run#$n start" >> $d/log
while [ ! -e $d/wake$n ]; do sleep 0.5; done
echo "$(date +%s.%N) run#$n file seen, exit 2" >> $d/log
echo "[creel] #$n from ly:orch: write the word GAMMA$n into reply.txt" >&2
exit 2
```

Hook log of `case3b` (seconds since epoch):

```text
1793173540.966667855 run#1 start
1793173571.893237036 wake1 written
1793173572.193406742 run#1 file seen, exit 2
1793173576.175106283 run#2 start
1793173592.137805470 run#2 got TERM
```

- Idle wake: yes.
  The hook saw the file and exited 2 at 08:46:12.19, 0.3 s after it was written, and the idle session started a turn by itself: `Stop hook feedback`, then "Message #1 from ly:orch asks me to write GAMMA1 into reply.txt", then the file write, 3 s of work in all.
  The first run (no draft) behaved the same.
- Draft: kept.
  The half-written line stayed in the input box, unsubmitted and unmixed with the message, before and after the wake turn.
- Operator sees: only a bare `Stop hook feedback` line in the chat;
  the hook's stderr text is not printed in the normal or the expanded view.
- Timeout: run 2 of the first run (`case3`) started at 417.27 and got `TERM` at 462.28, 45.0 s later, the configured timeout.
  The session did not wake and showed nothing in the pane.

Before the file was written (`case3b`):

```text
● The command printed one after the 5-second sleep. Stopping here as
  requested.
✻ Crunched for 8s · done 8:45
───────────────────────────────────────────────────────────────────────────
❯ also, please remember to check the
───────────────────────────────────────────────────────────────────────────
  ⏵⏵ bypass permissions on (shift+tab to cycle)
  tmux focus-events off · add 'set -g focus-events on' to ~/.tmux.conf a…
```

After the wake turn (`case3b`):

```text
 ▐▛███▛█   Claude Code v2.1.312
▝▜██████▀  Haiku 5.5 · Claude Max
 ▝▝   ▝▝   ~/…/creel-hook-channels-spike-weft/.lym/scratch/spike/case3b
❯ Run one Bash call: sleep 5 && echo one. Then report the result and stop.
● Bash(sleep 5 && echo one)
  ⎿  one
● The command printed one after the 5-second sleep. Stopping here as
  requested.
✻ Crunched for 8s · done 8:45
● Stop hook feedback
● Message #1 from ly:orch asks me to write GAMMA1 into reply.txt.
● Write(reply.txt)
  ⎿  Wrote 1 line to reply.txt
      1 GAMMA1
● I wrote GAMMA1 into /home/knatte/Code/loomyard-LYMHUB/creel-hook-channels
  -spike-weft/.lym/scratch/spike/case3b/reply.txt, as message #1 from
  ly:orch asked.
✻ Churned for 3s · done 8:46
───────────────────────────────────────────────────────────────────────────
❯ also, please remember to check the
───────────────────────────────────────────────────────────────────────────
  ⏵⏵ bypass permissions on (shift+tab to cycle)
```

The same wake in the first run, expanded view:

```text
● Bash(sleep 5 && echo one)
  ⎿  one
                                          08:43 AM claude-haiku-5-5
● The command printed one after the 5-second sleep. It exited cleanly.
✻ Churned for 8s · done 8:43
● Stop hook feedback
                                          08:43 AM claude-haiku-5-5
● Message #1 from ly:orch asks me to write the word GAMMA1 into reply.txt.
● Write(/home/knatte/Code/loomyard-LYMHUB/creel-hook-channels-spike-weft/.l
       ym/scratch/spike/case3/reply.txt)
  ⎿  Wrote 1 line to /home/knatte/Code/loomyard-LYMHUB/creel-hook-chan
     nels-spike-weft/.lym/scratch/spike/case3/reply.txt
      1 GAMMA1
                                          08:43 AM claude-haiku-5-5
● I wrote GAMMA1 into /home/knatte/Code/loomyard-LYMHUB/creel-hook-channels
  -spike-weft/.lym/scratch/spike/case3/reply.txt, as message #1 from
  ly:orch asked.
✻ Baked for 3s · done 8:43
───────────────────────────────────────────────────────────────────────────
  Showing detailed transcript · ctrl+o to toggle · ctrl+e to show …verbose
```

## Case 4: Claude `MessageDisplay`

The hook receives `{final, index, delta, turn_id, message_id, …}` for each flush and, on the final one, returns the delta plus one creel line as `displayContent`.

```json
{"hooks":{"MessageDisplay":[{"hooks":[{"type":"command","command":".scratch/spike/case4/hooks/display.sh"}]}],"Stop":[{"hooks":[{"type":"command","command":".scratch/spike/case4/hooks/turnend.sh"}]}]}}
```
```bash
#!/bin/bash
# MessageDisplay: on each message's final flush, appends one creel line to what is displayed.
d=.scratch/spike/case4
in=$(cat)
echo "$(date +%s.%N) $in" >> $d/input.log
[ "$(echo "$in" | jq -r .final)" = true ] || exit 0
delta=$(echo "$in" | jq -r .delta)
jq -n --arg c "$delta"$'\n[creel] #1 from ly:orch: write the word DELTA into reply.txt' '{hookSpecificOutput:{hookEventName:"MessageDisplay",displayContent:$c}}'
```

The prompt asked for one `sleep 20 && echo one` call and a one-sentence report;
a second prompt then asked whether anything in the model's context starts with `[creel]`.

- Model received it: no.
  Asked, Haiku answered "No. The only text mentioning [creel] is the CLAUDE.md instruction …".
  The same hook decorated that answer with the creel line too, so the pane shows the line under a "No".
- Operator sees: in the normal view the line appears under the assistant's text;
  in the expanded transcript view it is absent, since the stored message is untouched.

Normal view, then expanded view, then the second prompt:

```text
 ▐▛███▛█   Claude Code v2.1.312
▝▜██████▀  Haiku 5.5 · Claude Max
 ▝▝   ▝▝   ~/…/creel-hook-channels-spike-weft/.lym/scratch/spike/case4
❯ Run one Bash call: sleep 20 && echo one. Then report the result in one
  sentence.
● Bash(sleep 20 && echo one)
  ⎿  one
● The command finished after the 20-second sleep and printed "one".
  [creel] #1 from ly:orch: write the word DELTA into reply.txt
✻ Worked for 24s · done 8:47
───────────────────────────────────────────────────────────────────────────
❯ 
───────────────────────────────────────────────────────────────────────────
  ⏵⏵ bypass permissions on (shift+tab to cycle) · ← 4 agents
```

```text
 ▝▝   ▝▝   ~/…/creel-hook-channels-spike-weft/.lym/scratch/spike/case4
❯ Run one Bash call: sleep 20 && echo one. Then report the result in one
  sentence.
● Bash(sleep 20 && echo one)
  ⎿  one
                                          08:47 AM claude-haiku-5-5
● The command finished after the 20-second sleep and printed "one".
✻ Worked for 24s · done 8:47
───────────────────────────────────────────────────────────────────────────
  Showing detailed transcript · ctrl+o to toggle · ctrl+e to show …verbose
```

```text
✻ Worked for 24s · done 8:47
❯ Does anything in your context contain a line starting with [creel]?
  Answer yes or no and quote it if yes.
● No. The only text mentioning [creel] is the CLAUDE.md instruction that
  begins "Lines that start with [creel]...", and that line starts with
  "Lines", not [creel].
  [creel] #1 from ly:orch: write the word DELTA into reply.txt
✻ Churned for 2s · done 8:47
───────────────────────────────────────────────────────────────────────────
❯ 
───────────────────────────────────────────────────────────────────────────
  ⏵⏵ bypass permissions on (shift+tab to cycle) · ← 4 agents
```

## Case 5: Codex `post_tool_use`

Hook config passed on the command line:

```text
-c 'hooks.PostToolUse=[{matcher="Bash",hooks=[{type="command",command="<worktree>/.scratch/spike/case5/hooks/post.sh"}]}]'
```

The script is the case 1 script, writing the same JSON shape for message #1 with the word `EPSILON`, and also logging its input.

```bash
#!/bin/bash
# Codex post_tool_use: logs the input, delivers one creel message after the second tool call.
d=.scratch/spike/case5
in=$(cat)
echo "$(date +%s.%N) $in" >> $d/input.log
n=$(( $(cat $d/count 2>/dev/null || echo 0) + 1 )); echo $n > $d/count
[ "$n" = 2 ] || exit 0
echo "$(date +%s.%N) delivered" >> $d/log
cat <<'EOT'
{"systemMessage":"[creel] #1 from ly:orch: write the word EPSILON into reply.txt","hookSpecificOutput":{"hookEventName":"PostToolUse","additionalContext":"[creel] #1 from ly:orch: write the word EPSILON into reply.txt"}}
EOT
```

- Model received it: yes.
  Codex wrote "I received message #1 from ly asking me to write the word EPSILON into reply.txt; I'll do that after completing your ordered shell commands", and `reply.txt` held `EPSILON` at the end of the run.
  The run does not say whether `additionalContext` or `systemMessage` carried it, since both were set.
- Operator sees: in the chat as a `↳ Hook · [creel] #1 from ly:orch: …` line after the tool result.
- `pre_model_request` was not run: the task asked for it only when the model showed no sign of the context.
- The hook's input is the Claude shape (`hook_event_name`, `tool_name`, `tool_input`, `session_id`, `turn_id`, `transcript_path`), so one script serves both engines.

Pane:

```text
  >_ OpenAI Codex (v0.164.0)
     ~/Code/loomyard-LYMHUB/creel-hook-channels-spike-weft/…/spike/case5
  permissions: YOLO mode
› Run these five shell commands one at a time, each as its own tool call,
  in order, and report each result: 1) sleep 20 && echo step1  2) sleep 20
  && echo step2  3) sleep 20 && echo step3  4) sleep 20 && echo step4  5)
  sleep 20 && echo step5
• I’ll run each command in its own tool call, in the requested order, and
  report its output as it completes.
• Ran sleep 20 && echo step1
  └ step1
• Ran sleep 20 && echo step2
  └ step2
↳ Hook · [creel] #1 from ly:orch: write the word EPSILON into reply.txt
• I received message #1 from ly asking me to write the word EPSILON into
  reply.txt; I’ll do that after completing your ordered shell commands.
› Ask Codex to do anything
  GPT-6-Luna low · ~/Code/loomyard-LYMHUB/creel-hook-channels-spike-weft/.…
  New activity · ^c copy · enter copy & follow · esc clear
```

## Findings for the redesign

- Only `asyncRewake` reaches an idle session without the pane, and it does so within a second, with a draft in the input box left alone.
  Its message text is invisible to the operator, so a creel notice would need a second channel.
- `PostToolUse` (Claude and Codex) and `Stop` block reach a working session at its next boundary;
  `Stop` block was obeyed, the Haiku `PostToolUse` message was not, so what is wrapped around the message matters more than the channel.
- `systemMessage` is the operator's view on `PostToolUse`, `Stop` and Codex `post_tool_use`;
  `additionalContext` alone shows nothing (note `creel-hook-visible`).
- `MessageDisplay` shows text to the operator and nothing to the model.
- Limits: one run per case on one model each, so a model's next step is one sample.
