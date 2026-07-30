---
name: prowler
description: Fetch blocked/restricted web pages and answer questions about their content
argument-hint: "<url> [url2...] [question]"
---

Use this skill only when the built-in `WebFetch` tool fails or returns unusable content (bot-blocked sites, paywalls, JS-rendered pages, Reddit posts). It drives a real headless browser plus Readability extraction as a fallback, so it can read pages `WebFetch` cannot.

Raw output is often long and noisy (a fetched Reddit page especially mixes nav/sidebar chrome with the real content), so decide for yourself whether to read it directly or wrap the fetch in a Haiku subagent:

- **Already a small, isolated worker** (e.g. a dedicated research subagent reading a handful of sources) — read the file yourself; do steps 1-2 inline.
- **Otherwise** (a general-purpose or long-lived thread, any expensive tier) — wrap the fetch(es) in a Haiku subagent (step 3). Dispatch overhead (~5-10s) is paid once per subagent, mostly startup, not per URL — so give every URL for one batch of related questions to a single dispatch's one `run.sh` call (it already fetches them concurrently and joins the results) rather than spawning one subagent per URL. Only split into multiple subagents (dispatched in parallel, never a sequential loop) when sources genuinely need independent, unmixed answers.

Steps:

1. Resolve the wrapper's absolute path now, while `${CLAUDE_SKILL_DIR}` is still set — a dispatched subagent won't have it: `RUN_SH="$(cd "${CLAUDE_SKILL_DIR}/../../scripts" && pwd)/run.sh"`.
2. Per fetch: run `path=$(bash "$RUN_SH" <url1> [url2...])`, checking the exit code (report stderr and stop on failure). Read `$path`. Answer the question, or give a 3-5 sentence summary if none was asked — say explicitly if the content doesn't support an answer, never guess. Delete the file: `bash -c 'rm -f "<path>"'`.
3. If wrapping: dispatch one Haiku-tier subagent (Agent tool, `model: haiku`) running step 2 with every URL for this batch passed to the same `run.sh` call. Dispatch more than one subagent only when sources need independent, unmixed answers — in that case, all of them in one message (parallel), never a sequential loop. Each subagent returns only its distilled answer, never the raw content.
4. Relay each answer, or synthesize across several. Never dump raw fetched content to the user, or carry it into your own context as a substitute for the above.
