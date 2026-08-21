# Plan: Shed-setup validity checker

```yaml
task: "Shed-setup validity checker"
slug: "shed-setup-validity-checker"
approved: true
started: "20260821-081752"
parent: "main"
root: ""
verify: go vet ./...
```

## Batch Index

_The fenced yaml block below is the authoritative DAG mill-go reads to schedule batches.
Every batch lives at `NN-<batch-slug>.md` in this directory and is mirrored as one entry here._

```yaml
batches:
  - number: 1
    name: shedcheck-package
    file: 01-shedcheck-package.md
    depends-on: []
    verify: go test ./internal/shedcheck/...
  - number: 2
    name: loomshed-invariant-test
    file: 02-loomshed-invariant-test.md
    depends-on: [1]
    verify: go test ./internal/loomshed/... ./internal/shedengine/...
  - number: 3
    name: docs
    file: 03-docs.md
    depends-on: [1, 2]
    verify: go test ./internal/lyxcwd/...
```

## Shared Decisions

### Decision: live rows vs. shadowed duplicate rows

- **Decision:** define a row as **live** when its list index equals the index the first occurrence of its `Name` holds, and **shadowed** otherwise.
  A shadowed row contributes no edges to the graph, and every per-row check skips it — except `unreachable`, which always reports it.
- **Rationale:** `shedengine.findProducer` is a linear first-match scan, so `Run` would never reach or walk a shadowed duplicate.
  `Check`'s model of the graph must be the model `Run` would walk.
  This is the mechanism behind the discussion's stated behaviour: "the shadowed later duplicate is reported as `unreachable`, and any other check that would key on the shadowed row's own fields is skipped for it."
- **Applies to:** shedcheck-package

### Decision: reachability is tracked per list index, never per name

- **Decision:** the reachable set is a set of **list indices**, not a set of names.
  Traversal from a target name `T` moves to the single index `indexByName[T]`.
- **Rationale:** a shadowed duplicate's `Name` is reachable while the shadowed row itself is not, and a name-keyed reachable set cannot express that difference.
  Indexing also makes the `unreachable` rule uniform for the empty-`Name` row (no edge can ever name `""`, since `""` is the terminal/escalate sentinel on both fields), which is why an empty-`Name` row surfaces as `unreachable` with no special case.
- **Applies to:** shedcheck-package

### Decision: `done-cycle` reports cycles of length ≥ 1, not only length ≥ 2

- **Decision:** the cycle walk reports a self-referencing `OnDone` (`P.OnDone == P.Name`) as a `done-cycle` finding with `Producer` and `Target` both naming that row, exactly as it reports longer cycles.
- **Rationale:** the discussion defines the kind as "a cycle among reachable rows using done edges only" and separately notes that `shedengine.validate()` already rejects the length-1 case, "so this catches length ≥ 2".
  That second sentence describes the gap `Check` closes relative to `validate()`, not an exclusion from `Check`'s own definition.
  `Check` is explicitly required to be robust on input `validate()` has never seen and may never see (the same reason `dangling-target` duplicates `validate()`'s own dangling check on purpose), so a self-`OnDone` — a statically certain infinite loop — must degrade into a finding rather than into silence.
  Special-casing it out would mean a graph that never reaches `validate()` gets a *worse* diagnosis than one that does.
- **Applies to:** shedcheck-package

### Decision: no `reflect.DeepEqual` on `Finding`

- **Decision:** every test comparison is field-by-field over `Kind`, `Producer`, and `Target` only, or over a `Message`-free projection.
  `Message` is asserted non-empty and never compared for wording.
- **Rationale:** the discussion pins `Kind`/`Producer`/`Target` as the contract and deliberately leaves `Message` unpinned;
  `reflect.DeepEqual` on the whole struct would drag `Message` back into the contract, so every future clarification of a message would break a test that was never checking behaviour.
- **Applies to:** shedcheck-package, loomshed-invariant-test

### Decision: no map is ever range-iterated for output

- **Decision:** any intermediate map in `check.go` is read by lookup only.
  Every loop that can produce a finding iterates the producer slice (or the caller-supplied `terminals` slice) in index order.
  Where a set of collected cycles must be ordered, it is sorted explicitly by its lowest member index rather than emitted in discovery order.
- **Rationale:** Go map iteration order is randomised, and the whole output-order decision is machine-enforced by full-slice literal assertions.
  A single stray `for k := range m` would make the suite flaky rather than wrong-looking.
  Discovery order is separately insufficient: a walk starting at a row outside a cycle can discover a high-index cycle before a low-index one, so the sort is required, not decorative.
- **Applies to:** shedcheck-package

### Decision: TDD within a card, never a red commit

- **Decision:** cards that pair a test file with the code it exercises list both in the same card, and the card's `Requirements:` states that the test file is written first.
  No card commits a package that does not compile or whose tests do not pass.
- **Rationale:** the discussion names both test suites as TDD candidates, and TDD is a way of working rather than a commit-boundary rule.
  Splitting the test into its own card would commit a non-compiling package referencing a `Check` that does not exist yet, which mill-go's own per-batch `verify:` would then have no way to interpret.
- **Applies to:** all batches

### Decision: `Segment` is never read, and no invariant is added

- **Decision:** no file this task writes reads, sets, or mentions `ProducerDef.Segment` as something to honour.
  `CONSTRAINTS.md` gains no new section, and neither `internal/shedengine/seam_enforcement_test.go`'s allowlist nor `internal/loomshed/seam_enforcement_test.go`'s `loomshedAllowedImports` is touched.
- **Rationale:** `Segment` removal belongs to the Shed-recipe loader items.
  The `shedengine` allowlist already forbids the dangerous import direction (`shedengine` → `shedcheck`), so no new invariant is needed.
  The `loomshed` allowlist governs production imports only — its walk skips `_test.go` files outright at `internal/loomshed/seam_enforcement_test.go:58` — and the new invariant test is a `_test.go` file, so adding `shedcheck` there would assert `shedcheck` is a permitted *production* dependency of `loomshed`, which this task deliberately does not make it.
  State the "no new invariant" conclusion explicitly in the commit body rather than leaving it ambiguous.
- **Applies to:** all batches

### Decision: comments explain the non-obvious in the code, not only in the design doc

- **Decision:** each new `.go` file opens with a file-level comment stating what it implements.
  Two behaviours carry their own in-place explanation: the edge-drop rule (`dangling-target` removes the edge, so a check keying on a field value reads the raw field while a check keying on graph structure reads the post-drop graph) and the functional-graph cycle walk (every row has at most one outgoing done edge, so a visited-marks walk suffices and no SCC algorithm is needed).
- **Rationale:** repo convention (`golang:golang-comments` and the existing `shedengine` files) puts the reasoning where the code lives.
  Both behaviours read as arbitrary without it.
- **Applies to:** shedcheck-package

## All Files Touched

_Full union of every `Creates:` / `Edits:` / `Moves:` **target** path across every batch, sorted alphabetically (Move **source** paths are excluded — they disappear, like `Deletes:` tokens).
Cards are the source of truth;
this section is the input `_plan_validate.py`'s `all-files-touched-mismatch` check cross-references against the derived union of every card's `Edits:`/`Creates:`/Move-target paths, to catch drift between the hand/agent-maintained list here and that derived union._

- `docs/overview.md`
- `internal/loomshed/loomshed_test.go`
- `internal/shedcheck/check.go`
- `internal/shedcheck/check_test.go`
- `internal/shedcheck/doc.go`
- `internal/shedcheck/finding.go`
- `internal/shedcheck/finding_test.go`
- `manifest/designs/shed-recipe.md`
- `manifest/designs/shed.md`
- `manifest/roadmap.md`
