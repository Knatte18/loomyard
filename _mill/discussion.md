# Discussion: shedengine: per-producer bounce budget + explicit `OnDone` routing

```yaml
task: 'shedengine: per-producer bounce budget + explicit OnDone routing'
slug: shedengine-segments-bounce-budget
status: discussing
parent: main
```

## Problem

`internal/shedengine` today routes `Done` by physical list position and caps bounces with a single run-wide counter.
Both were right for a flat, linear pipeline, and both break under the shape the next two roadmap items introduce.

`Done` currently advances to `s.Producers[indexAfter(...)]`, and "the run is finished" is decided by `def.Name == s.Producers[len(s.Producers)-1].Name` — the producer list therefore carries routing meaning on top of being storage.
`Stuck` already routes by an explicit `OnStuck` field, so the list is a hybrid: a reader must first work out which mode a given `ProducerDef` is in before list order can be trusted at all.
The bounce budget is one `bouncesRemaining` counter in `Run`, decremented on every bounce regardless of which producer bounced.

**Why now.**
The "Perch → Shed flattening" group (`manifest/roadmap.md`, Planned) replaces `perch`'s own round-loop with two ordinary Shed rows per review — a `Bouncer` (judge, and the segment's *entry point*) and a `Burler` (one A-review→B-fix round, which always returns `Stuck` and never `Done`).
That shape breaks both mechanisms at once.
The Bouncer's approval must jump *out* of the segment to whatever follows it, not to the physically next row, and the pair's routine round-loop is expected to iterate several times as normal operation — sharing one global budget with a mechanical validate gate's rare bounce conflates "something is structurally broken" with "this is the Nth normal round."
Landing this while the task already touches `ProducerDef`/`validate()` is cheaper than landing it later with more rows in place.

## Scope

**In:**

- `internal/shedengine/producer.go` — two new `ProducerDef` fields: `OnDone string` and `Segment string`, plus `MaxBounces int`.
- `internal/shedengine/run.go` — `Done` routes via `OnDone` only; `indexAfter` and the physically-last check deleted; the run-wide `bouncesRemaining` counter replaced by a per-producer, episode-scoped count derived from `Status.History`.
- `internal/shedengine/validate.go` — three new rules (`OnDone` must exist, `OnDone` must not self-reference, `OnStuck` must share `Segment`) plus a negative per-producer `MaxBounces` rule.
- `internal/shedengine/shed.go` — `Shed.MaxBounces`'s doc comment: it becomes the inherited default for `ProducerDef.MaxBounces: 0`, not a run-wide total.
- `internal/loomshed/loomshed.go` — an explicit `OnDone` on all 12 rows preserving today's linear behavior; `New`'s doc comment and `Deps.MaxBounces`'s field doc updated.
- Tests, new or substantially rewritten: `internal/shedengine/run_routing_test.go`, `internal/shedengine/validate_test.go`, `internal/loomshed/resume_test.go`, `internal/loomshed/loomshed_test.go`.
- Tests, mechanical re-wiring only (every scenario that relies on sequential `Done` advance needs an explicit `OnDone` chain, but no scenario changes meaning): `internal/shedengine/run_pause_test.go`, `internal/shedengine/run_persist_test.go`, `internal/shedengine/testsupport_test.go` (a linear-chain builder helper), `internal/loomshed/fixture_test.go`, `internal/loomshed/sequence_test.go`.
  The completeness check for this list is **mechanical, not the test run**: grep for every `[]shedengine.ProducerDef` / `[]ProducerDef` literal and every `.Producers =` assignment in the repo, and re-wire each one.
  A green `go test ./...` is necessary but *not* sufficient evidence of a complete migration — a suite asserting only `RunDone` / `state: "done"` (several scenarios in `run_persist_test.go` are exactly this shape) passes unchanged on a run that silently ended early at a row with a forgotten `OnDone`.
- Docs, same commit: `manifest/designs/shed.md`; `internal/shedengine/doc.go`; `internal/shedengine/producer.go:34`'s struct-level comment; `internal/loomshed/loomshed.go`'s `New` doc comment and `Deps.MaxBounces` field doc; `manifest/designs/loom.md`'s stale sequential-routing sentences; a one-line `contracts/specs/loom-status-spec.md` note that `history[]` is budget-bearing; and `manifest/roadmap.md`, whose Planned item 1 moves to Shipped.
  See "Docs carrying statements this task falsifies" under Technical context for the full inventory and the grep sweep that must back it.

**Out:**

- The `Bouncer` and `Burler` producers themselves, and any `shedadapters` change — separate roadmap items that consume this task's fields.
- Any actual segment in loom's producer list. Every `loomshed` row keeps `Segment: ""` after this task; named segments arrive with the review-producer tasks.
- Any new persisted status-file field. The budget is derived from the existing `history[]`.
  `contracts/specs/loom-status-spec.md`'s *schema* is unchanged, but the spec does gain one prose line (see the In list): after this task `history[]` is no longer only a log — it is the sole storage of every producer's bounce budget — and the spec states no never-truncate guarantee today, so a future compaction or retention task would silently hand every producer a fresh budget with nothing in the spec to warn it.
- Reachability analysis over the producer graph, and `Done`-cycle detection beyond the self-reference rule.
- Renaming `Shed.MaxBounces` (and therefore `loomshed.Deps.MaxBounces` / `internal/loomcli/wiring.go`).
- Truncating, compacting, or otherwise changing `history[]`'s append-only shape.
- `internal/landingshed`, `internal/websterengine`, `internal/treadleengine`.

## Decisions

### OnDone replaces sequential routing entirely, with no fallback

- Decision: `ProducerDef` gains `OnDone string`, parallel to the existing `OnStuck`.
  Set → `Done` jumps to the named producer, forward or backward, the same freedom `OnStuck` already has.
  Empty → `Done` here finishes the whole `Shed` (`state: "done"`, `Result.Outcome = RunDone`), regardless of the producer's position in the list.
  `run.go`'s `indexAfter` helper and its `def.Name == s.Producers[len(s.Producers)-1].Name` check are both removed outright.
  On the terminal path `current_producer` keeps the just-finished producer's own name, never the empty string — unchanged from today, because `activity.now` and `Result.HaltedProducer` are both defined in terms of it.
  After this change the producer list is pure storage plus `validate()`'s iteration order plus cosmetic display order, with zero routing meaning of its own.
- Rationale: a hybrid where some producers route by physical position and others by an explicit field is harder to read than either pure form — a reader must first check which mode a given `ProducerDef` is in before trusting list order.
  Going fully explicit means one `ProducerDef` read in isolation always tells the whole story.
  The concrete forcing case is the Bouncer, whose approval must exit its segment rather than fall through to the physically next row (which is its own Burler).
- Rejected: keeping sequential routing as a fallback when `OnDone` is empty — that preserves exactly the hybrid this decision exists to remove, and makes "empty `OnDone`" mean two different things depending on list position.
- Decision, the silent-terminal risk: an **omitted** `OnDone` is indistinguishable from an intended terminal one, and ends the run quietly with `RunDone` rather than failing loud.
  Accepted, with an exhaustive `OnDone` assertion in `loomshed_test.go` plus the `New`-passes-`validate()` check as the named mitigation — not with a new `validate()` rule.
  `shed.md` states the risk in its own words, so a reader knows the empty value is load-bearing.
- Rationale: the empty string is already a load-bearing, silent default on the sibling field — `OnStuck: ""` means "escalate to a human", and a forgotten `OnStuck` is exactly as quiet — so rejecting an empty `OnDone` would make the two fields inconsistent for no principled reason.
  A candidate rule ("at most one producer may have an empty `OnDone`") was considered and rejected: multiple legitimate terminals are permitted by an explicitly-routed list, and a rule forbidding them would smuggle back the implicit structure this decision removes.
  The real defence is a test that pins the whole routing table, which is cheap and catches the migration case the risk is actually about.

### Per-producer bounce budget, counted from `Status.History`

- Decision: `ProducerDef` gains `MaxBounces int`, where `0` means "inherit `Shed.MaxBounces`" (which itself falls back to `defaultMaxBounces = 10` when `0`), the same convention `Shed.MaxBounces` already uses.
  On a `Stuck` with a non-empty `OnStuck`, `Run` counts the `Stuck` entries already in `Status.History` whose `producer` equals the bouncing producer's own `Name` and which fall inside its current episode (see the episode-scoping decision below), and refuses the route when that count is at or above the producer's effective budget.
  The run-wide in-memory `bouncesRemaining` counter is deleted.
  The boundary is pinned exactly as today: a budget of three performs three bounce-backs and blocks on the fourth `Stuck` — with the count read from the history slice as it stood at step 1, before this iteration's own entry is appended.
- Rationale: a review round-loop iterates several times as normal operation, unlike a mechanical validate gate's rare bounce; one shared counter conflates the two.
  Deriving from `history[]` needs no new persisted field, because every `Stuck` is already recorded there with its producer name.
- Rejected: a per-segment budget summing `Stuck` entries across every row sharing a `Segment` — the roadmap item's own wording is per-producer and the field sits on `ProducerDef`, and a per-segment cap would need a tie-break rule for which row's `MaxBounces` defines it while pooling every standalone (`Segment: ""`) producer into one shared budget.
  In a Bouncer/Burler segment both rows go `Stuck` once per round in lockstep anyway, so a per-producer cap of N is already a de-facto N-round cap.
  Also rejected: an in-memory `map[string]int` — see the next decision for why the persisted derivation is the point, not an implementation detail.

### The count is episode-scoped: since that producer's last `Done`, not per-`Run`-call

- Decision: producer X's budget counts the `Stuck` entries in `history[]` for X that appear **after X's most recent `Done` entry** — all of X's `Stuck` entries when X has never returned `Done`.
  The count therefore spans invocations, crashes, and human resumes; only X's own success resets it.
  A run that blocked on an exhausted budget and is then resumed by a human re-blocks on X's next `Stuck`, because a blocked producer has by definition not returned `Done` since.
  **The `Stuck` entry written on the block path itself counts.**
  `run.go`'s `Stuck` arm appends the history entry (`nextHistory := appendHistory()`, `run.go:188`) *before* the inner switch decides, so the budget-exhausted block persists that entry too — after a block, X's episode holds `budget + 1` `Stuck` entries, and every subsequent resume that re-blocks adds one more.
  This is accepted rather than special-cased: a `Stuck` entry is appended on *every* `Stuck` route including the `OnStuck: ""` escalation, `contracts/specs/loom-status-spec.md`'s fresh-start check already depends on that unconditional append, and a block-path entry is not distinguishable from a bounce-path entry by reading the history alone.
- Rationale — why not per-`Run`-call: this deliberately inverts today's documented rationale (`run.go`'s `bouncesRemaining` comment and `shed.md`'s "per-`Run`-call and held in memory" paragraph), which grants a full fresh budget on every new invocation.
  The thing the budget guards is total wasted spend before a human is pulled in, and a crash-restart loop under the old rule is unbounded.
  Both stale rationale sites are rewritten in this commit, and the new prose must state the inversion *explicitly* — naming the old per-`Run`-call reset, saying it was deliberate, and saying why it was overturned — not merely describe the new behavior in isolation.
  Describing only the new behavior leaves the reset looking like an accidental omission, which is exactly how a future reader reintroduces it as a bugfix.
- Rationale — why not all-time: a strict all-time count is wrong for loom's own shipped rows, and this is the concrete case that decided it.
  `Discussion-Review`'s `OnStuck` is `Discussion-Write` (`loomshed.go:127`), and `Discussion-Write`'s `Done` routes on to `Discussion-Validate` (`loomshed.go:125-126`), so **every** review rejection re-enters `Discussion-Validate`.
  Under all-time counting, `Discussion-Validate`'s failures accumulate for the whole life of the task even when separated by successful passes, so a long-but-healthy review cycle silently eats a gate's budget and blocks the run for reasons unrelated to anything currently broken — exactly the "is this structurally broken, or is this the Nth normal round" conflation this task exists to remove, reintroduced one level down.
  Episode scoping fixes that without weakening the anti-crash-loop property: a producer that keeps failing and never succeeds has no `Done` to reset from, so its count is all-time in precisely the case where all-time is correct.
  The default of ten is adequate under episode scoping — it bounds consecutive failures of one producer, not a task's lifetime total.
- Rejected: counting only entries appended during the current `Run` call (snapshot `len(history)` at entry) — that reduces the history derivation to a roundabout in-memory counter and keeps the unbounded crash-restart loop.
  Also rejected: strict all-time counting — see the preceding rationale; it was this discussion's own earlier position and was overturned by the `Discussion-Validate` re-entry case, which review round 3 surfaced.
  Also rejected: granting fresh budget when the status file read at step 1 is in `state: "blocked"` — it preserves the resume ergonomics, but adds an unrequested rule and re-introduces the unbounded human-driven loop by a different door; episode scoping already gives a *working* producer its reset without giving a broken one a free retry.

### Escape hatches when a budget is exhausted, and what they cost

- Decision: the supported remedy in loom today is **fixing the underlying failure so the producer returns `Done`**, which resets its episode by itself — under episode scoping this is a real remedy, not a restatement of the problem.
  When that is not possible, the remedy is **raising the producer's `MaxBounces` (or `Shed.MaxBounces`) strictly above that producer's current episode `Stuck` count** — which is `budget + 1` immediately after the first block, not `budget`, so raising it by one is never enough.
  State plainly in `shed.md` that this is today a **source edit and rebuild**, not an operator action: `MaxBounces` reaches no CLI flag and no config key anywhere in the repo (`internal/loomcli/wiring.go:91` leaves `Deps.MaxBounces` zero and nothing else sets it), and putting it on an operator surface is explicitly out of this task's scope.
  Hand-editing `history[]` in the status file is **not** endorsed as a hatch: it contradicts `contracts/specs/loom-status-spec.md`'s "one entry per producer call" and the append-only property the derivation depends on.
- Rationale: an escape hatch nobody can reach is worse than no hatch, because it stops the design from noticing the gap.
  Naming the source edit honestly, and ruling out history surgery, leaves the reader with one true remedy plus one documented limitation, rather than two hatches of which one is unreachable and the other is spec-violating.
  `shed.md` must state the arithmetic (the `+ 1` and the per-resume growth) rather than leaving an operator to discover it by re-blocking.
- Decision, the never-`Done` shape: for a producer that **structurally never returns `Done`**, episode scoping degrades to a task-lifetime cap, and that is accepted.
  The Burler is exactly this shape by design — "always returns `Stuck` with `OnStuck` pointing at its segment's Bouncer, never `Done`", since a round producer has no independent notion of finished.
  Such a row has no `Done` to reset from, so its `MaxBounces` must be sized as a **task-lifetime** cap, not a per-review one, and `shed.md` must say so in those words.
  In practice the two coincide for the Burler shape: each Burler row is entered in exactly one segment, once per task, so its lifetime cap *is* the segment's intended round cap — but that coincidence is a property of how the review-producer tasks wire their segments, not a guarantee `Shed` provides, so the sizing obligation is documented rather than assumed.
  The Bouncer, whose `Done` is the segment's approval exit, resets normally.
- Rationale: the alternative would be a second reset trigger — e.g. resetting a producer's episode when control enters it from a producer in a different `Segment` (a fresh segment entry).
  Rejected: it needs the *previous* history entry's producer plus that producer's `Segment` to decide, so the count stops being readable from the target producer's own entries; it gives `Segment` mechanical routing weight this task deliberately withheld from it; and it buys nothing for the Burler shape, where the lifetime cap already coincides with the intended round cap.
- Rejected: adding a `--max-bounces` flag or config key in this task — a real operator surface is worth having, but it is a `loomcli`/config change with its own plumbing and belongs to whoever owns that surface, not to an engine-routing task.

### Only entries the producer itself authored count

- Decision: producer X's budget consumes `history[]` entries where `producer == "X"` and `outcome == "stuck"` — entries X itself authored, regardless of where they routed.
  Entries where X was some other producer's `OnStuck` *target* do not count.
- Rationale: literal reading of the roadmap item, and it bounds the thing worth bounding — how many times X can refuse to pass.
  It is also readable from one history entry in isolation, with no need to resolve `OnStuck` targets back through the def list.
- Rejected: counting re-entries into X — off by one against the above and requires a def-list lookup per history entry.

### `Segment` is a grouping label enforced through `OnStuck`

- Decision: `ProducerDef` gains `Segment string`, empty meaning standalone.
  `validate()` requires that a producer with a non-empty `OnStuck` names a target whose `Segment` is string-equal to its own.
  Empty compares equal to empty, so the standalone producers form one implicit group and every existing `loomshed` row passes unchanged.
  `Segment` has no other mechanical meaning in this task — it does not scope the budget, does not affect `OnDone`, and is not otherwise validated.
- Rationale: this turns "these rows strictly belong together" into an enforced invariant rather than a naming convention, exactly where segments exist, while staying one plain equality rule with no special case.
  `OnDone` gets no such restriction, because crossing *out* of a segment on approval is the point.
- Rejected: applying the rule only when `p.Segment != ""` — behaviorally identical today, but two rules where one suffices.
  Also rejected: requiring a non-empty `Segment` on any producer with a non-empty `OnStuck` — that forces loom's existing rows into named segments now, turning a mechanical migration into a design pass over loom's producer table, which belongs to the review-producer tasks.

### The three new `validate()` rules, and what is deliberately not validated

- Decision: `validate()` gains, each with its own distinct `"shedengine: "`-prefixed message sharing wording with no other rule:
  1. `OnDone`, if set, must name an existing producer in the list — the same "must exist" check `OnStuck` already gets, checked after the whole name set is collected so forward references stay legal.
  2. `OnDone`, if set, must not name its own producer.
  3. `OnStuck`, if set, must name a producer whose `Segment` equals the bouncing producer's own `Segment`.
  Plus a per-`ProducerDef` `MaxBounces < 0` rejection, mirroring the existing `Shed.MaxBounces` rule.
  Not validated: reachability of any producer from any entry point, and `Done` cycles spanning two or more producers.
- Rationale: `Done` routing consumes no budget, so `OnDone: <self>` is a statically certain infinite loop and worth one cheap rule.
  `OnStuck: <self>` stays legal — it is budgeted, therefore bounded.
  Reachability cannot be checked because `validate()` does not know the entry producer: it comes from the seeded status file's `current_producer`, which `Shed` never writes first and never guesses.
  A multi-producer `Done` cycle is not statically infinite — any member may exit via `OnStuck` — so detecting it would reject legitimate backward jumps.
- **Runtime disposition for a `Done` cycle: accepted as unbounded and human-owned, with no iteration cap in `Run`.**
  This is a real behavior change and is stated rather than glossed: today `Done` only ever advances forward, so `Run`'s `for {}` terminates by list length alone; once `OnDone` permits backward jumps and `Done` consumes no budget, a `Done` cycle whose every member keeps returning `Done` runs forever.
  Accepted for three reasons.
  First, a runtime cap would be a new concept — every budget mechanism in `Shed` today is `Stuck`-based, and any total-iteration limit needs an arbitrary number that is either too low for a legitimate long run or too high to bound anything worth bounding.
  Second, `Done` routing is entirely author-configured and statically visible in one producer list, unlike `Stuck` routing whose reachability depends on producer verdicts — a `Done` cycle is a config bug a reader can see, not an emergent runtime condition.
  Third, and decisively, the loop is **not un-interruptible**: step 3 re-reads the status file and checks `pause_requested` *and* `ctx.Err()` at the top of every iteration, so an operator stops a runaway with Ctrl-C or by writing `pause_requested: true`, and the run exits cleanly as paused.
  `shed.md` states this explicitly — that a `Done` cycle is unbounded by design, that `validate()` catches only the single-producer self-reference case, and that pause/cancellation is the stop mechanism — so a future reader does not mistake the absence of a cap for an oversight.
- Rejected: adding reachability or cycle rules speculatively — YAGNI, and both produce false rejections on configurations the design explicitly permits.

### `Shed.MaxBounces` keeps its name

- Decision: `Shed.MaxBounces` stays named as-is and becomes "the default a `ProducerDef.MaxBounces` of `0` inherits", with its doc comment rewritten.
  No identifier is renamed, so `loomshed.Deps.MaxBounces` and `internal/loomcli/wiring.go` keep their names and call shapes — but their *doc comments* are in scope, since both describe the old run-wide semantics.
- Rationale: the roadmap item pins this convention ("0 = inherit `Shed`'s own default, same convention `Shed.MaxBounces` already uses"), and the rename would ripple through `Deps`, `wiring.go`, and three test files for a doc-comment-sized gain.
- Rejected: renaming to `DefaultMaxBounces` — defensible, since the semantics genuinely changed from total to per-producer default, but out of proportion to the benefit.

### The two `blocked` reason strings are unchanged

- Decision: `"stuck with no OnStuck target"` and `"bounce budget exhausted"` keep their exact current text, still written identically to both the persisted `error` field and `Result.Reason`.
- Rationale: `Result.HaltedProducer` already names which producer exhausted its budget, and `shed.md` pins both strings verbatim; changing them churns the doc and the assertions for no new information.
- Rejected: interpolating the producer name and its budget into the reason string.

### loomshed migration is mechanical and behavior-preserving

- Decision: each of the 12 rows in `loomshed.New` gains an explicit `OnDone` naming the next row in today's table order — `Preflight` → `Discussion-Write` → `Discussion-Validate` → `Discussion-Review` → `Plan-Write` → `Plan-Validate` → `Plan-Review` → `Batchifier` → `Webster` → `Webster-Review` → `Publish` → `Finalize`, with `Finalize` carrying `OnDone: ""`.
  Every row keeps `Segment: ""` and `MaxBounces: 0`.
  Every row's existing `OnStuck` is unchanged.
  Built from the existing `Name*` constants, never repeated string literals, per that file's own rule.
- Rationale: preserves today's observable behavior exactly while removing the implicit dependency on list order.
  Doing it now, while `ProducerDef` and `validate()` are already being touched, is cheaper than doing it later with more rows in place.
- Rejected: deferring the migration — `OnDone` has no fallback, so an unmigrated list would finish the whole run at row 1.

## Technical context

**`internal/shedengine` is a seam-enforced package.**
Its production code imports only the standard library, `internal/state`, and `internal/lock` (CONSTRAINTS.md's Shed Producer-Seam Invariant, machine-enforced by `internal/shedengine/seam_enforcement_test.go`'s `TestProducerSeamInvariant_AllowlistOnly`).
Nothing in this task needs a new import.

**Files that change, and what is in each:**

- `internal/shedengine/producer.go` (44 lines) — `ProducerDef` is the last declaration; today `Name`, `Producer`, `OnStuck`.
  Each field carries a doc comment explaining what it is for, not merely what it is; the three new fields follow that.
  The struct-level comment at `:34` ("the seam plus the two things the list needs around it") is rewritten in the same edit — see the doc-falsification inventory below.
- `internal/shedengine/run.go` (320 lines) — `Run`'s six-step loop.
  `findProducer`'s `int` return (`run.go:22`) is already discarded at its only call site (`run.go:108`) and becomes wholly unused once `Done` routes by name — **drop it with `indexAfter`**, leaving `findProducer` returning `(ProducerDef, bool)`.
  The bounce counter is initialised at the top (`bouncesRemaining := s.MaxBounces`, with the `0 → defaultMaxBounces` fallback) and lives across loop iterations.
  The `switch` after the producer call has five arms: `callErr != nil && ctx.Err() != nil`, `callErr != nil`, `outcome == Stuck`, `outcome == Done`, `default`.
  Only the `Stuck` and `Done` arms change.
  `appendHistory` is a closure over `st.History` that copies before appending, and `st.History` (the slice read at step 1) is what the new budget count must read — the pre-append value, which is what pins the boundary correctly.
  `indexAfter` sits at the bottom of the file, just above `persist`, and is called from exactly one site.
- `internal/shedengine/validate.go` (67 lines) — a flat sequence of field checks, then one loop collecting the `seen` name set, then a second loop checking `OnStuck` against it.
  The second loop is the natural home for the `OnDone` existence and `Segment` rules; the `MaxBounces < 0` per-producer check belongs in the first.
  **The existing `seen` map does not suffice for the `Segment` rule:** it is a `map[string]bool` carrying only name presence, so the `OnStuck` same-`Segment` check needs either a second name→`Segment` map built in the first loop, or a `findProducer`-style lookup per check.
  Widening `seen` to `map[string]string` (name → `Segment`) is not free either, since presence is currently tested as `!seen[p.OnStuck]` and an empty-string `Segment` is a legitimate value — a plan must pick one of the two shapes deliberately rather than assume the current map extends.
  The file's own doc comment explains why the non-obvious rules exist — new rules follow that pattern.
- `internal/shedengine/shed.go` (54 lines) — `Shed.MaxBounces`'s doc comment and `defaultMaxBounces`.
- `internal/shedengine/doc.go` (54 lines) — package documentation; check it for statements about sequential `Done` routing and the run-wide budget.
- `internal/loomshed/loomshed.go` — the 12-row `[]shedengine.ProducerDef` literal in `New`, plus `New`'s long doc comment which currently enumerates every row with its backing and `OnStuck` target ("The twelve rows, with their backing and OnStuck target: …") — that enumeration must gain the `OnDone` targets.

**`Status.History` shape** (`internal/shedengine/status.go`): `HistoryEntry{Producer string, Outcome Outcome, Output string, At string}`, JSON tags `producer`/`outcome`/`output`/`at`.
`history[]` is append-only and never truncated or compacted by anything in the repo — the budget derivation depends on that, and `contracts/specs/loom-status-spec.md` documents it as "one entry per producer call".
A `Stuck` history entry is appended on *every* `Stuck` route including the `OnStuck: ""` escalation and the budget-exhausted block, which is why `loom-status-spec.md`'s fresh-start check tolerates a leftover `Preflight` entry.

**Reading the episode.**
The count is a single backward scan of `st.History`: walk from the end, stop at the first entry with `Producer == def.Name && Outcome == Done`, and count the `Producer == def.Name && Outcome == Stuck` entries seen along the way.
No `Done` found means the whole history is the episode.
Entries authored by other producers are skipped, never terminating the scan — a `Done` by some *other* producer does not end X's episode.

**Ordering gotcha in the `Stuck` arm.**
Today `nextHistory := appendHistory()` runs first, then the `switch` decides.
The budget count must be taken from `st.History` (pre-append), not `nextHistory`, or the boundary shifts by one and `TestRun_BounceBudgetExhaustion`'s current expectation breaks in a way that looks like an off-by-one bug rather than a semantic change.

**Test helpers already available** (`internal/shedengine/testsupport_test.go`, declares no `Test` function): `funcProducer` (closure-backed `ShedProducer` with a `calls` counter, pointer receiver), `fixedOutcomeProducer(outcome, outputPath)`, `newTestShed(t)` (returns a `Shed` with no `Producers` wired to a fresh `t.TempDir()`, plus the three paths; lock parents deliberately left uncreated so `Run`'s own `MkdirAll` is exercised), and status-file seed/read helpers.
Do not redeclare these.

**Callers and fixtures outside `shedengine`:**

- `internal/loomshed/loomshed.go` is the only `[]shedengine.ProducerDef` literal in the repo outside the engine's own tests.
- `internal/loomshed/resume_test.go:269-303` (`TestBounceRouting_BudgetExhaustionBlocks`) drives Discussion-Validate to `Stuck` repeatedly with `deps.MaxBounces = 2` and asserts exactly `MaxBounces + 1` Stuck entries then `shedengine.RunBlocked`.
  Under a per-producer budget this scenario still involves exactly one bouncing producer, so the arithmetic is unchanged — but the test's comment explaining *why* the count is `MaxBounces+1` now describes a per-producer budget, and the assertion should be re-read against the new semantics rather than assumed to pass.
- `internal/loomshed/loomshed_test.go:99` and `internal/loomshed/fixture_test.go:118` set/assert `MaxBounces`; `loomshed_test.go` also asserts the assembled row shape, which now includes `OnDone`.
- `internal/loomcli/wiring.go:91` leaves `MaxBounces` zero with a comment about the internal default — still accurate, worth re-reading.

**Docs carrying statements this task falsifies.**
The list below is a starting inventory, not a closed set — it was enumerated by hand and a hand-enumerated list is exactly what goes stale.
The plan must run a **grep sweep** over `internal/**/*.go` doc comments, `manifest/*.md` (not only `manifest/designs/*.md` — `manifest/parallel-work.md:17` states this task is "`internal/shedengine` only", which the mandatory `loomshed` migration falsifies), and `contracts/specs/*.md` for at least: `in what order`, `next entry`, `next, separate producer`, `next producer in the list`, `bounce budget`, `MaxBounces`, `last entry`, `bounces back` — and commit a disposition (rewrite, or "still accurate because …") for **every** hit, rather than working from the enumeration alone.

Known hits:

- `manifest/designs/shed.md` — `### The Shed loop — exact mechanics` (step 6's `Done` bullet at `:85`: "advance `current_producer` to the next entry… Past the last entry"), the whole **Bounce-budget: a single total cap across the whole run, not per-producer** paragraph (which argues *against* a per-producer budget on the grounds that an A↔B cycle would run `2×budget` — that argument is now overridden and the doc must say so, not silently drop it), the `ProducerDef`/`Shed` code block, the `MaxBounces` prose under it, the validation bullet list under "What `Run` does before step 1", `:11` ("purely which producers are in the list, in what order"), and `:43` ("Review… is always the next, separate producer in the list").
- `manifest/designs/loom.md` — `:45` ("On `stuck`, `Shed` bounces back to an earlier producer in the list") plus its surrounding sequential framing, `:22` (same "in what order" phrasing as `shed.md:11`), and `:55` (same "next, separate producer in the list" phrasing as `shed.md:43`).
- `internal/shedengine/doc.go` — `:5` ("in its list and in what order") specifically, not merely "check the file": the sentence is falsified by this task's own "the producer list becomes pure storage… with zero routing meaning", and the whole file is re-read for the sequential-routing and run-wide-budget framing.
- `internal/shedengine/producer.go:34` — `ProducerDef`'s **struct-level** doc comment, "the seam plus the two things the list needs around it", which is arithmetically false once the struct carries six fields (three today plus `OnDone`, `Segment`, `MaxBounces`). In scope alongside the three new per-field comments.
- `internal/loomshed/loomshed.go` — both `New`'s doc comment (the row enumeration) and `Deps.MaxBounces`'s own field doc at `:50-52` ("MaxBounces is Shed's own told bounce budget"), which goes stale exactly as `Shed.MaxBounces`'s does: it is now an inherited per-producer default, not a run-wide budget.
- `internal/loomcli/wiring.go:91` — re-read; the "left zero so shedengine.Shed's own default applies" comment is probably still accurate, but "default" now means something different.
- `contracts/specs/loom-status-spec.md` — its schema is unchanged, but it gains a one-line note that `history[]` is budget-bearing and must never be truncated or compacted, since after this task the spec's own "one entry per producer call" line is the only thing standing between a retention task and a silent budget reset.
- `manifest/roadmap.md` moves only on completing a planned item — this task completes Planned item 1, so it moves to Shipped per that file's own Maintenance section.

The "in what order" family deserves its own note: those sentences are about what distinguishes `loom` from `Hardener` (which producers, in which sequence), not about routing mechanics, so several of them may survive with a small qualification rather than a rewrite — but each needs the qualification decided, not assumed.

## Constraints

From `CONSTRAINTS.md`:

- **Shed Producer-Seam Invariant** — `internal/shedengine` production code imports only the standard library, `internal/state`, and `internal/lock`.
  `internal/lyxcwd` is excluded specifically so `Shed` is *told* its geometry: `StatusPath`, `LockPath`, `StatusLockPath` are caller-supplied and the only paths the package constructs are the two lock parents.
  Enforced by `seam_enforcement_test.go`'s `TestProducerSeamInvariant_AllowlistOnly`.
- **Told-Geometry Invariant** — `internal/loomshed` is machine-enforced by its own `seam_enforcement_test.go`'s `TestToldGeometryInvariant_AllowlistOnly`; the migration adds only field values to an existing literal, so nothing here should trip it.
- **Documentation Lifecycle** / project `CLAUDE.md` — a task changing cross-cutting infrastructure updates its module doc (`manifest/designs/shed.md`) in the same commit.
  The roadmap moves because a Planned item completes.
  No new cross-cutting invariant is introduced by this task, so `CONSTRAINTS.md` itself is unchanged.
- **Markdown: semantic line breaks** — one sentence per line, plain newlines, in every `.md` file touched.

Discovered during discussion:

- `Shed` stays a plain exported-field struct with no `New` constructor — adding one would create a second, unvalidated door alongside `Run`'s validation.
  The three new fields are ordinary exported fields validated by `validate()`, nothing more.
- `Outcome` and `State` are open string types; every existing rule about rejecting values outside the legal sets stays exactly as it is.
- Both `blocked` causes must keep writing one identical string to the persisted `error` field and to `Result.Reason`.

## Testing

**`internal/shedengine` — TDD candidates, both suites are already table-driven or per-scenario in style.**

`validate_test.go` (`TestShed_Validate`) is the clearest TDD candidate: the four new rules are pure functions of the def list, each with its own distinct message, and each needs one passing and one failing case.
Scenarios to cover: `OnDone` naming a producer not in the list; `OnDone` naming its own producer; `OnDone` naming a *later* producer (forward reference must stay legal); `OnStuck` naming a producer in a different `Segment`; `OnStuck` naming a producer in the same non-empty `Segment` (must pass); `OnStuck` between two `Segment: ""` producers (must pass — this is the existing loom shape); a negative `ProducerDef.MaxBounces`.
Assert each error message is distinct from every other rule's, matching the file's existing discipline.

`run_routing_test.go` — the `Done`-routing rewrite:

- `Done` with an empty `OnDone` finishes the run from a *non-last* list position: `RunDone`, `state: "done"`, `current_producer` keeping that producer's own name, and no later producer called.
- `Done` with `OnDone` naming a *later* producer skips over the intervening rows entirely.
- `Done` with `OnDone` naming an *earlier* producer routes backward and the run continues.
- The existing `TestRun_HappyPath`, `TestRun_CompletionTerminalValues`, `TestRun_UnconditionalRecall`, and every scenario in `run_pause_test.go` / `run_persist_test.go` that relies on sequential advance must be re-wired with explicit `OnDone` chains; a fixture helper that builds a linear chain from a name list would keep that mechanical rather than error-prone.

`run_routing_test.go` — the per-producer budget:

- Boundary: a producer with `MaxBounces: 3` performs exactly three bounce-backs and blocks on the fourth `Stuck`, with reason `"bounce budget exhausted"` (this is `TestRun_BounceBudgetExhaustion` re-pointed at a per-producer budget).
- Independence: two producers each bouncing, each with its own budget, where the first exhausting its budget does not reduce the second's — the property the whole task exists for, and the one the old counter cannot express.
- Inheritance: `ProducerDef.MaxBounces: 0` inherits `Shed.MaxBounces`; `Shed.MaxBounces: 0` in turn inherits the internal default of ten (this is `TestRun_MaxBouncesZeroResolvesToDefault`, now two-level).
- History derivation across invocations: seed a status file whose `history[]` already contains N `Stuck` entries for a producer, run, and assert the budget accounts for them — the direct test of the persisted-count decision, and the one that would fail under a per-`Run`-call count.
- Episode reset: a producer that bounces, later returns `Done`, and is then re-entered and bounces again starts from zero on re-entry — `Stuck` entries preceding its last `Done` do not count.
  This is the loom `Discussion-Validate` case (re-entered on every `Discussion-Review` rejection via `Discussion-Write`) and the scenario that decided episode scoping over all-time counting; it deserves a named test, not a table row.
- No spurious reset: a `Done` entry authored by a *different* producer does not end this producer's episode.
- Never-passing gate: a producer with no `Done` entry anywhere in history accumulates all-time, which is the anti-crash-loop property episode scoping must not weaken.
- Attribution: `Stuck` entries authored by a *different* producer do not consume this producer's budget, even when that other producer's `OnStuck` targets it.
- The block-path entry: after a budget-exhausted block, the producer's episode `Stuck` count is `budget + 1`, and a resumed run whose `MaxBounces` was raised by exactly one blocks again immediately while raising it above the current count lets the run proceed — this pins the escape-hatch arithmetic the decision commits to, and is the test an operator's bug report would otherwise write.
- Unchanged: `Stuck` with `OnStuck: ""` still blocks with `"stuck with no OnStuck target"`, ahead of any budget check.

**`internal/loomshed`** — `loomshed_test.go`'s row-shape assertions gain the `OnDone` chain (and should assert it exhaustively, since an omitted `OnDone` silently ends the run rather than failing loud); `resume_test.go`'s bounce scenario re-read against per-producer semantics with its explanatory comment rewritten.
A cheap high-value addition: assert `New`'s returned `Shed` passes `validate()` — the whole 12-row migration is exactly the kind of mechanical edit where one missing or misspelled `OnDone` is invisible until a real run.

**Whole-repo:** `go build ./... && go test ./...` — the `Done`-routing change touches every test that assumed sequential advance, so a green full suite is the real gate, not the two packages' own suites.

## Q&A log

- **Q:** Does the per-producer budget count all-time `Stuck` entries in `history[]`, or only those appended during the current `Run` call? **A:** [auto-pick] A persisted count, not a per-`Run`-call one, accepting the inversion of today's reset-per-invocation rationale — **later narrowed in review round 3 from all-time to episode-scoped; see that entry below, which supersedes the scope of this answer.** **Why:** the budget guards total wasted spend before a human is pulled in, and a crash-restart loop under the old rule is unbounded.
- **Q:** Is the budget unit the producer or the segment? The roadmap item says per-producer, the Burler item says "per-segment bounce mechanism". **A:** [auto-pick] Strictly per-producer; `Segment` is only a validation/grouping label. **Why:** matches the item's explicit wording and the field's placement on `ProducerDef`; in a Bouncer/Burler segment both rows go `Stuck` in lockstep, so a per-producer cap of N is already a de-facto N-round cap.
- **Q:** Which history entries count toward producer X's budget — the ones X authored, or the ones where X was the `OnStuck` target? **A:** [auto-pick] The ones X itself authored. **Why:** literal roadmap reading, bounds how many times X can refuse to pass, and is readable from one history entry without a def-list lookup.
- **Q:** Every existing loom row has `Segment: ""`, so how does the `OnStuck` same-`Segment` rule treat standalone producers? **A:** [auto-pick] Plain string equality, with `""` documented as one implicit standalone group. **Why:** one rule, no special case, existing rows pass unchanged, and the invariant bites exactly where segments exist.
- **Q:** `Shed.MaxBounces` changes meaning from run-wide total to inherited per-producer default — rename it? **A:** [auto-pick] Keep the name, rewrite the doc comment. **Why:** the roadmap pins the convention, and the rename ripples through `loomshed.Deps`, `wiring.go`, and three test files for a doc-sized gain.
- **Q:** Should `validate()` reject producers unreachable from any entry point, now that list order carries no routing meaning? **A:** [auto-pick] No reachability rule. **Why:** `validate()` cannot know the entry producer — it comes from the seeded status file's `current_producer` — so any analysis would be guesswork.
- **Q:** Reject `OnDone` naming its own producer? **A:** [auto-pick] Yes. **Why:** `Done` routing consumes no budget, so it is a statically certain infinite loop; `OnStuck: <self>` stays legal because it is budgeted and therefore bounded.
- **Q:** Change the two `blocked` reason strings now that the budget is per-producer? **A:** [auto-pick] Keep both verbatim. **Why:** `Result.HaltedProducer` already names the producer, and `shed.md` pins both strings.
- **Q:** What does `Done` with an empty `OnDone` do from a non-last list position? **A:** [auto-pick] Finishes the whole Shed. **Why:** that is the no-fallback rule; `indexAfter` and the physically-last check are deleted outright.
- **Q:** How far does the loomshed migration go? **A:** [auto-pick] All 12 rows get an explicit `OnDone` in today's table order, `Finalize` gets `""`, every row keeps `Segment: ""`. **Why:** preserves observable behavior exactly; named segments belong to the review-producer tasks.
- **Q:** Any cycle protection for `Done` routing beyond the self-reference rule? **A:** [auto-pick] None. **Why:** a multi-producer `Done` cycle is not statically infinite — a member may exit via `OnStuck` — so detection would reject legitimate backward jumps.
- **Q:** Negative `ProducerDef.MaxBounces`? **A:** [auto-pick] `validate()` rejects it, mirroring the existing `Shed.MaxBounces` rule.
- **Q:** Is there a way to express "this producer may never bounce"? **A:** [auto-pick] `OnStuck: ""` already is it; `MaxBounces: 0` stays "inherit", never "no bounces allowed". **Why:** preserves the existing convention across both fields.
- **Q:** Which docs land in the same commit? **A:** [auto-pick] `manifest/designs/shed.md`, `internal/shedengine/doc.go`, `loomshed.go`'s `New` doc comment, `manifest/designs/loom.md`'s stale sequential-routing sentence, plus the roadmap item moving to Shipped.
- **Q:** (review r4 gap) A producer that structurally never returns `Done` — the Burler — can never reset its episode, so its budget is a task-lifetime cap with no reachable hatch. Accept, or add a second reset trigger? **A:** [auto-pick] Accept and document. **Why:** each Burler row is entered in exactly one segment once per task, so its lifetime cap coincides with the intended round cap; the alternative (reset on entry from another `Segment`) would give `Segment` routing weight this task deliberately withheld and would stop the count being readable from the producer's own entries.
- **Q:** (review r4 gap) An omitted `OnDone` is indistinguishable from an intended terminal and ends the run silently. Accept, or add a `validate()` terminal rule? **A:** [auto-pick] Accept, mitigated by an exhaustive `OnDone` assertion plus a `New`-passes-`validate()` check. **Why:** `OnStuck: ""` is already an equally silent load-bearing default, and "at most one empty `OnDone`" would forbid legitimate multiple terminals — reintroducing implicit structure. Scope's completeness check is switched from "`go test ./...` is the authority" to a mechanical sweep of every `[]ProducerDef` literal, since an end-state-only assertion passes on a silently shortened run.
- **Q:** (review r3 gap) All-time counting burns a gate's budget across successful passes — `Discussion-Validate` is re-entered on every `Discussion-Review` rejection. Keep all-time, or scope the count? **A:** [auto-pick] Scope it: count only since that producer's own last `Done`. **Why:** all-time counting reintroduces the "structurally broken vs. Nth normal round" conflation one level down, and episode scoping keeps the anti-crash-loop property intact, since a producer that never succeeds has no `Done` to reset from. This overturns the earlier all-time auto-pick on evidence from loom's shipped rows.
- **Q:** (review r3 gap) The stated escape hatch is unreachable — `MaxBounces` has no CLI flag or config key, and editing `history[]` violates the status spec. **A:** [auto-pick] Name the real remedy (fix the failure so the producer returns `Done`, which now resets its own episode), state that raising `MaxBounces` is a source edit and rebuild today, and rule out history surgery outright; an operator surface for `MaxBounces` stays out of scope.
- **Q:** (review r1 gap) `Done` cycles are unbounded at runtime once `OnDone` permits backward jumps — cap them in `Run`, or accept? **A:** [auto-pick] Accept as unbounded and human-owned, no iteration cap. **Why:** any cap needs an arbitrary number, `Done` routing is statically visible config rather than an emergent runtime condition, and step 3's per-iteration `pause_requested`/`ctx.Err()` check means an operator can always stop a runaway cleanly.
- **Q:** (review r1 gap) Does the `Stuck` entry written on the budget-exhausted block path itself count toward the budget? **A:** [auto-pick] Yes, it counts. **Why:** the append is unconditional in the `Stuck` arm and `loom-status-spec.md`'s fresh-start check depends on that; the escape hatch is restated as "raise `MaxBounces` above the producer's current episode `Stuck` count", since after a block that count is `budget + 1` and grows by one per re-block.
- **Q:** Test approach? **A:** [auto-pick] Extend the existing suites in place — `run_routing_test.go` and `validate_test.go` in the engine, `resume_test.go` and `loomshed_test.go` in loomshed — rather than adding a new test file.
