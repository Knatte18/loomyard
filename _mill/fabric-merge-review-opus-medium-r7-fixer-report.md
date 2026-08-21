# fabric merge surface — fixer report (round 7, `opus-medium-r7`)

Job 2 for `_mill/fabric-merge-review-opus-medium-r7.md`.
All 8 findings fixed, one commit each, nothing deferred, nothing pushed.

## Summary

| # | severity | finding | fixed | commit |
|---|---|---|---|---|
| F5 | MEDIUM | `Merge`'s not-synced guard defeated by evaluation order; merged over a genuinely diverged target | yes | `fabric: fix F5` |
| F1 | MEDIUM | no CLI route to stage a weft-side conflict resolution; the documented lifecycle could not complete | yes | `fabric: fix F1` |
| F2 | MEDIUM | round 6's Windows separator fix unguarded against its own deletion on every host lyx builds on | yes | `fabric: fix F2` |
| F4 | MEDIUM | `foreignMergeStatePresent`'s weft half and both probe kinds unproven | yes | `fabric: fix F4` |
| F7 | MEDIUM | `mergeSourceInFlight`'s linked-worktree scan — the common case — unproven | yes | `fabric: fix F7` |
| F3 | LOW | `StageResolved`'s stated reason for `git add -A` false on any modern git | yes | `fabric: fix F3` |
| F6 | LOW | `bothSidesAlreadyUpToDate`'s weft conjunct unproven | yes | `fabric: fix F6` |
| F8 | NIT | weft side's found-ness discarded, so an empty ref could reach `git merge` | yes | `fabric: fix F8` |

Counts: **MEDIUM 5, LOW 2, NIT 1, BLOCKING 0.**
One genuinely new behavioral defect (F5), one shipped operability gap (F1), five proof-quality/doc
gaps, one hardening NIT.

## What changed, per finding

### F5 — behavioral

`syncSideBeforeMerge` (merge.go) now classifies each side against its upstream EXHAUSTIVELY —
equal / behind / ahead / diverged — instead of testing `behind` alone and collapsing everything else
into `return nil`. A post-fetch divergence returns `newMergeGuardError([]string{mergeReasonNotSynced})`,
routed through a new `wrapMergeSyncError` so a guard refusal reaches the caller unwrapped and any
other error keeps its sync-step context. The pre-lock guard is unchanged and stays the fast path.

Why the second decision point is not duplication: the guard stage resolves `@{u}` before anything in
the call has fetched, and `Merge` fetches twice on its way in, so the divergence becomes knowable
inside the same call that just decided it was absent.

Four new integration tests in `merge_target_integration_test.go`, all with `t.Fatal` preconditions
asserting the exact ancestry shape under test:
`TestMerge_UnfetchedDivergedTargetRefuses`, `TestMerge_UnfetchedDivergedWeftRefuses`,
`TestMerge_FetchedDivergedWeftRefuses`, `TestMerge_FetchedBehindTargetIsSyncedNotRefused`.

The weft one earns its teeth from its fixture rather than its assertion: both layers now refuse a
diverged weft, so it pairs the diverged weft with a warp side left BEHIND its upstream and asserts an
empty mutation record and an unsynced warp — which is an assertion about WHICH layer refused.

Docs: `Merge`'s godoc, `syncedToUpstreamReason`'s godoc (now says plainly it is a pre-fetch fast path,
not the whole precondition), and a new doc.go paragraph.

Sabotage-verified after: dropping the refusal, treating `ahead` as diverged, dropping the
behind-passes clause, and dropping the guard's weft half are each caught, each by its own test.
Live re-drive: the diverged target now refuses with `mutations: []` and both HEADs unmoved; a merely
behind target still records `repo_advanced` and merges.

### F1 — operability

New `lyx fabric merge-stage <path>...` in `merge_verbs.go`, joined to the weft-verb family, on the
same `mutations`/`partial` envelope. `merge-in`'s and `merge`'s Long text now give the real
resolve → `merge-stage` → `--continue` sequence. doc.go, `docs/overview.md` (three verb lists) and
the sandbox suite's F18 corrected to match — F18 previously told the operator to `git add` each path,
which is the instruction that cannot be followed for a weft-side conflict.

`argsarity_test.go` gains a third bucket, `variadicArity`, because `merge-stage` is the cobra tree's
first unbounded-arity verb and "one more than it consumes" does not exist for it. It is pinned from
the too-FEW direction instead. Exempting it entirely would have let it inherit `ArbitraryArgs`
unnoticed, which is the defect that test exists for.

Three CLI integration tests. The first asserts with `t.Fatal` that plain `git add` really is refused
for the path under test, so "merge-stage is the only route" is proven rather than asserted.

### F2 — proof quality (the Windows deferred item)

`weftPathVisible` now delegates to `weftPathVisibleWithSeparator(…, separator rune)`, which does the
conversion explicitly. `filepath.ToSlash` is the identity function wherever `os.PathSeparator == '/'`,
so no test on any host this campaign has could distinguish the call from its absence — measured, not
assumed: deleting it left the whole suite green.

Seven new table rows drive the Windows separator directly on this Linux host, so BOTH wrong
implementations now fail here: deleting the conversion fails the `WindowsMultiSegmentAnchor` rows,
and blanket-replacing every backslash fails the `PosixBackslashInName` rows.

One atom stays beyond runtime reach — the `os.PathSeparator` argument at the entry point, which a
hardcoded `'/'` is indistinguishable from on this host — and is pinned by source inspection
(`TestMergePaths_WeftPathVisibleUsesTheOSSeparator`), the posture `cmd/lyx`'s existing guards already
take. Both the test and doc.go state that limitation rather than implying it away.

### F4 / F7 / F6 — proof quality

- **F4**: `TestMergeVerbs_ForeignMergeState_EverySideAndShapeRefuses`, a three-shape × two-side matrix.
  The shapes separate the probe kinds: conflicted + `MERGE_HEAD` (pins neither), `MERGE_HEAD` with an
  empty unmerged set (a foreign merge resolved but not concluded), and unmerged entries with no
  `MERGE_HEAD` from a conflicted `git merge --squash`. Each row asserts its own shape with `t.Fatal`,
  checks all five mutating merge verbs plus `MergeInProgress`, and checks the foreign state is
  byte-identical afterwards. Each of the four probes is now caught by its own row.
- **F7**: `TestMergeCrucible_RemoveRefusesWhenALinkedPairIsConsumingTheSource` runs the merge from a
  LINKED pair and drives `Remove` from the prime's location, asserting both record locations first
  (linked present, prime absent) so it cannot pass on the path it is not covering.
- **F6**: new pure-logic `mergestate_test.go` covering both derived flags across their combinations,
  including the empty-outcome row. Every conjunct and disjunct is now independently load-bearing.

Production behaviour was already correct for all three — F4 and F7 were confirmed live on a real hub
before any edit — so these close proof gaps, not defects.

### F3 / F8 — doc accuracy and hardening

- **F3**: `StageResolved`'s godoc claimed the plain `add --` form "errors on a missing pathspec".
  False since git 2.0; verified directly on git 2.53. The `-A` stays as a version pin (the posture
  `--ff` and `--no-edit` already argue for), and the reason is restated honestly, including that no
  test on a modern git can separate the two forms — with a matching note on the test that reads as
  though it could.
- **F8**: `resolveMergeSources` now appends `mergeReasonSourceNotFound` when the weft pick reports
  `found == false`, gated on `weftManaged` so the unmanaged case keeps its deliberate single-reason
  contract (an unmanaged source still reports only `source branch is not fabric-managed`). That gating
  was added after the ungated version broke `TestMergeIn_NotFabricManaged_NothingMutated`, which is
  the test that pins the contract.

## Verification

Every gate green after every commit, and again at the end:

- `go build ./...`, `go vet ./...`
- `go test -count=5 ./internal/fabricengine/... ./internal/fabriccli/... ./internal/gitrepo/... ./cmd/lyx/...`
- `go test -count=1 ./...` (repo-wide, hermetic)
- `go test -tags integration -count=1 -timeout 30m ./internal/fabricengine/... ./internal/fabriccli/... ./internal/gitrepo/... ./internal/mergeresolve/...`
- `./deploy-dev` re-run after every source change, with each affected live scenario re-driven

Final live sweep on a from-scratch hub: two-sided conflict → `merge-stage` → `--continue` (both
conclude commits carrying exactly two parents); sibling verbs refusing with the fixed message while
mid-merge; `merge --abort` restoring both HEADs exactly with clean worktrees and no `MERGE_HEAD`;
the diverged-target refusal; the behind-target sync.

Every sabotage that was passing at review time is now caught, each by a test that fails for its own
reason: S-d, S-4, S-8, S-10, S-11, S-12, S-13, S-24, S-28b, S-35, S-37.

## Deferred / not fixed

**Nothing.** All 8 findings are fixed and committed. No finding required an operator decision, and
none was large enough to belong in its own task.

Re-evaluated carry-forwards, per the round prompt:

- **Windows execution** — still impossible; no Windows host exists in this campaign. But the answer to
  "can you do more than round 6 did" is yes, and F2 is that: the separator logic is now
  host-independent and fully exercised here, with the single unexercisable atom named explicitly
  rather than left implicit. What remains genuinely un-executed is only that one argument.
- **Round 6's own new mechanisms** — re-sabotaged individually. Every one is caught by its own distinct
  test and fails for the right reason, except the `filepath.ToSlash` conversion (now fixed as F2).
  Round 6's two behavioral fixes are sound.
- **Four `MergeContinue`-stuck states** (first instalment round 2, rows 27/28/30/31) — no adjacent code
  touched; unchanged.
- **The post-record error-return class's 45-row adjudication** — not required this round; not attempted.

## Scratch state

The live hub was built under the session scratch directory, never inside the worktree.
`git status --porcelain` in the worktree is clean of everything but this round's own commits.
