# Roadmap: Loomyard

Loomyard replaces mill/millhouse (Python) with a Go orchestration layer, built as self-contained modules landed one at a time.
See [docs/overview.md](../docs/overview.md#principles) for the design principles.
This file is a numbered list of what's planned, what's committed-to- but-unscheduled, and what's shipped — for the detailed design of anything not yet built, see its doc under [designs/](designs/).
See Maintenance below for how the numbering works.

## Planned

Committed to, in this order, next — grouped into sub-categories below for readability; the order between categories is still the build order, top to bottom.

### Perch → Shed flattening

Priority now — `perch`'s own round-loop (`internal/treadleengine`) gets replaced by two ordinary `Shed` rows per review, not rewritten. The real-LLM-producer work below waits on this group, both because the three review-producer tasks depend on it directly and because it's the higher priority right now regardless.

1. **shedengine: per-producer bounce budget + explicit `OnDone` routing** — replaces `Run`'s single run-wide `bouncesRemaining` counter with a per-producer budget derived from `Status.History` (count prior `Stuck` entries for the same producer name, no new persisted field needed), and adds two new `ProducerDef` fields: `MaxBounces int` (0 = inherit `Shed`'s own default, same convention `Shed.MaxBounces` already uses) and `Segment string` (a grouping label, empty = standalone).
   **`OnDone string`, parallel to the existing `OnStuck`, replaces `Done`'s implicit sequential-next routing entirely — no fallback.** Set → `Done` jumps to the named producer (forward or backward, same freedom `OnStuck` already has). Empty → `Done` here finishes the whole `Shed`, not "because it happens to be the last entry in the list." `run.go`'s `indexAfter` helper and its "is this producer physically last" check are removed outright — the producer list becomes pure storage plus `validate()`'s iteration order plus cosmetic display order, with zero routing meaning of its own. Motivation, surfaced by the Bouncer/Burler segment shape below rather than invented in the abstract: a hybrid where some producers route by physical position and others by an explicit field is harder to read than either pure form, since a reader must first check which mode a given `ProducerDef` is in before trusting list order at all — going fully explicit means one `ProducerDef` read in isolation always tells the whole story.
   `validate()` gains two new rules: `OnDone`, if set, must name an existing producer in the list — the same "must exist" check `OnStuck` already gets, with no same-`Segment` restriction, since crossing out of a segment on approval is the point. `OnStuck`, if set, must name a producer sharing its own `Segment` — turns "these rows strictly belong together" into an enforced invariant rather than a naming convention. Name uniqueness is already global across the whole list (existing `is a duplicate of an earlier entry` rule).
   Migration cost: every row in `loomshed.go`'s existing producer list needs an explicit `OnDone` added (mechanical, one line each) to keep today's linear behavior — cheaper now, while this task already touches `ProducerDef`/`validate()`, than later with more rows in place.
   `designs/shed.md` needs updating in the same commit — it currently documents pure sequential `Done` routing and a single global bounce budget, both superseded here.
   Not loom-specific — `internal/shedengine` is the shared engine both `loom` and the Someday `Hardener` sit on.
   Motivation for the segment/budget half: a review round-loop (see the next two items) is expected to iterate several times as normal operation, unlike a mechanical validate gate's rare bounce — sharing one global budget between the two conflates "something is structurally broken" with "this is the Nth normal round."

1. **shedadapters: Burler-round producer** — a new reusable `ShedProducer` in `internal/shedadapters`, alongside the already-shipped `SingleLLMProducer` and `Webster` adapters. The existing `perch` adapter (`shedadapters.NewPerchProducer`) is NOT a third sibling here — this item and the next (`Bouncer`) are what supersede it; the three loom review-producer tasks below stop calling it, and its fate (kept for other callers, or retired alongside `perch`/`treadleengine` themselves) is exactly the open question the Someday `Bouncer → Perch` item defers. Wraps `internal/burlerengine`'s existing one-round (A-review → B-fix) API directly as a single Shed row, bypassing `perch`/`internal/treadleengine`'s outer round-loop entirely — the loop moves to `Shed`'s own per-segment bounce mechanism (previous item) instead.
   **Always returns `Stuck` with `OnStuck` pointing at its segment's Bouncer, never `Done`** — a round producer has no independent notion of "finished," only the judge does, so `Stuck` here is a pure hand-off signal reusing the existing routing primitive, not an error state; document this explicitly in the producer's own doc comment so a routine hand-off is never mistaken for a real stuck condition. Reached only via an explicit `OnStuck`/`OnDone` jump, never via `Done`-fallthrough from anything (see the Bouncer item's entry-point note below) — its physical position in the producer list carries no routing meaning and can sit wherever reads best.
   **Open risk to resolve during this task, not before:** `treadleengine`'s own doc states its two-attempt retry-on-death/timeout policy and "asking-triage" are `Engine`-level machinery shared by any `RoundRunner`, not part of `burlerengine` itself. Verify whether that machinery already lives in (or can cleanly move to) `burlerengine`'s own one-round API before treating this adapter as a thin wrapper — if not, this task must either reimplement the relevant slice or extract it from `treadleengine` into something both can share.
   `burlerengine`-specific, so not reusable by the Someday `Tenter`'s own round producer (a different domain, behavior/sandbox-based, not text review) — but still not loom-specific: any `Shed`-based module reviewing via `burler` (loom's three segments today) shares this one adapter.
   **Cluster fan-out trimming, driven by the Bouncer's next-round focus file:** `burlerengine.Profile.ClusterFan` today only resolves a fixed, named lens list from `burler.yaml` at profile-build time (`ResolveFan`) into the unexported `clusterLenses []Lens`, with no per-call override — so a round that found nothing via one lens has no way to drop that lens on the next round. This task adds a small, exported way for a caller to inject an explicit lens subset (or an exclusion list) instead of only resolving one named fan, so this adapter can read the Bouncer's structured (not prose) next-round directive and skip lenses that found nothing last round before spawning.

1. **Bouncer: the generic review-gate producer** — this task builds the Bouncer itself, not merely an adapter around it: the judge half of a Burler/Bouncer segment, and unlike the Burler-round producer above, genuinely domain-agnostic — parametrized purely by a rubric stencil path and a report/ledger file-path convention, not by which round producer it gates.
   **Is the segment's entry point, not its exit gate** — sits where the segment's slot is in the pipeline (inherits control from the previous stage's `Done`), so it runs *before* the first round exists, not only after. Two modes, told apart mechanically, never by state passed through `Call(ctx)` (whose signature has no room for it — same file-existence pattern already needed to fix the Discussion-Validate/Plan-Validate findings-discarded-on-Stuck gap): if the round producer's report artifact for the current round does not exist yet, this is the **seed call** — write the initial focus file only (no verdict possible yet) and return `Stuck` with `OnStuck` pointing at the round producer, unconditionally. If the artifact exists, this is the **judge call** — read it plus any prior bouncer report, judge it, write an updated finding-identity ledger and an optional next-round focus file, and return `Done` (approved — `OnDone` set to whatever comes after the segment, never falls through to the round producer) or `Stuck` (rejected — same `OnStuck` bounce as the seed call, another round).
   One constructor, instantiated per segment with its own rubric + paths — same "thin shared seam, not one per producer" shape the already-shipped `SingleLLMProducer`/`Webster` adapters established, and the `perch` adapter also followed before this pair superseded it (see [designs/shed.md](designs/shed.md#engine-adapters--a-thin-shared-seam-not-one-per-producer)).
   Reuse option to resolve during this task: `internal/treadleengine`'s existing exported parsers (`ParseJudgeVerdict`, `ParseHandoff`) are pure functions nothing forbids importing — the Treadle Runner-Seam Invariant only restricts `treadleengine`'s own imports, not who imports its exports. Decide whether to reuse them or write fresh ones.
   **The next-round focus file must be structured, not prose**, unlike treadle's own `PreRoundTargeting` ("unconstrained prose a RoundRunner MAY read or ignore entirely"): when a lens found nothing this round, the Bouncer writes a small, mechanically-parseable directive (e.g. a lens-name exclusion list) alongside its prose ledger, so the round producer's own Go code — not an LLM's discretion — decides whether to drop that lens next round (see `shedadapters: Burler-round producer`'s own cluster-fan-trimming note above). The seed call writes the same structured shape with no exclusions, so the round producer's read path has one format to handle, not two.
   Rubric must cover the seed call's focus-setting too, not only post-round judgment — this is also what makes Crucible's own today-behavior (an orchestrator picks focus before spawning a reviewer round) the working precedent for `Tenter`'s eventual reuse, not merely an analogy.
   Not loom-specific — this is the second of the two pieces the Someday `Tenter` review-loop is expected to reuse verbatim (see the `Tenter + Hardener` Someday item).

### loom: real LLM producers

What "loom: write and wire in the real LLM producers" split into — one prompt/rubric per task, each independently reviewable. Deliberately last: the only items in this initiative touching LLM-prompt content. The three review-producer tasks depend on the whole "Perch → Shed flattening" group above; `Discussion-Write`/`Plan-Write` don't and could in principle land earlier, but stay grouped here for continuity with the original split.

1. **loom: Discussion-Write producer** — replace the `Discussion-Write` stub with a real `SingleLLMProducer` around the already-built prompt (`loom-template-discussion.md`).
   See [designs/loom.md](designs/loom.md#the-phase-machine--a-flat-producer-list-no-predefined-slots).

1. **loom: Discussion-Review producer** — write `Discussion-Review`'s missing "what to check" rubric half (the "what not to flag" half already exists) as the rubric for a new `Discussion-Bouncer` instance (placeholder name — renaming to `Perch` is its own later Someday task, see below), instantiating the `Bouncer` producer above with it. The rubric must also cover the Bouncer's seed-call focus-setting pass (see the `Bouncer` item above), not only post-round judgment.
   Replace the `Discussion-Review` stub with a `Discussion-Bouncer`/`Discussion-Burler` segment: `Discussion-Bouncer` (an instance of `Bouncer: the generic review-gate producer` above) is the segment's entry point — its seed call sets initial focus, then every later call judges a round. `Discussion-Burler` (an instance of `shedadapters: Burler-round producer` above) runs one A-review→B-fix round and always hands back to `Discussion-Bouncer` (`Stuck`, `OnStuck: Discussion-Bouncer`), never advancing on its own. `Discussion-Bouncer`'s `OnStuck: Discussion-Burler` covers both the seed call and a rejection; its `OnDone` (approved) exits the segment. Both rows share `Segment: "Discussion-Review"` — the segment's shared name is what the rest of loom's docs/status-display keep referring to as "Discussion-Review," unchanged from today's outward framing even though it is now two rows, not one opaque `perch`-backed row — and physical list position no longer implies anything about this flow (see `shedengine: per-producer bounce budget + explicit OnDone routing` above).
   Depends on the three "Perch → Shed flattening" items above.
   See [designs/loom.md](designs/loom.md#discussion-producer-detail--validation-checks-and-review-rubric).

1. **loom: Plan-Write producer** — replace the `Plan-Write` stub with a real `SingleLLMProducer` around the already-built prompt (`loom-template-plan.md`).
   `Plan-Sweep` stays a stub (see Someday below) — this task's `Plan-Write` must treat `Plan-Sweep`'s empty stub output as "no scout inventory available yet," not as an error.
   See [designs/loom.md](designs/loom.md#the-phase-machine--a-flat-producer-list-no-predefined-slots).

1. **loom: Plan-Review producer** — write `Plan-Review`'s rubric from scratch (does not exist today; `loom-plan-spec.md` is a structural format spec, not review judgment criteria) as the rubric for a new `Plan-Bouncer` instance (placeholder name, see `Discussion-Review producer` above for the pattern), covering the seed-call focus pass too.
   Replace the `Plan-Review` stub with a `Plan-Bouncer`/`Plan-Burler` segment, same shape as `Discussion-Review producer` above: `Plan-Bouncer` is the entry point, `Segment: "Plan-Review"`, `OnStuck: Plan-Burler` from both the seed call and on rejection, `OnDone` exits the segment.
   Depends on the three "Perch → Shed flattening" items above.
   See [designs/loom.md](designs/loom.md#the-phase-machine--a-flat-producer-list-no-predefined-slots).

1. **loom: Webster-Review producer** — write `Webster-Review`'s rubric from scratch (same gap, same reason as `Plan-Review`) as the rubric for a new `Webster-Bouncer` instance (placeholder name, same pattern), covering the seed-call focus pass too.
   Replace the `Webster-Review` stub with a `Webster-Bouncer`/`Webster-Burler` segment: `Webster-Bouncer` is the entry point, `Segment: "Webster-Review"`, `OnStuck: Webster-Burler` from both the seed call and on rejection, `OnDone` exits the segment — same shape as the other two review-producer items above, against the full diff rather than a single artifact.
   Depends on the three "Perch → Shed flattening" items above.
   See [designs/loom.md](designs/loom.md#the-phase-machine--a-flat-producer-list-no-predefined-slots).

### Loom infrastructure cleanup

1. **preflight: split into two Shed rows — a generic one, and loom's own** — `internal/loomengine.Preflight` today bundles the orchestrator-agnostic tier-1/tier-2 checks (`internal/preflight.Check`, already generic) with a hardcoded check 4 (`_lyx/loom/status.json` presence/readability/coherence, against loom's own `Status`/`Product` shape) into one function, `runCheck4`, inside one `Preflight` Shed row.
   The row mechanism (`Deps.Preflight shedengine.ShedProducer`) is already generic — any `ShedProducer` can back it — but the concrete producer wired in today is not: `Hardener` cannot reuse `loomengine.Preflight` as-is the way it will reuse `Publish`/`Finalize` (see `designs/landing.md`).
   Scope: two separate `ShedProducer` rows instead of one. Row 1, `Preflight`, becomes a genuinely content-free wrapper around `internal/preflight.Check` — no loom-specific parameters at all, reusable by `Hardener` verbatim. Row 2, loom's own (name TBD, e.g. `Loom-Preflight`), carries today's check-4 content (`LoomStatusFile`/`LoomStatusLock`, the `Status`/`Product` shape, `checkCoherence`'s rules) as its own independent producer.
   This is not just code reuse: `shedengine`'s own sequencing already makes today's `check3BlocksSeed` short-circuit unnecessary — a `Stuck` row 1 bounces or blocks and `Shed` never advances to row 2 at all (`internal/shedengine/run.go`'s `Stuck` handling), so row 2 can assume tiers 1–3 already passed rather than re-deriving whether its own failure is a downstream consequence of an earlier one.
   `loomshed`'s producer list grows from 13 rows to 14; all "13-row" references across `loomshed.go`/`stub.go`/`sequence_test.go`/`loomshed_test.go`/`loom.md`/`docs/overview.md` move to "14-row" in the same commit, per the Documentation Lifecycle.
   No wiki depends_on beyond the already-Done `loom: phase-machine scaffolding`.

## Someday

Committed to eventually — will be done — but not scheduled next.
No build order is implied between these items.

1. **Bouncer → Perch: rename, and retire `internal/perchengine`/`internal/treadleengine`** — once the `*-Bouncer` producers (see the Planned `loom: Discussion-Review producer`/`Plan-Review producer`/`Webster-Review producer` items) are built and proven, rename the placeholder `*-Bouncer` producers to whatever the final name is (`Perch` is the leading candidate, per the segment's own outward name already being e.g. "Discussion-Review").
   Retirement, not indefinite coexistence, is the expected outcome: `internal/treadleengine` was built specifically so a second future consumer could reuse its round-loop machinery without duplicating it (see its own package doc); the Someday `Tenter + Hardener` item now expects `Tenter`'s review-loop to land as the same flat `Shed`-segment pattern (its own round producer + an instance of the shared generic Bouncer adapter) instead of a second `Treadle` consumer — so `Treadle` ends up with zero real consumers, not one. Final call on `internal/perchengine`/`internal/treadleengine` and the standalone `lyx perch run|pause` CLI stays deferred to this task regardless, since it should only be made once the new design is proven in practice on both `loom` and (eventually) `Tenter`, not asserted in advance.
   Deliberately not done at the same time as the split-out producer tasks above, to avoid "which Perch do you mean" confusion mid-rewrite.

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

1. **webster: parallel card execution** — worktree-per-card concurrent forking with a DAG;
   explored twice (pre- and during vacation discussion), rejected both times for git-index-race and mid-flight-visibility hazards.
   See [designs/webster-parallel-execution.md](designs/webster-parallel-execution.md).

1. **Tenter + Hardener** — behavior-based hardening of a live-substrate module (the archetype: `reed` driving real tmux), on-demand and post-loom, off the `shuttle → burler → perch → loom` spine.
   `Hardener` is the full campaign (`Shed` + `Tenter`, worktree-spawn via `fabric` + safe-merge-back).
   `Tenter`'s review-loop is expected to land as a `Shed` segment — a round producer (behavior-review's own equivalent of the Planned `shedadapters: Burler-round producer` item, wrapping whatever Tenter's own round mechanism turns out to be, not `burlerengine`) plus an instance of the Planned `Bouncer: the generic review-gate producer` item — the same flattened pattern `loom`'s own review producers use, not `Treadle`. This is the second data point (after `loom`) for the Someday `Bouncer → Perch` item's retirement question below.
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
   The "deferred idea" `scout-redesign.md` already refers to.
   Genuinely speculative, not yet designed in depth.
   See [designs/semantic-index.md](designs/semantic-index.md).

1. **self-report: two-tier friction capture** — loom's per-phase design means no single LLM session has full-run context the way Millhouse's self-report assumes; splits into Go-detected structural anomalies plus per-phase friction notes, aggregated for one reflection agent at natural end points.
   See [designs/self-report.md](designs/self-report.md).

1. **board: curation/triage automation** — the GitHub-issue-intake and periodic-triage workflow originally scoped in `designs/board-weft-storage.md`'s Curation flow section, deferred out of `board: move storage to weft:main`: an automated skill that ingests GitHub issues and extracts a logical next task from the manifest, promoting it via `promote-note` (which already ships as a plain mechanical CLI primitive — this item is the automation layer on top, not the primitive itself).
   See [designs/curation-triage.md](designs/curation-triage.md).

1. **scout-backed plan symbol fields** — `loom-plan-spec.md` deliberately deferred `creates-symbols`/`edits-symbols`/`reads-symbols` fields pending a verified `scout`; both `scout` and the loom Planner have since shipped, unblocking but not yet scoping this.
   Named prerequisite for `webster: parallel card execution`'s parked DAG scheduler.
   See [designs/scout-plan-symbol-fields.md](designs/scout-plan-symbol-fields.md).

1. **config: repo-wide default + per-worktree override, millhouse `config.local.yaml`-style** — every module's config today resolves only from `<cwd>/_lyx/config/<module>.yaml` (per-worktree, no shared default;
   `fabric.yaml` is the sole exception, anchored at `_board`/weft:main — see the Done `fabric: unified-repo view — slices 7-10` item).
   Add a repo-wide default layer, read from `_board`, with each worktree's own `_lyx/config/<module>.yaml` as an override on top — the same two-layer overlay millhouse's `mill-config.yaml` (hub root) → `.millhouse/config.local.yaml` (local override) already uses.
   Generalizes `fabric.yaml`'s existing `_board` anchor to every module's config, not just fabric's. Not yet designed.

1. **discussion-format / plan-format: classify review findings by kind** — carry a finding-class dimension (`design`, `scope`, `decision`, `consistency`) on review findings, and scope each review stage to what its downstream stage cannot catch better.
   See [designs/review-finding-classification.md](designs/review-finding-classification.md).

1. **scout: narrow the `"resolution":"complete"` trust-marker promise, or scope out cross-package interface-method noise** — `docs/benchmarks/scout-vs-grep.md` found a case where `refs` on an interface method returned `"resolution":"complete"` while most hits were real but irrelevant cross-package noise.
   Not yet designed.

1. **fabric: ordinary-monorepo verb surface** — against plain git, `fabric` is still missing `log`, `show`, `branch` (create/list/delete), `tag`, `stash`, `reset` (non-hard), `revert`, `restore`, `rm`/`mv`, `rebase`, `cherry-pick`, and `blame`.
   None blocks `Finalize`/`Hardener` today; scope by actual need when a consumer needs one, never by completing the list for its own sake.
   See the `fabric: merge-conflict primitive` item's audit findings.

1. **fabric: two-sided reset-to-SHA verb** — the post-conclude undo the merge surface deliberately does not ship: `MergeAbort` covers only the uncommitted merge-attempt window, so a landed merge is final at the Fabric layer until a `Fabric`-level reset to a visible (warp) SHA exists, resolving the paired weft SHA through the correspondence index and routing both resets through the destruction gate.
   See the `internal/fabricengine` package documentation's merge section.

1. **fabric: surface merge-in-progress in `lyx fabric status`** — `MergeInProgress` ships as Go API only; folding it into the `status` verb's output is a small follow-up.

1. **loom: build `Plan-Sweep` for real** — stays a stub past the split-out `loom: Plan-Write producer` item above; deferred because `scout`-backed work is low-priority project-wide right now and this is the only row in the initiative that touches `scout` — see the Someday `scout` items below, including the open question of whether `scout` stays part of this repo at all.
   Mechanical scout inventory over the approved `decision-record.md`, feeding `Plan-Write`; spec in `designs/loom.md#plan-sweep-detail--the-scout-inventory-spec`.
   Partial building blocks: `scoutengine.References` and symbol lookup exist, but no ready-made "inventory" function — needs new composition, not a new engine.

1. **finalize: the discrepancy-document conflict shape** — `finalize.md` originally sketched a second Fabric-to-Finalize conflict artifact, a precomputed "discrepancy document" for a divergence Fabric cannot express as a git conflict.
   Only the ordinary-git-conflict shape shipped; the document shape is not built.
   The existing `PullResult.PatternResidue` is the same shape and already exists for the rewrite case — answer this once, for both, when `Shed`/`loom` exist to consume it.

## Done

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

1. **producers standalone: told-geometry foundations** — `planparser` took over the plan-directory path from `loomengine`, `configengine` gained a template fallback so the producer config loaders (shuttle, reed, perch, webster) stop hard-failing on an absent file, and `shuttleengine`/`reedengine`/`tokenvocab` take plain path strings instead of a `*lyxcwd.Location`.
   See the [Told-Geometry Invariant](../CONSTRAINTS.md#told-geometry-invariant).

1. **producers standalone: mid-layer** — `pattern` takes a told anchor path (dropping `internal/lyxcwd` from its leaf allowlist), and the orchestrator preflight lifts out of `loomengine` — alongside the shared `internal/buildinfo`/`internal/standalonestate` foundations and the root-pre-run stencil-seed gate every standalone CLI entry needs — so `Hardener` and future `Shed` products stop having to re-implement any of it.
   See the [Told-Geometry Invariant](../CONSTRAINTS.md#told-geometry-invariant) and the `internal/preflight` package documentation.

1. **producers standalone: producer engines** — `burlerengine`+`perchengine` and `websterengine`+`webstercli` convert to told geometry; Webster also gains its own standalone CLI entry (`--stencils-dir`/`--target-dir`/`--plan-dir`).
   See the [Told-Geometry Invariant](../CONSTRAINTS.md#told-geometry-invariant) and the `internal/hubgeom` and `internal/standalonegeom` package documentation.

1. **producers standalone: the standalone CLI path** — `burlercli`/`perchcli` branch around `lyxcwd.Resolve` and take `--stencils-dir`/`--target-dir`, so `lyx burler run --profile p.yaml` works in a directory that is not a git repository; the optional `scoutengine` uniformity pass landed alongside it.
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

1. **plan-format: flat card list** — a card carries `What:`, five typed file-op fields, and `Depends-on:`; symbol fields wait for `scout`.
   See [contracts/specs/loom-plan-spec.md](../contracts/specs/loom-plan-spec.md).

1. **built-in CLI help** — self-documenting `lyx`/`lyx <module>`/`lyx <module> <cmd> --help`.

1. **selfreport** — file Loomyard bugs as GitHub issues (`lyx selfreport create`).

1. **loom: contracts, Preflight, Discussion producer** — the three loom pieces shipped so far (loom as a whole is not done — see the Planned `loom` item).

1. **loom: Planner producer** — reads the discussion decision-record and writes a plan-format flat-card plan; a prompt/profile fed to `shuttle.Run`, not a module.
   No review logic of its own.

1. **dev/test `lyx.exe` separated from production deploy** — a second deploy target (`deploy-dev`/`deploy-dev.cmd`) so review/sandbox tooling never overwrites the stable production binary with an in-progress test build.
   See CONSTRAINTS.md's Dev/Prod Binary Separation invariant.

1. **scout: LSP-backed code intelligence — V1 Go-only, built for multi-language** — deterministic "where is this defined / used" lookups, replacing blind grepping.
   `lyx` is an LSP **client**, never a server.
   Two entry points: an in-process Go API and `lyx scout refs|definition|symbol`.
   See the `internal/scoutengine` package documentation.

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

1. **loom: phase-machine scaffolding** — `internal/loomshed` carries loom's full 12-row producer list: `Discussion-Validate`, `Plan-Validate`, and `Batchifier` built for real, `Preflight` and `Webster` wired in as-is, `Publish` and `Finalize` since built for real by the `landing: Publish + Finalize producers` item above, and the remaining five rows (`Discussion-Write`, `Discussion-Review`, `Plan-Write`, `Plan-Review`, `Webster-Review`) stubbed. (`Plan-Sweep` is not a row — see the Someday `loom: build Plan-Sweep for real` item.)
   loom's status file migrated onto `shedengine.Status`, with `loomshed.Seed` as its production seeder.
   See the `internal/loomshed` package documentation and the [Told-Geometry Invariant](../CONSTRAINTS.md#told-geometry-invariant).

## Maintenance

- **Numbering is automatic, not manual, and restarts at 1 in each section.**
  Every item is written literally as `1.` in the source — GitHub/CommonMark renders ordered-list items sequentially from the first item in a contiguous list block regardless of the literal digit on the rest,
  and a new `##` heading starts a new block.
  So Planned, Someday, and Done each render as their own 1, 2, 3, … with **zero number edits ever needed** — inserting, removing, or reordering items anywhere just works.
- **Numbers are not stable cross-reference IDs** (the same number exists in all three sections).
  Cross-reference by **bold item name** instead (e.g. "the Planned `board` item," "Someday's `scout` item") — every reference elsewhere in this file and in `designs/*.md` already does this.
- **Entries are short — a name plus one or two sentences of what/why, never a design writeup.**
  Detail belongs in the entry's own `designs/<name>.md` while the item is Planned or Someday.
  Delete that doc once the module ships (see the [documentation lifecycle](../docs/overview.md#documentation-lifecycle)) — a Done entry instead points at the module's own package documentation, which is where its durable detail lives from then on.
  If an entry keeps growing past a couple of sentences, that is a signal to move the growth into the doc it points to, not to let the entry itself grow.
- Move an item from Planned or Someday to Done, with a link to its module doc if one exists, when it ships — no renumbering needed anywhere.
- Someday items get a `designs/<name>.md` doc when there's real design behind them (`scout`, `raddle`, `webster: parallel card execution`, `hardener`, `warp-visibility`, `semantic-index` above do);
  trivial ones don't need one until they're promoted to Planned.
- This file is the single home for everything not scheduled, whether firmly committed to (`scout`, `raddle`) or genuinely speculative (`hardener`, the shuttle `Spec` ideas) — no separate long-term-ideas file.
  Add new speculative ideas directly to Someday.
