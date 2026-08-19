# Loom status spec — loom's spawn/handover status file

> **Status: Contract — pinned.** This doc pins the `_lyx/loom/status.json` schema: loom's single source of truth for orchestration state, and the t=0 "seed" a spawn-time lyx command hands off to loom. Durable reference doc — kept, not deleted on landing — the loom analogue of [webster-spec.md](webster-spec.md) and `contracts/stencils/loom/loom-template-plan.md`.
> Product-scoped under `loom/` (renamed 2026-08-15 from a bare `_lyx/status.json`), since `Shed` (see [shed.md](../../manifest/designs/shed.md)) is a generic engine more than one product configures — the Someday `Hardener` product needs its own status file too, and a bare path could not serve both.

## What it is

`_lyx/loom/status.json` is `shedengine.Status` (see `internal/shedengine`'s own package documentation for the shell's field semantics — `current_producer`, `state`, `error`, `pause_requested`, `activity`, `history`) plus loom's own `product` payload: `slug`, `parent`, and `start_sha`.
`lyx loom run` (via `Shed.Run`) rewrites the shell on every step;
its t=0 "seed" — the handoff instant a task is spawned and given to loom, before any `lyx loom run` has executed — is written once at spawn time (see [The seed / handover](#the-seed--handover) below).

It is durable **fabric-overlay state**: it lives under `_lyx/` (git-synced via fabric, not `.lyx/`'s ephemeral machine-local state), which is what makes resume work across machines.
Its path resolves via `internal/loomengine.LoomStatusFile`, joined onto `internal/lyxcwd`'s resolved coordinates — this doc describes the file, it does not construct the path.

## Format decision (defended)

The file is **JSON via the existing `internal/state` primitive** (`WriteJSON[T]`/ `ReadJSON[T]`: locked, atomic, typed) — the same mechanism `webster` uses for its own `_lyx/webster/state.json`.

`_lyx/loom/status.json` is machine-written, machine-read orchestration state, not something a human is expected to hand-edit, and `lyx loom status --watch` pretty-prints it for humans — so the on-disk file need not be hand-readable.
Reusing `internal/state` gives locking and atomic writes for free and keeps one state primitive across modules, rather than a second one-off for loom.

## The seed / handover

The **seed** is the t=0 contents of `_lyx/loom/status.json` at the instant a task is spawned and handed to loom — not a separate file or a separate schema, just the initial snapshot of the same file loom then keeps rewriting.

It is written by a **lyx Go command** at spawn time — the mill-spawn analogue, but Go, never an agent.
This doc names the *role* ("the spawn-time lyx command"), not the exact subcommand;
which one it binds to (`fabric add` vs a dedicated `lyx loom init`/`spawn`) is pinned when that command lands.
An optional thin `ly-spawn` skill may wrap it later,
but the Go command is always the writer.

loom's Preflight **requires the file to exist** and fails loud if it is missing — the file's existence *is* the handoff signal, consistent with Preflight's other precondition checks (clean worktree, fabric ready, no half-finished prior run).

A fresh seed carries `current_producer: "Preflight"`, `state: "running"`, empty `history`, and a `product` with only `slug`/`parent` populated (`start_sha: null`) — `Shed` fills the rest as it runs, and `Shed` itself owns `current_producer`/`state`/`error`/`activity`/`history` from the first persist onward.

## The schema

```jsonc
{
  "current_producer": "Preflight",             // Shed-owned: which producer this run is at
  "state": "running",                          // Shed-owned: running | paused | done | blocked | failed
  "error": "",                                 // Shed-owned: human-readable detail for a failed/blocked halt
  "pause_requested": false,                    // shared write-to-clear: set true by an outside actor, cleared by Shed
  "activity": {"now": "...", "last": "...", "wait": "..."}, // Shed-owned, mechanically composed
  "history": [                                 // Shed-owned: one entry per producer call
    {"producer": "Preflight", "outcome": "done", "output": "", "at": "2026-07-17T10:01:30Z"}
  ],
  "product": {                                 // loom-owned, opaque to Shed
    "slug": "loom-contracts",                  // board-task pointer (board owns title/description)
    "parent": "main",                          // parent branch
    "start_sha": null                          // repo HEAD stamped when Webster begins (Raddle diff base)
  }
}
```

Per-field notes — `product`'s three fields are the whole of loom's own half of the schema; every other field is `Shed`'s, documented in `internal/shedengine`'s own package documentation, not restated here:

- **`product.slug` / `product.parent`** — the only handoff pointers into the wider task record;
  the board owns durable title/description, not this file.
- **`product.start_sha`** — the repo `HEAD` stamped when Webster begins, so Raddle can diff `start_sha..HEAD`. `null` until Webster starts.
- **No `schema_version`/`format` field.**
  This file has a single writer (`Shed`, plus loom's own spawn-time seeder and pause verb writing through the same lock) and no version-compatibility pressure.
  A version stamp here would be a rarely-exercised guard that goes stale;
  it is deliberately omitted, to be reintroduced only if a real incompatibility ever forces it.

## Parse discipline

Strict, fail-loud parsing: the `internal/state` read (`state.ReadJSONStrict`) rejects unknown or malformed fields via `json.Decoder.DisallowUnknownFields()` at the shell level — the JSON analogue of the discipline webster's own strict `outcome.yaml` decode and the burler verdict-parse apply to their own YAML.
An unparseable or malformed status file is a hard error;
loom never guesses a status.
`product` is decoded separately, after the strict shell decode succeeds — see [Validation checklist](#validation-checklist) below for how a `product` that fails to decode as loom's own shape is classified.

## Validation checklist

Spec for check 4, loom's own precondition layered over `Shed`'s shell (`internal/loomengine.checkCoherence`):

- `product.slug` and `product.parent` are mandatory: an empty string counts as absent.
- `shed.current_producer` must equal `"Preflight"` — the only way check 4 is ever reached.
- `shed.state` must be one of `Shed`'s five legal values and must not be `"done"`, a finished run.
- `shed.error` is tolerated at any value, including non-empty — it is the previous halt's reason a human resumes after reading.
- `shed.activity` is never validated — `Shed` recomposes it mechanically on every persist.
- Every `shed.history[].outcome` must be `"done"` or `"stuck"`, and every `shed.history[].at` must be RFC3339 UTC.
- **Fresh-start check:** a `shed.history[]` entry naming any producer other than `"Preflight"` is a half-finished failure; entries naming `"Preflight"` itself are tolerated, since `Shed.Run` appends a history entry before persisting `state: "blocked"` on every `Stuck` route including the `OnStuck: ""` escalation, so a `Stuck` at row 1 leaves one `Preflight` entry behind and a resumable blocked run must not fail this check forever.
- A non-null `product.start_sha`, or `shed.pause_requested: true`, is also a half-finished failure — the task has already advanced past the point Preflight is meant to gate.
- A `product` that fails to decode as loom's own shape is a `seed-incoherent` verdict, not an infra error.

## Worked example

A realistic **seed** — written by the spawn-time lyx command, before `lyx loom run` has executed:

```jsonc
{
  "current_producer": "Preflight",
  "state": "running",
  "error": "",
  "pause_requested": false,
  "activity": {"now": "Preflight", "last": "", "wait": ""},
  "history": [],
  "product": {
    "slug": "loom-contracts",
    "parent": "main",
    "start_sha": null
  }
}
```

A realistic **mid-run** instance of the same file, later in the same task's life — Preflight and Discussion-Write done, Webster-Review now blocked pending a human:

```jsonc
{
  "current_producer": "Webster-Review",
  "state": "blocked",
  "error": "stuck with no OnStuck target",
  "pause_requested": false,
  "activity": {"now": "Webster-Review", "last": "Webster-Review → stuck", "wait": "stuck with no OnStuck target"},
  "history": [
    {"producer": "Preflight", "outcome": "done", "output": "", "at": "2026-07-17T10:01:30Z"},
    {"producer": "Discussion-Write", "outcome": "done", "output": "_lyx/discussion/decision-record.md", "at": "2026-07-17T10:22:14Z"},
    {"producer": "Webster-Review", "outcome": "stuck", "output": "", "at": "2026-07-17T11:14:02Z"}
  ],
  "product": {
    "slug": "loom-contracts",
    "parent": "main",
    "start_sha": "a1b2c3d4e5f60718293a4b5c6d7e8f90a1b2c3d4"
  }
}
```
