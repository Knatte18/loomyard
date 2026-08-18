# Batch: preflight doc correction

```yaml
task: websterengine + webstercli told-geometry, and Webster standalone entry
batch: preflight doc correction
number: 3
cards: 1
verify: go test ./internal/preflight/...
depends-on: []
```

## Batch Scope

`internal/preflight`'s shipped prose states that `Wired` is the hub-mode trigger a standalone-capable CLI's pre-run consults, and that the resolved-but-not-wired case "is exactly the one a standalone-capable CLI must answer with standalone mode".
Batch 8 keys mode selection on `HubPresent` alone and therefore contradicts that sentence, so this batch rewrites the prose ahead of it.
No `preflight` code changes: both predicates keep their current behaviour and signatures, and `HubPresent` keeps its stencil-seed-gate role.
This is a considered override of the package author's stated intent, not a doc catching up to code, and the rewrite must say so.

## Cards

### Card 9: Rewrite `preflight`'s two-predicates prose

- **Context:**
  - `internal/preflight/preflight.go`
  - `internal/fabricengine/ready.go`
  - `internal/fabricengine/junctionnames.go`
  - `_mill/discussion.md`
- **Edits:**
  - `internal/preflight/doc.go`
  - `internal/preflight/predicates.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Rewrite the "# Why there are two predicates" section of `internal/preflight/doc.go` so it states the corrected trigger:
  `HubPresent` is what a standalone-capable CLI's pre-run keys mode selection on — hub mode when a hub exists for this location, standalone mode when one does not — **and** it remains the gate `cmd/lyx`'s stencil seed uses.
  `Wired` stays in the package as the per-worktree fabric-readiness question a composing orchestrator asks, and is explicitly **not** the mode trigger.
  The rewrite must carry the reasoning, not just the conclusion:
  `fabricengine.Ready` probes the paired weft sibling of the current worktree rather than the hub, so `Wired` is false at `<hub>/_board`, false in an unpaired sibling, and false in a worktree whose pair was removed — three real, healthy hub situations that run producer verbs today.
  Routing those three to standalone mode would relocate a live hub's state to the per-OS standalone state directory, which is strictly worse than the misclassification it would be trying to avoid.
  A genuinely damaged hub therefore gets hub mode and fails loudly at the point of use, and a plain downloaded git repository still lands in standalone because it has no board-level lyx directory beside it — which was the hazard the two-predicate split was built for.
  Keep the existing paragraph explaining why gating the *stencil seed* on `Wired` would be wrong;
  it is unaffected and still correct.
  Delete or rewrite the final paragraph asserting that "that resolved-but-not-wired case is exactly the one a standalone-capable CLI must answer with standalone mode" — that sentence is what this task overrides, and leaving it would leave the package doc contradicting the shipped CLI.
  Apply the same correction to the two function doc comments in `internal/preflight/predicates.go`:
  `Wired`'s comment must stop calling itself "the hub-mode trigger a standalone-capable CLI consults to choose hub mode over standalone mode", and `HubPresent`'s comment must state that it is both the stencil seed gate and the mode-selection trigger, replacing its own closing sentence that repeats the overridden claim.
  Preserve every other sentence in both comments verbatim, including the never-block-a-command return contract and the no-extra-spawn guarantee.
  Add no code, change no signature, and add no new exported symbol.
- **Commit:** `docs(preflight): key mode selection on HubPresent, not Wired`

## Batch Tests

`verify:` is `go test ./internal/preflight/...`.
This batch is doc-only, so the suite is a compile-and-regression check rather than a new assertion: it confirms the edited files still build and that no existing `preflight` test asserted the removed prose.
The corrected trigger itself is pinned by executable tests in batch 8, whose untagged wiring-function truth-table test covers both `HubPresent` rows including the three healthy-but-unwired locations;
there is nothing to assert here beyond the package still compiling, because no `preflight` behaviour changed.
