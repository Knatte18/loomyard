---
name: prowler
description: Fetch blocked/restricted web pages and answer questions about their content
argument-hint: "<url> [url2...] [question]"
---

Use this skill only when the built-in `WebFetch` tool fails or returns unusable content (bot-blocked sites, paywalls, JS-rendered pages, Reddit posts). It drives a real headless browser plus Readability extraction as a fallback, so it can read pages `WebFetch` cannot.

Steps:

1. Run the wrapper and capture its output path: `path=$(bash "${CLAUDE_SKILL_DIR}/../../scripts/run.sh" <url1> [url2...])`.
2. Check the wrapper's exit code before doing anything else. On non-zero exit, the run failed — report the failure surfaced on stderr and stop. Do not attempt to read a path.
3. On success, read the file at `$path`.
4. Answer the user's question about the fetched content, or give a 3-5 sentence per-source summary when no question was asked.
5. Delete the output file: `bash -c 'rm -f "<path>"'` (issued as `bash -c` so the pinned `Bash(bash *)` permission covers it — never a bare `rm`).
6. Never dump raw fetched content to the user — always answer or summarize.
