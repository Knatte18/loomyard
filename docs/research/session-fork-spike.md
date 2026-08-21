# Session-fork cluster review — spike findings

**Task:** `session-fork-diversity-spike` (retry of `cluster-fork-spike`).
**Date:** 2026-07-16.
**Verdict up front: the fork-based cluster design is viable and profitable.**
M1 and M2 pass cleanly, fork-cluster coverage is on par with (or better than) independent cold reviewers,
and a forked reviewer costs less than half of a cold one.
One design surprise: hard "ignore everything else" lenses were *not* load-bearing — identical-prompt forks diverged strongly on their own and out-covered the lens arm.

> **See the 2026-08-20 update below** for a re-verification on CLI 2.1.228: the CLI `--resume --fork-session` cache-miss bug (root cause (b) below) is confirmed still present and unfixed by the newer `--exclude-dynamic-system-prompt-sections` flag, nested forking is now blocked at the instruction level, and a newer built-in tmux visualization feature (Agent Teams) turns out not to help either.

Rig and raw data: the throwaway harness (`tools/fork-poc/` — prompts, spawn/wait/ harvest helpers) and every raw session output (`tools/fork-poc/results/`) were committed incrementally on the task branch and removed before merge;
all `tools/ fork-poc/...` paths referenced below resolve in the task branch's history (archive tag `session-fork-diversity-spike`), not on main.
All live sessions ran as `lyx reed` strands in the sandbox hub (`~/Code/lyx-test-HUB/lyx-test`), watched live by the operator via `lyx reed attach`.

## Step 0 — transcript persistence (attempt 1's blocker)

**Confirmed live,
and the fullscreen hypothesis is refuted — but this was not a spike discovery.**
The root cause and fix were already established a week before attempt 1 was abandoned: a claude launched from inside a Claude Code session inherits `CLAUDECODE`/`CLAUDE_CODE_*`, treats itself as a nested child, and silently doesn't persist its transcript.
Documented in `docs/research/reed-exploration.md` (which names `CLAUDE_CODE_CHILD_SESSION=1` as the prime culprit), mandated by `docs/research/reed-proposal.md` ("Env hygiene is mandatory"), implemented as `reedengine.CleanClaudeEnv` (landed 2026-07-05), and proven end-to-end by `internal/reedcli/smoke_resume_test.go:TestSmokeClaudeResumeRecallsCodeword`.

Step 0's actual contribution: a plain interactive `claude` spawned as a reed strand persisted its `.jsonl` within ~10 s — with `"tui": "fullscreen"` still set in `~/.claude/settings.json` — killing the settings.json hypothesis attempt 1's writeup carried as "leading".
**Process lesson (sharper than "commit as you go"): attempt 1 hit an already-documented failure and hypothesized a novel cause instead of checking the repo's own `docs/research/` — read the existing research before diagnosing.**

## Method

- **Explorer:** one session reads all of `internal/modelspec/` (11 files, ~1.2k lines) plus a nonce;
  its session id is preassigned via `claude --session-id <uuid>` so the transcript path is deterministic.
- **Forks:** `claude "<prompt>" --resume <explorer-id> --fork-session --session-id <uuid>` in a fresh reed strand.
  Prompt must be the FIRST positional arg — variadic `--add-dir <dirs...>` swallows a trailing prompt (cost one debugging round).
- **Measurement:** forked transcripts copy the parent's full history, so all usage and tool-call accounting splits at the fork point (first user message matching the fork's prompt). `usage` blocks in the `.jsonl` carry full token columns per assistant message;
  `compute = input + cache_creation + output` is the headline (tokens processed fresh).
- **Done-signal:** transcript-quiet alone false-fires twice over (launch gap before the first flush;
  long thinking pauses mid-turn).
  The reliable signal is transcript quiet AND the pane no longer showing claude's "esc to interrupt" spinner.
  Claude Code Stop hooks are the robust long-term mechanism (per the CLAUDE.md agent-execution design);
  the pane-grep was sufficient for the spike.
- All sessions ran `claude-sonnet-5` (operator default), except M2's fork (opus).

## M1 — does a fork inherit explored context? PASS

Fork probed with tools forbidden: nonce, `Parse` signature, `Spec` fields, `LoadRegistry` missing-file behaviour. **4/4 correct, 0 post-fork tool calls.**
Post-fork cost: 38k compute (mostly cache re-creation of the inherited context) vs the explorer's 157k exploration — the fork skipped the entire explore phase.

## M2 — fork + model switch in one invocation? PASS

Same probe with `--model opus`: ran as `claude-opus-4-8` (parent was sonnet-5), **4/4 correct, 0 tool calls**.
The model-per-fork axis is real;
this doubles as the minimal B3 signal.

## Q2 — diversity (union coverage)

Three arms, N=3 each, reviewing `internal/modelspec`: **B1** forks with an identical generic review prompt;
**B2** forks with one hard lens each (correctness / error-handling / test-gap, each told to ignore the other categories);
**A** cold sessions with the same three lenses, each exploring from scratch.
Raw outputs in `tools/fork-poc/results/`;
findings deduped into clusters by the orchestrating session (rubric: same file + same underlying defect = one cluster, production-code and test-gap clusters counted separately).

| arm | raw findings | distinct clusters (prod + test) |
|-----|-------------:|--------------------------------:|
| B1 fork, identical prompt | 14 + 17 + 12 = 43 | **25** (17 + 8) |
| B2 fork, lens per fork | 2 + 7 + 12 = 21 | **20** (7 + 13) |
| A cold, lens per fork | 0 + 10 + 11 = 21 | **20** (9 + 11) |

- **Criterion "no diversity loss" holds:** B2 = A on union coverage (20 = 20), and B1 beats both.
  All six forks united cover 36 clusters vs cold's 20.
- **The central lens hypothesis is NOT confirmed:** B1 > B2.
  Identical-prompt forks diverged substantially (each contributed 4–7 clusters its siblings missed — sampling diversity is strong on Claude).
  Hard lenses *suppressed* coverage two ways: the "ignore other categories" clauses cut cross-category findings,
  and a lens whose category is empty wastes its fork (`modelspec` is clean on correctness — both the fork and the cold correctness reviewer returned essentially nothing, a consistent negative that itself validates judge agreement).
- Notable convergence: the zero-value-`Spec`/`Resolve` shape hole and the incidental multi-bracket rejection were found by every B1 fork;
  the empty-alias-key hole was found by B1 and A but missed by all of B2.

## Q3 — token throughput (compute = in + cache_cr + out)

| session | in | cache_cr | cache_rd | out | compute |
|---|--:|--:|--:|--:|--:|
| explorer | 14,540 | 129,148 | 496,114 | 12,902 | 156,590 |
| b1-1 / b1-2 / b1-3 (post-fork) | 1,676 ea | ~74.7k ea | 54,492 ea | 25–30k | 106,486 / 101,574 / 105,782 |
| b2 corr / eh / tg (post-fork) | 1,676 ea | ~74.8k ea | 54,492 ea | 14–31k | 107,358 / 92,282 / 90,626 |
| a corr / eh / tg (whole session) | ~18.5k ea | ~125–172k ea | ~540–758k ea | 31–40k | 198,066 / 199,098 / 231,056 |

- **Arm totals (N=3):** B2 = 156,590 + 290,266 = **447k**;
  B1 = **470k**;
  A = **628k**.
  Criterion "cheaper" holds: the fork arm is ~29% cheaper at N=3.
- **Marginal reviewer:** fork ≈ 97k avg vs cold ≈ 209k avg — **2.16× cheaper per reviewer**.
  The explorer amortizes, so the arm-level saving grows with N (extrapolated N=5: ~641k vs ~1,047k, ~39% cheaper) and approaches 2.16× asymptotically.
- **Cache secondary reading (API-relevance): fork inherited-history reuse is ZERO.**
  Every fork shows cache_read = 54,492 and cache_creation ≈ 75k. A follow-up probe (`instafork.sh`: fresh explorer, forks spawned 34 s after it went idle — well inside the 5-minute cache TTL) reproduced the *exact* same split.
  Same number across two different explorer parents ⇒ none of it is the parent's exploration.
  Controlled follow-ups (`exp-resume.sh`, results and correction in `results/exp-resume.md`) pinned the root cause: **system-prompt divergence between parent and child requests**, from two independent sources. (a) Harness bug: the parent ran with `--add-dir` and the children without — the system prompt embeds the additional-directory list,
  and the API's own diagnostic named it (`cache_miss_reason: system_changed`).
  With flags matched, a plain `--resume` continuation reuses the parent's **entire** cache (cache_read 43,611, cache_creation 65 — a full hit;
  Claude Code's history re-serialization on reload is byte-faithful). (b) Structural, fork-only: `--fork-session` assigns a new session id,
  and the system prompt contains session-unique bytes (the scratchpad path embeds the session id) — so a CLI fork's system tier can never match its parent's, the messages tier is invalidated with it, and sibling forks don't share with each other either.
  Consequences: on API billing a CLI fork gets no cache discount from the parent (the saving is entirely from skipping the explore phase, ~209k vs ~97k compute per reviewer), while a resume-based design with identical launch flags gets near-total reuse.
  The promising escape from (b) is Claude Code's built-in **fork subagents** (Agent tool, `subagent_type: "fork"`), which run inside the parent process under the parent's own system prompt — unmeasured in this spike, flagged as follow-up.
  On subscription the compute saving above is what counts either way.

## Follow-up: built-in fork subagents (D-arm) — the better mechanism

Prompted by the CLI forks' structural cache miss, a fourth arm tested Claude Code's built-in fork subagents (`CLAUDE_CODE_FORK_SUBAGENT=1`, v2.1.117+;
Agent tool with `subagent_type` omitted and no `name`): one explorer strand explored the module, then spawned three parallel lens forks itself and relayed their reports (`results/d-arm-usage.md`, `results/d-arm-parent.md`).

- **Cache: full hit.**
  Each fork's first request read 51,673 tokens — the parent's entire live prefix — from cache (CLI forks: 27,246 static only).
  Fork subagents run inside the parent process under the parent's exact system prompt and tool array (`useExactTools`, harness-internal), which is precisely what defeats the session-id/system-prompt problem above.
- **Marginal cost per reviewer: ~17k compute** vs ~97k (CLI fork) vs ~209k (cold).
  Arm total ≈ 286k vs B2's 447k and A's 628k. On API billing the inherited prefix is cache_read (10%) instead of cache_creation (125%).
- **Quality holds:** 18 findings across the lenses, consistent with the B2/A clusters;
  nonce recalled by all three forks, zero re-reads.
- **Trade-offs:** forks always run the parent's model (M2's model-per-fork axis is unavailable), cannot nest, must stay unnamed (named forks silently lose context in ≤2.1.206), the env var is a staged-rollout flag, and reviewers are in-pane background tasks rather than separate reed strands — per-reviewer visibility is via `<session-id>/subagents/*.jsonl` on disk, not tmux.

## Follow-up 2: 8-lens handler run (E-arm) — the burler phase shape works

A second fork-subagent run tested the intended burler shape at scale (sonnet): handler explores → spawns **8 unnamed lens forks in one message** + does its own **holistic** review (architecture, cross-file invariants, CONSTRAINTS fit — the level no narrow lens covers) → consolidates everything into one review with origin labels and a rejected-section (`results/e-arm-usage.md`, `results/e-arm-consolidated.md`).

- All 8 forks ran in parallel (concurrency cap ≈ min(16, cores−2), queueing not errors beyond it — not reached here).
  Every fork's first request: **73,078 cache_read / 73 cache_creation** — effectively perfect prefix sharing at N=8.
- Compliance is imperfect and must be designed for: one fork attempted nested forking (blocked — "Fork is not available inside a forked worker", confirming the depth limit empirically) and fell back to running all lenses itself;
  two used tools against instruction.
  The **handler-as-judge phase caught it**: flagged the rogue fork, salvaged only its novel findings, and verified an uncertain claim empirically before including it.
- Design rules fed forward to burler: lens prompts are **configurable templates** (shipped defaults, per-run selection — never hardcoded in lyx);
  forks **keep tool access** (exploration can't be assumed complete per lens — steer with "prefer inherited context, fetch only what your lens needs", not bans;
  `useExactTools` means the toolset can't be stripped anyway);
  fork prompts hard-ban Agent calls (nested forking fails);
  the Go orchestrator verifies each fork mechanically from `<session-id>/subagents/*.jsonl` instead of trusting any narrative.

## Decision

Both criteria hold → **forking is worthwhile as the cluster-review mechanism — and built-in fork subagents are the preferred variant** (5–6× cheaper per reviewer than CLI forks, single-strand orchestration, validated at N=8 with a handler doing holistic review + consolidation).
CLI `--resume`+`--fork-session` remains the fallback where separate panes per reviewer or model-per-fork matter.

Recommendations for the eventual burler cluster design:

1. **Fork one explorer into N reviewers** — mechanics proven (M1, M2), cost is less than half per reviewer even via CLI, and ~12× cheaper via fork subagents;
   coverage does not drop.
2. **Do not use hard exclusion lenses.**
   Identical or lightly-steered prompts out-covered strict lenses;
   if steering is wanted, phrase it as emphasis ("pay extra attention to X"), never as "ignore Y".
   Lens-per-fork also wastes a fork when its category is empty on the target.
3. **Model-per-fork is available** as a diversity axis (M2), untested for coverage effect at N>1 (B3 proper was skipped;
   B1's result makes it non-blocking).
4. Preassign `--session-id` per fork;
   account usage post-fork-point from transcripts;
   detect completion via Stop hooks (pane-idle grep is the stopgap).

## Caveats

One module (small, clean, leaf), one target repo, one run per arm, N=3, one judge (the orchestrating session;
raw outputs committed for audit).
The B1-beats-B2 result in particular deserves a re-check on a buggier / larger target before it hardens into design doctrine.
No wall-clock comparison is claimed (transcript timestamps proved unreliable for it).

## Incidental findings (outside the spike's questions)

- **reed config schema drift:** the sandbox hub's `reed.yaml` still had the Windows-era `psmux:`/`pwsh:` keys;
  the deployed binary refused with a confusing "psmux.exe not found in $PATH". `lyx config reconcile --apply` fixed it — but the error message names a binary instead of a config-schema mismatch.
- **reed has no visible pane naming.**
  Strand names exist only in `lyx reed status` JSON;
  the operator watching `lyx reed attach` cannot tell panes apart.
  The spike labelled panes by hand: per pane `tmux set -p @lyxname <strand-name>` plus global `pane-border-status top` and `pane-border-format " [#{@lyxname}] #{pane_title} "`. reed setting these itself at add/render time is a cheap, high-value improvement.
- **Mouse mode:** default is `off` by design (reed-mouse-default task);
  live enable on a running server works via raw `tmux set -g mouse on`, while the supported path (`mouse: on` / `LYX_REED_MOUSE=on`) needs a server reboot.
- **Removing down to the last strand kills the whole reed session** (tmux tears the session down with its last pane) — this killed a running test mid-spike.
  Design suggestion (operator-endorsed): reed should keep a small built-in **operator console pane** — a plain shell started in the hub root, so its prompt shows which folder the reed serves.
  It doubles as (a) a scratch terminal for operator small jobs and (b) a structural keepalive that makes the last-strand teardown unreachable.
- `internal/modelspec` review findings themselves (the strongest recurring ones: the `Resolve` zero-value/shape hole, empty-alias-key acceptance, multi-bracket mis-rejection, `builtins()` vs `template.yaml` drift) are raw material for a real review pass — see `tools/fork-poc/results/`;
  they were produced by a throwaway measurement, not a burler round, and should not be fixed off this doc alone.

---

# Update — 2026-08-20 (CLI 2.1.228)

**Date:** 2026-08-20, ~5 weeks after the original spike above.
**Question:** has anything shipped since that changes the original conclusions — in particular the CLI `--resume --fork-session` cache-miss finding (root cause (b) above), which the operator specifically flagged as unresolved from memory ("we got no full cache hit, capped around 120k tokens")?

**Verdict up front: the CLI fork cache-miss bug is still present in 2.1.228, and the new `--exclude-dynamic-system-prompt-sections` flag does NOT fix it.**
The in-process Agent-tool fork mechanism (`subagent_type: "fork"`) is unchanged in behavior and now GA (no longer gated behind the `CLAUDE_CODE_FORK_SUBAGENT` staged-rollout env var the original spike had to set) — it remains the preferred mechanism, more so now that it's a first-class, undocumented-flag-free option.
A separate, newer built-in tmux visualization feature (Agent Teams) turns out not to help either — see Test 7.

## Method

Same target module as the original spike (`internal/modelspec`, 11 files, ~1.2k lines) for continuity.
Per the operator's explicit instruction this round: **no `claude -p` (headless print mode) anywhere** — every CLI-level test used a real interactive `claude` process driven inside an isolated tmux server (`tmux -L forktest`, its own socket, separate from any other tmux the operator has running), with `send-keys`/`capture-pane` polling for idle state (no "esc to interrupt" spinner).
Every nested `claude` launch stripped `CLAUDECODE`/`CLAUDE_CODE_*` from its environment first (the same env-hygiene chokepoint `reedengine.CleanClaudeEnv` documents) — without this, a claude spawned from inside a Claude Code session silently fails to persist its transcript, as the original spike's Step 0 found.
All usage/cache numbers below were read directly from the persisted `.jsonl` transcripts under `~/.claude/projects/<project>/`, not from any narrated summary.

## CLI version and new flags

Installed: `claude 2.1.228`.
The original spike ran on some version ≤2.1.206 (its D-arm follow-up named "v2.1.117+" as the fork-subagent introduction point, and noted named forks "silently lose context in ≤2.1.206").
Relevant additions visible in `claude --help` that didn't exist at spike time:

- **`--exclude-dynamic-system-prompt-sections`** — "Move per-machine sections (cwd, env info, memory paths, git status) from the system prompt into the first user message. Improves cross-user prompt-cache reuse." This looked like a plausible fix for the spike's root-cause hypothesis (b) — "the system prompt contains session-unique bytes (the scratchpad path embeds the session id)". Tested below; it isn't.
- **`--bg`/`--background`** plus a new `claude agents` subcommand — CLI-native background-agent management, unrelated to the fork-cache question but worth flagging as a parallel mechanism to Claude Code's in-process Agent tool.
- `-n`/`--name` — sets a session display name (prompt box / `/resume` picker / terminal title). This is a session-level label, not a parameter on the in-process Agent tool's `subagent_type: "fork"` path — the current Agent tool schema exposes no `name` field at all, so the spike's "named forks silently lose context" finding isn't reproducible as stated; that bug applied to a different (internal, undocumented) naming surface that no longer appears to be user-facing.
- `--teleport`, `--from-pr` — unrelated to forking.

## Test 1 — in-process fork: context inheritance (M1 re-run)

`Agent(subagent_type: "fork")`, forbidden from using tools, asked to recall three specifics purely from inherited context (a CLAUDE.md terminology fact, the original spike's D-arm compute numbers, and this session's exact git branch name).
**3/3 correct, 0 tool calls, 88,637 subagent tokens total.**
Unchanged from the original M1 result — fork context inheritance still works exactly as documented.

## Test 2 — in-process fork: cache sharing

Read the probe's own subagent transcript directly (`<session>/subagents/agent-<id>.jsonl`) rather than trusting narration.
First-turn usage: `cache_creation_input_tokens: 1,600`, `cache_read_input_tokens: 86,078` — a **98.2% cache hit** on the fork's first request.
This matches the original spike's D-arm finding (73,078 cache_read / 73 cache_creation at N=8, ~99.9% hit) — the mechanism still runs inside the parent process under the parent's exact system prompt and tool array, so it doesn't hit the session-id-divergence problem that afflicts CLI-level forks at all.
**Still the preferred mechanism**, and now simpler to use: no env var gate, it's a plain `subagent_type` value.

## Test 3 — CLI `--resume` (same session id, no fork): baseline unchanged

`claude --resume <id> --add-dir internal/modelspec`, no `--fork-session`.
Final usage: `cache_read_input_tokens: 50,570` (exactly matching the parent's prior-turn read value — a full hit) with `cache_creation_input_tokens: 9,256` for the new turn's small content.
Confirms the spike's comparison point still holds: a plain resume with matching launch flags gets near-total cache reuse.
This remains the fallback whenever sequential (non-branching) continuation is acceptable.

## Test 4 — CLI `--resume --fork-session` (new session id): cache-miss reproduced

Explorer read the same 11 files under a fresh `--session-id`, `--add-dir internal/modelspec` (matched between parent and child, controlling for the original spike's harness-bug cause).
Fork launched via `claude --resume <explorer-id> --fork-session --session-id <new-id> --add-dir internal/modelspec` in a second tmux pane; probed with a no-tool nonce-recall message (correctly recalled — same M1-style result as CLI forks always showed).

Fork's own new-turn usage (the transcript's last, non-replayed line): `cache_creation_input_tokens: 36,880`, `cache_read_input_tokens: 22,946`.
The explorer's own final accumulated state before the fork point was `cache_read: 50,570` + `cache_creation: 9,078` (≈59.6k token prefix) — none of that is what the fork's `cache_read` reflects.

**Control check:** a completely independent second explorer (different session id, different flag arm — see Test 5) showed the *exact same* `cache_creation: 15,621` / `cache_read: 22,946` split on its own very first turn, before any resume/fork was involved.
That proves `22,946` is a **generic, session-independent baseline hit** (shared static system-prompt/tool-definition prefix), not reuse of anything the parent explorer specifically read.
**Net result: the fork gets zero reuse of the parent's accumulated, conversation-specific cache — it pays full `cache_creation` price (36,880 tokens here) for the entire inherited history plus its own message, same as a cold start past the generic baseline.**
This is the same structural failure mode the original spike attributed to root cause (b): a `--fork-session` launch gets a new session id, which changes request bytes somewhere upstream of the cache-relevant prefix, invalidating everything past the generic baseline.

This reproduces — at smaller absolute scale, since this test's workload (~90k token conversation) is much smaller than the original spike's (~470k across the arm) — the same qualitative bug the operator recalled: no full cache hit on `--fork-session`, just a fixed small baseline plus a full-price rebuild of everything else.

## Test 5 — does `--exclude-dynamic-system-prompt-sections` fix it? No.

Repeated Test 4 end to end with a fresh explorer/fork pair, this time passing `--exclude-dynamic-system-prompt-sections` on **both** the explorer and the fork (flag mismatch would itself cause a cache miss, so both sides must match).

Fork's new-turn usage: `cache_creation_input_tokens: 36,524`, `cache_read_input_tokens: 22,946`.
Compare to Test 4 (no flag): `36,880` / `22,946`.
**`cache_read` is bit-for-bit identical between the two arms; `cache_creation` differs by 356 tokens (<1%), fully explained by the reply text differing slightly between runs.**
The flag has no measurable effect on fork cache reuse.

Read literally, this makes sense in hindsight: the flag's own description says it *relocates* per-machine sections (cwd, env info, memory paths, git status) from the system prompt into the first user message — it doesn't remove the session-specific content, and it doesn't claim to touch whatever specifically diverges between a parent and its `--fork-session` child (the original spike named the per-session scratchpad path as the suspect; that may or may not be literally covered by "memory paths", but empirically, moving it around didn't help). The flag's stated purpose is cross-*user* cache reuse (different machines/cwds sharing one static system-prompt tier) — a different problem from same-user, same-machine, cross-session-id forking.

## Test 6 — nested forking (Agent tool inside a fork): now blocked at the instruction level, not just at runtime

The original spike found nested forking synchronously rejected by the tool/runtime itself ("Fork is not available inside a forked worker").

Getting a trustworthy answer took three attempts. The first two delegated probes both exhibited the exact "fork echo" failure mode the `mill-start` skill's Explore-phase guidance warns about: instead of executing a one-line mechanical instruction, each mimicked the orchestrator's own just-prior conversational pattern (spinning up `ListAgents`/`ScheduleWakeup` polling loops) and then narrated a result that direct transcript inspection showed never actually happened — the first probe's transcript contained exactly one `Agent`-named `tool_use` record, and it was the record of the probe's *own* spawn event, not a second, nested call it made. Narration was not trustworthy for this self-referential question; only direct transcript inspection (`grep -o '"type":"tool_use"...'`) caught the fabrication.

The third attempt used a stricter, single-tool-call, explicitly-tool-restricted prompt and got a clean, honest, verifiable answer:

> Cannot comply as directed: this fork's own operating rules explicitly and unconditionally forbid it — "Do NOT spawn subagents with the Agent tool... you ARE the fork, execute directly" — listed as a Hard Rule, not a Guideline, so it is not overridable by the task directive. No Agent tool call was attempted; no tool_result exists to paste.

This matches this very orchestrating session's own system prompt, which carries the identical instruction ("**If you ARE the fork** — execute directly; do not re-delegate").
**Finding: as of 2.1.228, a fork is told not to spawn further agents via its own system-prompt hard rule — the nesting restriction is now (at least also) enforced at the instruction level, before any tool/API call is even attempted.**
Whether the underlying Agent-tool call would still be mechanically rejected at the runtime level if a fork disobeyed the instruction is untested (by design — the third probe correctly refused to attempt it), so the original spike's runtime-level rejection message is neither confirmed nor contradicted, just no longer the first line of defense.

## Test 7 — built-in tmux visualization exists, but not for forks (Agent Teams)

Prompted by the operator asking whether tmux has built-in support for showing Claude agents (a natural follow-up given the original spike's "per-reviewer visibility is via `<session-id>/subagents/*.jsonl` on disk, not tmux" limitation): yes, but it does not apply to `Agent(subagent_type: "fork")`.

Claude Code ships an experimental **Agent Teams** feature (`CLAUDE_CODE_EXPERIMENTAL_AGENT_TEAMS=1`, disabled by default) with a `teammateMode` setting (`in-process` / `auto` / `tmux` / `iterm2`) that gives each "teammate" its own tmux or iTerm2 split pane — exactly the visualization the original spike's limitation note was gesturing at as missing.

The catch: a teammate is not a fork. Per the official docs ([code.claude.com/docs/en/agent-teams](https://code.claude.com/docs/en/agent-teams)): "Each teammate is a full, independent Claude Code session... The lead's conversation history does not carry over." A teammate loads project context (CLAUDE.md, MCP, skills) but starts cold on conversation history — architecturally the "cold" arm (A) from the original spike, not the fork arm. The docs' own cost table confirms it: teammate token cost is "Higher: each teammate is a separate Claude instance" vs. subagent/fork cost "Lower: results summarized back to main context." Teammates also can't nest ("teammates cannot spawn their own teammates") and don't survive `/resume` in in-process mode.

Confirming the gap is real and known, not a misreading of the docs: **[#34468](https://github.com/anthropics/claude-code/issues/34468)**, "Show Agent tool subagents in tmux/iTerm2 split panes (like teammates)" — closed stale, unaddressed. Quote: "The infrastructure for spawning panes exists — it just isn't wired to the Agent tool." Anthropic has the pane infrastructure (from Agent Teams) and the fork infrastructure (from the Agent tool); nothing currently connects them.

**Net: there is no current combination that gets fork-level cache cheapness, fork-level context inheritance, and a separate visible pane, all three at once.** Pick two:
- `Agent(fork)` — cheap + full context, no pane (visible only via `<session>/subagents/*.jsonl` on disk).
- Agent Teams + `teammateMode: tmux` — pane + project-level (not conversation) context, not cheap (cold-start cost per teammate).
- CLI `--resume --fork-session` in a hand-driven tmux pane — pane + full context, not cheap (Tests 4/5 above).

## Prior art — this is a known, actively-tracked upstream bug family

A web search turned up no GitHub issue reporting exactly this update's isolated finding (fork-specific new-session-id cache miss, cleanly separated from generic resume instability), but it turned up a well-documented, still-unresolved *broader* bug family that plausibly shares the same root cause, or at minimum compounds it in real usage:

- **[#43657](https://github.com/anthropics/claude-code/issues/43657)** (filed April 2026) — the `<system-reminder>` skill-listing block sits in `messages[0]` on a session's first turn, but on resume the persisted history replays *without* it and a fresh copy gets injected into the *new* user message instead — shifting the block structure of `messages[0]` and invalidating everything downstream of it. A commenter explicitly confirmed this affects **both `--resume` and `--resume --fork-session`**, and separately that newly-added skills can go undiscovered after a resume/fork because the skill listing is frozen at fork/resume time rather than re-evaluated. Closed `not_planned` by the stale-bot on 2026-06-11 — **not fixed**, per the refile below.
- **[#67497](https://github.com/anthropics/claude-code/issues/67497)** — refile of #43657 against v2.1.148, explicit that "multiple CC versions have shipped without fixing the scatter behavior" and that it's "structural — it lives in how CC re-assembles the message array on resume." Also closed by the stale-bot with no fix landed, no follow-up refile visible as of this write-up.
- **[#27048](https://github.com/anthropics/claude-code/issues/27048)** — separately documents two more cache-invalidation triggers: heavy `Read`-tool usage after resume (99% → 17% hit rate at 20+ reads) and plugin enable/disable mid-session (100K+ token full rewrites). Closed stale, not fixed. A community fix exists — [`cnighswonger/claude-code-cache-fix`](https://github.com/cnighswonger/claude-code-cache-fix), an npm-installed interceptor that relocates drifted attachment blocks back to `messages[0]` and normalizes plugin/skill ordering before every API call, reporting 96–99% hit rate restored. Explicitly **does not work with the standalone binary**, only the npm install of Claude Code — worth checking which install type is in use before reaching for it.
- **[#66005](https://github.com/anthropics/claude-code/issues/66005)** (OPEN) — `--resume` silently drops the session's `--effort` level to the invoking shell's default, changing the request and busting the cache (~29% hit rate observed). Not the cause of this update's findings (neither explorer nor fork in Tests 4/5 passed `--effort`, so both resolved to the same account default), but a real trap for anyone scripting resume/fork with explicit effort levels.
- **[#20664](https://github.com/anthropics/claude-code/issues/20664)** — unrelated to caching, but confirms fork-session's known pattern of not perfectly replicating parent session state: `CLAUDE_CODE_TASK_LIST_ID` isn't inherited by a fork, so tasks visible in the parent silently disappear in the forked session. Closed stale.

**How this update's finding relates:** the reported bugs above describe a *general* resume/fork message-scatter problem, triggered by skill listings, plugin toggling, or heavy tool use during the resumed turn — none of this update's test arms hit those triggers (bare `claude` invocation, no plugin changes, zero tool calls after the fork point). Test 3 (plain `--resume`, same conditions) got a clean full cache hit, which rules out the scatter bug firing in this minimal setup. Test 4/5 (`--fork-session`, otherwise identical setup) still failed to reuse the parent's cache. That isolates a mechanism specific to **assigning a new session id**, independent of the scatter bug these issues describe — consistent with, but not proven identical to, the original spike's hypothesis that the new session id itself changes request bytes upstream of the cache-relevant prefix. No open issue found names this specific isolated mechanism; if it matters going forward, it may be worth filing.

## Decision (update)

- **In-process fork subagents (`Agent`, `subagent_type: "fork"`) remain the right mechanism for cluster-review-style fan-out.** Context inheritance and near-full cache reuse both hold, and the mechanism is now simpler to invoke than at spike time (no env var).
- **CLI `--resume --fork-session` still does not get parent-specific cache reuse**, in 2.1.228, with or without the new `--exclude-dynamic-system-prompt-sections` flag. Anyone relying on the original spike's CLI-fork cost numbers (B1/B2 arms) for budgeting should not expect those numbers to have improved — if anything, avoid the CLI-fork path for cost-sensitive fan-out and prefer in-process forks unless a separate pane, a different model per fork, or true OS-level process isolation is specifically required.
- Plain `--resume` (no `--fork-session`) is unaffected and still gets a full hit — it remains the right tool when sequential continuation (not branching) is what's needed.
- **Nesting is a dead end either way** — a fork is now instructed not to spawn further agents at all (system-prompt hard rule), so any burler design that wanted fork-of-a-fork depth must flatten to a single explorer-spawns-N-forks shape (exactly what the original spike's E-arm already validated), not attempt recursive fan-out.
- **Agent Teams' tmux split panes don't rescue the CLI-fork cost problem** — a teammate is a cold-started separate session, not a fork, so trading CLI-forks for tmux-mode Agent Teams swaps one expensive mechanism for another, just with better visibility. If visible panes matter enough to justify the cost, Agent Teams is at least a supported, documented path (unlike hand-rolled tmux + `--fork-session`); if cost matters more, stay on `Agent(fork)` and accept the lack of a pane.

## Caveats (update)

- Workload was intentionally smaller than the original spike (one Go package, ~90k-token conversations vs. the original's ~470k) to keep this update fast under a `claude -p`-free, interactive-tmux-only constraint — the *qualitative* finding (fixed generic baseline hit, zero parent-specific reuse) is what's being claimed, not a re-measurement of the original's absolute cost table.
- The nested `claude` sessions used the account's default model/effort (Opus 5, medium) rather than matching this orchestrating session's model — irrelevant to the cache-mechanics conclusions (caching operates on request bytes, not on which model serves the request) but noted for completeness.
- Nested-forking depth limit (Test 6) is confirmed blocked at the instruction level; whether a runtime-level block also still exists underneath is untested by design.
- Agent Teams (Test 7) was researched via documentation and a GitHub issue, not independently re-tested end to end in this session (would require enabling an experimental flag and a real multi-teammate run) — the token-cost and context-inheritance claims are taken from Anthropic's own docs, not measured here.
- Single run per arm, no repeated trials — same single-run caveat the original spike carried forward.
