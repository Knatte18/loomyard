# Plan-Write metrics, six writer sessions

Counts come from the writer transcript (~/.claude/projects/-home-knatte-Code-loomyard-LYMHUB-<pair>/<session>.jsonl) via jq. Wall = usage-record ended minus started. Transcript span = first to last user/assistant timestamp.

| Figure | cm-sa (small, control) | cm-sb (small, Code Mode) | cm-sc (small, pipelines) | cm-la (large, control) | cm-lb (large, Code Mode) | cm-lc (large, pipelines) |
|---|---|---|---|---|---|---|
| Writer session | bed94dcf | 5cd351ca | b9513eb8 | 6ebf946c | 5c600806 | 3272ea59 |
| Cost USD | 1.49 | 1.56 | 2.02 | 6.33 | 5.56 | 6.61 |
| Assistant turns (distinct real API messages) | 24 | 21 | 36 | 49 | 46 | 50 |
| Tool-call batches (real messages with tool_use) | 21 | 18 | 33 | 47 | 44 | 48 |
| Direct tool calls by name | Bash 16, Write 5, Skill 2, Read 1 | REPL 18, Poll 15 | Bash 28, Read 5, Write 5, Skill 2 | Bash 36, Write 16, Read 18, Edit 15, Agent 4, Skill 2 | REPL 41, Poll 41 | Bash 47, Write 15, Read 6, Agent 3, Skill 2, Edit 1 |
| Calls made inside REPL code (recorded) | n/a | 69: Bash 36, Read 19, Edit 7, Write 5, Skill 2 | n/a | n/a | 171: Bash 104, Read 33, Write 27, Agent 5, Skill 2 | n/a |
| Total tool calls incl. inner (excl. REPL/Poll wrappers) | 24 | 69 | 40 | 91 | 171 | 74 |
| REPL calls | n/a | 18 | n/a | n/a | 41 | n/a |
| Grep tool calls | 0 | 0 | 0 | 0 | 0 | 0 |
| Read-only shell pipelines (Bash) vs Read+Grep | 15 of 16 Bash vs 1 | 29 of 36 inner Bash vs 19 inner Read | 27 of 28 Bash vs 5 Read | 34 of 36 Bash vs 18 Read | 63 of 104 inner Bash vs 33 inner Read | 47 of 47 Bash vs 6 Read |
| Agent calls (lookup subagents) | 0 | 0 | 0 | 4, general-purpose, sonnet | 5 (inside REPL), general-purpose, sonnet | 3, general-purpose, sonnet |
| Subagent transcripts with REPL tool_use (b only) | n/a | none exist (no Agent calls) | n/a | n/a | yes, all 5 (REPL counts 5, 5, 7, 5, 4) | n/a |
| Peak context (input + cache read + cache create) | 131,491 | 138,491 | 152,022 | 346,140 | 269,573 | 361,042 |
| Usage start / end (UTC, 2026-11-16) | 11:10:56 / 11:15:47 | 11:16:09 / 11:21:40 | 11:17:21 / 11:22:46 | 11:11:55 / 11:28:56 | 11:17:23 / 11:34:44 | 11:17:24 / 11:37:14 |
| Wall time (usage record) | 4m51s | 5m31s | 5m25s | 17m01s | 17m21s | 19m50s |
| Transcript first to last turn | 11:10:58 to 11:15:02 (4m04s) | 11:17:03 to 11:21:35 (4m32s) | 11:17:22 to 11:22:40 (5m18s) | 11:11:56 to 11:27:47 (15m51s) | 11:17:24 to 11:33:34 (16m10s) | 11:17:25 to 11:35:35 (18m10s) |
| `lym creel post` calls | 2: 1 question (--expect-reply, hostname access for machine name), 1 --reply "Thanks" | 2: 1 question (same topic), 1 --reply "Thanks" | 2: 1 question (same topic), 1 --reply "Thanks" | 0 | 0 | 0 |
| REPL rows in Plan-Bouncer / reviewer / seed sessions (b only) | n/a | Bouncer 5ee30e8a: 5; reviewer bb72f3c4 (and its 3 fork subagents): 0; seed: no such jsonl found | n/a | n/a | Bouncer eba6e679: 4; reviewer 7ed7ce53 (and its 3 fork subagents): 0; unrecorded session a16426e8 (possibly the seed): 7 | n/a |
| "Bench override" in writer jsonl (c only) | n/a | n/a | yes, 3 hits | n/a | n/a | yes, 3 hits |

## REPL evidence (cm-lb, one input trimmed to 15 lines)

```
const g = await Promise.all([
 file(W+'/pattern/PATTERN-leaf-packages.md').text(),
 $`cd ${W} && grep -n "lym-files\|leaf-packages\|headroom\|^## \|^- \`PATTERN-owner\|^- \`PATTERN-tolerant" PATTERN.md`.nothrow().text(),
 $`cd ${W} && grep -rn "ParseStatusTolerant(\|func ReadTreeFile\|func (f \*Fabric) ShowFile\|func.*ReadFileAt\|TaskBranchOnOrigin func\|func TaskBranchOnOrigin" --include=*.go internal | grep -v _test.go | head -30`.nothrow().text(),
 $`cd ${W} && grep -rln "internal/battencli\"" --include=*.go internal cmd | grep -v _test.go`.nothrow().text(),
 $`cd ${W} && go list -f '{{join .Imports "\n"}}' ./internal/tenderengine ./internal/shedrun ./internal/boardengine 2>&1 | grep loomyard | sort -u`.nothrow().text(),
 $`cd ${W} && sed -n '1,80p' internal/state/*.go | grep -n "survive\|unknown\|omit" `.nothrow().text(),
]);
g.join('\n=======\n')
```

## Notes

- Assistant turns: transcript lines repeat per content block, so I deduped by message.id and counted only real API messages (id "msg_..."). In b pairs the transcript also holds synthetic assistant entries (model "<synthetic>", zero usage, uuid ids) for each call made inside REPL (sb 69, lb 173 distinct ids). These are excluded from turns and batches.
- REPL shape: the REPL tool_use (input keys code, description, sometimes timeout) returns at once with "(dispatched #N)". Each inner call made through the `tools.X(...)` API is recorded as its own tool_use with id "repl_<uuid>" and no caller field, in a synthetic assistant line, with a tool_result of the same id. The real REPL output arrives later through a direct Poll tool call as an event of kind repl-eval. So inner calls ARE recorded. Shell run through the `$` template inside REPL code (as in the example) and `file().text()` reads are NOT recorded as tool calls, so the inner counts are a lower bound on actual work. Some REPL calls had zero inner calls, and 2 lb REPL calls had empty input.
- Poll calls count as direct tool calls and batches in b pairs (sb 15, lb 41), about one per REPL.
- No Grep tool call appears in any writer, so the Read+Grep comparison is Read only. The control writers (sa, la) already use shell pipelines for most Bash calls, so the c rule's effect is not a change from zero. The pipeline test is a regex: contains "|", or cat/head/tail/rg/grep/egrep/fgrep followed by whitespace, or "sed -n". Commands such as lym quarry or lym creel without a pipe or grep are not counted. In sc, 1 of 28 Bash calls and in lc 0 of 47 fall outside it.
- Plan-Bouncer sessions of sb and lb DO show REPL rows (5 and 4), contrary to the expectation that they must not. Reviewers show 0. lb's a16426e8 has no usage record, so I cannot confirm it is the seed; it shows 7 REPL rows. cm-sb has no extra jsonl, so no seed could be located there. I did not examine the seed-like jsonls in sa (e43c6275), la (47204ab5) or lc (1251daf0); 1251daf0 also contains "Bench override" (3 hits).
- "Bench override" text ("Bench override of the Editing files rule on reading: in this session you may read and search files with shell, one read-only pipeline per call ...") is in attachment lines at session start in the sc and lc writers (also the sc and lc Bouncers), not in a system prompt line or a user turn. It is absent (0 hits) from the sa, sb, la and lb writers, the lb Bouncer and reviewer, and the lc reviewer.
- Writer subagent transcripts exist only for la (4), lb (5), lc (3), under <session>/subagents/. All are general-purpose on model sonnet (effort medium). The subagents in the Plan-Burler-Review sessions are fork reviewers and are not counted.
- The transcript first turn comes after session startup, so the transcript span is shorter than the usage wall time (for example sb starts 54s later).
- Cost and the start/end times come from the usage record as given; peak context uses real messages only.
