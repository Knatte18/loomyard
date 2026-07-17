# Handoff — loom build-out, webster v2 schema question

Written for a returning user or a fresh LLM session picking this up after a break.
Reflects wiki state and `main` as of this commit.

## State right now

`main` is green (`go build ./...`, `go test ./...` both clean). Wiki tasks done and merged
this session: `loom-contracts` (#0), `loom-preflight` (#1), `loom-discussion-producer` (#2),
`master-builder` (#7), `codeintel-spike` (#8), `codeintel-multilang` (#9).

Nothing is currently spawned/active. Wiki task list is authoritative — read it via the wiki
client (`_client.list_tasks_brief`/`list_tasks_full`), never by opening `Home.md` directly (see
CLAUDE.md `## Mill wiki`).

## loom build order (docs/roadmap.md milestone 12)

1. Contracts — done
2. Preflight — done
3. Discussion producer — done
4. **Plan producer (#3)** — unblocked, not started. **Hold — see below.**
5. **Phase-machine skeleton (#4)** — unblocked, not started, no dependency on the item below.
6. Session bootstrap (#5) — blocked on #4
7. Finalize (#6) — blocked on #4 (master-builder/#7 already done)

Raddle stays deferred per the roadmap's own framing — not a blocker for loom running end to
end.

**One known gap, not yet a wiki task:** nothing in the roadmap covers writing a task's initial
`_lyx/status.json` seed (the "mill-claim analogue"). `warp` only seeds junctions/git-excludes.
`loom-preflight`'s discussion.md explicitly deferred this. Needs a decision: new build item, or
fold into session bootstrap (#5)?

## Why plan-producer (#3) should wait

Long design discussion this session concluded #3 should **not** start yet, because its output
format (the plan's card schema) is exactly what's in question for a possible webster v2. See
`docs/modules/websterv2.md` (Part A = card-level parallel execution, Part B = structured
impact lookup via LSP — the two former separate docs, merged into one this session).

**Recommended next step:** spawn one new, narrowly-scoped wiki task — call it something like
`webster-plan-schema` — that decides *only* the plan card schema, not the full v2 executor.
Once pinned, #3 can build against it, and #4 (phase-machine) can proceed in parallel regardless
(it doesn't touch plan internals).

### What that schema decision needs to settle (from this session's discussion)

- **No explicit `depends-cards` field.** Instead, cards declare only `changes-files` (writes)
  and `depends-files`/context (reads, including files whose *symbols* the card depends on even
  if never literally re-opened). A Go pass derives the DAG mechanically: a card depends on
  whichever card **most recently wrote** each file in its `depends-files` set (not every card
  that ever touched it); two cards writing the same file get a conflict edge, plan order breaks
  ties. This moves graph-construction into deterministic Go, leaving the LLM only the narrower,
  per-card judgment of "what do I touch" — not "what depends on what across the whole plan."
- **Why not trust LLM-declared dependencies directly:** Millhouse's `mill-plan` has its own DAG
  validation specifically because LLM-declared dependencies are "off" often enough to need it.
  Whatever schema is chosen, validation (cycle detection, file-conflict-vs-declared-edge
  cross-check) must run **at Plan-review time**, not discovered later.
- **The bigger, harder risk — `websterv2.md` §A.7 hazard #1:** implementers often need to touch
  files the plan didn't declare, once real implementation starts. Under today's v1 this is
  cheap (stop, extend the plan, keep going serially). Under a v2 parallel executor it becomes a
  fail-closed worktree abort + re-plan — expensive, and per this session's discussion, likely
  **frequent enough to matter**, not a rare edge case.
- **Before committing to the v2 parallel executor (piece "b" in §A.8),** gather two numbers,
  not just the wave-width case study already in §A.6:
  1. Real wave widths across several completed plans (§A.9's existing decision gate).
  2. How often builder/webster cards today need the "STOP → extend plan" escalation — ideally
     measured both with and without LSP-assisted card authoring (querying `codeintel`'s
     find-references/call-hierarchy when declaring `depends-files`, now that `codeintel-spike`
     and `codeintel-multilang` are both done). LSP assistance should *lower* this rate; it is
     **not** expected to eliminate it (precision limits: interfaces, reflection, generics — see
     `websterv2.md` §B.3).
  - Win (a) alone (planner emits real dependencies, Go computes waves) is cheap and worth taking
    regardless of whether the full parallel executor (b) ever ships.

## Reference docs

- `docs/modules/websterv2.md` — the merged v2 design doc (Part A + Part B).
- `docs/roadmap.md` milestone 12 — loom's pinned build order.
- `docs/reference/discussion-format.md` — the `decision-record.md`/`support-log.md` contract
  `loom-discussion-producer` implements.
- `docs/reference/builder-contract.md` — the v1 cross-module contract `builder`/`webster` share
  (the schema v2 would tighten).
- `CONSTRAINTS.md` — repo invariants, read before touching any module.
