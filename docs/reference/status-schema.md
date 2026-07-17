# Status schema — loom's spawn/handover status file

> **Status: Contract — pinned.** This doc pins the `_lyx/status.json` schema: loom's
> single source of truth for orchestration state, and the t=0 "seed" a spawn-time lyx
> command hands off to loom. Durable reference doc — kept, not deleted on landing — the
> loom analogue of [builder-contract.md](builder-contract.md) and
> [plan-format.md](plan-format.md).

## What it is

`_lyx/status.json` is loom's single source of truth for orchestration state: current
phase, current review sub-state, the phase-level outcome trail, and the human-readable
narration `lyx loom status --watch` prints. `lyx loom run` rewrites it on every step; its
t=0 "seed" — the handoff instant a task is spawned and given to loom, before any
`lyx loom run` has executed — is written once at spawn time (see
[The seed / handover](#the-seed--handover) below).

It is durable **weft-overlay state**: it lives under `_lyx/` (git-synced via weft, not
`.lyx/`'s ephemeral machine-local state), which is what makes resume work across
machines. Its path resolves via `internal/hubgeometry` once code lands — this doc
describes the file, it does not construct the path.

## Format decision (defended)

The file is **JSON via the existing `internal/state` primitive** (`WriteJSON[T]`/
`ReadJSON[T]`: locked, atomic, typed) — the same mechanism `builder` uses for its own
`state.json`.

This **overrides the board brief's "plain YAML."** The brief's real point was
"structured, not markdown-with-frontmatter" — and that point stands, unchanged. But
JSON was chosen over YAML deliberately: `_lyx/status.json` is machine-written,
machine-read orchestration state, not something a human is expected to hand-edit, and
`lyx loom status --watch` pretty-prints it for humans — so the on-disk file need not be
hand-readable. Reusing `internal/state` gives locking and atomic writes for free and
keeps one state primitive across modules, rather than a second one-off for loom.

## The seed / handover

The **seed** is the t=0 contents of `_lyx/status.json` at the instant a task is spawned
and handed to loom — not a separate file or a separate schema, just the initial
snapshot of the same file loom then keeps rewriting (see
[status-single-schema-superset](#the-schema)).

It is written by a **lyx Go command** at spawn time — the mill-spawn analogue, but Go,
never an agent. This doc names the *role* ("the spawn-time lyx command"), not the exact
subcommand; which one it binds to (`warp add` vs a dedicated `lyx loom init`/`spawn`) is
pinned when that command lands. An optional thin `ly-spawn` skill may wrap it later for
convenience, but the Go command is always the writer.

loom's Preflight **requires the file to exist** and fails loud if it is missing — the
file's existence *is* the handoff signal, consistent with Preflight's other
precondition checks (clean worktree, weft pairing in sync, no half-finished prior run).

**One schema, a superset — not two.** The seed is the same schema as the ongoing status
file, with only the handoff fields populated (`slug`, `parent`, `phase: "discussion"`,
an initial `narration`) and everything else at its zero/null value (`history: []`,
`start_sha: null`, `pause_requested: false`, `next_action: null`). loom fills the rest
as it runs; there is no seed→full conversion step.

## The schema

```jsonc
{
  "slug": "loom-contracts",        // board-task pointer (board owns title/description)
  "parent": "main",                // parent branch
  "phase": "builder",              // preflight | discussion | plan | builder | raddle | finalize | done
  "stage": "gate",                 // "produce" | "gate": producing the artifact vs in its review gate
  "narration": "now: … / last: … / wait: …",  // human line for `lyx loom status --watch`
  "history": [                     // per-phase outcome trail (per-round verdicts live in perch's block files)
    { "phase": "discussion", "outcome": "approved", "ts": "2026-07-17T10:01:30Z" },
    { "phase": "plan", "outcome": "stuck", "bounced_to": "discussion", "ts": "2026-07-17T11:14:02Z" }
  ],
  "start_sha": null,               // host HEAD stamped when Builder begins (Raddle diff base)
  "pause_requested": false,        // pause flag kept IN-status (diverges from builder, which uses a separate flag file)
  "next_action": null              // when loom yields at a human gate: what the human must do next
}
```

Per-field notes:

- **`slug` / `parent`** — the only handoff pointers into the wider task record; the
  board owns durable title/description, not this file.
- **`phase`** — the phase enum from loom's phase machine, plus the terminal `done`:
  `preflight | discussion | plan | builder | raddle | finalize | done`.
- **`stage`** — `produce | gate`: whether the current phase is mid-produce or mid-gate.
  Kept because this file is loom's single total overview of *where it is* and loom
  needs produce-vs-gate for resume; the finer per-round detail stays in perch's block
  files (see [Parse discipline](#parse-discipline) and history below).
- **`narration`** — one composed human string with `now:`/`last:`/`wait:` segments.
  loom writes it, the `lyx loom status --watch` strand prints it; mux never parses it.
- **`history`** — a **per-phase outcome trail**: one entry per phase attempt
  (`{phase, outcome: approved | stuck, bounced_to?, ts}`), including stuck-handler
  bounce-backs. Per-*round* verdicts are **not** duplicated here — those live in
  perch's block files, since the progress-judge that needs them lives inside perch and
  reads perch's own files directly. `bounced_to` is present only on a `stuck` entry
  that routes back to an earlier phase.
- **`start_sha`** — the host `HEAD` stamped when Builder begins, so Raddle can diff
  `start_sha..HEAD`. `null` until Builder starts.
- **`pause_requested`** — the pause flag, kept **in-status**. This diverges from
  `builder`, which uses a separate pause *flag file* — called out here deliberately so
  the divergence reads as a decision, not an inconsistency.
- **`next_action`** — a dedicated, machine-checkable field for "is this blocked on a
  human, and on what?", set whenever loom yields at a human gate. It is also reflected
  in `narration`'s `wait:` segment, so a human reading the narration sees the same
  fact in prose.
- **No `schema_version`/`format` field.** Unlike `plan-format.md` (which needs
  `format:` because `builder` mechanically validates plans across a real v1→v2 bump),
  this file has a single writer (loom itself) and no version-compatibility pressure.
  A version stamp here would be a rarely-exercised guard that goes stale; it is
  deliberately omitted, to be reintroduced only if a real incompatibility ever forces
  it.
- **Timestamps.** Every timestamp field (currently only `history[].ts`) is **RFC3339
  UTC**, e.g. `2026-07-17T10:01:30Z`.

## Parse discipline

Strict, fail-loud parsing: the `internal/state` read rejects unknown or malformed
fields — the same `KnownFields(true)` discipline as builder's `ParseOutcome` and the
burler verdict-parse. An unparseable or malformed status file is a hard error; loom
never guesses a status.

## Validation checklist

Spec for a future validator:

- Required fields (`slug`, `parent`, `phase`, `stage`, `narration`, `history`,
  `start_sha`, `pause_requested`, `next_action`) are present.
- `phase` is one of `preflight | discussion | plan | builder | raddle | finalize | done`.
- `stage` is one of `produce | gate`.
- Every `history[].outcome` is one of `approved | stuck`; `bounced_to` is present only
  when `outcome: stuck`.
- Every timestamp field is RFC3339 UTC.
- Strict fail-loud parse: unknown or malformed fields are rejected, never ignored.

## Worked example

A realistic **seed** — written by the spawn-time lyx command, before `lyx loom run` has
executed:

```jsonc
{
  "slug": "loom-contracts",
  "parent": "main",
  "phase": "discussion",
  "stage": "produce",
  "narration": "now: awaiting discussion input / last: — / wait: operator to run `lyx run`",
  "history": [],
  "start_sha": null,
  "pause_requested": false,
  "next_action": null
}
```

A realistic **mid-run** instance of the same file, later in the same task's life —
Discussion and Plan approved, Builder now mid-review-gate:

```jsonc
{
  "slug": "loom-contracts",
  "parent": "main",
  "phase": "builder",
  "stage": "gate",
  "narration": "now: spawned builder-review round 2, waiting on Stop hook / last: round 1 BLOCKING, 3 findings / wait: —",
  "history": [
    { "phase": "discussion", "outcome": "approved", "ts": "2026-07-17T10:01:30Z" },
    { "phase": "plan", "outcome": "approved", "ts": "2026-07-17T10:22:14Z" }
  ],
  "start_sha": "a1b2c3d4e5f60718293a4b5c6d7e8f90a1b2c3d4",
  "pause_requested": false,
  "next_action": null
}
```
