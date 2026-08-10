# Batch: guard-and-docs

```yaml
task: 'fabric: one ownership-and-dirtiness gate for all destruction (slice 12)'
batch: 'guard-and-docs'
number: 6
cards: 5
verify: go test ./cmd/lyx/... ./internal/lyxcwd/...
depends-on: [4, 5]
```

## Batch Scope

This batch adds the static bypass guard that turns the slice from a refactor into a closed class, and the four documentation changes CLAUDE.md requires to land with cross-cutting infrastructure: the `CONSTRAINTS.md` invariant, the package doc's rationale, the design doc's slice-12 status and its one stale sentence, and the roadmap entry.

It runs after batches 4 and 5 because the guard cannot pass until every conversion has landed — that is the whole point of committing it last, and the discussion's `one-commit-for-the-slice` decision rejects the alternative ordering precisely because it would leave a commit where the guard passes only because its allowlist is wide open.

It runs in parallel with batch 7, which touches no file in this batch.

Batch-local decision beyond `## Shared Decisions`: the guard's banned-token set is the discussion's final seven tokens plus `createdToken{`, added per the overview's decision that the token's unforgeability is guard-enforced rather than type-enforced.

## Cards

### Card 27: the bypass guard

- **Context:**
  - `cmd/lyx/rawgitmutation_test.go`
  - `_mill/discussion.md`
  - `internal/fabricengine/destroy.go`
  - `internal/fabricengine/doc.go`
  - `internal/fabricengine/warpprobe.go`
  - `internal/fabricengine/gitexclude.go`
  - `internal/fabricengine/index.go`
  - `internal/fabricengine/hook.go`
  - `internal/fabricengine/ancestors.go`
  - `internal/fabricengine/junction.go`
- **Edits:** none
- **Creates:**
  - `cmd/lyx/destructiveguard_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** create the guard by cloning `cmd/lyx/rawgitmutation_test.go`'s machinery wholesale — the module-relative scan-package list, the raw-substring banned-token slice, the per-file allowlist map keyed by module-relative slash-separated path with a reason as its value, the minimum-scanned-files floor, the `exec.LookPath("go")` clean skip, the `go env GOMOD` module-root resolution, the `filepath.WalkDir` skipping test files, and the `filepath.ToSlash` normalisation before any comparison, which matters because Windows is the primary dev OS.
  Scan `internal/fabricengine` only.
  The banned token set is exactly: `RemoveAll(`, `os.Remove(`, `"worktree", "remove"`, `"branch", "-D"`, `warp.ResetHard(`, `weft.ResetHard(`, `fslink.Remove(`, and `createdToken{`.
  Two of these were corrected against a naive first guess in opposite directions and the reasons must be recorded in the file's header comment, because both mistakes are easy to reintroduce.
  `RemoveAll(` rather than `os.RemoveAll(`: the package's removal seam is called bare, so the qualified spelling is not a substring of the call and would miss the two sites the slice most wants policed, one of them the hub teardown.
  The bare form is a superset that also catches the qualified one, and the seam's own declaration carries no trailing paren so it does not self-flag.
  `warp.ResetHard(` and `weft.ResetHard(` rather than `.ResetHard(`: the broad form would flag the *correctly migrated* callers, since the gated reset is reached as a method on the pair handle.
  Banning the raw handles instead targets what is actually forbidden — reaching past the gate to the underlying repo field — and needs no leading dot, so it matches under any receiver name.
  The allowlist is complete and every entry carries its reason: the gate's own file;
  the exclude rewriter, whose removals clean up a temp file the same function created under a repo-wide lock;
  the weft-binding probe, which removes a probe directory the same function created;
  the empty-ancestor sweeper, whose removal is refused by the OS when the directory is non-empty and whose loop halts on the first refusal;
  the correspondence-index cache, deliberately deleted then rebuilt so a failed refresh misses honestly rather than answering cross-branch;
  the junction file, for the one removal of a directory the loop above it just emptied by rename;
  the hook file, for the removal of the user-hook backup that same function wrote ten lines earlier on its own rollback path;
  and the package doc file, because this slice writes destruction rationale into it and a raw substring match over prose would trip the guard from inside the document explaining the rule — its only non-comment line is the package clause, so it can never contain a real call.
  Note in the header that the junction and hook entries are whole-file allowlists, so a *new* raw removal added to either would not be caught, and that this is the same limitation the file being cloned already has.
  Set the vacuous-scan floor to a value comfortably below the package's current production file count and comfortably above zero, and state in a comment that its job is catching a misconfigured walk rather than tracking the package's size.
- **Commit:** `test(cmd/lyx): add the destructive-bypass guard over internal/fabricengine`

### Card 28: allowlist the guard for tier purity

- **Context:**
  - `cmd/lyx/destructiveguard_test.go`
- **Edits:**
  - `cmd/lyx/tierpurity_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** add one entry to the Test Tier Purity Invariant's spawner allowlist for the new guard file, keyed by its module-relative slash-separated path with a one-line reason in the same register as the neighbouring entries.
  The reason is that it resolves its scan root via `go env GOMOD`, which means it contains `exec.Command`, and that it carries banned destructive tokens as its own scan data.
  Place the entry beside the existing raw-git-mutation guard entry, which is the closest precedent and has the same two justifications.
  The guard file is untagged, so without this entry the tier purity test fails on it — verify that the entry is what makes both tests pass together rather than assuming it.
- **Commit:** `test(cmd/lyx): allowlist the destructive guard for tier purity`

### Card 29: the CONSTRAINTS.md invariant

- **Context:**
  - `cmd/lyx/destructiveguard_test.go`
  - `internal/fabricengine/destroy.go`
  - `_mill/discussion.md`
- **Edits:**
  - `CONSTRAINTS.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** add a new top-level section named the Fabric Destruction Chokepoint Invariant, placed among the existing top-level invariant sections rather than as a sub-bullet under the Fabric Git Invariant.
  A sub-bullet would be the wrong parent for three of the five primitives, which are not git operations at all — two of the eight defects this slice closes were plain recursive directory removals — and the Never Force-Add Invariant is a narrower rule that already has its own section, which is the precedent.
  Keep it short and imperative, rules only, matching the file's own preamble.
  No rationale, no incident narrative, no history — those belong in the package doc, which card 30 writes.
  The entry states: the one file in the package that may perform a destructive primitive;
  the banned token set;
  the four checks and their fixed order;
  that force answers dirtiness and never containment or ownership;
  that a gate refusal is never discarded on a best-effort path;
  that every allowlist entry carries a reason;
  and the enforcing test name, in the same bolded "Enforced by" form every other section uses.
  Include exactly one known-blind-spot sentence, in the same honest register the gitrepo Client Boundary Invariant and the Fabric Vocabulary Invariant already use: raw substring matching, so an alternative argument-slice spelling with different spacing, a dynamically built argument slice, and aliasing a raw repo handle to a local all evade the check, and the allowlist is per-file, so a new raw call added inside an allowlisted file is not caught.
  Do not resolve the repo's broader static-analysis-guard consolidation question here;
  note it in one clause at most.
  Rebase onto the parent branch and re-read this file before committing — another in-flight task edits a different section of it, so the merge is expected to be clean but must not be assumed.
- **Commit:** `docs(constraints): add the Fabric Destruction Chokepoint Invariant`

### Card 30: the package doc's destruction rationale

- **Context:**
  - `internal/fabricengine/destroy.go`
  - `internal/fabricengine/dirtiness.go`
  - `CONSTRAINTS.md`
  - `_mill/discussion.md`
- **Edits:**
  - `internal/fabricengine/doc.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** add a section to the package doc carrying the rationale the `CONSTRAINTS.md` entry deliberately omits.
  Cover: why destruction needs a chokepoint at all, stated as the shape rather than as eight anecdotes — a destructive operation acting on a path it does not own, or without checking whether there is work there to lose;
  why the gate executes rather than approves, and that this is what makes a raw removal outside one file mechanically bannable;
  why ownership is a closed enum with no caller-supplied predicate, since every one of the enumerated destructive sites already had the freedom to define its own check and that freedom is what produced the class;
  why dirtiness scope is declared by the caller rather than derived from the primitive, with the prune-versus-remove split as the worked example;
  why the probe lives beside the gate rather than inside it, so the guard's allowlist can honestly mean "the only file that destroys" rather than "the only file that also runs git status";
  and why the two token-carrying ownership kinds exist, including the honest note that their unforgeability rests on the guard's banned token rather than on Go's type system.
  Keep the fabric vocabulary — this is an owner file explaining fabric's own mechanism, so warp and weft are used freely.
  Beware the guard: this file is on its allowlist precisely so prose may name the banned tokens, and card 27's allowlist entry must already be in place before this card's text lands.
  Do not restate the invariant's rules verbatim;
  point at `CONSTRAINTS.md` for the rules and keep this text to the reasoning.
- **Commit:** `docs(fabricengine): record the destruction chokepoint's rationale in doc.go`

### Card 31: the manifest and roadmap

- **Context:**
  - `internal/fabricengine/destroy.go`
  - `internal/fabricengine/dirtiness.go`
  - `_mill/discussion.md`
- **Edits:**
  - `manifest/designs/fabric-crucible-followups.md`
  - `manifest/roadmap.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** two edits.
  In `manifest/designs/fabric-crucible-followups.md`, mark slice 12 landed rather than deleting the section — the Documentation Lifecycle deletes this file only once all four slices land, and this slice folds its own share of the rationale into the package doc now.
  In the same edit, correct the stale sentence in that slice's open questions claiming the dirtiness probe "is deliberately tracked-only today" and that the chokepoint should inherit it rather than silently widen it.
  That over-generalised one verb's comment to the whole package and is contradicted by the verified split — four of the eight probe sites are tracked-only and four are untracked-inclusive.
  Replace it with the decision actually taken: scope is a caller-declared member of a closed sum type, every site keeps its current scope, and normalising to tracked-only would have opened a new data-loss path where git's untracked refusal routes into a directory-removal fallback.
  Resolve the section's other two open questions in place as well, since this slice answers both: the gate lives in the package rather than lower, and enforcement is the existing grep-the-tree pattern with the broader consolidation question noted rather than resolved.
  In `manifest/roadmap.md`, move slice 12 to completed in whatever form that file already uses for a landed item, leaving the slice 13 and slice 14 dependency statements intact.
  Every inline markdown link either file gains or touches must resolve, including its anchor for a markdown target — both files are scan sources for the repo's markdown-link enforcement test, and that test resolves anchors for targets outside the scanned roots too.
- **Commit:** `docs(manifest): mark slice 12 landed and correct its stale probe-scope claim`

## Batch Tests

`verify: go test ./cmd/lyx/... ./internal/lyxcwd/...` covers exactly the two packages this batch can break.

`cmd/lyx` holds the new guard, the tier purity test that must accept it, and the help-tree and registration tests that would catch an accidental CLI change (there is none — this slice adds no command).
`internal/lyxcwd` holds the markdown-link enforcement test that scans the two manifest files card 31 edits, and the fabric-vocabulary enforcement test that scans every production Go file and every markdown file under the internal tree, which reaches the package doc card 30 edits.
Both are cheap, and both are the specific gates this batch's edits can trip.

The fabricengine package is deliberately not re-run here: batch 6 changes no code in it beyond the package doc's comment block, and batches 3, 4 and 5 each already ran both of its tiers.

Sabotage-prove the guard by hand before committing card 27, twice, because one of the two cases is the one a naive token set misses.
Add a qualified raw recursive removal to a scanned non-allowlisted file, confirm the guard fails, revert.
Then add a **bare** removal call — the seam spelling — to a scanned file, confirm the guard fails, revert.
Then point the scan at a path that does not exist and confirm the vacuous-scan floor fires rather than the walk silently passing.
None of these three probes is committed;
they are a manual proof that the guard is not vacuous, and the batch is not done until all three have been run.
