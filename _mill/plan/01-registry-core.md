# Batch: registry-core

```yaml
task: "Centralize glyph ref-shape enumeration"
batch: "registry-core"
number: 1
cards: 2
verify: go test ./internal/planparser/ ./internal/planglyph/
depends-on: []
```

## Batch Scope

This batch creates the kind-policy registry itself — `internal/planparser/shape.go` with the disposition vocabulary, the full ledger of every gate's policy, the fail-closed lookup helper, the canonical `allRefKinds` slice, and the two relocated behavior-dispatch switches — plus the two meta-tests that make the ledger self-enforcing (enum↔slice set-equality sync and per-policy domain completeness) and the documentation that must land with the registry (the new CONSTRAINTS.md invariant and the design-doc sentence).
It is one batch because everything here is the registry's own declaration surface; no dispatch site migrates yet (that is batches 3–4), so every policy is declared-but-unconsumed data and the tree's behavior is byte-identical except for one behavior-identical literal swap in `classify.go`.
External interface for later batches: the `refGate` constants, `lookup`, `allRefKinds`, and the relocated `diskPathForRef`/`refKindName` (same package, same identifiers).

## Cards

### Card 1: Create shape.go with the kind-policy ledger, relocate the two dispatch switches, land the docs

- **Context:**
  - `internal/planparser/normalize.go`
  - `internal/planparser/containment.go`
  - `internal/planparser/handle.go`
  - `internal/planparser/doc.go`
  - `_mill/discussion.md`
- **Edits:**
  - `internal/planparser/classify.go`
  - `internal/planparser/validate.go`
  - `CONSTRAINTS.md`
  - `manifest/designs/quarry-glyph-plan-alphabet.md`
- **Creates:**
  - `internal/planparser/shape.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Create `internal/planparser/shape.go` (tier1-pure: string analysis only, stdlib imports only, no disk access, no `glyph.Parse`) containing:
  1. The `disposition` type: `type disposition int` with `dispKeep disposition = iota + 1`, then `dispSkip`, `dispFinding`.
     Doc comments state the semantics: `dispKeep` — the gate acts on this kind; `dispSkip` — the gate deliberately ignores this kind (the ref passes through untouched); `dispFinding` — the gate raises its own finding for this kind; the unnamed zero value means "undeclared" and the lookup path fails closed on it.
  2. The `refGate` type (`type refGate string`) and one named constant per gate, with a doc comment on each naming the consuming function and file:
     `gateBareSymbolTarget` (consumer `checkBareSymbolTarget`: Symbol=`dispFinding`, Path/Glyph/Handle=`dispSkip`),
     `gateDirectoryTarget` (consumer `checkDirectoryTarget`: Path=`dispKeep`, others=`dispSkip`),
     `gateGlyphMalformed` (consumer `checkGlyphMalformed`: Glyph=`dispKeep`, others=`dispSkip`),
     `gateHandleMalformed` (consumer `checkHandleMalformed`: Handle=`dispKeep`, others=`dispSkip`),
     `gateSyntacticContainment` (consumer `syntacticContainment` in `internal/planparser/containment.go`: Glyph=`dispKeep`, others=`dispSkip`),
     `gateHandleClaims` (consumer `handleClaims`: Handle=`dispKeep`, others=`dispSkip`),
     `gateReferencedHandles` (consumer `referencedHandles`: Handle=`dispKeep`, others=`dispSkip`),
     `gateFileRenamePair` (consumer `isFileRenamePair`, one policy applied to both pair sides: Glyph=`dispKeep`, others=`dispSkip`),
     `gateRenameTo` (consumer `checkRenamePairShape`'s to-side: Handle=`dispKeep`, Path/Symbol/Glyph=`dispFinding`),
     `gateRenameFrom` (consumer `checkRenamePairShape`'s from-side: Glyph=`dispKeep`, Path/Symbol/Handle=`dispFinding`),
     `gateNormalizePath` (consumer `normalizeRefIfPath`: Path=`dispKeep`, others=`dispSkip`),
     `gateCanonicalizePath` (consumer `canonicalizeCard`'s canon closure: Path=`dispKeep`, others=`dispSkip`),
     `gateProsaPathOnly` (consumer `checkProsaSymbolTarget`'s `language: none` branch: Path=`dispKeep`, Symbol/Glyph/Handle=`dispFinding`).
  3. The package-level `ledger` variable: `map[refGate]map[refKind]disposition` registering exactly the thirteen policies above with the dispositions stated per gate.
  4. `allRefKinds`: `var allRefKinds = []refKind{refKindPath, refKindSymbol, refKindGlyph, refKindHandle}`, doc-commented as the canonical kind list the meta-tests key on.
  5. The lookup helper, split for testability into a pure core:
     `func lookupIn(g refGate, policy map[refKind]disposition, ref string) (refKind, disposition)` classifies `ref` via `classifyRef`, reads `policy[k]`, and panics with a message naming the gate and `refKindName(k)` when the disposition is the zero value;
     `func lookup(g refGate, ref string) (refKind, disposition)` delegates to `lookupIn(g, ledger[g], ref)`.
     A missing gate entry in `ledger` yields a nil map and therefore the same fail-closed panic.
  6. `diskPathForRef` and `refKindName`, relocated verbatim from `internal/planparser/validate.go` (delete them there; same package, so all callers compile unchanged).
     Both keep their existing doc comments and behavior exactly — `diskPathForRef`'s `default` arm and `refKindName`'s "an unrecognized shape" fallthrough are the file's two sanctioned switch-form ledger entries, and shape.go's package-region doc comment says so.
  In `internal/planparser/classify.go`, replace the open-coded `"plan:"` literal in `classifyRef`'s rule-1 test with the same-package `HandlePrefix` constant (behavior-identical; `HandlePrefix` is declared in `internal/planparser/handle.go`, which stays read-only in this card).
  In `CONSTRAINTS.md`, add a new `## Ref-Shape Registry Invariant` section (placed with the other planparser-family invariants, after the Planparser Sole-Parser Invariant) stating:
  `internal/planparser` is the sole declarer of ref-shape vocabulary — classification (`classifyRef`/`refKind`) and the `plan:` handle grammar (`HandlePrefix` and the exported handle helpers);
  every ref-shape decision in `internal/planparser` and `internal/planglyph` routes through the kind-policy ledger in `internal/planparser/shape.go` or the exported handle vocabulary;
  named enforcement: the enum↔slice sync meta-test, the ledger completeness meta-test, and the AST-based boundary-enforcement scans with per-scan package-qualified exempt sets (`{internal/planparser/classify.go, internal/planparser/shape.go}` for the `refKind` scan, plus `internal/planparser/handle.go` for the `plan:`-op scan; no planglyph file is exempt).
  In `manifest/designs/quarry-glyph-plan-alphabet.md`, add one sentence to the "The package-ownership seam" section naming the kind-policy registry (`internal/planparser/shape.go`) and the exported handle vocabulary as the mechanism keeping shape dispatch single-sourced across the two packages.
  Do not migrate any dispatch site in this card — the policies are declared data; batches 3–4 flip the consumers.
- **Commit:** `planparser: add shape.go kind-policy registry, relocate dispatch switches, land Ref-Shape Registry Invariant`

### Card 2: Registry meta-tests — enum↔slice sync, ledger completeness, fail-closed lookup

- **Context:**
  - `internal/planparser/shape.go`
  - `internal/planparser/classify.go`
  - `internal/cliwire/bannedecl_enforcement_test.go`
- **Edits:** none
- **Creates:**
  - `internal/planparser/shape_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Create `internal/planparser/shape_test.go` (in-package test, no build tag) with three tests:
  1. `TestRefKindEnumMatchesAllRefKinds` — the enum↔slice sync test.
     Locate `internal/planparser/classify.go` from `runtime.Caller(0)` and parse it with stdlib `go/parser` (the AST idiom of `internal/cliwire/bannedecl_enforcement_test.go`).
     Extract the `refKind`-typed const block's declared identifiers in declaration order.
     Bridge the two vocabularies by `iota` position: identifier at position `i` has value `refKind(i)`, so render each as `refKindName(refKind(i))`.
     Assert set equality, not just cardinality: the multiset of rendered names from the const block equals the multiset `{refKindName(k) for k in allRefKinds}` (build `map[string]int` counts on both sides and compare), and additionally assert both sides contain no duplicate rendering — so a duplicate-plus-omission hand edit of `allRefKinds` fails even at matching length, and a fifth enum constant fails immediately.
  2. `TestLedgerCompleteness` — for every `refGate` in `ledger`, assert the policy's key set equals the members of `allRefKinds` exactly (no missing kind, no extra key), so adding a fifth kind fails every gate's policy until each is re-acknowledged.
  3. `TestLookupFailsClosedOnUndeclaredKind` — call `lookupIn` with a synthetic incomplete policy (a map deliberately missing one kind) and a ref classifying to the missing kind, and assert it panics (via `recover`); also assert `lookup` panics for a `refGate` absent from `ledger`.
     Assert the happy path too: `lookupIn` on a complete policy returns the classified kind and its declared disposition.
- **Commit:** `planparser: add registry meta-tests (enum-slice sync, ledger completeness, fail-closed lookup)`

## Batch Tests

`verify: go test ./internal/planparser/ ./internal/planglyph/` — runs both packages' untagged unit suites: planparser proves the relocations and the literal swap changed no behavior (the whole existing suite), and the new `internal/planparser/shape_test.go` meta-tests prove the ledger is complete and fail-closed from the moment it exists.
planglyph is included because it compiles against planparser and its suite is the cheapest cross-package regression net.
