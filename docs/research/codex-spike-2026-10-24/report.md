# Codex CLI hands-on spike

Spike of 2026-10-24, step 1 of the note `codex-engine` and of `docs/research/codex-feasibility.md` section 3.
Codex CLI 0.164.0 (`codex-tui`), ChatGPT sign-in (plan `plus`), Linux, tmux 3.6.
Nothing was built.

Every session ran in a private tmux server (`tmux -L codex-spike`, 140x50), in a throwaway git repository under the task worktree's `.scratch/`.
The cheapest model, `gpt-6-luna` at effort `low`, took the prompts unless a test says otherwise; each prompt was one short line.
`<work>` below stands for that throwaway directory's real path.
The worktree's `.scratch/` is a symlink, so Codex reported and keyed everything by the resolved path.

## Summary

| Item | Result |
|---|---|
| Startup and trust | A trust prompt shows on first use of a directory; trust is written to `~/.codex/config.toml` by Codex itself, and `-c projects.…` cannot pre-set it |
| Hook trust | Hooks given with `-c` need review; `--dangerously-bypass-hook-trust` runs them without it |
| Idle and busy | `Working (Ns • esc to interrupt)` marks busy; the `› Ask Codex to do anything` composer with no `Working` line marks idle |
| Send | Typed text followed by Enter in one burst loses the Enter; bracketed paste followed by an immediate Enter submits |
| Paste placeholder | Large pastes show `[Pasted Content N chars]` |
| Esc | Interrupts the turn; no `Stop` hook fires; a background command keeps running |
| Hooks | `SessionStart`, `UserPromptSubmit`, `Stop`, `PermissionRequest` and `PreToolUse` fire with JSON on stdin; `PreToolUse` and `PermissionRequest` can deny |
| Rollout file | `~/.codex/sessions/YYYY/MM/DD/rollout-<time>-<session_id>.jsonl`, named in every hook payload; `token_count` events carry usage and rate limits |
| Model and effort | `-m` and `-c model_reasoning_effort=` take effect and show in the rollout and the status line |
| Approvals | `-a never` stopped every prompt; the model is told escalation is unavailable |
| Sandbox | `workspace-write` blocked writes outside the workspace and all network, except where an allow rule in `~/.codex/rules/` exempts a command from the sandbox |
| `AGENTS.md` | Loaded at session start; `CLAUDE.md` loads through the fallback setting only when no `AGENTS.md` sits in the directory |
| Skills | `.agents/skills/<name>/SKILL.md` loaded and ran with `$name` |
| Fork | `/fork` inherits the conversation; a subagent inherits it only when spawned with `fork_turns` set to `all` |

## Side effects on the operator's Codex state

Codex wrote one entry to `~/.codex/config.toml` itself, when the trust prompt was accepted:

```toml
[projects."<work>"]
trust_level = "trusted"
```

The spike never edited the file or `~/.codex/rules/`.
The entry is harmless and can be deleted by hand.
The sessions of this spike sit in `~/.codex/sessions/2026/10/24/`.

## Startup and trust

First launch in an unknown directory, `--no-alt-screen` set, shows this before anything else:

```text
  Folder access
  <work>

  Trust this folder? Codex can read, edit, and run files here, subject to your permission settings. Folder settings can run code
  automatically, even without a model request. Continue only if you trust these files. Your trust decision will be saved.

› 1. Trust and continue
  2. Quit

  enter continue · esc quit
```

- Enter accepts and saves the decision; Esc quits Codex and returns to the shell.
- `-c 'projects."<work>".trust_level="trusted"'` did not skip the prompt: the startup warnings list `projects."<work>"` as an ignored session flag.
  A `codexengine` therefore cannot pre-trust a worktree through `-c`; it must either recognise the prompt and press Enter, or have `~/.codex/config.toml` hold the entry (written by Codex on the first accept).
- Any `-c` override also makes Codex run "without the shared background server" (embedded mode), which appears as a startup warning.

With hooks given by `-c` and no bypass flag, a second prompt follows:

```text
  Hooks need review
  4 hooks are new or changed.
  Hooks can run outside the sandbox after you trust them.

› 1. Review hooks
  2. Trust all and continue
  3. Continue without trusting (hooks won't run)

  enter confirm · esc skip
```

Choosing "Trust all" would persist hook hashes in the config.
`--dangerously-bypass-hook-trust` skips the prompt for that invocation and adds a startup warning: `` `--dangerously-bypass-hook-trust` is enabled. ``
A `codexengine` that writes a per-run hook config uses this flag.

The ready screen, once startup is done:

```text
  >_ OpenAI Codex (v0.164.0)
     <work>

  Tip: Use the desktop app. Install it from https://learn.chatgpt.com/docs/linux/linux-app and run chatgpt.

› Ask Codex to do anything

  GPT-6-Luna low · <work> · Workspace · Context 0% used · 5h 97% left · weekly 100%   ⚠ 3 warnings · f2 to view
```

- The tip line changes between launches.
- `⚠ N warnings · f2 to view` opens a warning pager (`k keep & next · esc dismiss & close`).
  The warnings seen: the ignored `projects` flag, the embedded-mode notice and the hook-trust bypass.
- Startup takes under 9 seconds to a usable composer.
- `/quit` exits and prints `To continue this session, run:` and `codex resume <session id>`; the shell prompt returns.

## Idle and busy readings

Busy, one second after submit:

```text
› Reply with the single word ok.

Working (0s • esc to interrupt)

› Ask Codex to do anything

  GPT-6-Luna low · <work> · Workspace · Context 0% used · 5h 97% left · weekly 100…
```

Busy with a command running in the background:

```text
Working (1s • esc to interrupt) · 1 background terminal running · /ps to view · /stop to close
```

Idle, after the turn:

```text
› Reply with the single word ok.

• ok

  Worked for 1s • 11:56 AM

› Ask Codex to do anything

  GPT-6-Luna low · <work> · Reply with ok · Workspace · Context 1% used · 5h 97% l…
```

- Busy: a line that starts with `Working (` and contains `esc to interrupt`.
  Idle: no such line, a `Worked for …` line above the composer, and the composer placeholder `› Ask Codex to do anything`.
- The composer is shown during busy turns too, so a composer match alone is not an idle reading.
- The status line carries model and effort (`GPT-6-Luna low`), cwd, thread name, permissions (`Workspace`), `Context N% used` and the 5-hour and weekly limits.
  It is configured in `~/.codex/config.toml` (`[tui] status_line`), so the operator's choice decides what it shows.
- A command still running in a background terminal does not keep the turn busy once the model finishes; `N background terminal running` stays in the footer line.

## Sending text: typing, paste, queueing

- `tmux send-keys "text" Enter` in one call left the text in the composer with the Enter lost; reproduced three times.
  The same text followed by a separate Enter one second later submitted.
  This matches a paste-burst guard: characters arriving back to back are treated as paste, and an Enter inside the burst does not submit.
- `tmux load-buffer` plus `paste-buffer -p` (bracketed paste) followed by Enter sent immediately, in a separate `send-keys`, submitted both a 3-line paste and a 1,686-character paste.
- A 3-line paste shows its lines in the composer.
  A 1,686-character, 41-line paste shows a placeholder:

  ```text
  › [Pasted Content 1686 chars]
  ```

- Typing a `$` opens a skill popup (`no matches`, `enter insert · esc close`) that takes the next Enter; Esc closes it.
  The skill list loaded lazily: the popup said `no matches` for `$spikeskill`, yet the skill ran once the prompt was submitted.
- A message typed and submitted while a turn is busy is accepted and shows in the transcript at once as a `› second message` line, ahead of the running turn's end.
  `Tab` queues instead of sending, per the startup tip; not exercised.

## Esc interrupt

A prompt asked for `sleep 40`; Esc two seconds in:

```text
• I’ll run the requested sleep command.

■ Conversation interrupted - use /feedback if something went wrong

  1 background terminal running · /ps to view · /stop to close

› Ask Codex to do anything
```

- The pane returns to idle at once and shows `■ Conversation interrupted`.
- No `Stop` hook fired (five prompts, four `Stop` payloads).
  The rollout ends the turn with `event_msg` `turn_aborted`, `reason: "interrupted"`.
- The `sleep 40` process kept running; `/stop` printed `Stopping all background terminals.`, and the process was still listed one moment later.
- Esc on a pending approval prompt cancels the prompt and interrupts the turn the same way.

## Hook payloads

Config, one `-c` per event, each pointing at a script that saves its stdin and prints `{}`:

```text
-c 'hooks.Stop=[{hooks=[{type="command",command="<script> Stop"}]}]'
```

`PreToolUse` takes a `matcher` (`matcher="*"`).
A script that is not executable exits 126; Codex shows `Hook failed / hook exited with code 126` and runs the command anyway, so a failing hook fails open.

Payloads, session ids shortened:

```json
{"session_id":"01a16d56-…","transcript_path":"~/.codex/sessions/2026/10/24/rollout-2026-10-24T11-55-30-01a16d56-….jsonl","cwd":"<work>","hook_event_name":"SessionStart","model":"gpt-6-luna","permission_mode":"bypassPermissions","source":"startup"}
{"session_id":"01a16d56-…","turn_id":"01a16d56-c982-…","transcript_path":"…","cwd":"<work>","hook_event_name":"UserPromptSubmit","model":"gpt-6-luna","permission_mode":"bypassPermissions","prompt":"Reply with the single word ok."}
{"session_id":"01a16d56-…","turn_id":"01a16d56-c982-…","transcript_path":"…","cwd":"<work>","hook_event_name":"Stop","model":"gpt-6-luna","permission_mode":"bypassPermissions","stop_hook_active":false,"last_assistant_message":"ok"}
{"session_id":"01a16d5d-…","turn_id":"01a16d5d-fc2e-…","transcript_path":"…","cwd":"<work>","hook_event_name":"PermissionRequest","model":"gpt-6-luna","permission_mode":"default","tool_name":"Bash","tool_input":{"command":"touch /home/knatte/csp-perm","description":"May I create /home/knatte/csp-perm by retrying the requested touch command with elevated permissions?"}}
{"session_id":"01a16d5e-…","turn_id":"01a16d5e-fae7-…","transcript_path":"…","cwd":"<work>","hook_event_name":"PreToolUse","model":"gpt-6-luna","permission_mode":"default","tool_name":"Bash","tool_input":{"command":"echo FORBIDDEN2"},"tool_use_id":"exec-af3afc1e-…"}
```

- `SessionStart` fired when the first prompt was submitted, in the same instant as `UserPromptSubmit`, not at launch.
  An idle freshly started pane has produced no hook yet, so the startup classifier has to come from the pane.
- `source` was `startup` for a new session and `fork` for a `/fork`.
- `permission_mode` reads `bypassPermissions` under `-a never` and `default` under `-a on-request`.
- `Stop` carries `last_assistant_message`, so a completion signal and the reply text arrive without parsing the rollout.
  `Stop` fires once per turn; it did not fire for an interrupted turn.
- A subagent produced no hook events of its own: no `SessionStart` and no `Stop` for the child, only the parent's.
- `PreToolUse` fires for shell commands with `tool_name` `Bash`.
  A hook that prints the following blocks the command, and Codex shows `Blocked by hook` with the reason, which the model then reports:

  ```json
  {"hookSpecificOutput":{"hookEventName":"PreToolUse","permissionDecision":"deny","permissionDecisionReason":"spike guard: FORBIDDEN is blocked"}}
  ```

  This is the shape the deny guardrails of the feasibility study (step 5) need.
- `PermissionRequest` fires only when Codex would show an approval prompt (the model asked for `require_escalated`).
  With `-a on-request` and a hook that printed no decision, the prompt still appeared (screen below).
  A hook that printed the following replaced the prompt:

  ```json
  {"hookSpecificOutput":{"hookEventName":"PermissionRequest","decision":{"behavior":"deny","message":"spike hook denied this request"}}}
  ```

  Pane: `Blocked by hook / spike hook denied this request`.
  The `allow` behavior was not tried.

The approval prompt as shown:

```text
  Would you like to run the following command?

  Environment: local

  Reason: May I create /home/knatte/csp-perm by retrying the requested touch command with elevated permissions?

  $ touch /home/knatte/csp-perm

› 1. Yes, proceed (y)
  2. Yes, and don't ask again for commands that start with `touch /home/knatte/csp-perm` (p)
  3. No, and tell Codex what to do differently (esc)

  Press enter to confirm or esc to cancel
```

## Rollout file

- Path: `~/.codex/sessions/YYYY/MM/DD/rollout-<local time>-<session id>.jsonl`, named by `transcript_path` in every hook payload.
  The file exists from the first prompt, not from launch (a launch quit at the trust prompt wrote none).
- Line types seen: `session_meta`, `event_msg` (`task_started`, `thread_settings_applied`, `item_completed`, `token_count`, `task_complete`, `turn_aborted`), `response_item` (`message`, `custom_tool_call`, `custom_tool_call_output`, `additional_tools`), `turn_context`, `world_state`, `token_usage_record`.
- `session_meta` carries `id`, `session_id`, `timestamp`, `cwd`, `originator` (`codex-tui`), `cli_version`, `source`, `model_provider`, `git`, and for a fork `forked_from_id`.
  It also holds account ids; a reader that copies it elsewhere should drop them.
- `turn_context` carries `approval_policy`, `sandbox_policy` and the `permission_profile`, `model` and `effort`.
- `task_complete` carries `last_agent_message`, `duration_ms` and `time_to_first_token_ms`.

A `token_count` event, after the first turn:

```json
{"type":"event_msg","payload":{"type":"token_count","info":{"total_token_usage":{"input_tokens":13866,"cached_input_tokens":11392,"cache_write_input_tokens":0,"output_tokens":5,"reasoning_output_tokens":0,"total_tokens":13871},"last_token_usage":{"input_tokens":13866,"cached_input_tokens":11392,"cache_write_input_tokens":0,"output_tokens":5,"reasoning_output_tokens":0,"total_tokens":13871},"model_context_window":258400},"rate_limits":{"limit_id":"codex","primary":{"used_percent":3.0,"window_minutes":300,"resets_at":1792851420},"secondary":{"used_percent":0.0,"window_minutes":10080,"resets_at":1793438220},"plan_type":"plus","rate_limit_reached_type":null}}}
```

- A "reply ok" turn already costs about 13.9k input tokens (11.4k cached): the system prompt and tool definitions.
- `info.total_token_usage` is cumulative for the session and `last_token_usage` is the latest model call, so a usage reader sums nothing; it takes the last event.
  `input_tokens` includes `cached_input_tokens`.
- `rate_limits` gives the 5-hour and weekly percentages the status line shows.

## Model and effort

- `-m gpt-6-luna` and `-c model_reasoning_effort=low|xhigh` take effect: the rollout's `turn_context` reads `"model":"gpt-6-luna","effort":"xhigh"`, and the status line reads `GPT-6-Luna xhigh`.
- Models in the account's cache at spike time: `gpt-6.1-sol`, `gpt-6-astra`, `gpt-6-sol`, `gpt-6-luna`, `gpt-5.6-sol`, `gpt-5.6-terra`, `gpt-5.6-luna`, `gpt-reserve` and `codex-auto-review`.
  The efforts offered are `low`, `medium`, `high`, `xhigh`, `max`, plus `ultra` on every model but Luna and `gpt-reserve`.
  So the note's aliases Sol, Luna and Astra map to slugs `gpt-6.1-sol`, `gpt-6-luna` and `gpt-6-astra`; the slug carries a generation, so an alias table will need one to track it.
- The in-session `/model` picker lists the models, then a reasoning-level screen with `enter default · s session`.
  Its default choice persists to the config, so an engine switches model through the launch line or a `/model` + `s` sequence, never the default choice.

## Approvals and sandbox

`-a never -s workspace-write`, hooks bypassed, `gpt-6-luna`:

- No approval prompt appeared in any turn.
  Asked to run a command with `require_escalated`, the model answered that the approval policy is Never and escalation cannot be requested, and ran nothing.
- The rollout's `turn_context` recorded `"approval_policy":"never"` and a `sandbox_policy` of `workspace-write` with `network_access:false`; the writable roots were the workspace, `/tmp` and `$TMPDIR`, with `.git`, `.agents` and `.codex` under the workspace read-only.
- Writing inside the workspace worked.
- Plain commands outside it were blocked: `touch /home/knatte/csp-a` returned `Read-only file system`.
  `codex sandbox -c 'sandbox_mode="workspace-write"' -- sh -c 'echo b > /home/knatte/x'` gave the same error outside the TUI, with an inside write succeeding.
- Network was blocked: `curl https://example.com` failed with `Could not resolve host`, while the same command from the shell returned HTTP 200.
- The exemption: the operator's `~/.codex/rules/default.rules` holds `prefix_rule(pattern=["bash"], decision="allow")` and the same for `sh`, `python3`, `git`, `gh`, `dotnet`, `cat`, `ls` and `rg`.
  A command matching such a rule ran outside the sandbox without a prompt: `bash -c 'touch /home/knatte/csp-b'` created the file, and `echo b > /home/knatte/codex-spike-outside.txt` did as well.
  So with `-a never`, an allow rule widens the sandbox for that command prefix, not only the approval.
  An engine that relies on `workspace-write` as a boundary must account for the operator's rules; `codex exec` has `--ignore-rules`, but the interactive `codex` rejects that flag (`unexpected argument '--ignore-rules'`).
  An isolated `CODEX_HOME` would avoid it but needs its own auth; it was not tried because a copied or shared `auth.json` risks the operator's token refresh.
- The probe files were removed after each test.

With `-a on-request` the model attempts the command in the sandbox first; only when it asks for `require_escalated` does the approval prompt (above) or the `PermissionRequest` hook appear.
A plain failure in the sandbox is returned to the model without a prompt.

## `AGENTS.md`, `CLAUDE.md` and skills

- A repository-root `AGENTS.md` reached the model as a user-role message, `# AGENTS.md instructions for <dir>` with the text inside `<INSTRUCTIONS>…</INSTRUCTIONS>`; asked for the codeword it contained, the model answered it.
- With both `AGENTS.md` and `CLAUDE.md` present and `-c 'project_doc_fallback_filenames=["CLAUDE.md"]'`, only `AGENTS.md` loaded.
  With `AGENTS.md` moved aside, the same setting loaded `CLAUDE.md` and the model answered its codeword.
  A repository that holds both files loses `CLAUDE.md` under this setting.
- A skill at `<work>/.agents/skills/spikeskill/SKILL.md` (frontmatter `name`, `description`) was found in a repository-level `.agents/skills`; `$spikeskill` in a prompt made Codex answer with the skill's text, and the pane printed `I’m using the spikeskill instructions you provided.`
- `~/.agents/skills` was not tried.

## `/fork` and subagents

- `/fork` asks `Where should the forked conversation run?` (`Use current Git worktree` or `Create new Git worktree`), then prints `Fork created. You can continue here.`
  The fork wrote a new rollout whose `session_meta` has `forked_from_id` set to the parent, fired `SessionStart` with `source: "fork"`, and answered a question about a number given only before the fork.
  The fork inherits the conversation.
- The model spawned a subagent with a `spawn_agent` call whose arguments include `task_name`, `message` and `fork_turns`.
  With `fork_turns: "none"` (the model's own pick) the subagent said `No secret number has been shared in this conversation`.
  With `fork_turns: "all"` it answered the number.
  So a subagent inherits the conversation only when the spawn asks for it, and the parent can be told to.
- A subagent runs as its own rollout, with `source.subagent.thread_spawn` naming `parent_thread_id`, `depth`, `agent_path` and a nickname.
  The pane shows `Started /root/<task_name>`, `Waiting for agents`, `Completed …`; the first wait returned `No agents completed yet` and the parent waited again.
- No hooks fired for the subagent.

## Not tested

- `Tab` queueing and Esc-Esc backtrack.
- A `PermissionRequest` hook returning `allow`.
- `~/.agents/skills` and `/skills`.
- `codex exec`, `codex resume` and billing terms: the spike used the interactive TUI only.
- The hook trust hash path ("Trust all"), to keep the operator's config unchanged.

## What a `codexengine` would use

Launch line, one Codex process per strand in a tmux pane:

```text
codex --no-alt-screen -a never -s workspace-write -m <slug> \
  -c model_reasoning_effort=<effort> \
  -c 'project_doc_fallback_filenames=["CLAUDE.md"]' \
  -c 'hooks.SessionStart=[…]' -c 'hooks.UserPromptSubmit=[…]' -c 'hooks.Stop=[…]' \
  -c 'hooks.PreToolUse=[{matcher="*",hooks=[…]}]' -c 'hooks.PermissionRequest=[…]' \
  --dangerously-bypass-hook-trust
```

- `-a never -s workspace-write` stops every prompt, but an allow rule in `~/.codex/rules/` still lifts the sandbox for its prefix; see Approvals and sandbox.
- Hooks write one line per event to the run's events file; the transcript path and session id come from the first payload.
- The `PreToolUse` hook carries the shell-guard table; its deny shape is above.
- A repository with both `AGENTS.md` and `CLAUDE.md` needs `AGENTS.md` to point at `CLAUDE.md`, or the fallback setting does nothing there.
- Trust cannot be set on the launch line: the first launch in a new worktree shows the trust prompt, and the engine's startup classifier presses Enter on it.

Pane classifiers, matched on the visible pane text (`capture-pane -p`):

| State | Match |
|---|---|
| Trust prompt | `Trust this folder?` and `1. Trust and continue`; answer Enter |
| Hook-trust prompt | `Hooks need review`; avoided by the bypass flag, otherwise choose `2. Trust all and continue` |
| Ready, idle | `› Ask Codex to do anything` (or a draft after `› `) in the last block, no `esc to interrupt` anywhere in the visible pane |
| Busy | A line starting `Working (` with `esc to interrupt` |
| Background work | `N background terminal running`; does not make the pane busy |
| Approval prompt | `Would you like to run the following command?` with `Press enter to confirm or esc to cancel` |
| Interrupted | `■ Conversation interrupted` |
| Paste placeholder | `› [Pasted Content N chars]` |
| Skill popup | `enter insert · esc close` with `no matches` or a list; Esc closes it |
| Warning pager | `k keep & next · esc dismiss & close` |
| Exited | The shell prompt and `codex resume <id>` after `To continue this session, run:` |

Send sequence: bracketed paste (`paste-buffer -p`) of the whole prompt, then Enter as a separate key; never typed characters with Enter in the same burst.
The turn is done on the `Stop` hook (or `task_complete` in the rollout) and not on the pane alone, since an interrupt ends a turn without `Stop`.
The usage reader takes the last `token_count` event of the rollout named in the hook payloads.
