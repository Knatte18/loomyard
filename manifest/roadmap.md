# Roadmap: Loomyard

Loomyard replaces mill/millhouse (Python) with a Go orchestration layer, built as self-contained modules landed one at a time.
See [docs/overview.md](../docs/overview.md#principles) for the design principles.
This file is a numbered list of what's planned, what's committed-to- but-unscheduled, and what's shipped — for the detailed design of anything not yet built, see its doc under [designs/](designs/).
See Maintenance below for how the numbering works.

## Planned

Committed to, in this order, next — grouped into sub-categories below for readability; the order between categories is still the build order, top to bottom.

### Shed recipe: declarative producer lists

A 2026-08-21 discussion concluded `internal/loomshed`'s hardcoded `[]shedengine.ProducerDef` Go literal (`loomshed.go:137-151`) should become a declarative recipe instead — see [designs/shed-recipe.md](designs/shed-recipe.md) for the full design. Four separable pieces, in dependency order:

1. **Shed recipe: engine registry** — a name → constructor mapping for every existing `ShedProducer` type, shared adapters (`SingleLLMProducer`, `Bouncer`, `shedadapters: Burler-round producer`, `Webster`, `Preflight`, `Publish`, `Finalize`) and loom-specific types alike (`Loom-Preflight`, `Batchifier`, `DiscussionValidate`, `PlanValidate`) — the foundational piece the recipe loader below resolves `Engine` names against. Restricted to names already implementing `shedengine.ShedProducer`, never arbitrary Go modules — every existing row already satisfies the interface, so this costs nothing.
   See [designs/shed-recipe.md](designs/shed-recipe.md).

1. **Shed recipe: loader/builder** — reads a declarative recipe file (`{Name, Engine, Config, OnDone, OnStuck, MaxBounces}` per row), resolves `Engine` names via the registry above, merges each row's static `Config` with caller-supplied geometry (never contained in the recipe itself — resolved once, centrally, by whichever CLI entry point invokes the builder), and assembles the `[]shedengine.ProducerDef` list `shedengine.Shed` already consumes unchanged. Depends on the engine-registry item above.
   See [designs/shed-recipe.md](designs/shed-recipe.md).

1. **Shed-setup validity checker** — a standalone tool inspecting an assembled `OnDone`/`OnStuck` producer graph for blind gates: unreachable rows, unintended cross-wiring. Needed independent of the recipe work above — `shedengine.validate()` has never enforced this for `OnDone` (only `OnStuck` gets a same-`Segment` check, and the recipe work above drops `Segment` entirely, already a no-op when left unset today). Can land before or independent of the other three items here.
   See [designs/shed-recipe.md](designs/shed-recipe.md).

1. **loom: convert to a Shed recipe** — replace `internal/loomshed`'s hardcoded `[]shedengine.ProducerDef` Go literal with an actual recipe file, using the engine registry and loader built above — the mechanism's first real consumer and proof it works. Depends on all three items above. Converts the list as it stands (including the still-stubbed `*-Write`/`*-Review` rows) — does not itself require the `loom: real LLM producers` group below to land first, and precedes it here so those five tasks author their rows directly in recipe form rather than as a Go literal that then needs converting.
   See [designs/shed-recipe.md](designs/shed-recipe.md).

### loom: real LLM producers

What "loom: write and wire in the real LLM producers" split into — one prompt/rubric per task, each independently reviewable. The only items in this initiative touching LLM-prompt content — the "Shed flattening" group (`shedadapters: Burler-round producer`, `Bouncer`) this used to wait on has shipped, see Done below. The three review-producer tasks depend on those two shipped items — both landed, all three are unblocked; `Discussion-Write`/`Plan-Write` don't depend on either and could in principle land in any order, but stay grouped here for continuity with the original split. Sequenced after the "Shed recipe" group above so these five tasks write their rows directly as recipe entries.

1. **loom: Discussion-Write producer** — replace the `Discussion-Write` stub with a real `SingleLLMProducer` around the already-built prompt (`loom-template-discussion.md`).
   See [designs/loom.md](designs/loom.md#the-phase-machine--a-flat-producer-list-no-predefined-slots).

1. **loom: Discussion-Review producer** — write `Discussion-Review`'s missing "what to check" rubric half (the "what not to flag" half already exists) as the rubric for a new `Discussion-Bouncer` instance, instantiating the shipped `Bouncer` producer with it. The rubric must also cover the Bouncer's seed-call focus-setting pass (see the shipped `Bouncer` item's own rubric-coverage note), not only post-round judgment.
   Replace the `Discussion-Review` stub with a `Discussion-Bouncer`/`Discussion-Burler` segment: `Discussion-Bouncer` (an instance of the shipped `Bouncer: the generic review-gate producer`) is the segment's entry point — its seed call sets initial focus, then every later call judges a round. `Discussion-Burler` (an instance of the shipped `shedadapters: Burler-round producer`) runs one A-review→B-fix round and always hands back to `Discussion-Bouncer` (`Stuck`, `OnStuck: Discussion-Bouncer`), never advancing on its own. `Discussion-Bouncer`'s `OnStuck: Discussion-Burler` covers both the seed call and a rejection; its `OnDone` (approved) exits the segment. Both rows share `Segment: "Discussion-Review"` — the segment's shared name is what the rest of loom's docs/status-display keep referring to as "Discussion-Review," unchanged from today's outward framing even though it is now two rows, not one opaque stub row — and physical list position no longer implies anything about this flow (see `shedengine: per-producer bounce budget + explicit OnDone routing` above). This `Bouncer`+`Burler` wiring is hand-rolled directly in this task, same as `Plan-Review`/`Webster-Review` below each hand-roll their own — no shared module or abstraction wraps the pair (see the `CLAUDE.md` terminology note on "perch," the folk name for this wiring shape).
   Depends on the shipped `shedadapters: Burler-round producer` and `Bouncer` producers — both landed, this item is unblocked.
   See [designs/loom.md](designs/loom.md#discussion-producer-detail--validation-checks-and-review-rubric).

1. **loom: Plan-Write producer** — replace the `Plan-Write` stub with a real `SingleLLMProducer` around the already-built prompt (`loom-template-plan.md`).
   `Plan-Sweep` stays a stub (see Someday below) — this task's `Plan-Write` must treat `Plan-Sweep`'s empty stub output as "no quarry inventory available yet," not as an error.
   See [designs/loom.md](designs/loom.md#the-phase-machine--a-flat-producer-list-no-predefined-slots).

1. **loom: Plan-Review producer** — write `Plan-Review`'s rubric from scratch (does not exist today; `loom-plan-spec.md` is a structural format spec, not review judgment criteria) as the rubric for a new `Plan-Bouncer` instance (same hand-wiring pattern as `Discussion-Review producer` above), covering the seed-call focus pass too.
   Replace the `Plan-Review` stub with a `Plan-Bouncer`/`Plan-Burler` segment, same shape as `Discussion-Review producer` above: `Plan-Bouncer` is the entry point, `Segment: "Plan-Review"`, `OnStuck: Plan-Burler` from both the seed call and on rejection, `OnDone` exits the segment.
   Depends on the shipped `shedadapters: Burler-round producer` and `Bouncer` producers — both landed, this item is unblocked.
   See [designs/loom.md](designs/loom.md#the-phase-machine--a-flat-producer-list-no-predefined-slots).

1. **loom: Webster-Review producer** — write `Webster-Review`'s rubric from scratch (same gap, same reason as `Plan-Review`) as the rubric for a new `Webster-Bouncer` instance (same hand-wiring pattern), covering the seed-call focus pass too.
   Replace the `Webster-Review` stub with a `Webster-Bouncer`/`Webster-Burler` segment: `Webster-Bouncer` is the entry point, `Segment: "Webster-Review"`, `OnStuck: Webster-Burler` from both the seed call and on rejection, `OnDone` exits the segment — same shape as the other two review-producer items above, against the full diff rather than a single artifact.
   Depends on the shipped `shedadapters: Burler-round producer` and `Bouncer` producers — both landed, this item is unblocked.
   See [designs/loom.md](designs/loom.md#the-phase-machine--a-flat-producer-list-no-predefined-slots).

## Someday

Committed to eventually — will be done — but not scheduled next.
No build order is implied between these items.

1. **worktree spawn/teardown as Shed producers** — today, starting a task means three independent, manually-sequenced steps: (1) call `lyx fabric` to create the worktree (warp+weft paired), (2) inside it, run `lyx loom run` (or `lyx run`), (3) once loom finishes and branches are merged into the parent, independently call `lyx fabric` again to tear the worktree down safely. Worktree creation and teardown could instead be their own `ShedProducer` rows (e.g. bookending `loom`'s own producer list, or a small wrapper list around it), so the whole task lifecycle — create, run, merge, destroy — is one driven `Shed` run instead of a human manually bridging three separate CLI invocations.
   A 2026-08-21 discussion also raised that `fabric`'s worktree creation currently stays deliberately outside `_launchers`/`_board` wiring (a decision made to protect the Fabric illusion); revisiting that wiring is a likely prerequisite here and needs its own look before this item is scoped further.

1. **VS Code as opt-in per worktree, not spun up by default** — the long-term direction is for `loom`/Loomyard to be mostly CLI/tmux-based, since a VS Code instance per worktree costs real resources and is rarely needed — a 2026-08-21 discussion noted the common case is reviewing the final PR, not watching an agent edit live. The existing anchor-aware `lyx ide` launcher (`internal/ideengine`, see the Done `worktree + ide` item) already solves opening VS Code at the right anchor subdirectory — something the generic "Git Worktree Manager" VS Code extension cannot do, since it only ever opens a worktree's root. The idea is a fourth per-worktree launcher variant (alongside the existing `ide`/`fabric-checkout`/`run<ext>` set — see `internal/fabricengine/launchers.go`) that, instead of opening VS Code as a full editor window, opens VS Code just far enough to run a task that spawns a tmux terminal and `lyx reed attach`es into the `reed` server the worktree's own `run<ext>` launcher already started — VS Code as a terminal-launcher convenience, not a standing editor per worktree.

1. **doctor** — diagnostics command (`lyx doctor`): checks `_lyx/` layout, config parse, board reachability, stale locks.

1. **session sync** — copy Claude `.jsonl` transcripts across machines so `--resume` works elsewhere.

1. **Claude Code plugin packaging** — ship `lyx` as an installable plugin.

1. **reed: cross-worktree columns** — all worktrees in one window, a column per worktree.

1. **reed: daemon → Slack relay** — standalone watchdog + bidirectional Slack relay per worktree.

1. **reed: own-window strand anchoring** — a `display` anchor that spawns a strand into its own switchable tmux window instead of a pane.

1. **Real-Linux validation** — run the sandbox suite and validate every tmux/`/proc` assumption on a real Linux box (built and cross-compiled so far, never executed there).

1. **fabric: Windows path behaviour is unverified after six hardening rounds** — the platform sibling of `Real-Linux validation` above; needs a Windows host to close, not further design.
   See [designs/fabric-windows-verification.md](designs/fabric-windows-verification.md).

1. **raddle** — codeguide's woven-in successor;
   parallel-regeneration design exists;
   folds into `Finalize`'s own contract rather than a separate producer — `Shed` has no slots for it to occupy.
   See [designs/raddle.md](designs/raddle.md).

1. **webster: parallel card/batch execution** — earlier concurrent-forking-in-one-tree shape rejected twice for git-index-race and mid-flight-visibility hazards (forks sharing one working tree/index).
   A 2026-08-20 discussion landed on a structurally different shape that may dodge both: DAG-independent groups (a batch, possibly one card) each get their own `fabric`-spawned worktree, running the existing `Preflight → Webster → Finalize` row set unchanged, with `Webster`'s `Geometry.PlanDir` (already told, not derived — see the Told-Geometry Invariant) pointed at the source plan and a new batch-filter selecting the one group to run; merge-back reuses `fabric`'s existing merge machinery, not new infrastructure.
   Genuinely own worktree per lane (own git index/HEAD) is what the old shape lacked — grouping granularity (one card vs. several) is orthogonal and does not by itself determine safety.
   Not yet a plan: needs the DAG source (see `scout-backed plan symbol fields` below) and a design writeup reconciling this with `designs/webster-parallel-execution.md`'s still-open questions (typical-plan wave-width evidence, the batchifier/planner change needed to emit groups).
   See [designs/webster-parallel-execution.md](designs/webster-parallel-execution.md) (status banner there is now stale — written for the rejected shape, not this one).

1. **Tenter + Hardener** — behavior-based hardening of a live-substrate module (the archetype: `reed` driving real tmux), on-demand and post-loom, off the `shuttle → burler → shed → loom` spine.
   `Hardener` is the full campaign (`Shed` + `Tenter`, worktree-spawn via `fabric` + safe-merge-back).
   `Tenter`'s review-loop is expected to land as a `Shed` segment — a round producer (behavior-review's own equivalent of `shedadapters: Burler-round producer` being shipped in this task, wrapping whatever Tenter's own round mechanism turns out to be, not `burlerengine`) plus an instance of the shipped `Bouncer: the generic review-gate producer` — the same hand-wired shape `loom`'s own review producers use (folk name "perch," see the `CLAUDE.md` terminology note — `Tenter`'s own segment is "a perch" in that same loose sense, same shape, different round producer inside it), not `Treadle`. Only `Bouncer` is literally reusable code here: `burlerengine`'s own round mechanism is inherently text/diff-specific (Target/Fasit/Rubric over a shuttle session editing text), so `Tenter`'s round producer needs its own from-scratch implementation of the same always-`Stuck`-until-approved contract, not a port of `burlerengine`. This is the second data point (after `loom`) for the `treadleengine` retirement question the Done `Retire perch` item leaves open.
   See [designs/hardener.md](designs/hardener.md) (a DRAFT doc, do not implement from it yet).

1. **warp-visibility: CLAUDE.local.md invisible in the Fabric repo's git history** — `CLAUDE.local.md` via symlink (with a Windows-Developer-Mode note and a copy fallback), so nothing lyx-related shows up in the Fabric repo's own git history.
   The `CONSTRAINTS.md`-equivalent half is **superseded by the Planned `PATTERN.md`** — it lives in `weft`, already invisible to the Fabric repo, so no junction-to-hide-a-constraints-dir is needed;
   only `CLAUDE.local.md` remains.
   See [designs/warp-visibility.md](designs/warp-visibility.md).

1. **reed daemon: foreign-pane self-heal** — extends the **reed: daemon → Slack relay** item.
   Today reed is one-shot, so an operator-split or stray "faux" pane is only reaped on the *next* reed verb;
   the daemon could reconcile on its own.
   Prefer event-driven tmux hooks (`after-split-window`/`window-layout-changed`) over polling;
   gate behind a policy that distinguishes a bug-induced faux pane from an operator's intentional scratch pane.
   Prerequisite: make the reap probe cheaper first (it currently spawns a fresh pwsh + full `Win32_Process` WMI enumeration per poll).

1. **shuttle `Spec`: generic tools-restriction** — meaningless for today's single-session A→B agent;
   cluster reviewers turned out to be fork subagents inside the handler's own session (`useExactTools`), not separate sessions needing their own `settings.json`, so this stays unmotivated rather than blocked on anything.

1. **shuttle `Spec`: per-round provider selector** — today "provider" means whichever engine is wired into the `Runner`;
   a selector field is only needed once a second engine lands (non-Claude engines are not a current priority, per `CLAUDE.md`).
   Scope, if picked up: almost everything lyx spawns (Discussion, Planner, Webster, Burler rounds, the progress-judge) is markdown-instruction + file-contract driven — no skill/slash-command/plugin dependency baked into task content, unlike Millhouse's Claude-Code-specific skill layer — which is what makes a second engine a real swap: it only has to solve spawn/completion-detection/resume, not rewrite prompts.
   The one real trade-off: Burler's cluster-review fan-out (N reviewers as cheap, context-sharing forks via Claude Code's own Agent tool) is a genuine strength, including token cost — a non-Claude engine has no equivalent to fork into, so cluster mode there would mean N full separate sessions instead, costlier by construction.
   Only Burler's default single-reviewer round (no clustering) is unaffected either way.

1. **Bulk-mode clusters + provider-side context caching** — a `burler` cluster round can run *tool-use* or *bulk* (Go concatenates target + fasit + rubric into one blob).
   Bulk is what makes provider-side context caching (e.g. Gemini's explicit cache) pay off, and only if modelled as one shared prefix + N distinct suffixes, never N full prompts.

1. **semantic-index** — semantic search over docstrings/comments (Enzyme-inspired: catalysts + embeddings + temporal decay), to find code by concept rather than literal keyword.
   Genuinely speculative, not yet designed in depth.
   See [designs/semantic-index.md](designs/semantic-index.md).

1. **self-report: two-tier friction capture** — loom's per-phase design means no single LLM session has full-run context the way Millhouse's self-report assumes; splits into Go-detected structural anomalies plus per-phase friction notes, aggregated for one reflection agent at natural end points.
   See [designs/self-report.md](designs/self-report.md).

1. **board: curation/triage automation** — the GitHub-issue-intake and periodic-triage workflow originally scoped in `designs/board-weft-storage.md`'s Curation flow section, deferred out of `board: move storage to weft:main`: an automated skill that ingests GitHub issues and extracts a logical next task from the manifest, promoting it via `promote-note` (which already ships as a plain mechanical CLI primitive — this item is the automation layer on top, not the primitive itself).
   See [designs/curation-triage.md](designs/curation-triage.md).

1. **scout-backed plan symbol fields** — `loom-plan-spec.md` deliberately deferred `creates-symbols`/`edits-symbols`/`reads-symbols` fields pending a verified code-intelligence lookup tool; that tool (now `quarry`, an external Go module dependency) and the loom Planner have since shipped, unblocking but not yet scoping this.
   Named prerequisite for `webster: parallel card execution`'s parked DAG scheduler.
   See [designs/scout-plan-symbol-fields.md](designs/scout-plan-symbol-fields.md).

1. **config: repo-wide default + per-worktree override, millhouse `config.local.yaml`-style** — every module's config today resolves only from `<cwd>/_lyx/config/<module>.yaml` (per-worktree, no shared default;
   `fabric.yaml` is the sole exception, anchored at `_board`/weft:main — see the Done `fabric: unified-repo view — slices 7-10` item).
   Add a repo-wide default layer, read from `_board`, with each worktree's own `_lyx/config/<module>.yaml` as an override on top — the same two-layer overlay millhouse's `mill-config.yaml` (hub root) → `.millhouse/config.local.yaml` (local override) already uses.
   Generalizes `fabric.yaml`'s existing `_board` anchor to every module's config, not just fabric's. Not yet designed.

1. **discussion-format / plan-format: classify review findings by kind** — carry a finding-class dimension (`design`, `scope`, `decision`, `consistency`) on review findings, and scope each review stage to what its downstream stage cannot catch better.
   See [designs/review-finding-classification.md](designs/review-finding-classification.md).

1. **fabric: ordinary-monorepo verb surface** — against plain git, `fabric` is still missing `log`, `show`, `branch` (create/list/delete), `tag`, `stash`, `reset` (non-hard), `revert`, `restore`, `rm`/`mv`, `rebase`, `cherry-pick`, and `blame`.
   None blocks `Finalize`/`Hardener` today; scope by actual need when a consumer needs one, never by completing the list for its own sake.
   See the `fabric: merge-conflict primitive` item's audit findings.

1. **fabric: two-sided reset-to-SHA verb** — the post-conclude undo the merge surface deliberately does not ship: `MergeAbort` covers only the uncommitted merge-attempt window, so a landed merge is final at the Fabric layer until a `Fabric`-level reset to a visible (warp) SHA exists, resolving the paired weft SHA through the correspondence index and routing both resets through the destruction gate.
   See the `internal/fabricengine` package documentation's merge section.

1. **fabric: surface merge-in-progress in `lyx fabric status`** — `MergeInProgress` ships as Go API only; folding it into the `status` verb's output is a small follow-up.

1. **loom: build `Plan-Sweep` for real** — stays a stub past the split-out `loom: Plan-Write producer` item above; deferred because quarry-backed work is low-priority project-wide right now and this is the only row in the initiative that touches quarry — see the Someday `scout-backed plan symbol fields` item below.
   Mechanical quarry inventory over the approved `decision-record.md`, feeding `Plan-Write`; spec in `designs/loom.md#plan-sweep-detail--the-quarry-inventory-spec`.
   Partial building blocks: quarry's reference-lookup and symbol-lookup APIs exist as an external dependency, but no ready-made "inventory" function — needs new composition, not a new engine.

1. **finalize: the discrepancy-document conflict shape** — `finalize.md` originally sketched a second Fabric-to-Finalize conflict artifact, a precomputed "discrepancy document" for a divergence Fabric cannot express as a git conflict.
   Only the ordinary-git-conflict shape shipped; the document shape is not built.
   The existing `PullResult.PatternResidue` is the same shape and already exists for the rewrite case — answer this once, for both, when `Shed`/`loom` exist to consume it.

## Done

1. **Retire perch** — deleted `internal/perchengine`, `internal/perchcli`, and the `lyx perch run|pause` CLI verb outright, together with every perch-only surface they anchored: `hubgeom.PerchGeometry`, `standalonegeom.PerchGeometry`, `configreg`'s `perch` config module, `shedadapters.PerchProducer`, and the `perch-suite` sandbox scheme.
   `loom` never called the module (only stubs), so nothing active depended on it; the replacement is the hand-wired `Bouncer`+`Burler` pair each review-producer task builds directly (see the `CLAUDE.md` terminology note on "perch," the folk name for that pair).
   `internal/treadleengine` deliberately stays, reserved for a possible future `Tenter` consumer — its own retirement is a separate call made once `Tenter` lands.

1. **Bouncer: the generic review-gate producer** — shipped the generic `Bouncer` producer in `internal/shedadapters`: its four `Call` modes (seed, re-bounce, judge, replay) told apart by on-disk artifacts alone, its three own file contracts, the exported `ResolveRound` helper both halves of a segment share, and the two generic stencil templates.
   See the `internal/shedadapters` package documentation and [designs/shed.md](designs/shed.md#engine-adapters--a-thin-shared-seam-not-one-per-producer).

1. **preflight: split into two Shed rows — a generic one, and loom's own** — `loomengine.Preflight`'s single function bundling the orchestrator-agnostic tier-1/tier-2 checks with loom's own check-4 seed coherence split into two `ShedProducer` rows.
   Row 1, `Preflight`, now lives in `internal/preflightshed` as a told-name `ShedProducer` over `internal/preflight.Check`, reusable verbatim by a second product's producer list.
   Row 2, loom's own, is `internal/loomengine.CheckSeed` over told paths, driven as a second row named `Loom-Preflight`.
   The `check3BlocksSeed` short-circuit is gone: `Shed`'s own sequencing already provides what it hand-rolled, since a `Stuck` row 1 bounces or blocks and `Shed` never advances to row 2 at all.
   Seed-check coverage moved from Tier 2 to Tier 1.
   See the `internal/preflightshed` and `internal/loomengine` package documentation.

1. **shedengine: per-producer bounce budget + explicit `OnDone` routing** — replaces `Run`'s single run-wide `bouncesRemaining` counter with a per-producer, episode-scoped budget derived from `Status.History`, and replaces `Done`'s implicit sequential-next routing with an explicit `OnDone` field with no positional fallback — the producer list is now pure storage plus display order, with zero routing meaning of its own.
   See the `internal/shedengine` package documentation and [designs/shed.md](designs/shed.md#the-shed-loop--exact-mechanics).

1. **loom: session bootstrap** — `lyx loom run` (alias `lyx run`), the entry point that makes the phase machine actually reachable, shipped with all four verbs: `run` (the bootstrap), `drive` (the no-tmux foreground escape hatch), `status` (one-shot and `--watch`), and `pause`.
   `run` seeds the status file and commits it weft-side before the driver ever spawns — the ordering that makes loom's own first Preflight precondition row pass immediately rather than blocking on the seed's own dirt — then spawns the detached driver and waits on a handshake for it to take the run lock, so a re-entrant invocation ensures substrate and attaches rather than double-spawning.
   The pair-creating fabric verb now writes and commits a parent-branch provenance record (`_lyx/fabric/origin.json`) at pair-creation time, which `run` reads rather than infers, and the per-worktree launcher set gained a third script, `run<ext>`, alongside the existing `ide`/`fabric-checkout` scripts.
   See [designs/loom.md](designs/loom.md#entry-point--the-session-bootstrap) and the `internal/loomcli` package documentation.

1. **landing: Publish + Finalize producers** — two general `ShedProducer`s (see `designs/shed.md`), not loom-specific: shared by reference with both `loom`'s and the Someday `Hardener`'s producer lists, never `Shed`-special-cased. `Publish` opens a pull request when the parent branch requires one and returns `stuck` (never `done`) while it awaits review, so `Shed` cannot advance past it and merge seconds later; `Finalize` always syncs against the parent through the shared `internal/mergeresolve` engine, then merges the task branch back, with Raddle regeneration folded into that same merge critical section rather than a separate step.
   See the `internal/landingshed` and `internal/mergeresolve` package documentation.

1. **fabric: merge-conflict primitive** — Fabric's merge/conflict lifecycle: `MergeIn`/`Merge`/`MergeContinue`/`MergeAbort`/`MergeInProgress` on `Fabric`, surfaced as `lyx fabric merge-in`/`lyx fabric merge [--squash] [--continue|--abort]`, with git-mirroring exit codes and conflicts reported as unified, worktree-relative paths, never exposing which internal side (warp/weft) produced them.
   See the `internal/fabricengine` package documentation for the merge surface's own mechanism.

1. **producers standalone: invariants and docs** — landed the cross-cutting Told-Geometry Invariant in `CONSTRAINTS.md` (the three-tier producer/orchestrator split), reworded the Cwd Resolution Invariant to state what `Resolve` actually validates, and closed out the design doc per the documentation lifecycle.
   The final consolidation task for this line of work.
   See the [Told-Geometry Invariant](../CONSTRAINTS.md#told-geometry-invariant).

1. **producers standalone: told-geometry foundations** — `planparser` took over the plan-directory path from `loomengine`, `configengine` gained a template fallback so the producer config loaders (shuttle, reed, webster) stop hard-failing on an absent file, and `shuttleengine`/`reedengine`/`tokenvocab` take plain path strings instead of a `*lyxcwd.Location`.
   See the [Told-Geometry Invariant](../CONSTRAINTS.md#told-geometry-invariant).

1. **producers standalone: mid-layer** — `pattern` takes a told anchor path (dropping `internal/lyxcwd` from its leaf allowlist), and the orchestrator preflight lifts out of `loomengine` — alongside the shared `internal/buildinfo`/`internal/standalonestate` foundations and the root-pre-run stencil-seed gate every standalone CLI entry needs — so `Hardener` and future `Shed` products stop having to re-implement any of it.
   See the [Told-Geometry Invariant](../CONSTRAINTS.md#told-geometry-invariant) and the `internal/preflight` package documentation.

1. **producers standalone: producer engines** — `burlerengine`+`perchengine` and `websterengine`+`webstercli` convert to told geometry; Webster also gains its own standalone CLI entry (`--stencils-dir`/`--target-dir`/`--plan-dir`).
   See the [Told-Geometry Invariant](../CONSTRAINTS.md#told-geometry-invariant) and the `internal/hubgeom` and `internal/standalonegeom` package documentation.

1. **producers standalone: the standalone CLI path** — `burlercli`/`perchcli` branch around `lyxcwd.Resolve` and take `--stencils-dir`/`--target-dir`, so `lyx burler run --profile p.yaml` works in a directory that is not a git repository; the optional quarry uniformity pass landed alongside it.
   See the [Told-Geometry Invariant](../CONSTRAINTS.md#told-geometry-invariant).

1. **lyxtest builds real fabric hubs — invert the dependency** — hub fixtures are now built by really cloning (`internal/gitkit`/`internal/hubforge`), never hand-assembled.
   See the `internal/gitkit` and `internal/hubforge` package documentation.

1. **fabric** — unified warp↔weft git-coordination module replacing warp/weft; cut over and old modules deleted.

1. **fabric: unified-repo view — slices 7-10** — the `internal/hubgeometry`-shrink follow-up campaign (GitHub issue #127).
   See [designs/fabric-unified-view.md](designs/fabric-unified-view.md) — the doc survives this task because a later orchestration-layer slice is still open.

1. **fabric: crucible follow-ups — slices 12-15** — closed out the v2 crucible campaign's eight data-loss defects: a single destructive-operation gate (`internal/fabricengine/destroy.go`), a mutation-record envelope on every mutating verb result, and a read-modify-write race fix in the correspondence index.
   Landed with the Fabric Destruction Chokepoint Invariant and the Mutation Record Invariant in `CONSTRAINTS.md`.
   See the `internal/fabricengine` package documentation.

1. **git-native-library: feasibility spike** — evaluated `go-git` as a replacement for `internal/gitexec`'s shell-out plumbing.
   Recommendation: ADOPT-PARTIAL, narrowed further by the `native clients` migration below.
   See the `internal/gitrepo` package documentation.

1. **native clients: migrate `gitrepo` to `go-git` + `selfreportengine`'s `gh`-CLI transport to `go-github`** — `gitrepo`'s read surface migrated to go-git; write/rebase paths stay CLI-bound on measured Windows evidence.
   `selfreportengine`'s `CreateIssue` migrated to `go-github` via the new `internal/githubclient` leaf.
   Landed the GitHub Auth Invariant and the gitrepo Client Boundary Invariant in `CONSTRAINTS.md`.
   See the `internal/gitrepo` and `internal/githubclient` package documentation.

1. **board** — task tracker (storage model superseded by the `board: move storage to weft:main` item below).

1. **board: use `gitrepo` as its git operator** — board's detached sync talks to git exclusively through `gitrepo.Repo`, replacing hand-rolled `gitexec` calls.

1. **board: move storage to `weft:main`** — replaces board's separate remote repo with a reserved `weft:main` branch, a second weft worktree at `<hub>/_board`.
   See the `internal/boardengine` package documentation.

1. **shared infra** — `internal/configengine`, `internal/gitexec`, `internal/lock`, `internal/state`.

1. **gitrepo** — generic, repo-agnostic git primitives, split across two backends — go-git for local object/ref reads, `internal/gitexec` for anything remote-authenticating or working-tree-mutating.
   Consumed by `fabric`.

1. **worktree + ide** — worktree/portal management, VS Code launcher (worktree itself superseded by `warp`).

1. **weft** — companion weft repo, paired warp+weft spawn/teardown (superseded by the `fabric` module).

1. **config TUI** — `lyx config` interactive menu + `reconcile`.

1. **warp** — warp↔weft-coordinated git topology (clone, add/remove, checkout, reconcile, cleanup) (superseded by the `fabric` module).

1. **proc** — cross-OS process spawn.

1. **reed** — tmux overlay + strand bookkeeping + render (renamed from `mux`, no behavior change).

1. **shuttle** — run one LLM agent as an interactive tmux strand over a swappable engine.

1. **burler** — one review+fix round (A-review → B-fix).

1. **perch** — the gate loop: run `burler` rounds until `APPROVED`/`STUCK`.

1. **webster: rewrite for flat card list** — fork-per-card, consumes the flat card-list plan format via `internal/planparser` (sole parser) and `internal/batcher`.
   See the `internal/websterengine` package documentation.

1. **plan-format: flat card list** — a card carries `What:`, five typed file-op fields, and `Depends-on:`; symbol fields wait for quarry.
   See [contracts/specs/loom-plan-spec.md](../contracts/specs/loom-plan-spec.md).

1. **built-in CLI help** — self-documenting `lyx`/`lyx <module>`/`lyx <module> <cmd> --help`.

1. **selfreport** — file Loomyard bugs as GitHub issues (`lyx selfreport create`).

1. **loom: contracts, Preflight, Discussion producer** — the three loom pieces shipped so far (loom as a whole is not done — see the Planned `loom` item).

1. **loom: Planner producer** — reads the discussion decision-record and writes a plan-format flat-card plan; a prompt/profile fed to `shuttle.Run`, not a module.
   No review logic of its own.

1. **dev/test `lyx.exe` separated from production deploy** — a second deploy target (`deploy-dev`/`deploy-dev.cmd`) so review/sandbox tooling never overwrites the stable production binary with an in-progress test build.
   See CONSTRAINTS.md's Dev/Prod Binary Separation invariant.

1. **Treadle: shared round-loop engine, combined with the `perch` rewrite** — generalized `perch`'s round loop into `internal/treadleengine`, a shared engine with a pluggable `RoundRunner` seam; `perch` rewritten onto it in the same task, behavior/CLI unchanged from the outside.
   See the `internal/treadleengine` package documentation.

1. **gitexec: checked entry point + call-site migration** — `internal/gitexec`/`internal/gitrepo` each gained a checked, must-succeed entry point (`Run`/`runChecked`) alongside the original raw form; every call site now uses whichever is correct.
   Landed the gitexec Checked-Call Invariant in `CONSTRAINTS.md`.
   See the `internal/gitexec` package documentation.

1. **`PATTERN.md` — loomyard's own invariants mechanism, wired into every code-touching agent** — a from-scratch equivalent of Millhouse's `CONSTRAINTS.md` (present in this repo only because mill develops loomyard), owned by loomyard instead.
   Supersedes the constraints-hiding half of Someday's `warp-visibility`.
   See the `internal/pattern` package documentation.

1. **Shed: shared outer phase-FSM, no predefined slots** — shipped the skeleton: `internal/shedengine`'s loop, status file, `ShedProducer` interface, and producer-list validation, which `loom` and the eventual `Hardener` are each `Shed` plus their own producer list on top of.
   This is the skeleton only — the three engine adapters (`SingleLLMProducer`, the `perch` adapter, the `Webster` adapter) shipped as their own later task, below.
   Landed the Shed Producer-Seam Invariant in `CONSTRAINTS.md`.
   See the `internal/shedengine` package documentation and [designs/shed.md](designs/shed.md), whose retention past this landing is justified in its own status banner rather than restated here.

1. **Shed's engine adapters — `SingleLLMProducer`, the `perch` adapter, the `Webster` adapter** — shipped three reusable `ShedProducer` implementations in one new package, `internal/shedadapters`, each a thin wrapper over an already-shipped engine.
   See [designs/shed.md](designs/shed.md#engine-adapters--a-thin-shared-seam-not-one-per-producer).

1. **PATTERN directives: move from Go constants to stencil files** — `internal/pattern.Directive`'s three role-keyed directive strings now live as real, directly-editable stencil files instead of Go source, read at call time through `stencilstore.Read`, same as every other producer prompt.
   See the `internal/pattern` package documentation.

1. **loom: phase-machine scaffolding** — `internal/loomshed` carries loom's full 13-row producer list: `Preflight`, `Loom-Preflight`, `Discussion-Validate`, `Plan-Validate`, and `Batchifier` built for real, `Webster` wired in as-is, `Publish` and `Finalize` since built for real by the `landing: Publish + Finalize producers` item above, and the remaining five rows (`Discussion-Write`, `Discussion-Review`, `Plan-Write`, `Plan-Review`, `Webster-Review`) stubbed. (`Plan-Sweep` is not a row — see the Someday `loom: build Plan-Sweep for real` item.)
   loom's status file migrated onto `shedengine.Status`, with `loomshed.Seed` as its production seeder.
   See the `internal/loomshed` package documentation and the [Told-Geometry Invariant](../CONSTRAINTS.md#told-geometry-invariant).

1. **shedadapters: Burler-round producer** — shipped `BurlerProducer`, a new reusable `ShedProducer` in `internal/shedadapters` wrapping one `burlerengine` A-review/B-fix round as a single Shed row.
   It always returns `Stuck` to its segment's `Bouncer`, never `Done`, and resolves its round from disk over the review/fixer-report pair predicate rather than holding an attempt number in memory.
   `burlerengine.Profile.ClusterExclude` shipped alongside it as the per-call cluster-fan trimming knob.
   See the `internal/shedadapters` package documentation.

## Maintenance

- **Numbering is automatic, not manual, and restarts at 1 in each section.**
  Every item is written literally as `1.` in the source — GitHub/CommonMark renders ordered-list items sequentially from the first item in a contiguous list block regardless of the literal digit on the rest,
  and a new `##` heading starts a new block.
  So Planned, Someday, and Done each render as their own 1, 2, 3, … with **zero number edits ever needed** — inserting, removing, or reordering items anywhere just works.
- **Numbers are not stable cross-reference IDs** (the same number exists in all three sections).
  Cross-reference by **bold item name** instead (e.g. "the Planned `board` item," "Someday's `raddle` item") — every reference elsewhere in this file and in `designs/*.md` already does this.
- **Entries are short — a name plus one or two sentences of what/why, never a design writeup.**
  Detail belongs in the entry's own `designs/<name>.md` while the item is Planned or Someday.
  Delete that doc once the module ships (see the [documentation lifecycle](../docs/overview.md#documentation-lifecycle)) — a Done entry instead points at the module's own package documentation, which is where its durable detail lives from then on.
  If an entry keeps growing past a couple of sentences, that is a signal to move the growth into the doc it points to, not to let the entry itself grow.
- Move an item from Planned or Someday to Done, with a link to its module doc if one exists, when it ships — no renumbering needed anywhere.
- Someday items get a `designs/<name>.md` doc when there's real design behind them (`scout-backed plan symbol fields`, `raddle`, `webster: parallel card execution`, `hardener`, `warp-visibility`, `semantic-index` above do);
  trivial ones don't need one until they're promoted to Planned.
- This file is the single home for everything not scheduled, whether firmly committed to (`warp-visibility`, `raddle`) or genuinely speculative (`hardener`, the shuttle `Spec` ideas) — no separate long-term-ideas file.
  Add new speculative ideas directly to Someday.
