# Long-term ideas

Speculative, longer-horizon design ideas that are **not** yet planned milestones. `roadmap.md` is
reserved for planned milestones only (see its Maintenance section) — this file is where a
promising direction gets written down *before* it's mature enough to become one, so it isn't lost
and doesn't clutter the roadmap with maybes. Entries here have no schedule and may never be built.
When an idea here matures into a concrete, scoped plan, it graduates to a numbered milestone in
`roadmap.md` (and this entry should be removed or marked graduated).

## Webster: parallel batches via a DAG (further-out, beyond the sequential fork model)

**Status:** speculative, not scoped. The sequential fork-based module this would extend has
graduated out of this file — see `roadmap.md` milestone 26 / wiki task `master-builder`. This
entry is only the riskier remainder that wasn't picked up with it.

Today's plan format is deliberately a flat ordered list with **no DAG** (see
`modules/plan-format.md` and `roadmap.md` milestone 12 — "task-level parallelism via separate
worktrees + `lyx run`, not intra-plan"). Running independent batches as *parallel* forks would
require reintroducing a DAG into the plan, which reopens exactly the problem that decision
avoided: two forks writing to the same worktree at the same time can collide on disk. That's only
safe if either (a) the DAG's independence edges actually guarantee no file overlap between
parallel batches — a guarantee the DAG would need to prove, not just assume — or (b) each parallel
batch gets its own worktree/branch, which reintroduces the multi-branch merge complexity the
no-DAG decision was specifically chosen to avoid. Not pursued further until webster's
sequential model is real and this looks worth the complexity.

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
