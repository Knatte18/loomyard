# Benchmarks: scout (LSP) vs grep on hard symbol-resolution tasks

Compares `lyx scout` (LSP-backed, semantic) against grep/rg-based text search on three tasks chosen specifically to be hard for text search: two Go interface-dispatch call sites (no textual link between the interface method and its implementation) of increasing difficulty, and a same-named-function-in-multiple-packages rename-safety check. Every task was run twice — once by an agent required to use `lyx scout`, once by an agent forbidden from using it or any other LSP tool — as two independent, fresh, general-purpose subagents per condition, with no shared context between any run and no knowledge of the other condition's existence. Ground truth for every task was verified by hand (via `lyx scout refs`/`assert-no-callers` plus manual code reading) before any agent was dispatched, so grading is against a fixed, independently-known-correct answer, not against either agent's own claim.

Tasks 1 and 2 were run first (Round 1). Task 3 was added afterward (Round 2) specifically because Round 1's results were a fairly modest win for scout — both hand-picked "hard" cases turned out to have a distinctive textual anchor (a rare type name, a package-qualified function name) that let a skilled grep-based agent route around the ambiguity entirely. Task 3 was deliberately designed to remove that escape hatch: an unexported interface with a maximally generic name and method set (`clock { Now(); Sleep() }`), copy-pasted near-identically across three unrelated packages, with no package-qualification possible since it's unexported.

**This is a single run (n=1 per cell), not a statistically robust study.** Treat it as a qualitative case study with real, honestly-reported numbers — not a definitive verdict on LSP vs grep in general. See also the [scout-plan-symbol-fields.md](../../manifest/designs/scout-plan-symbol-fields.md) design doc, which cites external research (ManoMano's Project Aegis benchmark, a Semble Show HN thread) reaching the same "task-dependent, not uniform" conclusion this run reaches independently.

## How to reproduce

Both tasks target `internal/perchengine`/`internal/hubgeometry` in this repo at commit `448e5b25` (after the supervised-daemon stderr fix). Ground truth commands:

```sh
go build -o /tmp/lyx-bench ./cmd/lyx
/tmp/lyx-bench codeintel refs internal/perchengine/engine.go:34:2 --target-dir .      # Task 1
/tmp/lyx-bench codeintel assert-no-callers internal/hubgeometry/hubgeometry.go:107:6 --target-dir .  # Task 2
/tmp/lyx-bench codeintel refs internal/builderengine/poll.go:177:2 --target-dir .    # Task 3, Now()
/tmp/lyx-bench codeintel refs internal/builderengine/poll.go:178:2 --target-dir .    # Task 3, Sleep()
```

(At commit `448e5b25` the CLI subcommand was still `codeintel`, pre-dating the `scout` rename — use `codeintel`, not `scout`, when checking out that commit to reproduce.)

Each condition was a fresh `general-purpose` subagent (Claude Sonnet 5) given only the task description and either a mandate or a prohibition on using `lyx scout`; neither was told the other condition existed or given any hint toward the answer. Full agent prompts are reproduced in the "Task" sections below.

## Task 1 — interface dispatch: `perchengine.Burler.Run`

**Question:** find every real, live production call site (excluding tests, the interface declaration, and the compile-time `var _ Burler = ...` assertion) where code actually invokes `.Run(...)` on a value of the `Burler` interface type.

**Why this is hard for naive text search:** `Run` is one of the most overloaded method names in this codebase — a repo-wide `grep -rn "\.Run(" --include="*.go" . | grep -v _test.go` returns **26 hits** across completely unrelated types (`cmd.Run()`, `sh.Run(spec)`, `c.engine.Run(...)`, `builderengine.Run(...)`, and more), with zero textual signal distinguishing the one that dispatches through `Burler` from the other 25.

**Ground truth:** exactly one site, `internal/perchengine/adapter.go:35` (`result, err := a.burler.Run(...)`, where `a.burler` is declared `Burler` on `burlerAdapter`).

| Condition | Correct? | Tool calls | Tokens | Wall-clock |
|---|---|---|---|---|
| scout required | ✅ `adapter.go:35` | 7 | 38,476 | 79.6 s |
| grep only | ✅ `adapter.go:35` | 5 | 40,792 | 38.8 s |

**What actually happened, honestly:** neither agent took the naive "grep for `.Run(`" path I designed the task to punish. Both independently searched for the distinctive **type name** `Burler` first (14 raw hits for the scout agent's `refs` on the method; a handful for the grep agent's `grep -rn Burler`), then manually verified each candidate's static type by reading the surrounding code — the same core strategy either way. The scout agent surfaced a real, worth-noting tool limitation along the way: `lyx scout refs` on the interface method **conflated** the true interface-dispatch site with `internal/burlercli/run.go:186`'s unrelated `c.engine.Run(...)` call on a concrete `*burlerengine.Engine` — gopls' references for an interface method apparently include every method satisfying that signature, not just calls through the interface value. Both agents had to manually rule that candidate out by checking the receiver's declared type; scout's tool did not do this disambiguation for them.

**Result:** grep-only was faster and used fewer tool calls here, at comparable token cost. Scout's semantic guarantee (a result is provably a real dispatch, not a name coincidence) didn't translate into a measured efficiency win on this specific task, because a careful grep-based agent reasoning about types manually was just as correct and cheaper. This directly cuts against a simple "LSP always wins on interface dispatch" narrative — it wins on *certainty*, not necessarily on *cost*, when the agent on the other side is skilled rather than naive.

## Task 2 — rename safety with a same-named function in multiple packages: `hubgeometry.Resolve`

**Question:** before renaming `hubgeometry.Resolve` (`internal/hubgeometry/hubgeometry.go:107`), find every real caller of *this specific function* — not `yamlengine.Resolve` or `modelspec.Registry.Resolve`, two unrelated functions elsewhere in the repo that share the bare name `Resolve`.

**Why this is hard for naive text search:** a repo-wide `grep -rn "\bResolve(" --include="*.go" . | grep -v _test.go` returns **46 hits**, of which **11 are false positives** — comments, and real calls to the other two unrelated `Resolve` functions.

**Ground truth:** exactly 31 real callers (verified independently by both `lyx scout refs` and `lyx scout assert-no-callers`, which agreed exactly). Full list omitted here for space; both agents' lists matched it item-for-item.

| Condition | Correct? | Tool calls | Tokens | Wall-clock |
|---|---|---|---|---|
| scout required | ⚠️ data correct, headline miscounted | 4 | 32,515 | 38.3 s |
| grep only | ✅ 31/31, plus a bonus scoping note | 7 | 52,388 | 114.0 s |

**What actually happened, honestly:** the scout agent's underlying data was fully correct — its own table lists all 31 sites — but its prose summary stated "Total real call sites: 30", a self-acknowledged table-formatting slip (it flagged the mismatch itself: "table numbering above has a slot mismatch"), not a tool or methodology failure. The grep agent avoided the naive trap entirely: instead of grepping the bare `Resolve(` (which is what produced this task's 46-hits/11-false-positives premise), it first checked for dot-imports/aliases of `hubgeometry` (finding none), then grepped for the fully-qualified `hubgeometry.Resolve(` directly — which is unambiguous by construction in idiomatic, non-dot-imported Go. It got a fully correct 31/31 list, and additionally flagged a genuinely useful scoping question neither the task nor the scout agent raised: `internal/lyxtest/lyxtest.go` is a test-support library file (not `_test.go`), so it's ambiguous whether it should count under a stricter "no test code" reading — a real judgment call the grep agent surfaced and the scout agent didn't.

**Result:** scout was clearly more efficient for equally-correct final data — roughly half the tool calls, a third of the wall-clock, and ~40% fewer tokens. This is the one clean, honest win in this run, and it came from the task the grep agent could still solve well once it stopped searching for the bare name — the cost gap, not a correctness gap.

## Task 3 (Round 2) — no distinctive anchor at all: `builderengine.clock`

**Question:** `internal/builderengine/poll.go` declares a local, unexported interface `clock { Now() time.Time; Sleep(d time.Duration) }`. Find every real production call site *within `internal/builderengine` only* where `.Now()`/`.Sleep()` is invoked polymorphically on a value of this specific interface — excluding the concrete `realClock` implementation's own method bodies, any direct `time.Now()`/`time.Sleep()` stdlib call, and (critically) call sites belonging to two other, unrelated, structurally-identical `clock` interfaces independently declared in `internal/shuttleengine` and `internal/websterengine`.

**Why this is harder than Tasks 1-2:** `clock`, `Now`, and `Sleep` are about as generic as Go identifiers get, and — unlike Task 2's `hubgeometry.Resolve` — the interface is unexported, so there is no package-qualified form to grep for at all (`clk.Now()` inside `builderengine` is textually identical to `clk.Now()` inside `shuttleengine` or `websterengine`, since all three packages independently chose the same parameter name too). No distinctive word exists anywhere in this task for either condition to search for.

**Ground truth:** exactly 3 sites, all inside `PollUntilTerminal` in `poll.go`: line 203 (`clk.Now()`), line 213 (`clk.Now()`), line 216 (`clk.Sleep(pollTick)`).

| Condition | Correct? | Tool calls | Tokens | Wall-clock |
|---|---|---|---|---|
| scout required | ✅ 3/3 | 6 | 35,691 | 66.5 s |
| grep only | ✅ 3/3 | 4 | 33,737 | 36.2 s |

**What actually happened, honestly — and this is the most important result in this document:** `lyx scout refs` on the interface method, queried from `builderengine/poll.go`, returned **~30 hits for `Now()` and ~8 for `Sleep()`**, and the large majority were not from `builderengine` at all — they were real call sites belonging to `shuttleengine`'s and `websterengine`'s own separate, unrelated `clock` interfaces (plus their `_test.go` files). This is not a bug in this repo's wrapper; it is gopls' documented behavior for references on an interface method — because Go interfaces are satisfied structurally, gopls conservatively includes every method matching that name+signature across every structurally-compatible interface in the whole workspace, not just the one actually queried. **The response still carried `"resolution":"complete"`** — the trust marker this codebase's own CLI help text and `scoutengine/refs.go` documentation promise means "a caller does not need to cross-check it with grep or re-verify individual candidates." On this task, that promise was false: both conditions' agents had to manually filter the raw tool output by file path to recover the correct 3-item answer, which is exactly the re-verification the marker exists to make unnecessary.

Grep, meanwhile, got a *free, structural* advantage here that has nothing to do with text-matching precision: a `grep` invocation naturally scopes to the directory/files you point it at, so `grep -rn ... internal/builderengine/` never saw the other two packages' identical-looking interfaces in the first place. The scout agent had no equivalent scoping lever at the time this round ran — `lyx scout refs` had no `--within <dir>` flag, so it got the whole workspace's structurally-matching hits by default and had to filter after the fact.

**Result:** grep-only won on all three metrics — fewer tool calls, fewer tokens, less than half the wall-clock — while both conditions still reached the fully correct answer. This is the clearest reversal in this document: the exact "no textual anchor" scenario this task was designed to make grep fail at is precisely where `scout`'s own workspace-wide interface-reference behavior became a liability, not an advantage.

**Fixed same day, commit after this benchmark:** `refs`, `definition`, and `assert-no-callers` all gained a `--within <dir>` flag that filters results to references whose file lies within `<dir>`, applied before `assert-no-callers`' own `--except` check. Re-running this task's exact query with `--within internal/builderengine` drops the raw hit count for the `Now()` position from 32 (including the interface declaration itself) to the correct 3, and `assert-no-callers` on the same position goes from 31 false-positive "violations" to the 2 genuine ones. This closes the gap this task exposed for any *future* query that already knows its intended package scope — it does not retroactively change this round's recorded numbers above, which reflect the tool as it existed when the agents ran.

## Combined totals

| Condition | Tool calls | Tokens | Wall-clock |
|---|---|---|---|
| scout required (all 3 tasks) | 17 | 106,682 | 184.4 s |
| grep only (all 3 tasks) | 16 | 126,917 | 189.0 s |
| **Delta** | **+1 (+6%, scout used more)** | **-20,235 (-16%)** | **-4.6 s (-2%, ≈ a wash)** |

Adding Task 3 substantially narrows the Round 1 picture: scout's wall-clock advantage shrinks from -23% to essentially zero, and it now uses *more* tool calls in aggregate than grep, not fewer. The token advantage survives (-16%) but is smaller than Round 1 alone suggested (-24%).

## Honest takeaways

- **No dramatic, universal win, and it got weaker, not stronger, as the tasks got harder.** All three conditions produced fully correct final data on all three tasks except one prose miscount (Task 2, data itself correct). This matches the external research already cited in [scout-plan-symbol-fields.md](../../manifest/designs/scout-plan-symbol-fields.md): LSP-backed navigation helps, but it's task-dependent, not a uniform multiplier — and a *skilled* text-search agent that avoids naive traps closes most of the gap, and on Task 3, reverses it.
- **The most important finding is the trust-marker mismatch, not the win/loss tally.** `"resolution":"complete"` was present on Task 3's raw `refs` output even though the majority of that output was cross-package noise requiring manual re-verification — exactly the failure mode the marker exists to rule out. This is a real, actionable gap in `scoutengine`/`scoutcli`, not a benchmark artifact: either the marker's promise needs to be narrowed (e.g. it should mean "every result shown is a genuine reference," not "no further filtering is ever needed"), or `refs`/`assert-no-callers` need a way to scope a query to one package/directory so a caller isn't forced to filter workspace-wide interface-method noise by hand every time.
- **Grep's directory-scoping is a real, structural advantage scout currently has no equivalent for.** This is worth a deliberate design response (a `--within <dir>` flag, or equivalent), not just a documentation caveat.
- **Self-reported summaries can drift from the underlying data even when the data is right** — the scout agent's "30" vs. its own correct 31-row table in Task 2 is a small but real reminder to check an agent's listed data, not just its stated headline number, when grading or consuming this kind of report.
- Small sample, single run, three hand-picked tasks. A larger, repeated benchmark (matching this doc's own [board-performance.md](board-performance.md) convention of dated, repeatable measurement blocks) would be needed before treating any of these deltas as durable.
