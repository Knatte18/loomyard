# Roadmap: Loomyard

Loomyard replaces mill/millhouse (Python) with a Go orchestration layer, built as self-contained modules landed one at a time.
See [docs/overview.md](../docs/overview.md#principles) for the design principles.
This file is a numbered list of what's planned, what's committed-to- but-unscheduled, and what's shipped — for the detailed design of anything not yet built, see its doc under [designs/](designs/).
See Maintenance below for how the numbering works.

## Planned

This section holds what's committed to next.

## Someday

Committed to eventually — will be done — but not scheduled next.
No build order is implied between these items.

1. **webster: worktree-per-card parallel execution** — spawn independently-executable cards/batches into their own `git worktree` (via `internal/fabricengine`) instead of forking in one shared worktree, gating a batch's completion on a build+test of the merged result; the ready set recomputes wave to wave, never precomputed upfront. Depends on the shipped `webster: DAG-derived card sequencing` for the dependency graph. Deliberately Someday, not Planned: a speed optimization over an already-correct sequential system, not a prerequisite — wait for the sequential path to get real mileage first.
   See [designs/plan-card-format.md](designs/plan-card-format.md) and [designs/webster-parallel-execution.md](designs/webster-parallel-execution.md) (stale, reconcile in this task — its prior rejection was about concurrent forks sharing one checkout's git index, a different model than worktree-per-card, which does not share that race).

1. **worktree spawn/teardown as Shed producers** — fold today's three manually-sequenced steps (`lyx fabric` create, `lyx loom run`, `lyx fabric` teardown) into `ShedProducer` rows bookending `loom`'s own list, so the whole task lifecycle is one driven `Shed` run instead of a human bridging three CLI invocations. Likely needs `fabric`'s worktree creation brought into `_launchers`/`_board` wiring first (deliberately kept out today to protect the Fabric illusion) — needs its own look before this is scoped further.

1. **VS Code as opt-in per worktree, not spun up by default** — `loom`/Loomyard should default to CLI/tmux, spinning up VS Code only on request, since the common case is reviewing the final PR rather than watching an agent edit live. Likely shape: a fourth per-worktree launcher variant (alongside `ide`/`fabric-checkout`/`run<ext>`, see `internal/fabricengine/launchers.go`) that opens VS Code just far enough to `lyx reed attach` into the worktree's already-running `reed` server — a terminal-launcher convenience, not a standing editor.

1. **doctor** — diagnostics command (`lyx doctor`): checks `_lyx/` layout, config parse, board reachability, stale locks.

1. **session sync** — copy Claude `.jsonl` transcripts across machines so `--resume` works elsewhere.

1. **Claude Code plugin packaging** — ship `lyx` as an installable plugin.

1. **reed: cross-worktree columns** — all worktrees in one window, a column per worktree.

1. **reed: own-window strand anchoring** — a `display` anchor that spawns a strand into its own switchable tmux window instead of a pane.

1. **fabric: Windows path behaviour is unverified after six hardening rounds** — the platform sibling of the now-Done `Real-Linux validation`; needs a Windows host to close, not further design.
   See [designs/fabric-windows-verification.md](designs/fabric-windows-verification.md).

1. **raddle** — codeguide's woven-in successor;
   parallel-regeneration design exists;
   folds into `Finalize`'s own contract rather than a separate producer — `Shed` has no slots for it to occupy.
   See [designs/raddle.md](designs/raddle.md).

1. **webster: parallel card/batch execution** — a possible unblocking shape exists (own `fabric` worktree per DAG-independent group, avoiding the git-index race the earlier rejected shape hit) but isn't yet a plan — needs the DAG source and more design work.
   See [designs/webster-parallel-execution.md](designs/webster-parallel-execution.md) (status banner there is stale — written for the earlier, rejected shape, not this one).

1. **Tenter + Hardener** — behavior-based hardening of a live-substrate module (archetype: `reed` driving real tmux), on-demand and post-`loom`, off the `shuttle → burler → shed → loom` spine; `Hardener` is the full campaign (`Shed` + `Tenter`, worktree-spawn via `fabric` + safe merge-back).
   See [designs/hardener.md](designs/hardener.md) (a DRAFT doc, do not implement from it yet).

1. **warp-visibility: `CLAUDE.local.md` invisible in the Fabric repo's git history** — expose `CLAUDE.local.md` via symlink (Windows-Developer-Mode note + copy fallback) so nothing lyx-related shows up in the Fabric repo's own git history; the `CONSTRAINTS.md`-equivalent half is already covered by the shipped `PATTERN.md`, which lives in `weft` and is already invisible there.
   See [designs/warp-visibility.md](designs/warp-visibility.md).

1. **shuttle `Spec`: generic tools-restriction** — meaningless for today's single-session A→B agent;
   cluster reviewers turned out to be fork subagents inside the handler's own session (`useExactTools`), not separate sessions needing their own `settings.json`, so this stays unmotivated rather than blocked on anything.

1. **shuttle `Spec`: per-round provider selector** — meaningless until a second engine lands (non-Claude engines are not a current priority, per `CLAUDE.md`); today "provider" just means whichever engine is wired into the `Runner`. The main cost if picked up: `burler`'s cluster-review fan-out (N reviewers as cheap, context-sharing forks) has no non-Claude equivalent — a non-Claude engine would need N full separate sessions instead, costlier by construction.

1. **Bulk-mode clusters + provider-side context caching** — a `burler` cluster round can run *tool-use* or *bulk* (Go concatenates target + fasit + rubric into one blob).
   Bulk is what makes provider-side context caching (e.g. Gemini's explicit cache) pay off, and only if modelled as one shared prefix + N distinct suffixes, never N full prompts.

1. **semantic-index** — semantic search over docstrings/comments (Enzyme-inspired: catalysts + embeddings + temporal decay), to find code by concept rather than literal keyword.
   Genuinely speculative, not yet designed in depth.
   See [designs/semantic-index.md](designs/semantic-index.md).

1. **self-report: two-tier friction capture** — loom's per-phase design means no single LLM session has full-run context the way Millhouse's self-report assumes; splits into Go-detected structural anomalies plus per-phase friction notes, aggregated for one reflection agent at natural end points.
   See [designs/self-report.md](designs/self-report.md).

1. **board: curation/triage automation** — the GitHub-issue-intake and periodic-triage workflow originally scoped in `designs/board-weft-storage.md`'s Curation flow section, deferred out of `board: move storage to weft:main`: an automated skill that ingests GitHub issues and extracts a logical next task from the manifest, promoting it via `promote-note` (which already ships as a plain mechanical CLI primitive — this item is the automation layer on top, not the primitive itself).
   See [designs/curation-triage.md](designs/curation-triage.md).

1. **quarry-backed plan symbol fields** — `loom-plan-spec.md` deliberately deferred `creates-symbols`/`edits-symbols`/`reads-symbols` fields pending a verified code-intelligence lookup tool; that tool (now `quarry`, an external Go module dependency) and the loom Planner have since shipped, unblocking but not yet scoping this.
   Named prerequisite for `webster: parallel card/batch execution`'s parked DAG scheduler.
   See [designs/quarry-plan-symbol-fields.md](designs/quarry-plan-symbol-fields.md).

1. **config: repo-wide default + per-worktree override, millhouse `config.local.yaml`-style** — every module's config today resolves only from `<cwd>/_lyx/config/<module>.yaml` (per-worktree, no shared default; `fabric.yaml` is the sole exception, anchored at `_board`/weft:main). Add a repo-wide default layer, read from `_board`, with each worktree's own `_lyx/config/<module>.yaml` as an override on top — the same two-layer overlay millhouse's `mill-config.yaml` (hub root) → `.millhouse/config.local.yaml` (local override) already uses. Not yet designed.

1. **discussion-format / plan-format: classify review findings by kind** — carry a finding-class dimension (`design`, `scope`, `decision`, `consistency`) on review findings, and scope each review stage to what its downstream stage cannot catch better.
   See [designs/review-finding-classification.md](designs/review-finding-classification.md).

1. **fabric: ordinary-monorepo verb surface** — against plain git, `fabric` is still missing `log`, `show`, `branch` (create/list/delete), `tag`, `stash`, `reset` (non-hard), `revert`, `restore`, `rm`/`mv`, `rebase`, `cherry-pick`, and `blame`.
   None blocks `Finalize`/`Hardener` today; scope by actual need when a consumer needs one, never by completing the list for its own sake.

1. **fabric: two-sided reset-to-SHA verb** — the post-conclude undo the merge surface deliberately does not ship: `MergeAbort` covers only the uncommitted merge-attempt window, so a landed merge is final at the Fabric layer until a `Fabric`-level reset to a visible (warp) SHA exists, resolving the paired weft SHA through the correspondence index and routing both resets through the destruction gate.
   See the `internal/fabricengine` package documentation's merge section.

1. **fabric: surface merge-in-progress in `lyx fabric status`** — `MergeInProgress` ships as Go API only; folding it into the `status` verb's output is a small follow-up.

1. **loom: build `Plan-Sweep` for real** — stays a stub past the shipped `loom: Plan-Write producer`; deferred because quarry-backed work is low-priority project-wide right now and this is the only row in the initiative that touches quarry. Full spec already written.
   See [designs/loom.md](designs/loom.md#plan-sweep-detail--the-quarry-inventory-spec).

1. **finalize: the discrepancy-document conflict shape** — `finalize.md` originally sketched a second Fabric-to-Finalize conflict artifact, a precomputed "discrepancy document" for a divergence Fabric cannot express as a git conflict.
   Only the ordinary-git-conflict shape shipped; the document shape is not built.
   The existing `PullResult.PatternResidue` is the same shape and already exists for the rewrite case — answer this once, for both, whenever picked up (`Shed`/`loom` now exist to consume it).

1. **shedrecipe: capability-declaration instead of manual seam-threading** — giving a producer a new capability today means hand-threading a passthrough `Env` field through three layers (`shedrecipe` → `loomrecipe` → `loomcli`), since `shedrecipe` can't import the capability's owning package directly (Shed Recipe Registry Invariant); both shipped `loom: Discussion-Write producer` and `loom: Plan-Write producer` repeated this identical three-layer edit for their own two `Env` fields. The idea — not yet designed — is for a producer to declare what it needs and have the registry wire it automatically, closer to how a VS Code extension declares its own capabilities than to hand-editing a host per extension. Genuinely deep: likely touches the Shed Recipe Registry Invariant itself and all fourteen existing registry entries already wired the old way.

1. **reed: daemon Slack relay** — bidirectional Slack relay per worktree, riding on the now-Done `reed: watchdog daemon`. Low priority, well behind the daemon's own self-heal jobs — split out on purpose so it never blocks or gets conflated with the watchdog work.

## Done

Cleared 2026-08-25 to keep this file lean — shipped items' history lives in `git log` and each module's own package documentation, not here.

1. **reed: watchdog daemon** — the watch loop hosted in the per-worktree header pane, with its `window-resized` hook entry, poll fallback, debounce, and `watchdog: on|off` key in `reed.yaml`;
   both halves landed, the resize-geometry reconcile and the pane reap.
   See `internal/reedengine`'s package documentation.

1. **Real-Linux validation** — the sandbox suite and every tmux/`/proc` assumption are exercised on real Linux, which is now the platform everything runs on.
   Its platform sibling, `fabric: Windows path behaviour`, stays open in Someday.

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
- Someday items get a `designs/<name>.md` doc when there's real design behind them (`quarry-backed plan symbol fields`, `raddle`, `webster: parallel card/batch execution`, `hardener`, `warp-visibility`, `semantic-index` above do);
  trivial ones don't need one until they're promoted to Planned.
- This file is the single home for everything not scheduled, whether firmly committed to (`warp-visibility`, `raddle`) or genuinely speculative (`hardener`, the shuttle `Spec` ideas) — no separate long-term-ideas file.
  Add new speculative ideas directly to Someday.
