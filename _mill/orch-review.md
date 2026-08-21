# Orchestrator review — discussion.md

Reviewed against `main` (unchanged in this worktree — only `_mill/discussion.md`/`status.md` exist so far).

## Citation check

The densest discussion reviewed in this initiative — a table of eleven constructor signatures plus a dozen file/line citations. Verified every one against actual source.

| Claim | Status |
|---|---|
| `internal/loomshed.New` builds the 13-row list as a Go literal at `loomshed.go:137-151` | Correct (verified in an earlier review this session) |
| `internal/batcher/registry.go` — package-global `map[string]Batcher`, `Select(name)` | Correct — `registry.go:14,29` |
| `internal/batcher/identity.go` — `init()` self-registration | Correct — `identity.go:28` |
| `internal/loomshed/seam_enforcement_test.go` — `TestToldGeometryInvariant_AllowlistOnly`, "closest model" for the new allowlist test | Correct, exact test name — `seam_enforcement_test.go:42` |
| `internal/burlercli/run.go:53` — `burlerengine.Profile` buildable as a partial literal from outside the package despite the unexported `clusterLenses` field | Correct — the function builds and returns a `Profile{Target: ..., Fasit: ...}` literal at that location |
| `internal/loomshed/webster.go:41` — `newWebsterProducer(name, anchorPath string, run shedadapters.WebsterRunner, deps websterengine.RunDeps) *websterProducer` | Correct, exact signature match |
| `shedadapters.NewSingleLLMProducer(name string, specs SpecSource, shuttle Shuttle, now func() time.Time)` | Correct, exact — `singlellm.go:49` |
| `shedadapters.NewBouncer(cfg BouncerConfig) (*Bouncer, error)` | Correct, exact — `bouncer.go:81` |
| `shedadapters.NewBurlerProducer(name string, runner BurlerRunner, profile burlerengine.Profile, opts burlerengine.RunOpts, runDir string, now func() time.Time)` | Correct, exact — `burler.go:70` |
| `preflightshed.NewPreflight(name, cwd string) shedengine.ShedProducer` — "already returns the interface" | Correct, exact — `preflight.go:27` |
| `landingshed.NewPublish(deps Deps) (*Publish, error)` / `NewFinalize(deps Deps) (*Finalize, error)` | Correct, exact — `publish.go:60`, `finalize.go:69` |
| `websterengine.RunDeps` / `modelspec.Registry` types exist | Correct — `runlevel.go:104`, `modelspec.go:91` |
| `bouncer.go:330-346`, `413-433` — seed-call and judge-call both do `stencilstore.Read` (template), `stencilstore.Read` (rubric), `stencil.StripLeadingComment`, `stencil.Fill` | Correct — both call sites follow exactly this sequence |
| `bouncer.go:131` — constructor probes the rubric stencil via `stencilstore.Read`, so it can fail on a missing stencil at construction time | Correct |
| `bouncer.go:394` — the literal `"(none)"` | **Off by three lines.** Line 394 is the start of the explanatory comment; the literal itself (`previousLedger := "(none)"`) is at line 397. Same minor citation class as a prior review's `singlellm.go:107` finding — cites the explanation, not the value. Non-blocking. |
| `CONSTRAINTS.md` — Lyxdirs Single-Declarer Invariant, Batcher Registry+Config Invariant, Config Strictness Invariant | Correct — lines 83, 518, 532 respectively |
| `loomshed` imports `shedadapters`, making a `shedadapters → loomshed` import impossible without a cycle | Correct — `loomshed.go:10` |
| `shedengine.ProducerDef{Name, Producer, OnStuck, OnDone, Segment, MaxBounces}`, `ShedProducer.Call(ctx) (Outcome, OutputPointer, error)` | Correct |

Every constructor signature in the eleven-row technical-context table checked out exactly, argument order and types included — a meaningfully higher citation density than any discussion reviewed so far in this initiative, with one line-number miss.

## Design read

**The `Env` decision is the standout piece, and it's honestly framed as the discussion's own addition rather than something the parent design doc already settled.** `manifest/designs/shed-recipe.md` covers geometry exclusion from the recipe but says nothing about non-serializable injected seams — `Shuttle`, `BurlerRunner`, `WebsterRunner`, `websterengine.RunDeps`, the clock, `landingshed.Deps`'s closures. Rather than silently smuggling these into `Config` (which would quietly break the recipe's portability guarantee the parent doc spent real effort establishing) or inventing an unrelated second mechanism, the discussion extends the existing told-geometry channel to also carry live seams, and says outright that this is "the single decision most worth re-examining if the loader task finds it awkward" — an unusually honest confidence calibration for an auto-picked decision.

**The central-table-vs-`init()` decision correctly identifies why the in-repo precedent doesn't transfer, rather than either blindly copying it or blindly rejecting it.** `internal/batcher`'s `init()` self-registration works specifically because every batcher lives in the registry's own package; here the entries span four packages, so the same mechanism would make "which engines exist" depend on link-time blank imports. Copying the *lookup and error* shape while deliberately departing from the *registration* mechanism is the right level of precedent-following — reuse what generalizes, not what doesn't.

**Catching `Stub`'s absence from the roadmap's own enumeration is a real, well-argued correction, not a scope-creep add.** The roadmap says piece 4 "converts the list as it stands (including the still-stubbed rows)" — which is provably impossible without a `Stub` engine name, since five of loom's thirteen rows are still stubs today. Naming this as "an oversight in the enumeration, not a deliberate exclusion" and shipping it anyway is the correct call: shipping only the eleven named engines would silently block piece 4 on a follow-up task nobody had scoped.

**The coverage-guard test is a good example of testing against the actual production stakes rather than the abstract contract.** The registry's entire reason to exist is resolving every one of loom's current rows; without the guard, a row added to `loomshed` between now and piece 4 would silently have no engine, surfacing only as piece 4's own failure. Correctly rejects reflecting over `loomshed.New`'s output (the `ProducerDef` carries no engine name to reflect on) in favor of an explicit, maintained table — the honest acknowledgment that the mapping has to be written down somewhere, not derived.

**The `landingshed.Deps` whole-struct passthrough is consistent with existing precedent rather than a fresh judgment call** — it cites `loomshed.Deps.Landing`'s own existing field-doc reasoning for the same choice, so the registry's `Env.Landing` field isn't a new design decision, it's the same one already made one layer up, carried through.

**Scope discipline holds under a large surface area.** Twelve registry entries, five renamed exports, a new invariant, and a design-doc banner rewrite is a lot of simultaneous surface — but the Out list is precise about what stays untouched: no recipe file format, no loader, no validity checker, no `shedengine` change, no `Segment` behavior change, no rubric/prompt content, no CLI wiring. Each is a real adjacent task this one could have reached into.

One thing worth flagging for the plan stage, not a defect: the `Env` struct as sketched has nine geometry fields plus six injected-seam fields — large for a single struct threaded through twelve constructors. The discussion itself already flags this exact decision as the one most likely to need revisiting once the loader is built against it, so this isn't a missed risk, just worth the plan stage keeping an eye on whether `Env`'s shape holds up once a second caller (a future standalone CLI, or `Hardener`) actually exists to stress it.

## Verdict

Sound. Nothing here should block moving to Plan. One citation to fix if convenient: `bouncer.go:394` should point at line 397 for the `"(none)"` literal itself, not the comment explaining it — not worth a discussion round on its own.
