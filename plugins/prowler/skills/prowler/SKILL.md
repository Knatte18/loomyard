---
name: prowler
description: Fetch blocked/restricted web pages and answer questions about their content
argument-hint: "<url> [url2...] [question]"
---

Use this skill only when the built-in `WebFetch` tool fails or returns unusable content (bot-blocked sites, paywalls, JS-rendered pages, Reddit posts). It drives a real headless browser plus Readability extraction as a fallback, so it can read pages `WebFetch` cannot.

The raw fetched output is often long and noisy — a fetched Reddit page in particular mixes nav/sidebar chrome in with the actual post and comments — so reading it directly into a long-lived or expensive-tier context is often wasteful. Weigh that against two measured costs before deciding whether to wrap the fetch in a Haiku subagent:

- **Latency:** dispatching a Haiku subagent to run+read+extract measured ~12s wall-clock for a single trivial fetch, against ~2.4s for running the wrapper directly — roughly 10s of pure agent-dispatch overhead (fresh context + multi-turn tool loop), on top of whatever the fetch itself costs. Dispatching several fetches in parallel (one Agent call per URL, all issued in the same message) pays that ~10s roughly once for the whole batch, not once per URL — always prefer parallel dispatch over a sequential loop when fetching more than one URL this way.
- **Reliability:** a cheap model can misread or fail to find what was asked, and — unlike a raw file you could re-check yourself — you have no way to verify a subagent's distilled answer against the source afterward. Every Haiku dispatch below must therefore be told: explicitly say so if the fetched content does not clearly contain an answer to the question asked, rather than guessing or filling the gap with plausible-sounding but unsupported text.

**Decide for yourself which side of this you're on** — this skill does not hardcode the answer:

- **You are already a small, dedicated, isolated worker** (e.g., a "trawler"-style research subagent spawned to read just a handful of sources for one specific question, not a long-lived general-purpose thread) — the isolation a Haiku wrapper exists to provide is already in place around you. Read and extract inline yourself; an extra Haiku layer under an already-cheap, already-isolated worker mostly just adds the ~10s dispatch cost above for little benefit.
- **You are not that** (a general-purpose thread, a long-lived orchestrator, or anything on an expensive model tier, regardless of how many pages you're fetching) — always wrap each fetch in a Haiku subagent per the steps below. Raw fetched chrome/noise accumulating in a context like that is exactly the cost this pattern exists to avoid.

Steps (when you decide to wrap):

1. Resolve the wrapper's absolute path now, while `${CLAUDE_SKILL_DIR}` is still set: `RUN_SH="$(cd "${CLAUDE_SKILL_DIR}/../../scripts" && pwd)/run.sh"`. A dispatched subagent starts a fresh context with no `${CLAUDE_SKILL_DIR}`, so the absolute path must be resolved here and embedded literally in its prompt.
2. Dispatch one Haiku-tier subagent (Agent tool, `model: haiku`) per logical fetch — one URL or one small batch of related URLs — with Bash and Read access, instructing it to, inside its own context:
   a. Run `path=$(bash "$RUN_SH" <url1> [url2...])` and check the exit code — on non-zero, report the failure surfaced on stderr and stop.
   b. Read the file at `$path`.
   c. Answer the given question against its content, or give a 3-5 sentence summary when no question was asked — ignoring page chrome/navigation noise. If the content does not clearly support an answer, say that explicitly instead of guessing.
   d. Delete the file: `bash -c 'rm -f "<path>"'` (issued as `bash -c` so the pinned `Bash(bash *)` permission covers it — never a bare `rm`).
   e. Return only the distilled answer/summary (or the explicit "couldn't find it" note) as the final message — never the raw fetched content.
3. When more than one fetch is needed, dispatch all of that round's Agent calls together in one message (parallel), not one-by-one in a loop — see the latency note above.
4. Relay each subagent's answer, or synthesize across several once all have returned. Never read a fetched output file directly in your own context as a substitute for this. If a subagent reported it couldn't find the answer, either try a different source or say so rather than inventing content on its behalf.
5. Never dump raw fetched content to the user or carry it forward in your own context — always go through the distillation in step 2 (or your own inline read, if you decided above that you're already the isolated worker this pattern exists to protect against).
