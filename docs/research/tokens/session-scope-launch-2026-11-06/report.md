# Session-scope settings in a real launch (2026-11-06)

Claude Code 2.1.319, newer than the 2.1.318 whose binary named the settings.
Four of the six levers work, `skillOverrides` does not hide a plugin's skills, and the `enabledPlugins` "on" case was not isolated.
The scope cuts a spawned role's start load by 3 to 7 percent and the orch's by 5 percent, far less than the 31 percent the startup-context report projected,
because the scoped sessions load a `Workflow` tool schema about 5,800 tokens larger than the unscoped ones.

| Session | Scope | First request | After the skill preload |
|---|---|---:|---:|
| Spawned role (`opus[high]`, preloads `scribe:prose` and `scribe:code-quality`) | on | 37,330 | 49,611 |
| Spawned role, same launch | off | 38,463 | 53,313 |
| Orch, first request of a fresh session after a cycle | on | 34,383 | |
| Orch, same | off | 36,269 | |

The startup-context report's 34,252 and 45,865 do not compare with these:
its darn writer preloaded five skills and ran on 2.1.318,
and its orch figure was the first request after a compaction, with a summary and re-read files in the context.
Compare only the rows above with each other.

## Which settings work

| Setting | Result | Evidence |
|---|---|---|
| `enabledPlugins` `prowler@scribe` off | works | prowler is in neither scoped listing; the user's settings turn it on |
| `enabledPlugins` `ly@loomyard` off | works over the project layer | the role's listing has no `ly` skills; the hub's project settings turn `ly` on |
| `enabledPlugins` `ly@loomyard` on | not isolated | the orch lists `ly`, but the hub's project settings already turn it on; the user's turn it off |
| `skillOverrides` for a claude.ai skill (`anthropic-skills:docx`) | not isolated | the whole `anthropic-skills` listing, 3,252 tokens in the role, is gone, with `disableClaudeAiConnectors` on beside it |
| `skillOverrides` for a plugin skill (`scribe:python-testing`) | does not work | all fourteen `scribe` skills are listed in the scoped role, the twelve set `off` included |
| `disableClaudeAiConnectors` | works for claude.ai connectors | Claude Docs, Gmail, Drive and Calendar are gone; the `claude-artifacts` MCP server stays, with 32 deferred tool names in the orch and its schemas loaded at the second request |
| `disableBundledSkills` | works | the built-in listing falls from 2,761 to 120 tokens in the role; `ladder-run` and `plugin-authoring` stay and are not bundled |
| `autoMemoryEnabled: false` | works | the role has no auto-memory section, 718 tokens unscoped; the orch keeps its 734 |
| `permissions.deny: ["ListAgents"]` | works | the scoped role's tool schemas lack `ListAgents`, 1,180 characters, which the unscoped role has |

## What the scope costs

The scoped role's tool schemas total 86,589 characters against 70,815 unscoped.
`Workflow` accounts for the difference and more: 22,339 characters scoped, 5,384 unscoped, the extra being its workflow authoring reference.
The scoped orch shows the same, tool schemas 17,989 tokens against 12,111.
The cause is not established; the unscoped sessions carry the `claude.ai` connectors as deferred tools, and the larger schema appears only without them.

At the first request the role saves about 7,500 tokens in listings, memory and `ListAgents`, and gives about 5,800 back to `Workflow`.

## Launches

Capped at two per kind, all in the sandbox hub `lyt`:

- Role, twice: `lym shuttle open --role darn-trial --model 'opus[high]' --skill scribe:prose --skill scribe:code-quality`, once with the hub's `session_scope` as shipped and once with a local override `session_scope.enabled=false`, removed afterwards.
- Orch, twice: the hub's orch strand predated the scope, so `lym orch refresh` gave the unscoped row.
  `lym orch stop`, `lym beam start` and `lym orch refresh` gave the scoped row; the start alone resumed the old session, which is no fresh start.

Measured with [`measure.sh`](../startup-context-2026-11-05/measure.sh) on each session's transcript, from line 1, request 1 and request 2.
The two role sessions and the orch's windows are left open in `lyt`.

## Follow-ups

Filed as bug notes: `plugin-skilloverrides-ignored` and `scope-enlarges-workflow-schema`.
Not yet tried: a `lyt` launch whose project settings leave `ly@loomyard` unset, to isolate the flag layer's "on", and a launch that sets `anthropic-skills:docx` off without `disableClaudeAiConnectors`.
