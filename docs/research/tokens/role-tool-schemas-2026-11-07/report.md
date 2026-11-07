# Tool schemas in a spawned role's start (2026-11-07)

Claude Code 2.1.319, the production `lym`.
No code or config of the repository changed; the operator decides which tools to drop from the tables below.

- **The 22k of tool schemas is a sandbox figure.**
  The 29,087-token first request and its 21,947 tokens of tool schemas come from the sandbox hub `lyt`.
  A role of this repository's hub carries 5,382 tokens of tool schemas in a 13,449-token first request,
  because the tracked `.claude/settings.json` has denied `Artifact`, `ReportFindings`, `ScheduleWakeup`, `SendFeedback` and others since 2026-10-02.
  The four unused tools weigh 16,755 tokens in a role that is not denied them.
- **Why a role's schemas are "more than twice the orch's".**
  The 9,927 tokens counted for the orch leave out its seven `claude-artifacts` connector tools, 6,208 tokens that `measure.sh` lists separately.
  Counted alike, the sandbox role carries 21,948 tokens and the sandbox orch 16,135, a ratio of 1.36.
  The gap is `Artifact`, one built-in tool of 12,517 tokens in the role, against the orch's seven connector tools of 6,208 and its `ListAgents` of 407.
- **Both ways of dropping a tool work, and they are equal.**
  `--disallowedTools` on the launch line and `permissions.deny` in the settings lym passes each removed the `ScheduleWakeup` schema,
  1,626 and 1,622 tokens off the 29,087 baseline.
- **No tool can go from every role.**
  Four tools are never called by any role, and in a hub that does not deny them they are the whole saving.
  In this hub what is left is per role: `Agent`, `Edit`, `Bash` and `ToolSearch`, 510 to 3,735 tokens for the roles that never call them.

## Schema size per tool

Tokens split the reply's input tokens in proportion to characters, as `measure.sh` does; [`tool-tokens.sh`](tool-tokens.sh) prints them per tool.
Each column is request 1 of one session.
A dash is a tool the session does not carry.
`DeferredToolPlaceholder` and `advisor` are deferred tools, and the request holds only their stubs.

| Tool | Role in `lyt` | Orch in `lyt` | Role in the hub | Orch in the hub |
|---|---:|---:|---:|---:|
| `Agent` | 1,624 | 1,640 | 1,650 | 1,660 |
| `Artifact` | 12,517 | – | – | – |
| `Bash` | 1,104 | 1,115 | 1,230 | 1,073 |
| `Edit` | 340 | 343 | 345 | 347 |
| `ListAgents` | – | 407 | – | 412 |
| `Read` | 666 | 672 | 676 | 681 |
| `ReadNotifications` | – | – | – | 749 |
| `ReportFindings` | 754 | 761 | – | – |
| `ScheduleWakeup` | 1,592 | 1,607 | – | – |
| `SendFeedback` | 1,892 | 1,910 | – | – |
| `Skill` | 626 | 632 | 636 | 640 |
| `ToolSearch` | 502 | 507 | 510 | 513 |
| `Write` | 229 | 231 | 232 | 234 |
| `DeferredToolPlaceholder` | 70 | 71 | 71 | 72 |
| `advisor` | 31 | 32 | 32 | 32 |
| seven `mcp__claude-artifacts__*` tools | – | 6,208 | – | – |
| **Tool schemas** | **21,948** | **16,135** | **5,382** | **6,413** |
| First request, input tokens | 29,087 | 24,388 | 13,449 | 21,859 |

Sessions measured, all from the first request after line 1:

- Role in `lyt`: `0db4a1a8…`, `lym shuttle open --role darn-measure`, the session of the earlier launch report.
- Orch in `lyt`: `cb1ebfb2…`, `lym shuttle open --role orch`, the same report's.
- Role in the hub: `ba04f398…`, the darn writer that wrote this report, in a worktree of the loomyard hub.
- Orch in the hub: `715c9c34…`, the hub orch's first request of a session.
  Its 21,859 tokens hold the orch role file and its instructions, which the `lyt` orch lacks, so the two first-request rows do not compare.

The hub's `Bash` schema is 311 characters longer than the sandbox's, which is the one tool that differs between the role columns.

## Dropping a tool: launch line or settings

Two launches in `lyt`, each by `lym shuttle open --role darn-measure --model 'opus[high]' --skill scribe:prose --skill scribe:code-quality`,
the earlier launch report's command, which measured 29,087 tokens with none dropped.

| Launch | Way | Session | First request | Tool schemas | `ScheduleWakeup` in `tools` |
|---|---|---|---:|---:|---|
| Baseline, earlier report | none | `0db4a1a8…` | 29,087 | 21,947 | present |
| A | `--disallowedTools ScheduleWakeup` on the launch line | `34e0052d…`, `lyt:darn-measure-2` | 27,461 | 20,278 | gone |
| B | `ScheduleWakeup` added to `permissions.deny` of the `--settings` file | `be3d376c…`, `lyt:darn-measure-3` | 27,465 | 20,281 | gone |

Both ways take the schema out of the request, not only out of the tool's use: the `tools` array of the request-shape record lacks it.
They differ by 4 tokens.
The 1,626 and 1,622 tokens saved are what `ScheduleWakeup`'s schema weighs, 1,592 by characters.

lym has no setting for either way, so each launch went through a wrapper:
the hub's `claude.yaml` named a wrapper script as `binary`, and the script either appended the flag or added the deny to the settings file the launch line names, then ran the real binary.
The hub file is restored, and the wrapper is not kept.
Both windows are left open in `lyt`.

The settings way is in use already:

- lym's own settings for every session but the orch's carry `"permissions":{"deny":["ListAgents"]}`, from `session_scope`'s resolver, and a hub role lacks `ListAgents` while the hub orch carries it.
- The tracked `.claude/settings.json` of this repository denies fourteen tools and skills the same way, and a hub role carries none of the four unused tools.
- One list serves a fresh launch and a resume alike, since the launch line and the resume line both name the settings file.
  A launch-line flag would have to be added to each of the three line builders in `internal/shuttleengine/claudeengine/command.go`.

## Which tools each role called

[`calls-by-role.sh`](calls-by-role.sh) counts them over every session transcript on this machine that has an agent name, once per tool-use id.
A role is the last segment of the agent name, such as `darn` in `ly:role-tool-schemas:darn`.
The transcripts run from Claude Code 2.1.202 to 2.1.319, across the loomyard hub's worktrees, its sandbox hub and the C# and scribe hubs.
Subagent transcripts are not read; they hold the subagents' calls.

Across all named sessions, no session called `Artifact`, `ReportFindings`, `ScheduleWakeup`, `SendFeedback` or `advisor`, or any `claude-artifacts` tool.
`Artifact`, `ReportFindings` and `SendFeedback` were carried by 104 to 107 of them and `ScheduleWakeup` by 105, so the zero is a measured one, not an absence of the schema.
`advisor` is a deferred stub in every session and nothing loads it.
`Workflow`, carried by 95 earlier sessions, was never called either, and is gone since `c6b5ca66`.

The deferred tools are loaded through `ToolSearch`, which sessions asked for `SendMessage`, `Monitor`, `TaskStop`, `WebFetch` and `WebSearch`.
`ToolSearch` is the only way a session gets one of them.

## A table per role

One row is one role's table: the tools its sessions carry in the hub, the calls seen as `calls (sessions that called)`, which tools can go and what its start would lose.
Every row's schema tokens are the "Role in the hub" column above: `Agent` 1,650, `Bash` 1,230, `Edit` 345, `Read` 676, `Skill` 636, `ToolSearch` 510, `Write` 232.

A tool can go for a role when its sessions never called it, and the role has ten or more sessions.
A role with fewer is shown with its calls and no verdict.
A stencil can still ask for a tool the role has not called yet.
The stencils that name the `Agent` tool belong to the darn, burler, plan, discussion, webster, orch, raddle, twill and seat roles,
and none of those has `Agent` in its can-go column.

| Role | Sessions | `Agent` | `Bash` | `Edit` | `Read` | `Skill` | `ToolSearch` | `Write` | Can go | Start loses |
|---|---:|---:|---:|---:|---:|---:|---:|---:|---|---:|
| `darn` | 268 | 225 (19) | 9,261 (268) | 5,642 (230) | 3,648 (246) | 1,342 (268) | 21 (19) | 753 (246) | none | 0 |
| `code-bouncer-judge` | 264 | 0 | 1 (1) | 0 | 827 (264) | 269 (264) | 0 | 792 (264) | `Agent`, `Edit`, `ToolSearch` | 2,505 |
| `code-burler-review` | 264 | 703 (227) | 2,811 (264) | 59 (36) | 3,481 (264) | 1,333 (264) | 0 | 921 (256) | `ToolSearch` | 510 |
| `code-burler-fix` | 235 | 940 (235) | 3,021 (235) | 3,555 (229) | 3,337 (235) | 1,190 (235) | 0 | 1,098 (211) | `ToolSearch` | 510 |
| `describe` | 231 | 0 | 953 (231) | 41 (31) | 503 (231) | 232 (231) | 0 | 189 (189) | `Agent`, `ToolSearch` | 2,160 |
| `code-bouncer-seed` | 230 | 0 | 444 (230) | 2 (2) | 488 (230) | 232 (230) | 0 | 219 (219) | `Agent`, `ToolSearch` | 2,160 |
| `gate-fixer` | 132 | 0 | 708 (132) | 486 (122) | 754 (132) | 270 (132) | 0 | 134 (131) | `Agent`, `ToolSearch` | 2,160 |
| `board-tidy` | 57 | 0 | 471 (57) | 23 (10) | 72 (57) | 61 (57) | 0 | 96 (45) | `Agent`, `ToolSearch` | 2,160 |
| `plan-burler-review` | 55 | 87 (29) | 2,101 (55) | 70 (24) | 1,283 (55) | 166 (55) | 1 (1) | 163 (55) | none | 0 |
| `discussion-burler-review` | 51 | 178 (41) | 1,197 (51) | 27 (19) | 537 (45) | 153 (51) | 4 (4) | 235 (49) | none | 0 |
| `plan-burler-fix` | 50 | 200 (50) | 768 (50) | 830 (39) | 454 (44) | 150 (50) | 0 | 251 (49) | `ToolSearch` | 510 |
| `discussion-bouncer-judge` | 48 | 0 | 0 | 0 | 169 (48) | 49 (48) | 0 | 144 (48) | `Agent`, `Bash`, `Edit`, `ToolSearch` | 3,735 |
| `discussion-burler-fix` | 48 | 192 (48) | 551 (48) | 1,210 (43) | 275 (43) | 144 (48) | 2 (2) | 207 (48) | none | 0 |
| `fixer` | 48 | 0 | 668 (48) | 39 (23) | 225 (42) | 166 (48) | 0 | 49 (48) | `Agent`, `ToolSearch` | 2,160 |
| `plan-bouncer-judge` | 48 | 0 | 2 (1) | 0 | 169 (48) | 49 (48) | 0 | 141 (47) | `Agent`, `Edit`, `ToolSearch` | 2,505 |
| `tackler` | 32 | 0 | 79 (27) | 3 (2) | 188 (32) | 62 (31) | 0 | 36 (32) | `Agent`, `ToolSearch` | 2,160 |
| `friction` | 28 | 0 | 75 (28) | 0 | 45 (28) | 29 (28) | 0 | 28 (28) | `Agent`, `Edit`, `ToolSearch` | 2,505 |
| `plan-bouncer-seed` | 28 | 0 | 51 (28) | 0 | 73 (28) | 29 (28) | 0 | 26 (26) | `Agent`, `Edit`, `ToolSearch` | 2,505 |
| `conflict` | 26 | 0 | 142 (26) | 111 (26) | 98 (25) | 116 (26) | 0 | 26 (26) | `Agent`, `ToolSearch` | 2,160 |
| `discussion` | 24 | 172 (23) | 633 (24) | 90 (21) | 162 (24) | 48 (24) | 3 (2) | 235 (23) | none | 0 |
| `plan` | 24 | 178 (24) | 1,317 (24) | 633 (24) | 811 (24) | 48 (24) | 0 | 202 (24) | `ToolSearch` | 510 |
| `webster` | 24 | 122 (24) | 284 (24) | 1 (1) | 230 (24) | 121 (24) | 1 (1) | 71 (24) | none | 0 |
| `discussion-bouncer-seed` | 23 | 0 | 0 | 0 | 98 (23) | 24 (23) | 0 | 23 (23) | `Agent`, `Bash`, `Edit`, `ToolSearch` | 3,735 |
| `arbiter` | 19 | 0 | 131 (19) | 0 | 105 (16) | 24 (15) | 0 | 19 (19) | `Agent`, `Edit`, `ToolSearch` | 2,505 |
| `fix-reviewer` | 9 | 0 | 39 (9) | 0 | 36 (9) | 17 (9) | 0 | 9 (9) | too few sessions | |
| `drift-baseline` | 7 | 0 | 19 (6) | 0 | 21 (6) | 7 (7) | 0 | 6 (6) | too few sessions | |
| `pr-reviewer` | 7 | 0 | 25 (7) | 0 | 22 (7) | 24 (7) | 0 | 0 | too few sessions | |

`Bash`, `Read`, `Skill` and `Write` are called by almost every role, and `Skill` carries every role's skill preload, so none of the four is a candidate.
The `Edit` verdicts are the code and plan judges, the plan and discussion seeds, the friction writer and the arbiter, which write whole files and never edit one.

In a hub whose settings do not deny them, every role also drops `Artifact`, `ReportFindings`, `ScheduleWakeup` and `SendFeedback`, 16,755 tokens on top of the column above.

### The orch

The hub orch has nine sessions, which is too few for a verdict on a tool it never called.

| Tool | Schema tokens | Calls seen (sessions) | Can go |
|---|---:|---:|---|
| `Agent` | 1,660 | 37 (3) | no |
| `Bash` | 1,073 | 3,752 (7) | no |
| `Edit` | 347 | 246 (3) | no |
| `ListAgents` | 412 | 5 (1) | no |
| `Read` | 681 | 202 (9) | no |
| `ReadNotifications` | 749 | 0 | too few: one session carried it |
| `Skill` | 640 | 67 (9) | no |
| `ToolSearch` | 513 | 17 (2) | no: its `SendMessage` loads |
| `Write` | 234 | 358 (5) | no |

## Not covered

- Dropping `Agent` or `ToolSearch` was not launched.
  Dropping `Agent` may also take the 1,624-token `Available agent types` listing out of the request, and dropping `ToolSearch` the two deferred stubs; neither is tested.
- No role of a hub without a tool deny was measured by a real run, only by `lym shuttle open` in `lyt`.
- A role's calls are counted over sessions of every Claude Code version since 2.1.202, so a tool added late has fewer sessions that could have called it.
