# Loom status spec — loom's spawn/handover status file

> **Status: Contract — pinned.** This doc pins the `.lyx/loom/status.json` schema: loom's single source of truth for orchestration state, and the t=0 "seed" a spawn-time lyx command hands off to loom. Durable reference doc — kept, not deleted on landing — the loom analogue of [webster-spec.md](webster-spec.md) and `contracts/stencils/loom/loom-template-plan.md`.
> Product-scoped under `loom/` (renamed 2026-08-15 from a bare `_lyx/status.json`), since `Shed` (see [shed.md](../../manifest/designs/shed.md)) is a generic engine more than one product configures — the Someday `Hardener` product needs its own status file too, and a bare path could not serve both.

## What it is

`.lyx/loom/status.json` is `shedengine.Status` (see `internal/shedengine`'s own package documentation for the shell's field semantics — `current_producer`, `state`, `error`, `pause_requested`, `activity`, `history`) plus loom's own `product` payload: `slug`, `parent`, and `start_sha`.
`lyx loom drive` (via `Shed.Run`) rewrites the shell on every step — that loop belongs to the driver verb `lyx loom run` spawns detached, never to `lyx loom run` itself;
its t=0 "seed" — the handoff instant a task is spawned and given to loom — is written once, by `lyx loom run`'s own first invocation, before that spawn happens (see [The seed / handover](#the-seed--handover) below).

It is machine-local, never-tracked state: it lives under the ephemeral `.lyx/` tree, beside its own lock, and loom's orchestration state does not travel between machines.
Its path resolves via `internal/loomengine.LoomStatusFile`, joined onto `internal/lyxcwd`'s resolved coordinates — this doc describes the file, it does not construct the path.

## Format decision (defended)

The file is **JSON via the existing `internal/state` primitive** (`WriteJSON[T]`/ `ReadJSON[T]`: locked, atomic, typed) — the same mechanism `webster` uses for its own `_lyx/webster/state.json`.

`.lyx/loom/status.json` is machine-written, machine-read orchestration state, not something a human is expected to hand-edit, and `lyx loom status --watch` pretty-prints it for humans — so the on-disk file need not be hand-readable.
Reusing `internal/state` gives locking and atomic writes for free and keeps one state primitive across modules, rather than a second one-off for loom.

## The seed / handover

The **seed** is the t=0 contents of `.lyx/loom/status.json` at the instant a task is spawned and handed to loom — not a separate file or a separate schema, just the initial snapshot of the same file loom then keeps rewriting.

It is written by **`lyx loom run`**, the session bootstrap, at its own first invocation — the mill-spawn analogue, but Go, never an agent.
That binding is now pinned: `lyx loom run` seeds the file itself when it is absent, tolerating a re-run's already-seeded case rather than re-seeding it.
An optional thin `ly-spawn` skill may wrap it later,
but `lyx loom run` is always the writer.

The file's existence *is* the handoff signal, but it is `Shed`'s own step-1 read gate that **requires the file to exist** and fails loud if it is missing — not loom's own row.
loom's own row, `Loom-Preflight`, makes a narrower claim: that the file, once it exists, is a *coherent fresh seed* (no half-finished prior run).

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
- **`history[]` is budget-bearing, not only a log.**
  Its one-entry-per-producer-call rule (see the schema block above) is no longer merely an audit trail: it is the sole storage of every producer's per-producer, episode-scoped bounce budget, derived by counting a producer's own `stuck` entries since its own most recent `done` entry.
  It must never be truncated or compacted — doing so would silently hand every producer a fresh budget with nothing here to warn a future retention task that it just did.
  The unconditional append this depends on is the same one the fresh-start check below already relies on;
  see that check for why a `stuck` entry is appended on every `stuck` route, including a budget-exhausted block.
- **No `schema_version`/`format` field.**
  This file has a single writer (`Shed`, plus `lyx loom run` as the seeder and `lyx loom pause` as the pause verb, both writing through the same lock) and no version-compatibility pressure.
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
- `shed.current_producer` must equal `"Loom-Preflight"` — that is what `Shed` persists before calling row 2, since `Run` writes the next row's name into `current_producer` and appends the finished row's history entry before making the call.
- `shed.state` must be one of `Shed`'s five legal values and must not be `"done"`, a finished run.
- `shed.error` is tolerated at any value, including non-empty — it is the previous halt's reason a human resumes after reading.
- `shed.activity` is never validated — `Shed` recomposes it mechanically on every persist.
- Every `shed.history[].outcome` must be `"done"` or `"stuck"`, and every `shed.history[].at` must be RFC3339 UTC.
- **Fresh-start check:** a `shed.history[]` entry naming any producer other than `"Preflight"` or `"Loom-Preflight"` is a half-finished failure; entries naming either of those two are tolerated, since `Shed.Run` appends a history entry before persisting `state: "blocked"` on every `Stuck` route including the `OnStuck: ""` escalation, so a `Stuck` at either row 1 or row 2 leaves one matching entry behind and a resumable blocked run must not fail this check forever.
- A non-null `product.start_sha`, or `shed.pause_requested: true`, is also a half-finished failure — the task has already advanced past the point the two Preflight rows are meant to gate.
- A `product` that fails to decode as loom's own shape is a `seed-incoherent` verdict, not an infra error.
- **A fresh seed still carries `current_producer: "Preflight"`** (see [the schema](#the-schema) below), never `"Loom-Preflight"` — `internal/loomshed/seed.go` writes the seed once, at row 1, and `Shed` overwrites `current_producer` to `"Loom-Preflight"` itself as it advances past row 1.
  That divergence between the seed contract and this checklist's row-2 contract is real and deliberate, not a typo: the seed names the first row a fresh run starts at, the checklist names the row check 4 actually gates.

## Worked example

A realistic **seed** — written by `lyx loom run`'s own first invocation, before it spawns the detached driver:

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

A realistic **mid-run** instance of the same file, later in the same task's life — Preflight and Discussion-Write done, Webster now blocked pending a human:

```jsonc
{
  "current_producer": "Webster",
  "state": "blocked",
  "error": "stuck with no OnStuck target",
  "pause_requested": false,
  "activity": {"now": "Webster", "last": "Webster → stuck", "wait": "stuck with no OnStuck target"},
  "history": [
    {"producer": "Preflight", "outcome": "done", "output": "", "at": "2026-07-17T10:01:30Z"},
    {"producer": "Discussion-Write", "outcome": "done", "output": "_lyx/discussion/decision-record.md", "at": "2026-07-17T10:22:14Z"},
    {"producer": "Webster", "outcome": "stuck", "output": "", "at": "2026-07-17T11:14:02Z"}
  ],
  "product": {
    "slug": "loom-contracts",
    "parent": "main",
    "start_sha": "a1b2c3d4e5f60718293a4b5c6d7e8f90a1b2c3d4"
  }
}
```
