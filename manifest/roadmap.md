# Roadmap: Loomyard

Loomyard replaces mill/millhouse (Python) with a Go orchestration layer, built as self-contained modules landed one at a time.
See [docs/overview.md](../docs/overview.md#principles) for the design principles.
This file is a numbered list of what's planned, what's committed-to- but-unscheduled, and what's shipped — for the detailed design of anything not yet built, see its doc under [designs/](designs/).
See Maintenance below for how the numbering works.

## Planned

This section holds what's committed to next.

1. **seeded driver choice: ly-drive strand as the child's driver** — the seed's `driver` field selects who steps a run: the detached Go runner, or a Claude strand running ly-drive in the worktree's own reed session, booted by the same Spawn seam.
   Depends on the seed contract from the item above.
   See [designs/seeded-shed.md](designs/seeded-shed.md).
## Next Up

What comes right after Planned clears — committed and ordered, unlike Someday below.
Not yet started, and exact order can still shift as Planned work reveals what unblocks what, but the rough sequence below is the current best guess.

## Someday

Committed to eventually — will be done — but not scheduled next.
No build order is implied between these items.

1. **webster: worktree-per-card parallel execution** — give each DAG-independent group its own `fabric`-spawned worktree, so concurrent cards stop sharing one git index. A speed optimization over an already-correct sequential system.
   See [designs/plan-card-format.md](designs/plan-card-format.md) and [designs/webster-parallel-execution.md](designs/webster-parallel-execution.md).

1. **VS Code as opt-in per worktree, not spun up by default** — default to CLI/tmux and start VS Code only on request, since the common case is reviewing the final PR rather than watching an agent edit live. Likely shape: a fourth per-worktree launcher variant (see `internal/fabricengine/launchers.go`) that opens VS Code just far enough to `lyx reed attach` — a terminal-launcher convenience, not a standing editor.

1. **doctor** — diagnostics command (`lyx doctor`): checks `_lyx/` layout, config parse, board reachability, stale locks.

1. **session sync** — copy Claude `.jsonl` transcripts across machines so `--resume` works elsewhere.

1. **Claude Code plugin packaging** — ship `lyx` as an installable plugin.

1. **reed: strand-based mailbox/addressing system** — deliver messages/events to any Strand by address; being a Strand is required to *receive* mail, not to *send* it.
   See [designs/reed-mailbox.md](designs/reed-mailbox.md).

1. **reed: cross-worktree columns** — all worktrees in one tmux window, a column per worktree; needs a name for the new per-worktree grouping layer this introduces and a column-count/fallback policy.
   See [designs/reed-multi-window.md](designs/reed-multi-window.md#cross-worktree-columns).

1. **reed: own-window strand anchoring** — a `display` anchor that spawns a strand into its own switchable tmux window instead of a pane.
   See [designs/reed-multi-window.md](designs/reed-multi-window.md#own-window-strand-anchoring).

1. **reed: independent per-window attach via tmux session groups** — tmux's session-groups feature would let multiple `lyx reed attach` invocations show different windows independently, instead of sharing one today.
   See [designs/reed-multi-window.md](designs/reed-multi-window.md#independent-per-window-attach-via-tmux-session-groups).

1. **fabric: Windows path behaviour is unverified after six hardening rounds** — the platform sibling of the now-Done `Real-Linux validation`; needs a Windows host to close, not further design.
   See [designs/fabric-windows-verification.md](designs/fabric-windows-verification.md).

1. **raddle** — codeguide's woven-in successor;
   parallel-regeneration design exists;
   folds into `Finalize`'s own contract rather than a separate producer — `Shed` has no slots for it to occupy.
   See [designs/raddle.md](designs/raddle.md).

1. **Tenter + Hardener** — behavior-based hardening of a live-substrate module (archetype: `reed` driving real tmux), on-demand and post-`loom`, off the `shuttle → burler → shed → loom` spine; `Hardener` is the full campaign (`Shed` + `Tenter`, worktree-spawn via `fabric` + safe merge-back).
   See [designs/hardener.md](designs/hardener.md) (a DRAFT doc, do not implement from it yet).

1. **warp-visibility: `CLAUDE.local.md` invisible in the Fabric repo's git history** — expose `CLAUDE.local.md` via symlink (Windows-Developer-Mode note + copy fallback) so nothing lyx-related shows up in the Fabric repo's own git history; the `CONSTRAINTS.md`-equivalent half is already covered by the shipped `PATTERN.md`, which lives in `weft` and is already invisible there.
   See [designs/warp-visibility.md](designs/warp-visibility.md).

1. **shuttle `Spec`: generic tools-restriction** — meaningless for today's single-session A→B agent;
   cluster reviewers turned out to be fork subagents inside the handler's own session (`useExactTools`), not separate sessions needing their own `settings.json`, so this stays unmotivated rather than blocked on anything.

1. **shuttle `Spec`: per-round provider selector** — meaningless until a second engine lands (non-Claude engines are not a current priority, per `CLAUDE.md`); today "provider" just means whichever engine is wired into the `Runner`. The cost if picked up: `burler`'s cluster-review fan-out has no non-Claude equivalent, needing N full sessions where Claude uses N cheap context-sharing forks.

1. **Bulk-mode clusters + provider-side context caching** — a `burler` cluster round can run *tool-use* or *bulk* (Go concatenates target + fasit + rubric into one blob).
   Bulk is what makes provider-side context caching (e.g. Gemini's explicit cache) pay off, and only if modelled as one shared prefix + N distinct suffixes, never N full prompts.

1. **semantic-index** — semantic search over docstrings/comments (Enzyme-inspired: catalysts + embeddings + temporal decay), to find code by concept rather than literal keyword.
   Genuinely speculative, not yet designed in depth.
   See [designs/semantic-index.md](designs/semantic-index.md).

1. **board: curation/triage automation** — an automated skill that ingests GitHub issues and extracts a logical next task from the manifest, promoting it via the already-shipped `promote-note` primitive. This is the automation layer on top of that primitive, deferred out of `board: move storage to weft:main`.
   See [designs/curation-triage.md](designs/curation-triage.md).

1. **config: repo-wide default + per-worktree override, millhouse `config.local.yaml`-style** — every module's config resolves only from `<cwd>/_lyx/config/<module>.yaml` today, per-worktree with no shared default (`fabric.yaml` is the sole exception, anchored at `_board`). Add a repo-wide default layer read from `_board`, with each worktree's own file as an override on top — the two-layer overlay millhouse already uses; not yet designed.

1. **discussion-format / plan-format: classify review findings by kind** — carry a finding-class dimension (`design`, `scope`, `decision`, `consistency`) on review findings, and scope each review stage to what its downstream stage cannot catch better.
   See [designs/review-finding-classification.md](designs/review-finding-classification.md).

1. **fabric: ordinary-monorepo verb surface** — against plain git, `fabric` is still missing `log`, `show`, `branch` (create/list/delete), `tag`, `stash`, `reset` (non-hard), `revert`, `restore`, `rm`/`mv`, `rebase`, `cherry-pick`, and `blame`.
   None blocks `Finalize`/`Hardener` today; scope by actual need when a consumer needs one, never by completing the list for its own sake.

1. **fabric: two-sided reset-to-SHA verb** — the post-conclude undo the merge surface deliberately does not ship: `MergeAbort` covers only the uncommitted merge-attempt window, so a landed merge is final at the Fabric layer. Closing it means a reset to a visible warp SHA that resolves the paired weft SHA through the correspondence index and routes both sides through the destruction gate.
   See the `internal/fabricengine` package documentation's merge section.

1. **loom: build `Plan-Sweep` for real** — the one row of loom's design table never built, and not a `loom-recipe.yaml` row at all; deferred because quarry-backed work is low-priority project-wide and this is the only row in the initiative that touches quarry. Full spec already written.
   See [designs/loom.md](designs/loom.md#plan-sweep-detail--the-quarry-inventory-spec).

1. **finalize: the discrepancy-document conflict shape** — some divergences cannot be expressed as a git conflict at all, so there are no markers to hand a resolving agent; the answer is a precomputed document describing the disagreement instead. Only the ordinary-git-conflict shape shipped (`internal/mergeresolve`), while `PullResult.PatternResidue` already is this shape for the history-rewrite case — design it once, for both, whenever picked up.

1. **shedrecipe: capability-declaration instead of manual seam-threading** — giving a producer a new capability means hand-threading a passthrough `Env` field through three layers, because the Shed Recipe Registry Invariant bars `shedrecipe` from importing the capability's owning package. The idea, not yet designed: let a producer declare what it needs and have the registry wire it — deep, likely touching the invariant itself and all seventeen registry entries.

1. **reed: daemon Slack relay** — bidirectional Slack relay per worktree, riding on the now-Done `reed: watchdog daemon`. Low priority, well behind the daemon's own self-heal jobs — split out on purpose so it never blocks or gets conflated with the watchdog work.

## Done

Cleared 2026-08-25 to keep this file lean — shipped items' history lives in `git log` and each module's own package documentation, not here.

1. **seeded Shed core: run addressing, seed contract, batten** — every Shed run now has a run directory addressed by run-id (durable `_lyx/shed/<run-id>/` holding `seed.json`+`status.json`, ephemeral `.lyx/shed/<run-id>/` holding its locks), defaulting to `self`; `internal/shedrun` owns the run-id vocabulary and the seed contract end to end. A new `Seed-Child` row and the Board's `type` field carry the recipe choice into a task worktree's own inner run. The lifecycle recipe is renamed **batten** (`internal/battenshed`+`internal/battenrecipe`+`internal/battencli`), its `Loom-Run` row becomes the product-neutral, step-friendly `Run-Shed`, and `lyx shed seed`/`lyx batten run|step|status|pause` replace the old `--recipe` flag and `lyx lifecycle` verbs.
   See [designs/seeded-shed.md](designs/seeded-shed.md).

1. **generalize `ly-drive` and loom's `start`/`run`/`step` CLI verbs into a Shed-generic watchdog** — the generic `run`/`step`/`status`/`pause` verb bodies now live in `internal/shedverbs`, armed by a new named-recipe `lyx shed` subtree (`internal/shedcli`) alongside loom's and lifecycle's own subtrees; lifecycle gained `pause` and `status --watch` to match. The `ly-drive` skill now drives any recipe through `lyx shed step --recipe <name>`, defaulting to `loom`.
   See [designs/shed-generic-watchdog.md](designs/shed-generic-watchdog.md).

1. **reed: per-hub daemon reaps orphaned sessions** — the per-hub daemon now checks each live session name's worktree directory every discovery cycle, reaps a session confirmed gone across three consecutive affirmative cycles by capturing its pane process closure and then killing the session by exact target, and refuses to act at all while the hub directory itself does not stat live.
   See [designs/reed-header-selvage.md](designs/reed-header-selvage.md).

1. **reed: born-as-strand for the operator's `loom start` attach** — `lyx loom start`'s terminal handoff now adds an operator-owned Strand before attaching, spawn-then-attach like `reed add` does, and the same verb spawns the per-hub watchdog daemon its session runs on regardless of `--attach`.
   See the `internal/loomcli` and `internal/reedengine` package documentation.

1. **reed: extract Selvage-pane lifecycle out of apply/reconcile/spawn/lifecycle** — Selvage's pane creation, reap-exemption, and split-target lifecycle now lives in one file, `internal/reedengine/selvagepane.go`, with a mechanical AST enforcement test barring it from re-scattering across `apply.go`/`reconcile.go`/`spawn.go`/`lifecycle.go`. See `internal/reedengine`'s package documentation.
   See [designs/reed-selvage-pane-extraction.md](designs/reed-selvage-pane-extraction.md).

1. **reed: `AddStrand` and `attach` self-heal a cold worktree instead of requiring `up` first** — both verbs pre-flight through the new `ensureSessionLocked`/`EnsureSession()` seam, which probes session liveness first and only delegates to `upLocked()` when nothing usable is up, so any spawn OR view into a worktree nobody has visited (no VS Code, no manual `up`) just works. `attach` boots via `EnsureSession()` and keeps its existing `Status()` call, in that order, so both the friendly no-session diagnosis and the foreign-session refusal survive unchanged. A warm call against a live session is never routed through `Up()`/`upLocked()`: that path reaches `planReconcile`, which would kill an operator's hand-split pane, and validates config ahead of its already-up early return, which would refuse a healthy `attach` on an unrelated typo. Fully internal to `reedengine` — fabric never needs to know reed exists.

1. **loom CLI: rename `run`/`drive`/`step` for verb/engine symmetry, plus rename `ly-supervise`** — `drive` became `run`, the old `run` became `start`, `step` is unchanged, the root alias became `lyx start`, and `ly-supervise` became `ly-drive`.

1. **ly-drive + orchestrator: launch via `lyx reed add`, not ad hoc** — the generated VS Code `folderOpen` task is now the reed launch chain, with both binary paths stamped absolute; `lyx reed add` gained `--if-absent` so reopening a worktree is idempotent; and the `/ly:ly-drive` skill now treats the session running it as the orchestrator strand itself, rather than telling the operator to open a second one.

1. **fabric: no remote/GitHub branch deletion** — `lyx fabric cleanup` and `lyx fabric remove` both gained an opt-in `--remote` flag that additionally deletes each deleted weft branch's copy on the weft remote.
   See the `internal/fabricengine` package documentation's destruction chokepoint section.

1. **worktree spawn/teardown as Shed producers** — one driven `Shed` run now takes a task worktree through create, run the loom session to a terminal state, and tear down, driven from the hub's prime worktree (`internal/lifecycleshed` + `internal/lifecyclerecipe` + `internal/lifecyclecli`; `lyx lifecycle run|status`); teardown is one row sequencing session shutdown before worktree removal, never forcing. Optional VS Code embedding did not ship as part of this driven run — see the Someday `VS Code as opt-in per worktree, not spun up by default` item for where that work lives.
   See the `internal/lifecycleshed` and `internal/lifecyclerecipe` package documentation.

1. **Adopt quarry's glyph alphabet as the plan alphabet** — `planparser`/the validator switched a card's symbol declarations from bare names to quarry glyphs, resolved via batched `Resolve`, with placeholder handles (`plan:<expected-glyph>`) for symbols a plan itself creates and mechanical drift detection against the code. Superseded the Someday `quarry-backed plan symbol verification` item.
   See [designs/quarry-glyph-plan-alphabet.md](designs/quarry-glyph-plan-alphabet.md).

1. **self-report Tier 1: Go-detected structural anomalies** — loom's own status file now records crash-resumes, `stuck` escalations, and repeated review rounds, and files these directly via the shipped `selfreport` primitive, with no LLM call and no session watching needed.
   See [designs/self-report-tier1.md](designs/self-report-tier1.md).

1. **fabric: surface merge-in-progress in `lyx fabric status`** — `status` now reports a `merge_in_progress` boolean, whether THIS pair has a fabric merge parked.
   See the `internal/fabricengine` package documentation's merge section.

1. **reed: watchdog daemon** — the header-pane watch loop, with both halves landed: the resize-geometry reconcile and the pane reap.
   See `internal/reedengine`'s package documentation.

1. **reed: replace the header pane with a native tmux status-line, a permanent "Selvage" terminal pane, and a detached per-hub watchdog process** — the one header pane's three conflated jobs are split apart: identity content now renders through tmux's own native status-line; a deliberately ordinary shell pane, named **Selvage**, is the always-on, pinned-to-the-bottom control terminal that keeps the session alive and doubles as where you'd run `lyx reed add` and friends directly; and the watchdog daemon moved out of any pane entirely, into its own detached background process scoped one-per-hub (matching the existing tmux-server-per-hub boundary).
   See [designs/reed-header-selvage.md](designs/reed-header-selvage.md).

1. **Real-Linux validation** — the sandbox suite and every tmux/`/proc` assumption are exercised on real Linux, now the platform everything runs on.

1. **loom: Discussion-Review producer** — replaced the `Discussion-Review` stub with a `Discussion-Bouncer`/`Discussion-Burler` segment.
   See [designs/loom.md](designs/loom.md#discussion-producer-detail--validation-checks-and-review-rubric).

1. **loom: Webster-Review producer** — replaced the `Webster-Review` stub row with a `Webster-Bouncer`/`Webster-Burler` segment gating the committed diff.
   See [designs/loom.md](designs/loom.md#webster-review-rubric).

1. **loom: interactive Discussion-Write** — flipped `internal/loomcli`'s `wire()` `autonomous` argument from hardcoded `true` to the `discussion_interactive` config key, and solved the resume defect that made autonomous-only the right call so far by giving `shuttleengine` a live-agent-aware `Attach`.
   See [designs/loom.md](designs/loom.md#crash-recovery--resume-on-output-files-not-live-processes).

1. **loom: `Discussion-Burler` fix-scope corrected to `overlay`** — the `Discussion-Burler` row now runs `fix-scope: overlay` and `Discussion-Bouncer` commits its approved settle through `commit_seam: discussion`, restoring compliance with the Fabric Git Invariant, with a parse-level guard added so the class of violation cannot ship again.
   See [designs/loom.md](designs/loom.md#the-gate).

1. **loom: review segments resolve `_lyx` paths against the wrong root and don't clear their Bouncer run directory on re-entry** — both defects are fixed across all three review segments: the segments' `_lyx` paths now resolve against the anchor path their commit seam already anchored at, and a `Bouncer` re-entered after approving now archives its run directory and re-judges rather than replaying a settled verdict.
   See [designs/loom.md](designs/loom.md#the-gate).

1. **producer-agnostic final-summary artifact** — the read contract is now a producer-agnostic leaf, `internal/summaryparser`, and `landingshed` takes a told path rather than reaching into a producer's own directory; `Finalize`'s squash-merge `MergeOptions.Message` is now wired to the composed title and body.
   See [final-summary-spec.md](../contracts/specs/final-summary-spec.md).

1. **self-report Tier 2: per-agent friction notes for unsupervised runs** — every one of the seven prompt-composing agents now gets an optional friction-note directive injected into its prompt, default-on via `loom.yaml`'s `friction` model-spec key, and `internal/loomcli`'s run verb spawns one dedicated reflection agent per run to aggregate whatever notes were written and file them via `lyx selfreport create`.
   See the `internal/friction` and `internal/frictionengine` package documentation, and [designs/self-report-tier2.md](designs/self-report-tier2.md).

1. **`lyx loom step` + an external supervisor skill** — a new Go verb runs exactly one of `loom`'s next phases (loom still owns all sequencing) and returns; the `/ly:ly-drive` skill drives it in a loop, watching live for anything a mechanical gate wouldn't catch, and handing back to the operator on any non-running state or error envelope. Supersedes `llm-driven-loom-alternative`.
   See [designs/loom-step.md](designs/loom-step.md).

## Maintenance

- **Numbering is automatic, not manual, and restarts at 1 in each section.**
  Every item is written literally as `1.` in the source — GitHub/CommonMark renders ordered-list items sequentially from the first item in a contiguous list block regardless of the literal digit on the rest,
  and a new `##` heading starts a new block.
  So Planned, Next Up, Someday, and Done each render as their own 1, 2, 3, … with **zero number edits ever needed** — inserting, removing, or reordering items anywhere just works.
- **Numbers are not stable cross-reference IDs** (the same number exists in all four sections).
  Cross-reference by **bold item name** instead (e.g. "the Planned `board` item," "Someday's `raddle` item") — every reference elsewhere in this file and in `designs/*.md` already does this.
- **Entries are short — a name plus one or two sentences of what/why, never a design writeup.**
  Detail belongs in the entry's own `designs/<name>.md` while the item is Planned or Someday.
  Delete that doc once the module ships (see the [documentation lifecycle](../docs/overview.md#documentation-lifecycle)) — a Done entry instead points at the module's own package documentation, which is where its durable detail lives from then on.
  If an entry keeps growing past a couple of sentences, that is a signal to move the growth into the doc it points to, not to let the entry itself grow.
- Move an item from Planned or Someday to Done, with a link to its module doc if one exists, when it ships — no renumbering needed anywhere.
- Someday and Next Up items get a `designs/<name>.md` doc when there's real design behind them (`raddle`, `webster: worktree-per-card parallel execution`, `hardener`, `warp-visibility`, `semantic-index` above do);
  trivial ones don't need one until they're promoted to Planned.
- This file is the single home for everything not scheduled, whether firmly committed to (`warp-visibility`, `raddle`) or genuinely speculative (`hardener`, the shuttle `Spec` ideas) — no separate long-term-ideas file.
  Add new speculative ideas directly to Someday — Next Up is a promotion stage, not an entry point, so nothing is added there fresh.
- **Promotion path:** Someday → Next Up → Planned → Done.
  An item can skip Next Up straight from Someday to Planned if it becomes urgent; Next Up is a waypoint, not a mandatory gate.
