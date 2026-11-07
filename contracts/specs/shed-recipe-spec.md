# Shed recipe — the declarative producer-list format

> **Status: Contract — pinned.** This doc pins the **recipe format**: the data file that names a `Shed` run's producers and their routing, which `internal/shedbuild` is the sole parser and `internal/shedrecipe`'s registry resolves engine names against.
> A recipe is assembled into the `[]shedengine.ProducerDef` `shedengine.Shed` consumes; `internal/shedcheck` checks the assembled list.
> Kept as a durable Go-to-Go reference doc under `contracts/specs/`, not deleted on landing, per the [documentation lifecycle](../../docs/overview.md#documentation-lifecycle).
> It is not a deployed spec: nothing outside this repository reads a recipe, and no agent ever writes one, so it carries no registry entry in `specs.go` and is never seeded into a target repo.
> The shipped recipes live under `contracts/recipes/`, each read through its own recipe package.

## The idea

`internal/loomrecipe.New()` builds tapestry's `[]shedengine.ProducerDef` by parsing and building a declarative recipe.
The recipe is the data file `contracts/recipes/tapestry-recipe.yaml`, which names, per row, its `Name`, `Engine`, `Config` and routing fields.
The rows are assembled into the `[]shedengine.ProducerDef` `shedengine.Shed` consumes, with no change to `shedengine`.

Most rows are pure `Engine + Config`, the shape a declarative recipe expresses cleanly; `Discussion-Write` is not.
Its table also carries per-run values a static recipe `Config` cannot hold: the task slug and the mode-rules block.
It also takes a model and timeout resolved from the `discussion` role's own config rather than from recipe strings.
That is why it ships as its own `DiscussionWrite` registry entry rather than as a `MultiLLM` row.
The entry reads its table through an injected `shedadapters.TableSource` closure.
The entry runs the lead and its advisors, when the table has any, on `MultiLLMProducer`.
The table is told through `Env.DiscussionTable`.
It carries the row's gates on the lead.
The recipe file names `DiscussionWrite`,
and the built row carries that engine whatever `discussion_advisors` and `discussion_lenses` hold.
`Plan-Write` is the second such row, and it ships as its own `PlanWrite` entry for the same reason.
Its seat table is resolved from the `plan` role's own model-spec and timeout rather than from recipe strings.
Building it needs a `*lymcwd.Location` to build the plan paths.
A loom-specific row need not be reusable to fit the pattern: it needs a name in the same registry as every other row.
A bespoke, single-consumer `Engine` is as valid a registry entry as a widely shared one.

## What's in a recipe row

- **`Name`** — the row's identifier (`shedengine.ProducerDef.Name`).
- **`Engine`** — a name looked up in a registry mapping engine-name strings to `ShedProducer`-constructing functions.
  It is restricted to names implementing `shedengine.ShedProducer`, not any Go module.
  Every row satisfies the interface, shared adapter or loom-specific type alike.
  The Shed Producer-Seam Invariant requires it of anything in a `ProducerDef`.
  The restriction costs nothing and keeps the recipe-builder simple:
  a registry lookup and a fixed-signature constructor call, with no reflection and no handling of arbitrary shapes.
- **`Config`** — the row's static, portable configuration (e.g. which rubric stencil, which prompt template).
  It never contains absolute paths.
  This is what lets one `Engine` (e.g. `MultiLLMProducer`) serve many rows:
  the row-to-row difference lives entirely in `Config`.
  No row field picks a row's engine from a setting.
  The `Implementer` engine does it inside the registry:
  it builds the row with the implementer its `implementer_setting` key's `module.key` setting names from a closed table.
  Every choice builds the same row with the same edges.
- **`OnDone` / `OnStuck`** — the routing fields of `ProducerDef`, carried over with the same semantics.
- **`OnReroute`** — the one row a Stuck verdict that asks for a reroute goes to; empty means the row cannot reroute.
  Its target may sit in another segment,
  and `shedengine`'s validation refuses one naming no row in the list or the row itself.
- **`MaxBounces`** — a static, per-row integer, set once at recipe-authoring time.
  No producer can adjust it at run time, a Bouncer judging its own segment included.
  The cap is a safety backstop against runaway review-fix cycles,
  and a producer that could raise its own ceiling would undermine it.

A recipe row carries an optional `Segment`, mapped straight onto `shedengine.ProducerDef.Segment`.
`shedengine.validate()` enforces that a non-empty `OnStuck` names a producer sharing the bouncing row's `Segment`.
A producer list mixing rows at the empty default with rows at a real segment name would therefore fail validation.
That failure would come at run time rather than at authoring time.
The recipe states each row's segment itself.
`internal/shedcheck` catches illegitimate cross-segment `OnStuck` wiring.
It is needed regardless, since `OnDone` has no segment-style enforcement.

## What's never in a recipe

**Geometry** is absolute paths such as `AnchorPath`, `WorktreeRoot`, `StencilsDir` and `PlanDir`.
It is resolved once, centrally, by whichever caller invokes the recipe-builder:
`lym loom start` for hub mode, a standalone CLI entry point for told mode.
The two mirror the `hubgeom`/`standalonegeom` dual-constructor split.
It is merged with each row's `Config` when the row's `ProducerDef` is constructed.
The recipe itself never names a geometry mode and never contains a path,
so it stays portable across every worktree of the product it describes.
This extends the Told-Geometry Invariant to the recipe layer.

**Live seams**, alongside geometry, are the other thing never in a recipe:
the runners and closures the engines call, such as `shedadapters.Shuttle` and `shedadapters.SeatRunner` (`Env.Seats`).
The injected clock is one too.
None of these can be written in a file: a recipe row names an `Engine` and carries `Config`, and a seam is neither.
They travel in the same told bundle as geometry, `shedrecipe.Env`.
Whichever caller invokes the registry fills it once.
The `Env`-versus-`Config` split is decidable by one rule.
`Env` holds roots, run-wide values and the per-segment settings of `Segments`.
`Segments` is keyed by segment, so a segment's rows share one entry.
Anything per-row is a relative path or scalar in `Config`.
The entry that reads it resolves it against one of those roots.

A `Config` key may select among the seams the told `Env` already carries, by name, without carrying one.
A `Bouncer` row's `commit_seam` key takes one of exactly two literal values, `plan` and `discussion`.
They resolve to `Env.CommitPlan` and `Env.CommitDiscussion` respectively.
Two rules make it safe: an absent key is a legitimate "no seam configured" and leaves the closure nil;
a present key naming a closure the `Env` does not carry is a construction error, not a silent nil.
A nil closure would silently mean "commit nothing," the exact condition the key exists to eliminate.
A `Bouncer` row's optional `skip_seam` key takes one value, `rework-exempt`.
It resolves to `Env.SkipPlanReview`, a closure that tells the Bouncer to approve without a judge.
It follows the same two rules as `commit_seam`: absent leaves the seam nil,
and a present key naming a closure the `Env` does not carry is a construction error.
An unknown value is a construction error naming `rework-exempt`.
A `Bouncer` row's optional `carry_over` key is a scalar naming the row's own review segment, for example `Plan-Review`.
It selects `Env.CarryOver`.
That closure writes and commits the segment's carry-over entry into the decision record.
It follows the same two rules as `commit_seam` and `skip_seam`: absent leaves the seam nil,
and a present key naming a closure the `Env` does not carry is a construction error.
The value is the segment name the entry is filed under, so a recipe test pins it equal to the row's `segment`.
A `Bouncer` row's optional `subject` key takes one of exactly two literal values, `document` and `code`.
It selects the seams that tell the Bouncer what its segment judges.
`document` selects `Env.DocumentSubject`, told the row's resolved artifact paths.
`document` also selects `Env.RecordHead`, told as the settled base.
`code` selects `Env.CodeSubject` and `Env.MergeInOnly`.
Every kind also tells the Bouncer `Env.ReviewInput`, which stays optional.
Every kind also tells it the segment's `AmendMaxBounces` as the amendment allowance.
It follows the same two rules as `commit_seam`.
An absent key tells no subject and keeps the Bouncer on its clear-and-reseed path,
and a present key naming a seam the `Env` does not carry is a construction error naming that seam.
An unknown value is a construction error naming the key, the value and the two legal ones.
A `Bouncer` row's optional boolean `escalate_decisions` key defaults to false.
When true, a judged round whose review holds a `decision`-class finding escalates with cause `decision`.
The arbiter settles that escalation where the board entry and the decision record answer it,
and the run's parent settles it otherwise.
A `BurlerFix` row's optional boolean `decisions_escalated` key (default false) says its segment's Bouncer does so:
the fixer is given no `decision` finding and the row posts no parent notice of its own.
Loom's recipe sets both keys on its Discussion-Review rows and on no other row.

A `MultiLLM` row runs one lead and its advisors as seats.
Its keys are `role`, `segment` and `seats`, and the optional `gates`, `timeout_min`, `interactive` and `tokens`.
Each `seats` element carries `stencil`, `model` and `outputs`, and the optional `inputs`, `skills` and `values`;
`inputs` and `outputs` are worktree-relative paths.
The first `seats` element is the lead and carries no `name`.
Every later element requires a unique `name` starting with a letter.
A seat's `model` is a model-spec string resolved against `Env.Models`, the run-wide registry,
and an unknown alias is a construction error naming the seat.
The row drives `Env.Seats`, and a nil seam is a construction error.

A gate-capable row's `gates` key selects validators by name from a closed vocabulary, in order.
Each element carries its own `attempts` budget (0 means off) and an optional `pass_on_cap`.
An absent key is ungated, while an empty list, a duplicate name, or a negative budget is a construction error.
The vocabulary is `clean`, `discussion`, `plan`, `parts`, `description` and `verify`.
`clean` fails while git reports an uncommitted or untracked path in the worktree.
It requires the absolute `Env.WorktreeRoot`.
`verify` lints the comments, then runs the verify command the wiring tells in `Env.VerifyCommand`, which it requires.
`Env.VerifyTreeScope`, when set, selects the darn form; the round form is the default.
The darn form runs the tree-scoped command it answers for a diff wholly under the configured trees.
For any other diff it runs the impact-scoped command.
`Env.VerifyMergeBase`, when set, is the base of the headroom lint, which runs before the comment lint.
Under the darn form it is the base of both lints, and that form requires it.
A gate element of the `DarnWrite`, `PlanWrite` and `DiscussionWrite` rows also takes the optional `fixer` key:
a `<module>.<key>` setting holding a model-spec, or `""` to turn the fixer off.
Any other row refuses `fixer` as an unknown key.

## The recipe loader/builder

`internal/shedbuild` is the recipe file format's loader and builder, shipped as a single package.
Its production import set is one-way:
it imports `internal/shedrecipe`, `internal/shedengine`, and `internal/shedcheck`,
and none of those three, nor `internal/loomshed` or `internal/loomcli`, imports it back.

**Document shape.** A recipe document decodes straight into the package's `Recipe` type, with no intermediate struct.
It requires a `version`, of which only `1` is accepted.
It also requires a told `entry` producer name, a told `terminals` list, and a `producers` list.
It may also set an optional top-level `amendment_target`, the name of the row a plan amendment routes to;
`Parse` decodes it without checking that it names a row, since routing is `shedengine`'s and `shedcheck`'s.
It may also set a top-level `contract`, empty or `loom`.
It may also set a top-level `verify_source`, empty, `plan` or `tree_scope`;
`Parse` refuses any other value of either, naming the key, the legal values and the way forward.
A row is an engine row or a block row.
An engine row carries `name`, `engine`, `config`, `on_done`, `on_stuck` and `on_reroute`.
It also carries `segment`, `max_bounces` and `entry_gate`.
These are the same fields `shedengine.ProducerDef` itself carries.
A block row carries only `name`, `block`, `block_gate` and `on_done`, and no engine.
`block` names the recipe to run inline, and `block_gate` names the shipped check that gates it.
`Parse` refuses a block row that sets any other key or lacks `block` or `block_gate`.
It also refuses a hand-written `engine: Block`, which is reserved for the row a block becomes once expanded.

**Exported functions.**
`Parse` decodes a byte slice into a `Recipe` and runs every shape check the value must pass before `Build` consumes it.
`Load` reads the recipe file at a told absolute path and delegates to `Parse`.
It is the only function in the package whose own code touches the filesystem.
`Expand` inlines each block row's sub-recipe into its parent, as the expansion paragraph below describes.
`Build` resolves each row's `Engine` name against the `internal/shedrecipe` registry.
It calls the returned constructor with a caller-supplied `shedrecipe.Env`.
It assembles the `[]shedengine.ProducerDef` list `shedengine.Shed` consumes.
`RoutingFrom` projects the routing of an expanded recipe, each row's skip target and block path included.
It builds no engine.
`Build` and `RoutingFrom` take the expanded recipe:
`Build` refuses a block row that was never expanded, and a block row's gate is the check its `block_gate` names.
`Check` forwards an assembled producer list, with a `Recipe`'s own `Entry` and `Terminals`, to `shedcheck.Check`.
It is non-production, for a caller's own authoring-time test suite.

**Expansion.** `Expand` takes a parsed `Recipe` and a told resolver from a recipe name to its bytes.
It parses each block's recipe through `Parse`.
It returns a new `Recipe` in which every block row is followed by the rows of its recipe, under their own names.
The block row becomes an `engine: Block` row.
Its `on_done` is the sub-recipe's `entry`, and its skip target is the block's exit.
The exit is the block row's own `on_done`, or, when that is empty, the exit of the block enclosing it,
and an outermost block row must set `on_done`.
Every inlined row's empty `on_done` becomes the exit of the innermost block enclosing the row.
So does an inlined block row's empty skip target.
Each inlined row carries its block path, the enclosing block rows' names joined by `/`, outermost first.
A sub-recipe's `terminals`, `contract`, `verify_source` and `amendment_target` are not read.
They belong to the run, so a recipe that runs alone may also be a block.
A recipe with no block row is returned unchanged.

**Strictness.** Unknown keys and duplicate keys are errors at both document level and row level,
and every message the decoder produces keeps its own yaml line number.
Every error this package raises after a successful decode names the offending row's zero-based index and `name`.

**The loom contract.** A recipe declaring `contract: loom` is checked by `Build` before any registry lookup.
`Build` refuses one that breaks a clause, naming the clause and its way forward.
The clauses are checked in this order:

1. The recipe holds exactly one record row, an engine of `shedrecipe.LoomRecordEngines`.
2. It holds exactly one build row, an engine of `shedrecipe.LoomBuildEngines`;
   the build segment is the rows a variant owns that turn the record into the change,
   and the contract checks the one row of it that is a build engine.
3. The build row lies after the record row on the `on_done` path walked from `entry`.
   That walk ends at a terminal, an empty `on_done` or a revisited row.
4. A row whose engine is `shedrecipe.LoomFinalizeEngine` exists,
   and every row reached by following `on_done` from it is in `terminals`.
5. `verify_source` is set.

A recipe holding no build row and no `shedrecipe.LoomFinalizeEngine` row is the contract's record-only form:
it writes and reviews the record and lands nothing, so clause 1 is its whole contract.
A recipe without `contract: loom` is not checked and is no loom.

**Validation split.** This package owns file shape and engine-name resolution alone.
It runs no reachability, cycle, blind-gate, dangling-target, or segment analysis of its own.
`shedengine`'s own validation and `internal/shedcheck` already own routing, cycles, and reachability.
A third copy would drift.
The one exception is `Expand`'s block refusals, which only the expansion can see:
a block recipe including itself, an inlined edge naming no row of its recipe, and a segment shared across a block edge.

**Building is not filesystem-free.** Some registry constructors reach disk of their own accord at construction time,
and `Build` passes those effects through rather than suppressing or wrapping them;
`internal/shedbuild/doc.go` is the single site enumerating which constructor produces which effect.

**No on-disk location.** This package defines no on-disk location for recipe files:
no directory constant, no filename convention, no embedded default.
The tapestry recipe's location is settled under "Decisions this piece settled" below.

## Decisions this piece settled

- **On-disk location.** The tapestry recipe ships as an embedded default at `contracts/recipes/tapestry-recipe.yaml`.
  It is read through `shedbuild.Parse` on the embedded bytes, never `shedbuild.Load`.
  The bytes are `contracts/recipes/recipes.go`'s `TapestryRecipe`.
  There is no seeding, no operator override, and no runtime on-disk path.
- **The consumer.** `internal/loomrecipe` is the tapestry recipe's sole consumer.
  It sits above `internal/loomshed` rather than inside it:
  `internal/shedrecipe`'s registry imports `loomshed` for its tapestry-specific constructors,
  so a consumer inside `loomshed` would close a `loomshed` → `shedbuild` → `shedrecipe` → `loomshed` import cycle.
- **Test ownership.** The assembled-graph tests live in `internal/loomrecipe`, not `internal/loomshed`.
  They include the coverage guard driving tapestry's real row list against the registry.
  They also include the sequencing, cancellation and resume tests.

**Unused entry.** Loom reaches its recipe through the embedded bytes, never a told path,
so `shedbuild.Load` has no production caller.
It stays exported and covered as the entry a non-embedded consumer, such as a second recipe-backed product, would use.

## Escalation and the future watchdog

The recipe needs no escalation mechanism of its own:
`OnStuck: ""` halts the run and records a reason (`RunBlocked` + `Status.Reason`).
That signal is generic:
a watchdog process outside the `lym` loop would read the halted or blocked output of `lym loom` as a human does.

## Related

- `internal/shedengine`'s and `internal/shedadapters`' package documentation:
  `Shed`'s generic mechanism, which this layer sits on top of: the loop, the producer contract and the engine adapters.
- `internal/shedbuild`'s and `internal/shedcheck`'s package documentation: the parser and the assembled-list checker.
- `PATTERN-sole-parsers`, `PATTERN-shed-recipe-registry` and `PATTERN-cwd-resolution`:
  the rules that directly shape this format.
  The import seam `internal/shedengine`'s package doc states shapes it too.
