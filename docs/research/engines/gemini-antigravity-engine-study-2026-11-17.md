# Gemini CLI and Antigravity as a third agent engine

Desk study, 2026-11-17.
No tool was installed or run, and no account or API was called:
the findings come from lym's code at `8492b54a`, the public documents and, for Gemini CLI, its source.
The verdict on whether to build is the operator's.

## Verdict

Neither fits lym yet, and the first blocker is Google's terms, not the code.

- **Gemini CLI** is the cheaper build.
  It is open source, its hooks map almost one to one onto Claude Code's,
  and a per-run settings file and a session id chosen up front fit `Prepare` better than Codex's `-c` flags do.
  But Google has moved free and Google AI Pro / Ultra users off it to Antigravity CLI since 2026-06-18.
  It now serves Gemini Code Assist Standard / Enterprise and paid API or Vertex keys,
  and its README says it is not tuned for the Gemini 4 models.
  Building on it builds on a harness Google is winding down for individuals.
- **Antigravity CLI** (`agy`) is where Google's models and harness work now go,
  and its hooks give lym a turn-end signal that knows about background work.
  But it is a closed binary with no licence, its transcript layout is undocumented,
  and its FAQ says wrapping `agy` inside third-party coding agents or applications is not supported
  under consumer plans.
  lym is such a wrapper.
- If a Google engine is wanted, the better target is Antigravity CLI on a Gemini API key or a Gemini Enterprise account.
  Do it only after the operator has settled whether lym driving `agy` under that account is allowed,
  and after a hands-on spike like Codex's of 2026-10-24.
  Measured against Codex, the build to Codex's level is about 7 to 8.5k lines of non-test Go, 15 to 20 landed tasks,
  and 6 to 7 spikes and trials.

## What an engine gives lym today

The seam is `shuttleengine.Engine` in `internal/shuttleengine/engine.go`,
and only `internal/shuttleengines` builds one.

- **The core interface.**
  - `Prepare` writes the run's artifacts, returns the launch and resume lines, and writes the launch manifest.
  - `ParseEvents` reads the hook-fed events file.
  - `Startup`, `TrustDismissSequence`, `InterruptSequence`, `ComposeSend` and `ColorSequence` read and drive the pane.
  - `PromptCacheLifetime` and `KeepWarmCaps` size the keep-warm waits.
- **Optional capabilities.** Each is an interface the engine may implement;
  lym works around a missing one or refuses the role.
  - Creel delivery is `ToolBoundaryContextRenderer`, with the idle waiter or `IdleWaker`.
  - Hooks that act on tool calls are `BatchingNudger` and `EditRecorder`.
  - Skills and instructions are `SkillLoader` and `InstructionFileLister`.
  - Sessions are `SessionResumer`, `SessionHolderFinder` and `SessionOpener`.
  - Pane readings are `InputBoxReader`, `InterruptGuard`, `TextPaster`, the picker readers and `SessionCycler`.
  - Forks are `RequirementSupporter`, `ForkAuditor` and `OutstandingForkReader`.
  - Records are `TranscriptReader`, `UsageReader` and `MessagePricer`, read by tokencount and the usage writer.
- **Requirements.**
  - A spec that forks requires `RequirementForks`.
  - `RequirementBackgroundWake` is required by:
    - Merriam's run level (`internal/websterengine/runlevel.go`);
    - the loom driver (`internal/loomcli/driverspec.go`);
    - the Discussion and Plan leads (`internal/loomengine`).
  - Codex meets neither, so those roles stay on Claude.
- **Outside the engine package**, a new engine touches:
  - the registry entry, its config module and its env scrub in `internal/shuttleengines/shuttleengines.go`;
  - the known engines and model aliases in `internal/modelspec`;
  - a file target and a prompt target in `internal/heddleengine/target.go`;
  - the per-engine steer and notice texts of the shell guard rows in `internal/shellguard/rows.go`;
  - a copy of each engine block under `contracts/stencils/engine/<engine>/`;
  - `internal/shuttleparity`.
    It names `ReferenceEngine` (claude) and `ComparedEngine` (codex) as two constants,
    so a third engine first turns that check into reference against each other engine.
- **The launch manifest and parity test** (`3bbc92a0`) fail a test when an engine's launch lacks an item Claude's has:
  a standing passage, an instruction source, a skill load or listing, a plugin context, a guard row,
  the batching nudge or a notice.
  A third engine therefore has to reach parity before its seats pass the test, not after.

## The two candidates

| | Gemini CLI | Antigravity CLI |
|---|---|---|
| Version read | v0.68.0 stable, 2026-11-10 (v0.69.0-preview.0 exists) | `agy` v1.5.10, 2026-11-17 (date from a release tracker); Antigravity App v2.32.1, 2026-11-13 |
| Source | Apache-2.0, TypeScript, about 284k non-test lines | public repository with no licence; closed Go binary; the Python SDK is Apache-2.0 over a closed runtime |
| Release pace | stable weekly, preview weekly, nightly daily | a CLI release most days in the week read |
| Who may use it | Code Assist Standard / Enterprise, Code Assist for GitHub, legacy paid API and Vertex keys | free, Google AI Pro / Ultra, Google Cloud, and a Gemini API key (`modelProvider: "gemini"`) |
| Models | Gemini 3.x and 2.5; README: not tuned for Gemini 4 | Gemini 4 Carbon / Argon / Hydrogen, Gemini 3.x Flash and Pro, Claude 5.5 |
| Terms on third-party use | using third-party tools to access the services behind it, e.g. with its OAuth, violates the terms | using third-party software to access Antigravity violates the terms; wrapping `agy` in third-party agents or applications is not supported under consumer plans; for third-party agents it recommends a Gemini Enterprise or AI Studio API key |

Antigravity CLI shares the Antigravity App's agent harness, not Gemini CLI's code.
It reads `GEMINI.md` and `AGENTS.md` as Gemini CLI does, and `agy plugin import gemini` converts Gemini CLI extensions.

Antigravity the IDE is a desktop editor and has no place in lym's pane model; only its CLI is judged below.

## Item by item

Native: the tool does it as documented.
Workaround: lym can reach it another way, named in the cell.
Missing: neither.

| lym need | Codex today | Gemini CLI 0.68.0 | Antigravity CLI 1.5.10 |
|---|---|---|---|
| Pane launch under reed | TUI with `--no-alt-screen`, prompt pointer as first turn | **Native**: normal screen buffer by default, `-i "<prompt>"` runs a first prompt and stays interactive | **Native**: `-i "<prompt>"`; `altScreenMode: "never"` only in `settings.json`; no cwd flag, the pane cwd is the workspace |
| Pane readings (ready, busy, interrupt, paste) | `›` composer, `Working (… esc to interrupt)` | **Native**, readable from source: placeholder `Type your message or @path/to/file`, `(esc to cancel, Ns)`, Esc cancels, bracketed paste always on, large pastes collapse to a placeholder | **Workaround**: Ctrl+C interrupts and Alt+M queues; the screens are learnt only from a spike, since the source is closed |
| Folder trust gate | lym keeps the trusted roots in the lym Codex home | **Native**: off by default; `--skip-trust` or `GEMINI_CLI_TRUST_WORKSPACE=true` skips it | **Workaround**: `trustedWorkspaces` in settings; the prompt's shape is unknown |
| Model and effort | `-m`, `-c model_reasoning_effort=…` | **Workaround**: `-m`; the thinking level only through `modelConfigs` in a settings file | **Native**: `--model`, `--effort` (`low` to `max`) |
| Per-run config, user's files untouched | `CODEX_HOME` plus `-c` settings | **Native**: `GEMINI_CLI_SYSTEM_SETTINGS_PATH` names a per-run settings file at the highest file layer; `GEMINI_CLI_HOME` moves the home | **Workaround**: no documented home or settings override; hooks load only from `.agents/hooks.json`, `~/.gemini/config/hooks.json` or a plugin, so lym installs one plugin whose hooks read the run's paths from variables on the launch line |
| Turn and session events into the events file | SessionStart, UserPromptSubmit, Stop, PermissionRequest hooks; exit record | **Native**: SessionStart, SessionEnd, BeforeAgent (`prompt`), AfterAgent (`prompt_response`), Notification; payloads carry `session_id` and `transcript_path` | **Native**: SessionStart, PreInvocation, PostInvocation, Stop (`terminationReason`, `fullyIdle`); payloads carry `conversationId` and `transcriptPath`; no session-end hook, so an exit record as on Codex |
| Shell guard rows and question deny | PreToolUse hook prints a deny with the Codex steer | **Native**: BeforeTool `decision: "deny"` with `reason`, or exit 2; the TOML policy engine also denies with a `deny_message` | **Native**: PreToolUse `decision: "deny"` with `reason`; `permissions.deny` rules |
| Creel mail at a tool boundary | Stop or PostToolUse block; Codex reads the call as failed | **Native**: AfterTool `additionalContext` appends the mail to the tool result | **Native**: PostInvocation `injectSteps` with a `systemMessage` |
| Creel mail at a turn end | Stop block continues the turn | **Workaround**: AfterAgent `decision: "deny"` sends `reason` as a new prompt, but it rejects the reply as a retry | **Native**: Stop `decision: "continue"` re-enters the loop with `reason` injected as a system message |
| Idle wake | async waiter hook, then `codex queue --thread` | **Missing**: hooks run synchronously and nothing queues a turn from outside; creel's bell typed into the pane is the fallback | **Missing** for the TUI: the remote-control dashboard has no API; creel's bell typed into the pane is the fallback |
| Edit records for the edit-loop watch | none (board note `codex-edit-loop-recorder`) | **Native**: AfterTool on the write and replace tools carries `tool_input` | **Native**: PostToolUse with a tool matcher; payload fields not documented |
| Batching nudge | PostToolUse hook reads the rollout tail | **Native**: the chat JSONL records each tool call | **Workaround**: the transcript JSONL schema is undocumented |
| Skill load (`SkillLoader`) | the model reads each SKILL.md by path | **Workaround**: `activate_skill` shows a consent prompt that would stall an unattended pane; load by path as on Codex | **Native**: Agent Skills in `.agents/skills`, each a slash command; loading by path as on Codex is the uniform route |
| Plugin contexts and user setup | carried in `developer_instructions` | **Native**: SessionStart `additionalContext` | **Native**: SessionStart `injectSteps` |
| Standing passages and notices | `developer_instructions` on the launch line | **Workaround**: `GEMINI_SYSTEM_MD` replaces the whole system prompt and cannot append; carry them as SessionStart context | **Workaround**: no system-prompt flag; SessionStart `injectSteps`, or a rules file |
| Instruction files and heddle target | `AGENTS.override.md` (`codex-file`) | **Native**: `context.fileName` takes a list, so `CLAUDE.md`, `AGENTS.md` and `GEMINI.md` are all read | **Workaround**: reads `AGENTS.md`, `GEMINI.md` and `.agents/rules/*.md` but not `CLAUDE.md`; a rules file is capped at 24,000 bytes, within a shared 20,000-token budget |
| Forks (`RequirementForks`) | not met | **Missing**: subagents in `.gemini/agents/*.md` start with a clean context and cannot nest | **Missing** for agents: `invoke_subagent` starts clean; `/fork` clones a conversation, but only as the user's command |
| Background wake (`RequirementBackgroundWake`) | not met | **Workaround**, experimental: `tools.shell.backgroundCompletionBehavior: "inject"` with `experimental.modelSteering` starts a turn when a background shell ends | **Workaround**: Stop's `fullyIdle: false` marks a turn end with background work running, which is lym's waiting turn end; whether completion then starts a turn is not documented |
| Session id and resume | Codex mints the id at the first turn, so the resume line starts nothing | **Native**: `--session-id <id>` chosen up front, `--resume <uuid>` | **Native**: `--conversation <id>`, `-c`; whether the id can be chosen up front in the CLI is not documented |
| Usage, context size and cost | rollout `token_count`; no price table | **Native**: each model message in the chat JSONL records `input`, `cached`, `output`, `thoughts`, `tool` and `total` tokens; a price table is lym's to add | **Workaround**: `usage` only in headless JSON and the SDK; the transcript's token fields are undocumented |
| Liveness | no registry; exit record | **Missing**: no session registry known; exit record as on Codex | **Missing**: none documented; exit record as on Codex |
| Clear, compact, color | `/clear`, `/compact`; no color | **Native** `/clear` and `/compress`; **missing** `/color` | **Native** `/clear`; compaction command not found; **missing** color |
| Permissions and sandbox | `-a never`, `-s` mode settled by a bubblewrap probe | **Native** `--approval-mode yolo`; the sandboxes are containers (Docker, Podman, gVisor, LXC) with no native Linux one, so lym would run without a sandbox as Claude seats do | **Native** `--dangerously-skip-permissions`; `--sandbox` on Linux uses user namespaces, the same limit Ubuntu's AppArmor puts on Codex's bubblewrap |
| Headless | not used | `-p` with `stream-json`; lym's pane rule leaves it unused | `-p` with `stream-json` input and output; unused for the same reason |

## The Codex yardstick

Each Codex task landed as one squashed commit.
Lines are added / removed, from `git diff --numstat <commit>^1 <commit>`.
Go is non-test Go, Test is `_test.go`, Other is stencils, markdown, YAML and test data.

**Seam work before any Codex code (2026-10-15).** A third engine inherits it.

| Commit | Task | Go | Test | Other |
|---|---|---|---|---|
| `d35dd56d` | Engine registry: shuttle picks and records each run's engine | +996/-271 | +1709/-223 | +19/-13 |
| `73205119` | Engine-owned config, shell-guard table, provider-blind reed | +1514/-457 | +1716/-722 | +293/-22 |
| `220fa9e8` | Every role spec carries its resolved engine through ModelChoice | +643/-456 | +1715/-318 | +37/-8 |
| `d4715f21` | tokencount reads usage and cost through the shuttle engines | +1094/-661 | +1287/-435 | +580/-1 |

**Build and first extension (2026-11-03 to 2026-11-11).**

| Commit | Task | Go | Test | Other |
|---|---|---|---|---|
| `935fb407` | Codex CLI as a second shuttle engine: codexengine | +3246/-325 | +4132/-127 | +230/-10 |
| `cd897ef0` | run takes a model spec, open starts codex | +220/-105 | +327/-29 | +1/-1 |
| `dde941fc` | refuse hidden anchor; Codex state follows its turns | +21/-12 | +100/-86 | 0 |
| `6762816f` | Codex sessions read idle, end on exit, and reach creel | +64/-13 | +65/-15 | 0 |
| `fe2663d3` | Codex sessions can commit in a worktree and use the Go cache | +73/-14 | +117/-18 | +2/-1 |
| `e3aedbc5` | Codex sessions get creel mail through hooks | +277/-51 | +339/-28 | +3/-3 |
| `af100b7b` | creelengine: hold the bell while the woken turn runs | +15/-10 | +29/-0 | 0 |
| `81e74d76` | Credit Codex in the landing credit block | 0 | +1/-1 | +1/-0 |
| `0e02aaad` | read a skill loaded when a tool output holds its SKILL.md | +89/-26 | +48/-3 | 0 |
| `539525a0` | a machine-settable `sandbox_mode` key | +70/-29 | +121/-44 | +6/-4 |
| `9874c966` | sandbox probe at launch, auto mode | +132/-20 | +249/-14 | +2/-2 |

**Parity with Claude (2026-11-11 to 2026-11-17).**

| Commit | Task | Go | Test | Other |
|---|---|---|---|---|
| `15de027c` | heddle: split claude-file and codex-file, add agents-file | +161/-44 | +143/-17 | 0 |
| `22b74f24` | Render lym's heddle text into AGENTS.override.md too | 0 | +61/-30 | +4/-4 |
| `3a854635` | Render every producer's prompt and edit directive for its launch engine | +845/-454 | +1093/-253 | +64/-39 |
| `f8cc499c` | Codex launches carry Claude's standing items and user setup | +1600/-84 | +1933/-84 | +17/-2 |
| `40c66912` | Move Claude-only wording into engine blocks with Codex copies | +360/-111 | +681/-48 | +255/-116 |
| `b2cd8150` | Codex gets the question-tool deny and the batching nudge | +803/-88 | +751/-76 | +172/-12 |
| `3bbc92a0` | Launch manifests per engine, per-seat parity test, `lym shuttle manifest` | +2118/-93 | +2739/-23 | +74/-2 |

The Codex work after the seam came to 18 landed tasks, about 8.6k net non-test Go and 12.0k net test Go.
`internal/shuttleengine/codexengine` now holds 4,079 non-test and 4,267 test lines,
against 8,271 and 11,116 in `claudeengine`.
The build took 9 calendar days and the parity series 3.
Hands-on work ran beside it, each piece recorded as its own report under `docs/research/codex/`:
three spikes and an audit before the build (2026-10-24), a hook-delivery test (10-28),
and five engine trials (11-04 and 11-05), each of which led to fix tasks within a day.

No record gives the tokens, dollars or session time of the runs that built these tasks:
the credit blocks name no usage, and the run records under `_lym/` are not tracked.
The cost is therefore given in lines and landed tasks only.

## Estimate per candidate

About 3.1k lines of the parity series' non-test Go is one-time machinery a third engine reuses:
the per-launch prompt engine, the manifests and parity test, and the heddle split.
The per-engine share of Codex's work is then about 7k lines of non-test Go added.
The estimates scale that share by what each candidate makes cheaper or dearer.

| | Codex, landed | Gemini CLI, estimate | Antigravity CLI, estimate |
|---|---|---|---|
| Engine package, non-test Go | 4.1k | 3 to 3.5k | 3.5 to 4.5k |
| Outside the package, non-test Go | about 2.9k per-engine share | 2.5 to 3k | 3.5 to 4k |
| Non-test Go added | about 7k per-engine share | 5.5 to 6.5k | 7 to 8.5k |
| Test Go added | about 9k per-engine share | 7 to 9k | 9 to 11k |
| Landed tasks | 18 | 12 to 15 | 15 to 20 |
| Spikes and trials | 3 spikes, 5 trials | 1 spike, 3 trials | 2 spikes, 4 to 5 trials |

The outside-the-package share includes three costs Codex did not have:
reworking `internal/shuttleparity` from two named engines to the reference against each other engine;
a third copy of each block under `contracts/stencils/engine/`;
and a price table, which Codex still lacks.

**What makes Gemini CLI cheaper than Codex.**
- The pane readings and the transcript schema can be read from source instead of probed.
- The hook payloads match Claude Code's in shape, down to a `CLAUDE_PROJECT_DIR` alias.
- A per-run settings file replaces Codex's argument-length-capped `developer_instructions` and its home record.
- `--session-id` gives the resume line a session to resume.

**What it costs on top.**
- The standing passages ride SessionStart context instead of a system prompt.
- The thinking level is set through the settings file.
- The skill-consent prompt has to be avoided.
- With no idle wake, creel falls back to the pane bell.

**What makes Antigravity CLI dearer than Codex.**
- The closed binary and undocumented transcript put every reading behind a spike.
- Hooks need an installed lym plugin and launch-line variables instead of a per-run file.
- The rules-file caps force lym's instruction files into SessionStart steps or several rules files.
- Keyring auth has to survive a pane launch.
- A CLI release most days means the pane readings and the transcript reader need keeping up.

**What it gives back.**
- The Stop hook's `continue` and `fullyIdle` fit creel's turn-end delivery and lym's waiting turn end
  better than Codex's hooks do.
- `fullyIdle` is the first signal on any second engine toward `RequirementBackgroundWake`.

## A first minimal level

The first level matches Codex's: the single-session, fork-free roles whose output is files.
These are the darn writer, a Burler reviewer and fixer with no fan, the landing describe,
the Bouncer judge and the friction reflection.
It holds:

- the registry entry, its config module and env scrub, the model aliases, and a heddle file and prompt target;
- `Prepare` with the launch and resume lines, the hook wiring that feeds the events file, and the exit record;
- `ParseEvents`, the startup, trust and interrupt readings, and the paste-based send with an input-box reading;
- every shell guard row and the question-tool deny as deny hooks, with the engine's own steer and notice texts;
- skill load by SKILL.md path, the user setup and plugin contexts as SessionStart context, and the standing passages;
- creel delivery at the tool boundary and the turn end, and the pane bell for an idle session;
- a usage reader from the transcript, and the launch manifest passing the parity test for those seats;
- a copy of each engine block under `contracts/stencils/engine/`.

It leaves out:

- forks, background wake and therefore the Merriam, Discussion and Plan leads and the loom driver;
- the orch, liveness, session colour and the batching nudge.

For Gemini CLI that level is about 4k lines of non-test Go and 8 landed tasks.
For Antigravity CLI it is about 5k and 10, after the spike.

## Not settled by documents

- Whether lym driving `agy` in a pane counts as wrapping it under the FAQ.
  That holds for a consumer login, a Gemini API key and a Gemini Enterprise account alike.
- Whether Gemini CLI on a Code Assist Standard licence may be driven by lym.
  Its terms text forbids reaching its services with third-party tools through its OAuth, not running the CLI itself.
- How either TUI behaves under tmux: bracketed paste through `paste-buffer -p`, an Enter right after a paste,
  and the screens of the busy, idle and trust states.
- Whether Gemini CLI's `activate_skill` consent can be skipped,
  and whether its experimental background-completion wake runs as the source reads.
- What AfterAgent's retry does to the rejected reply in Gemini CLI's history when creel mail continues a turn.
- Antigravity CLI's transcript schema and token fields, its on-disk layout,
  and whether its keyring login works in a lym pane.
- Whether Antigravity CLI accepts a conversation id chosen up front,
  and whether its `self` subagent inherits the caller's history.

## Sources

Gemini CLI, at tag `v0.68.0` of `https://github.com/google-gemini/gemini-cli`:
- `README.md` (status notice), `docs/resources/tos-privacy.md`, `docs/resources/quota-and-pricing.md`,
  `docs/get-started/authentication.mdx`;
- `docs/hooks/index.md`, `docs/hooks/reference.md`, `docs/cli/cli-reference.md`, `docs/cli/trusted-folders.md`;
- `docs/cli/skills.md`, `docs/cli/gemini-md.md`, `docs/cli/system-prompt.md`, `docs/cli/session-management.md`;
- `docs/cli/sandbox.md`, `docs/cli/headless.md`, `docs/core/subagents.md`;
- `docs/reference/configuration.md`, `docs/reference/policy-engine.md`, `docs/reference/commands.md`;
- source: `packages/cli/src/config/config.ts`, `packages/cli/src/ui/components/LoadingIndicator.tsx`,
  `packages/cli/src/ui/AppContainer.tsx`, `packages/core/src/services/chatRecordingService.ts`,
  `packages/core/src/services/chatRecordingTypes.ts`, `packages/core/src/hooks/types.ts`.

Antigravity, under `https://antigravity.google/docs/`:
- `faq`, `hooks`, `cli/reference`, `cli/headless`, `cli/gcli-migration`, `cli/using`, `cli/conversations`;
- `rules`, `skills`, `subagents`, `permissions`, `sandbox`, `models`, `plans`, `remote-control`, `changelog`;
- `https://github.com/google-antigravity/antigravity-cli` (README, CHANGELOG; no licence file).

The terms texts, Gemini CLI's status notice and Antigravity's hook schema were read from the fetched pages themselves.
The rest is the reading of documents and source above, not checked on a running tool.
