# Loom status spec — loom's spawn/handover status file

> **Status: Contract — pinned.** This doc pins the `_lym/shed/<slug>/status.json` schema (`self` resolves to the worktree's slug): loom's single source of truth for orchestration state, and the t=0 "seed" a spawn-time lym command hands off to loom. Durable reference doc, kept on landing, and the loom analogue of [webster-spec.md](webster-spec.md) and `contracts/stencils/loom/loom-template-plan.md`.
> The status file's t=0 "seed" is not the seeded-shed core's `seed.json` (see `internal/shedrun`'s own package documentation).
> The seed is the initial snapshot of the status shell, while `seed.json` is a separate, write-once file recording a run's recipe, driver, and parameters.
> The file sits in a run directory of `Shed` (`internal/shedengine`), a generic engine that more than one product configures.

## What it is

`_lym/shed/<slug>/status.json` is `shedengine.Status` plus loom's own `product` payload: `slug`, `parent` and `start_sha`.
`internal/shedengine`'s own package documentation gives the shell's field semantics: `current_producer`, `state`, `error`, `pause_requested`, `activity` and `history`.
`lym loom run` (via `Shed.Run`) rewrites the shell on every step.
That loop belongs to the driver verb `lym loom start` spawns detached, never to `lym loom start` itself.
The t=0 seed is written once, by `lym loom start`'s own first invocation, before it spawns the driver (see [The seed / handover](#the-seed--handover) below).

It is durable **fabric-overlay state**: it lives under `_lym/`, which fabric git-syncs, not under the machine-local `.lym/`.
That is what makes resume work across machines.
Its path resolves via `internal/shedrun.StatusFile`, joined onto `internal/lymcwd`'s resolved coordinates.

## Format decision (defended)

The file is **JSON via the existing `internal/state` primitive** (`WriteJSON[T]` and `ReadJSON[T]`: locked, atomic, typed), the same mechanism `webster` uses for its own `_lym/webster/state.json`.

`_lym/shed/<slug>/status.json` is machine-written, machine-read orchestration state that a human is not expected to hand-edit.
`lym loom status --watch` pretty-prints it for humans, so the on-disk file need not be hand-readable.
Reusing `internal/state` gives locking and atomic writes for free.
It keeps one state primitive across modules instead of a second one-off for loom.

## The seed / handover

The **seed** is the t=0 contents of `_lym/shed/<slug>/status.json` at the instant a task is spawned and handed to loom.
It is not a separate file or schema, only the initial snapshot of the file loom then keeps rewriting.

It is written by **`lym loom start`**, the session bootstrap, at its own first invocation; Go code writes it, never an agent.
`lym loom start` seeds the file when it is absent and leaves an already-seeded file alone on a re-run.
It commits the seed with fabric's records before it spawns the detached driver.

The file's existence is the handoff signal.
`Shed`'s step-1 read gate **requires the file to exist** and fails loud when it is missing.

A fresh seed carries an empty `current_producer`, which means the run starts at the recipe's first row.
It carries `state: "running"`, empty `history`, and a `product` with only `slug` and `parent` populated (`start_sha: null`).
`Shed` owns `current_producer`, `state`, `error`, `activity` and `history` from the first persist onward.

## The schema

```jsonc
{
  "current_producer": "Preflight",             // Shed-owned: which producer this run is at; empty in a fresh seed, the run's start
  "state": "running",                          // Shed-owned: running | paused | done | blocked | awaiting | failed
  "error": "",                                 // Shed-owned: human-readable detail for a failed, blocked or awaiting halt
    // awaiting is a review segment's escalation to the parent at its Bouncer row
    // a blocked halt carries the producer's own stuck reason, or "stuck with no OnStuck target" when it supplied none
    // an exhausted bounce budget starts "bounce budget exhausted" and carries the told resume clause after it,
    // which names the parent's resume, never a goto
    // a precondition the run cannot settle on its own starts "precondition unmet",
    // continues with what is unmet and a trailing way-forward clause;
    // the driver hands such a halt back and never resumes it
  "parent_notice": "",                         // optional, Shed-owned: the one-line notice an awaiting escalation hands the run's parent; set only on an awaiting halt and absent on every other write
  "pause_reason": "",                          // optional, Shed-owned: why the run is paused; set by every write that records paused and cleared by every other write
  "pause_requested": false,                    // shared write-to-clear: set true by an outside actor, cleared by Shed
  "pause_requested_by": "",                    // optional, outside-set: the caller label of the actor that set pause_requested; cleared in the write that clears the request
  "pause_before": "",                          // optional, outside-set: a producer name the run pauses before running; Shed clears it with pause_after when a condition fires, and `goto` clears both
  "pause_before_by": "",                       // optional, outside-set: the caller label of the actor that recorded pause_before; cleared with it
  "pause_after": "",                           // optional, outside-set: a producer name the run pauses after it returns a routed running outcome; cleared with pause_before
  "pause_after_by": "",                        // optional, outside-set: the caller label of the actor that recorded pause_after; cleared with it
  "activity": {"now": "...", "last": "...", "wait": "..."}, // Shed-owned, mechanically composed
  "history": [                                 // Shed-owned: one entry per producer call
    {"producer": "Preflight", "outcome": "done", "output": "", "at": "2026-07-17T10:01:30Z"}
    // a reroute entry also carries "routed_to": "<target row>"; so does a Stuck entry whose on_stuck edge bounced the run
    // the Stuck entry of a reroute-cap halt carries "reroute_capped": true
    // an entry of a row inside a block carries "block": "<block path>", omitted outside every block
    // the block path is the enclosing block rows' names joined by "/"
  ],
  "product": {                                 // loom-owned, opaque to Shed
    "slug": "loom-contracts",                  // board-task pointer (board owns title/description)
    "parent": "main",                          // parent branch
    "start_sha": null                          // repo HEAD stamped when Webster begins (Raddle diff base)
  }
}
```

Per-field notes follow.
`product`'s fields are the whole of loom's own half of the schema;
every other field is `Shed`'s, documented in `internal/shedengine`'s own package documentation and not restated here:

- **`product.slug` / `product.parent`** — the only handoff pointers into the wider task record;
  the board owns durable title/description, not this file.
- **`product.start_sha`** — the repo `HEAD` stamped when Webster begins, so Raddle can diff `start_sha..HEAD`.
  It is `null` until Webster starts.
- **`pause_before` / `pause_after`** — stop conditions an outside actor records through `lym loom pause --before|--after` or `lym shed pause --before|--after`.
  Those are the loom pause verb's only forms, and both keys are absent when empty.
  `pause_before` fires when `current_producer` equals it, before that producer is called.
  `pause_after` fires when the named producer returns a routed running outcome, pausing at the routed row;
  it stays recorded across a halting outcome or a producer error.
  A fired condition pauses the run and clears both keys.
  The reason reads `paused before <target>, as requested` or `paused after <target>, as requested`.
- **`pause_reason` and the `*_by` labels**
  `pause_reason` is written by `Shed` on every write that records `paused` and cleared by every other write.
  A pause request reads `pause requested` (or `pause requested by <label>`); a cancelled context reads `cancelled`;
  a fired condition reads its condition reason;
  a goto reads `goto to <row>`; a relaunch onto a row reads `re-launched onto <row>`.
  `pause_requested_by`, `pause_before_by` and `pause_after_by` are the caller labels of the outside actor that set the request or condition they accompany;
  the pause verb writes each beside it.
  Each is cleared in the write that clears its request:
  the persist that records `paused` clears `pause_requested_by`;
  a fired condition or a goto clears both condition labels.
- **`history[]` is budget-bearing, not only a log.**
  It holds one entry per producer call and is the sole storage of every producer's episode-scoped bounce budget.
  The budget is the count of a producer's own `stuck` entries since the latest of these, which ends its episode:
  its own most recent `done` entry;
  the most recent `done` by a producer of the same segment;
  or the most recent `reroute` entry into its segment (into the producer itself when it has no segment).
  A Burler review or fix row never returns `done`, so its review segment's Bouncer passing ends its episode.
  A `goto` entry does not end an episode: it keeps the spent budget.
  A `budget_capped` `stuck` entry, the one that exhausts the budget, ends the scan without being counted.
  A resume of that halt grants one more budget.
  When a `goto` entry directly follows it, the entry is counted and the resume grants none.
  A `generation_start` `stuck` entry is counted and ends the scan right after itself.
  A new generation therefore starts its count at one.
  A `reroute` entry names its target in `routed_to` and spends no budget;
  a row's reroutes since its last `done` or `reroute_capped` entry are capped at three.
  `history` must never be truncated or compacted: doing so would silently hand every producer a fresh budget.
  The budget relies on a `stuck` entry being appended on every `stuck` route, including a budget-exhausted block.
- **No `schema_version` or `format` field.**
  This file has a single writer (`Shed`, plus `lym loom start` as the seeder and `lym loom pause` as the pause verb, both writing through the same lock).
  It has no version-compatibility pressure, so a version stamp would be a rarely-exercised guard that goes stale.

## The `hand_back` key

`lym loom status` carries a top-level `hand_back` key, and the loop's stop envelope carries `loop.hand_back`.
Both are true only when the run is blocked and the recipe's told check answers true.
The check reads the status's `current_producer` and `error`; with no check told, both are false.
For tapestry and darn the check is `landingshed.HandsBack`:
a blocked stop at Parent-Align or Finalize whose reason opens with the landing-stop prefix.
A true key marks a halt that goes to the orch, so every driver hands it back instead of routing it.
A tapestry, darn or sampler fixer's escalate halt opens with the landing-escalate prefix.
`landingshed.EscalateHalt` answers true for it, and the key stays false: the halt goes to the arbiter first.

## The `restep_hold` key

`lym loom status` carries a top-level `restep_hold` key, and the loop's stop envelope carries `loop.restep_hold`.
Both are true only when the run is blocked and the recipe's told check answers true.
The check reads the status's `current_producer` and `error`; with no check told, both are false.
For tapestry and darn the check is `landingshed.EscalateHalt`:
a blocked stop at Parent-Align or Finalize whose reason opens with the landing-escalate prefix.
A true key marks a halt the LLM driver's own build re-launch and binary watch leave alone;
it still goes to the arbiter first, and only the arbiter's `resume` or the orch's `lym loom resume` moves it.

## The `waiting` key

`lym loom status` adds a `waiting` key to its envelope while the run's state is `running` and one of four waits is present.
The waits are read in order:

1. a gate wait, read from the worktree's live gate wait records, which each gate caller writes while it waits for a gate slot;
2. a parent lock wait, read from the record a landing row writes while it waits for the parent lock;
3. a verify, read from the verify marker, running or waiting for a gate slot;
4. a shuttle wait, read from the run's wait marker (a gate entry or the background-shell wait).

A gate wait does not hide the others: its note is followed, after `; `, by the first other wait with a live pid.

The value is one line.
A gate wait renders `<producer>: <site> waiting for a gate slot <elapsed>`, one clause per live record, oldest first and joined with `; `.
A single `(holders: <worktree> <site>, ...)` clause follows the last one.
For example:
`Plan-Write: lym gate test ./internal/foo waiting for a gate slot 2m (holders: /hub/other lym gate test ./internal/bar)`.
A running verify or shuttle wait renders `<producer>: <what> running <elapsed> (<detail>)`.
For example:
`Code-Burler-Fix: verify running 6m (attempt 2; lym gate test --wait --gate code-burler-fix)`.
A verify waiting for a gate slot renders `<producer>: verify waiting for a gate slot <elapsed> (<site>; holders: <worktree> <site>, ...)`.
Its elapsed is measured from the wait start.
A parent lock wait renders `<producer>: waiting for the parent lock on <branch> held by <slug> <elapsed>`.
Its elapsed is measured from the record's start.
A lock wait record whose writer is dead is ignored.
A holders clause is dropped when no slot is held or the holders cannot be read.
`<producer>` is the run's current producer.
`<elapsed>` is whole seconds under a minute, whole minutes under an hour and hours with minutes beyond (`45s`, `6m`, `1h12m`).
A verify's attempt clause is dropped when no attempt counter applies.
The key is absent when none of the waits applies, or when the run is in any state other than `running`.
`lym loom status --watch` appends the same note to its line.
It decides whether a poll prints by the note with every `<elapsed>` left out,
so a wait whose elapsed time alone grows prints no new line.
The note is read from the markers, the wait records, the parent lock wait record and the round store.
It is never read from the status file, so the file's schema above is unchanged.
It is display only: no Go decision reads it.

## Parse discipline

Fail-loud parsing: the `internal/state` read (`state.ReadJSON`) rejects malformed JSON and accepts unknown fields, and a rewrite keeps them.
An unparseable or malformed status file is a hard error;
loom never guesses a status.
`product` is decoded separately, after the shell decode succeeds;
the [Validation checklist](#validation-checklist) below classifies a `product` that fails to decode as loom's own shape.

## Validation checklist

`Shed`'s step-1 read gate refuses a missing file, a file that does not parse and a `shed.state` outside its legal values.
It finishes a run whose `shed.state` is `"done"` without calling a producer.
No loom row re-validates the file.
A fresh seed's empty `current_producer` (see [the schema](#the-schema) above) reads as the recipe's first row.
The run resumes from whatever the file records.

`lym loom` runs `internal/loomengine.VerifySeedOwnership` before it spawns a driver.
It checks that the file records this worktree's own task:

- A `product.slug` that differs from the worktree's own slug is refused, since a forked pair inherits the old task's state.
- A missing file, an empty slug and a `product` that fails to decode as loom's own shape all pass.
  A file that fails to decode as `Shed`'s shell passes too: the driver's own step-1 gate reports it.

## Worked example

A **seed**, written by `lym loom start`'s own first invocation before it spawns the detached driver:

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

A **mid-run** instance of the same file:
Preflight and Discussion-Write are done and Webster is blocked pending a human.

```jsonc
{
  "current_producer": "Webster",
  "state": "blocked",
  "error": "batch 3 still fails its tests after two fix rounds",
  "pause_requested": false,
  "activity": {"now": "Webster", "last": "Webster → stuck", "wait": "batch 3 still fails its tests after two fix rounds"},
  "history": [
    {"producer": "Preflight", "outcome": "done", "output": "", "at": "2026-07-17T10:01:30Z"},
    {"producer": "Discussion-Write", "outcome": "done", "output": "_lym/discussion/decision-record.md", "at": "2026-07-17T10:22:14Z"},
    {"producer": "Webster", "outcome": "stuck", "output": "", "at": "2026-07-17T11:14:02Z"}
  ],
  "product": {
    "slug": "loom-contracts",
    "parent": "main",
    "start_sha": "a1b2c3d4e5f60718293a4b5c6d7e8f90a1b2c3d4"
  }
}
```
