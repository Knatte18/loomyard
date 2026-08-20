# Batch: engine-fields-and-validation

```yaml
task: 'shedengine: per-producer bounce budget + explicit OnDone routing'
batch: 'engine-fields-and-validation'
number: 1
cards: 4
verify: go test ./internal/shedengine/...
depends-on: []
```

## Batch Scope

This batch adds the three new `ProducerDef` fields and every `validate()` rule that governs them, without touching `Run`'s routing at all.
It is one batch because the fields and the rules that police them are a single readable unit, and because it leaves the package green: `run.go` compiles unchanged against the widened struct, every existing test still passes, and the new fields are simply unread until batch 2 consumes them.
The external interface batch 2 consumes is exactly the three new fields plus the guarantee `validate()` now gives about them — that a non-empty `OnDone` names an existing, non-self producer, so `Run`'s `Done` arm may route to it without a lookup or a fallback.

Batch-local decision, made deliberately rather than assumed: the existing `seen` map in `validate.go` is a `map[string]bool` carrying only name presence, and it does not suffice for the `OnStuck` same-`Segment` rule.
Rather than widening `seen` to `map[string]string` — where an empty-string `Segment` is a legitimate value and would collide with the current `!seen[p.OnStuck]` presence test — this batch adds a **second** map built in the same first loop, leaving `seen`'s presence semantics untouched.

## Cards

### Card 1: Add `OnDone`, `Segment`, and `MaxBounces` to `ProducerDef`

- **Context:**
  - `internal/shedengine/shed.go`
  - `_mill/discussion.md`
- **Edits:**
  - `internal/shedengine/producer.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Add three exported fields to `ProducerDef` in `internal/shedengine/producer.go`, each with its own doc comment explaining what it is for rather than merely what it is, matching the existing `OnStuck` field comment's style.
  `OnDone string` — the sole router for a `Done` verdict: `""` finishes the whole `Shed` regardless of this entry's list position, else `Done` jumps to the `Name` it names, forward or backward.
  Its comment must state that the empty value is load-bearing and silent, exactly as `OnStuck: ""` already is, so an omitted `OnDone` ends the run quietly rather than failing loud.
  `Segment string` — a grouping label, `""` meaning standalone; its only mechanical effect is `validate()`'s rule that a non-empty `OnStuck` must name a target in the same `Segment`, and the comment must say it does not scope the bounce budget.
  `MaxBounces int` — this producer's own episode `Stuck` budget, where `0` means "inherit `Shed.MaxBounces`" and never "no bounces allowed".
  Also rewrite the struct-level doc comment above `type ProducerDef struct` — today it reads "the seam plus the two things the list needs around it", which is arithmetically false once the struct carries six fields.
  Do not add any import to this file.
- **Commit:** `feat(shedengine): add OnDone, Segment, and MaxBounces to ProducerDef`

### Card 2: Rewrite `Shed.MaxBounces`'s doc comment for the inherited-default semantics

- **Context:**
  - `internal/shedengine/producer.go`
  - `_mill/discussion.md`
- **Edits:**
  - `internal/shedengine/shed.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `internal/shedengine/shed.go`, rewrite the doc comment on the `MaxBounces` field of `Shed`.
  It currently reads "the total Stuck-routed bounce budget for one Run call, in-memory and never persisted", which is false after this task.
  The replacement states that `Shed.MaxBounces` is the default a `ProducerDef.MaxBounces` of `0` inherits, that `0` on `Shed` itself falls back to the internal default of ten, and that the budget it seeds is now per-producer and episode-scoped, counted from the persisted `history[]` rather than held in memory.
  The comment must name the inversion explicitly — that the budget used to be run-wide and reset on every new `Run` call by deliberate design, and that this task overturned that because a crash-restart loop under the old rule was unbounded — so a future reader does not read the missing reset as an accidental omission.
  Also update the file's own top-of-file comment, whose second line describes "the default bounce budget Run falls back to when the caller leaves MaxBounces unset", so it describes the two-level inheritance.
  Leave `defaultMaxBounces`'s value at `10`; update only its doc comment to say it is the default an unset budget resolves to at the second level.
  Do not rename `MaxBounces`.
- **Commit:** `docs(shedengine): rewrite Shed.MaxBounces doc for the inherited per-producer default`

### Card 3: Add the four new `validate()` rules

- **Context:**
  - `internal/shedengine/producer.go`
  - `internal/shedengine/shed.go`
  - `_mill/discussion.md`
- **Edits:**
  - `internal/shedengine/validate.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Extend `(*Shed).validate` in `internal/shedengine/validate.go` with four rules, each returning its own distinct `"shedengine: "`-prefixed message that shares wording with no other rule in the file, and each naming the offending producer by `%q` and its index by `%d` in the style the existing per-producer rules already use.
  In the **first** loop (the one that collects `seen` and rejects empty `Name`, nil `Producer`, and duplicates), add a per-`ProducerDef` rejection of `p.MaxBounces < 0`, mirroring the existing `s.MaxBounces < 0` rule above the loop but with a message distinguishable from it.
  In that same first loop, build a second map — name it `segmentByName` and declare it as `map[string]string` with the same `len(s.Producers)` capacity hint `seen` uses — recording `p.Segment` under `p.Name`.
  Do not widen or replace `seen`: its `!seen[...]` presence test must keep working unchanged, and an empty-string `Segment` is a legitimate value that would make a widened `seen` ambiguous.
  In the **second** loop (the one that checks `OnStuck` after the whole name set is collected, so forward references stay legal), add three rules: `p.OnDone != "" && !seen[p.OnDone]` rejects an `OnDone` naming no producer in the list; `p.OnDone != "" && p.OnDone == p.Name` rejects a self-referencing `OnDone`; and `p.OnStuck != "" && segmentByName[p.OnStuck] != p.Segment` rejects an `OnStuck` whose target is in a different `Segment`.
  Order the two `OnDone` rules so the existence check runs before the self-reference check.
  Order the new same-`Segment` rule **after** the pre-existing check that rejects an `OnStuck` naming no producer in the list, so a typo'd `OnStuck` reports that it names no producer rather than reporting a `Segment` mismatch.
  Getting that order wrong is not hypothetical: a name absent from the list has no entry in `segmentByName`, so the lookup yields the zero value, and against a bouncing producer whose own `Segment` is the empty string the mismatch rule would not even fire — while against one inside a named segment it would fire with a message blaming the wrong thing.
  Do not add a self-reference rule for `OnStuck` — `OnStuck: <self>` stays legal, because it is budgeted and therefore bounded.
  Do not add any reachability or multi-producer `Done`-cycle rule.
  Extend the function's own doc comment, which already explains why the non-obvious rules exist, with the reason `OnDone: <self>` is rejected while `OnStuck: <self>` is not: `Done` routing consumes no budget, so a self-referencing `OnDone` is a statically certain infinite loop.
  Do not add any import to this file.
- **Commit:** `feat(shedengine): validate OnDone existence, OnDone self-reference, OnStuck segment, and per-producer MaxBounces`

### Card 4: Cover the four new rules in `validate_test.go`

- **Context:**
  - `internal/shedengine/validate.go`
  - `internal/shedengine/producer.go`
  - `_mill/discussion.md`
- **Edits:**
  - `internal/shedengine/validate_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Extend the `TestShed_Validate` table in `internal/shedengine/validate_test.go` with one case per new rule plus the passing cases that pin what stays legal.
  Failing cases: `OnDone` naming a producer not in the list; `OnDone` naming its own producer; `OnStuck` naming a producer whose `Segment` differs from the bouncing producer's; a negative `ProducerDef.MaxBounces`.
  Passing cases (`wantErr: ""`): `OnDone` naming a *later* producer, so a forward reference stays legal; `OnStuck` naming a producer sharing the same non-empty `Segment`; `OnStuck` between two producers that both keep `Segment: ""`, which is the existing loom shape and must pass unchanged; `OnStuck` naming its own producer, which stays legal.
  The two-row `validShed()` helper is enough for every case except the same-non-empty-`Segment` one, which needs both rows given the same non-empty `Segment` inside its own `mutate` func — set it there rather than changing `validShed()`'s defaults, so every other case keeps exercising the `Segment: ""` shape.
  Each failing case's `wantErr` substring must be distinct enough to tell the four new rules apart from each other and from the existing `MaxBounces` and `OnStuck` rules — assert on wording unique to the rule under test rather than on a bare field name shared with another rule.
  Do not redeclare `stubProducer` or `validShed`.
- **Commit:** `test(shedengine): cover the four new validate rules`

## Batch Tests

`verify: go test ./internal/shedengine/...` runs the whole `shedengine` package suite, which is the correct scope here: every file this batch touches lives in that one package, the suite is small and fast, and it includes `seam_enforcement_test.go`'s `TestProducerSeamInvariant_AllowlistOnly`, which is the machine check that the three new fields and four new rules introduced no import outside the standard library, `internal/state`, and `internal/lock`.
The new coverage is entirely in `validate_test.go`'s `TestShed_Validate` table (card 4).
`run.go` is untouched by this batch, so every existing routing, pause, and persist test must pass unchanged — a failure in any of them means a field addition broke something it should not have.
