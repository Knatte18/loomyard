# Benchmarks: codeintel (LSP) vs grep on hard symbol-resolution tasks

Compares `lyx codeintel` (LSP-backed, semantic) against grep/rg-based text search on two tasks chosen specifically to be hard for text search: a Go interface-dispatch call site (no textual link between the interface method and its implementation) and a same-named-function-in-multiple-packages rename-safety check. Both tasks were run twice — once by an agent required to use `lyx codeintel`, once by an agent forbidden from using it or any other LSP tool — as two independent, fresh, general-purpose subagents per condition, with no shared context between any of the four runs and no knowledge of each other's existence. Ground truth for both tasks was verified by hand (via `lyx codeintel refs`/`assert-no-callers` plus manual code reading) before any agent was dispatched, so grading is against a fixed, independently-known-correct answer, not against either agent's own claim.

**This is a single run (n=1 per cell), not a statistically robust study.** Treat it as a qualitative case study with real, honestly-reported numbers — not a definitive verdict on LSP vs grep in general. See also the [codeintel-plan-symbol-fields.md](../../manifest/designs/codeintel-plan-symbol-fields.md) design doc, which cites external research (ManoMano's Project Aegis benchmark, a Semble Show HN thread) reaching the same "task-dependent, not uniform" conclusion this run reaches independently.

## How to reproduce

Both tasks target `internal/perchengine`/`internal/hubgeometry` in this repo at commit `448e5b25` (after the supervised-daemon stderr fix). Ground truth commands:

```sh
go build -o /tmp/lyx-bench ./cmd/lyx
/tmp/lyx-bench codeintel refs internal/perchengine/engine.go:34:2 --target-dir .      # Task 1
/tmp/lyx-bench codeintel assert-no-callers internal/hubgeometry/hubgeometry.go:107:6 --target-dir .  # Task 2
```

Each condition was a fresh `general-purpose` subagent (Claude Sonnet 5) given only the task description and either a mandate or a prohibition on using `lyx codeintel`; neither was told the other condition existed or given any hint toward the answer. Full agent prompts are reproduced in the "Task" sections below.

## Task 1 — interface dispatch: `perchengine.Burler.Run`

**Question:** find every real, live production call site (excluding tests, the interface declaration, and the compile-time `var _ Burler = ...` assertion) where code actually invokes `.Run(...)` on a value of the `Burler` interface type.

**Why this is hard for naive text search:** `Run` is one of the most overloaded method names in this codebase — a repo-wide `grep -rn "\.Run(" --include="*.go" . | grep -v _test.go` returns **26 hits** across completely unrelated types (`cmd.Run()`, `sh.Run(spec)`, `c.engine.Run(...)`, `builderengine.Run(...)`, and more), with zero textual signal distinguishing the one that dispatches through `Burler` from the other 25.

**Ground truth:** exactly one site, `internal/perchengine/adapter.go:35` (`result, err := a.burler.Run(...)`, where `a.burler` is declared `Burler` on `burlerAdapter`).

| Condition | Correct? | Tool calls | Tokens | Wall-clock |
|---|---|---|---|---|
| codeintel required | ✅ `adapter.go:35` | 7 | 38,476 | 79.6 s |
| grep only | ✅ `adapter.go:35` | 5 | 40,792 | 38.8 s |

**What actually happened, honestly:** neither agent took the naive "grep for `.Run(`" path I designed the task to punish. Both independently searched for the distinctive **type name** `Burler` first (14 raw hits for the codeintel agent's `refs` on the method; a handful for the grep agent's `grep -rn Burler`), then manually verified each candidate's static type by reading the surrounding code — the same core strategy either way. The codeintel agent surfaced a real, worth-noting tool limitation along the way: `lyx codeintel refs` on the interface method **conflated** the true interface-dispatch site with `internal/burlercli/run.go:186`'s unrelated `c.engine.Run(...)` call on a concrete `*burlerengine.Engine` — gopls' references for an interface method apparently include every method satisfying that signature, not just calls through the interface value. Both agents had to manually rule that candidate out by checking the receiver's declared type; codeintel's tool did not do this disambiguation for them.

**Result:** grep-only was faster and used fewer tool calls here, at comparable token cost. Codeintel's semantic guarantee (a result is provably a real dispatch, not a name coincidence) didn't translate into a measured efficiency win on this specific task, because a careful grep-based agent reasoning about types manually was just as correct and cheaper. This directly cuts against a simple "LSP always wins on interface dispatch" narrative — it wins on *certainty*, not necessarily on *cost*, when the agent on the other side is skilled rather than naive.

## Task 2 — rename safety with a same-named function in multiple packages: `hubgeometry.Resolve`

**Question:** before renaming `hubgeometry.Resolve` (`internal/hubgeometry/hubgeometry.go:107`), find every real caller of *this specific function* — not `yamlengine.Resolve` or `modelspec.Registry.Resolve`, two unrelated functions elsewhere in the repo that share the bare name `Resolve`.

**Why this is hard for naive text search:** a repo-wide `grep -rn "\bResolve(" --include="*.go" . | grep -v _test.go` returns **46 hits**, of which **11 are false positives** — comments, and real calls to the other two unrelated `Resolve` functions.

**Ground truth:** exactly 31 real callers (verified independently by both `lyx codeintel refs` and `lyx codeintel assert-no-callers`, which agreed exactly). Full list omitted here for space; both agents' lists matched it item-for-item.

| Condition | Correct? | Tool calls | Tokens | Wall-clock |
|---|---|---|---|---|
| codeintel required | ⚠️ data correct, headline miscounted | 4 | 32,515 | 38.3 s |
| grep only | ✅ 31/31, plus a bonus scoping note | 7 | 52,388 | 114.0 s |

**What actually happened, honestly:** the codeintel agent's underlying data was fully correct — its own table lists all 31 sites — but its prose summary stated "Total real call sites: 30", a self-acknowledged table-formatting slip (it flagged the mismatch itself: "table numbering above has a slot mismatch"), not a tool or methodology failure. The grep agent avoided the naive trap entirely: instead of grepping the bare `Resolve(` (which is what produced this task's 46-hits/11-false-positives premise), it first checked for dot-imports/aliases of `hubgeometry` (finding none), then grepped for the fully-qualified `hubgeometry.Resolve(` directly — which is unambiguous by construction in idiomatic, non-dot-imported Go. It got a fully correct 31/31 list, and additionally flagged a genuinely useful scoping question neither the task nor the codeintel agent raised: `internal/lyxtest/lyxtest.go` is a test-support library file (not `_test.go`), so it's ambiguous whether it should count under a stricter "no test code" reading — a real judgment call the grep agent surfaced and the codeintel agent didn't.

**Result:** codeintel was clearly more efficient for equally-correct final data — roughly half the tool calls, a third of the wall-clock, and ~40% fewer tokens. This is the one clean, honest win in this run, and it came from the task the grep agent could still solve well once it stopped searching for the bare name — the cost gap, not a correctness gap.

## Combined totals

| Condition | Tool calls | Tokens | Wall-clock |
|---|---|---|---|
| codeintel required (both tasks) | 11 | 70,991 | 117.9 s |
| grep only (both tasks) | 12 | 93,180 | 152.8 s |
| **Delta** | **-1 (-8%)** | **-22,189 (-24%)** | **-34.9 s (-23%)** |

## Honest takeaways

- **No dramatic, universal win.** Both conditions produced fully correct final data on both tasks in this run. This matches the external research already cited in [codeintel-plan-symbol-fields.md](../../manifest/designs/codeintel-plan-symbol-fields.md): LSP-backed navigation helps, but it's task-dependent, not a uniform multiplier — and it does not help at all against a *skilled* text-search agent that avoids naive traps, only against a naive one.
- **The real, measured effect was cost, not correctness** — a ~20-25% reduction in tokens and wall-clock for equally-correct answers, concentrated entirely in Task 2. Task 1 showed no codeintel advantage at all; grep-only was faster there.
- **A genuine tool limitation surfaced, not papered over:** `lyx codeintel refs` on an interface method returns callers of *every* method satisfying that signature, not just calls through the interface value — a caller still has to manually verify each candidate's static type. This is worth fixing or documenting, not quietly ignoring.
- **Self-reported summaries can drift from the underlying data even when the data is right** — the codeintel agent's "30" vs. its own correct 31-row table is a small but real reminder to check an agent's listed data, not just its stated headline number, when grading or consuming this kind of report.
- Small sample, single run, two hand-picked tasks. A larger, repeated benchmark (matching this doc's own [board-performance.md](board-performance.md) convention of dated, repeatable measurement blocks) would be needed before treating any of these deltas as durable.
