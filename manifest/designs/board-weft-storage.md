# board — move storage to `weft:main`

> **Status: Design — not built.** Supersedes the currently-shipped `board`'s storage model
> (`internal/boardcli` + `internal/boardengine`, which uses board's own separate remote repo —
> the planned "init: board-repo creation from scratch" item this replaces). Also supersedes an
> intermediate idea (GitHub wiki as board's storage/rendering). Does not change anything about
> `codeintel`, `raddle`, or the card/plan-format work described elsewhere. Per the
> [documentation lifecycle](../../docs/overview.md#documentation-lifecycle), durable parts fold
> into `internal/boardengine`'s package doc when this lands and this file is deleted.

## Why the earlier approaches were rejected

- **A separate third repo for `board`** (today's shipped model): works, but is extra
  repo-management overhead for something that doesn't need its own git identity.
- **GitHub wiki as board's storage/rendering** (an intermediate idea, also rejected): wiki
  functionality requires the repo to be **public** on GitHub's free tier — a real, disqualifying
  problem for consulting work. A private client repo's weft companion would have to go public
  just to get wiki rendering, exposing what is effectively the operator's own private "workbook"
  content.

## The design: one repo (`weft`) for everything lyx-related

Everything that isn't `warp` (the host repo's own content) already lives in `weft` — raddle,
`PATTERN.md` (a future, loomyard-owned CONSTRAINTS.md-equivalent — see the roadmap's Someday
list), per-task discussion/plan artifacts. **Board joins that same repo**, on a reserved branch,
rather than getting its own
repo or relying on GitHub wiki.

## Branch naming convention (uniform, no special cases)

- **Warp branch:** `<slug>`
- **Weft branch:** `<slug>-weft`

This is the *only* rule — applied identically to every task, including the primary/long-lived
worktree ("prime" — not yet a settled name in the weaving vocabulary). No branch-naming
exceptions anywhere in the system. **This enforcement lives in [fabric.md](fabric.md)** (the
module that owns branch creation, since it absorbs today's `warp` topology responsibilities in
full) — board's design here simply depends on it being followed without exception.

**Consequence:** no task's weft branch can ever be named exactly `main` (every paired weft
branch carries the `-weft` suffix). That makes `weft:main` permanently unclaimed by the pairing
convention — **reserved exclusively for board**, never at risk of collision with any task's own
branch, now or in the future. (An earlier variant — reusing `weft:main` as prime's own paired
branch under a special name like `main-prime` — was considered and rejected: it would mix
board's history with a per-task companion-branch history on the same line, a fragile combination
even if unlikely to actually collide in practice.)

## Rendering, privacy-safe

Board's front page is a plain `README.md` at the root of `weft:main`. GitHub renders `README.md`
automatically for **any** repo, private or public — unlike wiki, this requires no visibility
change. Solves the consulting-privacy concern completely, not just partially.

## The "prime" worktree's asymmetric relationship to weft

The long-lived, primary worktree ("prime") is the only worktree with a reason to check out
**two** weft branches simultaneously:

- `weft:main-weft` — prime's own ordinary companion branch, following the standard pairing rule
  unchanged (warp `main` ↔ weft `main-weft`). Prime's own private raddle/`PATTERN.md`/scratch
  content lives here, exactly like any other task's companion branch.
- `weft:main` — a separate, **never paired with any warp branch**, checked out solely for board
  access.

No other worktree checks out `weft:main` directly or clones/branches its own copy of it.

## How other worktrees reach board

Every other worktree reaches board through a **portal** (the existing junction-based mechanism
already used for raddle — a subdir-mirrored view into another worktree's files) pointing at
**prime's** `weft:main` checkout. Deliberate, not incidental:

- **Avoids the clutter problem by construction.** Since no other worktree's weft branch is ever
  `main`, board's files are never checked out/cloned into any task worktree in the first place —
  nothing to sparse-checkout around, nothing to accidentally leave stale copies of. (A
  sparse-checkout-based fix was considered and rejected in favor of this — it would have required
  explicit per-worktree configuration the naming convention avoids needing entirely.)
- **Avoids a staleness/visibility problem, not just a cosmetic one.** If board data were instead
  forked into each task's own weft branch (checked out once at task start), it would become a
  frozen snapshot from that moment — two concurrent tasks would each see (and could each
  independently, invisibly-to-each-other diverge) their own stale copy, defeating board's entire
  reason for existing ("immediately visible across worktrees"). Routing all access through one
  physical location (prime's checkout, reached via portal, no branching/cloning involved)
  preserves true real-time shared visibility.

## `fabric` consequence

`fabric`'s design (see [fabric.md](fabric.md)) assumes one warp↔weft pair per task, with a
`Warp-SHA` commit trailer in weft recording which warp SHA a weft commit corresponds to. **This
still applies unchanged to every ordinary `<slug>`/`<slug>-weft` pair.** Prime's `weft:main`
checkout is explicitly **outside** this mechanism — it has no corresponding warp branch, so the
trailer/correspondence-index machinery (`RecordCorrespondence`/`WeftSHAForWarpSHA`) does not
apply to it. Board-related reads/writes to `weft:main` are a separate, standalone concern, not
routed through `fabric.SyncWeft`/`fabric.RevertWithWeft`.

## Board's data model

A rendered front page (`README.md`, generated) backed by a small JSON database, with four
categories:

1. **Proposals** — raw, uncurated. Anyone can add an entry: any worktree's LLM session (via a
   lightweight direct-write command), or a human via a GitHub issue on the `weft` repo (read/
   triaged periodically by the orchestrating thread — see Curation flow below). Each entry
   records its source (`issue-86`, `worktree:<slug>-session`, etc.) for later traceability.
2. **Manifest** — curated. Short bullet-point entries only, each linking to a longer proposal/
   rationale file for detail — the same short-index-plus-linked-detail pattern already proven
   for `PATTERN.md`/`CONSTRAINTS.md` and for raddle's `Overview.md` → module docs. Never
   long-form prose inline.
3. **Tasks** — forward-committed work, extracted from the manifest. Format directly modeled on
   the existing, long-proven Millhouse-wiki task-list pattern (short id/title, one-line
   description, `depends-on`, link to a longer proposal file, status/layer grouping) — a port of
   something already working, not a new format.
4. **Done** — archive, periodically cleaned (same discipline already practiced in the Millhouse
   wiki).

## Curation flow

- **Anyone can add a raw proposal.** No gatekeeping at intake — deliberate, since requiring every
  spontaneous idea to go through a single owner would create friction and lose ideas from
  discusser/planner sessions in other worktrees.
- **Only the orchestrating thread (running in prime) curates proposals into the manifest.** This
  is where LLM judgment is actually needed (is this proposal coherent, well-formed, worth
  keeping) and where consistency of voice/format matters.
- **Task extraction from manifest is a deliberate, explicit command** (a skill, invoked by the
  operator, not an autonomous background loop): "extract a logical next task from the manifest."
  Atomically removes (or marks superseded — history worth keeping, not silently dropped) the
  source manifest entry and creates the corresponding task entry in one operation, mirroring the
  "advance only on confirmed success" transactional principle already used for `SnapshotSHA`
  elsewhere in the design (see [fabric.md](fabric.md)). Kept human-triggered for now rather than
  autonomous, consistent with this project's general pattern of starting cautious and only
  removing human-in-the-loop steps once behavior is observed and trusted.
- Once a task exists in board, everything downstream is already-designed, unchanged `loom`
  machinery (discussion → plan → webster → finalize) — task creation is a new *intake* into an
  existing pipeline, not a new pipeline.

## Note on this manifest folder's own name

Board's "Manifest" category (short curated bullets + linked detail) is conceptually the same
pattern this repo's own top-level `manifest/roadmap.md` already follows. Worth keeping in mind
when board actually ships: the file-based `manifest/` convention here may become a precursor to,
or eventually be superseded by, board's JSON-backed Manifest category — not a coincidence, likely
the same idea arrived at independently.

## Related

- [fabric.md](fabric.md) — owns the branch-naming enforcement this design depends on.
