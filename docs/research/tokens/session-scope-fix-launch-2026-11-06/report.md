# The session-scope fix in a real launch (2026-11-06)

Claude Code 2.1.319, a `lym` built from `2c4864bb`, which holds `c6b5ca66`.
The fix holds: the `Workflow` tool is gone from both sessions, the preloaded skills are in the spawned role, and the spawned role's first request falls from the trial's 37,330 tokens to 29,087.
The orch's first request is 24,388 against the trial's 34,383, but the two do not compare: this orch is a bare `shuttle open` session, and the trial's was the hub's orch with its role file.

| Session | Scope | First request | After the skill preload |
|---|---|---:|---:|
| Spawned role (`opus[high]`, preloads `scribe:prose` and `scribe:code-quality`), this report | on, with the fix | 29,087 | 41,368 |
| Spawned role, launch trial | on | 37,330 | 49,611 |
| Spawned role, launch trial | off | 38,463 | 53,313 |
| Orch, this report (`shuttle open --role orch`, no skills) | on, with the fix | 24,388 | |
| Orch, launch trial (the hub's orch, first request of a fresh session after a cycle) | on | 34,383 | |
| Orch, launch trial | off | 36,269 | |

Against the trial, the spawned role's first request is 8,243 tokens lower than the scoped launch and 9,376 lower than the unscoped one.
The same 8,243 holds after the preload, whose two skill bodies add 12,281 tokens in this launch and the trial alike.
The scope now cuts the spawned role's first request by 24 percent against the trial's unscoped launch, where the trial's cut was 3 percent.

## Why the orch does not compare

The trial's orch came from `lym beam start`, which loads the orch role file; this launch is `lym shuttle open --role orch` and carries only the session's own instructions.
The orch's first request here also holds one creel message, 45 tokens, because a session opened with no skill takes no turn until it gets input, and the message was that input.
No unscoped bare orch was launched, so this report has no scope-off row for it.
What the orch row does show is the fix's effect on its parts: no `Workflow` tool, and the tool schemas below.

## What was seen

| Question | Spawned role | Orch |
|---|---|---|
| `Workflow` in the session's tool list | gone | gone |
| `cc: tool schemas` | 64,249 characters, 21,947 tokens | 28,787 characters, 9,927 tokens |
| Plugin skills listed | all fourteen `scribe` skills; `ly` not listed | none |
| Other skills listed | `ladder-run` and `plugin-authoring`; built-in listing 118 tokens | the same two; built-in listing 119 tokens |
| Auto-memory section | none | 723 tokens |
| `ListAgents` tool | absent | present |

The tool list is the `tools` array of the transcript's request-shape record.
The role's tools are `Agent`, `Artifact`, `Bash`, `Edit`, `Read`, `ReportFindings`, `ScheduleWakeup`, `SendFeedback`, `Skill`, `ToolSearch`, `Write` and `advisor`.
The orch's add `ListAgents` and the seven `claude-artifacts` MCP tools; neither session names `Workflow` anywhere in its transcript.

**Workflow.**
The trial's scoped role carried tool schemas of 86,589 characters against 70,815 unscoped, the `Workflow` tool being 22,339 characters of the scoped total.
This role's tool schemas are 64,249 characters, which is 22,340 less than the trial's scoped figure.
The fix removes the tool and the whole of its extra schema.

**Preloaded skills.**
The role's second request holds the bodies of `scribe:code-quality` (2,588 tokens) and `scribe:prose` (1,766 tokens), loaded by the `Skill` calls of the open's first turn.
The orch preloads none, and none is expected.

**Plugin skills.**
The role lists all fourteen `scribe` skills, though it preloads two: a plugin that is on lists all its skills, as `c6b5ca66` now states.
The `ly` plugin is on in the hub's project settings and has no skill in either session, since neither preloads one of its skills and the scope turns the plugin off.
The orch lists no plugin skill for the same reason.

## Launches

Two, in the sandbox hub `lyt`, each by `lym shuttle open`; no headless launch and no other:

- `lym shuttle open --role darn-measure --model 'opus[high]' --skill scribe:prose --skill scribe:code-quality`, session `0db4a1a8-3202-468c-aaf2-cb159c437fca`.
- `lym shuttle open --role orch --model 'opus[high]'`, session `cb1ebfb2-1d87-4767-91e2-31b37f9e0da1`, named `lyt:orch-2` by reed because the hub already held an orch.
  It was given `lym creel post lyt:orch-2 "Reply with the single word ok."` to take its first turn.

The hub's `session_scope` was as shipped; no local override was set.
Measured with [`measure.sh`](../startup-context-2026-11-05/measure.sh) on each transcript, from line 1, requests 1 and 2.
Both windows are left open in `lyt`.

## Not covered

- A scope-off bare orch, which would make the orch row a like-for-like saving.
- The hub orch through `lym beam start`, the trial's own route, which `shuttle open` does not reproduce.
- Whether the saving holds on a darn writer that preloads five skills, as the startup-context report's did.
