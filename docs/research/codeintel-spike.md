# Structured Go reference/call-graph lookup — spike findings

**Task:** `codeintel-spike`. **Date:** 2026-07-17. **Machine:** Linux x86_64, 12 cores,
30GiB RAM. **Toolchain:** `go version go1.26.0 linux/amd64`, `golang.org/x/tools/gopls
v0.23.0` (installed live via `go install golang.org/x/tools/gopls@latest` — the network
fetch succeeded, so no gopls arm degraded to docs-only characterization). Target module:
this repo, `github.com/Knatte18/loomyard`, ~47 `internal/` packages plus `cmd/`/`tools/`.

**Verdict up front:**

- **Direct references / call hierarchy: ADOPT** — `go/packages`+`go/types` in-process is
  cheap (sub-second warm-up, sub-5ms steady-state, no separate process) and, for the
  `refs` mechanism (type-checker `Uses`/`Defs` resolution), had **zero false negatives
  and zero false positives** across all 5 benchmark categories. The `callers` (call
  hierarchy) prototype in this harness has one **real, disqualifying-per-rubric bug** —
  it misses every call to a generic function invoked with explicit type arguments (100%
  false-negative rate on `state.ReadJSON`) — but the fix is well-understood and narrow
  (resolve callees via `TypesInfo.Uses` on the identifier, the way `refs` already does,
  rather than pattern-matching `*ast.CallExpr.Fun`'s outer AST shape). **A follow-up
  production implementation must build call-hierarchy the same `Uses`/`Defs`-resolved
  way `refs` does, not the way this prototype's `callers.go` does.**
- **Transitive impact (callgraph): DEFER.** CHA and RTA are unusable on this repo — both
  over-approximate a niche method's transitive callers by two to three orders of
  magnitude (thousands of functions for a target with 1–25 direct callers). VTA is far
  more precise and **sound on the hand-verified anchor** (misses no real caller) but
  still carries bounded, explainable false positives from this repo's shared
  CLI-command-wrapper pattern (`internal/clihelp.WrapRun`), which conflates sibling
  commands' entry points into the transitive set. Direct call-hierarchy is the sweet
  spot; transitive analysis is not worth building now — revisit if a future consumer
  specifically needs multi-hop impact and can afford a root-set redesign to cut the
  wrapper-conflation noise.
- **CC-native LSP tool: characterized, not measured live** — architectural mismatch
  recorded below; not a candidate for the orchestrator-driven use case regardless of its
  precision, since it is an interactive-LLM-chooses-to-call-it capability, not one the
  Go orchestrator can invoke deterministically.

## Method

Every mechanism was driven through the throwaway harness at `tools/codeintel-poc/`
(`go run ./tools/codeintel-poc -mode=<mode> -symbol=<spec> ...`; see `main.go`'s
`-help` text for the full flag/spec surface). Each symbol's `refs`/`callers` run
recorded a **warm-up** (one-time module load, `packages.Load` with `Tests: false`) and
**steady-state** (`n=5` repeated queries against the already-loaded packages, in the
same process) separately, per the run-scoped warm-host model (see below). `gopls-refs`
(held-open LSP subprocess) recorded warm-up as spawn+`initialize`+first query and
steady-state as `n-1` further queries over the same connection; `gopls-cli-refs` (fresh
`gopls references ...` process per call) recorded every call as effectively-warm-up
since the CLI form never persists state between invocations. `callgraph` recorded SSA-build
time and per-algorithm analysis time separately, for `-algo=cha|rta|vta`. Raw JSON output
for every run above is under `.scratch/codeintel/` (gitignored, not part of this commit,
per Shared Decision `measurement-artifacts-to-scratch`); the tables below are the
distilled result.

**Ground truth** for the precision table was established by hand: `grep`-ing every
non-test call site of each benchmark symbol across the module and manually excluding
false grep matches (e.g. a doc-comment mentioning the symbol's name), then diffing that
hand-built set's *count* against the harness's reported count (the harness always
returned the exact position list too, spot-checked per symbol, not just counted).

## Benchmark symbols

Chosen per `_mill/discussion.md` → `benchmark-symbols`; full selection rationale and
fan-in survey in `.scratch/codeintel/symbols.md`.

| # | Category | Symbol |
|---|---|---|
| 1 | Interface satisfaction | `shuttleengine.Engine.Prepare` (interface method) vs. `claudeengine.Claude.Prepare` (its one real implementer) |
| 2 | Generics | `state.WriteJSON[T]` / `state.ReadJSON[T]` |
| 3 | High fan-in plain function | `hubgeometry.Resolve` (25 production references — the actual highest-fan-in hubgeometry function surveyed, beating `Getwd`=21, `ConfigFile`=16) |
| 4 | Method with many call sites | `hubgeometry.Layout.WeftWorktree` (17 production call sites) |
| 5 | Reflection-adjacent / negative case | `output.Err` / `output.Ok` (173 / 49 production call sites; JSON envelope built from `map[string]any` via `encoding/json`'s reflection-based `Marshal`) |

## Cost table (warm-up once-per-run + per-query steady-state, ms)

| mechanism | warm-up (ms, range across symbols) | steady-state median (ms, range) |
|---|---:|---:|
| `go/packages` in-process, `refs` | 489–640 | 0.32–0.66 |
| `go/packages` in-process, `callers` | 490–563 | 2.03–4.29 |
| `gopls` held-open LSP, first query | 175–392 | 3.29–19.0 |
| `gopls` cold CLI, per call | 190–1276 | 201–1765 (every call pays the load — "warm-up" and "steady-state" converge, confirming the discussion's "cold every time" prediction for the CLI form) |
| `callgraph` (SSA build + analysis, CHA/RTA/VTA) | build 190–248 | analysis: CHA 135–149, RTA 87–90, VTA 381–407 (all one-shot per query, no repeat-query steady-state concept for this mode) |

**Both load-bearing numbers land comfortably inside the rubric's bar**: warm-up is a few
hundred milliseconds (well under the rubric's "order of a few seconds" ceiling),
steady-state is sub-5ms for every in-process mode (`refs` sub-millisecond) — both
trivially "effectively free relative to an LLM turn." The gopls-held-open comparison is
similarly cheap but strictly slower than in-process at every step (spawn + LSP framing
overhead), and requires supervising an external subprocess for the run's duration — no
efficiency reason to prefer it over in-process on this repo. The gopls **CLI** form is
the one mechanism that fails the cost bar outright: with no held-open state, every call
pays the full load, ~200ms–1.8s per query — usable for a one-shot check, not for a
run that needs many queries.

## Precision table (per symbol, vs. hand-verified ground truth)

| symbol | mode | ground truth (production call sites) | harness result | false neg | false pos |
|---|---|---:|---:|---:|---:|
| `hubgeometry.Resolve` | refs | 24 calls (+1 def = 25) | 25 | 0 | 0 |
| `hubgeometry.Resolve` | callers | 24 | 24 | 0 | 0 |
| `hubgeometry.Layout.WeftWorktree` | refs | 17 calls (+1 def = 18) | 18 | 0 | 0 |
| `hubgeometry.Layout.WeftWorktree` | callers | 17 | 17 | 0 | 0 |
| `state.WriteJSON` | refs | 6 calls (+1 def = 7) | 7 | 0 | 0 |
| `state.WriteJSON` | callers | 6 | 6 | 0 | 0 |
| `state.ReadJSON` | refs | 6 calls (+1 def = 7) | 7 | 0 | 0 |
| `state.ReadJSON` | callers | 6 | **0** | **6 (100%)** | 0 |
| `output.Err` | refs | 173 calls (+1 def = 174) | 174 | 0 | 0 |
| `output.Err` | callers | 173 | 173 | 0 | 0 |
| `output.Ok` | refs | 49 calls (+1 def = 50) | 50 | 0 | 0 |
| `output.Ok` | callers | 49 | 49 | 0 | 0 |
| `shuttleengine.Engine.Prepare` | refs | 1 call (+1 def = 2), production only | 2 | 0 | 0 |
| `claudeengine.Claude.Prepare` | refs | 0 production calls (+1 def = 1) | 1 | 0 | 0 |

**The one real bug** (`state.ReadJSON`, `callers` mode): `state.ReadJSON`'s type
parameter cannot be inferred from its arguments (`path, lockPath string`; `T` only
appears in the return type), so every real call site uses explicit instantiation syntax
— `state.ReadJSON[RunState](path, lockPath)`. `callers.go`'s `resolveCallee` only
type-switches `call.Fun` on `*ast.Ident` / `*ast.SelectorExpr`; a generic instantiation's
`call.Fun` is instead an `*ast.IndexExpr` (single type arg) wrapping one of those, which
the switch's `default` case silently drops, resolving to no callee at all. `refs` mode
is unaffected because it walks `TypesInfo.Uses`/`Defs` directly (the type checker
already resolved the identifier inside the `IndexExpr` regardless of the wrapping AST
shape) — this is exactly why the recommended production fix is "build call-hierarchy on
`Uses`/`Defs` like `refs` does," not "special-case `IndexExpr` in the AST walk": the
`Uses`/`Defs` approach is categorically immune to this whole class of AST-shape bug.

**Interface satisfaction — zero production false negatives, but a real latent gap
demonstrated by test code.** No production code in this repo calls
`claudeengine.Claude.Prepare` directly by name — every real call dispatches through the
`shuttleengine.Engine` interface value (`claudeengine.New()` is boxed into the interface
field immediately at every construction site: `internal/shuttlecli/cli.go:103`,
`internal/buildercli/cli.go:204`, `internal/burlercli/cli.go:121`,
`internal/perchcli/cli.go:140`), so querying `refs` on the interface method's
`types.Object` correctly finds the one real dispatch site and nothing is missed. But
`claudeengine`'s own **test files** call the concrete method directly
(`c := New(); c.Prepare(...)`, e.g. `internal/shuttleengine/claudeengine/prepare_test.go:32`)
— a `types.Object` distinct from the interface method's, so a `refs` query on
`Engine.Prepare` would **not** find those call sites even if the harness loaded test
files (it doesn't — `packages.Load` runs with `Tests: false`, a batch-1 harness
scope decision, not a fundamental `go/packages` limitation). This is the real,
structural interface-dispatch precision risk the rubric warns about: if a future card's
edit called a concrete implementation directly instead of through the interface,
`refs`-on-the-interface-method would silently miss it. It happens not to bite on this
repo's *production* code today, but a production implementation should query **both**
the interface method and its known implementers' methods for a `find all callers of this
interface method` request, not the interface method alone.

**`gopls` finds more than the in-process arm for the same reason**: `gopls`'s workspace
indexes test files by default (its `references` results for `weftworktree`/`writejson`/
`readjson`/`outputerr` were consistently 2–4 entries higher than the in-process `refs`
count — the delta is test-file call sites), and (per the interface case above) `gopls`
unifies an interface method's references with its implementers' references into one
set — querying either `Engine.Prepare` or `Claude.Prepare` returned the identical
11-entry set (2 production + 9 test-file concrete-dispatch calls). This is a genuine
precision **advantage** for `gopls` on the interface case, traded against needing an
external binary and being slower per-query than in-process (cost table above).
**Methodological footnote:** `gopls-cli-refs`'s reported count is consistently exactly
one less than `gopls-refs`'s for the same symbol (e.g. 24 vs 25 for `Resolve`) — the CLI
form defaults `includeDeclaration` to `false` where this harness's LSP client explicitly
requests `true`; not a precision difference, a request-parameter difference.

**Reflection-adjacent case, honest negative result:** `output.Err`/`Ok` were chosen to
stress static analysis at a reflection boundary (`encoding/json.Marshal` on a
`map[string]any`), but the actual *call sites* to `Ok`/`Err` themselves are ordinary,
statically-resolvable function calls — the reflection happens inside `Marshal` on the
map's runtime values, not on the call expression the harness resolves. Result: **0 false
negatives/positives**, an honest confirmation that this particular reflection-adjacent
shape does not, in fact, stress this class of tool — the risk reflection poses to static
analysis is to *value/field-level* tracking (e.g. `reflect.ValueOf(x).FieldByName(...)`),
not to *call-site* resolution, and this repo has no examples of the former on a
convenient benchmark symbol.

## CHA/RTA/VTA divergence

**Callgraph roots** (shared across `rta`/`vta`, `cha` needs none — see
`_mill/discussion.md` → `transitive-impact-in-scope`): `cmd/lyx`'s `main.main`
(`cmd/lyx/main.go:38`), every loaded package's synthetic `init` (which itself calls each
package's explicit `init#N` functions), and any `TestMain` (none present — `Tests:
false`). This is `callgraph.go`'s existing `seedRoots`, unmodified.

| symbol | CHA callers | RTA callers | VTA callers | CHA/VTA ratio | RTA/VTA ratio |
|---|---:|---:|---:|---:|---:|
| `claudeengine.Claude.Prepare` | 6,759 | 3,803 | 45 | 150× | 85× |
| `hubgeometry.Layout.WeftWorktree` | 6,761 | 3,805 | 54 | 125× | 70× |
| `hubgeometry.WeftHostSlug` (anchor, below) | 6,759 | 3,803 | 30 | 225× | 127× |

CHA and RTA are effectively unusable as a ripple-impact signal on this repo: both report
transitive-caller sets in the thousands **regardless of the target** (all three targets
above land within ±3 functions of each other on both CHA and RTA, despite having 1, 17,
and 1 direct callers respectively) — the transitive closure has already saturated most
of the program's reachable call graph a few hops out, which any CLI binary with a shared
command-dispatch spine is prone to. VTA is dramatically tighter (30–54, tracking each
target's actual local structure) and cheap (comparable analysis time to CHA/RTA, all
under half a second including SSA build).

### Small-symbol soundness anchor: `hubgeometry.WeftHostSlug`

Chosen as the small, shallow (2–3 hop) case: `WeftHostSlug` has exactly one direct call
site (`internal/warpengine/prune.go:127`, inside `(*Worktree).Prune`), `Prune` has
exactly one direct call site (`internal/warpcli/warp.go:430`, the `pruneCmd`'s `RunE`
closure — the 6th `cobra.Command` literal in `warp.go`'s `Command()`, hence VTA's own
name for it, `warpcli.Command$6`), and that closure is wired through the shared
`clihelp.WrapRun` → `RunRoot` → `Execute` path every module's `RunCLI` uses, up to
`cmd/lyx.main`. Hand-traced **true chain** (verified against
`internal/clihelp/exec.go`, where `RunRoot` calls `cmd.ExecuteContext(ctx)` directly —
cobra's own `ExecuteContext` → `Execute` → `ExecuteC` → `execute` delegation chain is
therefore a real, exercised link here, not an artifact):

```
WeftHostSlug → Prune → warpcli.runPruneWithFlag → warpcli.Command$6 →
clihelp.WrapRun$1 → (*cobra.Command).execute → (*cobra.Command).ExecuteC →
(*cobra.Command).Execute → (*cobra.Command).ExecuteContext → clihelp.RunRoot →
clihelp.Execute → warpcli.RunCLI → cmd/lyx.main
```

VTA's reported 30-entry set **contains every link of this 12-entry true chain — zero
missed real callers, confirming soundness** — but pads it with 18 extra entries: the
sibling modules' own `RunCLI` functions (`boardcli.RunCLI`, `buildercli.RunCLI`,
`burlercli.RunCLI`, `idecli.RunCLI`, `initcli.RunInit`, `muxcli.RunCLI`,
`perchcli.RunCLI`, `selfreportcli.RunCLI`, `shuttlecli.RunCLI`, `weftcli.RunCLI` — 10
functions) plus `configcli`'s own internal call fan-out (`configcli.Command$1`,
`configcli.RunCLI`, `configcli.dispatch`, `configcli.editOne`, `configcli.menu`,
`configcli.runConfig`, `configcli.runConfig$1`, `configcli.setModule` — 8 functions).
None of these 18 functions' code
paths can actually reach `warpcli.runPruneWithFlag` — they are **false positives**
caused by every module wrapping its own commands through the same `clihelp.WrapRun`
closure shape (`func(io.Writer, []string) int`), which VTA's type-based abstraction
cannot fully disambiguate by closure identity once several different closures share
that exact signature. This is the "bounded, explainable false positives" the rubric
tolerates — explainable here down to the exact repo pattern causing it — but it is
real, present even in VTA (the most precise of the three), and is what keeps the
callgraph sub-verdict at Defer rather than Adopt: an implementer using VTA's
transitive result on this repo would need to manually discount every unrelated
sibling-module entry, which undermines "trust it instead of grep" for the transitive
case specifically (direct `refs`/`callers` carry no such caveat).

12 (true chain) + 18 (sibling-module false positives) = the full 30-entry
`transitive_callers` set in `.scratch/codeintel/weftHostSlug-callgraph-vta.json`.

## Run-scoped warm-host model

Confirmed exactly as designed in `_mill/discussion.md` → `warm-host-model`: the
in-process `go/packages` arm needs **no separate process at all** — "warm" is simply the
`[]*packages.Package` slice the harness's own `main` process holds in memory after
`loadPackages` returns; a real orchestrator run (`lyx builder run` / a card's
implementer session) would load once at run start and query many times across the
run's cards, tearing down naturally when the run process exits. The `gopls`-held-open
arm is the same shape one level up — a run-supervised child process, spawned once,
queried many times, torn down at run end — never a machine-wide daemon. The `gopls`
**CLI** form (a fresh process per query) is deliberately *not* the warm path; its cost
numbers above confirm the discussion's prediction that it is cold every time and
therefore the wrong shape for a run that needs more than one query.

## CC-native LSP tool: architectural mismatch

Per Card 6 (`chore(codeintel-poc): add throwaway .lsp.json + characterize CC-native
LSP`), enabling Claude Code's native LSP tool requires `ENABLE_LSP_TOOL=1` set *before*
an interactive session starts — this mill-go implementer session could not toggle it on
itself mid-session (confirmed via `CLAUDECODE=1`/`CLAUDE_CODE_CHILD_SESSION=1` env,
`ENABLE_LSP_TOOL` unset), so per `_mill/discussion.md` → `cc-native-lsp-mismatch`'s
Accepted-outcome note, this arm is a **docs-only characterization**, not a live
measurement — an accepted, non-blocking spike outcome. The wiring itself (repo-root
`.lsp.json` pointing `gopls` at stdio transport, per the recipe below) is in place and
`gopls` is confirmed installed and working via the other two arms, so the recipe is
runnable by an operator with a fresh session.

**The recipe:** (1) `go install golang.org/x/tools/gopls@latest` so `gopls` resolves on
`$PATH`; (2) a repo-root `.lsp.json` of the shape committed in Card 6; (3) launch Claude
Code with `ENABLE_LSP_TOOL=1` in the process environment before the session starts; (4)
prompt the model to use the LSP tool — it is an **LLM-invoked** capability, not one the
harness or an orchestrator can call directly.

**Why this doesn't compete with the other two mechanisms regardless of precision**:
loomyard's agent-execution model is the **file contract** — the Go orchestrator invokes
a capability, gets a bounded structured result, and folds it into a digest, all before
the LLM turn that consumes it starts. Claude Code's native LSP tool is the opposite
shape: an **interactive-LLM-chooses-to-call-it** capability, invoked mid-turn at the
model's discretion, with no deterministic, orchestrator-driven call the way
`builder`/`webster`/`burler` need. It answers a different question (an interactive aid
available *to* the model during its own turn) than this spike's question (a
deterministic, pre-computed impact digest handed *to* the model). This is why it is
measured only as a baseline, never a lead candidate, per the discussion's own framing.

## How-to recipe (adopt-now path: `go/packages`+`go/types` in-process references)

Verified live during this spike (Card 7 step (b), the `refs` runs tabulated above).
Minimal, runnable shape a follow-up production Go verb can lift directly — see
`tools/codeintel-poc/gopackages.go` on this task branch's history (archive tag
`codeintel-spike`) for the full, tested version this recipe is extracted from:

```go
import (
    "go/token"
    "go/types"
    "golang.org/x/tools/go/packages"
)

// Load once per run (the warm-up tax measured above), hold pkgs in memory for
// the rest of the run.
cfg := &packages.Config{
    Dir:  moduleRoot,
    Mode: packages.NeedName | packages.NeedFiles | packages.NeedSyntax |
        packages.NeedTypes | packages.NeedTypesInfo | packages.NeedDeps |
        packages.NeedImports | packages.NeedModule,
    // Tests: true if a future consumer needs call sites inside _test.go files
    // too — see the interface-satisfaction finding above for why this matters.
}
pkgs, err := packages.Load(cfg, "./...")
// packages.PrintErrors(pkgs) > 0 => treat as a load failure, not a partial result.

// Resolve a symbol to its types.Object (package-level func/type/var, or a
// method — for a method on an INTERFACE type, look up on the named type
// itself, not types.NewPointer(named); pointer-to-interface has no method
// set at all).

// Then, per query (the steady-state cost measured above), walk every loaded
// package's already-computed type info — no re-parsing, no re-type-checking:
for _, pkg := range pkgs {
    for ident, use := range pkg.TypesInfo.Uses {
        if use == targetObj {
            pos := pkg.Fset.Position(ident.Pos()) // file:line:col, ready to hand to the LLM
        }
    }
}
```

**Do not** reuse this task's `callers.go` (syntactic `*ast.CallExpr` scan) for a
production call-hierarchy verb — build it on `TypesInfo.Uses`/`Defs` the same way
`refs` above does (filter to `Uses` entries whose enclosing declaration is a
`*ast.FuncDecl`/`*ast.FuncLit`), which sidesteps the generic-instantiation bug found in
this spike entirely rather than patching around it.

## Caveats

One target repo, one machine, one run per arm (`n=5` steady-state repeats per in-process
query, `n=3` for `gopls-refs`, `n=2` for `gopls-cli-refs` — enough to distinguish
warm-up from steady-state, not a statistically rigorous benchmark; numbers throughout
are order-of-magnitude, matching `docs/benchmarks/` house style). 5 benchmark symbols,
hand-picked to stress specific static-analysis edge cases rather than sampled — the
generics false-negative and the interface latent-gap findings are exactly the kind of
result deliberate stress-case selection exists to surface, but a different symbol set
could surface different edge cases this spike didn't hit. The callgraph section
hand-verified soundness on **one** small anchor, not every symbol — CHA/RTA's numbers
are reported as-is (thousands of callers) without a matching hand-enumeration, since
that scale is exactly what makes hand-enumeration intractable for anything but the
smallest case (the reason the discussion's method treats VTA as the relative reference
instead). No wall-clock comparison against the LLM-driven grep/Read baseline this spike
exists to replace is claimed — that would need a separate, real card-implementation
comparison, out of scope for a feasibility measurement.
