# Stencil audit: Claude-specific mechanics and what a second engine needs

Audit of 2026-10-24 at commit `f7b7c852d`, for the note `codex-engine`.
Nothing was built; every site was found by reading `contracts/stencils/`, `contracts/specs/` and the Go code that renders or inlines prompt text.
Line numbers are those of that commit.
The Codex column of every block cites only `docs/research/codex-spike-2026-10-24/report.md` (the spike); what the spike did not exercise is marked untested, and a mechanic the spike found no counterpart for is marked no counterpart.

## Summary

- Most stencil text names a task and is engine-neutral.
  The Claude-specific text is mechanics, and it falls into thirteen mechanics, each proposed as one named include block that an engine supplies.
- Three mechanics carry most of the rewrite and most of the risk: the fork spawn (blocks 1 to 5), the wait on background completion (block 6) and the agent-to-agent message (block 7).
  The spike found a Codex counterpart for the fork spawn, a partial one for the wait, and none for the message.
- The text of the include blocks sits in stencils; the mechanics that make the text true (hooks, pane keys, skill load, `--append-system-prompt`) already live in `internal/shuttleengine/claudeengine`, and a `codexengine` would carry its own.
- Size: 13 blocks, 127 stencil sites in 45 of 83 stencils, 8 spec sites in 6 of 11 specs, and 35 Go sites (see Size estimate).

## Reading the tables

A site is one file and line range where the text depends on the mechanic.
A site tagged `comment` is an HTML comment in a stencil, which no agent reads, and needs a docs edit only.
A site that uses the word "fork" only as the name of a lyx role, such as a list of process words to avoid, depends on no mechanic and is not listed.

Codex terms from the spike: `spawn_agent` (arguments `task_name`, `message`, `fork_turns`), the hooks `SessionStart`, `UserPromptSubmit`, `Stop`, `PermissionRequest` and `PreToolUse`, `AGENTS.md`, `.agents/skills/<name>/SKILL.md` with `$name`, the rollout file, and `-a never -s workspace-write`.

## Block 1: `spawn-fork-fan`

One message spawns every read-only lens fork of a review, or the single writer fork `fork0`.

The Claude text each site would hold, from `contracts/stencils/twill/twill-directive.md:13`:
"Spawn every lens below in one message through the Agent tool with `subagent_type: "fork"`, one fork per lens, and never pass a `name`."

Codex needs: a `spawn_agent` call per lens with `fork_turns` set to `all`, since the spike shows the default `none` inherits nothing; each call needs a `task_name`, so "never pass a `name`" becomes "pass a lens task name".
Whether several `spawn_agent` calls in one turn run in parallel is untested.
A fork that returns nothing is described by the spike as a wait that returns `No agents completed yet`, so "a fork that returned no report" needs a defined Codex reading.

| Site | Mechanic |
|---|---|
| `contracts/stencils/burler/burler-cluster-fan.md:4` | Spawn all fork reviewers in a single message via the Agent tool; named forks lose inherited context |
| `contracts/stencils/burler/burler-cluster-fan.md:11` | Spawn the forks in the same turn as the reading manifest |
| `contracts/stencils/burler/burler-cluster-fan.md:20-22` | Write nothing while a fork is outstanding; save each fork's final message verbatim |
| `contracts/stencils/burler/burler-fix-self-check.md:10` | Fixer spawns the lenses in one message through the Agent tool |
| `contracts/stencils/burler/burler-fix-self-check.md:12-14` | Wait for every fork to return; save each final message verbatim |
| `contracts/stencils/burler/burler-fix-self-check.md:22-23` | One fork pass per fixer; a resumed session has lost its forks |
| `contracts/stencils/twill/twill-directive.md:13` | Writer spawns every lens in one message through the Agent tool |
| `contracts/stencils/twill/twill-directive.md:15-17` | Wait for every fork to return; save each final message verbatim |
| `contracts/stencils/twill/twill-directive.md:22` | One fork pass per writer spawn |
| `contracts/stencils/twill/twill-writer-fork.md:12` | Writer spawns `fork0` through the Agent tool with `subagent_type: "fork"` |
| `contracts/stencils/twill/twill-writer-fork.md:24-34` | Resume rules keyed to whether the Agent call was in flight, returned or never made |
| `contracts/stencils/twill/twill-relaunch.md:6-12` | A relaunched writer that lost its Agent call spawns no second fork |
| `contracts/stencils/twill/twill-judge-plain.md:9` | Name a fork that returned no report |
| `contracts/stencils/twill/twill-judge-critical.md:21` | Name a fork that returned no report |
| `contracts/stencils/burler/burler-cluster-solo.md:4` | Names the absence of cluster forks (neutral wording, kept for the block's other branch) |
| `contracts/stencils/burler/burler-review-format.md:48` | `origin:` key names a fork lens |
| `contracts/stencils/burler/burler-template-reviewer.md:12` | `comment`: points at the cluster fork-spawn prose |
| `contracts/stencils/loom/loom-template-discussion-chair.md:2` | `comment`: the lead forks its lens review |
| `contracts/stencils/loom/loom-template-discussion-advisor.md:1` | `comment`: an advisor never forks |

Go sites that decide the fan from the engine side:

| Site | Mechanic |
|---|---|
| `internal/loomengine/plan.go:14-21` | Doc: the lead sets `ForkSubagents` so the writer may spawn subagents of type fork; the Agent hook admits forks only |
| `internal/seatengine/doc.go:11` | Doc: `ForkSubagents` authorizes a seat to spawn in-session fork subagents |
| `internal/shuttleengine/claudeengine/settings.go:32-33,157-161` | `PreToolUse(Agent)` hook admits only `"subagent_type":"fork"` |

## Block 2: `spawn-batch-fork`

Webster's Merriam spawns one implementer fork per batch, or one verify-gate fixer fork, in the background, with a prompt forwarded verbatim.

The Claude text, from `contracts/stencils/webster/webster-template-master.md:74`:
"Spawn exactly ONE fork via the Agent tool, `subagent_type: "fork"`, NO name."

Codex needs: `spawn_agent` with `fork_turns` `all` and a verbatim `message`.
The spike observed a subagent produce no hooks of its own and write its own rollout under `source.subagent.thread_spawn`, so the audits that read fork transcripts (`record-batch`, the exit audit) need a rollout reader for subagents.
Backgrounding is a property of `spawn_agent` plus a wait call in Codex, not of the spawn itself (see block 6).

| Site | Mechanic |
|---|---|
| `contracts/stencils/webster/webster-template-master.md:3` | `comment`: forks one implementer per batch in-session (Claude Code's fork) |
| `contracts/stencils/webster/webster-template-master.md:12` | Role disambiguation keyed on a spawn "via the Agent tool" |
| `contracts/stencils/webster/webster-template-master.md:74-75` | Spawn exactly one fork, `subagent_type: "fork"`, no name; the prompt is a fixed verbatim line |
| `contracts/stencils/webster/webster-template-master.md:86-87` | Fixed sequence: `subagent_type: "fork"` with no name, prompt forwarded verbatim |
| `contracts/stencils/webster/webster-template-master.md:114` | Re-fork a batch with the same prompt file after a missing report |
| `contracts/stencils/webster/webster-template-master.md:152` | Re-fork after an archived report |
| `contracts/stencils/webster/webster-template-master.md:184` | Spawn one fixer fork in the background the same way (Agent tool, no name) |
| `contracts/stencils/webster/webster-template-master.md:250` | Never spawn a non-fork or named subagent |
| `contracts/stencils/webster/webster-prefix-fork.md:3` | `comment`: Master's Agent-tool fork call |
| `contracts/stencils/webster/webster-prefix-verify-fix-fork.md:2` | `comment`: Merriam's Agent-tool fork call idiom |
| `contracts/stencils/webster/webster-prefix-recovery.md:11` | Contrasts the cold recovery strand with an in-session fork |
| `contracts/stencils/pattern/pattern-directive-orchestrator.md:2,11,13` | Master "only forks": constraints are read before forking |

Go sites:

| Site | Mechanic |
|---|---|
| `internal/websterengine/audit.go:174` | Audit message: "attempted N Agent tool call(s)" |
| `internal/burlerengine/cluster.go:231` | Cluster error message: a fork attempted an Agent tool call |
| `internal/shuttleengine/forkaudit.go:27,43,47` | Fork audit counts Agent tool invocations and Bash `tool_use` entries in a Claude transcript |

## Block 3: `spawn-oneshot-fork`

A parent, driver or review loop reads a stop, a review notice or an intake list through one fork of its own session.

The Claude text, from `contracts/stencils/orch/orch-template-role.md:88`:
"the stop goes to a one-shot fork."
No stencil names the Agent tool here; the fork is the Claude fork the word names.

Codex needs: a `spawn_agent` with `fork_turns` `all`, or a `/fork`.
The spike shows `/fork` asks a worktree question and starts a new session, which a role cannot answer unattended, so the subagent route is the one the stencils need.

| Site | Mechanic |
|---|---|
| `contracts/stencils/orch/orch-template-role.md:43` | Never fork a review, even when the brief asks for a one-shot fork |
| `contracts/stencils/orch/orch-template-role.md:57` | Never fork a hand-on question |
| `contracts/stencils/orch/orch-template-role.md:74` | Peer-run questions never go to a fork |
| `contracts/stencils/orch/orch-template-role.md:87-100` | A stop goes to one read-only one-shot fork; open forks are named in the note |
| `contracts/stencils/orch/orch-template-role.md:117-119` | A code fix never goes to a fork; a test run goes to a fork or an agent |
| `contracts/stencils/orch/orch-template-role.md:190-198` | Intake runs through one one-shot fork |
| `contracts/stencils/orch/orch-template-role.md:246-247` | Role table lists forks as spawned agents |
| `contracts/stencils/orch/orch-template-note.md:10` | The orch note records a fork left half done |
| `contracts/stencils/orch/orch-template-handoff-soft.md:4` | A cycle is deferred while a fork is mid-task |
| `contracts/stencils/orch/orch-template-compact.md:4` | Compact focus keeps every open review or stop fork |
| `contracts/stencils/bouncer/bouncer-template-escalation.md:1,8` | Brief is read by a one-shot fork of the parent session |
| `contracts/stencils/bouncer/bouncer-template-parent-notice.md:1` | Notice asks the parent to review with a one-shot fork |
| `contracts/stencils/loom/loom-template-parent-review-brief.md:1` | `comment`: brief handed to the parent's one-shot fork |
| `contracts/stencils/loom/loom-template-parent-review-delivery.md:5` | The delivery message asks the parent to review with a one-shot fork |
| `contracts/stencils/shed/shed-template-driver.md:84-86` | Driver spawns exactly one fork of itself at an error stop; never a named or other subagent |
| `contracts/stencils/shed/shed-template-driver-guide.md:1,8` | The guide is read by the one-shot fork a driver spawned |
| `contracts/stencils/friction/friction-directive-implementer.md:2-3` | `comment`: names the fork prompts that consume the directive |

## Block 4: `fork-inheritance`

Text that is true only because a fork inherits the spawner's context: reads done once before the spawn, a rubric read "so every fork inherits it", a prompt that warns the fork its history looks like the spawner's.

The Claude text, from `contracts/stencils/burler/burler-fork-reading.md:6`:
"each fork inherits your context as it stands at the spawn."

Codex needs: `fork_turns: "all"` on every spawn.
The spike shows a subagent spawned with `none` answered `No secret number has been shared in this conversation`, and with `all` answered the number, so the claim holds on Codex only if the spawn block sets it.
The spike did not test whether the spawner's `AGENTS.md` message and tool definitions are part of what a fork inherits.

| Site | Mechanic |
|---|---|
| `contracts/stencils/burler/burler-fork-reading.md:1-2,6,7,19,21` | Pre-read so every fork inherits it; a judgement in context colours every fork |
| `contracts/stencils/burler/burler-cluster-fan.md:16` | Fork boilerplate: the inherited context already holds the target, the rules and the reading |
| `contracts/stencils/burler/burler-fix-self-check.md:9,11` | Read the rubric so every fork inherits it; the fork prompt is the boilerplate and nothing else |
| `contracts/stencils/burler/burler-fix-check-rubric.md:1-2` | `comment`: the fixer reads the rubric before it spawns so every fork inherits it |
| `contracts/stencils/twill/twill-directive.md:12` | Read the rubric so every fork inherits it |
| `contracts/stencils/twill/twill-writer-fork.md:11,15,21` | `fork0` and the lens forks inherit the rubric; `fork0`'s context holds the input record |
| `contracts/stencils/twill/twill-relaunch.md:7` | A relaunched writer reads back every file it did not write so the forks inherit them |
| `contracts/stencils/webster/webster-template-master.md:9-14` | The prompt is inherited by every fork; the spawn instruction is authoritative over inherited history |
| `contracts/stencils/webster/webster-template-master.md:18-19,28-31` | Reads Master does once are what every fork inherits; Master never reads code |
| `contracts/stencils/webster/webster-template-master.md:75,184` | The fork prompt warns that the inherited context looks like Master's own |
| `contracts/stencils/webster/webster-prefix-fork.md:4-5,12` | `comment` and body: forked in-session, inherits Master's context |
| `contracts/stencils/webster/webster-prefix-verify-fix-fork.md:8,14` | Fixer fork forked in-session from Merriam, not Merriam |
| `contracts/stencils/webster/webster-prefix-verify-fix-cold.md:2` | `comment`: the cold strand inherits nothing, unlike the fork prefix |
| `contracts/stencils/webster/webster-prefix-recovery.md:3,28` | Recovery strand gives what an in-session fork inherits |
| `contracts/stencils/pattern/pattern-directive-orchestrator.md:19-21` | Every fork inherits the constraints; they bind the forks it spawns |
| `contracts/stencils/friction/friction-directive-orchestrator.md:2,18,20,23` | Friction note keyed to forking implementers |

## Block 5: `no-nested-spawn`

The boilerplate that bans a fork from calling the Agent tool.

The Claude text, from `contracts/stencils/twill/twill-directive.md:36`:
"never call the Agent tool" and, at line 39, "A fork never calls the Agent tool, so the writer is the only session that spawns."

Codex needs: a statement in terms of `spawn_agent`, and an enforcement.
Claude enforces by a `PreToolUse(Agent)` deny hook; the spike showed `PreToolUse` deny works for `Bash` and fires with a `matcher` of `*`, but not whether it fires for `spawn_agent`.
Subagents fire no hooks of their own in the spike, so a deny hook may not reach a fork's own calls at all: untested, and the likeliest gap in this block.

| Site | Mechanic |
|---|---|
| `contracts/stencils/burler/burler-fix-self-check.md:37,40` | Fork boilerplate and its closing sentence |
| `contracts/stencils/twill/twill-directive.md:36,39` | Fork boilerplate and its closing sentence |
| `contracts/stencils/twill/twill-writer-fork.md:18` | `fork0` never calls the Agent tool |
| `contracts/stencils/webster/webster-body-implementer.md:66` | Implementer never calls the Agent tool; nested forks are banned |

Go sites:

| Site | Mechanic |
|---|---|
| `internal/shuttleengine/claudeengine/settings.go:29-47` | Deny reasons and system-prompt notices for the Agent tool, fork and non-fork |
| `internal/shuttleengine/claudeengine/settings.go:164-168` | `PreToolUse(Agent)` deny hook for a non-fork run |

## Block 6: `await-completion`

A session ends its turn and a notification starts the next one: a fork's completion, a backgrounded `lyx` command's exit, a background shell job.

The Claude text, from `contracts/stencils/webster/webster-template-master.md:76-79`:
the fork is a backgrounded agent, its tool call returns at once, and "When the fork finishes, its completion notification starts your next turn."

Codex needs, from the spike:
a turn is done on the `Stop` hook or `task_complete`, and not on the pane alone;
a command left running in a background terminal keeps running and does not keep the turn busy, and the footer reads `N background terminal running`;
an Esc interrupt fires no `Stop`;
a subagent shows `Waiting for agents`, and the first wait returned `No agents completed yet`, so the parent waited again.
Untested, and no counterpart observed: a completion notification that starts a new turn after the session ended its turn, for a subagent or for a background command.
Every site below depends on that, and a Codex block may instead have to be a blocking wait call, which changes the "end your turn with no tool call" rule at its core.

| Site | Mechanic |
|---|---|
| `contracts/stencils/webster/webster-template-master.md:68` | Heading: begin-batch, fork, wait for its notification, record-batch |
| `contracts/stencils/webster/webster-template-master.md:76-82` | Fork is backgrounded; end the turn; the completion notification starts the next turn; a fork notice that says the fork is waiting is not completion |
| `contracts/stencils/webster/webster-template-master.md:88-90` | Turn ended while the fork runs, never a polling loop; `recover-batch` backgrounded |
| `contracts/stencils/webster/webster-template-master.md:114-120` | Re-fork then end turn; `recover-batch` as a backgrounded Bash command, the output arrives as a completion notification |
| `contracts/stencils/webster/webster-template-master.md:124-131` | Second and amended `recover-batch` runs backgrounded |
| `contracts/stencils/webster/webster-template-master.md:147,152-155` | `record-batch` refusals answered by a backgrounded `recover-batch` |
| `contracts/stencils/webster/webster-template-master.md:168-195` | Gate failure: backgrounded `fix-verify` or a background fixer fork; no polling, no `sleep`; a turn end is waiting |
| `contracts/stencils/webster/webster-template-master.md:202-204` | While a fork or backgrounded command runs, end the turn with no tool call |
| `contracts/stencils/webster/webster-template-master.md:231` | A correctness finding goes to a backgrounded `recover-batch` |
| `contracts/stencils/webster/webster-prefix-fork.md:22-24` | The Master is woken by the fork's completion notification |
| `contracts/stencils/webster/webster-body-implementer.md:28` | Never run a Bash call in the background |
| `contracts/stencils/webster/webster-body-verify-fix.md:22` | Never run a Bash call in the background |
| `contracts/stencils/shed/shed-template-driver.md:22-28` | Drive the run with one background job; the session is idle until the envelope; a turn ends only while the job runs |
| `contracts/stencils/shed/shed-template-driver.md:36-42` | Run the loop as one background job from the session's cwd |
| `contracts/stencils/shed/shed-template-driver.md:106-107` | The arbiter command runs as a background job; wait for its exit |
| `contracts/stencils/shed/shed-template-driver.md:120,127` | Re-launch as a background job; a wake finds no loop job |
| `contracts/stencils/shed/shed-template-driver.md:144,151` | Binary watch as a background job; a wake is a resume line, a job's exit or a compaction |
| `contracts/stencils/shed/shed-template-driver-guide.md:65` | The binary watch is a background shell loop |
| `contracts/stencils/orch/orch-template-role.md:34-35` | Start no Monitor; the wake-up is a line creel types |
| `contracts/stencils/orch/orch-template-adopt.md:4` | The previous process's background shells and Monitors are gone |
| `contracts/stencils/orch/orch-template-adopt.md:1` | `comment`: resumes an already-running Claude session |

Go sites:

| Site | Mechanic |
|---|---|
| `internal/shuttleengine/claudeengine/background.go:1-10` | Reads `background_tasks` of a Stop payload and `<task-notification>` messages; a fork is an Agent or Task launch, a shell a Bash or Monitor launch |
| `internal/websterengine/awaitbatch.go:22` | Wait cap kept under Claude Code's two-minute auto-background |
| `internal/shuttleengine/await.go:14` | Wait cap set under the Bash tool timeout |
| `internal/shuttlecli/await.go:39` | Help text: run the command with a Bash tool timeout above `--cap` |

## Block 7: `message-agent`

One agent messages another by name, or the named agent list is read.

The Claude text, from `contracts/stencils/loom/loom-template-parent-review-delivery.md:5`:
"send one message through Claude Code's SendMessage to {{.reviewer}}".

Codex: no counterpart.
The spike found no tool by which one Codex session addresses another by name, and no session name flag (the status line shows a thread name; setting it from the launch line is untested).
The one input path the spike proved is the pane: a bracketed paste followed by a separate Enter.
That is the creel path the orch already uses (`PATTERN-orch-pane-single-writer`), so the block could name a creel post in place of `SendMessage`, but every site that expects a reply "by `SendMessage` to the strand name" then needs a reply route, and `ListAgents` has no Codex counterpart.

| Site | Mechanic |
|---|---|
| `contracts/stencils/parent/parent-directive-parent.md:8` | The parent is reached by `SendMessage` |
| `contracts/stencils/seat/seat-directive-chair.md:9` | Advisors talk to the chair over Claude Code's session message |
| `contracts/stencils/seat/seat-directive-chair.md:19` | Ask an advisor with `SendMessage` to its strand name |
| `contracts/stencils/seat/seat-directive-advisor.md:12` | Message the chair only, with `SendMessage` |
| `contracts/stencils/loom/loom-template-discussion-advisor.md:42` | Answer the chair over the session message channel |
| `contracts/stencils/loom/loom-template-parent-review-delivery.md:4-5` | The delivery line names Claude Code's `SendMessage`; `comment` states the wording lives here only |
| `contracts/stencils/shed/shed-template-driver-notify.md:3` | One short `SendMessage` to the parent at every escalation |
| `contracts/stencils/shed/shed-template-driver-notify-watched.md:3` | Three cases, each one short `SendMessage` |
| `contracts/stencils/webster/webster-template-master.md:204` | Tell the parent by `SendMessage` that every batch is recorded |
| `contracts/stencils/orch/orch-template-role.md:233-235` | Every agent is reachable through `ListAgents` and `SendMessage`; never type into a pane |
| `contracts/stencils/orch/orch-template-role.md:237-238` | A hold notice is answered by `SendMessage` to the strand it names |

Go sites:

| Site | Mechanic |
|---|---|
| `internal/shuttleengine/hold.go:56` | Hold notice text: "To answer it, SendMessage to <strand>" |
| `internal/loomengine/parentreview.go:3` | Comment: the SendMessage wording lives in the delivery stencil only |
| `internal/seatengine/doc.go:3` | Doc: advisors talk to the chair over Claude Code's session message |
| `internal/shuttleengine/claudeengine/command.go:242-243` | The launch line names the session with `--name`, which is what makes a strand addressable |

## Block 8: `no-modal-question`

The ban on `AskUserQuestion`, and the generic rule not to wait on a dialog.

The Claude text, from `contracts/stencils/loom/loom-template-plan.md:290`:
"Never call the `AskUserQuestion` tool at any point in this session".

Codex needs: nothing for the tool, since the spike saw no counterpart to a modal question tool.
The nearest Codex dialog is the approval prompt, which `-a never` removes, and the trust and hook-review prompts, which the engine handles at startup.
The block can be empty on Codex, with the sentence "make best-judgment calls and never block on a dialog" kept as the neutral rule.

| Site | Mechanic |
|---|---|
| `contracts/stencils/loom/loom-template-discussion.md:128-130` | Never call `AskUserQuestion`, in either mode |
| `contracts/stencils/loom/loom-template-plan.md:288-291` | Never call `AskUserQuestion`; the session is autonomous |
| `contracts/stencils/darn/darn-template-write.md:14` | Never block on a dialog (neutral wording, kept) |

Go sites:

| Site | Mechanic |
|---|---|
| `internal/loomengine/prompt.go:19` | Autonomous mode rule names the `AskUserQuestion` tool |
| `internal/loomengine/prompt.go:26` | Interactive mode rule: the tool opens a modal dialog the resume mechanism cannot drive |
| `internal/shuttleengine/claudeengine/settings.go:268` | The `AskUserQuestion` deny, controlled by `deny_ask_user_question` |

## Block 9: `skill-load`

Where a stencil or the launch refers to a skill by name, and the typed message that loads it.

The Claude text, from `internal/shuttleengine/claudeengine/skillload.go:20-22`:
"Load these skills now, all in this one turn, with one Skill tool call each, in this order: ... Do nothing else, and reply only `ok` followed by the skill names in order."

Codex needs, from the spike: skills at `.agents/skills/<name>/SKILL.md`, invoked as `$name` in a prompt; the skill popup opens on `$` and takes the next Enter until Esc closes it; the skill list loads lazily, so the popup can say `no matches` for a skill that still runs.
Untested: `~/.agents/skills`, `/skills`, a skill name with a colon (`scribe:prose`, `ly:board`), and any Codex record that a skill loaded, which `ClassifySkillLoad` reads from the Claude transcript.
The plugin deploy that places skills for Claude would need a second target.

| Site | Mechanic |
|---|---|
| `contracts/stencils/orch/orch-template-role.md:3-4` | `comment`: asks no agent to load a skill; lyx loads the orch's skills after `/clear` |
| `contracts/stencils/orch/orch-template-role.md:153` | Board mechanics are in the `ly:board` skill |
| `contracts/stencils/orch/orch-template-reload.md:2` | `comment`: a compaction keeps the session's skills |
| `contracts/stencils/shed/shed-template-driver.md:2` | `comment`: replaces the skill the driver once loaded |
| `contracts/specs/shed-recipe-spec.md:64` | A seat carries an optional `skills` list |
| `contracts/specs/loom-status-spec.md:29` | An optional `ly-spawn` skill may wrap the verb later |

Go sites:

| Site | Mechanic |
|---|---|
| `internal/shuttleengine/claudeengine/skillload.go:17-52` | The typed load message and the transcript check of Skill tool results |
| `internal/shuttleengine/session.go:25-75` | `SkillLoader` capability required of an engine |
| `internal/shuttleengine/run.go:520-524,713-853` | Startup loads skills before the prompt; per-skill warnings |
| `internal/orchengine/watcher.go:1271-1307,1355-1447` | Reload step loads skills in one turn after a clear |

## Block 10: `session-lifecycle`

Slash commands and session identity the orch and creel type into a live session.

The Claude text, from `internal/shuttleengine/claudeengine/session.go:326-355`: `/clear`, `/compact` followed by a focus, `/reload-plugins`; and `/color <name>` from `color.go:32`.

Codex: untested for every command above, and no counterpart observed for `/color` and `/reload-plugins`.
The spike used `/quit`, `/fork`, `/model`, `/ps` and `/stop`; it did not exercise `/compact` or a clear.
Resume exists (`codex resume <session id>` printed on exit), untested.
Context cycling is the orch's loop, so this block gates whether the orch can run on Codex at all.

| Site | Mechanic |
|---|---|
| `contracts/stencils/orch/orch-template-compact.md:1-2` | `comment`: the focus text typed after `/compact`, one line |
| `contracts/stencils/orch/orch-template-role.md:3-4` | `comment`: after `/clear` lyx loads the skills; a compaction keeps them |
| `contracts/stencils/orch/orch-template-reload.md:1` | `comment`: the pointer typed after an auto-compaction and `/reload-plugins` |
| `contracts/stencils/orch/orch-template-adopt.md:1,4` | Adopt resumes an already-running Claude session |
| `contracts/stencils/shed/shed-template-driver.md:151` | A wake can be a context compaction |

Go sites:

| Site | Mechanic |
|---|---|
| `internal/shuttleengine/claudeengine/session.go:326-355` | Key sequences for `/clear`, `/compact`, `/reload-plugins` |
| `internal/shuttleengine/claudeengine/color.go:23-32` | `/color <name>` mapped from the palette |
| `internal/shuttleengine/claudeengine/sessionname.go:90-97` | `/rename` typed to name a session |
| `internal/shuttleengine/claudeengine/command.go:265-291` | Resume line with `--resume` and `--session-id` |
| `internal/orchengine/prompt.go:147-149` | Renders the one-line focus typed after `/compact` |
| `internal/creelengine/command.go:26,54` | The creel command set names clear, compact, reload-plugins, color and load-skills |

## Block 11: `edit-tool-names`

Instructions that name Claude's file tools.

The Claude text, from `contracts/stencils/edit/edit-directive.md:6`:
"Change files only with Edit or Write, never with a script that rewrites them".

Codex needs: the names of Codex's file-edit and read tools.
The spike recorded `custom_tool_call` entries in the rollout and `tool_name` `Bash` in hooks, but never named an edit tool, so the block's text is unknown until a rollout of an editing turn is read.
The rule's shape (an edit tool exists and a script rewrite is banned) carries over.
The sandbox rule `workspace-write` also makes `.git`, `.agents` and `.codex` read-only inside the workspace, which a stencil that edits under `.agents` would meet.

| Site | Mechanic |
|---|---|
| `contracts/stencils/edit/edit-directive.md:6-7` | Edit or Write only; one Edit per site, `replace_all` for a bulk change |
| `contracts/stencils/webster/webster-body-implementer.md:27` | Paths passed to the Read, Write and Edit tools stay absolute |
| `contracts/stencils/webster/webster-body-verify-fix.md:21` | Paths passed to the Read, Write and Edit tools stay absolute |
| `contracts/stencils/twill/twill-writer-fork.md:21` | Read each file in full with the Read tool |
| `contracts/stencils/webster/webster-template-master.md:245` | Master writes its two contract files with Write |

Go site:

| Site | Mechanic |
|---|---|
| `internal/shuttleengine/forkaudit.go:47` | The audit classifies Bash `tool_use` commands from a Claude transcript |

## Block 12: `foreground-gate-call`

The gate rule, the Bash tool and the standing denies.

The Claude text, from `contracts/stencils/webster/webster-body-implementer.md:51`:
"each as its own foreground Bash call under the gate rule in your system prompt."
The rule itself reaches the session as an `--append-system-prompt` notice built from the shell-guard rows (`internal/shuttleengine/claudeengine/command.go:254`, `settings.go:243`), including a Bash tool `timeout` of up to one hour from `BASH_MAX_TIMEOUT_MS`.

Codex needs: a place for the gate rule.
The spike found no system-prompt flag; `AGENTS.md` reached the model as a user-role message, which a fork spawned with `fork_turns` `all` also inherits, so the rule can ride there.
The deny hooks have a counterpart: `PreToolUse` with the `permissionDecision` `deny` shape blocks a `Bash` command and shows the reason.
Untested: a per-call timeout on the Codex shell tool, and whether the one-hour ceiling is configurable.
The orch's remark that "only a fork inherits that prompt" (`orch-template-role.md:119`) holds on Codex only for an `AGENTS.md`-borne rule.

| Site | Mechanic |
|---|---|
| `contracts/stencils/webster/webster-body-implementer.md:26` | Run every Bash command from the worktree root; paths worktree-relative |
| `contracts/stencils/webster/webster-body-implementer.md:28` | Gate steps run as foreground Bash calls under the gate rule |
| `contracts/stencils/webster/webster-body-implementer.md:51,68,90` | Each card gate step, `card-verify` and the batch gate as foreground Bash calls under the gate rule in the system prompt |
| `contracts/stencils/webster/webster-body-verify-fix.md:20` | Bash from the worktree root; follow the gate rule in the system prompt for `cd` |
| `contracts/stencils/webster/webster-body-verify-fix.md:35` | Run the fix gate as a foreground Bash call under the gate rule in the system prompt |
| `contracts/stencils/webster/webster-template-master.md:9` | `lyx` is not a listed tool; it is a CLI on PATH |
| `contracts/stencils/orch/orch-template-role.md:119` | A non-fork agent's brief carries the gate rule from the orch's system prompt, timeout included |
| `contracts/stencils/parent/parent-directive-parent.md:13` | Obey every limit and deny the instructions and settings set |
| `contracts/specs/loom-plan-spec.md:533,536` | An agent's settings deny a raw test command; run the gate in the foreground under the rule in the agent's system prompt |

Go sites:

| Site | Mechanic |
|---|---|
| `internal/planparser/verifymodule.go:65,77` | Refusal text: an agent's settings deny the command; run the gate in the foreground under the gate rule |
| `internal/shuttleengine/claudeengine/command.go:254` | `--append-system-prompt` carries the deny notice |
| `internal/shuttleengine/claudeengine/settings.go:243-268` | Builds the deny notice from the same list as the hooks |
| `internal/shellguard/rows.go:288` | A row refuses before Claude Code's statically-unresolvable-target dialog |
| `internal/shuttleengine/claudeengine/doc.go` (Bash timeout paragraph) | `BASH_MAX_TIMEOUT_MS` and `CLAUDE_ENV_FILE` set the Bash tool's ceiling and default stdin |

## Block 13: `project-doc-file`

The repository instruction file an agent is told to read.

The Claude text, from `contracts/stencils/darn/darn-template-write.md:26`:
"Also read the repository's `CLAUDE.md`."

Codex needs, from the spike: `AGENTS.md`, loaded at session start.
`CLAUDE.md` loads only through `project_doc_fallback_filenames` and only where no `AGENTS.md` exists in the directory.
This repository holds `CLAUDE.md` and no `AGENTS.md`, so the fallback setting from the spike's launch line would work here, and a hub that adds an `AGENTS.md` loses `CLAUDE.md`.

| Site | Mechanic |
|---|---|
| `contracts/stencils/darn/darn-template-write.md:26` | Read the repository's `CLAUDE.md` |
| `contracts/stencils/darn/darn-template-write.md:58` | Update the docs the repository's `CLAUDE.md` names |

## Specs outside the blocks

These sites are config and contract text, not prompt mechanics, and need an edit when a second engine ships.

| Site | What Codex needs |
|---|---|
| `contracts/specs/llm-model-spec.md:18-67` | The aliases `sonnet`, `opus`, `haiku`, `fable` map to `engine: claude`; Codex needs aliases that map to slugs such as `gpt-6.1-sol`, `gpt-6-luna`, `gpt-6-astra`, and the alias table must track the generation the slug carries |
| `contracts/specs/llm-model-spec.md:56,61,67` | "The Claude CLI resolves `--model sonnet` to the newest Sonnet"; the Codex launch takes `-m <slug>` with no newest-alias resolution observed |
| `contracts/specs/llm-model-spec.md:102,106-114` | Effort and context rules are Claude's; Codex efforts are `low` to `max`, plus `ultra` on every model but Luna and `gpt-reserve` |
| `contracts/specs/webster-spec.md:1,7,10,49,62` | The contract is described as fork-based, with the fixer "a fork in Merriam's session" |
| `contracts/specs/twill-lens-report-spec.md:7-8,27,29` | Lens report keyed to a fork's final message |

## Not Claude-specific

The remaining stencils and specs name tasks, formats and lyx verbs, and carry no mechanic above.
The `lyx` verbs, `PATTERN.md`, the plan, discussion, summary and status formats, and the tmux and creel text work the same on any engine.
The word "fork" in `friction-directive-*`, `landing-template-describe.md:44`, `loom-template-arbiter.md:5,29` and the bouncer lists is a lyx role name and holds as long as a fork exists.

## Size estimate

| Measure | Count |
|---|---|
| Proposed include blocks | 13 |
| Listed sites in stencils (`contracts/stencils/`), comments included | 127 |
| Of those, `comment` sites needing a docs edit only | 19 |
| Stencil files touched | 45 of 83 |
| Spec sites | 8 |
| Spec files touched | 6 of 11 |
| Go sites (prompt strings, docs and adapter code that carry a mechanic) | 35 |

A site that depends on two mechanics is listed once per block, so the totals count rows, not distinct lines.

Where the work sits, by risk:

- Low: blocks 8, 11 and 13 are one sentence each and need only the engine's tool and file names.
- Medium: blocks 1 to 5 and 9 need the `spawn_agent` text, `fork_turns` set to `all`, a defined reading of a missing fork report, and a rollout reader in place of the Claude transcript audits.
- High: blocks 6, 7 and 10 depend on mechanics the spike did not find or did not test (a completion notification that starts a turn, a named agent message, `/compact` and clear), and the webster Master, the shed driver and the orch role cannot run on Codex until each has a tested answer.

The block mechanism itself exists: `stencil.FillWith` renders `{{template "name"}}` from a told include set, and `seatengine` already pulls `seat-directive-chair` and `seat-directive-advisor` this way.
What is missing is an engine-keyed choice of include set at render time, which no renderer outside `seatengine` and `twilldirective` has today.
