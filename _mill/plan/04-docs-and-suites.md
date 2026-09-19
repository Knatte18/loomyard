# Batch: docs-and-suites

```yaml
task: 'reed: AddStrand and attach self-heal a cold worktree'
batch: 'docs-and-suites'
number: 4
cards: 2
verify: go test ./internal/lyxcwd/ ./cmd/lyx/
depends-on: [1, 2]
```

## Batch Scope

This batch lands every markdown obligation the change carries: the module docs and the roadmap, the two design documents whose claims the change breaks, and the three sandbox suites that assert the refusal this task deletes.
It is one batch because every card is prose against the same shipped behaviour and none touches Go.

Batch-local decision: the sandbox card treats its three named hits as confirmed rather than candidate.
Two of them name the deleted refusal as their `OK` outcome, so leaving either would make the suite report a defect for correct behaviour.
The card still requires a fresh grep sweep, because a suite added between planning and implementation would otherwise be missed.

## Cards

### Card 9: module docs, roadmap, and the two design documents

- **Context:**
  - `internal/reedengine/lifecycle.go`
  - `internal/reedengine/strand.go`
  - `internal/reedcli/attach.go`
  - `CONSTRAINTS.md`
- **Edits:**
  - `docs/overview.md`
  - `manifest/roadmap.md`
  - `manifest/designs/worktree-lifecycle-shed-producers.md`
  - `manifest/designs/reed-fabric-standalone-api.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  In `manifest/designs/reed-fabric-standalone-api.md`, three sites freeze the engine handle's exported surface and all three must move together, since `EnsureSession` breaks the count.
  The "Public surface" paragraph states the handle carries seventeen exported methods and then enumerates all seventeen by name; the "External contract footprint" paragraph restates the same count; and the "Reasoning" paragraph says one handle type carrying seventeen methods.
  Each becomes eighteen, with `EnsureSession` added to the enumeration.
  The line-count figure in the "Size" paragraph is a measured value rather than a frozen contract — leave it unless this change makes it grossly wrong.
  The same document's paragraph about identifiers a bare grep returns cites `requireSessionLocked` at a line anchor inside `internal/reedcli/attach.go`; batch 2 rewrote that file's header comment, so re-verify the anchor and correct it if it moved, or restate the citation without a line number.

  In `manifest/designs/worktree-lifecycle-shed-producers.md`, the premise bullet stating that up needs no explicit owner because it is ambient, self-healing behaviour inside reed's own entrypoints is written as a design assumption for an unbuilt item.
  Rewrite it in the present tense as shipped behaviour, keeping the reboot-survival argument.
  Update the "Related" section's self-heal bullet the same way, so it records the mechanism as landed rather than pending.
  The document's "Today, without this item built yet" section is about the teardown verb and is unaffected — leave it alone.

  In `manifest/roadmap.md`, move the reed cold-worktree self-heal item out of the Planned section and into the Done section, rewriting its body to describe what shipped: the `ensureSessionLocked` and `EnsureSession` seam, both call sites, and the deliberate choice not to route a warm call through `Up`.
  In the Next Up item for worktree spawn/teardown as Shed producers, the sentence naming this item as a blocking dependency must be updated to record that the dependency has landed.

  In `docs/overview.md`, the reed module entry in the module table describes attach as an interactive-handoff exception whose fallible steps all run pre-flight.
  That stays true and gains a second fallible step; state it.
  Add a sentence to the same entry recording that add and attach boot this worktree's session when none is up, with up semantics — a bare substrate, with no persisted strand relaunched — and that every other reed verb still refuses.
  The webster entry's sentences about standalone booting its own private reed session stay accurate and must not be rewritten.

  Every markdown edit follows the repo's semantic-line-break rule: one sentence per line, no fixed-column hard-wrap, plain newlines rather than trailing double-spaces.
  Table cells and blockquotes stay on one line.
- **Commit:** `docs: record the reed cold-worktree self-heal across overview, roadmap and designs`

### Card 10: sandbox suites

- **Context:**
  - `internal/reedengine/strand.go`
  - `internal/reedcli/attach.go`
  - `tools/sandbox/SANDBOX-BURLER-SUITE.md`
  - `tools/sandbox/SANDBOX-REED-WATCH-SUITE.md`
- **Edits:**
  - `tools/sandbox/SANDBOX-REED-SUITE.md`
  - `tools/sandbox/SANDBOX-WEBSTER-SUITE.md`
  - `tools/sandbox/SANDBOX-SHUTTLE-SUITE.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Before editing, grep every suite file under `tools/sandbox/` for the no-session error text and for the `reed up` verb, and confirm the hit set against the three files listed below.
  A suite added since planning that asserts the deleted refusal is in scope for this card even though it is not named here.
  The planning-time sweep confirmed that every other `reed up` hit describes the up verb itself and is unaffected.

  In `tools/sandbox/SANDBOX-REED-SUITE.md`, scenario M1 names an add invocation failing with the friendly no-session error as its `OK` outcome, and covers two verbs in one Watch line.
  Those two verbs now diverge: remove still refuses, add no longer does.
  Split M1 into two scenarios rather than rewording it — one for the verbs that still refuse, whose `OK` outcome is unchanged, and one for the self-healing add, whose `OK` outcome is that a session comes up and the strand is added.
  Renumber the following scenarios if the suite's numbering requires it, and keep each scenario's existing Goal/Watch/Verdict shape.
  Neither half needs a coverage tag: M1 carries none today, and the suite's reed-module coverage is satisfied independently by the three later scenarios that do carry one, so the split cannot drop coverage the guard was relying on.

  In `tools/sandbox/SANDBOX-WEBSTER-SUITE.md`, prerequisite 5 states that an explicit boot is required before any spawn and that without it the spawn fails loud with the no-session error.
  That is falsified: webster's run verb spawns Master through shuttle, which reaches `AddStrand`, which now boots.
  Rewrite the prerequisite rather than deleting it — an operator may still want to boot explicitly, and the numbered list's later entries reference their own positions.

  In `tools/sandbox/SANDBOX-SHUTTLE-SUITE.md`, scenario S5's recovery step tells the operator to start a second shuttle run with reed's state file still absent, parenthesised as an expected add-strand failure that costs no tokens.
  Both halves invert: that second run now boots a session and spawns a real agent.
  Preserve the step's actual purpose — confirming the first run's directory under the shuttle state directory survives — while replacing its mechanism, and drop or re-scope the no-token claim.
  An operator following the step verbatim must not burn tokens the suite promises they will not.

  Every markdown edit follows the repo's semantic-line-break rule, matching the surrounding suite's existing style.
- **Commit:** `docs(sandbox): re-scope the three suites that assert reed's deleted no-session refusal`

## Batch Tests

`verify: go test ./internal/lyxcwd/ ./cmd/lyx/` runs the two packages that mechanically guard this batch's output.
`internal/lyxcwd/docslink_test.go` enforces CONSTRAINTS.md's Markdown Link Integrity invariant over the manifest and docs trees, which is what card 9's roadmap move and design-document edits can break — a relocated item taking a relative link with it, or an anchor that no longer resolves.
`cmd/lyx/sandbox_coverage_test.go` enforces the Sandbox Suite Coverage invariant over card 10's edits: its check is module-level rather than per-scenario, and the reed module's coverage comes from scenarios M1 does not touch, so the M1 split is expected to leave it green — this run is what confirms that rather than assuming it.
Neither card has a runnable Go surface of its own, so these two guards are the whole automated signal for this batch; the rest is review.
