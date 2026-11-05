# What a lym session loads at start, by source (2026-11-05)

A session's start load is the context of the request that carries its first real work:
for a spawned role, the turn whose prompt points at the role's `prompt.md`;
for the hub orch, the first request after a context cycle.
Three sessions were measured, each once, from their recorded transcripts:

| Session | Model | Transcript, request | Input tokens |
|---|---|---|---|
| Hub orch, first request after the compaction at 16:56 | Opus 5.5 | `715c9c34…`, from line 56770, request 1 | 45,865 |
| Darn writer of `startup-token-trim` | Opus 5.5 | `61754351…`, request 3 | 34,252 |
| Burler reviewer of `anchor-scratch-excluded` | Sonnet 5.5 | `f94b00b3…`, request 3 | 34,252 |

Claude Code 2.1.318 on all three.
Thirty spawned sessions of the same day across three worktrees (cold gate fixers, Bouncer, Burler review and fix, Webster, the arbiter, the change description, darn) open their first turn at 21,224 to 21,745 tokens,
so the two spawned roles measured stand for every role up to its role prompt.

## Method

Claude Code records each API request in the transcript:
an `api-request-shape` record holds the system prompt and tool schemas,
and each `api-request` names the message blobs it keeps and adds.
[`measure.sh`](measure.sh) rebuilds one request from those records,
and [`classify.jq`](classify.jq) sorts every line into the source that put it there.
Tokens per source split the input tokens of the reply, the request's measured total, in proportion to characters:
2.8 characters per token for the spawned roles and 3.1 for the orch, whose context holds more prose.
A source's token figure is therefore an estimate within about a tenth;
the totals are measured.
The raw tables are [`orch.tsv`](orch.tsv), [`darn-writer.tsv`](darn-writer.tsv) and [`burler-review.tsv`](burler-review.tsv).

```bash
docs/research/tokens/startup-context-2026-11-05/measure.sh ~/.claude/projects/<project>/<session>.jsonl <from-line> <request>
```

Passing a source name as a fourth argument prints that source's lines, to check the sorting.

## Spawned role: darn writer and Burler reviewer

The two tables agree within a few dozen tokens per row, so one table serves both.

| Source | Tokens | Needed by the role |
|---|---:|---|
| Claude Code tool schemas (Agent, Bash, Edit, ListAgents, Read, Skill, ToolSearch, Write, advisor) | 5,850 | mostly; ListAgents (~420) no |
| Claude Code system prompt sections | 2,120 | yes |
| Claude Code agent type list | 1,670 | partly: `general-purpose` yes, `claude-code-guide`, `statusline-setup`, `web-fetch` no |
| Claude Code environment block, git status and email, date | 1,350 | environment yes, git status marginal |
| Claude Code deferred tool names, bypass-mode note | 410 | no |
| claude.ai connectors: Claude Docs tool schemas, MCP instructions, Gmail, Drive, Calendar and Docs deferred names | 2,125 | no |
| Skill listing: `anthropic-skills` plugin (docs, pptx, xlsx, docx, pdf, browser, computer use and more) | 3,400 | no |
| Skill listing: bundled skills (dataviz, code-review, claude-api, artifact skills, update-config, loop, schedule and more) | 2,880 | no |
| Skill listing: scribe | 555 | Go and language-agnostic lines yes; Python and C# lines (~210) no |
| Skill listing: ly | 330 | no for a writer or reviewer; the board skills serve the orch |
| Skill listing: prowler | 145 | no |
| Preloaded skill bodies: code-quality, golang-comments, prose, testing, golang-testing | 9,500 | yes, by the role's stencil and `language.yaml` |
| Auto-memory: system prompt section and the `MEMORY.md` index | 1,380 | no: the index holds the operator's notes to the orch |
| User's global `~/.claude/CLAUDE.md` | 145 | yes |
| lym: `CLAUDE.local.md` from heddle | 930 | yes |
| lym: append-system-prompt, deny notices and creel trust | 890 | yes |
| lym: skill preload request, role prompt pointer, pane color | 205 | yes |
| scribe SessionStart hook text | 90 | no: it asks for `scribe:conversation`, which the preload request forbids |
| **Total** | **34,252** | about 11,000 not needed |

The role's own `prompt.md` comes on top, read in the next turn:
the darn writer's is 20,176 characters, and the request after reading it measures 42,023 tokens, 7,771 more.

## Hub orch, first request after a compaction

| Source | Tokens | Needed by the orch |
|---|---:|---|
| Claude Code tool schemas (adds Monitor, SendMessage, ReadNotifications, TaskStop, WebSearch) | 11,080 | yes |
| Claude Code system prompt sections, agent types, environment, git status, deferred names | 4,570 | yes, git status marginal |
| Compaction summary | 5,080 | yes |
| Files Claude Code re-reads after a compaction (the handoff note, two scratch files, a memory file, another hub's `.git/info/exclude`) | 3,805 | the handoff note (~1,700) yes, the rest no |
| Re-injected skill bodies: `ly:board`, `ly:board-policy`, `scribe:prose`, `scribe:conversation`, `ly:distill` | 8,420 | board and board-policy yes; prose and conversation govern its replies; distill no |
| claude.ai connectors | 1,925 | no |
| Skill listing: `anthropic-skills` | 3,080 | no |
| Skill listing: bundled skills | 2,610 | no |
| Skill listing: scribe, ly, prowler | 930 | ly and scribe yes, prowler no |
| Auto-memory: section and `MEMORY.md` index | 1,250 | yes |
| User's global `~/.claude/CLAUDE.md` | 130 | yes |
| lym: `CLAUDE.local.md` from heddle | 840 | yes |
| lym: append-system-prompt, deny notices, creel trust and the hub's orch rules | 1,050 | yes |
| SessionStart hooks: scribe's skill request and lym's resume pointer | 175 | yes |
| Operator's message, reminder wrappers and the rest | 915 | yes |
| **Total** | **45,865** | about 10,000 not needed, 7,900 of it removable by settings |

The resume pointer tells the orch to read `.lym/orch/role.md` and the cycle's note.
In this cycle the orch went straight to the operator's question and did not read the role.
An earlier cycle in the same transcript did: the read took the context from 46,125 to 65,515 tokens,
so the role, 54,443 characters, costs 19,390 tokens each time it is read.
That makes the role file the largest single source in an orch start, larger than everything Claude Code itself loads.

## Sources to leave out

Every lever below goes in the settings file lym already passes with `--settings` to each run, or on the launch line `buildLaunchCmd` composes,
so lym can set each per role at launch.
"Named in the binary" means the setting or variable appears in the Claude Code 2.1.318 binary with that meaning in its own text;
none of them was tried in a launch here.

| Source | Tokens per start | Way to leave it out | Per role at launch |
|---|---:|---|---|
| claude.ai connectors | 1,900–2,100 | `"disableClaudeAiConnectors": true` in the run's settings, or `ENABLE_CLAUDEAI_MCP_SERVERS=false`; named in the binary | yes, for every role and the orch |
| `anthropic-skills` listing | 3,100–3,400 | `skillOverrides` setting `off` per skill; whether disabling connectors also drops them is not known | yes |
| Bundled skills listing | 2,600–2,900 | `"disableBundledSkills": true`, or `CLAUDE_CODE_DISABLE_BUNDLED_SKILLS=1`; named in the binary | yes; the orch may keep `update-config` through `skillOverrides` instead |
| scribe Python and C# skills, prowler | ~350 | `skillOverrides` for the scribe skills; `"enabledPlugins": {"prowler@scribe": false}` | yes, when the flag settings layer overrides the user's `enabledPlugins`, not tried |
| ly skills in a spawned role | ~330 | `skillOverrides` for `ly:*` in run settings | yes |
| Auto-memory in spawned roles | ~1,380 | `"autoMemoryEnabled": false`, or `CLAUDE_CODE_DISABLE_AUTO_MEMORY=1`; named in the binary | yes; the orch keeps it |
| ListAgents tool | ~420 | `--disallowedTools ListAgents` | yes |
| Unused agent types | part of ~1,670 | no setting found that hides a built-in agent type | no |
| Git status block | ~500 | `"includeGitInstructions": false`, or `CLAUDE_CODE_DISABLE_GIT_INSTRUCTIONS=1`; named in the binary, but whether it drops the status snapshot or only the git guidance is not known | yes |
| scribe SessionStart hook in spawned roles | ~90, and a contradicting instruction | no per-plugin hook switch; `disableAllHooks` would also drop lym's own hooks | no |
| Orch role file | 19,390 per read | shorten `contracts/orchrole`, or split the parts the orch reads only for a rare verb into a skill or a file it reads on demand | lym's own text |
| Files re-read after compaction | ~2,100 of 3,805 | none found; Claude Code picks the recently read files | no |

Leaving out the connectors, both skill listings, the unneeded plugin skills and auto-memory
would take a spawned role from 34,252 to about 23,800 tokens at its prompt turn, 31 percent less,
and the orch from 45,865 to about 37,900 per cycle start.

## Why the start load costs more than its size suggests

Each spawned session writes most of its start load to the prompt cache instead of reading it:
a first request reads at most about 9,300 tokens from cache, the system prompt and tool schemas Claude Code caches across sessions,
and writes the other 12,000 or so; with no warm prefix, as in the darn writer measured here, it reads nothing and writes all 21,500.
The skill listings, connectors and instruction files ride the first user message,
which differs per session through the git status and the environment block,
so no session reuses another's cache for them.
Every token cut from that message is a cache write saved in every spawned session.

## Not covered

- The orch's start after `/clear` was not measured: its transcript holds two clears, both on 2026-10-25 under an older Claude Code, and every cycle since is a compaction.
  It loads the same Claude Code and connector sources without the summary and the re-read files, then the skill step, the role and the note.
- The role prompts of the other roles were not sized; only the darn writer's was.
- No lever above was tried in a launch.
