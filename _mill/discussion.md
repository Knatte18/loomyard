# Discussion: Centralize glyph ref-shape enumeration

```yaml
task: Centralize glyph ref-shape enumeration
slug: centralize-glyph-shape-enum
status: discussing
parent: main
```

## Problem

The `crucible-loom-glyph-hardening` campaign (10 rounds, 5 models, now merged — commit `e9654f746`) traced roughly two dozen of its most significant findings to one root shape:
an enumerated ref shape (bare glyph, `plan:` handle, self glyph, path-shaped ref, symbol handle) is handled correctly at most call sites in `internal/planparser` and `internal/planglyph` and silently missed at one.
`classifyRef` (`internal/planparser/classify.go`) is already the sole *classifier* with four kinds (`refKindPath`/`refKindSymbol`/`refKindGlyph`/`refKindHandle`),
but its ~14 in-package consumers each hand-pick a kind subset (switches, single-kind filters, negation chains, boolean gates),
and `internal/planglyph` cannot see `refKind` at all (unexported), so it open-codes `strings.HasPrefix(x, planparser.HandlePrefix)`-style shape logic at ~10 sites.
Nothing fails today if a fifth kind is added: no exhaustiveness meta-test exists anywhere over `refKind`.
Every crucible fix was a local patch to the one site that round happened to find;
this task replaces the N independent enumerations with a single registry every call site consults, plus enforcement that a new shape variant cannot be added without every consumer being forced to re-acknowledge its handling.

Why now: the campaign is closed and merged, round 9/10's narrowed scope confirmed the pattern is structural (not review scope-creep), and the precondition ("start only after that campaign's branch is merged") is satisfied.

## Scope

**In:**

- A kind-policy registry inside `internal/planparser` (ledger of per-site `map[refKind]disposition` policies + completeness meta-test + grep-shaped enforcement tests).
- Migration of every `refKind` dispatch site in `internal/planparser` onto the registry (full site list under Technical context).
- An exported handle-vocabulary surface on `internal/planparser` (`IsHandleRef`, `HandleBody`, `NewHandle`, `HandleMember`, `HandleIdentifier`, joining existing `HandlePrefix`/`HandleUnit`), and migration of every open-coded handle string op in `internal/planglyph` onto it.
- Export of two duplicated seams: `Plan.GlyphLanguage()` (replaces planglyph's `resolveLanguage` duplicate of planparser's `planLanguage`) and `Card.ID()` (replaces planglyph's `cardIDOf` duplicate of planparser's `cardID`).
- Family 1 hardening (`quarry.ResolveResult.Status`): fix the one confirmed fail-open consumer (`internal/planglyph/containment.go:112`), add a grep tripwire test over `.Status` consumers, document two intentional asymmetries in place.
- Family 2 hardening (batched-answer coverage): add the missing per-key guard in `internal/planglyph/drift.go`'s post-repair path, add a grep chokepoint test pinning `repo.Resolve` to `resolveTargets` and `quarry.Name` to `CanonicalizeHandles`.
- New CONSTRAINTS.md invariant ("Ref-Shape Registry Invariant") and a registry sentence in `manifest/designs/quarry-glyph-plan-alphabet.md`'s package-ownership section, same commit as the code they describe.
- Deletion of the dead `isGlyphRef` wrapper; `isHandleRef` is superseded by the exported `IsHandleRef`.

**Out:**

- Quarry's `glyph` package (separate repo `github.com/Knatte18/quarry`, pinned `v0.1.0`) — explicitly ruled out as a home for any of this by the task proposal; no changes there.
- Exporting `RefKind`/`ClassifyRef` — planglyph deliberately never 4-way-classifies (see Decision: parse-success-glyph-test-kept); the enum and classifier stay unexported.
- Any change to `classifyRef`'s five spec-pinned rules or any observable classification/validation behavior, except the two named hardenings (see Decision: behavior-preservation).
- `renameSignature`'s Go-declaration shape logic (`internal/planglyph/handle.go:88-97`) — declaration grammar, not ref shape; stays as is.
- A registry for the `.Status` or batch-coverage families — both are confirmed near-consolidated; they get tripwires, not registries.
- Shape logic outside the two packages (websterengine's DAG edge derivation, batcher, quarrycli) — those consume `planparser.Plan` fields, not ref shapes.
- `manifest/roadmap.md` — moves only if this task is listed there as a Planned item; not otherwise touched.

## Decisions

### registry-home-and-exported-surface

- Decision: `internal/planparser` owns the registry.
  The exported cross-package surface is the handle vocabulary only: `IsHandleRef(string) bool`, `HandleBody(string) (string, bool)` (the `plan:`-strip that `resolveKeyFor`/`BindHandles` open-code today), `NewHandle(glyphID string) string` (the `HandlePrefix + id` construction), `HandleMember(string) (string, bool)` (planglyph's `draftHandleMember` moves here), `HandleIdentifier(string) (string, bool)` (planglyph's `draftHandleIdentifier` moves here), beside the existing `HandlePrefix` and `HandleUnit`.
- Rationale: planparser is already the spec-pinned sole classifier (`contracts/specs/loom-plan-spec.md` § "The shape classifier") and the sole declarer of `HandlePrefix`;
  `HandlePrefix`/`HandleUnit` set the export precedent;
  planglyph's only classification need is handle-vs-not (its glyph test is parse-success by design), so exporting the full enum would be API without a consumer.
- Rejected: a new shared `internal/refshape` package (splits the classifier out of the Sole-Parser package, churn without benefit);
  exporting `RefKind`/`ClassifyRef`/`AllRefKinds` (invites shape work outside the two packages, no consumer for the 4-way form).

### kind-policy-ledger

- Decision: every planparser site that branches on `refKind` declares its handling as data — a per-site policy mapping every kind to a disposition — registered in one package-level ledger in the registry file.
  Filter-shaped sites (keep-one-kind, keep-subset) consult their policy through a lookup helper;
  behavior-dispatch sites that need per-kind *code* (`diskPathForRef`, `refKindName`) stay as switches but must name every kind or carry a fail-closed `default` arm, and are listed in the ledger too.
  Two meta-tests enforce it:
  (1) a completeness test asserting every registered policy's domain equals the full kind list — adding a fifth kind fails every site's policy until each is re-acknowledged;
  (2) a grep-shaped enforcement test (precedent: `internal/cliwire`'s bannedecl/callerset enforcement tests, `cmd/lyx/tierpurity_test.go`) asserting no `classifyRef`/`refKind` comparison and no open-coded `plan:` string op (`HasPrefix`/`TrimPrefix`/concat against `HandlePrefix`) exists outside the registry file and the exported handle helpers, in either package.
- Rationale: this is the task's requirement 4 made concrete — a declared-but-unhandled variant becomes a test failure at every consumer, not a silent skip at one.
  Grep/AST-lite enforcement tests are established repo idiom; a `go/analysis` dependency is not.
- Rejected: an `exhaustive`-lint via `go/analysis` (new dependency, no repo precedent);
  handler-struct dispatch (`dispatchRef(ref, handlers{...})` — reshapes 14 sites, runtime indirection, completeness only checkable at runtime anyway).

### no-unknown-zero-value

- Decision: no `refKindUnknown` constant; the enum keeps `refKindPath = iota`.
  Fail-closed against out-of-vocabulary values is enforced two ways instead:
  behavior-dispatch switches must carry a fail-closed `default` arm (part of the enforcement test above),
  and the disposition type's zero value means "undeclared → fail closed" in the policy-lookup helper, so `policy[k]` on a kind outside the declared domain can never silently mean "keep" or "path".
- Rationale: `refKind` exists only as `classifyRef`'s return value consumed inline — no struct stores one, and the type stays unexported, so the "forgotten zero-valued field means path" scenario has no code to occur in.
  An Unknown member would force every policy to declare a disposition for a value that cannot arise, or be excluded from the kind list and protect nothing.
- Rejected: `refKindUnknown = 0` (defense against a future struct-stored `refKind`; bought at the cost of ledger noise for a scenario that does not exist).

### planglyph-dedup-seams

- Decision: beyond the handle vocabulary, export `Plan.GlyphLanguage() (string, bool)` (method form of planparser's unexported `planLanguage`) and `Card.ID() string` (method form of unexported `cardID`);
  planglyph's verbatim duplicates (`resolveLanguage` at `planglyph.go:251-258`, `cardIDOf` at `resolve.go:16-21`) are deleted, call sites migrated.
- Rationale: both duplicates carry comments admitting they mirror planparser's unexported originals;
  same visibility pathology as `refKind`, tiny diff, and `planLanguage` is the master gate on all shape logic — exactly the kind of thing that must not drift between the packages.
- Rejected: handle-ops-only scope (leaves two documented duplicates for the next review round to flag).

### parse-success-glyph-test-kept

- Decision: `collectGlyphTargets` (`planglyph.go:285-290`) and `resolveContainment` (`planglyph/containment.go:95-104`) keep parse-success (`glyph.Parse` returning nil error) as their glyph test;
  only their handle-exclusion guards reroute through the exported `IsHandleRef`.
  The known divergence — a malformed `#`-carrying ref is `refKindGlyph` to planparser but silently dropped by planglyph — is documented at both sites as intentional and covered: `checkGlyphMalformed` (`validate.go:520`) already raises the blocking finding for exactly that ref.
- Rationale: planglyph's test is resolve-oriented by design (its own comment at `planglyph.go:277-281` justifies "no shape classifier of planglyph's own");
  switching to shape classification would change behavior for malformed refs and would require exporting the classifier this design keeps internal.
- Rejected: routing both sites through an exported `ClassifyRef` (behavior change, contradicts registry-home-and-exported-surface).

### status-family-disposition

- Decision: no registry for `quarry.ResolveResult.Status` consumers.
  Three actions instead:
  (1) fix the one confirmed fail-open site, `resolveContainment`'s bare 2-of-4 allow-list at `planglyph/containment.go:112` (an unknown/absent status silently drops a member from the containment index, so an overlap goes unreported) — give it the same vocabulary-guard-then-derive shape `doneCheckVerdicts` uses (`donecheck.go:162-181`), with a regression test;
  (2) add a grep tripwire test asserting every `.Status` switch/comparison in `internal/planglyph` sits in an allowlisted, fail-closed form (switch-with-default, or a boolean derived after a vocabulary guard), so a *future* unguarded consumer is caught;
  (3) document two intentional asymmetries in place: `renameDeclSource`'s Found-only rule (`handle.go:119` — accepting `Multipart` would arbitrarily derive from `Symbols[0]`), and `createFindings`' routing of `Ambiguous` to the `default`/`glyph-rejected` arm (`create.go:151-171`).
- Rationale: round 10's claim that `doneCheckVerdicts` was "the last unswitched `Status` consumer" is disproven by inventory (containment.go:112 remains), so one real fix is owed;
  but 5 of 7 consumers already fail closed — the family is near-consolidated and needs a tripwire, not a table, exactly as the task proposal steers.
- Rejected: full registry treatment (proposal explicitly warns against it);
  doing nothing (leaves a live fail-open hole).

### batch-coverage-disposition

- Decision: no new chokepoint.
  Two actions:
  (1) add the missing per-key guard in `drift.go`'s post-repair path (`drift.go:208-227`): after resolving `collectGlyphTargets(reloaded, lang)`, assert every glyph in the `introduced` set actually received an answer before filtering, erroring `ErrQuarryUnavailable` on a miss, mirroring `DoneChecks`' per-key guard (`donecheck.go:139-149`);
  (2) add a grep chokepoint test pinning `repo.Resolve(` to `resolveTargets` (`repo.go:97`) and `quarry.Name(` to `CanonicalizeHandles` (`handle.go:224`), so the existing length-guard chokepoints (`ensureResolveCoverage`, the Name length+echo guard) cannot be bypassed by a future call site.
- Rationale: inventory confirms round 10's consolidation claim holds for this family — every batch boundary is guarded except drift's post-repair site, which is unreachable-in-practice only because of the length guard it does not itself own.
- Rejected: doing nothing (leaves the drift soft spot resting on a guard it cannot see);
  a coverage registry (nothing left to register).

### behavior-preservation

- Decision: pure refactor except two named hardenings.
  Every existing test in both packages — including every crucible regression test (R1/R3/R8/R9/R10 fixes) — passes unchanged.
  The only observable behavior changes are the status-family fail-open fix and the drift per-key guard, each landing with its own new regression test.
  Any further behavior defect discovered mid-migration is recorded as a finding for a follow-up task, not fixed opportunistically.
- Rationale: the crucible regression suite is the proof that no shape handling silently changed under the registry — that proof only works if the suite is untouched.
- Rejected: opportunistic fixing (scope creep, and it would blur what the regression suite proves).

### constraints-and-docs

- Decision: CONSTRAINTS.md gains a "Ref-Shape Registry Invariant" in the same commit as the registry:
  `internal/planparser` is the sole declarer of ref-shape vocabulary (classification and `plan:` handle grammar);
  every ref-shape decision in `planparser`/`planglyph` routes through the registry's policies or the exported handle helpers;
  named enforcement: the ledger completeness meta-test and the grep enforcement tests.
  `manifest/designs/quarry-glyph-plan-alphabet.md`'s "The package-ownership seam" section gains a sentence naming the registry and the exported handle vocabulary.
  `contracts/specs/loom-plan-spec.md` is untouched (classification behavior is identical).
  `docs/overview.md` is untouched (no new module, no CLI/execution-stack change).
- Rationale: repo's Documentation Lifecycle rule (docs land in the same commit); the task proposal's "Related" section names both files.
- Rejected: CONSTRAINTS.md only (drops the design-doc reference the proposal asks for).

### dead-wrappers

- Decision: `isHandleRef` is superseded by the exported `IsHandleRef` (its in-package callers migrate);
  `isGlyphRef` is deleted (zero production callers; its test coverage folds into the registry tests);
  `isPathRef` stays unexported (three in-package production callers, no external consumer).
- Rationale: keep-for-symmetry is dead API; the wrappers' stated purpose (guarding `classifyRef`'s return-value shape) is subsumed by the registry meta-tests.
- Rejected: keeping both for symmetry.

## Technical context

### The classifier today

`internal/planparser/classify.go` — `refKind` (int, unexported, `refKindPath = iota`, then Symbol/Glyph/Handle), `classifyRef` (five ordered rules, spec-pinned in `contracts/specs/loom-plan-spec.md` § "The shape classifier"), wrappers `isPathRef`/`isGlyphRef`/`isHandleRef`.
The file is a tier1-pure leaf (string analysis only, never stats disk, never calls `glyph.Parse`).
`isGlyphRef`/`isHandleRef` have zero production callers (test-only).

### planparser migration sites (Inventory A)

True switches (stay switches, ledger-listed, default-arm-required):

- `validate.go:360-374` `diskPathForRef` — the sole ref→disk-path mapper (Path→itself; Glyph→language gate + `parseGlyph` + `IsSelf` + `UnitPath`; Symbol/Handle→default not-ok). Feeds `checkCardPathMalformed`, `checkPathMissing`, `createTargetsUnion`, `renameTargetsUnion`.
- `validate.go:828-843` `refKindName` — exhaustive kind→prose switch with deliberate non-panicking fallthrough ("an unrecognized shape"). The registry nucleus; promote to the registry file.

Single-kind filters (migrate to policy lookups):

- `validate.go:426` `checkBareSymbolTarget` (keeps Symbol), `validate.go:467` `checkDirectoryTarget` (keeps Path), `validate.go:520` `checkGlyphMalformed` (keeps Glyph), `validate.go:696` `checkHandleMalformed` (keeps Handle), `containment.go:53` `syntacticContainment` (keeps Glyph), `handle.go:104` `handleClaims` (keeps Handle), `handle.go:146` `referencedHandles` (keeps Handle).

Multi-kind / negation / gate forms (migrate to policy lookups):

- `validate.go:733` `isFileRenamePair` — both pair sides must be Glyph (2-of-4 conjunction) then both `IsSelf()`.
- `validate.go:768-822` `checkRenamePairShape` — three negation arms (`!= refKindHandle` → `rename-to-not-handle`; `!= refKindGlyph` → `rename-from-not-glyph`; the self-glyph-old/handle-new third arm). Its doc comment (755-766) records the R9-2 leak that motivated negation form — the strongest in-code rationale for this task.
- `normalize.go:112` `normalizeRefIfPath` (`!isPathRef` early return — the single `root:`-join gate; its comment calls it "the single sharpest regression this migration can introduce": treat as the migration's most carefully-tested site).
- `normalize.go:178` `canonicalizeCard`'s canon closure (`!isPathRef || !canonicalizablePath`).
- `validate.go:1040` `checkProsaSymbolTarget` — language-forked vocabulary: glyph-enabled uses `parseGlyph(...).IsSelf()`, `language: none` uses `isPathRef`.
- `rewrite.go:162` — open-coded `strings.HasPrefix(oldRef, HandlePrefix)` twice (Create-bullet collapse decision); route through the registry/helper.

Shape-adjacent helpers that co-vary with the classifier but are not keyed on `refKind` (keep, colocate or cross-reference from the registry file): `hasFileExtension` (`normalize.go:127`), `canonicalizablePath` (`normalize.go:159`, encodes rule 4's downstream half), `hasWorktreeRootEscape` (`normalize.go:58`, the `//` prefix — a surface-syntax marker, not a fifth kind).

### planglyph migration sites (Inventory B)

Handle-op sites (migrate to exported helpers):

- `donecheck.go:35-40` `resolveKeyFor` — the de-facto shared helper (7 callers across 4 files); becomes a thin wrapper over `HandleBody` or is replaced by it.
- `create.go:71` (`HasPrefix` filter, duplicating `resolveKeyFor`'s predicate two lines above its own use), `handle.go:203` `CanonicalizeHandles` and `handle.go:335` `cardOwnHandles` (Rename-New handle tests), `handle.go:402` `BindHandles` (open-coded `TrimPrefix`), `handle.go:256` (`HandlePrefix + res.ID` construction → `NewHandle`), `planglyph.go:285-290` `collectGlyphTargets` (handle-exclusion guard only; parse-success glyph test stays).
- `handle.go:32-40` `draftHandleMember` and `handle.go:49-60` `draftHandleIdentifier` move to planparser's handle grammar file as `HandleMember`/`HandleIdentifier` (planparser's `handle.go` doc already claims handle-grammar ownership; today the one split is implemented twice — left half in planparser's `handleUnit`, right half here).

Duplicate seams (delete, migrate to exported methods): `planglyph.go:251-258` `resolveLanguage` → `Plan.GlyphLanguage()`; `resolve.go:16-21` `cardIDOf` → `Card.ID()`.

Not migrated: `containment.go:95-104` `resolveContainment`'s parse-success glyph test (see Decision: parse-success-glyph-test-kept); `handle.go:88-97` `renameSignature` (declaration shape, out of scope).
There is no string-level self-glyph re-derivation anywhere in planglyph (`Glyph.IsSelf()` everywhere) — the Glyph Conversion Chokepoint is holding on that axis; the registry must not add one.

### Status consumers (Inventory C — family 1)

Fail-closed already: `resolve.go:88-129` `statusFindings` (switch-with-default, R9-6), `create.go:151-171` `createFindings` (switch-with-default; `Ambiguous` deliberately lumped to default), `donecheck.go:162-181` `doneCheckVerdicts` (vocabulary allow-list stage then derived booleans, crucible R10 F1), `handle.go:119` `renameDeclSource` (Found-only), plus renderer `resolve.go:139-150` `unreadableStatusDetail`.
The one fail-open site: `containment.go:112` (bare `Found/Multipart` allow-list, no vocabulary guard, silent `continue`).

### Batch boundaries (Inventory D — family 2)

Chokepoint: `repo.go:97` `resolveTargets` with `ensureResolveCoverage` length guard (`repo.go:112-118`) — the package's only `Resolve` call.
Per-key guards additionally at `create.go:93` `matchHandleResults` (R10 F3) and `donecheck.go:139-149` (R5-6).
`handle.go:224` `quarry.Name` has a length guard plus per-result echo check (`handle.go:230-236`).
The soft spot: `drift.go:208-227` post-repair resolve filters to the `introduced` set with no per-key guard of its own.
`delta.go:36` `DeltaGit` and the `repo.go:124/135/159` single-target passthroughs have no batch contract — out of scope.

### Registry precedents to mimic

`refKindName` (`validate.go:828`) as the nucleus; `typeLabels` (`parse.go:363-371`) as the repo's closest table-shaped registry for a sibling closed vocabulary (`CardType`); `unreadableStatusDetail` as the shared-renderer precedent; grep/AST-lite enforcement tests in `internal/cliwire` (`bannedecl_enforcement_test.go`, `callerset_enforcement_test.go`) and `cmd/lyx/tierpurity_test.go` as the enforcement-test idiom.

### Crucible provenance

The regression tests that must keep passing came from: R1 F2/F11/F14, R3 F3/F6, R8 PG-1/PG-2, R9 R9-1..R9-5, R10 F4/F5 (ref-shape family); R9-6/R10 F1 (status family); R5-6/R10 F2/F3 (coverage family).
Full reports live in the (torn-down-on-merge) crucible worktree's `_mill/`; the surviving record is the merged tests themselves and the comments citing round IDs (e.g. `classify.go:59`, `handle.go:95`, `donecheck.go:155-161`).

## Constraints

From CONSTRAINTS.md, binding on this task:

- **Planparser Sole-Parser Invariant** — the registry lives inside the sole parser; no new package reads the plan tree.
- **Glyph Conversion Chokepoint Invariant** — the registry performs no glyph↔path conversion and adds no `#`-string operation over glyph-typed values; handle string ops are loomyard's own `plan:` token work, which the existing code (`planparser/handle.go:38`, `planglyph/handle.go:33`) already documents as outside the chokepoint.
- **Test Tier Purity Invariant** — `classify.go` and the registry file stay tier1-pure leaves (string analysis only); the grep enforcement tests are untagged and read source files only.
- **Told-Geometry Invariant** — both packages remain bound packages; the registry introduces no path derivation.
- **Documentation Lifecycle / Task completion** — CONSTRAINTS.md invariant + design-doc sentence land in the same commit as the registry.
- New, added by this task: **Ref-Shape Registry Invariant** (see Decision: constraints-and-docs for its content).

## Testing

- **Regression floor:** `go test ./internal/planparser/... ./internal/planglyph/...` green with zero edits to existing test files — the crucible regression suite is the behavior-preservation proof.
  (`CGO_ENABLED=1` + C compiler required per the Quarry CGO Requirement Invariant; planglyph integration tests spawn quarry.)
- **TDD candidates (write the test first, watch it fail):**
  the ledger completeness meta-test (add a fake fifth kind in a test-local copy is not possible against a package-level ledger — instead assert domain equality against the declared kind list, and unit-test the disposition zero-value fail-closed lookup path);
  the grep enforcement tests (seed with a deliberate violation in a scratch string to prove the pattern matches, then assert zero hits in the real tree);
  the `containment.go:112` fail-open fix (a `ResolveResult` with an out-of-vocabulary `Status` must surface, not silently drop the member from the containment index);
  the drift per-key guard (an `introduced` glyph absent from the post-repair answer must error `ErrQuarryUnavailable`, not silently produce no finding).
- **Migration safety:** `normalizeRefIfPath` is the highest-risk site (the single `root:`-join gate) — its policy migration gets dedicated table-driven cases covering all four kinds plus the `//` escape.
- **Meta-tests to add:** ledger completeness; grep enforcement (no `classifyRef`/`refKind` comparisons outside the registry file; no open-coded `plan:` ops in either package outside the exported helpers); `.Status` tripwire (every consumer allowlisted + fail-closed); batch chokepoint pins (`repo.Resolve(` only in `resolveTargets`, `quarry.Name(` only in `CanonicalizeHandles`).
- **Exported-surface tests:** `IsHandleRef`/`HandleBody`/`NewHandle`/`HandleMember`/`HandleIdentifier`/`GlyphLanguage`/`Card.ID` each get direct unit tests in planparser; planglyph's migrated call sites are covered by its existing suites.
- Exact assertion shapes are mill-plan's job.

## Q&A log

- **Q:** Where does the registry live? **A:** planparser owns it; export only the handle vocabulary (`IsHandleRef`, `HandleBody`, `NewHandle`, `HandleMember`, `HandleIdentifier`); `RefKind`/`ClassifyRef` stay unexported. Rejected: `internal/refshape` package; full-classifier export.
- **Q:** Registry mechanism? **A:** Kind-policy ledger (per-site `map[refKind]disposition` + package-level ledger) with a completeness meta-test and grep-shaped enforcement tests; behavior switches stay switches with required default arms. Rejected: `go/analysis` lint; handler-struct dispatch.
- **Q:** Add a `refKindUnknown` zero value? **A:** No — `refKind` is only ever an inline `classifyRef` return, unexported, never struct-stored; fail-closed lives in required default arms and in the disposition type's zero value meaning "undeclared → fail closed".
- **Q:** planglyph dedup beyond handle ops? **A:** Yes — export `Plan.GlyphLanguage()` and `Card.ID()`, delete the verbatim duplicates.
- **Q:** collectGlyphTargets/resolveContainment glyph test? **A:** Keep parse-success semantics; reroute only the handle-exclusion guards; document the malformed-`#`-ref divergence as covered by `checkGlyphMalformed`.
- **Q:** Status family? **A:** Fix the one fail-open site (`containment.go:112`), add a grep tripwire, document the `renameDeclSource` Found-only and `createFindings` Ambiguous-to-default asymmetries. No registry.
- **Q:** Batch-coverage family? **A:** Add drift's missing per-key guard and the chokepoint grep pins. No new chokepoint.
- **Q:** Behavior contract? **A:** Pure refactor except the two named hardenings, each with its own new test; existing test files untouched; other defects found mid-migration become follow-up findings.
- **Q:** Docs? **A:** New Ref-Shape Registry Invariant in CONSTRAINTS.md + a registry sentence in `quarry-glyph-plan-alphabet.md`, same commit; spec and overview untouched.
- **Q:** Dead wrappers? **A:** `isHandleRef` superseded by exported `IsHandleRef`; `isGlyphRef` deleted; `isPathRef` stays unexported.
