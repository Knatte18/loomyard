# Batch: vocabulary enforcement test

```yaml
task: 'fabric: close the weft-visibility leak (slice 8)'
batch: 'vocabulary enforcement test'
number: 7
cards: 2
verify: go test ./internal/lyxcwd/
depends-on: [4, 5, 6]
```

## Batch Scope

Adds the machine check that stops the leak from reopening: `TestEnforcement_FabricVocabulary` in `internal/lyxcwd/enforcement_test.go`, per decision `enforcement-test`.
Runs last among the code batches — every vocabulary cleanup (batches 02-06) must be landed so the test goes green on first activation.
Also extracts the shared walk helper the file's three enforcement tests will use.
Placing the test in `lyxcwd`'s file is a convenience (it reuses the walk helper), not an ownership claim — batch 08 records that in `CONSTRAINTS.md`.

## Cards

### Card 25: extract the shared walk helper

- **Context:**
  - `_mill/discussion.md`
- **Edits:**
  - `internal/lyxcwd/enforcement_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Extract a single `filepath.WalkDir`-based helper from the duplicated walk logic in the existing `TestEnforcement` and `TestEnforcement_GeometryLiterals`, shaped so card 26's new test can reuse it (parameterise on: roots to walk, file-suffix filter, per-file callback).
  Pure refactor — both existing tests keep passing with identical semantics (same skip rules, same file set).
  Untagged file: the helper must not introduce any spawn (Test Tier Purity Invariant).
- **Commit:** `test(lyxcwd): extract shared enforcement walk helper`

### Card 26: `TestEnforcement_FabricVocabulary`

- **Context:**
  - `internal/configsync/configsync.go`
  - `internal/weftname/weftname.go`
  - `CONSTRAINTS.md`
  - `_mill/discussion.md`
- **Edits:**
  - `internal/lyxcwd/enforcement_test.go`
  - `internal/buildercli/sync.go`
  - `internal/webstercli/sync.go`
  - `internal/gitrepo/doc.go`
  - `internal/websterengine/audit.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Per decisions `enforcement-test` and `fabric-vocabulary-rule`, add `TestEnforcement_FabricVocabulary` with three machine-checked rules:
  (1) any production `.go` file under `internal/` and `cmd/` outside the owner set containing the bare token `weft` or `warp` — in identifiers, string literals, or comments — fails;
  (2) same files containing a fabric-sense `host` phrase fail, where the predicate is phrase-based, never whole-word: `host repo`, `host repository`, `host worktree`, `host working tree`, `host checkout`, `host branch`, `host junction`, `host path`, `host side`, `host HEAD` (any case, hyphenated or spaced), plus `host` as a component of a fabric-geometry identifier (`hostBranch`, `hostLayoutFor`, `hostReason`, `HostJunction`, `hostClean`);
  (3) any file outside `{fabricengine, fabriccli, lyxtest}` importing `internal/weftname` fails.
  Coverage additionally includes a plain `internal/**/*.md` walk (NOT a parse of `//go:embed` directives — a plain walk fails safe for future non-embedded templates).
  `*_test.go` files are excluded from the machine check, and that exclusion applies to **all three rules, rule (3) included** — this is load-bearing, not editorial: `internal/lyxcwd/geometry_test.go:13` legitimately imports `internal/weftname` (it tests `weftname.SiblingPath`), so a rule (3) that walked test files would fail on first activation.
  Owner set expressed as a map in the same idiom as the file's existing `geometryTokenOwners`: `fabricengine`, `fabriccli`, `weftname`, `lyxtest`, `boardengine`, and `configsync` documented as string-literal-and-comment (a `configsync` **identifier** hit still fails;
  its literals and the comments documenting them are carved out — see the overview's `vocabulary owner set and carve-outs` decision for why the machine rule is coarser than the review rule here).
  `tools/` and `sandbox/` are outside the walked roots by construction — no per-file exception needed.
  Include comments in the scan (deliberate divergence from the sibling `stripGoComments` guard — here the prose is itself the leak).
  Predicate sub-test on synthetic snippets, mirroring the existing `t.Run("predicate", …)` idiom: a non-owner file with `weft` in an identifier fails;
  `warp` in a string literal fails;
  `weft` in a comment fails;
  an embedded-`.md`-style body with `weft` fails;
  an owner-set file with all of the above passes;
  a `configsync`-row file passes on a string literal and on a comment but fails on an identifier;
  `host` cases in both directions — `"the host repo"` and `hostBranch` fail;
  `"cannot host a strand"`, `"a non-Windows test host"`, and `Write-Host` pass.
  Also hand-clean any of this file's own pre-existing prose that the new rules would flag if it were production code, purely for consistency (test files are outside the machine check).
  Rule (2)'s "same files" scoping shares rule (1)'s owner-set exclusion (`fabricVocabularyOwners`), not a separate no-exceptions rule -- `internal/fabricengine`'s own untouched files (`add.go`, `junction.go`, `reconcile.go`, etc., none of which are in this plan's "All Files Touched" list) use `host` pervasively as owner-internal vocabulary with zero production callers outside fabric, so a literal no-owner-exception reading of the discussion's `host` prose would fail the tree-scan against files no batch was ever asked to touch.
  On first activation the tree-scan additionally caught four straggler bare-token comments earlier batches missed inside files they did edit (`internal/buildercli/sync.go`, `internal/webstercli/sync.go`, `internal/gitrepo/doc.go`, `internal/websterengine/audit.go` -- each naming an internal `fabricengine` identifier like `seedWeftArtifactExcludes`/`commitWeftAt`, or a historical CLI spelling, in prose); this card hand-cleans those four straggler lines rather than weakening the predicate, since they are genuine leftover leaks in files this task already edits.
- **Commit:** `test(lyxcwd): add TestEnforcement_FabricVocabulary`

## Batch Tests

`verify:` runs `go test ./internal/lyxcwd/` — the new enforcement test must pass against the fully-swept tree, and its predicate sub-test proves the matcher itself (both fail and pass directions for all three tokens), so it can never silently stop matching and stay green.
