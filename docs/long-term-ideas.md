# Long-term ideas

Speculative, longer-horizon design ideas that are **not** yet planned milestones. `roadmap.md` is
reserved for planned milestones only (see its Maintenance section) — this file is where a
promising direction gets written down *before* it's mature enough to become one, so it isn't lost
and doesn't clutter the roadmap with maybes. Entries here have no schedule and may never be built.
When an idea here matures into a concrete, scoped plan, it graduates to a numbered milestone in
`roadmap.md` (and this entry should be removed or marked graduated).

## Master Builder: fork-based batch implementation

**Status:** idea, not scoped. Builder as it stands today (see
[modules/builder-contract.md](modules/builder-contract.md)) is fine for now — this is a possible
future optimization, not a gap.

**The problem today:** the orchestrator spawns one fresh implementer thread per batch. Each fresh
thread has to re-orient itself from scratch — read the relevant code, understand conventions and
architecture, find where things live — before it can even start on its actual batch. That
orientation cost is paid again, from zero, on every single batch.

**The idea:** apply the same shape `burler` already validated for cluster review — one parent
session does the expensive orientation once, then forks out per unit of work, so forks inherit
that orientation instead of rebuilding it. For Builder: a **Master Builder** reads the codebase and
the overall implementation plan once, then forks out one implementer per batch instead of spawning
a fresh thread per batch. Sequential forking (one batch at a time, same order as today) is the
straightforward version — no new concurrency risk, since it's structurally the same loop Builder
runs today, just with forks instead of fresh spawns.

**The catch, and why it's not just "inherit everything":** unlike burler's reviewers (read-only,
so inherited file content stays valid for the life of the review), Builder's forks *write* code.
If Batch B depends on Batch A, the files A touched have changed by the time B starts — but B's
fork was forked from Master's *pre-A* snapshot, so anything A changed is stale in B's inherited
context. Re-reading those files fixes correctness but creates duplicate, confusing context (a
stale copy inherited from the fork point, a fresh copy read again) — exactly the "context
clutter" this idea is trying to avoid in the first place.

**Proposed resolution — separate what's stable from what's mutable:**

1. **Stable orientation context** — codebase structure, conventions, `CONSTRAINTS.md`, module
   interfaces, the implementation plan itself. This doesn't change as batches land. This is the
   expensive-to-rebuild part, and it's safe to inherit through every fork indefinitely.
2. **Mutable file content** — the actual files a batch will edit. This *does* go stale the moment
   an earlier batch commits. Master should never treat raw file content as part of the durable,
   inherited prefix; each fork does its own fresh read of the specific files its batch touches,
   right before it starts writing. This avoids the stale-vs-fresh duplicate problem entirely,
   because the inherited context never claimed the file content was current — only the
   orientation was.
3. **Cross-batch dependencies (B depends on A)** — B needs to know not just A's finished files
   (covered by point 2) but *what A did and why* (decisions, deviations from plan). Reusing the
   principle already established for the loom orchestrator loop — **distilled batch reports, never
   raw sub-agent prose** (the "mill-go bloat lesson", see `roadmap.md` milestone 12) — Master
   absorbs a short distilled summary of a completed batch before forking the next batch that
   depends on it. Master's own context then grows only by one small summary per completed
   dependency, not by re-read file content or raw transcripts.
4. **Independent batches** (no dependency edge) can all fork from the same common Master
   snapshot — there's no cross-batch information they need from each other.

Open question not yet resolved: should the distilled summary be something **Master itself writes**
after reading a batch's report (mirroring how loom's orchestrator already digests batch reports),
or should the **implementer fork be contractually required to produce it** as part of its own
output (more structured, but needs a new field in the batch/digest contract)?

**Further-out, riskier extension — parallel batches via a DAG:** today's plan format is
deliberately a flat ordered list with **no DAG** (see `modules/plan-format.md` and `roadmap.md`
milestone 12 — "task-level parallelism via separate worktrees + `lyx run`, not intra-plan"). Running
independent batches as *parallel* forks would require reintroducing a DAG into the plan, which
reopens exactly the problem that decision avoided: two forks writing to the same worktree at the
same time can collide on disk. That's only safe if either (a) the DAG's independence edges
actually guarantee no file overlap between parallel batches — a guarantee the DAG would need to
prove, not just assume — or (b) each parallel batch gets its own worktree/branch, which
reintroduces the multi-branch merge complexity the no-DAG decision was specifically chosen to
avoid. Not pursued further until the sequential version above is real and this looks worth the
complexity.

## `hardener`: behavior-based hardening of live-substrate modules

**Status:** draft concept, not settled. Moved here from what used to be roadmap milestone 23 — it
never belonged among scoped, committed milestones.

A separate, **on-demand, post-loom** reviewer that *runs* a live-substrate module (the archetype:
`mux` driving real tmux) in a **sandbox repo** and reacts to what it observes, rather than reading
an artifact as `perch` (see the `internal/perchengine` package documentation) does. Orchestrated by
an accumulating (per-round-respawn + handoff) orchestrator that targets each round and verifies via
a **deterministic gate** (N× concurrent smoke, zero stray state); shares only the `burler` round
discipline (see the `internal/burlerengine` package documentation). Token- and wall-clock-heavy (a
campaign ran a weekend), but it hardened `mux` where text-review could not. **Off the
`burler → perch → loom` spine** — it blocks nothing and is picked up only after `loom` works.
Concept still being figured out; see [modules/hardener.md](modules/hardener.md) (a DRAFT doc, do
not implement from it yet).

## mux daemon: foreign-pane self-heal

**Status:** possible extension to roadmap milestone 14 (mux daemon), not yet scoped as part of it.

Today mux is one-shot, so an operator-split or stray "faux" pane is only reaped on the *next* mux
verb (reconcile owns the session window). The daemon (once built, per milestone 14) could close
that gap by reconciling on its own. Design steer for when this is picked up: prefer **event-driven
tmux hooks** (e.g. `after-split-window` / `window-layout-changed`) over a polling loop — near-free
and responsive; fall back to a low-frequency sweep only if tmux lacks the hook. Gate it behind a
policy that distinguishes a bug-induced faux pane from an operator's **intentional** scratch pane
(a grace period or an opt-out) — silent real-time reaping is a UX hazard, not a win. Prerequisite:
make the reap probe cheaper first (it currently spawns a fresh pwsh + full `Win32_Process` WMI
enumeration per poll), so a daemon loop is not a CPU drain.

## Shuttle `Spec`: generic tools-restriction

**Status:** speculative, unmotivated today.

Meaningless for today's single-session A→B agent (B must always write, and `claudeengine` writes
`settings.json` once at launch for the whole session). This entry's original premise — that it
would matter for cluster reviewers as separate, pure-review sessions gated on the now-superseded
own-window-anchoring dependency — is stale: cluster reviewers are fork subagents running inside the
handler's own session (`useExactTools`, no separate `settings.json` of their own), so this remains
speculative and unmotivated rather than blocked on anything.

## Shuttle `Spec`: per-round provider selector

**Status:** speculative, blocked on a precondition that doesn't exist yet.

Today "provider" means whichever engine is wired into the `Runner`; a selector field is only needed
once a second engine lands (non-Claude engines are not a current priority, per `CLAUDE.md`).

## Bulk-mode clusters + provider-side context caching

**Status:** speculative, not scoped.

A `burler` cluster round can run *tool-use* (each reviewer explores independently) or *bulk* (Go
concatenates target + fasit + rubric into one blob passed as a value). Bulk is what makes
provider-side context caching (e.g. Gemini's explicit cache) pay off, and only if it is modelled as
**one shared prefix + N distinct suffixes**, never N full prompts — that modelling constraint is
what keeps caching possible instead of foreclosed once bulk is eventually built.

## Maintenance

- New long-horizon ideas go here, not into `roadmap.md`, until they're scoped enough to commit to.
- When an idea here becomes a real, scoped plan, move it to `roadmap.md` as a numbered milestone
  and remove (or mark graduated) its entry here.
- Entries here are free-form — no numbering, no required ordering. Don't force one out just because
  it's been sitting a while; only drop an entry if it's been superseded, resolved, or abandoned.
