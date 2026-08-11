# Batch: guard-and-docs

```yaml
task: 'fabric: accumulate the result envelope from mutations, not control flow (slice 14)'
batch: 'guard-and-docs'
number: 8
cards: 4
verify: go test ./cmd/lyx/ ./internal/lyxcwd/
depends-on: [7]
```

## Batch Scope

This batch makes the rule survive the next verb: a machine guard in `cmd/lyx/destructiveguard_test.go` pinning that every executor in `destroy.go` records and every mutating result type carries the record, a new **Mutation Record Invariant** in `CONSTRAINTS.md` that names the guard and states its blind spots honestly, and the documentation the task-completion rule requires in the same commit.
Without a guard this rule rots the first time a verb is added — and this is precisely the class of rule a new verb silently skips, because nothing fails when it does.

It runs last because both the guard and the docs describe the finished shape, and writing either against a half-landed implementation would mean writing it twice.

## Cards

### Card 32: the Mutation Record guard

- **Context:**
  - `cmd/lyx/rawgitmutation_test.go`
  - `internal/fabricengine/destroy.go`
  - `internal/fabricengine/mutation.go`
  - `internal/fabricengine/add.go`
  - `internal/fabricengine/status.go`
  - `internal/fabricengine/diff.go`
- **Edits:**
  - `cmd/lyx/destructiveguard_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Add a second test function to `cmd/lyx/destructiveguard_test.go`, `TestMutationRecord_FabricengineProductionSource`, reusing this file's existing machinery wholesale — the `exec.LookPath("go")` clean skip, the `go env GOMOD` module-root resolution, `filepath.WalkDir`, the `_test.go` skip, the `filepath.ToSlash` normalisation before any comparison (Windows is the primary dev OS), and the per-entry-carries-a-reason allowlist convention.

  It asserts two things by raw source inspection:

  1. **Every executor in `internal/fabricengine/destroy.go` records.** Declare the eight executor names (`removePath`, `removeGitWorktree`, `removeLink`, `repointLink`, `deleteBranch`, `createExclusiveDir`, `createGitWorktree`, `resetHardTo`) as a table, and for each assert its declaration line in `destroy.go` carries a `rec *Mutations` parameter. `repointLink` carries an allowlist-style exemption from the *body* check with its reason — it records nothing of its own, passing `rec` straight through to `removeLink`, because there is deliberately no `link_repointed` kind — while still being required to take the parameter.
  2. **Every mutating result type carries the record.** Declare the twelve result-type names (`AddResult`, `RemoveResult`, `CheckoutResult`, `PruneResult`, `CleanupResult`, `UnwireVerbResult`, `UnwireResult`, `ReconcileResult`, `CommitResult`, `PullResult`, `CloneResult`, `PushResult`) with the file each is declared in, and assert each declaration embeds `MutationRecord`. Declare the two read-only result types (`StatusResult`, `DiffResult`) in a companion table asserting they do **not** — the which-verbs scope decision is machine-held, not a convention.

  Carry a vacuous-scan floor for each table the same way `destructiveGuardMinScannedFiles` does for the existing walk, so a table that silently stopped matching fails loudly rather than passing on zero rows.

  State the guard's blind spots in its own file-header comment, as the chokepoint guard already does for its raw-substring matching: it pins the parameter and the embed, not that a recording call is *correct*, and a new `Kind` added without a recording site is caught by nothing here.
  That honesty is required by the invariant text card 33 writes.
- **Commit:** `test(lyx): guard that every executor records and every mutating result carries it`

### Card 33: the Mutation Record Invariant

- **Context:**
  - `cmd/lyx/destructiveguard_test.go`
  - `internal/fabricengine/mutation.go`
  - `internal/fabricengine/destroy.go`
- **Edits:**
  - `CONSTRAINTS.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Add a `## Mutation Record Invariant` section to `CONSTRAINTS.md`, placed immediately after `## Fabric Destruction Chokepoint Invariant`, since it builds directly on that one's total-coverage guarantee.
  Match the file's existing section shape and its semantic-line-break prose style.

  The invariant states:

  - Every destructive executor in `internal/fabricengine/destroy.go` takes a `rec *Mutations` parameter and appends its own primitive to it, **after** the primitive observably changed state — never on a no-op, never on a refusal, never before the act.
  - Every mutating verb's result type embeds `MutationRecord`; the four read-only verbs' result types must not.
  - `internal/fabricengine/mutation.go` is the single declarer of the `Kind` enum. A new member lands in the same commit as its recording site and its guard-test entry, never ahead of either.
  - A `CheckForce` member must never be added to `Check`: force is consulted only inside `checkPathDirtiness`, where it makes the dirtiness check *pass* rather than fail, so a refusal can never be attributed to it.
  - The envelope's key set is fixed: `mutations` is always an array (empty, never `null`) and `partial` always a bool (`false`, never absent) on every mutating verb, success and failure alike;
    `partial` derives from exactly one rule, `error ≠ nil ∧ record non-empty`.
  - Enforced by `cmd/lyx/destructiveguard_test.go`'s `TestMutationRecord_FabricengineProductionSource`, with its blind spots named: it pins the parameter and the embed by raw source inspection, not the correctness of any recording call, and a new `Kind` with no recording site is caught by review, not by the guard.

  Update the existing `## Fabric Destruction Chokepoint Invariant` section with one sentence noting that the recorder is threaded *into* `destroy.go` and must never be worked around by recording at a call site outside it — that is what makes destructive coverage provably total rather than a per-call-site review obligation.
- **Commit:** `docs(constraints): add the Mutation Record Invariant`

### Card 34: module and package docs

- **Context:**
  - `CONSTRAINTS.md`
  - `internal/fabricengine/mutation.go`
  - `internal/fabriccli/envelope.go`
  - `internal/output/output.go`
  - `internal/lyxcwd/docslink_test.go`
- **Edits:**
  - `internal/fabricengine/doc.go`
  - `internal/fabricengine/fabrictest/doc.go`
  - `docs/overview.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  `internal/fabricengine/doc.go`: document the mutation record where the package doc describes result shapes — the vocabulary, the accumulate-as-you-mutate rule, the verb-owned recorder plus populating defer, the gate's auto-recording and its record-only-on-observed-effect rule, and the fixed `mutations`/`partial` envelope key set.
  Say plainly what the record is for: `ok` means "no error was returned" and never meant "nothing happened", and `mutations` plus `partial` are what answer that second question directly.

  `internal/fabricengine/fabrictest/doc.go`: card 31 already updated the sabotage table and the deferred-work note.
  This card adds the oracle's own contract to the package doc — both cross-check directions, the manifest-observable/git-state kind split and why it exists, the coverage rule for worktree-rooted entries, the net-effect fold on the commission direction, and the twice-called `DiffManifest` (permitted roots suppress diff noise only, never the honesty assertion).

  `docs/overview.md`: check whether the module table or execution-stack description needs a change.
  `internal/output` gains a function but no new module, and no module's position in the stack moves — so if the table genuinely does not change, make no edit to it and instead record that finding in the commit body rather than inventing churn.
  Do change whatever prose in that file describes fabric's JSON output shape, if any exists, since the envelope now carries two more always-present keys.
  Every inline link in the file must still resolve, file part and `#anchor` alike — `internal/lyxcwd/docslink_test.go` enforces it and this batch's verify runs it.
- **Commit:** `docs(fabricengine): document the mutation record and the truthfulness oracle`

### Card 35: design-doc corrections and roadmap

- **Context:**
  - `_mill/discussion.md`
  - `internal/boardengine/sync.go`
  - `internal/fabricengine/bolt.go`
  - `internal/lyxcwd/docslink_test.go`
- **Edits:**
  - `manifest/designs/fabric-crucible-followups.md`
  - `manifest/roadmap.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Two corrections to `manifest/designs/fabric-crucible-followups.md`, both in this commit so the design doc and the implementation do not disagree:

  1. **The stale consumer claim.** The "Scope and risk" paragraph says `internal/boardengine` "routes through `CommitWeftAt`/`PushWeftAt`" and is the first consumer to check. Neither function exists any more. `boardengine` uses `fabricengine.Bolt` (`internal/boardengine/sync.go`), an in-process Go API returning `(sha, committed, err)` — no JSON involved, and deliberately out of this slice's scope. Correct the sentence to say so, and record the exploration finding that no programmatic parser of fabric's JSON output exists anywhere in the tree: the sandbox suites drive `lyx fabric` as prose read by an agent, not as parsed JSON, so the JSON-shape risk the doc flags up front is materially lower than it assumed.
  2. **Requirement 2.** The doc states requirement 2 as "`ok` becomes a statement about that record plus the error, not a synonym for 'no error was returned'". This slice deliberately does the opposite for `ok` itself and satisfies the *intent* behind that requirement — that the envelope as a whole stop lying — through `mutations` and `partial` instead. Amend the requirement to say what shipped and why: redefining `ok` in place would silently change the meaning of a field every existing consumer already reads, trading one dishonesty for another. Keep the requirement numbered as it is;
     amend it, do not delete it.

  Keep every inline link in the file resolving, file part and `#anchor` alike (Markdown Link Integrity, enforced by `internal/lyxcwd/docslink_test.go`).

  `manifest/roadmap.md`: slice 14 is a planned roadmap item and this task completes it, so the roadmap does move — mark the slice complete in whatever form the file's existing entries use for a landed slice, and note that slice 15 (`corrindex` two-phase race) is next in the chain.
  Do not restructure the entry or add items;
  the roadmap moves only on completing or adding a planned item.
- **Commit:** `docs(manifest): correct the design doc's stale claims and advance the roadmap`

## Batch Tests

`verify: go test ./cmd/lyx/ ./internal/lyxcwd/` runs the two packages holding this batch's enforcement.
`./cmd/lyx/` covers the new `TestMutationRecord_FabricengineProductionSource` guard alongside the existing chokepoint, raw-git-mutation and tier-purity guards, which must all stay green.
`./internal/lyxcwd/` covers `TestEnforcement_FabricVocabulary` (the `host`-ban and geometry-identifier check, which reads production `.go` under `internal/` and `cmd/` plus `internal/**/*.md`, so every identifier, JSON key and doc comment this task added is in its scope) and `docslink_test.go`'s Markdown Link Integrity check over `manifest/` and `docs/`.
Both are the right scope: this batch adds no runtime behaviour, so no fabric package needs re-running here — and the repo-wide `pipeline.done_gate` (`go test ./... && go test -tags integration ./...`) is what re-proves the whole tree before the task is marked done.
