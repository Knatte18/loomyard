# Batch: shed-commitstatus-seam

```yaml
task: "Add a local-only file category to weft"
batch: "shed-commitstatus-seam"
number: 6
cards: 4
verify: go build ./cmd/lyx && go test ./internal/shedengine/... ./internal/loomrecipe/... ./internal/lyxcwd/...
depends-on: []
```

## Batch Scope

`Shed.persist` is the loop's single write path — every state change in `internal/shedengine/run.go` funnels through it — so one hook there covers running, paused, blocked, failed, stuck-bounce, the resume write and the terminal write alike.
This batch adds `Shed.CommitStatus`, an injected, nil-checked closure `persist` calls after every successful write, and threads it through `loomrecipe.ShedPaths` onto the constructed `Shed`.
It fills nothing: `internal/loomcli`'s two fill sites are batch 7's job, and until then the field stays nil and the hook is a silent no-op.

This batch shares no file with any other batch, so it carries no `depends-on` edge, and it can run alongside batches 1–5.

Batch-local decisions beyond `## Shared Decisions`:

- The call fires **after** `state.UpdateJSON` returns, outside the write lock, never inside the mutate callback.
  `persist` is today a single `return state.UpdateJSON(...)`, so it must be restructured to capture the error, let `UpdateJSON` release, then call the closure.
  `UpdateJSON` holds `lock.AcquireWriteLock(lockPath)` for its whole body, and a synchronous network push inside that window would block every `ReadJSON`/`ReadJSONStrict` reader on `StatusLockPath` for the push's duration — `lyx loom status --watch` included, which is precisely the observability this task exists to deliver.
- The consequence is accepted and must be documented rather than hidden: this opens a read-then-commit window in which a reader sees the new state on disk while git still carries the old one.
  That is strictly better than today, where the gap lasts the whole run rather than milliseconds.
- A closure error propagates out of `persist` and halts `Run`.
  The status-file write itself has already happened by then, and that ordering is load-bearing.
- `internal/shedengine`'s import allowlist is stdlib, `internal/state` and `internal/lock`, enforced by `seam_enforcement_test.go`.
  The seam is a closure precisely so the engine never learns `fabricengine` exists.
  This batch adds no import to that package.

## Cards

### Card 25: add the Shed.CommitStatus field

- **Context:**
  - `internal/shedengine/run.go`
  - `internal/shedengine/validate.go`
  - `internal/shedengine/seam_enforcement_test.go`
  - `internal/shedadapters/bouncer.go`
- **Edits:**
  - `internal/shedengine/shed.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Add a `CommitStatus func(producer, state string) error` field to the `Shed` struct in `internal/shedengine/shed.go`, after `MaxBounces`.
  Its field doc must say: it is the injected closure `persist` calls after every successful status-file write;
  nil is the absent value and means "commit nothing", which is what keeps a product that wires no seam behaving exactly as before, matching `shedadapters.BouncerConfig.Commit`'s own nil convention;
  it receives the transition's own `current_producer` and `state` as plain strings so the owner can build a per-transition commit message rather than a repeated constant;
  and it is called outside `internal/state`'s write lock, never inside the mutate callback, with the lock-ordering reason stated.
  The two parameters are plain `string`, not `State`, so a filling caller in another package never has to import this engine's enum type.
  Add no validation rule for the field in `internal/shedengine/validate.go`: nil is legal, so there is nothing to reject.
  Add no import to this package.
- **Commit:** `feat(shedengine): add the injected Shed.CommitStatus seam`

### Card 26: persist calls CommitStatus outside the write lock

- **Context:**
  - `internal/shedengine/shed.go`
  - `internal/shedengine/status.go`
  - `internal/state/state.go`
  - `internal/shedengine/seam_enforcement_test.go`
- **Edits:**
  - `internal/shedengine/run.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Restructure `(*Shed).persist` in `internal/shedengine/run.go`.
  It is today a single `return state.UpdateJSON(s.StatusPath, s.StatusLockPath, func(cur Status, found bool) (Status, error) {...})`.
  Capture that call's error into a local instead, return it immediately when non-nil, then — after `UpdateJSON` has returned and released `StatusLockPath` — return `nil` when `s.CommitStatus` is nil, and otherwise return `s.CommitStatus(nextCurrentProducer, string(nextState))`.
  Make no change of any kind inside the mutate callback.
  Extend `persist`'s doc comment with a new paragraph covering four things:
  that `CommitStatus` fires after the write and outside the lock, with the blocked-readers reason;
  that a nil `CommitStatus` is a silent no-op;
  that a closure error propagates out of `persist` and therefore halts `Run`, with the status-file write already durable;
  and that this opens a millisecond-scale read-then-commit window in which a reader can see the new state on disk before git carries it, which is the accepted cost.
  Call the closure on every `persist` invocation, never conditionally on `current_producer` having changed — `state`, `history` and `error` change without it, and the pause and resume writes happen outside any producer call.
  Add no import to this file.
- **Commit:** `feat(shedengine): persist calls CommitStatus after the write, outside the lock`

### Card 27: thread CommitStatus through loomrecipe.ShedPaths

- **Context:**
  - `internal/shedengine/shed.go`
  - `internal/shedrecipe/env.go`
  - `internal/loomrecipe/seam_enforcement_test.go`
- **Edits:**
  - `internal/loomrecipe/loomrecipe.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Add a `CommitStatus func(producer, state string) error` field to `loomrecipe.ShedPaths` in `internal/loomrecipe/loomrecipe.go`, after `MaxBounces`, and copy it onto the constructed `shedengine.Shed` in `New`'s return literal.
  Its field doc must point at `shedengine.Shed.CommitStatus`'s own field doc rather than restating it, matching the pointer style the four existing `ShedPaths` fields already use.
  Add no coherence check for it in `New`: the two existing checks exist because `StatusPath` and `StatusLockPath` are deliberately told twice, once in `shedrecipe.Env` and once in `ShedPaths`, and this field has no second copy to disagree with.
  Amend `ShedPaths`' own struct doc comment, which currently says it carries "the four told values": correct the count and add `CommitStatus` to the list.
  Leave `New`'s parse, build and error-wrapping behaviour unchanged.
- **Commit:** `feat(loomrecipe): thread CommitStatus onto the constructed Shed`

### Card 28: document and test the persistence policy

- **Context:**
  - `internal/shedengine/shed.go`
  - `internal/shedengine/run.go`
  - `internal/shedengine/run_persist_test.go`
  - `internal/shedengine/run_pause_test.go`
  - `internal/shedengine/run_routing_test.go`
  - `internal/shedengine/testsupport_test.go`
  - `internal/shedengine/status.go`
  - `internal/state/state.go`
  - `internal/lock/lock.go`
  - `manifest/designs/loom.md`
  - `manifest/roadmap.md`
- **Edits:**
  - `manifest/designs/shed.md`
- **Creates:**
  - `internal/shedengine/run_commitstatus_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `manifest/designs/shed.md`, extend the status-file contract section — the one around the "Shed-owned, rewritten on every persist" ownership split and the step-5/6 single-atomic-persist rule — with the new persistence policy.
  State: `Shed` exposes an optional injected `CommitStatus` seam;
  when a product wires it, the status file is committed once per transition rather than only when the product's own callers commit it;
  the call fires after the write and outside `internal/state`'s lock, so a slow seam never blocks a status reader;
  a nil seam is a silent no-op;
  and a seam error halts the run with the file already written.
  Keep every existing heading text in the file unchanged — `manifest/roadmap.md` and `manifest/designs/loom.md` both link into `manifest/designs/`, and the Markdown Link Integrity invariant binds.
  Follow the repo's semantic-line-break rule.
  Create `internal/shedengine/run_commitstatus_test.go` as an untagged test file in the package's existing test-package convention, spawning no git and no process, reusing `testsupport_test.go`'s fake producers.
  Cover five properties, one test function each:
  (1) `CommitStatus` is called on every write path of one run — the resume write, a running-to-running transition, a stuck bounce, a blocked terminal, a failed terminal and a done terminal — asserted by counting calls against the number of persists the run performs;
  (2) a nil `CommitStatus` is a silent no-op and the run completes exactly as it does today;
  (3) a closure returning an error propagates that error out of `Run`, and the status file on disk still carries the write that persist had already made;
  (4) the closure receives the same `producer` and `state` strings that were just written to the file, checked for at least the running, blocked and done transitions;
  (5) the lock boundary — a closure that itself acquires a read lock on `StatusLockPath` completes rather than deadlocking, which is the executable form of "outside the lock".
  Property 5 is the one that fails if the call is made inside the mutate callback, so it must not be dropped as redundant with property 1.
- **Commit:** `test(shedengine): pin the CommitStatus hook and its lock boundary`

## Batch Tests

`verify:` runs `go build ./cmd/lyx`, then `./internal/shedengine/...`, `./internal/loomrecipe/...` and `./internal/lyxcwd/...`.

- No `-tags integration` invocation is chained here, and that is deliberate rather than an omission: every file this batch touches is untagged, card 28's new test spawns no git and no process, and the Test Tier Purity Invariant forbids putting it in a tagged file it does not need.
- `./internal/shedengine/...` also runs `seam_enforcement_test.go`, which is what catches a card-26 implementation that reached for `fabricengine` instead of the injected closure.
- `./internal/loomrecipe/...` runs that package's own seam-enforcement and drift-guard tests, which is what catches a `ShedPaths` field added without being copied onto the `Shed`.
- `./internal/lyxcwd/...` is included because card 28 edits `manifest/designs/shed.md` and `docslink_test.go` in that package is where the Markdown Link Integrity invariant is enforced.
- Card 28's property 5 is this batch's primary proof of the lock ordering;
  properties 1–4 all pass against an implementation that calls the closure inside the mutate callback.
