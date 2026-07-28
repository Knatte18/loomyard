# PATTERN — loomyard's own invariants doc, wired into every agent

> **Status: Design — not built. Planned.** Two clearly separated pieces with different timing: (1) the **wiring** — how an active PATTERN doc reaches every code-touching agent — is buildable **now** (it depends only on already-shipped `fabric` junctions and `internal/stencil`, not on `loom`); (2) the **content migration** — moving loomyard's own invariants out of the mill-owned `CONSTRAINTS.md` into PATTERN — happens **only when loomyard is initialized via lyx itself** (dogfooding), never in this repo now. Per the [documentation lifecycle](../../docs/overview.md#documentation-lifecycle), durable parts fold into the owning package doc when this lands and this file is deleted.

## What PATTERN is (and is not)

PATTERN is **loomyard's own invariants mechanism** — the equivalent of Millhouse's `CONSTRAINTS.md`, but owned by loomyard, from scratch, not a port.

Loomyard does **not** have one today. The `CONSTRAINTS.md` at this repo's root is **Millhouse's** artifact — it exists here only because Millhouse is the tool currently developing loomyard, and mill tooling + `CLAUDE.md` read it every session. It is not loomyard's own mechanism; PATTERN is the missing piece.

Why loomyard needs its own: **lyx initialized in any repo must be able to carry føringer (guidance/invariants) to the agents it spawns there.** PATTERN is that carrier. It matters for two cases:

- **lyx-in-a-client-repo** — a consulting host repo where lyx drives development; the invariants for that repo travel with it, invisibly to the host's own git history (weft-resident — see below).
- **loomyard-via-loomyard (dogfooding)** — when loomyard develops itself onto `loom` instead of onto mill, `CONSTRAINTS.md` becomes worthless (mill is no longer the developer), and loomyard's invariants must by then live in PATTERN.

## Shape: a weft-backed `_pattern/` folder, not a single file

PATTERN is a **directory**, reached from the warp worktree through a `_pattern` junction into `weft` — already anticipated in [fabric-unified-view.md](fabric-unified-view.md) and [finalize.md](finalize.md). It is `_lyx`'s first sibling junction, not a third peer alongside an already-junctioned `_raddle`: `_raddle` carries no junction of its own today, so `_pattern` is the *second* junction `hubgeometry` declares, not one of three. The directory holds:

- **`_pattern/PATTERN.md`** — the index: short two-line entries, one per invariant (the constraint stated in a line, plus a pointer to its detail doc). Never long-form prose inline.
- **`_pattern/<topic>/…`** — a detail submap: one per-topic doc per invariant carrying the full rule / rationale / enforcement. This is the same short-index-plus-linked-detail structure already proven for raddle's `Overview.md` → module docs, and named as the shared pattern in [board-weft-storage.md](board-weft-storage.md).

All PATTERN content lives in `weft`, so it is invisible to the host repo's own git history by construction (this is what supersedes the `CONSTRAINTS.md`-equivalent half of the `host-visibility` item — see [host-visibility.md](host-visibility.md)).

## The wiring: how constraints reach every agent

The problem the wiring solves: an agent lyx spawns (implementer, reviewer, planner, a webster fork, a burler round) must be told to follow the active invariants — the same way `CLAUDE.md` tells a session "Read `CONSTRAINTS.md` before writing or reviewing any code" today.

The mechanism, reusing what already exists:

- **A conditional `stencil` marker** — `{{.pattern_directive}}` — placed in every **code-touching** template (implementer, reviewer, planner, webster fork, burler round). Not in discussion / judge templates, which do not write or review code. (Note: the `<NN>`-style angle-bracket placeholders already in templates are literal text, not stencil syntax; stencil substitutes `{{.X}}` markers.)
- **Go computes the marker value at prompt-assembly time.** A cheap active-check — does `_pattern/PATTERN.md` exist? — decides:
  - **active** → the value is a short directive, e.g. *"Before writing or reviewing any code, read `_pattern/PATTERN.md` and follow every constraint listed there."*
  - **inactive** → empty.
- **A pointer is injected, never the constraints inline.** The directive names the file; the agent reads it itself. This keeps every prompt lean regardless of how large PATTERN grows, and mirrors today's `CLAUDE.md` → `CONSTRAINTS.md` discipline exactly.

A single shared helper in the prompt-assembly path returns the directive-or-empty, called wherever `stencil.Fill` runs on a code-touching template — so the active-check lives in one place, not copied per engine.

## Consequence for `stencil`: an optional (allowed-empty) marker

`stencil.Fill` carries one load-bearing guarantee: **every top-level `{{.X}}` marker must resolve to a non-empty value** — it treats an absent or whitespace-only value as an unfilled marker and fails (`unfilledTopLevelMarkers` in `internal/stencil/stencil.go`). A conditional PATTERN directive that is **empty when inactive** violates that invariant head-on.

So the wiring requires a small, real `stencil` extension: a notion of an **optional marker** — one explicitly allowed to be empty, exempt from the non-empty guarantee. PATTERN is **not** the first conditional token in the system: `websterengine`'s `rename_mechanic` predates it, sitting inside a `{{if .rename_mechanic}}` block in `fork-template.md`, in production before this wiring landed. The optional-marker extension is a deliberate design choice, not a forced one — it puts optionality in Go, where it is testable per call site, and it keeps the "no conditionals in templates" banner rule uniform across every other prompt template, rather than existing because `{{if}}` could not have worked. (The alternative — giving the inactive marker a benign non-empty value like a lone space or a hidden comment — is a hack that pollutes the rendered prompt; prefer the explicit optional-marker concept.)

## Junction wiring and activation

The split is narrower than "`fabric`'s responsibility": `internal/hubgeometry` declares the `_pattern` junction record and owns every `_pattern` path literal (the Hub Geometry Invariant); `internal/fabricengine`'s `seedLyxJunction` (called from `WireJunctions`) materialises the weft-side target and creates the junction itself; `internal/initengine`'s `Init` is the caller that actually wires it for a fresh worktree. `fabric add` explicitly does **not** wire the host junction — its own code states the junction is wired by `lyx init` via `WireJunctions`, not by `add` (junction creation is core, already-shipped `fabricengine` — not the Someday `fabric-unified-view` work, which only *mentions* `_pattern`).

Activation is by **file existence, not junction presence**:

- `initengine.Init`, via `fabricengine.WireJunctions` → `seedLyxJunction`, materialises the `_pattern/` directory in `weft` (possibly empty) and the junction on every fresh `lyx init` — simplest, and a junction needs its target to exist to resolve. No other `WireJunctions` caller (`fabricengine/checkout.go`, `fabricengine/reconcile.go`) materialises a weft directory itself; they only repair or verify an existing junction.
- PATTERN is **active** iff `_pattern/PATTERN.md` is present. A repo with no invariants yet (no `PATTERN.md`) simply has an empty `_pattern/`, the Go active-check returns false, and the directive marker renders empty everywhere. No special "PATTERN not configured" branch anywhere — the file's presence is the whole switch.
- Activation also requires `_pattern` to be listed in `fabric`'s own weft pathspec, or PATTERN content written under `_pattern/` never leaves the machine (weft never commits it). A worktree initialised before this pathspec widened keeps its narrower pathspec and must be widened by hand — the wiring does not retroactively repair an already-initialised worktree's weft commit scope.

## Scope boundary — wiring now, content migration only at init

**In scope now (buildable, `loom`-independent):**

- the `{{.pattern_directive}}` marker in the code-touching templates,
- the `stencil` optional-marker extension,
- the Go active-check + shared directive helper,
- `hubgeometry` declaring the `_pattern` junction record, `fabricengine`'s `seedLyxJunction` materialising the weft dir + junction, and `initengine.Init` calling it.

**Explicitly NOT now — deferred to loomyard-init-via-lyx (the dogfooding transition):**

- moving loomyard's own invariants out of `CONSTRAINTS.md` into `_pattern/PATTERN.md` + detail docs, and retiring `CONSTRAINTS.md`. While mill still develops loomyard, `CONSTRAINTS.md` stays the single live invariants doc (`CLAUDE.md` and mill tooling read it). Authoring PATTERN content in this repo now would just create a redundant second doc under dual maintenance for no benefit until the cutover.

The wiring built now is inert in this repo until an actual `_pattern/PATTERN.md` exists — which is correct: it lets the mechanism ship and be tested (against a fixture `PATTERN.md`) long before any real content migration.

## Open questions

Four of the five questions this section originally posed are settled by the wiring task; one is not, and stays open rather than being silently dropped:

- **Exact template set — settled.** Five templates carry the marker: builder's implementer prompt, burler's round prompt, webster's fork prompt, webster's Master prompt, and loom's plan prompt — the last two admitted by explicit clauses (a context-inheritance root whose in-session forks write code; the author of typed file-op instructions a later code-writing agent executes near-verbatim), not by the naive "reviewer/planner" guess this section originally listed. Discussion is excluded: it emits a decision record the Plan producer re-derives from, so a constraint miss there still has a gate after it.
- **Home of the active-check helper — settled.** `internal/pattern`, a new leaf package — not `stencil` (which stays generic, no PATTERN-specific knowledge) and not `fabric` (which owns the junction, not the prompt-assembly-time check).
- **Directive wording — settled.** Three role variants, as literal Go constants (`RoleImplementer`, `RoleReviewFix`, `RoleOrchestrator`), each an imperative checklist under its own `##` heading rather than a single sentence — the role varies the wording precisely because a reviewer's job (judge, then fix) differs from an implementer's (write) and from an orchestrator's (fork only, never edit).
- **Optional-marker surface in `stencil` — settled.** An explicit allow-list passed as `FillOptional`'s third parameter (`optional []string`), not a naming convention: the caller declares which markers are optional for that specific fill, so the same template can be filled once with a marker required and once with it optional depending on the call site.
- **Detail-submap layout — still open.** Whether `_pattern/<topic>/` has a fixed structure or is free-form per invariant. This is a question about PATTERN's own *content*, not its wiring, and belongs to the content migration this task explicitly defers to loomyard-init-via-lyx (see Scope boundary above) — nothing in the wiring batches settles it.

## Related

- `manifest/roadmap.md` — the Planned `PATTERN.md` item this doc details.
- [board-weft-storage.md](board-weft-storage.md) — establishes that `PATTERN.md` (and all non-warp content) lives in `weft`; names the short-index-plus-linked-detail structure PATTERN reuses.
- [host-visibility.md](host-visibility.md) — its `CONSTRAINTS.md`-equivalent half is superseded by PATTERN-in-weft.
- [finalize.md](finalize.md) — merge-back forwards `_pattern` (like `_raddle`) via a narrowed weft pathspec; PATTERN content is genuinely hand/LLM-authored, so it is the weft-side document-driven conflict path's main real case.
- [fabric-unified-view.md](fabric-unified-view.md) — where `_pattern` is listed among the weft junctions; note this is a *Someday* API-unification item, **not** a dependency — PATTERN needs only base `fabric` junction creation.
- `internal/stencil` — the template-fill leaf the `{{.pattern_directive}}` marker rides on; the optional-marker extension lands here.
- `internal/fabricengine` — materialises the `_pattern` junction (via `seedLyxJunction`), alongside `_lyx`; `_raddle` carries no junction today.
- [raddle.md](raddle.md) — `_pattern`'s neighbor in `weft`; its `Overview.md` → module-docs shape is PATTERN's index-plus-detail precedent.
- Root `CONSTRAINTS.md` + `CLAUDE.md` — the Millhouse mechanism PATTERN mirrors and (only at dogfooding) replaces.
