# Discussion: Shed-setup validity checker

```yaml
task: Shed-setup validity checker
slug: shed-setup-validity-checker
status: discussing
parent: main
```

## Problem

`shedengine.Shed` walks a flat, ordered producer list where every routing decision is a per-producer field — `OnDone` on a `Done` verdict, `OnStuck` on a `Stuck` verdict — and list position carries no routing meaning at all.
That design is deliberate, but it means the routing table is a graph nobody checks.
`internal/shedengine/validate.go` catches only local, single-row defects: empty name, nil producer, duplicate name, negative `MaxBounces`, a target naming no row, and `OnDone` naming itself.
It never asks the graph-shaped questions — is every row reachable from where the run actually starts, can the run ever finish, does a `Stuck` bounce ever come back to the gate that bounced it.
`shedengine`'s own package doc states the gap outright: "an omitted `OnDone` is indistinguishable from an intended terminal one and ends the run quietly, so a caller assembling a producer list is responsible for asserting its own routing table exhaustively rather than relying on Shed to catch a missing entry."
Today no caller does.

Why now: two things converge.
The `Segment` field is the only graph-ish check that exists (`validate()` requires a non-empty `OnStuck` to name a row in the same `Segment`), and it is already a no-op in production — nothing outside `internal/shedengine/validate_test.go` sets `Segment`, so `loomshed`'s 13-row list passes that check vacuously.
The planned Shed-recipe work drops `Segment` from recipe rows entirely, so the one existing cross-wiring guard is on its way out.
Meanwhile three of the five `loom: real LLM producers` tasks — `Discussion-Review`, `Plan-Review`, and `Webster-Review`;
the other two, `Discussion-Write` and `Plan-Write`, are plain `SingleLLMProducer` rows with no segment — are each about to hand-wire a two-row `Bouncer`+`Burler` segment whose whole correctness *is* its routing: the `Burler` row must always hand back to its `Bouncer` (`Stuck`, never `Done`), and the `Bouncer`'s `OnDone` must exit the segment.
Hand-wiring three of those with no machine check is what this task exists to make safe — for the graph-shaped half of that wiring, which is most but not all of it;
see "What this catches in a mis-wired perch" under Decisions for the exact line.

## Scope

**In:**

- A new package `internal/shedcheck` holding one exported entry point, `Check(producers []shedengine.ProducerDef, entry string, terminals []string) []Finding`, that inspects an assembled `OnDone`/`OnStuck` graph and returns structural findings.
- Eight finding kinds: `bad-entry`, `no-terminals`, `bad-terminal`, `dangling-target`, `unreachable`, `unexpected-terminal`, `done-cycle`, `blind-gate` (defined under Decisions below, together with the exact `Producer`/`Target` mapping for each).
- A `Finding` value type with a stable machine-readable `Kind`, the offending producer and target names, and a human-readable `Message`, plus a `String()` method.
  `Kind`/`Producer`/`Target` are the pinned contract;
  `Message` wording is deliberately not pinned by any test.
- Table-driven unit tests in `internal/shedcheck` covering every kind, the clean case, and the tolerated-malformed cases.
- A `go test` invariant in `internal/loomshed` asserting loom's real production 13-row list is clean under `Check` with entry `NamePreflight` and terminals `{NameFinalize}`.
- Docs in the same commit: `internal/shedcheck/doc.go`, a new section in `manifest/designs/shed.md`, the module row and shed section in `docs/overview.md`, the piece-3 bullet in `manifest/designs/shed-recipe.md` repointed at the shipped module, and `manifest/roadmap.md` moving this item from Planned to Done.

**Out:**

- Any change to `internal/shedengine` — no new field, no change to `validate()`, no call into `shedcheck` from `Run`.
- Removing the `Segment` field or its `validate()` check. That belongs to the Shed-recipe loader items, which are the ones actually dropping `Segment`.
- Any CLI verb. No `lyx shed check`, no cobra registration, no `output` envelope.
- Any call from `loomshed.New()` or any other production constructor. The enforcement point is a test.
- Any recipe-file parsing or pre-assembly row type. `Check` consumes assembled `ProducerDef`s only.
- Any analysis of `MaxBounces` values, `Segment` values, or producer behaviour. `Check` never calls a `ShedProducer`.
  This is what puts the `Burler`-returns-`Done`-instead-of-`Stuck` mis-wiring out of reach: it is a verdict choice inside `Call`, invisible in the routing graph.
- Any change to `internal/shedadapters`, `internal/landingshed`, or the `loomshed` producer list itself. If the invariant test surfaces a real defect in loom's list, report it — do not silently rewire the list under cover of this task.

## Decisions

### Placement — a new `internal/shedcheck` package, not inside `shedengine`

- Decision: the checker lives in a new package `internal/shedcheck`, which imports `internal/shedengine` and the standard library, and nothing else.
- Rationale: `shedengine`'s identity is a minimal runtime engine whose production imports are stdlib + `internal/state` + `internal/lock` (the Shed Producer-Seam Invariant).
  The checker is an authoring-time analysis, not part of the engine's runtime contract, and putting it in `shedengine` would imply `Run` enforces it — which it deliberately does not (see the entry-point decision below).
  A separate package also gives the eventual recipe loader and any future CLI verb a natural, already-shaped dependency, and keeps the finding model free to grow without touching the engine.
  The direction of the import is the safe one: `shedcheck` → `shedengine` is fine, and the reverse is already forbidden by `internal/shedengine/seam_enforcement_test.go`'s allowlist, so no new invariant is needed to protect it.
- Rejected: exporting an `Analyze` from `shedengine` (couples an authoring-time check to the runtime engine, and invites someone to call it from `Run`);
  a private test helper in `internal/loomshed` (unavailable to the recipe loader, which is the next consumer).

### Both endpoints are told, never inferred — the entry row and the terminal set

- Decision: `Check` takes the entry producer's name and the set of intended terminal rows as explicit arguments.
  An empty entry or one naming no row produces `bad-entry`;
  an empty terminal set produces `no-terminals`;
  a terminal naming no row produces `bad-terminal`.
  When any of the three fires, `Check` returns those findings alone and runs no other check — reachability and terminality are meaningless without told endpoints, so the checker reports that one fact rather than an avalanche of derived noise.
  These are three kinds rather than one `bad-endpoints` kind for a specific reason: every finding must be fully distinguishable by its pinned fields alone (`Kind`, `Producer`, `Target`), and a single kind would collapse "the entry is empty" and "the terminal set is empty" into an identical `Producer: ""`, `Target: ""` finding whose only difference lived in the deliberately-unpinned `Message`.
- Rationale: `Shed` has no entry field and no terminal field.
  A run starts from whatever `current_producer` the status file carries, and the only production seeder is `loomshed.Seed`, which writes `NamePreflight`.
  Defaulting to `Producers[0]` would re-introduce exactly the positional routing meaning `shedengine/doc.go` spends a paragraph disclaiming ("list order is cosmetic, carrying zero routing meaning of its own").
  Inferring roots from rows with no inbound edge is worse still: a legitimate graph can route back into its entry row, leaving zero inferred roots.
  The terminal set is told for the mirror-image reason, and it is what makes the whole checker worth building: `shedengine/doc.go` states outright that "an omitted `OnDone` is indistinguishable from an intended terminal one and ends the run quietly."
  Indistinguishable *to Shed* — but not to a caller who knows which row is supposed to end the run.
  Telling the checker that one fact converts the engine's documented blind spot into a decidable check (`unexpected-terminal` below), and it is precisely the check that catches a `Burler` row wired with `OnDone: ""`, which would otherwise end an entire loom run silently on the first `Done` verdict from a review round.
- Rejected: default to `Producers[0]`;
  infer roots from in-degree;
  infer terminals as "every row with an empty `OnDone`" (that is the defect being looked for — inferring it would define the bug out of existence);
  a single `terminal string` rather than a set (a graph may legitimately have several terminal rows, and a set costs one line more than a scalar while surviving the first graph that needs two).

### Not wired into `shedengine.Run`, and not into `loomshed.New()`

- Decision: nothing in production calls `Check`.
  Its enforcement point in this task is a `go test` in `internal/loomshed`.
- Rationale: `Run` is never told an entry name, and a legitimately resumed run starts mid-graph from a persisted `current_producer` — reachability-from-entry is not a property `Run` is in a position to assert.
  `loomshed.New()` could call it, but then a deliberately-unreachable row during development would break production rather than a test, and the check would re-run on every invocation for a result that is fixed at compile time.
  A test is where a statically-fixed property belongs.
- Rejected: calling `Check` from `Run` (wrong information, wrong time);
  calling it from `loomshed.New()` and returning findings as an error (turns an authoring defect into a runtime failure).

### Input type — assembled `[]shedengine.ProducerDef` only

- Decision: `Check`'s first argument is `[]shedengine.ProducerDef`.
  There is no `shedcheck`-owned row type and no pre-assembly checking path.
- Rationale: the roadmap item specifies "an assembled `OnDone`/`OnStuck` producer graph", and the recipe loader assembles `ProducerDef`s as its own output anyway, so it can check post-assembly with no adapter.
  A parallel minimal row type would be a second input shape maintained for a caller that does not exist.
- Rejected: a `shedcheck.Row{Name, OnDone, OnStuck}` type plus a `FromProducerDefs` adapter.
- Consequence for the implementation: `Check` reads only `Name`, `OnDone`, and `OnStuck`.
  It must never dereference `Producer`, so a `ProducerDef` with a nil `Producer` is a valid input — which is what lets the unit tests build graphs out of bare literals with no fakes at all.

### Finding kinds — eight, each an exact graph property

- Decision: `Check` reports exactly these kinds, and nothing heuristic.

  Edges: for each row `P`, a *done edge* `P → P.OnDone` when `P.OnDone != ""`, and a *stuck edge* `P → P.OnStuck` when `P.OnStuck != ""`.
  An edge whose target names no row is dropped from the graph after being reported (see `dangling-target`).
  Reachability is over both edge types unless stated otherwise.

  **The field-value vs. graph-structure rule.**
  A dropped edge creates one ambiguity — does a row whose `OnDone` is dangling count as having no `OnDone`? — and it is resolved once, globally, rather than per check: **a check that keys on a field value reads the raw field, and a check that keys on graph structure reads the post-drop graph.**
  Concretely, `unexpected-terminal` keys on the raw `OnDone == ""`, so a row with a dangling (therefore non-empty) `OnDone` is *not* an unexpected terminal — it is a `dangling-target`, already reported, and reporting it twice under two names would be a worse diagnosis, not a better one.
  Every other check — `unreachable`, `done-cycle`, `blind-gate` — reads the post-drop graph, so a row whose only outbound edge is dangling is a structural sink, and a row whose stuck edge is dangling has no stuck edge and therefore cannot be a `blind-gate`.

  - **`bad-entry`** — `entry` is empty or names no row.
    At most one finding.
  - **`no-terminals`** — `terminals` is empty or nil.
    At most one finding.
  - **`bad-terminal`** — a supplied terminal name is empty or names no row.
    One finding per offending name.

  These three are terminal: when any fires, they are returned alone and no other check runs.
  - **`dangling-target`** — a row's `OnDone` or `OnStuck` names no row in the list.
    One finding per offending edge.
    The edge is then treated as absent for every structural check per the rule above, so `Check` produces useful output on a list that has never been through `shedengine.validate()` and never walks off the end of the graph.
  - **`unreachable`** — a row not reachable from `entry`.
    One finding per row.
    A told terminal that is unreachable surfaces here rather than under a kind of its own: it is the same defect with the same fix, and a second kind saying "and that one was a terminal" would double-report one row.
  - **`unexpected-terminal`** — a reachable row whose raw `OnDone` is `""` and whose `Name` is not in `terminals`.
    Per `shedengine`'s own routing contract, such a row ends the entire run quietly at `state: done` the first time it returns `Done` — the defect `shedengine/doc.go` names as indistinguishable-to-Shed and hands to the caller to assert.
    One finding per row.
    Note the converse is not a finding: a told terminal with a *non-empty* `OnDone` is legal (it routes on rather than ending the run), and flagging it would mean asserting the caller's terminal set against the graph rather than the graph against the terminal set.

    **What a row that never returns `Done` is expected to set.**
    This check demands a non-empty `OnDone` from every reachable non-terminal row, including rows contractually specified never to return `Done` — a `Burler` being the concrete case (`manifest/roadmap.md` specifies it "always hands back … `Stuck`, never `Done`").
    Such a row sets **`OnDone` to the same target as its `OnStuck`**: for a `Burler`, its own `Bouncer`.
    This is not a workaround for the check, it is the wiring that is correct on its own terms.
    A row whose verdict is supposed to be invariant should route invariantly, so that if it ever *does* return `Done` — contradicting its own contract, via a bug, a refactor, or an engine adapter change — the run continues to the right place instead of ending silently at `state: done` mid-task.
    Leaving `OnDone` empty is the opposite: it converts a contract violation into a silent, total run termination, which is precisely the failure `unexpected-terminal` exists to prevent.
    Note this does not weaken the check into a formality: `OnDone: <its Bouncer>` and `OnDone: ""` are different graphs, and only the second is the dangerous one.
    Nor does it collide with the uncatchable `Burler`-returns-`Done` case below: that case is about which verdict the producer *returns*, which is invisible here either way, and mirroring the two fields makes the graph tolerate both verdicts rather than making a wrong verdict harder to see.
    This paragraph is guidance recorded here for the three perch-wiring tasks to follow;
    it is not work performed by this task, which wires nothing.
  - **`done-cycle`** — a cycle among reachable rows using done edges only, reported as **one finding per member row**, each naming that member and the done edge carrying it round.
    A cycle is a set of mis-wired edges, and one finding per edge is what makes the whole defect expressible in `Producer`/`Target` with their existing meanings — no members list, no special-case field used by exactly one kind.
    Each member row is genuinely mis-wired, so per-row reporting is also the more actionable diagnosis.
    Unlike a stuck bounce, a `Done` route consumes no bounce budget, so such a cycle is a statically certain infinite loop.
    `validate()` already rejects the length-1 case (`OnDone` naming itself), so this catches length ≥ 2.
  - **`blind-gate`** — a reachable row `G` with a non-empty `OnStuck` target `T`, where `G` is not reachable from `T`.
    The bounce is one-way: whatever `T` does, the gate that rejected never re-runs to re-test it.
    This is the "blind gate" the roadmap item names, and it is the check that replaces the departing `Segment` rule — expressed as a real graph property rather than as a matching label.
    A self-bounce (`T == G`) is trivially not blind and is never reported;
    it is legal and budgeted.

- Rationale: every one of these is an exact property of the routing graph, decidable with no guesswork, so each finding is a real defect rather than a suggestion.
  `dangling-target` overlaps `validate()`'s own dangling check on purpose — `Check` must be robust standing alone, and duplicating one cheap check is better than crashing on input `validate()` has not yet seen.
- Rejected: reporting only `unreachable` + `blind-gate` (the two the roadmap names literally — but `unexpected-terminal` and `done-cycle` are the same class of defect and just as cheap);
  adding heuristic style checks such as "a gate with no `OnDone` exit" (a real graph can legitimately end on a gate).
- Explicitly not findings: `OnStuck: ""` (the escalate-to-human sentinel — a documented, intended halt);
  `OnDone: ""` on a told terminal (the intended end of the run);
  a self-referencing `OnStuck`;
  any `MaxBounces` value;
  any `Segment` value.

### What this catches in a mis-wired perch — and the one case it cannot

The motivating consumer is the five upcoming `loom: real LLM producers` tasks, each hand-wiring a `Bouncer`+`Burler` pair.
Stating exactly which mis-wirings the checker detects is part of this task's contract, because an over-claimed guarantee is worse than a narrow one:

- **`Burler` wired with `OnDone: ""`** — caught, as `unexpected-terminal`.
  This is the dangerous one: a `Done` verdict from a fix round would end the whole loom run at `state: done`, silently, mid-task.
  It is also the specific mis-wiring an author is most likely to produce, since "this row never returns `Done`" reads as a reason to leave `OnDone` unset.
- **`Bouncer.OnDone` pointing back into its own segment instead of exiting** — caught, as `unreachable` on every row downstream of the segment, or as `unexpected-terminal`/`unreachable` when the segment is the last one before the terminal.
  The diagnosis names the symptom (rows nothing can reach) rather than the cause (a segment with no exit), which is a weaker message but a true one.
- **`Bouncer.OnStuck` pointing somewhere that never routes back to the `Bouncer`** — caught, as `blind-gate`.
  This is the review-gate-that-never-re-tests case, and it is the whole reason `blind-gate` exists.
- **`Burler` handing back via `OnDone: Bouncer` instead of `OnStuck: Bouncer`** — **not caught, and cannot be.**
  Both wirings make the `Bouncer` reachable from the `Burler`, so the routing graph is identical under either;
  the difference lives entirely in which verdict the producer returns, which is behaviour inside `Call`, not a property of the graph.
  A graph checker that claimed to catch it would be guessing.
  The real consequence is a budget one — a `Done` route consumes no bounce budget, so the round is counted against the `Bouncer`'s budget rather than the `Burler`'s — and catching it belongs to whatever mechanism eventually asserts producer *behaviour*, not here.
  The roadmap does document the requirement in verdict terms, but in one place only: the `loom: Discussion-Review producer` item states it outright ("always hands back … `Stuck`, never `Done`").
  The `Plan-Review` and `Webster-Review` items incorporate it by reference — each says "same shape as `Discussion-Review producer` above" and never restates the verdict rule — so a reader who starts at either of those two does not see it stated.
  That is a documentation weakness in those items rather than something this task changes, but the plan writer should know the requirement is not restated at every site an author might start from.

- Rationale for stating this at all: Testing §2's invariant test is described below as the thing that fires when a future task mis-wires a perch.
  That claim is true for three of the four shapes above and false for the fourth, and a plan writer needs to know which, so the test's own comment can say what it does and does not guarantee rather than implying total coverage.

### Single severity, discriminated by `Kind`

- Decision: `Finding` carries no severity field.
  Every finding is a defect;
  `Kind` is the stable machine-readable discriminator a caller filters or allowlists on.
- Rationale: all eight checks are exact graph properties, so a warning tier would carry no information — there is no finding here that is "probably fine".
  A caller that genuinely wants to tolerate one case can filter on `Kind` and the named producer, which is strictly more precise than a two-level severity.
- Rejected: an `Error`/`Warning` split.

### `Finding`'s shape, and which of its fields the tests pin

- Decision: `Finding` carries four fields — `Kind` (the stable discriminator), `Producer` (the offending row's name), `Target` (the offending `OnDone`/`OnStuck` target), and `Message` (human-readable prose) — plus a `String()` method for test failure output and future reporting.
  `Kind` is a defined string type, `type Kind string`, with one exported constant per kind whose value is the hyphenated name used throughout this document: `KindBadEntry` = `"bad-entry"`, `KindNoTerminals` = `"no-terminals"`, `KindBadTerminal` = `"bad-terminal"`, `KindDanglingTarget` = `"dangling-target"`, `KindUnreachable` = `"unreachable"`, `KindUnexpectedTerminal` = `"unexpected-terminal"`, `KindDoneCycle` = `"done-cycle"`, `KindBlindGate` = `"blind-gate"`.

  **`Producer` and `Target` per kind** — this mapping is the pinned contract, so no case is left for the plan writer to invent:

  | Kind | `Producer` | `Target` |
  |---|---|---|
  | `bad-entry` | the `entry` argument as supplied (`""` when it was empty) | `""` |
  | `no-terminals` | `""` | `""` |
  | `bad-terminal` | the offending terminal name as supplied (`""` when that entry was empty) | `""` |
  | `dangling-target` | the row holding the edge | the dangling target name |
  | `unreachable` | the unreachable row | `""` |
  | `unexpected-terminal` | the row | `""` |
  | `done-cycle` | one member row of the cycle | that member's `OnDone` — the next member round the cycle |
  | `blind-gate` | the gate row that bounced | the gate's `OnStuck` target |

  Note the one case where `Kind` alone is load-bearing: `bad-entry` and `bad-terminal` with an empty supplied name are both `Producer: ""`, `Target: ""`, and only `Kind` separates them — which is exactly why the endpoint kinds were split rather than merged.

  Tests assert the full returned slice as a literal on `Kind`, `Producer`, and `Target` only.
  `Message`'s wording is **not** pinned by any test;
  a test asserts at most that it is non-empty.
- Rationale: `Kind`/`Producer`/`Target` are the contract — a caller filters and acts on those, and pinning them is what makes the deterministic-order decision machine-enforced.
  `Message` is prose for a human reading a test failure, and pinning its wording would mean every future clarification of a message breaks a test that was never checking behaviour.
  Saying this here rather than leaving it to the plan writer avoids the plan inventing message strings and freezing them into assertions.
  Implementation consequence: the comparison is field-by-field, or on a `Message`-free projection of `Finding` — not `reflect.DeepEqual` on the whole struct, which would drag `Message` back in.
- Rejected: pinning `Message` verbatim (freezes prose into the contract);
  dropping `Message` entirely (a `Kind` plus two names is not a usable test-failure line, and `String()` needs something to print).

### Deterministic output order

- Decision: `Check` returns findings in a fixed check order — `bad-entry`, `no-terminals`, `bad-terminal`, `dangling-target`, `unreachable`, `unexpected-terminal`, `done-cycle`, `blind-gate` — and within each check, in producer list order.
  `bad-terminal` findings follow the order the caller supplied the terminals in, not list order.
  For `dangling-target` on a single row, the `OnDone` edge is reported before the `OnStuck` edge.
  A `done-cycle`'s member findings are emitted starting from the member with the lowest list index and following done edges from there;
  when a graph holds more than one done cycle, the cycles themselves are ordered by their own lowest member index.
  Because each member is its own finding carrying `Producer` and `Target`, this ordering is pinned by the same literal-slice assertion as everything else, rather than living in unpinned prose.
  A clean graph returns `nil`, not an empty non-nil slice.
- Rationale: a checker whose output order depends on Go map iteration produces flaky tests and diff noise in any future report, and this repo's tests assert against literal expected lists (see `internal/loomshed/sequence_test.go`'s `wantSequenceOrder` and its comment on why it is a literal rather than a derivation).
  Any intermediate map in the implementation must therefore be iterated via the producer list, never directly.
- Rejected: sorting by producer index first and kind second (interleaves unrelated checks and makes a single check's output harder to read);
  unspecified order.

### Tolerating malformed input

- Decision: `Check` never panics and never returns an error.
  A nil or empty producer list returns endpoint findings only — `bad-entry`, plus `no-terminals` or a `bad-terminal` per supplied name, since no row can match either endpoint.
  A nil `Producer` field is ignored.
  A row with an empty `Name` is kept in the list for indexing purposes but can only ever be reached by an edge naming `""` — which by definition does not exist, since an empty `OnDone`/`OnStuck` is the terminal/escalate sentinel — so it will surface as `unreachable`.
  A duplicate `Name` resolves to the first occurrence in list order, with these exact consequences, stated so the test literal is read off this paragraph rather than invented: the first occurrence is the only row any edge can reach, so the **shadowed later duplicate is reported as `unreachable`**, and any other check that would key on the shadowed row's own fields is skipped for it, since `Check`'s model of the graph is the model `Run` would walk.
  `Check` does **not** report duplication as a kind of its own.
  That is a deliberate division of labour, not an oversight: `shedengine.validate()` already rejects a duplicate `Name` with its own distinct message, so a `duplicate-name` kind here would be a second, weaker report of a defect the engine refuses to run at all.
  The cost is a misleading diagnosis in the one case where `Check` runs on a list that never reaches `validate()` — the shadowed row reads as "unreachable" rather than "duplicate" — and that cost is accepted rather than paid for with an overlapping kind.
  The package doc must say so explicitly, so a reader who hits it is not left guessing.
- Rationale: `Check` is usable at points where `shedengine.validate()` has not run and may never run, so every defect `validate()` would have rejected must degrade into a finding or a defined no-op rather than a crash.
  Resolving a duplicate to the first occurrence matches `shedengine.findProducer`'s own linear-scan-first-match behaviour, so the checker's model of the graph matches the one `Run` would actually walk.
- Rejected: returning an error on malformed input;
  requiring the caller to run `validate()` first.

### `Segment` is untouched by this task

- Decision: this task does not remove the `Segment` field, does not remove `validate()`'s same-`Segment` rule on `OnStuck`, and `shedcheck` never reads `Segment`.
- Rationale: `Segment` removal is the Shed-recipe loader items' business — they are the ones dropping it from recipe rows.
  Doing it here would widen this task into `shedengine` for no gain, and the roadmap explicitly says this item "can land before or independent of the other three items".
  The `blind-gate` check is what makes the eventual removal safe, which is the whole point of landing this first.
- Rejected: removing the `Segment` check in the same commit.

### Documentation placement

- Decision: the design goes in a new section of `manifest/designs/shed.md` (the settled Shed doc), not in a new design file and not in `shed-recipe.md`.
  `manifest/designs/shed-recipe.md`'s "Pieces to build" item 3 is updated to point at the shipped module instead of describing an unbuilt one.
  `docs/overview.md` gains an `internal/shedcheck/` row in the module tree and a mention in its **shed** section.
  `manifest/roadmap.md` moves the item from the Planned "Shed recipe" group to Done.
- Rationale: the checker is explicitly independent of the recipe work, so documenting it only inside a doc marked "DRAFT — do not implement from this doc yet" would bury a shipped module in an unsettled design.
  `shed.md` is where the routing contract this checker enforces already lives.
- Rejected: a new `manifest/designs/shed-check.md` (a ~40-line module does not need its own design doc alongside `shed.md`);
  documenting it only in `shed-recipe.md`.

## Technical context

- `internal/shedengine/producer.go` — `ProducerDef{Name, Producer, OnStuck, OnDone, Segment, MaxBounces}`.
  Read the field comments on `OnStuck` and `OnDone`: `""` is load-bearing on both (escalate-to-human, and end-the-run-quietly respectively).
- `internal/shedengine/validate.go` — the existing local checks, and the extended comment explaining why `OnDone: <self>` is rejected while `OnStuck: <self>` is legal (Done routing consumes no bounce budget).
  That asymmetry is the direct rationale for the `done-cycle` kind: `shedcheck` generalises the length-1 case `validate()` already covers.
- `internal/shedengine/run.go` — `findProducer` is a linear first-match scan;
  `Run` starts from the status file's `current_producer` and hard-errors when it names no row.
  This is why the entry is told, and why duplicate names resolve first-wins.
- `internal/shedengine/doc.go` — the "Routing: OnDone and OnStuck, no positional fallback" section, including the sentence naming the caller's own responsibility to assert its routing table.
  Worth quoting in the new `shed.md` section as the gap this module closes.
- `internal/loomshed/loomshed.go:137-151` — the production 13-row list.
  Entry is `NamePreflight` (`internal/loomshed/seed.go` writes it as the seed's `CurrentProducer`).
  Terminal is `NameFinalize`, whose `OnDone` is `""`.
  Three rows bounce backwards: `DiscussionValidate` and `DiscussionReview` both `OnStuck: DiscussionWrite`, `PlanValidate` and `PlanReview` both `OnStuck: PlanWrite`, `WebsterReview` `OnStuck: Webster`.
  Every one of those bounce targets routes forward again through `OnDone` back to the bouncing row, and every row except `NameFinalize` carries a non-empty `OnDone`, so loom's list should come out clean under both `blind-gate` and `unexpected-terminal` — the invariant test is expected to pass on first run, and a failure means either a real latent defect or a bug in `Check`.
- `internal/loomshed/loomshed_test.go` — `TestNew_PassesShedValidation` is the closest existing test in shape and is the right neighbour for the new invariant test;
  `TestNew_ProducerTable` shows how the package builds a `New()` result in a test.
- `internal/loomshed/sequence_test.go` — `wantSequenceOrder` and its comment are the local precedent for asserting against a literal rather than a computed expectation.
- No production code anywhere sets `Segment`.
  The only place that assigns it is `internal/shedengine/validate_test.go`;
  the only other reference outside `shedengine`'s own field declaration and `validate()` is `internal/loomshed/loomshed_test.go:81-82`, which reads it to assert every production row leaves it empty.
  This is what makes the existing same-`Segment` rule vacuous today.
- The done-edge subgraph is *functional*: every row has at most one outgoing done edge.
  Cycle detection there is therefore a simple walk with visited marks, not a general SCC algorithm — no need to reach for anything heavier.

## Constraints

From `CONSTRAINTS.md`:

- **Shed Producer-Seam Invariant** — `internal/shedengine` production code imports only stdlib, `internal/state`, and `internal/lock`, enforced by `internal/shedengine/seam_enforcement_test.go`.
  This is why `shedcheck` is a separate package, and it already forbids the dangerous direction (`shedengine` importing `shedcheck`).
  Do not add `shedcheck` to that allowlist.
- **Cwd Resolution Invariant** — `shedcheck` resolves no cwd, touches no filesystem, and names no path.
  It is a pure function over a slice.
- **Told-Geometry Invariant** — nothing in `shedcheck` derives geometry;
  it never sees a path at all.
- **CLI / Cobra Invariant** — not engaged: this task registers no cobra command, so there is no `Command()`/`RunCLI` seam, no `Short`, and no help-tree test to update.
- **Documentation Lifecycle** — this task adds a module, so `docs/overview.md` and a module doc must land in the same commit (see the documentation-placement decision above), and the roadmap item moves in that same commit.

Discovered during discussion:

- No new cross-cutting invariant is introduced, so `CONSTRAINTS.md` needs no new section.
  Say so explicitly in the commit rather than leaving it ambiguous.
- `internal/shedcheck` does not need its own `leaf_enforcement_test.go`: it is not a leaf (it imports `shedengine`), and no invariant claims it is.
- `internal/loomshed/seam_enforcement_test.go`'s `loomshedAllowedImports` is **not** touched.
  The new invariant test is a `_test.go` file, and that walk skips `_test.go` files outright (`seam_enforcement_test.go:58`), so the allowlist governs production imports only and adding `shedcheck` to it would be both unnecessary and misleading — it would assert `shedcheck` is a permitted *production* dependency of `loomshed`, which this task deliberately does not make it.
- Repo comment convention (see `golang:golang-comments` and the existing `shedengine` files): file-level comments state what the file implements, and non-obvious decisions are explained in the code where they live, not only in the design doc.
  The `dangling-target`-drops-the-edge behaviour and the functional-graph cycle walk both need such a comment.

## Testing

TDD candidates — write the tests first for both:

1. **`internal/shedcheck`, table-driven, one case per finding kind plus the clean case.**
   Each case is a bare `[]shedengine.ProducerDef` literal with `Producer` left nil, an entry name, a terminal set, and the expected `[]Finding` as a literal — asserted on the full slice including order, over `Kind`/`Producer`/`Target` only per the `Finding`-shape decision, so the deterministic-order decision is machine-enforced rather than a review obligation.
   Scenarios that must be covered:
   - Clean linear graph ending at a told terminal → `nil`.
   - Clean graph with a legitimate backwards bounce whose target routes forward again (loom's real shape in miniature) → `nil`.
   - **Clean graph whose gate-return path exists only through a stuck edge** — `G` bounces to `T`, and the only route from `T` back to `G` is `T`'s own `OnStuck` — asserted clean.
     This case is load-bearing, not decorative: it is the perch's own shape (the `Burler` returns to its `Bouncer` via `OnStuck`), and without it a done-edge-only reachability implementation would pass every other clean-graph scenario in this list while flagging every future perch as a `blind-gate`.
   - Self-`OnStuck` → `nil` (not a blind gate).
   - `OnStuck: ""`, and `OnDone: ""` on a told terminal → neither is a finding.
   - Empty entry, and entry naming no row → `bad-entry` alone.
   - Empty/nil terminal set → `no-terminals` alone.
   - A terminal naming no row, and an empty terminal name → `bad-terminal` per offending name, in caller-supplied order.
   - Each of the above asserted on a graph that *also* carries other defects, proving the endpoint kinds are returned alone and suppress every other check.
   - Empty entry together with an empty terminal name → two findings distinguishable only by `Kind`, both with `Producer: ""` and `Target: ""`.
   - `OnDone` and `OnStuck` each naming no row → `dangling-target` per edge, `OnDone` before `OnStuck` on the same row, and the dropped edge does not then produce a phantom reachability result.
   - **A reachable row with a dangling `OnDone`** → `dangling-target` only, **not** also `unexpected-terminal` — the field-value vs. graph-structure rule, pinned.
   - **A reachable row with a dangling `OnStuck`** → `dangling-target` only, and no `blind-gate` for that row.
   - A row reachable only through a stuck edge → not `unreachable`.
   - A row reachable through nothing → `unreachable`;
     an unreachable told terminal → `unreachable` and nothing else.
   - A reachable non-terminal row with `OnDone: ""` → `unexpected-terminal` (the `Burler` mis-wiring in miniature);
     an *unreachable* row with `OnDone: ""` → `unreachable` only, no `unexpected-terminal`.
   - A told terminal with a non-empty `OnDone` → not a finding.
   - Two-row and three-row done cycles → one `done-cycle` finding per member, starting at the lowest list index and following done edges, each finding's `Target` naming the next member.
   - Two disjoint done cycles in one graph → cycles ordered by their own lowest member index.
   - A blind gate: `G` bounces to `T`, `T` routes forward past `G` and never back → `blind-gate` on `G`.
   - Nil producer list, empty producer list, empty `Name` → the tolerated behaviours named under the malformed-input decision, no panic.
   - Duplicate `Name` → the shadowed later row comes back as `unreachable`, and no `duplicate`-flavoured kind is emitted, exactly as that decision states.
   - A single graph carrying several kinds at once → findings come back in the fixed check order.
2. **`internal/loomshed`, the invariant test.**
   Build the production list via `New()` (following `TestNew_PassesShedValidation`'s existing setup) and assert `shedcheck.Check(sh.Producers, NamePreflight, []string{NameFinalize})` returns no findings, failing with each finding's `String()` so a future rewiring reports what broke rather than just a count.
   This is the test that fires when one of the five upcoming `loom: real LLM producers` tasks mis-wires a `Bouncer`/`Burler` pair — name that purpose in the test's own comment, and name its limit in the same breath: it catches a `Burler` left with `OnDone: ""`, a `Bouncer` whose `OnDone` never exits its segment, and a `Bouncer` whose `OnStuck` never routes back;
   it does **not** catch a `Burler` handing back via `OnDone` instead of `OnStuck`, for the reason given under "What this catches in a mis-wired perch".
   A comment claiming unqualified perch coverage would be false.

   **This test must migrate when the recipe conversion lands.**
   `manifest/roadmap.md` sequences "loom: convert to a Shed recipe" *before* the three perch-wiring tasks this guard exists for, and that item replaces `loomshed.go`'s Go literal — the very thing this test reads — with a recipe file.
   The guard must move onto the recipe-assembled list at that point rather than being deleted alongside the literal it happens to be written against.
   Say so in the test's own comment, so the conversion task's author finds the instruction at the site they are about to change instead of having to infer it.
   Note this is a note *to* the later task, not work in this one: nothing here builds a recipe or anticipates its shape.

Not tested: producer behaviour of any kind.
`Check` never calls `ShedProducer.Call`, and a test asserting that would be asserting the absence of code.

Standard repo verify applies — `gofmt`, `go vet`, `go build ./...`, `go test ./...` — plus the existing `internal/shedengine/seam_enforcement_test.go` must still pass unchanged, confirming the new package did not leak into the engine's allowlist.

## Q&A log

- **Q:** Where does the checker live — new `internal/shedcheck`, inside `shedengine`, or a `loomshed` test helper? **A:** [auto-pick] New package `internal/shedcheck`, importing `shedengine`. **Why:** keeps the engine's minimal stdlib+state+lock identity intact, avoids implying `Run` enforces the check, and gives the recipe loader a ready dependency;
  the dangerous import direction is already blocked by the Shed Producer-Seam Invariant test.
- **Q:** How does the checker learn the graph's entry row? **A:** [auto-pick] The caller supplies it explicitly;
  an empty or unknown entry is itself a finding. **Why:** `Shed` has no entry field — a run starts from the status file's `current_producer` — and defaulting to `Producers[0]` would re-introduce the positional routing meaning `shedengine/doc.go` explicitly disclaims.
- **Q:** Which defects does it report? **A:** [auto-pick] Six kinds at the time — superseded twice since, by the two review-round entries below: round 1 replaced `no-terminal` with `unexpected-terminal`, and round 2 split `bad-endpoints` into `bad-entry`/`no-terminals`/`bad-terminal`, bringing the total to eight. **Why:** each is an exact graph property rather than a heuristic;
  `dangling-target` is duplicated from `validate()` deliberately so `Check` is robust on input that has never been validated.
- **Q:** Severity model — one level or an error/warning split? **A:** [auto-pick] Single severity, discriminated by a stable machine-readable `Kind`. **Why:** none of the eight checks can fire on a graph that is actually fine, so a warning tier would carry no information;
  `Kind` plus producer name is a more precise filter than a severity anyway.
- **Q:** Does this task remove `Segment` and its `validate()` check? **A:** [auto-pick] No — additive only. **Why:** `Segment` removal belongs to the recipe-loader items that actually drop it, and the roadmap says this item can land independently;
  `blind-gate` is what makes that later removal safe.
- **Q:** How is the checker exposed — library only, a CLI verb, or wired into `Run`? **A:** [auto-pick] Library only, plus a `go test` invariant in `internal/loomshed`. **Why:** no recipe file exists yet for a CLI verb to point at, and `Run` is never told an entry name and legitimately resumes mid-graph, so reachability-from-entry is not a property it can assert.
- **Q:** Is it also called from `loomshed.New()` at runtime? **A:** [auto-pick] No — the test invariant is the enforcement point. **Why:** the property is fixed at compile time, and a runtime call would turn a development-time unreachable row into a production failure.
- **Q:** Does `Check` take assembled `ProducerDef`s, or its own pre-assembly row type? **A:** [auto-pick] Assembled `[]shedengine.ProducerDef` only. **Why:** the roadmap specifies an assembled graph and the recipe loader produces `ProducerDef`s anyway;
  a parallel row type would be a second input shape with no caller.
- **Q:** Where is this documented? **A:** [auto-pick] A new section in `manifest/designs/shed.md`, plus `doc.go`, the `docs/overview.md` module row and shed section, a repointed `shed-recipe.md` piece-3 bullet, and the roadmap move. **Why:** `shed-recipe.md` is marked DRAFT/do-not-implement, so a shipped module documented only there would be buried in an unsettled design;
  `shed.md` already holds the routing contract this checker enforces.
- **Q:** Review round 1 found that none of the six kinds detect the `Bouncer`/`Burler` mis-wirings the task exists to prevent. Add a kind, or drop the coverage claim? **A:** [auto-pick] Add one: tell `Check` the intended terminal set, and report a reachable non-terminal row with `OnDone: ""` as `unexpected-terminal`;
  then state precisely which of the four perch mis-wirings are and are not covered. **Why:** the dangerous case (a `Burler` left with `OnDone: ""` ending the entire run on its first `Done` verdict) becomes decidable the moment the caller says which row is supposed to end the run — the exact fact `shedengine/doc.go` says Shed itself cannot know.
  The remaining uncovered case (`Burler` handing back via `OnDone` rather than `OnStuck`) is a verdict choice inside `Call`, not a graph property, so it is documented as out of reach rather than guessed at.
- **Q:** Review round 1 found `dangling-target`'s "edge treated as absent" contradicts `no-terminal`'s phrasing on `OnDone == ""` — is a row with a dangling `OnDone` a terminal or not? **A:** [auto-pick] Resolve it once with a global rule rather than per check: a check keying on a **field value** reads the raw field, a check keying on **graph structure** reads the post-drop graph;
  so a dangling `OnDone` is a `dangling-target` and never also an `unexpected-terminal`, and a dangling `OnStuck` means no `blind-gate` for that row. **Why:** a per-check answer would leave the next check added to the package ambiguous again, and double-reporting one edge under two kinds is a worse diagnosis than one precise finding.
- **Q:** Review round 2 found two kinds have no expressible mapping into the pinned `Kind`/`Producer`/`Target` fields — `done-cycle` "names its members" with no members field, and `bad-endpoints` never said which field carries the offending name. Add fields, or restate? **A:** [auto-pick] Neither: express both in the existing fields.
  Split `bad-endpoints` into `bad-entry`/`no-terminals`/`bad-terminal` so no two findings collapse into an identical `Producer: ""`, `Target: ""` pair, and emit `done-cycle` as one finding per member row with `Target` naming the next member. **Why:** a members field would be a special case used by exactly one kind, and leaving cycle membership in the unpinned `Message` would have made the "members starting at the lowest list index" ordering rule unenforceable by the very test that is supposed to machine-enforce determinism.
  A cycle is a set of mis-wired edges, and `Producer`+`Target` already express exactly one edge — so per-member findings need no new field and give a more actionable diagnosis.
  A per-kind field-mapping table is now stated outright rather than left to the plan writer.
- **Q:** Review round 3 found `unexpected-terminal` demands a non-empty `OnDone` from every reachable non-terminal row, but a `Burler` is contractually specified never to return `Done` — what is such a row supposed to set? **A:** [auto-pick] It sets `OnDone` to the same target as its `OnStuck` (for a `Burler`, its own `Bouncer`), never empty. **Why:** a row whose verdict is meant to be invariant should route invariantly, so a contract violation — a `Burler` that somehow does return `Done` — continues to the right place instead of silently ending the whole run at `state: done`.
  That is the correct wiring on its own terms rather than an accommodation of the check, and it does not soften the check: `OnDone: <its Bouncer>` and `OnDone: ""` are different graphs, and only the second is dangerous.
