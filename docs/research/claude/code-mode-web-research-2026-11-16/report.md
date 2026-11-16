# Code Mode in use: what the web says, and whether it changes the Plan writer result

2026-11-16, board task `code-mode-web-research`.
Read and fetch only: no bench ran, and no production code changed.
The bench this follows is [code-mode-bench-2026-11-16](../code-mode-bench-2026-11-16/report.md).

## Question

The bench found no gain from Claude Code's experimental Code Mode on the Plan writer,
and explained it by each `REPL` call needing a `Poll` call.
This study gathers what Anthropic and users say Code Mode is for and how to get gains from it,
judges each tip against lym's writers, and names how a later bench would test it.

## Sources

Code Mode shipped in Claude Code 2.1.323 on 2026-11-12, four days before this study,
so first-hand reports are few and most coverage restates the changelog.
"Read" is the date this study fetched the page.

| # | Source | Date | Kind |
|---|---|---|---|
| S1 | [Claude Code docs, "Run tool calls from code with Code Mode"](https://code.claude.com/docs/en/code-mode) | undated, read 2026-11-16 | Anthropic docs |
| S2 | [Claude Code changelog](https://code.claude.com/docs/en/changelog), entries 2.1.323 and 2.1.325 | 2026-11-12, 2026-11-13 | Anthropic changelog |
| S3 | The `REPL` and `Poll` tool prompts inside the installed Claude Code 2.1.325 binary, read with `strings` | read 2026-11-16 | Anthropic, shipped text |
| S4 | The bench's own writer transcripts, `cm-sb` session `5cd351ca` and `cm-lb` session `5c600806`, read with `jq` | 2026-11-16 | lym's runs |
| S5 | [wmedia.es, "Code Mode in Claude Code: turn it on and every request costs fewer tokens"](https://wmedia.es/en/tips/claude-code-code-mode-turn-on) | 2026-11-13 | hands-on, one run per arm |
| S6 | [i10x.ai, "Claude Code's Experimental Code Mode Batches Tool Calls in JavaScript"](https://i10x.ai/news/claude-code-code-mode-javascript-tool-calls), Christopher Ort | 2026-11-15 | news, cites S5 and X posts by @kairofeed |
| S7 | [Developers Digest, "Claude Code Code Mode: How to Turn It On and What It Changes"](https://www.developersdigest.tech/blog/claude-code-code-mode-2026) | 2026-11-13 | docs summary; ran no Code Mode session |
| S8 | [clauding.de](https://clauding.de/en/posts/claude-code-code-mode-2-1-323/), [Classmethod](https://dev.classmethod.jp/en/articles/20261113-cc-updates-v2-1-323/), [DEV, RAXXO Studios](https://dev.to/raxxostudios/claude-code-21323-adds-code-mode-for-javascript-tool-calls-3ibk) | 2026-11-13 | changelog coverage, no test |
| S9 | [Automater](https://automater.ai/news/claude-code-2-1-323-code-mode-org-off-switch-permission-fixes/), [Dark Factory Dev](https://darkfactory.dev/news/2026-11-12-evening/story-1), [Chasing Next](https://chasingnext.com/updates/code-mode-in-claude-code-experimental), [AI/TLDR](https://ai-tldr.dev/releases/anthropic-claude-code-2-1-323/) | 2026-11-12 | changelog coverage, no test |
| S10 | [Anthropic, "Code execution with MCP"](https://www.anthropic.com/engineering/code-execution-with-mcp), Adam Jones and Conor Kelly | 2025-11-04 | the idea behind Code Mode, on MCP |
| S11 | [Anthropic, "Introducing advanced tool use"](https://www.anthropic.com/engineering/advanced-tool-use) | 2025-11-24 | programmatic tool calling on the API |
| S12 | [Claude Platform docs, programmatic tool calling](https://platform.claude.com/docs/en/agents-and-tools/tool-use/programmatic-tool-calling) | undated, read 2026-11-16 | API docs |
| S13 | [Cloudflare, "Code Mode: the better way to use MCP"](https://blog.cloudflare.com/code-mode/), Kenton Varda and Sunil Pai | 2025-09-26 | the name's origin |
| S14 | [darjss/claude-code-codemode](https://github.com/darjss/claude-code-codemode) | undated, 2026 | third-party plugin, own sessions measured |
| S15 | [ejklock/claude-code-mode](https://github.com/ejklock/claude-code-mode) and [its PR #2](https://github.com/ejklock/claude-code-mode/pull/2) | bench 2026-10-06, PR 2026-10-07 | third-party plugin, 3 runs per arm |
| S16 | Medium: ["Claude Code's Code Mode Cut My Tokens 57%"](https://medium.com/everyday-ai/claude-codes-code-mode-cut-my-tokens-57-09e91232fe82), ["Code Mode on Claude Cuts Tokens Faster Than It Cuts Your Bill"](https://medium.com/@chewloongnian/code-mode-on-claude-cuts-tokens-faster-than-it-cuts-your-bill-2f2bf3ddeb84) | November 2026 | refused the fetch (HTTP 403); only the titles and search snippets were read |
| S17 | [r/ClaudeCode, "Claude Code 2.1.323: experimental Code Mode ..."](https://www.reddit.com/r/ClaudeCode/comments/1xvw576/claude_code_21323_experimental_code_mode_claude/), 28 comments | 2026-11-14 | users of the built-in mode |
| S18 | [r/ClaudeAI, "Code Mode in 2.1.325 just turned my 13 tool calls into 2 turns"](https://www.reddit.com/r/ClaudeAI/comments/1xwv9hc/code_mode_in_21325_just_turned_my_13_tool_calls/) | 2026-11-15 | title only; the body was removed |
| S19 | [r/ClaudeAI, "Claude Code v2.1.108's new hidden REPL tool is cool"](https://www.reddit.com/r/ClaudeAI/comments/1sm99pj/claude_code_v21108s_new_hidden_repl_tool_is_cool/), 12 comments | 2026-04-15 | the earlier hidden `REPL` tool |
| S20 | [r/mcp](https://www.reddit.com/r/mcp/comments/1smamaz/how_cloudflares_code_mode_pattern_eliminated_the/) (2026-04-15), [r/webflow](https://www.reddit.com/r/webflow/comments/1x68vay/code_mode_use_60_less_claude_code_tokens_using/) (2026-10-15), [r/ClaudeCode](https://www.reddit.com/r/ClaudeCode/comments/1oxbegs/codemode_save_60_in_tokens_by_executing_mcp_tools/) (2025-11-14), [r/LLMDevs](https://www.reddit.com/r/LLMDevs/comments/1taj28j/your_agent_doesnt_need_more_tools_it_needs_to/) (2026-05-11) | as listed | Code Mode over MCP servers, not the built-in mode |

S14 and S15 are plugins that imitate Code Mode with one MCP tool beside the normal tools, not the built-in mode.
They are here because they measured where batching in a script pays and where it costs.

## What Anthropic says Code Mode is for

- Claude writes one JavaScript program that makes several tool calls, and Claude Code runs it in the session (S1).
- The docs name three fits (S1):
  "A change that repeats across many files",
  "A search whose results need filtering before Claude reads them",
  and "Several independent commands that can run at the same time".
- The docs give no cost or token figure (S1, S7).
  The widely shared "37% fewer tokens" is from the 2025 post on programmatic tool calling, not from Code Mode (S6, S11).
- For programmatic tool calling, Anthropic lists where it helps (S11):
  large data where only aggregates are needed, three or more dependent calls,
  filtering results before the model sees them, and parallel work across many items.
  It lists where it does not: single calls, quick lookups with small results,
  and "tasks where Claude should see and reason about all intermediate results".
  On τ²-bench, strictly sequential tasks, the API docs report unchanged scores at about 8% higher cost (S12).
- The tool prompt the model receives says the same in its own words (S3):
  each `REPL` call "is a full model round-trip", the work inside a script "is comparatively free",
  so "gather what a decision could plausibly need in the script you are already writing,
  and come back for another call when the next step depends on what this one shows you".

## How `REPL` and `Poll` work, and what the bench writers did

The shipped tool prompt has several variants (S3).
In the one the bench writers met, a `REPL` call "only dispatches its script and returns `(dispatched #N)` at once",
and the output arrives through `Poll`.
`Poll` "Sent in the same turn as a `REPL` call it waits for that call's script:
a long one ... then costs no extra round trip".
"In a mode that asks for permission, every `REPL` call instead waits for its script and returns its output";
lym launches its sessions with `--dangerously-skip-permissions`, so its writers get the dispatching form.
In a subagent, `Poll` is refused:
"each `REPL` call you make here waits for its script to finish and returns the result" (S3).

The bench writers' transcripts show where the `Poll` calls went (S4):

| Writer | Turns | `REPL` and `Poll` in one turn | `Poll` alone | `REPL` alone | No tool |
|---|---|---|---|---|---|
| `cm-sb`, small | 21 | 15 | 0 | 3 | 3 |
| `cm-lb`, large | 46 | 36 | 5 | 3 | 2 |

Every `REPL` call returned `(dispatched #N)`, but the writers sent the `Poll` in the same turn,
so `Poll` added at most five turns of 46 on the large input and none on the small one.
The bench report's reason for the flat turn count, that "each REPL call needs a Poll call",
explains the doubled direct-call count and not the turns.

The turns stayed level because a writer's turn ends where its next step needs a judgment on what it just read.
Code Mode packed more calls into each turn, 3.7 against 1.9 on the large input and 3.3 against 1.0 on the small one,
but the number of judgments a plan takes did not change,
and the control already batched by parallel calls and shell pipelines.

Shell run inside a script through the `$` template is recorded as an inner `Bash` call:
`cm-lb`'s scripts call no `tools.Bash`, yet the transcript holds 104 inner `Bash` rows.
The docs say each call a script makes passes `deny` and `ask` rules and `PreToolUse` hooks (S1),
so lym's shell guard should see them,
but no bench writer tried a guarded command, so no refusal confirms it.

## What users report

| Source | What was measured | Result |
|---|---|---|
| S5 | One task, 40 JSON files, top three customers; Opus 5.5 medium, one run per arm | 4 requests in both arms; input tokens with cache 150,364 against 94,659; output level; time 15 s against 37 s |
| S5 | `/context` system-tool tokens | 24.6k without Code Mode, 11.1k with it, on every request |
| S6, citing @kairofeed on X | Eight Sonnet 5.5 runs | "about 40% fewer tokens" on short tasks; 486K to 218K on a 30-file refactor; not checked here |
| S16 | Six sessions; and a scripted replay | by their titles and snippets only: tokens down 57% and the bill up 10%; a scripted replay 76% fewer tokens |
| S14 | A research task; a small coding task | all calls forced through the script tool: 2.62M against 3.45M tokens, 21 against 33 calls; coding 0.69M and 0.66M against 0.80M and 0.89M |
| S15 | Four tasks, Opus, three runs per arm | read 5 files: turns 7 to 2, cost $0.039 to $0.023; write 3 files: cost level, cache reads up; one `git grep`: the model did not use the script tool |

- S5's saving "didn't come from fewer turns": the control also wrote one script, in Python through `Bash`.
  It came from the smaller fixed tool context Code Mode sends on every request.
- S14's author routes to the script tool only chains where one result decides the next call,
  and outputs too large to read whole.
  "Plain independent reads are already covered by Claude's own parallel calls",
  and on a single-file edit "the script adds overhead and saves nothing".
- S15 found the gain on batched reads, none on writes, and none on one command.
  Its PR notes that "Every turn rereads the whole context,
  so cache-read tokens show the cost of turns that output tokens hide."
- Reported costs: slower wall time (S5, and the bench's +14% and +2%),
  a script to review instead of a list of calls (S8, S9),
  in auto mode a classifier that reviews the script and "doesn't see values computed at run time" (S1),
  and a session that resumes only with the variable set (S1).

## Reddit

Reddit refused every request from this machine with HTTP 403,
so a Codex session fetched the threads from the Arctic Shift archive instead.
Its subreddit searches mostly timed out, so threads the searches missed are not here.

S17 is the only thread with users of the built-in mode:

- "we ran code mode on the same repo for a week and the win was real on read heavy tasks.
  write tasks were worse because one bad js line killed the whole batch of tool calls" (54 points).
  The same user: "the savings come from not resending tool schemas and results every turn.
  on long read tasks it won, short tasks it lost" (16 points).
- "the js wrapper cost ~200 tokens but saved ~800 per batch on long reads.
  we lost 10% of runs on write tasks due to syntax errors".
- "reads are where it's actually good ... I just leave edits as normal tool calls and let it do the read heavy stuff in code";
  this user runs omp's code mode, not the built-in one.
- Raw search results of 20.3 KB came down to 2.95 KB by keeping only the key fields in the script.
- One reply puts the gain in edits made inside a script:
  the files read never enter the context, only "a single 'yup, that worked'".
- One worry that Code Mode defeats hook guards; the docs answer that each call still passes them (S1).

S18's title claims 13 tool calls in 2 turns, with no body left to say on what task.
S19 shows the mode's predecessor, a hidden `REPL` tool behind `CLAUDE_CODE_REPL` in 2.1.108.
The S20 threads are about Code Mode over MCP servers.
Their gains are fewer schemas and intermediate results kept out of context,
for example 49,000 against 148,000 tokens on one Webflow question,
and their authors use it for read-only operations, since a script's mutations are hard to gate per tool.

## Each tip, judged for lym's writers

| Tip | From | Could lym's writers use it? | How a later bench would test it |
|---|---|---|---|
| Use Code Mode where the work is mechanical: a change repeated across files, independent commands, a search to filter | S1, S11, S14, S15 | Not Plan-Write, whose turns are judgments on what it reads. A darn writer on a rename or repeated edit, and a gate fixer that runs tests and acts on exit codes, fit the named cases | Replay a landed darn task whose diff is a repeated edit across packages, control against Code Mode, three samples per arm; compare turns, cache-read tokens, cost and wall time |
| Filter inside the script and return only what the next decision needs | S1, S10, S11, S13, S14 | Partly. lym's batching rules tell writers to read a file whole, which returns everything; a Plan writer must see what it reasons over. The bench's Code Mode writer already peaked at 270k context against 346k on the large input | A Plan-Write arm with Code Mode and an appended instruction to return matched lines and spans instead of whole files, against plain Code Mode; compare peak context, cache-read tokens and the keys |
| Count the smaller fixed tool context as the gain, not the turns | S5 | It comes with the mode at no prompt change; it grows with the number of turns, which may be the large input's 12% lower cost | In any later arm, read the first request's system-tool tokens, and split cost into cache reads, cache writes and output |
| Send `Poll` in the same turn as `REPL`, or end the turn instead of a lone `Poll` | S3 | Already done: 51 of 67 turns paired them (S4) | None needed |
| Few, full scripts | S3 | Already the tool prompt's own rule, and lym's batching rules say the same | None needed |
| Let lookups run in Code Mode | S3, S4 | A subagent's `REPL` waits and returns its output, and lym's lookup subagents inherit the mode; their work is the search-and-filter case. No setting found that turns Code Mode on for subagents alone | Only as part of a whole-session arm: compare the lookup subagents' cost and returned length between arms |
| Route only some calls to scripts: reads in code, edits as direct calls | S14, S15, S17, S20 | Not today. The bench writers had only `REPL` and `Poll`. S3 holds a variant where scripts are for investigating and "the other tools in your list are also callable", but no documented setting picks it. The only routing lym has is per role, which the first row tests | If a setting appears, a Plan-Write arm with read-only scripts against plain Code Mode |
| Expect a script error to sink a batch of writes | S17 | Applies to a darn writer more than a Plan writer; `cm-lb` had 2 of 41 scripts end in a `TypeError` | In the first row's bench, count scripts that end in an error and the turns spent recovering |
| Document tool return formats so scripts can parse them | S11, S12 | lym's verbs already print JSON envelopes a script can parse | None needed |
| Check that guards still fire inside scripts | S1, S8, S9 | Needed before any adoption: lym's shell guard is a `PreToolUse` hook | In one Code Mode session, run a guarded command through `$` and through `tools.Bash`, and confirm the refusal each time |

## Verdict

- Nothing found would turn the bench's result on the Plan writer into a gain.
  Anthropic's own guidance names the Plan writer's work, reasoning over each result,
  as the case programmatic tool calls do not help.
- The bench's explanation needs a correction:
  `Poll` cost almost no turns, since the writers sent it in the same turn as `REPL`.
  The turns stayed level because each turn ends at a judgment, and the control already batched.
- Users agree on where it pays: long, read-heavy work,
  with the gain in tokens kept out of the context rather than in turns.
  They report writes as no better or worse.
- Where Code Mode could pay for lym is a writer doing mechanical work, or a Plan writer told to filter in its scripts,
  both measured on cache-read tokens rather than turns.
  Neither is shown by any source here on a task like lym's; the hands-on reports are single runs or unmeasured.
- Any adoption first needs the guard check of the last row.

## Limits

- Most sources restate the one-line changelog entry;
  S5 is the only first-hand test of the built-in mode with numbers, one run per arm.
- S14 and S15 measure plugins, not the built-in mode.
- S16 refused the fetch, and its figures here are from search snippets.
- The Reddit text is the archive's copy, and its scores and comment counts are the archive's snapshots.
  The S17 claims are unverified user reports.
  Two Codex sessions ran of the three allowed: the first met Reddit's block, the second used the archive.
- S3 is text shipped in one binary version;
  its variants may change, and which variant a session gets is not stated anywhere.
- The τ²-bench figure and the later part of S12 came from the fetch tool's summary of a long page,
  not the page's own words.
