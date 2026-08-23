# Batch: landingshed-comment-fixes

```yaml
task: 'landing: parent-fabric resolution chain'
batch: landingshed-comment-fixes
number: 3
cards: 1
verify: go test ./internal/landingshed/...
depends-on: []
```

## Batch Scope

This batch is comment-only, per the plan's `landing-config-loads-in-wire`/scope decisions: `internal/landingshed`'s production logic does not change, only three doc comments in `deps.go` that describe a gap this task closes elsewhere.
It is independent of every other batch — no other batch's code depends on the wording of these comments, and this batch depends on nothing.

No card in this batch has a non-empty `Moves:`.

## Cards

### Card 11: Correct three stale comments in `deps.go`

- **Context:**
  - `internal/loomcli/drive.go`
- **Edits:**
  - `internal/landingshed/deps.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Make three comment-only corrections in `deps.go`, all in this one commit, none touching any code:

  1. The `OpenFabric`/`OpenParentFabric` field doc (deps.go:73-76) currently reads: `` "Nothing in this task fills either closure: the resolution chain (list the worktrees, match the entry whose branch equals the parent branch, resolve that path, open it) belongs to the layer that legitimately resolves geometry, and the next roadmap item builds it. Until then both closures are exercised only by this package's own tests, which fill them directly." `` — this is now false, since batch 4 fills both closures in `internal/loomcli/drive.go`.
     Reword it to state that `internal/loomcli/drive.go` fills both closures via `fabricengine.Open`/`fabricengine.OpenParent`, and that this package's own tests still fill them directly with fakes rather than depending on a real fabric.

  2. The laziness wording the plan's `two-opens-in-drive-rather-than-a-shared-handle` decision names in the same field doc (`` "opening eagerly would fail before the run's own preflight has confirmed anything is wired" ``) stays true and does not need rewording on its own — but re-read it in context after correction 1 above and confirm the surrounding sentence still reads coherently; adjust connecting words only if correction 1's edit leaves a dangling reference.

  3. The `PushBranch` field doc (deps.go:62-65) currently reads: `` "The push verb's own name carries a token this package may not write in any identifier, so the layer that names it is the caller, and this package only calls the closure." `` — the phrase "the layer that names it is the caller" is now imprecise: per the plan's `push-verb-gets-a-neutral-fabric-method` decision, the layer that names the verb is `internal/fabricengine` (`Fabric.PushBranch`), not `internal/loomcli`, which cannot name it either (see the `fabric-vocabulary-owner-confinement` Shared Decision).
     Reword so the sentence says the verb is named inside `internal/fabricengine`'s `Fabric.PushBranch`, never by this package or its caller.
     Do **not** change the sentence's conclusion — `PushBranch` stays an injected closure — only its account of which layer names the verb.
- **Commit:** `landing: correct three stale deps.go comments`

## Batch Tests

`verify: go test ./internal/landingshed/...` — no test file changes in this batch; this run only confirms the comment-only edit left the package compiling and its existing suite green.
