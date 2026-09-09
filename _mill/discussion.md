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

- A kind-policy registry inside `internal/planparser` (new file `shape.go`: ledger of per-kind-gate `map[refKind]disposition` policies + completeness meta-test + AST-based enforcement tests with boundary `{classify.go, shape.go}`).
- Migration of every `refKind` dispatch site in `internal/planparser` onto the registry (full site list under Technical context).
- An exported handle-vocabulary surface on `internal/planparser` (`IsHandleRef`, `HandleBody`, `NewHandle`, `HandleMember`, `HandleIdentifier`, joining existing `HandlePrefix`/`HandleUnit`), and migration of every open-coded handle string op in `internal/planglyph` onto it.
- Export of two duplicated seams: `Plan.GlyphLanguage()` (replaces planglyph's `resolveLanguage` duplicate of planparser's `planLanguage`) and `Card.ID()` (replaces planglyph's `cardIDOf` duplicate of planparser's `cardID`).
- Family 1 hardening (`quarry.ResolveResult.Status`): fix the one confirmed fail-open consumer (`internal/planglyph/containment.go:112`, `Status` half only), add an AST-based tripwire test over `.Status` consumers, document two intentional asymmetries in place.
- Family 2 hardening (batched-answer coverage): add the missing per-key guard in `internal/planglyph/drift.go`'s post-repair path, add an AST-based chokepoint test pinning `repo.Resolve` to `resolveTargets` and `quarry.Name` to `CanonicalizeHandles`.
- New CONSTRAINTS.md invariant ("Ref-Shape Registry Invariant") and a registry sentence in `manifest/designs/quarry-glyph-plan-alphabet.md`'s package-ownership section, same commit as the code they describe.
- Deletion of all three classifier wrappers: `isGlyphRef` (dead), `isHandleRef` (superseded by the exported `IsHandleRef`), and `isPathRef` (its three callers are all policy-migration sites, emptying its caller set — see Decision: dead-wrappers).

**Out:**

- Changes to quarry (separate repo `github.com/Knatte18/quarry`, pinned `v0.1.0`, live worktree `/home/knatte/Code/quarry/wts/quarry`) — not because the repo is off limits (same author, originally part of loomyard), but per Decision: quarry-role below: the ref-shape vocabulary is plan-format semantics quarry deliberately never sees, and the two quarry-side improvements this task identified are filed as [quarry#30](https://github.com/Knatte18/quarry/issues/30) and [quarry#31](https://github.com/Knatte18/quarry/issues/31) for their own task there.
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

- Decision: every planparser kind-gate on `refKind` declares its handling as data — a policy mapping every kind to a disposition — registered in one package-level ledger in the registry file.
  **The registry file is a new `internal/planparser/shape.go`**, holding the ledger, the policies, the lookup helper, and the two relocated behavior-dispatch switches (`diskPathForRef`, `refKindName`);
  `classify.go` keeps the enum and `classifyRef` unchanged.
  **The enforcement boundary is per scan, and every exempt entry is package-qualified:** the `refKind`/`classifyRef` scan exempts exactly `{internal/planparser/classify.go, internal/planparser/shape.go}`;
  the `plan:`-op scan exempts exactly `{internal/planparser/classify.go, internal/planparser/shape.go, internal/planparser/handle.go}` — planparser's `handle.go` is the handle grammar's owner and legitimately implements it with raw `HasPrefix`/`TrimPrefix` against `HandlePrefix` (`handle.go:29`, `handle.go:39`).
  **No planglyph file is exempt from either scan** — in particular `internal/planglyph/handle.go`, which shares planparser's exempt basename and holds four of the six handle-op migration sites, is bound by the `plan:`-op scan precisely because that is where the invariant bites (review r6).
  `classify.go`'s one open-coded `"plan:"` literal (`classify.go:60`) is replaced by the same-package `HandlePrefix` const (behavior-identical, keeps the file tier1-pure), so no literal-exemption is needed inside the boundary either.
  **Disposition vocabulary — exactly three values plus an invalid zero:**
  `dispKeep` (the gate acts on this kind), `dispSkip` (the gate deliberately ignores this kind — the ref passes through untouched), `dispFinding` (the gate raises its own finding for this kind);
  the zero value means "undeclared" and the lookup helper fails closed on it (test failure/panic, never a silent skip).
  **Ledger granularity — the unit is one kind-gate, not one function.**
  A function with several independent gates registers one named policy per gate.
  Worked examples:
  `checkRenamePairShape` registers two policies — the to-side gate (Handle=`dispKeep`, Glyph/Path/Symbol=`dispFinding` → `rename-to-not-handle`) and the from-side gate (Glyph=`dispKeep`, Handle/Path/Symbol=`dispFinding` → `rename-from-not-glyph`);
  its third arm's own kind comparison (`validate.go:800`, `classifyRef(p.New) == refKindHandle`) **re-uses the to-side gate's registered policy through the lookup helper** — "does `p.New` classify as the to-side gate's `dispKeep` kind?" — so it needs no third policy and leaves no raw `refKind` comparison in `validate.go`;
  the arm's `IsSelf` half remains glyph-grammar refinement inside the from-side's `dispKeep` arm, outside ledger scope.
  `isFileRenamePair` registers one policy applied to both pair sides (Glyph=`dispKeep`, others=`dispSkip` — a non-glyph side just means "not a file-rename pair").
  `checkProsaSymbolTarget` registers one policy for its `language: none` branch only (Path=`dispKeep` i.e. passes clean, Glyph/Handle/Symbol=`dispFinding`);
  its glyph-enabled branch consults `parseGlyph`/`IsSelf`, not `refKind`, and stays outside the ledger.
  Filter-shaped sites (keep-one-kind, keep-subset) consult their policy through the lookup helper;
  the two behavior-dispatch switches stay switches inside `shape.go` but must name every kind or carry a fail-closed `default` arm, and are ledger-listed.
  **The lookup helper returns the classified kind alongside the disposition** (`lookup(gate, ref) (refKind, disposition)`), so a `dispFinding` arm renders its prose via `refKindName(k)` without re-classifying — calls *into* `shape.go`'s API (`lookup`, `refKindName`) are always legal anywhere in the package;
  what the boundary scan bans outside `{classify.go, shape.go}` is comparing/switching on `refKind` values and calling `classifyRef` directly.
  **The kind list's own source of truth:** `shape.go` declares the canonical `allRefKinds` slice, and a source-scanning meta-test parses `classify.go`'s `refKind` const block (the same AST idiom as the enforcement tests below) and asserts **set equality, not just cardinality**: the const block's declared identifier set must equal the slice's members (the bridge between the two vocabularies is `iota` position: the const block's identifiers are valued by declaration order, so the test maps identifier→`refKind` value positionally, then compares rendered `refKindName` forms, which are per-kind unique — no third hand-maintained name table), so a duplicate-plus-omission hand edit of `allRefKinds` fails even at matching length — and a fifth kind added to the enum fails immediately, cascading into every policy's completeness failure; the slice can never silently lag the enum (set-equality form: review r6).
  Three meta-tests enforce it:
  (1) the enum↔slice sync test just described;
  (2) a completeness test asserting every registered policy's domain equals `allRefKinds` — adding a fifth kind fails every gate's policy until each is re-acknowledged;
  (3) an enforcement test asserting, in either package, no `classifyRef`/`refKind` comparison outside the `refKind` scan's exempt set and no open-coded `plan:` string op (`HasPrefix`/`TrimPrefix`/concat against `HandlePrefix`, or the raw `"plan:"` literal) outside the `plan:`-op scan's exempt set — the per-scan exempt sets defined above.
  **All enforcement scans are AST-based, never raw-text** — exactly the cited precedent's own rule (`internal/cliwire/bannedecl_enforcement_test.go:71`: "The match is on the AST, never on raw text, so a doc comment naming a function cannot trip it"), using stdlib `go/parser`/`go/ast` only.
  This is what keeps the boundary honest on an untouched tree: production comments outside the boundary already name the banned tokens (`donecheck.go:24` and `planglyph/handle.go:326` spell `plan:`; `normalize.go:168`, `parse.go:160`, `validate.go:408/444/488/756-763` name `refKind*`/`isPathRef`) and stay legal, because comments and doc text never enter the AST match;
  the `"plan:"` ban matches string *literals in operand position of a shape operation* (argument to `HasPrefix`/`TrimPrefix`/concat), not every literal occurrence.
  **The scans cover production files only — `*_test.go` is excluded by design:** test files legitimately call `classifyRef` directly (`classify_test.go`'s table) and spell raw `plan:` refs as fixtures (79 occurrences across six planparser test files today); the invariant binds production dispatch sites, not test inputs.
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

- Decision: beyond the handle vocabulary, export `Plan.GlyphLanguage() (glyph.Language, bool)` (method form of planparser's unexported `planLanguage`, which returns `(glyph.Language, bool)` — the exported method keeps that return type verbatim so every migrated call site compiles unchanged) and `Card.ID() string` (method form of unexported `cardID`);
  planglyph's verbatim duplicates (`resolveLanguage` at `planglyph.go:251-258`, `cardIDOf` at `resolve.go:16-21`) are deleted, call sites migrated.
- Rationale: both duplicates carry comments admitting they mirror planparser's unexported originals;
  same visibility pathology as `refKind`, tiny diff, and `planLanguage` is the master gate on all shape logic — exactly the kind of thing that must not drift between the packages.
- Rejected: handle-ops-only scope (leaves two documented duplicates for the next review round to flag).

### parse-success-glyph-test-kept

- Decision: `collectGlyphTargets` (`planglyph.go:285-290`) and `resolveContainment` (`planglyph/containment.go:95-104`) keep parse-success (`glyph.Parse` returning nil error) as their glyph test.
  Only `collectGlyphTargets`' explicit handle-exclusion guard (`HasPrefix` at `planglyph.go:285`) reroutes through the exported `IsHandleRef`;
  `resolveContainment` has no such guard — it excludes handles implicitly via `glyph.Parse` failure (`containment.go:96`'s comment) — and its glyph test and handle handling are left unchanged;
  its one change is the `Status`-half vocabulary guard at `containment.go:112`, owned by Decision: status-family-disposition, not by this decision.
  The known divergence — a malformed `#`-carrying ref is `refKindGlyph` to planparser but silently dropped by planglyph — is documented at both sites as intentional and covered: `checkGlyphMalformed` (`validate.go:520`) already raises the blocking finding for exactly that ref.
- Rationale: planglyph's test is resolve-oriented by design (its own comment at `planglyph.go:277-281` justifies "no shape classifier of planglyph's own");
  switching to shape classification would change behavior for malformed refs and would require exporting the classifier this design keeps internal.
- Rejected: routing both sites through an exported `ClassifyRef` (behavior change, contradicts registry-home-and-exported-surface).

### quarry-role

- Decision: the registry stays in `internal/planparser`; quarry itself changes nothing in this task.
  The two producer-side improvements this task's inventory identified are filed as quarry issues for their own task there:
  [quarry#30](https://github.com/Knatte18/quarry/issues/30) — enforce per-target coverage in `Resolve`/`Name`'s batch contract (keyed answers or producer-verified alignment), which would let loomyard delete `ensureResolveCoverage` and the per-key guards this task hardens;
  [quarry#31](https://github.com/Knatte18/quarry/issues/31) — a fail-closed `Status` vocabulary helper (`Known()`/`Rejected()`), which would shrink this task's `.Status` tripwire to "every consumer guards via `Known()`".
  This task's Q6/Q7 guards are written against quarry as it is (`v0.1.0`); if the issues land later, a follow-up task migrates the guards onto the new API.
- Rationale: the ref-shape vocabulary (`plan:` handles, path/symbol spellings, `root:`/`//` resolution) is plan-format semantics — quarry never sees a handle by design, and teaching it that vocabulary inverts the layering;
  the registry's only consumers are the two loomyard packages, and iterating a cross-repo API during migration would need a version bump or `replace` per round;
  this agent is spawned in the loomyard worktree and does not edit another repo from here.
  Not adopted as rationale: "quarry is a shared dependency ruled out by the proposal" — the operator (quarry's author) explicitly reopened it; the boundary is hensiktsmessighet per family, not repo ownership.
- Rejected: expanding this task to fix families 1–2 at the source in quarry now (two-repo task with version coordination, and the loomyard-side guards are needed against the pinned version regardless);
  moving shape classification into quarry (inverted layering).

### status-family-disposition

- Decision: no registry for `quarry.ResolveResult.Status` consumers.
  Three actions instead:
  (1) fix the one confirmed fail-open site, `resolveContainment`'s bare 2-of-4 allow-list at `planglyph/containment.go:112` — **the vocabulary guard applies to the `Status` half only**: an out-of-vocabulary status (including the zero value) raises the shared unreadable-status blocking finding (rendered via the existing `unreadableStatusDetail`) instead of silently dropping the member from the containment index, with a regression test;
  the `!resolved` half (target absent from the results index) **keeps its silent `continue`, deliberately** — absence is structurally legitimate at this site, because `resolvePass` resolves `collectGlyphTargets(pending, …)` (`planglyph.go:169`) but calls `resolveContainment` on a plan that may have been reloaded after `CanonicalizeHandles` rewrote handles into glyph refs that were never in the resolve batch (review r4), and hard-erroring on absence à la `doneCheckVerdicts` would break canonicalization-introduced targets;
  a second regression test pins the absent-target skip as legal.
  Double-reporting alongside `statusFindings`' own `default` arm for the same anomalous target is accepted: both fire only on infrastructure-grade input, and suppressing one would re-couple the two consumers;
  (2) add an AST-based tripwire test (same mechanism as the registry's enforcement scans) asserting every `.Status` switch/comparison in `internal/planglyph` sits in an allowlisted, fail-closed form (switch-with-default, or a boolean derived after a vocabulary guard), so a *future* unguarded consumer is caught
  — `internal/quarrycli` also consumes `Status` (`resolve.go:48`, `expand.go:45`), both fail-closed today and deliberately outside this tripwire's file scope (quarrycli renders quarry's own output verbatim and is not part of the validation surface this task hardens);
  (3) document two intentional asymmetries in place: `renameDeclSource`'s Found-only rule (`handle.go:119` — accepting `Multipart` would arbitrarily derive from `Symbols[0]`), and `createFindings`' routing of `Ambiguous` to the `default`/`glyph-rejected` arm (`create.go:151-171`).
- Rationale: round 10's claim that `doneCheckVerdicts` was "the last unswitched `Status` consumer" is disproven by inventory (containment.go:112 remains), so one real fix is owed;
  but 5 of 7 consumers already fail closed — the family is near-consolidated and needs a tripwire, not a table, exactly as the task proposal steers.
  The producer-side fix (a fail-closed `Status` helper in quarry) is filed as [quarry#31](https://github.com/Knatte18/quarry/issues/31) — see Decision: quarry-role.
- Rejected: full registry treatment (proposal explicitly warns against it);
  doing nothing (leaves a live fail-open hole).

### batch-coverage-disposition

- Decision: no new chokepoint.
  Two actions:
  (1) add the missing per-key guard in `drift.go`'s post-repair path (`drift.go:208-227`) — **scoped precisely to answer coverage, not to batch membership**: the guard asserts that every target *actually requested in the post-repair resolve batch* (`collectGlyphTargets(reloaded, lang)`'s own output) received an answer, erroring `ErrQuarryUnavailable` on a miss, mirroring `DoneChecks`' per-key guard (`donecheck.go:139-149`);
  an introduced glyph **absent from the post-repair batch entirely** (its substitution never landed — `RewriteRefs` writes nothing for an unmatched sub, `rewrite.go:26-43` — or its `newID` fails `glyph.Parse` in `collectGlyphTargets`) keeps today's silent behavior, deliberately: a glyph not present in the reloaded plan cannot drift, whatever *is* in the plan is governed by ordinary validation, and hard-erroring on that absence would be the same absence-legitimacy mistake r4's carve-out fixed at `containment.go:112` (review r5) —
  the guard returns at the resolve boundary exactly like the sibling transport-error path (`drift.go:209-211`) already does, i.e. *before* the amendment loop (`drift.go:238-254`), so no amendment is appended on a coverage miss: an infrastructure failure produces no audit record, same as a transport failure today —
  and `drift.go:232-234`'s in-code comment ("the amendments are appended regardless") is amended in the same change to say "regardless of *findings*, never on an infrastructure error", so it does not contradict the new early-return;
  (2) add an AST-based chokepoint test pinning `repo.Resolve` calls to `resolveTargets` (`repo.go:97`) and `quarry.Name` calls to `CanonicalizeHandles` (`handle.go:224`), so the existing length-guard chokepoints (`ensureResolveCoverage`, the Name length+echo guard) cannot be bypassed by a future call site.
- Rationale: inventory confirms round 10's consolidation claim holds for this family — every batch boundary is guarded except drift's post-repair site, which is unreachable-in-practice only because of the length guard it does not itself own.
  The producer-side fix (coverage enforced in `Resolve`/`Name`'s own contract) is filed as [quarry#30](https://github.com/Knatte18/quarry/issues/30) — see Decision: quarry-role.
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
  named enforcement: the enum↔slice sync test, the ledger completeness meta-test, and the AST-based boundary-enforcement tests.
  `manifest/designs/quarry-glyph-plan-alphabet.md`'s "The package-ownership seam" section gains a sentence naming the registry and the exported handle vocabulary.
  `contracts/specs/loom-plan-spec.md` is untouched (classification behavior is identical).
  `docs/overview.md` is untouched (no new module, no CLI/execution-stack change).
- Rationale: repo's Documentation Lifecycle rule (docs land in the same commit); the task proposal's "Related" section names both files.
- Rejected: CONSTRAINTS.md only (drops the design-doc reference the proposal asks for).

### dead-wrappers

- Decision: all three wrappers go.
  `isHandleRef` is deleted as dead, identically to `isGlyphRef` — it has zero production callers (test-only, `classify_test.go`); the exported `IsHandleRef` is a new API for planglyph, not a rename with callers to migrate;
  `isGlyphRef` is deleted (zero production callers; its test coverage folds into the registry tests);
  `isPathRef` is deleted too — its only three production callers (`normalize.go:112`, `normalize.go:178`, `validate.go:1040`) are all policy-lookup migration sites in Inventory A, so after migration the wrapper is exactly as dead as `isGlyphRef`.
  (Supersedes the Q&A log's initial "stays unexported" answer, which predated noticing that the migration empties its caller set — review r2 finding.)
- Rationale: keep-for-symmetry is dead API; the wrappers' stated purpose (guarding `classifyRef`'s return-value shape) is subsumed by the registry meta-tests.
- Rejected: keeping any wrapper for symmetry;
  keeping `isPathRef` alongside an empty caller set.

## Technical context

### The classifier today

`internal/planparser/classify.go` — `refKind` (int, unexported, `refKindPath = iota`, then Symbol/Glyph/Handle), `classifyRef` (five ordered rules, spec-pinned in `contracts/specs/loom-plan-spec.md` § "The shape classifier"), wrappers `isPathRef`/`isGlyphRef`/`isHandleRef`.
The file is a tier1-pure leaf (string analysis only, never stats disk, never calls `glyph.Parse`).
`isGlyphRef`/`isHandleRef` have zero production callers (test-only).

### planparser migration sites (Inventory A)

True switches (stay switches, ledger-listed, default-arm-required, both relocated into the registry file per Decision: kind-policy-ledger):

- `validate.go:360-374` `diskPathForRef` — the sole ref→disk-path mapper (Path→itself; Glyph→language gate + `parseGlyph` + `IsSelf` + `UnitPath`; Symbol/Handle→default not-ok). Feeds `checkCardPathMalformed`, `checkPathMissing`, `createTargetsUnion`, `renameTargetsUnion`.
- `validate.go:828-843` `refKindName` — exhaustive kind→prose switch with deliberate non-panicking fallthrough ("an unrecognized shape"). The registry nucleus.

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

- `donecheck.go:35-40` `resolveKeyFor` — the de-facto shared helper (7 callers across 4 files); **kept, as a thin wrapper over `HandleBody`** — its `func(string) string` pass-through-on-non-handle signature is what all 7 call sites are built around, while `HandleBody` returns `(string, bool)`; replacing it outright would force every call site to handle the `ok` return for zero gain.
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
- **Test Tier Purity Invariant** — `classify.go` and the registry file stay tier1-pure leaves (string analysis only); the AST-based enforcement tests are untagged and parse source files only (stdlib `go/parser`, no spawns).
- **Told-Geometry Invariant** — both packages remain bound packages; the registry introduces no path derivation.
- **Documentation Lifecycle / Task completion** — CONSTRAINTS.md invariant + design-doc sentence land in the same commit as the registry.
- New, added by this task: **Ref-Shape Registry Invariant** (see Decision: constraints-and-docs for its content).

## Testing

- **Regression floor:** `go test ./internal/planparser/... ./internal/planglyph/...` green with no behavioral edits to existing test files.
  Sanctioned, behavior-neutral exceptions: `classify_test.go:100-126` (`TestClassifyRefWrappers` calls the three wrappers being deleted — it is rewired to `classifyRef`/the exported `IsHandleRef`, or folded into the registry tests);
  `doc.go:56` (the package doc names all three wrappers and is updated to the registry vocabulary);
  and comment-only updates wherever a comment names a deleted wrapper — `normalize.go:7`, `normalize.go:70`, `normalize.go:107`, `parse.go:160` in production, plus the comment mentions in `parse_test.go:650-651` and `validate_test.go:1775` — none of which alters any assertion.
  Every crucible-derived regression test's assertions stay untouched — those are the behavior-preservation proof.
  (`CGO_ENABLED=1` + C compiler required per the Quarry CGO Requirement Invariant; planglyph integration tests spawn quarry.)
- **TDD candidates (write the test first, watch it fail):**
  the ledger completeness meta-test (add a fake fifth kind in a test-local copy is not possible against a package-level ledger — instead assert domain equality against the declared kind list, and unit-test the disposition zero-value fail-closed lookup path);
  the AST enforcement tests (seed with a deliberate violation in a synthetic parsed file to prove the matcher fires, then assert zero hits in the real tree);
  the `containment.go:112` fix, both halves (a `ResolveResult` with an out-of-vocabulary `Status` must surface as the unreadable-status blocking finding, not silently drop the member; a target absent from the results index must still be silently skipped — the canonicalization-introduced-target case);
  the drift per-key guard, both halves (a target requested in the post-repair batch but unanswered must error `ErrQuarryUnavailable`; an introduced glyph absent from the post-repair batch entirely must keep today's silent pass).
- **Migration safety:** `normalizeRefIfPath` is the highest-risk site (the single `root:`-join gate) — its policy migration gets dedicated table-driven cases covering all four kinds plus the `//` escape.
- **Meta-tests to add** (every source scan AST-based per the cliwire precedent, production files only): enum↔slice sync (set equality between `classify.go`'s const-block declared kinds and `allRefKinds`' members, identifiers valued by `iota` position — never a bare count comparison); ledger completeness (every policy's domain == `allRefKinds`); boundary enforcement (no `classifyRef`/`refKind` comparisons outside `{classify.go, shape.go}`; no open-coded `plan:` shape ops in either package outside the exported helpers); `.Status` tripwire (every consumer allowlisted + fail-closed); batch chokepoint pins (`repo.Resolve` only in `resolveTargets`, `quarry.Name` only in `CanonicalizeHandles`).
- **Exported-surface tests:** `IsHandleRef`/`HandleBody`/`NewHandle`/`HandleMember`/`HandleIdentifier`/`GlyphLanguage`/`Card.ID` each get direct unit tests in planparser; planglyph's migrated call sites are covered by its existing suites.
- Exact assertion shapes are mill-plan's job.

## Q&A log

- **Q:** Where does the registry live? **A:** planparser owns it; export only the handle vocabulary (`IsHandleRef`, `HandleBody`, `NewHandle`, `HandleMember`, `HandleIdentifier`); `RefKind`/`ClassifyRef` stay unexported. Rejected: `internal/refshape` package; full-classifier export.
- **Q:** Registry mechanism? **A:** Kind-policy ledger (per-kind-gate `map[refKind]disposition` + package-level ledger) with a completeness meta-test and boundary-enforcement tests (initially sketched as greps; mechanism settled as AST-based in review r4 — see the later Q&A entry). Rejected: `go/analysis` lint; handler-struct dispatch.
- **Q:** Add a `refKindUnknown` zero value? **A:** No — `refKind` is only ever an inline `classifyRef` return, unexported, never struct-stored; fail-closed lives in required default arms and in the disposition type's zero value meaning "undeclared → fail closed".
- **Q:** planglyph dedup beyond handle ops? **A:** Yes — export `Plan.GlyphLanguage()` and `Card.ID()`, delete the verbatim duplicates.
- **Q:** collectGlyphTargets/resolveContainment glyph test? **A:** Keep parse-success semantics; reroute only the handle-exclusion guards; document the malformed-`#`-ref divergence as covered by `checkGlyphMalformed`.
- **Q:** Status family? **A:** Fix the one fail-open site (`containment.go:112`; scoped to the `Status` half in review r4 — see that Q&A entry), add a tripwire test, document the `renameDeclSource` Found-only and `createFindings` Ambiguous-to-default asymmetries. No registry.
- **Q:** Batch-coverage family? **A:** Add drift's missing per-key guard and the chokepoint pins. No new chokepoint.
- **Q:** Behavior contract? **A:** Pure refactor except the two named hardenings, each with its own new test; existing test files untouched; other defects found mid-migration become follow-up findings.
- **Q:** Docs? **A:** New Ref-Shape Registry Invariant in CONSTRAINTS.md + a registry sentence in `quarry-glyph-plan-alphabet.md`, same commit; spec and overview untouched.
- **Q:** Dead wrappers? **A:** All three deleted — `isHandleRef` superseded by exported `IsHandleRef`; `isGlyphRef` dead; `isPathRef`'s caller set is emptied by the policy migration (initial answer "stays unexported" superseded in review r2 — see Decision: dead-wrappers).
- **Q:** Is quarry off limits as a home for any of this? **A:** No — the operator (quarry's author) rejected the proposal's "ruled out" framing; the boundary is appropriateness per family, not repo ownership. Outcome: registry stays in planparser (layering — quarry never sees `plan:` vocabulary), and the two producer-side improvements are filed as quarry#30 (batch coverage in the API contract) and quarry#31 (fail-closed `Status` helper) for their own task in that repo.
- **Q:** Which file is the registry, and is `classify.go` inside the enforcement boundary? **A:** [auto-pick] New `internal/planparser/shape.go` holds ledger/policies/lookup + the two relocated switches; the enforcement boundary is exactly `{classify.go, shape.go}`, and `classify.go`'s open-coded `"plan:"` literal is replaced by `HandlePrefix`. **Why:** keeps `classify.go`'s spec-pinned classifier untouched while giving the scans a two-file exemption with no per-function carve-outs (review r2, BLOCKING).
- **Q:** Disposition vocabulary and ledger granularity? **A:** [auto-pick] Three values (`dispKeep`/`dispSkip`/`dispFinding`) plus invalid zero (fail-closed lookup); the ledger unit is one kind-gate, not one function — `checkRenamePairShape` registers two policies (its `IsSelf` third arm is a glyph-grammar refinement outside ledger scope), `isFileRenamePair` one policy over both sides, `checkProsaSymbolTarget` one policy for its `language: none` branch only. **Why:** makes the completeness meta-test's domain assertion well-defined at every multi-gate and language-forked site (review r2, BLOCKING).
- **Q:** What keeps the kind list in sync with `classify.go`'s enum? **A:** [auto-pick] `shape.go` declares the canonical `allRefKinds` slice, and a source-scanning meta-test asserts set equality between the const block's declared kinds and the slice's members (strengthened from the initial count-equality form in review r6 — see Decision: kind-policy-ledger for the `iota`-position bridge). **Why:** without it, a fifth enum constant fails nothing until someone remembers to append the slice — exactly the silent-lag requirement 4 exists to prevent (review r3, BLOCKING; set-equality form r6).
- **Q:** Which half of `containment.go:112` gets the guard? **A:** [auto-pick] Only the `Status` half — out-of-vocabulary status raises the shared unreadable-status blocking finding; the `!resolved` index-miss keeps its silent `continue`, because canonicalization-introduced targets are legitimately absent from the resolve batch. **Why:** hard-erroring on absence à la `doneCheckVerdicts` would break the reloaded-plan path (review r4, BLOCKING).
- **Q:** Are the enforcement scans raw-text greps or AST-based? **A:** [auto-pick] AST-based, stdlib `go/parser`/`go/ast`, matching the cliwire bannedecl precedent's own "never on raw text" rule; comments and non-operand string literals never trip, so existing production comments naming the tokens stay legal. **Why:** a raw-text grep fails on an untouched tree (banned tokens live in comments today) and the cited precedent already decides the mechanism (review r4, BLOCKING).
- **Q:** Are the scan exempt sets basenames or package-qualified paths? **A:** [auto-pick] Package-qualified (`internal/planparser/...`); no planglyph file is exempt from either scan. **Why:** `internal/planglyph/handle.go` shares planparser's exempt basename and holds four of the six handle-op migration sites — a basename exemption would disarm the scan exactly where it must bind (review r6, BLOCKING).
- **Q:** Which absences does the drift guard cover? **A:** [auto-pick] Only answer coverage — every target requested in the post-repair resolve batch must be answered, else `ErrQuarryUnavailable`; an introduced glyph absent from the batch entirely (unlanded sub or unparsable `newID`) keeps today's silent pass. **Why:** a glyph not in the reloaded plan cannot drift and is governed by ordinary validation — the same absence-legitimacy carve-out r4 established for `containment.go:112` (review r5, BLOCKING).
