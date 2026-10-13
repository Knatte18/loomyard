# Codex CLI feasibility for Loomyard

Read-only study, 2026-10-13.
Classes: (a) already behind the shuttle Engine interface, (b) easy to put behind it, (c) hard dependency on a Claude Code feature.

## 1. Claude couplings

| Coupling | Where | Class | Note |
|---|---|---|---|
| Engine choice hardwired | `claudeengine.NewFromConfig` at ~15 sites: `internal/{burlercli,webstercli,loomcli}/wiring.go`, `loomcli/looparm.go`, `loomcli/looplock.go`, `orchcli/cli.go`, `battencli/wire.go`, `battencli/parentlock.go`, `pausecli/deps.go`, `shuttlecli/cli.go`, `cliwire/sessionnamer.go` | b (medium) | One engine per Runner. `modelspec` already has an `Engine` field, but `knownEngines` is only `claude` and nothing consumes `Resolved.Engine`. `run.json` (`shuttleengine/rundir.go`) records no engine, so cross-run readers (`ReadAgentActivity`, `ReadSessionStates`, `ReadAgentWaits`) take a single engine. |
| Model specs `opus[high]` | `internal/modelspec/registry.go`, `modelspec.go`; effort validated in `claudeengine/command.go` | b (small) | Add a `codex` engine and aliases. Effort is already engine vocabulary. |
| Permission mode (bypass) | `Spec.PermissionMode` (`shuttleengine/spec.go`); `orchengine/config.go` accepts only bypass | a | Maps to `--dangerously-bypass-approvals-and-sandbox`. |
| Hooks feeding the events file | `claudeengine/settings.go`, `signals.go`, `events.go` | a | Codex hooks are GA with Stop, UserPromptSubmit, SessionStart/End, PermissionRequest, PreToolUse and Interrupt. Hooks must be trusted per hash (`--dangerously-bypass-hook-trust`). There is no equivalent of AskUserQuestion or elicitation asks. |
| Deny guardrails (python, raw `go test ./...` gate-slot bypass, `sed -i`, computed `rm`) | deny table in `claudeengine/settings.go` | a (port is medium) | They must be re-expressed as Codex PreToolUse hooks. The raw-go guard protects `gateslot`, so it is not optional. |
| Pane reading: startup/trust gate, "esc to interrupt", input box, paste placeholder, stray input, rewind picker | `claudeengine/startup.go`, `session.go`; optional `InputBoxReader`, `InterruptGuard`, `RewindPickerReader`, `TextPaster`, `SessionCycler` | a | Codex needs its own classifiers, derived empirically. Run with `--no-alt-screen`. Codex's Esc-Esc backtrack plays the role of Claude's rewind picker. |
| Transcript parsing: usage, activity, context tokens, skill-load check, fork audit | `claudeengine/usage.go`, `activity.go`, `audit.go`, `skillload.go` | a | Codex rollout JSONL has `token_count` events (input, cached, output, reasoning). |
| Session registry: liveness and resume check | `claudeengine/prober.go`, `resume.go` (`~/.claude/sessions`) | a | Optional capabilities. Without them, a dead provider behind a live pane reads as a timeout. |
| tokencount tool | `tools/tokencount/main.go` (`~/.claude/projects`), `cost.go` (Claude prices), `calibrate.go` (reads CLAUDE.md) | b (small–medium) | A dev tool outside the engine. It needs a Codex rollout reader and a Codex price table. |
| Skills (`scribe:*`, `ly:board`) | `Spec.Skills` via `SkillLoader`; the role lists in each spawner (e.g. `burlerengine/engine.go`, `landingshed/describe.go`) | a, plus deploy work (b) | An engine without `SkillLoader` is refused for any spec that names skills. Codex skills are `SKILL.md` files in `~/.agents/skills`, invoked as `$name`. `tools/deploy/main.go` syncs only `~/.claude/plugins/cache`. |
| Plugins | `plugins/ly/.claude-plugin`, `tools/deploy/main.go` | b (small) | Only the orch uses `ly:board`. |
| CLAUDE.md | Claude auto-loads it; `websterengine/merriambase.go` counts its lines | b (small) | Codex reads AGENTS.md. Use `-c project_doc_fallback_filenames=["CLAUDE.md"]`, which caps it at 32 KiB by default. |
| Slash commands `/clear`, `/compact`, `/color`, `/model`, `/reload-plugins` | `SessionCycler`, `ColorSequence`, `ModelSwitchSequence` in claudeengine | a | Mostly used by the orch. |
| Session naming (`--name`, `custom-title`) | `cliwire/sessionnamer.go` -> `claudeengine/sessionname.go` | b (small) | A no-op namer is enough for Codex. |
| Env scrub | `reedengine/env.go` `CleanClaudeEnv` | b (small) | |
| `claude_*` config keys (prompt-cache TTL, denies) | `shuttleengine/config.go` | b (small) | They are provider keys inside the neutral config. |
| IDE / sandbox launch `claude` | `ideengine/spawn.go`, `vscode/config.go`, `tools/sandbox/suite.go`, `testkit/llmkit` | b (small) | Operator and test tooling only. |
| Cross-session messaging (`SendMessage`/`ListAgents` by `--name`) | stencils: `parent/parent-directive-parent.md` (every role that has a parent), `seat/seat-directive-{chair,advisor}.md`, `loom/loom-template-parent-review-delivery.md`, `shed/shed-template-driver-notify*.md`, `orch/orch-template-role.md`; also the notice text in `shuttleengine/hold.go`, which is a seam leak into the core | c (workaround is medium) | Codex has no cross-session messaging. Two routes exist: `lyx shuttle send <guid>` (guarded paste) and the orch notice queue (`orchengine/notice.go`). cc-socks and creel have no references in this repo. |
| Agent-tool forks (`subagent_type: "fork"`) | `twill/twill-directive.md`, `webster/webster-template-master.md`, `webster-body-implementer.md`, burler cluster fan (`burlerengine/doc.go`), orch reviewer/stop forks (`orch-template-role.md`); `Engine.AuditForks` with `websterengine/audit.go` | c | Codex has subagents and `/fork`, but whether context is inherited is undocumented. Webster's fork audit reads Claude fork transcripts. |
| Background Bash plus completion-notification wake | `webster-template-master.md` (backgrounded `recover-batch`), `shed/shed-template-driver.md` (Claude-strand loom driver), `EventWaiting` from the Stop payload's `background_tasks` | c | Codex has background terminals, but no documented wake-on-completion. The loom driver already has a Go runner alternative. |
| ScheduleWakeup / Monitor | none used by spawned roles; the orch stencil forbids Monitor | n/a | |
| Orch `--adopt <claude session>` | `orchcli/start.go` | c | Orch only. |

## 2. Codex CLI (late 2026)

The research agent collected these.
I checked the hooks and non-interactive-mode pages myself.
Model names and rollout details come from source code or third-party pages.

- TUI in tmux works, but there is no tmux guidance. Alt-screen breaks tmux scrollback, so use `--no-alt-screen` or `tui.alternate_screen=never`. Esc interrupts, and Esc-Esc on an empty composer opens backtrack. Bracketed paste is not documented. Sources: https://learn.chatgpt.com/docs/config-file/config-reference , https://github.com/openai/codex/issues/28104
- AGENTS.md: `~/.codex/AGENTS.md`, then per directory from the git root down to the cwd, with an override file. 32 KiB cap. `project_doc_fallback_filenames` can add CLAUDE.md. Source: https://learn.chatgpt.com/docs/agent-configuration/agents-md
- Subagents: built-in `default`, `worker` and `explorer`, plus custom TOML agents, run in parallel. `codex fork` and `/fork` exist. Whether a subagent inherits the conversation history is undocumented. Source: https://learn.chatgpt.com/docs/agent-configuration/subagents
- Hooks: GA 2026-05-14 with 12 events. Stop carries `session_id`, `transcript_path`, `turn_id` and `last_assistant_message`. Hooks are trusted per hash. `notify` fires on turn complete. Source: https://learn.chatgpt.com/docs/hooks
- MCP: client via `[mcp_servers.*]`. `codex mcp-server` was removed in 2026-09; app-server replaces it. Source: https://learn.chatgpt.com/docs/changelog
- Skills and plugins: `SKILL.md` in `.agents/skills` or `~/.agents/skills`, invoked as `$name` or `/skills`. Plugins bundle skills and MCP. `/import` from Claude Code exists. Source: https://learn.chatgpt.com/docs/build-skills
- Transcripts: `~/.codex/sessions/YYYY/MM/DD/rollout-*.jsonl` with `session_meta`, `turn_context`, `response_item`, `event_msg` and `compacted` lines. `event_msg/token_count` carries total and last usage plus rate limits. Source: https://raw.githubusercontent.com/openai/codex/main/codex-rs/protocol/src/protocol.rs
- Model and effort: `-m`, `-c model_reasoning_effort=…` (low through xhigh and above, per model), profiles via `-p`. Reported current models are the gpt-6.x line; I did not verify that. Source: https://learn.chatgpt.com/docs/config-file/config-advanced
- Sandbox and approvals: `-a on-request|never`, `-s read-only|workspace-write|danger-full-access`, `--yolo`. Trust is stored per project. Source: https://learn.chatgpt.com/docs/config-file/config-reference
- Resume: `codex resume [ID|--last]` and `codex exec resume`. Source: https://learn.chatgpt.com/docs/developer-commands?surface=cli
- Billing: ChatGPT sign-in uses plan limits (5-hour windows plus weekly). `codex exec` reuses saved auth by default, and ChatGPT-managed auth for exec is documented as an "advanced" option that uses plan rate limits. API keys are recommended for automation. No policy moves exec or headless use onto API billing the way Anthropic did for `claude -p`. Check the current ToS. Sources: https://learn.chatgpt.com/docs/non-interactive-mode , https://learn.chatgpt.com/docs/auth , https://learn.chatgpt.com/docs/pricing
- Background: unified exec background terminals (`/ps`, `/stop`). There is no Monitor or ScheduleWakeup and no cross-session messaging. App-server JSON-RPC (`thread/start`, `turn/start`, `turn/steer`, `turn/interrupt`, `thread/tokenUsage/updated`) is experimental. Source: https://learn.chatgpt.com/docs/app-server

## 3. Recommendation

The first roles to move are single-session, fork-free and file-output-only.
Each has its own spawner:

- burler reviewer chair, with no cluster fan (`burlerengine/review.go`)
- burler fixer (`burlerengine/fix.go`)
- landing describe (`landingshed/describe.go`)
- bouncer judge and seed (`shedadapters/bouncer.go`)
- treadle judge and targeting (`treadleengine/`)
- friction reflection (`frictionengine/spec.go`)
- merge-conflict resolver (`mergeresolve/spec.go`)
- darn writer (`darnengine/spec.go`)
- alignment agent (`alignment/prompt.go`), which loads no skills

Blocked by hard dependencies:

- the webster Master and implementer forks, plus recovery
- the twill discussion writer
- the orch
- the burler cluster fan
- seat tables with advisors
- the discussion writer's parent-review delivery
- the Claude-strand loom driver, which can use the Go driver instead

Steps:

1. Hands-on spike: drive Codex in tmux and capture the startup, trust, idle, busy and paste behaviour, the hook payloads and a rollout file. Small.
2. Engine plumbing:
   - accept `codex` in `modelspec` and route on `Resolved.Engine`
   - record the engine in `run.json`
   - add an engine registry in `cliwire` in place of the ~15 hardwired constructors
   - let cross-run readers dispatch per run

   Medium.
3. A minimal `codexengine`:
   - Prepare: launch line with model, effort, `--yolo` or a sandbox mode, `--no-alt-screen`, CLAUDE.md fallback, a per-run hooks config and trust bypass
   - ParseEvents and SessionSignalParser fed from the Stop, UserPromptSubmit and SessionStart hooks
   - Startup and trust classifier
   - ComposeSend, TextPaster, InputBoxReader and InterruptGuard

   Medium to large; claudeengine is about 3,900 non-test lines.
4. Skills: a Codex `SkillLoader` using `$name` mentions, plus deploy syncing the scribe skills into `~/.agents/skills`. Small to medium.
5. Port the deny guardrails as Codex PreToolUse hooks, the raw-go gate-slot guard first. Medium.
6. Messaging:
   - render the no-parent directive, or a `lyx`-verb directive, for Codex strands
   - take the hold-notice wording out of `shuttleengine/hold.go` and give it per engine
   - have the orch answer Codex strands through `lyx shuttle send`

   Medium.
7. Observability: UsageReader and ActivityReader from rollout `token_count`, and Codex support in tokencount. Medium.
8. Forks, background-wake and orch parity. Large; defer.

Billing: OpenAI publishes no rule that prices `codex exec` differently from the TUI under a ChatGPT plan.
The tmux constraint is Anthropic-specific.
Keeping the TUI still reuses all of shuttle and reed, which is the cheaper route for now.
