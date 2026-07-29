# trace for LLM-agent use — findings from a live benchmark

**Context:** ad-hoc investigation on the `codeintel-v1` branch, after the module (and its `codeintel-daemon-persistence` follow-up) had already shipped — pre-rename branch names; the module is `trace` today. **Date:** 2026-07-29. Full raw data: [docs/benchmarks/trace-vs-grep.md](../benchmarks/trace-vs-grep.md). Related design follow-up: [manifest/designs/trace-plan-symbol-fields.md](../../manifest/designs/trace-plan-symbol-fields.md).

**Verdict up front: trace does not clearly help a Claude Code agent that is free to choose its own tools.** Across three benchmark tasks (two rounds), a plain grep-based agent matched or beat a trace-required agent on every efficiency metric once the task got genuinely hard, while both reached correct final answers throughout. Worse, the CI-shaped `assert-no-callers` gate this session built was found — live, not hypothetically — to report **31 false-positive "violations" where only 2 were real**, on an ordinary interface method. That bug is now mitigated (a `--within` flag, commit `136944f3`), but only for a caller who already knows to reach for it; the tool's *default* behavior on an interface method is still the one that produced the false positives.

## What was actually measured

Four fresh, independent general-purpose subagents per round (trace-required vs. grep-only, no shared context, no knowledge of each other), graded against ground truth verified by hand before dispatch — not against either agent's own claim. See the linked benchmark doc for full methodology, prompts, and per-task tables.

| Task | What it tested | Result |
|---|---|---|
| 1 — `Burler.Run` interface dispatch | A single real call site among 26 raw `.Run(` hits, no distinctive anchor if grepped naively | Both correct. Grep-only *faster* (38.8s vs 79.6s) — neither agent actually grepped naively; both searched the rare interface name first. |
| 2 — `hubgeometry.Resolve` name collision | 31 real callers among 46 raw hits, 3 unrelated same-named functions | Both correct. Trace meaningfully cheaper here (~40% fewer tokens) — the one clean win, because grep still had to search a bare, ambiguous name. |
| 3 — `builderengine.clock` interface, no anchor at all | An unexported interface, copy-pasted near-identically across 3 packages; no textual anchor exists for either condition | Both correct, but **grep won every metric** — grep's natural directory-scoping sidestepped the ambiguity for free; trace's `refs` had no equivalent and returned ~38 raw hits, mostly cross-package noise, that the agent had to filter by hand. |

Combined across all three tasks: trace's wall-clock edge shrank from -23% (after round 1 alone) to roughly a wash (-2%) once the harder round-2 task was added, and it ended up using *more* total tool calls than grep, not fewer. Only the token count kept a real edge (-16%).

## Yes — a genuine false positive, not just an efficiency loss

This is the finding worth calling out on its own, separate from the win/loss tally above. Task 3's interface, `internal/builderengine/poll.go`'s local `clock { Now() time.Time; Sleep(d time.Duration) }`, has exactly 2 real callers within `builderengine` (verified by hand: `poll.go:203` and `poll.go:213`). Before this session's same-day fix:

```
$ lyx trace assert-no-callers internal/builderengine/poll.go:177:2 --target-dir .
{"ok":true,"violation":true,"callers":[ ...31 entries... ]}
```

29 of those 31 reported "callers" were not callers of this interface at all — they were real, correct call sites belonging to two *other*, unrelated `clock` interfaces independently declared in `internal/shuttleengine` and `internal/websterengine`, plus their test files. gopls' `textDocument/references` on an interface method conservatively returns every method matching that name+signature across every structurally-compatible interface in the whole workspace — documented gopls behavior, not a bug in this repo's wrapper — but `assert-no-callers` passed that noise straight through as if every entry were a genuine violation. A caller (human or agent) trusting that exit code and that list without independently re-deriving ground truth would have wrongly concluded a safe, 2-caller interface method had 31 live dependents.

This landed on exactly the tool this session built earlier the same day specifically to remove human/agent judgment calls from a mill batch's `Deletes:`/`Moves:` safety check (see [manifest/designs/trace-plan-symbol-fields.md](../../manifest/designs/trace-plan-symbol-fields.md)). A false positive in a tool meant to replace review discipline with a deterministic gate is a worse failure mode than a false positive an agent might catch by re-reading the code — the entire point of building it was to make that re-reading unnecessary.

**The fix that shipped same day (`--within <dir>`, commit `136944f3`) requires the caller to already know the risk exists and opt in.** `assert-no-callers` run the same way, with no flags, on any other interface method in this codebase today still returns the same class of false positive by default. Whether `--within` (or an equivalent safe scope, e.g. defaulting to `--target-dir` itself) should become the *default* rather than opt-in is an open question this document does not resolve — see "Open follow-up" below.

## Why this doesn't reverse the earlier "via lyx" direction

Everything in this document is about **agent-facing use**: a Claude Code agent, mid-task, choosing whether and how to call `lyx trace`. This session had already concluded (before this benchmark) that trace's more durable value is **Go-native, deterministic use** — code calling `traceengine` directly, or a mill batch's `verify:` field calling `assert-no-callers` — where no agent is ever choosing whether to trust the result, so the "does an agent reach for the right tool and interpret its output correctly" question this benchmark tests doesn't arise the same way. That direction is not undermined by this document. If anything, the false-positive finding above is a reason to invest *more* carefully in the deterministic path (get the tool correct once, so every deterministic caller benefits) rather than in agent-facing promotion (where a skilled agent gets similar results from grep anyway, per the tables above).

## Open follow-up

- Should `assert-no-callers` default to a safe scope (e.g. `--target-dir`) instead of workspace-wide, requiring `--within all` or similar to opt *out* of scoping rather than opt in? Not decided or implemented here.
- The same interface-method conflation affects plain `refs`/`definition` too (just as a noise/cost problem there, not a false-positive-with-real-consequences problem, since those verbs don't assert anything) — `--within` fixes both, opt-in, as of `136944f3`.
- No equivalent live test has been run for the other four `trace` languages (Python, C#, TypeScript, Rust) or their language servers — this document's findings are Go/gopls-specific.
