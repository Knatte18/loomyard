# Batch: shedcheck-package

```yaml
task: "Shed-setup validity checker"
batch: "shedcheck-package"
number: 1
cards: 3
verify: go test ./internal/shedcheck/...
depends-on: []
```

## Batch Scope

This batch delivers the whole new package `internal/shedcheck`: the `Finding` value type with its `Kind` discriminator, the `Check` entry point that inspects an assembled `OnDone`/`OnStuck` graph, the table-driven test suite that pins the eight finding kinds and the deterministic output order, and the package doc.
It is one batch because the three files are one module with one exported surface — `Check` cannot be written without `Finding`, and the test suite asserts the full output slice of `Check` as a literal, so splitting it across batches would leave a batch whose own `verify:` cannot express what it built.

The external interface batch 2 consumes is exactly two exported names: `shedcheck.Check(producers []shedengine.ProducerDef, entry string, terminals []string) []Finding` and `Finding.String() string`.

Batch-local decisions beyond `## Shared Decisions` in the overview: none.
The package imports `internal/shedengine` and the standard library, and nothing else — no `internal/state`, no `internal/lock`, no `internal/logger`, no `internal/lyxcwd`.

## Cards

### Card 1: `Finding`, `Kind`, and the eight kind constants

- **Context:**
  - `internal/shedengine/producer.go`
  - `internal/shedengine/doc.go`
- **Edits:** none
- **Creates:**
  - `internal/shedcheck/finding.go`
  - `internal/shedcheck/finding_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Create the package `shedcheck` at `internal/shedcheck/finding.go`, declaring `package shedcheck`.
  Write `internal/shedcheck/finding_test.go` first (package `shedcheck`, an internal test), then `internal/shedcheck/finding.go` until it passes.

  `finding.go` declares:

  - `type Kind string` — the stable, machine-readable discriminator a caller filters or allowlists on.
  - Exactly eight exported constants of type `Kind`, in this declaration order, with these exact string values: `KindBadEntry = "bad-entry"`, `KindNoTerminals = "no-terminals"`, `KindBadTerminal = "bad-terminal"`, `KindDanglingTarget = "dangling-target"`, `KindUnreachable = "unreachable"`, `KindUnexpectedTerminal = "unexpected-terminal"`, `KindDoneCycle = "done-cycle"`, `KindBlindGate = "blind-gate"`.
    The declaration order is also `Check`'s fixed check order, so keep them in one `const` block in that order and say so in the block's comment.
  - `type Finding struct` with exactly four exported fields: `Kind Kind`, `Producer string`, `Target string`, `Message string`.
    Document on the type that `Kind`, `Producer`, and `Target` are the pinned contract and `Message` is human-readable prose whose wording is deliberately not pinned by any test.
  - `func (f Finding) String() string` — a single line suitable for a test-failure message, containing the `Kind`, the `Producer`, the `Target`, and the `Message`.
    Its exact format is not part of the contract;
    the test asserts only that the result is non-empty and contains each of those four values as a substring.

  There is no severity field, and none is to be added — every finding is a defect, and `Kind` is what a caller discriminates on.

  `finding_test.go` covers:

  - One test asserting each of the eight constants has its exact hyphenated string value, written as a literal table of `{Kind, string}` pairs so a renamed value fails loudly.
  - One test on `String()` for a fully-populated `Finding` and for a `Finding` whose `Producer`, `Target`, and `Message` are all `""`: in both cases the result is non-empty, and in the populated case it contains each of the four field values.

  Do not add a `Severity` field, do not add a members or list-valued field of any kind, and do not reference `ProducerDef.Segment` anywhere in this file.
- **Commit:** `feat(shedcheck): add Finding, Kind, and the eight finding-kind constants`

### Card 2: `Check` — the eight graph checks in fixed order

- **Context:**
  - `internal/shedengine/producer.go`
  - `internal/shedengine/validate.go`
  - `internal/shedengine/run.go`
  - `internal/shedengine/doc.go`
  - `internal/shedcheck/finding.go`
  - `internal/loomshed/sequence_test.go`
  - `_mill/discussion.md`
- **Edits:** none
- **Creates:**
  - `internal/shedcheck/check.go`
  - `internal/shedcheck/check_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Write `internal/shedcheck/check_test.go` first (package `shedcheck`, table-driven, an internal test), then `internal/shedcheck/check.go` until it passes.
  `internal/shedcheck/check.go` imports `github.com/Knatte18/loomyard/internal/shedengine` plus standard library packages only.

  **Signature.** `func Check(producers []shedengine.ProducerDef, entry string, terminals []string) []Finding`.
  It never panics, never returns an error, and returns `nil` — not an empty non-nil slice — when it finds nothing.
  It reads only `Name`, `OnDone`, and `OnStuck` off each `shedengine.ProducerDef`.
  It must never dereference the `Producer` field, so a `ProducerDef` with a nil `Producer` is valid input;
  it must never read `Segment` or `MaxBounces`.

  **Index and live rows.** Build `indexByName map[string]int` in a single forward pass, recording the *first* occurrence of each `Name` only (matching `findProducer`'s linear first-match scan in `internal/shedengine/run.go`).
  A row is **live** when its own list index equals `indexByName[row.Name]`, and **shadowed** otherwise.
  A shadowed row contributes no edges to the graph and is skipped by every check except `unreachable`.
  The map is used for lookups only and is never range-iterated.

  **Endpoint checks, and their short circuit.** In this order:

  1. `KindBadEntry` — when `entry == ""` or `indexByName` has no such key.
     At most one finding.
     `Producer` is the `entry` argument as supplied (so `""` when it was empty), `Target` is `""`.
  2. `KindNoTerminals` — when `terminals` is nil or has length zero.
     At most one finding.
     `Producer` and `Target` are both `""`.
  3. `KindBadTerminal` — one finding per supplied terminal name that is `""` or has no key in `indexByName`, emitted in the order the caller supplied `terminals`, not in producer list order.
     `Producer` is the offending terminal name as supplied, `Target` is `""`.

  If any of these three produced a finding, return those findings alone and run no further check.
  Reachability and terminality are meaningless without told endpoints, so `Check` reports that one fact rather than an avalanche of derived noise.
  A nil or empty `producers` slice therefore always returns endpoint findings only, since no row can match either endpoint.

  **The field-value vs. graph-structure rule.** Resolve the edge-drop ambiguity once, globally: a check that keys on a **field value** reads the raw field, and a check that keys on **graph structure** reads the post-drop graph.
  Write this rule as a comment in `internal/shedcheck/check.go` at the point the edges are built, since it reads as arbitrary without the explanation.
  Its two consequences are pinned by tests: `KindUnexpectedTerminal` keys on the raw `OnDone == ""`, so a row with a dangling (therefore non-empty) `OnDone` is never also an unexpected terminal;
  and a row whose stuck edge is dangling has no stuck edge at all and therefore can never be a `KindBlindGate`.

  **`KindDanglingTarget`.** Iterate live rows in list order.
  For each, check `OnDone` before `OnStuck`: a non-empty value with no key in `indexByName` yields one finding with `Producer` set to the row's `Name` and `Target` set to the dangling value.
  That edge is then absent from the graph every structural check below reads, so `Check` produces useful output on a list that has never been through `shedengine.validate()` and never walks off the end of the graph.
  Shadowed rows are skipped here — their fields are not part of the graph `Run` would walk.

  **Reachability.** Build the reachable set as a set of **list indices**, seeded with `indexByName[entry]`, walking both done and stuck edges of live rows over the post-drop graph;
  from a target name `T` the walk moves to the single index `indexByName[T]`.

  **`KindUnreachable`.** One finding per row, in list order, whose own list index is not in the reachable set.
  `Producer` is the row's `Name`, `Target` is `""`.
  A shadowed duplicate always lands here, because its index is never reached;
  so does a row with an empty `Name`, because no edge can name `""`.
  A told terminal that is unreachable surfaces here and under no kind of its own.
  `Check` does **not** report duplication as a kind of its own — `shedengine.validate()` already rejects a duplicate `Name` with its own distinct message.

  **`KindUnexpectedTerminal`.** One finding per live, reachable row, in list order, whose **raw** `OnDone` is `""` and whose `Name` is not in `terminals`.
  `Producer` is the row's `Name`, `Target` is `""`.
  Build the terminal-name lookup as a set, consulted by lookup only.
  The converse is not a finding: a told terminal with a non-empty `OnDone` is legal and is never reported.

  **`KindDoneCycle`.** Over live, reachable rows and **done edges only** (post-drop), find every cycle.
  The done-edge subgraph is functional — every row has at most one outgoing done edge — so this is a walk with visited marks, not a general SCC algorithm;
  state that in a comment in `internal/shedcheck/check.go` so the next reader does not reach for something heavier.
  Report **one finding per member row**: `Producer` is that member's `Name`, `Target` is that member's own `OnDone` — the next member round the cycle.
  Within one cycle, start at the member with the lowest list index and follow done edges from there.
  Collect all cycles first, then sort the collected cycles by their own lowest member index before emitting;
  discovery order is not the required order, because a walk starting at a row outside a cycle can reach a high-index cycle before a lower-index one is ever visited.
  A cycle of length 1 (a row whose `OnDone` names itself) is reported like any other, with `Producer` and `Target` both naming that row — see the overview's `done-cycle` Shared Decision for why it is not special-cased out.

  **`KindBlindGate`.** For each live, reachable row `G` in list order whose post-drop stuck edge names a row `T`: if `T` is the same row as `G`, report nothing (a self-bounce is trivially not blind, and it is legal and budgeted).
  Otherwise compute reachability from `T` over both edge types, starting at `T` itself, and if `G`'s index is not in that set, emit one finding with `Producer` set to `G.Name` and `Target` set to `G.OnStuck`.
  A row whose `OnStuck` is `""` is the escalate-to-human sentinel and is never a finding.

  **Output order.** The returned slice is the concatenation of the checks above in exactly the constant-declaration order from `internal/shedcheck/finding.go`, and within each check in producer list order — except `KindBadTerminal`, which follows caller-supplied order.
  Do not sort by producer index first and kind second.

  **`check_test.go`** is table-driven with one struct field per input (`producers`, `entry`, `terminals`) and a `want []Finding` literal compared field-by-field over `Kind`, `Producer`, and `Target` only, over the **full slice including order**, so the deterministic-order decision is machine-enforced.
  Never use `reflect.DeepEqual` on `Finding`.
  Assert separately that every returned finding's `Message` is non-empty.
  Each case builds bare `[]shedengine.ProducerDef` literals with `Producer` left nil — no fakes of any kind.
  Cover at minimum:

  - Clean linear graph ending at a told terminal → `nil`.
  - Clean graph with a legitimate backwards bounce whose target routes forward again (loom's real shape in miniature) → `nil`.
  - Clean graph whose gate-return path exists only through a stuck edge: `G` bounces to `T`, and the only route from `T` back to `G` is `T`'s own `OnStuck` → asserted clean.
    This case is load-bearing: it is the perch's own shape, and without it a done-edge-only reachability implementation would pass every other clean-graph case here while flagging every future perch as a blind gate.
  - Self-`OnStuck` → `nil`.
  - `OnStuck: ""` on an ordinary row, and `OnDone: ""` on a told terminal → neither is a finding.
  - Empty `entry`, and an `entry` naming no row → `KindBadEntry` alone.
  - Nil `terminals`, and empty `terminals` → `KindNoTerminals` alone.
  - A terminal naming no row, and an empty terminal name → one `KindBadTerminal` per offending name, in caller-supplied order.
  - Each endpoint case above asserted on a graph that also carries other defects, proving the endpoint kinds are returned alone and suppress every other check.
  - Empty `entry` together with an empty terminal name → two findings distinguishable only by `Kind`, both with `Producer: ""` and `Target: ""`.
  - `OnDone` and `OnStuck` on one row each naming no row → one `KindDanglingTarget` per edge, `OnDone` before `OnStuck`, and the dropped edges produce no phantom reachability result.
  - A reachable row with a dangling `OnDone` → `KindDanglingTarget` only, **not** also `KindUnexpectedTerminal`.
  - A reachable row with a dangling `OnStuck` → `KindDanglingTarget` only, and no `KindBlindGate` for that row.
  - A row reachable only through a stuck edge → not `KindUnreachable`.
  - A row reachable through nothing → `KindUnreachable`;
    an unreachable told terminal → `KindUnreachable` and nothing else.
  - A reachable non-terminal row with `OnDone: ""` → `KindUnexpectedTerminal`;
    an unreachable row with `OnDone: ""` → `KindUnreachable` only.
  - A told terminal with a non-empty `OnDone` → not a finding.
  - Two-row and three-row done cycles → one `KindDoneCycle` per member, starting at the lowest list index and following done edges, each finding's `Target` naming the next member.
  - A one-row done cycle (`OnDone` naming itself) → one `KindDoneCycle` whose `Producer` and `Target` both name that row.
  - Two disjoint done cycles in one graph, arranged so the lower-index cycle is discovered second by a naive list-order walk → cycles ordered by their own lowest member index.
  - A blind gate: `G` bounces to `T`, `T` routes forward past `G` and never back → `KindBlindGate` on `G`.
  - Nil `producers`, empty `producers`, and a row with an empty `Name` → the tolerated behaviours above, with no panic.
  - Duplicate `Name` → the shadowed later row comes back as `KindUnreachable`, with no duplicate-flavoured kind and no finding keyed on the shadowed row's own `OnDone`/`OnStuck`.
  - One graph carrying several kinds at once → findings come back in the fixed check order.

  Do not write a test that calls `shedengine.ShedProducer.Call`, and do not add a fake producer type to this package.
- **Commit:** `feat(shedcheck): add Check over an assembled OnDone/OnStuck graph`

### Card 3: package documentation

- **Context:**
  - `internal/shedcheck/check.go`
  - `internal/shedcheck/finding.go`
  - `internal/shedengine/doc.go`
  - `internal/loomshed/doc.go`
- **Edits:** none
- **Creates:**
  - `internal/shedcheck/doc.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Create `internal/shedcheck/doc.go` holding the `package shedcheck` doc comment and nothing else, following the shape of `internal/shedengine/doc.go` (godoc `#` headings, prose stating the contract rather than arguing for it).
  It states:

  - What the package is: an authoring-time structural analysis of an assembled `shedengine` producer graph, importing `internal/shedengine` and the standard library and nothing else.
  - That nothing in production calls `Check` — not `shedengine.Run`, not `loomshed.New` — and that its enforcement point is a `go test` invariant, because reachability-from-entry is not a property `Run` is in a position to assert (a legitimately resumed run starts mid-graph from a persisted `current_producer`).
  - That both endpoints are told and never inferred, and why: `Shed` has no entry field and no terminal field, and defaulting to `Producers[0]` would re-introduce the positional routing meaning `internal/shedengine/doc.go` explicitly disclaims.
  - The eight kinds, one line each.
  - The field-value vs. graph-structure rule and its two pinned consequences.
  - The deterministic output order, and that a clean graph returns `nil`.
  - The tolerated-malformed-input behaviours, explicitly including this one: a duplicate `Name` resolves to the first occurrence, so the shadowed later row is reported as `unreachable` rather than as a duplicate, and `Check` emits no duplicate-flavoured kind at all — `shedengine.validate()` already rejects a duplicate `Name` with its own distinct message.
    Say this outright so a reader who hits the misleading diagnosis is not left guessing.
  - The one perch mis-wiring `Check` cannot catch: a `Burler` handing back via `OnDone` instead of `OnStuck`.
    Both wirings make the gate reachable from the round producer, so the routing graph is identical under either;
    the difference is which verdict the producer returns, which is behaviour inside `Call` and not a property of the graph.
  - That `Check` never reads `Segment` and never calls a producer.
- **Commit:** `docs(shedcheck): add package documentation`

## Batch Tests

`verify: go test ./internal/shedcheck/...` runs the two new test files, `internal/shedcheck/finding_test.go` and `internal/shedcheck/check_test.go`, which are the only tests in the package.
The scope is exactly this batch's `Creates:` set — the package is new, so no existing test anywhere else can be affected by it, and nothing outside `internal/shedcheck` imports it yet.

The overview's module-wide `verify: go vet ./...` runs afterwards at the batch boundary and is what catches a compile-level mistake leaking beyond the package.

Both new test files are untagged and spawn nothing — no `exec.Command`, no git, no `t.TempDir` — so the Test Tier Purity Invariant and the Hermetic Git Test Environment Invariant both stay satisfied with no `TestMain`.
