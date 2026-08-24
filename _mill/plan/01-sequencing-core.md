# Batch: sequencing core

```yaml
task: 'webster: DAG-derived card sequencing'
batch: 'sequencing core'
number: 1
cards: 2
verify: go test ./internal/websterengine/...
depends-on: []
```

## Batch Scope

This batch delivers the whole sequencing mechanism as one new, self-contained file — `internal/websterengine/sequence.go` — plus its Tier 1 unit tests.
Nothing else in the repo calls it yet, so the batch compiles and passes on its own without changing any existing behavior: `SequenceBatches` is an exported function with no in-repo caller until batch 2 wires it in.
The external interface batch 2 consumes is exactly two exported identifiers: the `Cycle` type and the `SequenceBatches` function.

Batch-local decision beyond `## Shared Decisions`: the graph is derived at the **card** level and then lifted to the batch level, rather than derived from batch-level unions directly.
Under today's identity batchifier the two are identical (batch ≡ card), but the card-level derivation is what makes the shared-`Targets` rule's "lower-numbered card wins" direction well-defined once a grouping batchifier ships, and it is what the discussion's own test list is written against.

## Cards

### Card 1: sequence.go — edge derivation, SCC condensation, stable topological order

- **Context:**
  - `_mill/discussion.md`
  - `CONSTRAINTS.md`
  - `internal/batcher/batcher.go`
  - `internal/batcher/identity.go`
  - `internal/planparser/plan.go`
  - `internal/websterengine/runlevel.go`
  - `internal/websterengine/doc.go`
- **Edits:** none
- **Creates:**
  - `internal/websterengine/sequence.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Create `internal/websterengine/sequence.go` in package `websterengine`, carrying a file-level banner comment naming what it implements and why sequencing lives here rather than in `internal/batcher`.
  It imports `container/heap`, `sort`, `strings`, and `github.com/Knatte18/loomyard/internal/batcher` — nothing else, and specifically not `internal/planparser` beyond what reading `batcher.Batch.Cards` already gives it, and no path, config, clock, or cwd dependency of any kind.

  Declare the exported type `Cycle` with one exported field, `Batches []int`, documented as the cycle's member batch numbers in ascending order.
  Give `Cycle` one exported method, `func (c Cycle) Warning() string`, returning the single operator-facing line a caller surfaces for this cycle: it names every member batch number and states that the members were condensed into one execution group kept in declared order, and that mutually-dependent cards are worth checking in the plan.
  `Warning` is the sole renderer of that sentence — `Run` (batch 2) calls it rather than formatting its own string, so the wording lives in exactly one place and is directly unit-testable.
  It never panics on a zero-value `Cycle` or on an empty `Batches` slice.

  Declare the exported function `SequenceBatches(batches []batcher.Batch) ([]batcher.Batch, []Cycle)`.
  It must be pure and deterministic — same input, same output, every call, in every process — and length-preserving: `len(returned) == len(batches)` always, with the returned slice holding exactly the input's batch values reordered, never filtered, merged, or synthesized.
  A nil or empty input returns an empty (or nil) slice and no cycles, never a panic.
  It must never mutate the caller's input slice: build and return a fresh slice.

  Internally, implement it as four steps, each in its own unexported helper so the whole thing stays readable:

  1. **Vertex keys.** Each input batch is one vertex, identified by its index in the input slice.
     A vertex's *sort key* is the pair `(number, index)` where `number` is `batchIdentity(batches[i])`'s first return value (`batchIdentity` already exists in `runlevel.go` — call it, do not re-derive its logic).
     The index half of the key exists so the comparator is total even when two batches carry the same number (a zero-card batch yields number `0`);
     every ordering decision in this file compares this pair, never a bare number and never a map iteration.

  2. **Edge derivation** (unexported helper, e.g. `deriveEdges`), returning an adjacency list `[][]int` where each vertex's successor list is deduplicated and sorted ascending.
     Derive edges by iterating every ordered pair of cards `(a, b)` where `a` belongs to batch index `i`, `b` belongs to batch index `j`, and `i != j` (a pair of cards inside one batch produces no self-loop and is skipped):
     - add edge `i -> j` when `b.Uses` and `a.Targets` share at least one entry, compared by exact string equality;
     - add edge `i -> j` when `a.Targets` and `b.Targets` share at least one entry **and** `a.Number < b.Number` — declared card order settles two writers of the same ref.
     A shared entry between `a.Uses` and `b.Uses` never produces an edge: a read creates no ordering against another read.
     Refs that are empty or whitespace-only after `strings.TrimSpace` are ignored on both sides of every comparison, so a stray empty string never manufactures a false edge.
     Do not classify refs as symbol-shaped or path-shaped: both kinds participate identically, and `internal/planparser`'s own classifier is unexported by decision.

  3. **SCC condensation** (unexported helper, e.g. `stronglyConnected`), an iterative or recursive Tarjan over the adjacency list, returning `[][]int` — one member-index list per component.
     Because each vertex's successor list is sorted and the outer sweep runs over vertex indices in ascending order, the traversal itself is deterministic.

  4. **Condensed topological order** (unexported helper, e.g. `orderComponents`), Kahn's algorithm over the condensed DAG.
     Build the condensed edge set by mapping each vertex to its component and keeping only edges whose endpoints land in different components (deduplicated).
     The ready set is a min-heap (`container/heap`) keyed on the component's own lowest member sort key from step 1.
     Pop the lowest-keyed ready component, emit it, then decrement its successors' in-degrees.

  Emission: for each component in the order step 4 popped it, append that component's member batches sorted by their step-1 sort key ascending.
  This makes an already dependency-correct plan sequence to exactly its declared order, and it makes the result independent of the input slice's own order beyond the index tie-break.

  Cycles: return one `Cycle` per component of size greater than one, in the same order those components were emitted, each carrying its members' batch numbers sorted ascending.
  A fully acyclic input returns an empty (or nil) cycle slice.
  Detecting a cycle is never an error and never changes any exit path — `SequenceBatches` returns no `error` value at all.

  Document on `SequenceBatches` itself, in prose, the three properties callers depend on: purity/determinism, length-preservation, and the no-op property for an already dependency-correct plan.
- **Commit:** `feat(websterengine): derive execution order from card Targets/Uses`

### Card 2: sequence_test.go — Tier 1 coverage for derivation, condensation, ordering, determinism

- **Context:**
  - `_mill/discussion.md`
  - `CONSTRAINTS.md`
  - `internal/batcher/batcher.go`
  - `internal/planparser/plan.go`
  - `internal/websterengine/sequence.go`
  - `internal/websterengine/testmain_test.go`
  - `internal/websterengine/classify_test.go`
- **Edits:** none
- **Creates:**
  - `internal/websterengine/sequence_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Create `internal/websterengine/sequence_test.go` in package `websterengine_test`.
  The package is split roughly in half between internal `package websterengine` tests and external `package websterengine_test` ones, so this is a choice, not a default: the external package is correct here because `SequenceBatches` and `Cycle` are exported and this file tests only the exported surface, and because it matches every sibling file this task touches (`template_test.go`, `beginbatch_test.go`, `recoverbatch_test.go`, `runlevel_test.go`), all of which are external.
  It must be a Tier 1 file per the **Test Tier Purity Invariant**: untagged, no `//go:build` line, no `gitexec.Run`/`gitexec.RunGit`, no `exec.Command`/`exec.CommandContext`, no `gitkit.Copy*`, no `hubforge.NewHub`, no `time.Sleep`, no disk access, no git.
  Every fixture is a hand-built `[]batcher.Batch` literal;
  nothing goes through `planparser.ParsePlan`.
  Add a small local helper that builds a one-card `batcher.Batch` from a number, slug, `Targets` list, and `Uses` list, so the table rows stay readable.

  Cover edge derivation, asserting the resulting order (the derived edges are unexported, so assert them through `SequenceBatches`' observable output):

  - A card whose `Uses` matches an earlier card's `Targets` on a symbol-shaped ref orders producer-before-consumer.
  - The same on a path-shaped ref, proving path refs are not excluded.
  - Two cards sharing a `Targets` entry order lower card number before higher card number.
  - Two cards sharing only a `Uses` entry produce no ordering constraint, so both keep their declared positions.
  - A card whose `Targets`/`Uses` match nothing else keeps its declared position.
  - A ref appearing in one card's own `Targets` and its own `Uses` produces no self-edge and no cycle.
  - A Rename card's `Pairs` endpoints, which `planparser` has already projected into `Targets`, participate as targets — build the fixture with the endpoints already present in `Targets`, exactly as `planparser` hands them over, and assert the ordering they imply.
  - A multi-card batch: a card-level edge between two cards in different batches lifts to a batch-level edge, and a card-level edge between two cards inside the SAME batch produces neither a self-loop nor a spurious cycle.

  Cover SCC condensation and ordering:

  - An acyclic graph orders topologically.
  - A plan already in dependency-correct declared order sequences to **exactly** its declared order, and reports no cycles — the no-op property.
  - A plan where a consumer is declared before its producer sequences with the producer first, and the rest of the plan stays as close to declared order as the constraints allow.
  - A two-card cycle condenses into one component whose members run in declared order, with the remaining batches still correctly ordered around it.
  - A three-card cycle, and two disjoint cycles in one plan.
  - The returned `[]Cycle` names exactly the right member batch numbers, ascending, and a fully acyclic plan returns no cycles.

  Cover the structural guarantees:

  - `len(out) == len(in)` for every fixture above, and the multiset of batch numbers in the output equals the input's — sequencing reorders, never drops or duplicates.
  - Calling `SequenceBatches` twice on the same input returns identical order both times.
  - Calling `SequenceBatches` on a shuffled copy of an input returns the same order as the unshuffled call — shuffle deterministically by reversing the slice, never with a random source.
  - `SequenceBatches` does not mutate its input slice: capture the input's batch numbers before the call and assert them unchanged after.
  - A nil input and an empty input each return zero batches and zero cycles without panicking.

  Cover `Cycle.Warning`:

  - The returned line names every member batch number of the cycle it is called on.
  - A zero-value `Cycle` and a `Cycle` with an empty `Batches` slice each return a string without panicking.
- **Commit:** `test(websterengine): cover DAG edge derivation, SCC condensation, and ordering determinism`

## Batch Tests

`verify: go test ./internal/websterengine/...` runs the whole `internal/websterengine` package suite, which is the only package this batch's two files belong to.
The new `sequence_test.go` is the batch's own coverage;
running the rest of the package alongside it costs little and catches an accidental identifier collision with an existing unexported helper in the same package (for example a name clash against `classify.go`'s or `fingerprint.go`'s own helpers), which a single-file run would miss.
No test in this batch spawns git, tmux, or any external process, so the whole batch stays Tier 1 and `cmd/lyx/tierpurity_test.go` — which batch 2's verify scope reaches — has nothing new to flag.
