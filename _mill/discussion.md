# Discussion: Board fixes from sandbox run — payload keys, help, rerender

```yaml
task: Board fixes from sandbox run — payload keys, help, rerender
slug: board-sandbox-fixes
status: discussing
parent: main
```

## Problem

The first manual sandbox run of `lyx` (2026-06-28) hit two "hard bugs" in the board
module: `set-phase` appeared not to persist (B1) and `get` always returned `task:null`
(B2). Investigation confirms both are **one root cause plus one masking layer**:

1. **Inconsistent lookup key.** `upsert`/`set-deps` read the **`slug`** key, while
   `get`/`set-phase`/`remove` read **`id_or_slug`**. The agent called
   `set-phase`/`get` with `{"slug":"T1"}`, which unmarshals to `id_or_slug = nil` →
   no task matches → "not found" / `task:null`. (internal/board/cli.go:136-193)
2. **Silent no-op masking.** Even with the key right, `store.SetPhase` returns `nil`
   when the target isn't found ("idempotent: no error for missing task",
   internal/board/store.go:352). So a typo'd target silently "succeeds" with no
   feedback — the second half of why B1 looked like data loss.

The run also surfaced rough edges in the same module: `upsert` silently drops a
`phase` field (W11), board leaf commands have no `--help` payload schema (W1/W10), and
`rerender` leaves stale output files behind when an output filename changes (W13).

**Why now:** this is the first real exercise of the board CLI by an agent, and these
are exactly the footguns that make the tool unusable for autonomous operators. The
tool is pre-release with no external callers to preserve, so we fix the vocabulary and
contracts cleanly rather than papering over them.

This task owns **everything in the board module**. The warp/weft/config rough edges and
the cross-cutting error-format work are a sibling task (not touched here).

## Scope

**In:**

- Unify the single-target lookup contract on `get`/`set-status`/`remove`: accept
  **`slug`** (string) **or** `id` (integer); exactly one required. These commands
  **also reject unknown keys** (so a stale `phase`/`id_or_slug` errors instead of being
  silently ignored).
- Rename the status concept to a single name: command `set-phase` → **`set-status`**,
  payload key `phase` → **`status`**. The on-disk JSON field is already `status` and
  does **not** change.
- Remove the `id_or_slug` key and the `phase` key entirely — **no back-compat aliases**.
- Make `set-status` (and `merge`'s status step) **error** with "task not found" when the
  target does not exist (kill the silent no-op).
- On `set-status`, **require the `status` key to be present**: an explicit `"status":null`
  means "clear the status" (intentional), while an *absent* `status` key is an error
  (`missing required field: status`). This disambiguates a deliberate clear from a typo
  that would otherwise silently clear.
- Make **every board write and lookup payload reject unknown keys** with a clear error —
  `upsert`/`upsert-batch`/`merge` (both `merge`'s top-level keys and its inner `upsert`
  object) **and** `get`/`set-status`/`remove`. This fixes W11's silent `phase` drop,
  catches typos like `titel`, and closes the rename footgun on the renamed commands
  (a stale top-level `set_phase` on `merge`, or `phase` on `set-status`, errors loudly
  instead of being silently dropped).
- Allow `status` as a legitimate `upsert` field (so a task can be created with a status
  in one call) — it already round-trips correctly; the unknown-key rejection is what
  makes a stray `phase` an error instead of a silent drop.
- Convert `merge`'s positional `set_phase: [id_or_slug, phase]` to an object
  `set_status: {"slug"|"id": …, "status": …}`.
- Add a `Long` help block to every board leaf command (`upsert`, `upsert-batch`,
  `set-status`, `remove`, `get`, `merge`, `set-deps`) documenting required/optional
  fields and an example JSON payload, reflecting the **new** schema.
- Generalize `render.go` output cleanup to a **manifest** mechanism that removes any
  previously-rendered board file the current render no longer produces — covering
  `home`/`sidebar` renames as well as orphaned proposals (W13). This replaces the
  existing proposal-only glob cleanup.
- Update `cmd/lyx/helptree_test.go` pinned board subcommand set (`set-phase` →
  `set-status`) in the same commit.

**Out:**

- The on-disk `tasks.json` schema / `Task` struct field names — `status` stays
  `status`; no migration.
- Numeric ID as an *upsert* field — `id` remains auto-assigned; passing `id` to
  `upsert` is rejected (it is only a *lookup* key on get/set-status/remove).
- `list`/`list-full`/`sync`/`rerender` payload contracts (no lookup key; unchanged
  except rerender inherits the new cleanup behavior via the shared write path).
- warp/weft/config CLI help and the cross-cutting error-format work (sibling task).
- Any back-compat shim for `set-phase`/`phase`/`id_or_slug` — deliberately dropped.

## Decisions

### status-is-the-one-name (A)

- Decision: The task lifecycle field has exactly **one** operator-facing name: `status`.
  Rename the command `set-phase` → `set-status` and the payload key `phase` → `status`.
  The stored JSON field is already `status` and is left unchanged.
- Rationale: The board values (`active`, `done`, `pr-pending`, `ready-to-merge`,
  `abandoned`) are lifecycle *states*, which "status" names accurately; "phase" better
  describes mill's process steps (`discussing → planning → …`). `status` already appears
  on disk (tasks.json), in `render.go`, in `BriefTask`, and in the wiki task record
  (`status: active`), so making it canonical is the lowest-churn, most-consistent choice.
- Rejected: Keeping both names (`status` field + `phase` alias) — two names for one
  concept is exactly the W11 bug surface. Renaming the on-disk field to `phase` — large
  migration (tasks.json schema, render switch, BriefTask, all fixtures) for cosmetics.

### no-back-compat (B)

- Decision: `set-phase`, the `phase` key, and the `id_or_slug` key are removed outright.
  No hidden/deprecated aliases.
- Rationale: `lyx` is pre-release (the 2026-06-28 sandbox run was the first manual
  exercise); no internal Go code shells out with these payloads (only tests do). `phase`
  was a slip, not a contract. A clean break is correct and avoids carrying euphemisms.
- Rejected: Hidden deprecated aliases — would keep the second name alive in code,
  contradicting the one-name principle, for no real caller's benefit.

### slug-or-id lookup (Q1/Q5)

- Decision: Single-target commands (`get`, `set-status`, `remove`) accept a payload with
  **either** `slug` (non-empty string) **or** `id` (integer) — exactly one required;
  error if neither or both are present. `merge`'s status step becomes
  `set_status: {"slug"|"id": …, "status": …}`.
- Rationale: `slug` and `id` are two genuinely different identifiers (text name vs.
  auto-assigned number), not two names for one concept — so two honest keys respect the
  one-name principle. The user wants to reference a task by its number (e.g. "task #096"):
  an agent runs `lyx board get '{"id":96}'`, reads the slug from the result, and uses the
  slug for everything downstream (`depends_on`, `remove_slugs`, `set-deps` all key on
  slug). The slug is always visible in `Home.md`, so this is purely an ergonomic addition.
- Rejected: Keeping the vague `id_or_slug` single key — the name is a euphemism and the
  cross-command inconsistency it caused (`slug` vs `id_or_slug`) was the B1/B2 root cause.
  `slug`-only — would drop the wanted numeric-lookup ergonomic.

### error-on-missing-target (Q2)

- Decision: `store.SetStatus` (renamed from `SetPhase`) returns a "task not found: …"
  error when no task matches; `set-status` surfaces it, and `merge` fails atomically if
  its status target is missing. `remove` already errors on missing — unchanged.
- Rationale: The silent no-op was the masking half of B1; operators got zero feedback on
  a typo'd target. Erroring is the only way the CLI tells the truth. No production caller
  relies on idempotent set-phase (only tests), so the change is safe.
- `status` key required on `set-status`: because `status` is a nullable field, an absent
  key and an explicit `null` would otherwise both decode to nil and be
  indistinguishable. The contract is: `"status":null` clears the status (intentional);
  an absent `status` key is an error (`missing required field: status`). Combined with
  unknown-key rejection, this means `set-status '{"slug":"x","phase":"done"}'` errors
  twice over (unknown `phase` + missing `status`) rather than silently clearing.
- Rejected: Keeping the idempotent silent no-op — re-arms the exact footgun this task
  exists to remove. Treating absent `status` as a clear — cannot distinguish a typo from
  a deliberate clear.

### reject-unknown-keys (C)

- Decision: **Every** board write and lookup payload rejects unknown keys with a clear
  error naming the offending key — no command silently ignores a stray field.
  - **Upsert fields** (`upsert`, `upsert-batch`, and `merge`'s inner `upsert` object):
    allowed set `{slug, title, depends_on, isolated, deferred, brief, body, status}`.
    (`group` is already rejected; fold it into the same mechanism. `id` and `phase` are
    therefore rejected on upsert.)
  - **`merge` top-level keys:** allowed set `{remove_slugs, upsert, set_status}` — a
    stale top-level `set_phase` errors instead of being silently dropped (which would
    skip the status step with no feedback).
  - **Single-target lookup payloads** (`get`, `set-status`, `remove`): allowed set is
    `{slug, id}` for `get`/`remove` and `{slug, id, status}` for `set-status` — a stale
    `phase`/`id_or_slug` errors instead of leaving `Status` nil and silently clearing.
- Rationale: "No silent drop" (W11) generalizes: the JSON round-trip in
  `NewTask`/`ApplyPatch` and the typed single-target structs currently ignore *all*
  unknown fields, so `phase`, `titel`, and any typo vanish silently. The most dangerous
  case is the renamed commands: a `phase` left on `set-status` or a `set_phase` left on
  `merge` would re-arm the exact silent-no-op this task exists to kill. A strict
  allowlist on every payload turns every such mistake into immediate, actionable
  feedback. A friendly hint for the common `phase` case ("unknown field 'phase'; did
  you mean 'status'?") is a nice-to-have.
- Placement: the upsert-fields allowlist is enforced **at the store boundary**
  (`Store.UpsertTask` / `Store.UpsertTasksBatch` / `Store.MergeTasks`), before the JSON
  round-trip in `NewTask`/`ApplyPatch`, so a single chokepoint covers create, patch, and
  the merge-upsert path uniformly. The top-level `merge` and single-target lookup
  allowlists are enforced where those payloads are parsed (the cli.go RunE / facade
  boundary), since they never reach the upsert chokepoint.
- Rejected: Special-casing only `phase` — leaves every other typo silently dropped.
  Scoping rejection to upsert paths only — leaves the rename footgun live on
  `set-status` and `merge`'s top-level (the round-1 review gap). Doing nothing — fails
  the W11 "no silent drop" requirement.

### manifest-cleanup (Q6 / W13)

- Decision: Replace `removeOrphanProposals` (proposal-prefix glob only) with a manifest:
  on each render, write a sidecar in the board dir (e.g. `.board-rendered.json`) listing
  the filenames the render produced; before/after writing, remove any file named in the
  *previous* manifest that the *current* render no longer produces. This covers `home`
  and `sidebar` renames and proposal-prefix changes, not just orphaned proposals.
- Rationale: `render.go` already owns cleanup, so this *replaces* one mechanism with a
  more general one rather than adding a parallel one. It only ever deletes files the
  board itself previously wrote (listed in its own prior manifest), so it can never touch
  unrelated wiki pages (a hand-added `README.md` was never in the manifest). The
  manifest file lists only rendered `.md` outputs (never itself), so it is never a
  deletion candidate.
- Failure modes (best-effort, matching today's `removeOrphanProposals` which never fails
  a write): a **missing** manifest — including an existing board upgraded from a
  pre-manifest version (it has `Home.md` but no sidecar) — means "nothing known to clean
  up"; the render simply proceeds and **seeds** the manifest with the current output set
  (removing nothing on that first pass). A **corrupt/unreadable** manifest is treated the
  same as missing (degrade gracefully, log nothing fatal) and is overwritten by the
  current set. Manifest read/write errors never fail the write — the rendered `.md` files
  and `tasks.json` are the source of truth; a stale file left behind is harmless and gets
  cleaned on the next successful render.
- Rejected: Sweep (delete any top-level `*.md` not in the current render set) — would
  delete unrelated wiki files; unsafe. Narrow sidecar for only `home`/`sidebar` — leaves
  two cleanup mechanisms; the manifest unifies them for free. Dropping W13 — the user
  chose to fix it since render already has cleanup logic to generalize.

## Technical context

Board module lives in `internal/board/`. Key files and what changes:

- **`cli.go`** — the cobra tree (`Command()`), 11 subcommands. This is where the
  payload structs and command names live. Changes: rename `setPhaseCmd` → `set-status`
  (`Use`, `Short`, RunE); change its payload struct from `{IDOrSlug any; Phase *string}`
  to a `{Slug *string; ID *int; Status *string}` shape with exactly-one-of `slug`/`id`
  validation; same lookup-shape change for `get` and `remove`; change `merge`'s
  `SetPhase []any` to a `set_status` object; add `Long` to all seven leaf commands;
  route unknown-key rejection through the store/task layer. Note the existing
  `outputError`/`output.Ok` envelope helpers (cli.go:328-362) — reuse them; emit one
  JSON object per line via `internal/output`.
- **`board.go`** — facade methods (`SetPhase`→`SetStatus`, `GetTask`, `RemoveTask`,
  `MergeTasks`). Signatures change from `idOrSlug any` to an explicit slug/id selector
  (mirror whatever resolver shape cli.go parses). `writeOp` (board.go:46) is the locked
  write sequence; step (5) calls `RenderToDisk` — the manifest cleanup hooks in there,
  so `rerender` (which goes through `writeOp` with a no-op mutate, board.go:166) inherits
  it automatically.
- **`store.go`** — `GetTask`/`RemoveTask`/`SetPhase` switch on `any` (int/float64/string).
  Replace with explicit slug-vs-id resolution. **`SetPhase` → `SetStatus` must error on
  missing target** (store.go:335-354). `UpsertTask`/`UpsertTasksBatch`/`MergeTasks` are
  the **single chokepoint** for the upsert-fields allowlist: validate the incoming
  `fields` map keys against `{slug, title, depends_on, isolated, deferred, brief, body,
  status}` here, before the `NewTask`/`ApplyPatch` round-trip, so create + patch + merge-
  upsert are covered uniformly. JSON numbers from payloads arrive as `float64`; the `id`
  lookup must handle the int/float64 boundary.
- **`task.go`** — `NewTask`/`ApplyPatch` do the JSON round-trip that silently drops
  unknown fields (task.go:56-65, 94-99). The upsert allowlist is enforced *upstream* at
  the store boundary (above), not here — but the existing `group` rejection
  (task.go:30-32, 77-79) should be folded into that same allowlist so there is one error
  path, not two. `Task` struct field tags are unchanged (`status` stays `status`).
- **Single-target + merge top-level validation** lives at the cli.go RunE / facade
  boundary (those payloads never reach the store upsert chokepoint): `get`/`remove`
  accept only `{slug, id}`; `set-status` accepts only `{slug, id, status}` and requires
  `status` present; `merge` accepts only top-level `{remove_slugs, upsert, set_status}`.
  Decode into a `map[string]any` first to detect unknown keys (Go's typed-struct decode
  silently ignores them), then validate, then bind the typed fields.
- **`render.go`** — `RenderToDisk` (render.go:23) and `removeOrphanProposals`
  (render.go:41) are the cleanup seam. `Render` returns `map[filename → content]`
  (render.go:58); that map's keys are the current render set. Manifest = persist those
  keys; on next render diff old vs new and `os.Remove` the dropped ones. `Outputs` carries
  `Home`/`Sidebar`/`ProposalPrefix` (config.go:31-44); the output filenames come from
  config (template.yaml defaults: `Home.md`, `_Sidebar.md`, `proposal-`). Manifest
  read/write is best-effort like the current `removeOrphanProposals` — wrap in the same
  never-fail-the-write discipline; a missing or corrupt manifest is treated as "nothing
  to clean" and reseeded from the current render set.
- **`cmd/lyx/helptree_test.go`** — board `wantSubs` (line 50-53) pins `"set-phase"`;
  change to `"set-status"` in the **same commit** (CLI/Cobra Invariant + task-completion
  rule).
- **`cmd/lyx/drift_test.go`** — `TestDriftGuard_AllCommandsHaveShort` walks the tree; the
  renamed `set-status` must carry a non-empty `Short` (carry over "Set or clear the
  status of a task").

Reference draft: `git show c9d5c59 -- internal/board/cli.go` is a reverted first cut of
the `Long` blocks (item W1). **Use it as a starting point but correct it** — that draft
predates these decisions and is wrong in several ways: it documents a `"group"` field as
allowed (NewTask rejects it), uses the now-removed `id_or_slug`/`phase` keys, and lists
`"group"` for upsert. Rewrite the `Long` blocks against the final schema.

Docs: there is **no** `docs/modules/board.md` (only loom/mux/review/shuttle). The board
CLI's `--help` *is* the discovery-path documentation per the CLI/Cobra Invariant, so the
`Long` blocks are the primary doc deliverable. `docs/roadmap.md:62` and `docs/overview.md`
mention board but describe it as "done foundation"; update them only if the module
table or execution-stack description actually shifts (a CLI command rename + payload
schema change is observable behavior — confirm whether roadmap/overview need a note in
the same commit per the task-completion rule).

## Constraints

From `CONSTRAINTS.md` (read in full; the CLI/Cobra Invariant is the load-bearing one):

- **CLI / Cobra Invariant.** Every command keeps a non-empty `Short`; commands whose
  `--help` is the discovery path (all board leaf commands) get a `Long` with concrete
  examples. The module seam (`Command()` / `RunCLI` = `clihelp.Execute(Command(), …)`)
  is unchanged. Output stays on the `internal/output` JSON envelope (`output.Ok` /
  `output.Err`), one object per line. Help is co-located on each command — no central
  table. The help tree is pinned: update `helptree_test.go` `wantSubs` for the
  `set-phase`→`set-status` rename in the same commit.
- **Path Invariant.** Any board/config/`_lyx` path goes through `internal/paths`
  helpers, never string literals — applies to the manifest sidecar path too if it is
  derived from board geometry (it lives inside the already-resolved `boardPath`, so
  `filepath.Join(boardPath, …)` is fine, mirroring `writeLockFile`/`.swaplock`). No raw
  `os.Getwd` / `git rev-parse` outside `internal/paths` + `cmd/lyx/main.go`.
- **Task-completion docs discipline.** Observable CLI behavior changes here (command
  rename, payload schema), so docs/roadmap updates (if the module table/stack shifts)
  land in the same commit as the code.

## Testing

TDD candidates (write the failing test first, then implement):

- **Lookup contract (regression for B1/B2).** `get`/`set-status`/`remove` succeed with
  `{"slug":"…"}`; succeed with `{"id":N}`; error clearly with neither key; error with
  both keys. Cover the int-vs-float64 JSON-number boundary for `id`.
- **set-status error on missing target (regression for B1).** `set-status` on a
  non-existent slug/id returns a "task not found" error (not a silent success). Same for
  `merge` when its `set_status` target is absent → whole merge fails atomically and
  leaves the store unchanged.
- **Existing idempotency test must flip.** The current `store_test.go` test asserting
  `SetPhase` is a silent no-op on a missing task must be updated to assert the new error
  behavior (it is an intended contract change, not a regression).
- **Reject unknown keys — upsert paths (W11).** `upsert`/`upsert-batch`/`merge`'s inner
  `upsert` with a stray `phase` key error (not silently drop); a stray typo key (`titel`)
  errors; `group` still errors via the same path; a payload using only allowed keys
  (`status` included) succeeds and the `status` value is persisted.
- **Reject unknown keys — single-target + merge top-level (round-1 review gap).**
  `set-status '{"slug":"x","phase":"done"}'` errors (does not silently clear `status`);
  `get`/`remove` with a stray `id_or_slug`/`phase` key error; `merge` with a stale
  top-level `set_phase` key errors (does not skip the status step silently).
- **`set-status` requires `status` present.** `set-status '{"slug":"x"}'` (no `status`)
  errors with `missing required field: status`; `set-status '{"slug":"x","status":null}'`
  succeeds and clears the status (distinct outcomes for absent vs. explicit null).
- **status round-trips on upsert.** `upsert '{"slug":"x","status":"active"}'` creates a
  task whose stored `status` is `active` and renders the `[active]` badge in `Home.md`.
- **Manifest cleanup (W13).** Render once with `home: Home.md`; change config to
  `home: Index.md`; rerender; assert `Home.md` is gone and `Index.md` exists. Same for a
  `sidebar` rename and a `proposal_prefix` change. Assert an unrelated hand-added file
  (e.g. `README.md`) in the board dir is **never** removed. Keep the existing
  orphaned-proposal cleanup behavior green (a task losing its body still removes its
  proposal file) — now via the manifest path.
- **Manifest degradation (round-1 review note).** A board with rendered files but **no**
  manifest (pre-upgrade state) renders without error and seeds the manifest, removing
  nothing on that first pass. A **corrupt/unreadable** manifest does not fail the write —
  it is treated as absent and overwritten by the current render set.
- **Help schema (W1).** `--help` for each of the seven leaf commands contains the
  documented field schema (assert the new field names: `slug`/`id`/`status`, and absence
  of `id_or_slug`/`phase`/`group`).
- **Help tree pin.** `helptree_test.go` passes with `set-status` in `wantSubs`;
  `drift_test.go` passes (renamed command has a `Short`).

Follow `golang-testing` conventions; the board suite is the behavior guardrail —
existing passing tests for unaffected paths must stay green.

## Q&A log

- **Q:** Standardize on `phase` or `status` as the one name? **A:** `status` — values are
  lifecycle states, and `status` already exists on disk / in render / in the wiki record;
  rename `set-phase`→`set-status`, key `phase`→`status`, leave the on-disk field alone.
- **Q:** Keep any back-compat for `set-phase`/`phase`/`id_or_slug`? **A:** None. `phase`
  was a slip; lyx is pre-release with no internal callers. Clean break.
- **Q:** Drop numeric lookup, or keep it? **A:** Keep it as a real feature, but via an
  honest separate `id` key (not `id_or_slug`). `get`/`set-status`/`remove` accept `slug`
  OR `id`; an agent resolves "#096" → slug via `get '{"id":96}'`, then uses the slug.
- **Q:** How far does "no silent drop" reach? **A:** All the way — reject *any* unknown
  key on upsert/upsert-batch/merge (allowlist), not just `phase`.
- **Q:** Is W13 (stale output on filename change) worth fixing? **A:** Yes — it's an edge
  case, but `render.go` already has cleanup logic, so generalize it to a manifest that
  removes any previously-rendered file the current render no longer produces.
- **Q:** Could the cleanup delete unrelated wiki files? **A:** No — the manifest only
  records files board itself rendered, so cleanup only ever removes board-written files;
  hand-added pages are never in the manifest.
- **Q (review r1):** Does strict-key rejection cover the renamed single-target commands?
  **A:** Yes — extend it to *every* board write and lookup payload, not just upsert. A
  stale `phase` on `set-status` or `set_phase` on `merge` must error, else the silent
  no-op the task kills is re-armed on the most-renamed commands.
- **Q (review r1):** Absent vs. null `status` on `set-status`? **A:** `"status":null`
  clears (intentional); absent `status` key is an error. They must be distinguishable.
- **Q (review r1):** Where does the upsert allowlist live? **A:** One chokepoint at the
  store boundary (`UpsertTask`/`UpsertTasksBatch`/`MergeTasks`) before the JSON round-trip,
  covering create + patch + merge-upsert; fold the existing `group` rejection into it.
- **Q (review r1):** Manifest failure modes? **A:** Best-effort like today's proposal
  cleanup — missing/corrupt manifest degrades gracefully (never fails a write) and is
  reseeded from the current render set.
