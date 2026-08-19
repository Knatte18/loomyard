# Roadmap: Loomyard

Loomyard replaces mill/millhouse (Python) with a Go orchestration layer, built as self-contained modules landed one at a time.
See [docs/overview.md](../docs/overview.md#principles) for the design principles.
This file is a numbered list of what's planned, what's committed-to- but-unscheduled, and what's shipped — for the detailed design of anything not yet built, see its doc under [designs/](designs/).
See Maintenance below for how the numbering works.

## Planned

Committed to, in this order, next.

1. **landing: Publish + Finalize producers** — two general `ShedProducer`s (see `designs/shed.md`), not loom-specific: shared by reference with both `loom`'s and the Someday `Hardener`'s producer lists, never `Shed`-special-cased. Bundled as one task because both wrap the same shared piece — `internal/mergeresolve`, the merge-in + LLM-conflict-resolution engine neither producer owns alone.
   Depends on the Done `fabric: merge-conflict primitive` item below — unblocked.
   - `internal/mergeresolve`: `Fabric.MergeIn(parent)`, conflict-shape detection, LLM escalation to a fresh higher-capability session on conflict. Called by both producers below, owned by neither.
   - `Publish`: mechanical `require_pr_to_base` check. Unset → no-op `Done`. Set → `mergeresolve`'s merge-in, then PR opened mechanically from Webster's summary artifact, no LLM call of its own. Returns `Done` once the PR exists — never waits on review. Progress past an open PR is entirely out-of-band, human-triggered (a separate, not-yet-designed interactive CLI flow, outside `Shed`).
   - `Finalize`: always calls `mergeresolve`'s merge-in itself (the only merge-in in the no-PR case, a second check-for-drift one in the PR case), `_lyx` teardown, final Fabric merge to parent.
   - Own config surface (`landing.yaml`: `require_pr_to_base` and other safe defaults, overridable per orchestrating profile — same "profiles live in the caller, not the callee" precedent as `loom.yaml`/`hardener.yaml`), Raddle regeneration folded into `Finalize`'s own merge critical section rather than a separate step.
   See [designs/landing.md](designs/landing.md).

1. **loom: phase-machine scaffolding** — mechanical rows only, except `Finalize` (own Planned item above, stays a stub here too); every `LLM`/`LLM+perch` row stays a stub.
   - Instantiate `loom` as a `Shed` instance carrying the full 13-row producer list, every row present (stubs included), so sequencing is real from the start.
   - Build `Discussion-Validate` for real: both files exist under `_lyx/discussion/`; `decision-record.md` has all seven required sections. Thin — new code, but small (file-exists + section-presence checks only).
   - Build `Plan-Validate` for real: `loom-plan-spec.md`'s existing hard-fail checks (e.g. `depends-on-order`). Thin wrap, not new logic: `internal/planparser.Validate(plan, worktreeRoot)` already implements every one of these checks — the producer just calls it and maps the result.
   - Wire in `Preflight`, `Batchifier`, and `Webster` as-is — all three already shipped, no new code in any of them.
   - Stub `Discussion-Write`, `Discussion-Review`, `Plan-Sweep`, `Plan-Write`, `Plan-Review`, `Webster-Review`, `Publish`, and `Finalize` — each returns `Done` without doing real work; `Publish`/`Finalize` swap in for the real, shared-by-reference producers once `landing: Publish + Finalize producers` lands — not loom-specific, so not tied to this task's own build order. `Plan-Sweep` stays stubbed here too: its only consumer, `Plan-Write`, is itself a stub in this task, so a real `Plan-Sweep` has nothing to feed yet — building it now would be premature. Not needed for loom to function.
   - Verify: the full 13-row sequence runs against the stubs, including resume, crash-recovery, and pause.
   See [designs/loom.md](designs/loom.md#the-phase-machine--a-flat-producer-list-no-predefined-slots).

1. **loom: session bootstrap** — `lyx loom run` (alias `lyx run`), the entry point that makes the phase machine from the item above actually reachable.
   - `lyx loom run`: ensure the worktree's tmux session is up, add the status strand (`lyx loom status --watch`), spawn the `loom` driver detached via `internal/proc`, attach the terminal to the tmux session.
   - The run-launcher: `.lyx/lyxrun.cmd`, dropped by `lyx fabric add`, so a double-click does `cd <worktree> && lyx loom run`.
   See [designs/loom.md](designs/loom.md#entry-point--the-session-bootstrap).

1. **loom: write and wire in the real LLM producers** — the only task in this initiative that touches LLM-prompt content, deliberately last.
   - Write `Discussion-Review`'s missing "what to check" rubric half (the "what not to flag" half already exists).
   - Write `Plan-Review`'s rubric from scratch — does not exist today; `loom-plan-spec.md` is a structural format spec, not review judgment criteria.
   - Write `Webster-Review`'s rubric from scratch — same gap, same reason.
   - Replace the `Discussion-Write` stub with a real `SingleLLMProducer` around the already-built prompt (`loom-template-discussion.md`).
   - Replace the `Plan-Write` stub with a real `SingleLLMProducer` around the already-built prompt (`loom-template-plan.md`).
   - Build `Plan-Sweep` for real, here rather than in scaffolding — it has no consumer until `Plan-Write` is real. Lowest priority within this task too: `scout`-backed work is low-priority project-wide, and `Plan-Sweep` is the only row in this initiative that touches `scout`. Mechanical scout inventory over the approved `decision-record.md`, feeding `Plan-Write`; spec in `designs/loom.md#plan-sweep-detail--the-scout-inventory-spec`. Partial building blocks: `scoutengine.References` and symbol lookup exist, but no ready-made "inventory" function — needs new composition, not a new engine.
   - Replace the `Discussion-Review`/`Plan-Review`/`Webster-Review` stubs with real `perch` adapters (`shedadapters.NewPerchProducer`) driven by the rubrics above.
   - Explicitly untouched by this task: `perch`'s round-loop/gate/milestone-cap/cluster-fan-out machinery, `burler`'s A/B round machinery, `webster`'s own engine — all already-shipped Go infrastructure this task plugs profiles into, not something it builds.
   See [designs/loom.md](designs/loom.md#the-phase-machine--a-flat-producer-list-no-predefined-slots).
## Someday

Committed to eventually — will be done — but not scheduled next.
No build order is implied between these items.

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
   `Tenter` is the review-loop (`Treadle` configured for behavior-review); `Hardener` is the full campaign (`Shed` + `Tenter`, worktree-spawn via `fabric` + safe-merge-back).
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

1. **finalize: the discrepancy-document conflict shape** — `finalize.md` originally sketched a second Fabric-to-Finalize conflict artifact, a precomputed "discrepancy document" for a divergence Fabric cannot express as a git conflict.
   Only the ordinary-git-conflict shape shipped; the document shape is not built.
   The existing `PullResult.PatternResidue` is the same shape and already exists for the rewrite case — answer this once, for both, when `Shed`/`loom` exist to consume it.

## Done

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
