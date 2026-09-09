# `loom` crucible review — round 1, tag `opus5-high-r1`

> Independent clean-room review + fix of the two behavior-preserving refactors
> (`centralize-glyph-shape-enum`, `quarry-bump-v0-2-0-status-helpers`) driven LIVE through the real
> built `cmd/lyx` binary. Per `_mill/loom-review-prompt.md`.
> Clean-room constraint honored: nothing under `_mill/loom-review-*` was opened before this file's
> own findings list was complete.

## Executive summary

**The two refactors are behavior-preserving. I found no regression in either.** That conclusion is
not from re-running the unit suite (which was already green and proves nothing here) — it is from a
line-by-line diff audit of every migrated gate against its ledger row, plus **eleven live scenarios
driven through the freshly built `cmd/lyx` binary** against real git history and real quarry
resolution, including the full `record-batch` path that nobody had driven since the refactors landed.

All four scenarios the campaign was created to check now have live evidence:

1. **Create card + `plan:` handle canonicalization + binding** — draft handle rewritten to its
   canonical form on disk in both the declaring and the referencing card; Create inversion fires
   `create-already-exists` for an existing target and passes `create-new-unit` informationally for a
   new unit.
2. **Rename exact-tier auto-bind** — a real `quarry.DeltaGit` rename matched gate one and was
   correctly *not* treated as drift; the handle bound in both cards; no spurious amendment. The
   named prior regression (`renameCardPairs` missing an entry) is **not** reopened.
3. **Deliberate drift** — `plan-references-deleted-symbol` blocks, and the evidence tier writes no
   amendment. The exact tier's *repair* half also verified separately: one rewrite, exactly one
   six-field amendment.
4. **Fail-closed `lookup`** — the panic is unreachable today by construction (all 13 gates × 4 kinds
   declared), which is the correct state.

The five findings are therefore **not** "the refactors broke something". They are that the safety
machinery each refactor shipped to *keep* itself correct is weaker than its own documentation claims:

- **F1 (MEDIUM)** — the ref-shape registry enforces `refKind`↔`allRefKinds` sync with an AST
  meta-test but does **not** enforce `refGate`↔`ledger` sync at all, so one forgotten ledger entry
  ships green and panics live out of `validate-plan`.
- **F2 (MEDIUM)** — the `.Status` tripwire matches only the `Status` selector, so
  `ResolveResult.Rejected()` — the spelling the v0.2.0 refactor itself introduced, and the one that
  fails *open* for an unrecognized status — bypasses the tripwire entirely.
- **F4 (MEDIUM)** — the real `quarry.DeltaGit`→`DetectDrift` seam, gate one's load-bearing string
  comparison, has no test at any tier; every existing test feeds a synthetic delta.
- **F5 (LOW)** — `doc.go`'s self-declared exhaustive Check-ID list omits the `glyph-rejected` raiser
  this refactor added to `containment.go`.
- **F3 (LOW)** — a Create target resolving `ambiguous` is reported to the operator as an
  "unrecognized resolve status", which is false and points them at the wrong subsystem.

**Top risks:** F1 and F2, both of which convert a future routine edit into a silent loss of the exact
guarantee the campaign was asked to confirm. Neither is exploitable today.

**Merge-readiness opinion (pre-fix):** the two refactors themselves are merge-ready on correctness
grounds. I would not merge without F1, F2 and F4 closed, because the whole point of shipping a
registry and a predicate adoption is the mechanical guarantee, and two thirds of that guarantee is
currently aspirational.

## Scope assessment — plan-promised vs shipped

Both commits are labelled behavior-preserving refactors. Measured against that:

**Shipped as promised.** Every one of the 13 migrated ref-shape gates, and both quarry-predicate
adoptions, are exactly equivalent to the code they replaced (table below). The centralization also
removed four genuine duplications (`cardIDOf`, `resolveLanguage`, `draftHandleMember`,
`draftHandleIdentifier`) in favour of exported planparser forms with identical bodies.

**Shipped BEYOND the stated scope — two deliberate behavior changes inside a "behavior-preserving"
commit.** Neither is a defect; both widen fail-closed coverage, and both are the right disposition.
They are flagged here because a reader trusting the commit's own framing would not expect them:

1. `resolveContainment` (`containment.go:132-158`) gained a `default:` arm raising blocking
   `glyph-rejected`. Before the refactor, an unreadable status was a silent skip of the member entry.
2. `DetectDrift` gained `ensurePostRepairCoverage` (`drift.go:223-225, 292-300`), a new
   `ErrQuarryUnavailable` path that did not exist before.

Change 1 is the one that leaves a documentation debt — see F5.

**Deferred-that-should-be-v1:** none found. **Nothing shipped that the design doc forbids.**

## Status

- Job 1 (review): **COMPLETE** — findings below are final; report committed before any source edit.
- Job 2 (fix): pending.

## Environment

| Thing | Value |
| --- | --- |
| Host | Linux, `7.0.0-30-generic` |
| Go | `go1.26.0 linux/amd64` |
| `CGO_ENABLED` | `1` (gcc on PATH at `/usr/bin/gcc`) |
| `tmux` | present at `/usr/bin/tmux` — smoke tests will NOT skip-as-pass |
| quarry | `v0.2.0` (module cache `github.com/!knatte18/quarry@v0.2.0`) |
| Branch / HEAD | `crucible-loom-refshape-registry` @ `8503e22f3` |

## What was tested

### Hermetic baseline (before any edit)

| # | Command | Result |
| --- | --- | --- |
| H1 | `go build ./...` | exit 0, clean |
| H2 | `go vet ./internal/loomengine/... ./internal/loomcli/... ./internal/loomshed/... ./internal/planparser/... ./internal/planglyph/... ./internal/websterengine/... ./internal/webstercli/...` | exit 0, clean |
| H3 | `go test -count=5 ./internal/loomengine/... ./internal/loomcli/... ./internal/loomshed/... ./internal/planparser/... ./internal/planglyph/... ./internal/websterengine/... ./internal/webstercli/... ./cmd/lyx/...` | all 8 packages `ok`, exit 0 |

So the baseline is green: whatever this campaign finds, the unit suite does not see it. That is the
premise the campaign was created on and it holds.

### quarry v0.2.0 contract, read from source (not assumed)

`$(go env GOMODCACHE)/github.com/!knatte18/quarry@v0.2.0/internal/engine/answer.go`:

- `Status` is `string`; the closed vocabulary is `found` / `not_found` / `ambiguous` / `multipart`.
- `func (s Status) Known() bool` — `true` for exactly those four, `false` for `""` **and for any
  other non-empty value**.
- `func (r ResolveResult) Rejected() bool { return r.Status == "" }` — reads `Status`, deliberately
  NOT `Error != ""`, so a rejection with an empty message still reports `Rejected() == true`.

**The two predicates are therefore NOT complements.** `!Known()` is strictly weaker than `Rejected()`:

| `Status` | `Known()` | `Rejected()` |
| --- | --- | --- |
| `found`/`not_found`/`ambiguous`/`multipart` | true | false |
| `""` (pre-resolution rejection) | false | true |
| any other non-empty value (future vocabulary) | false | **false** |

Any call site that treats `!Known()` and `Rejected()` as the same predicate is wrong for the third
row. This is the exact seam the refactor introduced, so it is where I looked hardest.

### Behavior-preservation audit by diff (the sharpest available check)

Both refactors are single commits, so "is it behavior-preserving" is answerable exactly rather than
by inference. I read every production hunk of both.

`bbd3fdaf3` (quarry v0.2.0 adoption) — **2 production hunks, both exactly equivalent**:

| Site | Before | After | Equivalent? |
| --- | --- | --- | --- |
| `donecheck.go` `doneCheckVerdicts` | `switch r.Status { case Found, Multipart, Ambiguous, NotFound: ; default: →glyph-rejected }` | `if !r.Status.Known() { →glyph-rejected }` | YES — `Known()` is literally that four-value switch |
| `resolve.go` `unreadableStatusDetail` | `if r.Status == ""` | `if r.Rejected()` | YES — `Rejected()` is literally `r.Status == ""` |

`7c50e1a2f` (ref-shape registry) — every migrated gate, checked against its ledger row:

| Call site | Before | After | Ledger row | Equivalent? |
| --- | --- | --- | --- | --- |
| `containment.go` `syntacticContainment` | `!= refKindGlyph → skip` | `disp != dispKeep → skip` | glyph=Keep, rest=Skip | YES |
| `normalize.go` `normalizeRefIfPath` | `!isPathRef → passthrough` | `disp != dispKeep → passthrough` | path=Keep, rest=Skip | YES |
| `normalize.go` `canonicalizeCard`'s `canon` | `!isPathRef \|\| !canonicalizablePath` | two sequential guards, same order | path=Keep, rest=Skip | YES (short-circuit order preserved) |
| `validate.go` `checkBareSymbolTarget` | `!= refKindSymbol → skip` | `disp != dispFinding → skip` | symbol=Finding, rest=Skip | YES |
| `validate.go` `checkDirectoryTarget` | `!= refKindPath → skip` | `disp != dispKeep → skip` | path=Keep, rest=Skip | YES |
| `validate.go` `checkGlyphMalformed` | `!= refKindGlyph → skip` | `disp != dispKeep → skip` | glyph=Keep, rest=Skip | YES |
| `validate.go` `checkHandleMalformed` | `!= refKindHandle → skip` | `disp != dispKeep → skip` | handle=Keep, rest=Skip | YES |
| `validate.go` `isFileRenamePair` | `Old!=glyph \|\| New!=glyph → false` | two sequential lookups | glyph=Keep, rest=Skip | YES |
| `validate.go` `checkRenamePairShape` to-side | `k != refKindHandle → finding` | `disp == dispFinding → finding` | handle=Keep, rest=Finding | YES |
| `validate.go` `checkRenamePairShape` from-side | `k != refKindGlyph → finding` | `disp == dispFinding → finding` | glyph=Keep, rest=Finding | YES |
| `validate.go` `checkProsaSymbolTarget` | `isPathRef(t) → continue` | `disp == dispKeep → continue` | path=Keep, rest=Finding | YES |
| `rewrite.go` `rewriteBulletLine` | `strings.HasPrefix(x, HandlePrefix)` | `IsHandleRef(x)` | n/a | YES (`IsHandleRef` is that call) |
| `shape.go` `diskPathForRef`, `refKindName` | in `validate.go` | relocated verbatim | n/a | YES (byte-identical) |

planglyph-side deletions all replaced by verbatim-equivalent exported forms:
`cardIDOf`→`Card.ID()` (same `Sprintf`), `resolveLanguage`→`Plan.GlyphLanguage()` (same switch, via
`planLanguage`), `draftHandleMember`/`draftHandleIdentifier`→`planparser.HandleMember`/
`HandleIdentifier` (identical bodies), `strings.TrimPrefix(h, HandlePrefix)`→`resolveKeyFor(h)`
(identical for every handle-shaped input, and `cardOwnHandles` only ever yields handle-shaped ones).

**Conclusion of the diff audit: no silent behavior change in the migrated gates.** Two hunks in
`7c50e1a2f` are NOT behavior-preserving, but both widen fail-closed coverage deliberately — see the
scope assessment below.

### Live driving — the real built binary

Binary built fresh from this tree with the repo's own deploy flags
(`tools/deploy/main.go` pins `CGO_ENABLED=1`):

```
CGO_ENABLED=1 go build -ldflags "-X github.com/Knatte18/loomyard/internal/buildinfo.Channel=dev" \
  -o <scratch>/bin/lyx ./cmd/lyx
```

Fixture geometry (`lyxcwd` needs hub=parent-of-worktree, cwd==worktree root, anchor "."):

```
<scratch>/live1-HUB/wt/          <- git repo, cwd for every command below
  go.mod                         module fixture
  sub/a.go                       package sub: func Old(), func Keep()
  _lyx/config/*.yaml             seeded via `lyx config reconcile --apply`
  _lyx/plan/00-overview.md       format: 5, approved: true, language: go + Card Index
  _lyx/plan/01-create-card.md
  _lyx/plan/02-edit-card.md
```

| # | Scenario | Command | Observed | Verdict |
| --- | --- | --- | --- | --- |
| L0 | geometry bring-up | `lyx loom validate-plan` | `config file .../_lyx/config/loom.yaml not found` → fixed by `lyx config reconcile --apply` | n/a |
| L1 | **Create card, `plan:` draft handle, canonicalization** — card 1 declares `` `plan:sub#Draft` -> `func Actual() {}` ``, card 2 `Uses` `plan:sub#Draft` | `lyx loom validate-plan` | **Both card files rewritten on disk**: card 1's declaration AND card 2's reference became `plan:sub#Actual`. | **CORRECT.** `CanonicalizeHandles` → `quarry.Name` → `planparser.RewriteRefs` works end-to-end through the centralized registry, and the rewrite reaches the *referencing* card, not just the declaring one. |
| L2 | canonicalized plan revalidates clean, both gate modes | `lyx loom validate-plan` / `... --require-approved` | `{"ok":true,...}` exit 0 for both | **CORRECT.** Canonicalization is idempotent — no second rewrite, no `plan-unapproved`. |
| L3 | **Create inversion, blocking half** — `` `plan:sub#Keep` -> `func Keep() {}` `` where `sub#Keep` already exists | `lyx loom validate-plan` | `create-already-exists/1-create-card[blocking]: Create target "plan:sub#Keep" already resolves found` (plus `handle-unreferenced`) | **CORRECT** per spec §"the Create inversion". |
| L4 | **Create inversion, pass half** — `plan:brandnew#Thing` in a unit that does not exist | `lyx loom validate-plan` | `{"findings":["create-new-unit/1-create-card[informational]: ... introduces a new unit"],"ok":true}` exit **0** | **CORRECT.** Informational, surfaced on the pass path under its own key, does not fail the gate. |
| L5 | **genuine `ambiguous`** — second `func Keep() {}` added in `sub/b.go` | `lyx loom validate-plan` | `glyph-ambiguous/2-edit-card[blocking]: target "sub#Keep" is ambiguous among candidates: sub#Keep, sub#Keep` | **CORRECT** per spec, candidates listed. |
| L6 | **`ambiguous` on a Create target** — `` `plan:sub#Keep` `` while `sub#Keep` is ambiguous | `lyx loom validate-plan` | `glyph-rejected/1-create-card[blocking]: Create target "plan:sub#Keep" answered the unrecognized resolve status "ambiguous"` | **DISPOSITION correct, MESSAGE WRONG.** See finding F3. |

### Live driving — `lyx webster record-batch`, the `DetectDrift`/`BindHandles` surface

`record-batch` is the only CLI route to `DetectDrift`. Standing it up needs a webster run state, a
batch report, and Claude Code transcript stubs — **zero real LLM subprocesses**; empty `.jsonl`
files satisfy the fork audit. Geometry used (standalone mode, `--plan-dir` override):

```
state dir : ~/.local/state/lyx/e74a94dd/_lyx/webster/{state.json,reports/NN-<slug>.yaml}
transcripts: ~/.claude/projects/<worktree-path-with-non-alnum-as-dash>/crucible-master.jsonl
                                                        .../crucible-master/subagents/fork-N.jsonl
worktree  : <scratch>/live2-HUB/wt   (real git history, real Go symbols)
```

| # | Scenario | Command | Observed | Verdict |
| --- | --- | --- | --- | --- |
| L7 | **Rename to-side unit correction** — draft `` `sub#Old` -> `plan:totallywrongpkg#Renamed` `` | `lyx loom validate-plan` | rewritten on disk to `plan:sub#Renamed` in card 1 **and** in card 2's `Uses` | **CORRECT.** `renameDeclSource` takes Unit from the *resolved old side*, not the draft — exactly the spec's "a draft that misspells the unit is corrected by canonicalization rather than propagated". |
| L8 | **Rename exact-tier auto-bind** — real git commit renaming `Old`→`New`; card 1 declares `` `sub#Old` -> `plan:sub#New` `` | `lyx webster record-batch 1 --plan-dir …` | `{"batch":"01-rename-card","status":"done","ok":true}` exit 0. Card 1's to-side **and** card 2's `Uses` both bound `plan:sub#New`→`sub#New`. **No `amendments.md` written.** | **CORRECT, and the named regression is NOT reopened.** `renameCardPairs` gate one recognized the real `quarry.DeltaGit` rename as card 1's own declared outcome — no drift finding, no auto-"repair", no amendment. `BindHandles`+`cardOwnHandles` bound the Rename-pair handle (the sonnet-xhigh-r8 PG-2 fix) and the rewrite reached the referencing card. |
| L9 | **Deliberate drift** — batch 2 deletes `sub#Victim` with **no** declaring card, while still-pending card 3 `Uses` it | `lyx webster record-batch 2 --plan-dir …` | `{"card_not_done":true,"error":"…plan-references-deleted-symbol/3-later-card[blocking]: card 3 references \"sub#Victim\", which the delta reports deleted with no corresponding rename"}` exit 1. **No `amendments.md`.** | **CORRECT.** Blocking, and the evidence tier never called `RewriteRefs`/`AppendAmendment`. |
| L9b | drift attributed to the recording batch's **own** card | `lyx webster record-batch 2` (card 2 both deletes and references) | no drift finding; `scope-outside-plan` informational only | **CORRECT by design** — `completedCards(…, exclude)` folds the recording batch's own cards into "completed", which is exactly what `PendingPlan`'s doc says must happen so a Delete card's own batch can be recorded. |
| L10 | **Exact-tier auto-REPAIR** — batch 1 renames `sub#Gamma`→`sub#Delta` with no declared Rename card; pending card 2 `Uses` `sub#Gamma` | `lyx webster record-batch 1 --plan-dir …` | exit 0; card 2's `Uses` rewritten to `sub#Delta`; `amendments.md` created with **exactly one** entry: `Timestamp: 2026-09-09T10:48:00Z, Card: 2-later-card, OldGlyph: sub#Gamma, NewGlyph: sub#Delta, Tier: exact, SHA: b57f4f5b…` | **CORRECT.** All six amendment fields present, one entry, tier word `exact`. This is the first time the *real* `quarry.DeltaGit`→`DetectDrift` path has been driven — every existing test feeds a synthetic delta (see F4). |
| L11 | **`Delete` card naming a whole file by path** (`` `//sub/b.go` ``), file really deleted | `lyx loom validate-plan`, then `record-batch 1` | validate `ok:true`; record-batch `status:"done"` exit 0, **no `glyph-rejected`** | **CORRECT — and it disproves a hypothesis I formed by reading.** I expected `resolveKeyFor` to hand quarry the bare path `sub/b.go`, which quarry pre-resolution-rejects. It does not: `canonicalizeCard`'s `gateCanonicalizePath` gate canonicalizes the path to the **self glyph** `sub/b.go#` in memory at parse time, so `DoneChecks` resolves a parseable glyph. Confirmed against quarry directly: `lyx quarry resolve 'sub/b.go#' 'sub/b.go'` → the first answers `not_found`, the second answers `glyph: parse … a glyph needs a "#"`. |

**Consequence worth stating plainly:** the pre-resolution-rejection branch (`ResolveResult.Rejected()`
in `unreadableStatusDetail`, and every `glyph-rejected` fail-closed arm keyed on it) is
**structurally unreachable in live operation today** — `collectGlyphTargets` filters by `glyph.Parse`,
and every other Resolve input has already been canonicalized to a parseable glyph. That is the
correct state of affairs for a fail-closed guard, but it means the guards' only proof is unit-level,
and it answers the campaign prompt's question directly: nothing reaches those arms because upstream
prevents it, not because the arms are wrong.

### The fail-closed `lookup` — is the panic reachable?

Checked directly. All 13 declared `refGate` constants have a `ledger` entry, and
`TestLedgerCompleteness` proves every entry covers all four `refKind`s. So for every gate a live
dispatch site can name, `lookupIn`'s `d == 0` panic is **unreachable today** — by construction, not
by luck. No live scenario above produced a panic, as expected.

That is the right answer for the code as written. The problem is what *keeps* it true — see F1.

## Findings

Five findings. None is a behavior regression in the migrated gates — the diff audit and eleven live
scenarios agree the two refactors are behavior-preserving where they claim to be. All five are gaps
in what the refactors' own safety machinery actually enforces or documents.

### F1 — the registry's gate↔ledger sync is NOT enforced, so the fail-closed panic is a live CLI crash waiting on one edit (MEDIUM, CONFIRMED)

`internal/planparser/shape.go:107` (`ledger`), `internal/planparser/shape_test.go:120-145`
(`TestLedgerCompleteness`).

`shape.go`'s file doc claims the fail-closed guarantee is made "mechanical rather than aspirational"
by two meta-tests, and `CONSTRAINTS.md:231` names "the enum↔slice sync meta-test, the ledger
completeness meta-test" as the invariant's enforcement. One of those two syncs is real; the other
is not.

- `refKind` enum ↔ `allRefKinds` **is** genuinely enforced: `TestRefKindEnumMatchesAllRefKinds`
  parses `classify.go`'s AST for the const block and asserts set equality both directions
  (`shape_test.go:79-115`). A fifth `refKind` fails the suite immediately.
- `refGate` constants ↔ `ledger` keys is **not enforced at all.** `TestLedgerCompleteness` opens with
  `for gate, policy := range ledger` — it iterates the map's *existing* keys. A `refGate` constant
  declared in `shape.go` with **no** `ledger` entry contributes no iteration, so the test passes.

Failure scenario, concrete: a later task adds a 14th gate —

```go
gateSomethingNew refGate = "something-new"
```

— and a dispatch site `if _, disp := lookup(gateSomethingNew, ref); disp != dispKeep { … }`, but
forgets the `ledger` entry (or typos the map key, which is the same defect from the other side:
`TestLedgerCompleteness` happily accepts a ledger key that is not a declared constant). `go build`,
`go vet` and the whole test suite stay green — `ledger[gateSomethingNew]` is a nil map, `policy[k]`
is the zero disposition, and `lookupIn` **panics**. Not a finding, not an error envelope: a raw Go
panic out of `lyx loom validate-plan` / `lyx webster begin-batch`, on the first plan whose refs reach
that gate.

The disposition is right — fail-closed is what you want at runtime. What is wrong is that the
*prevention* the file's own doc promises does not exist, and the one place a reader would look to
confirm it (`TestLedgerCompleteness`) reads as though it does. Note that the two constructs are not
symmetric by accident: `allRefKinds` needed an AST-parsing test precisely because Go cannot reflect
over a const block, and `refGate` has exactly the same problem and got no such test.

CONFIRMED by code reading; the reasoning is airtight (a map range cannot visit an absent key), and
`TestLookupFailsClosedOnUndeclaredKind/PanicsOnGateAbsentFromLedger` (`shape_test.go:173-182`) already
proves the panic half of the scenario with a synthetic gate name.

Suggested fix: a `refGateConstNamesInDeclarationOrder`-style meta-test parsing `shape.go`'s own
`refGate` const block (the same `runtime.Caller(0)` + `go/parser` idiom `shape_test.go` already uses
for `classify.go`) and asserting set equality against `ledger`'s key set in **both** directions.
Correct `CONSTRAINTS.md`'s "Named enforcement" bullet in the same change so it names what is actually
enforced.

### F2 — the `.Status` tripwire is blind to `Rejected()`, the very spelling the quarry v0.2.0 refactor introduced (MEDIUM, CONFIRMED)

`internal/planglyph/status_enforcement_test.go:65-88` (`statusHitsIn`), matcher at line 70:

```go
if ok && sel.Sel != nil && sel.Sel.Name == "Status" {
```

The tripwire's stated purpose (its own file doc, lines 8-13) is that "a NEW `.Status` read, added
later without reading this file, is caught before it ships", forcing the author to make the consumer
fail closed and register it in `allowedStatusConsumers`.

Before quarry v0.2.0 there was exactly one way to read the resolve-status vocabulary: the `Status`
field. `bbd3fdaf3` added two more — `Status.Known()` and `ResolveResult.Rejected()` — and widened the
production code onto both, but did **not** widen the matcher.

- `r.Status.Known()` still trips the matcher: `r.Status` is itself a `SelectorExpr` named `Status`.
- **`r.Rejected()` does not.** Its only selector is named `Rejected`. `Status` never appears.

Why that specific blind spot is the dangerous one: the two predicates are **not complements**
(verified against quarry's source, table in "What was tested" above). `Rejected()` is
`r.Status == ""`, so it is **false** for an unrecognized non-empty status, whereas `!Known()` is
**true** for it. A new consumer written the natural way —

```go
func newConsumer(r quarry.ResolveResult) bool {
    if r.Rejected() {
        return false // quarry could not read the target
    }
    return true      // ... so it resolved
}
```

— fails **OPEN** for exactly the vocabulary-widening case the tripwire exists to catch, and ships with
the whole suite green and no allowlist entry demanded. That is the same defect class as
fable-high-r10's F1 (`delete-not-done` passing an unreadable answer as a successful deletion), which
is what motivated the tripwire in the first place.

CONFIRMED by reading the matcher; `TestStatusHitsIn_UnrelatedSelectorNotCaught`
(`status_enforcement_test.go:172-188`) already pins that a non-`Status` selector produces zero hits,
which is precisely the behavior that lets `Rejected` through.

Suggested fix: widen `statusHitsIn` to match `Status`, `Known` and `Rejected` selectors, keeping the
one allowlist and the seeded self-tests; add a seeded self-test whose fixture reads `r.Rejected()` so
the widening is proven to fire. Update the file's own doc comment to name all three spellings.

### F4 — the real `quarry.DeltaGit` → `DetectDrift` seam has no test at any tier (MEDIUM, CONFIRMED)

`internal/planglyph/drift_integration_test.go`, `internal/planglyph/drift_test.go`,
`internal/planglyph/delta_integration_test.go`.

`renameCardPairs` (`drift.go:40-53`) normalizes a declared Rename pair's New side through
`resolveKeyFor` specifically because "the plan format REQUIRES a symbol Rename pair's New side to be
a `plan:` handle … while quarry's delta reports the new symbol under its bare glyph. Comparing the
two verbatim can therefore never match … which silently turned every declared rename into detected
drift." That is gate one, and it is the single most consequential string comparison in the file.

Both sides of that comparison are only ever exercised against **hand-written** values:

- every `DetectDrift` test (`drift_test.go`, `drift_integration_test.go`) constructs
  `quarry.GitDeltaAnswer{…Renamed: []quarry.RenamedPair{{From: quarry.Symbol{ID: "sub#Old"}, To: quarry.Symbol{ID: "sub#New"}}}}`
  by hand — the spelling under test is supplied by the test itself;
- `delta_integration_test.go` is the only test that calls the real `Delta`, and it asserts only that
  `answer.Created` contains `sub#Foo`. It never produces a `Renamed` pair, and never feeds its answer
  into `DetectDrift`.

So nothing anywhere proves that quarry's *real* `RenamedPair.To.ID` spelling matches what
`resolveKeyFor` derives from a canonicalized handle. I proved it live this round (L8/L10 above), but
a change on either side of that seam — quarry's ID rendering, or `resolveKeyFor` — regresses every
declared rename into a false drift auto-repair with nothing to catch it. Given that this exact class
of break already bit a prior round, the gap matters.

Suggested fix: an integration-tier test that builds a real git repository with a real rename commit,
calls `planglyph.Delta` for the real answer, and drives `DetectDrift` with it twice — once with the
rename declared by a card (gate one: no finding, no rewrite, no amendment) and once undeclared
(exact tier: rewrite + exactly one amendment). `delta_integration_test.go`'s `deltaFixtureRepo`
helper already builds the git fixture; this is composing two existing helpers, not new machinery.

### F5 — `doc.go`'s "canonical list" of Check IDs omits a raiser this very refactor added (LOW, CONFIRMED)

`internal/planglyph/doc.go:24-57` opens: "Every resolve-backed `Finding.Check` ID this package can
raise, **the canonical list a caller checks a claim against without reading Go source**".

Commit `7c50e1a2f` added a fourth `glyph-rejected` raiser — `resolveContainment`'s new fail-closed
`default:` arm (`internal/planglyph/containment.go:148-158`) — and did not update the list. `doc.go`
attributes `glyph-rejected` to `resolve.go`, `create.go` and `donecheck.go` only (lines 29-35 and
55-57), while its containment bullet (lines 38-39) names `containment-file-overlap` alone.

A reader taking `doc.go` at its word concludes `containment.go` raises exactly one Check ID. It
raises two. This is small, but the doc's value is entirely in being exhaustive, and the repo's own
Documentation Lifecycle rule (`CLAUDE.md`) requires the module doc to move in the same change as the
behavior — which this refactor did not do for its own new finding site.

Suggested fix: add `glyph-rejected` to the containment bullet with the same one-line rationale the
`create.go` and `donecheck.go` mentions carry.

### F3 — `create-already-exists` hazard reported as an "unrecognized resolve status" (LOW, CONFIRMED live)

`internal/planglyph/create.go:168-182` routes `quarry.StatusAmbiguous` into the `default:` arm and
renders it through `unreadableStatusDetail` (`internal/planglyph/resolve.go:138-143`), whose
non-rejection branch is hardcoded to the words *"answered the unrecognized resolve status"*.

`ambiguous` is one of quarry's four **documented, recognized** statuses (`quarry.Statuses`;
`Status.Known()` returns **true** for it). The routing is deliberate and correct — create.go's own
comment explains that an ambiguous Create target means an existing declaration already occupies
that name, i.e. the `create-already-exists` hazard — but the operator-facing sentence asserts the
opposite of what is true, and points the operator at a quarry-vocabulary problem instead of at
their own plan.

Live repro (exact, from L6 above): with `sub#Keep` declared in two files,

```
**Create:**
- `plan:sub#Keep` -> `func Keep() {}`
```
→ `glyph-rejected/1-create-card[blocking]: Create target "plan:sub#Keep" answered the unrecognized resolve status "ambiguous"`

An operator reading that has been told quarry returned something lyx cannot read. Nothing in the
message names the actual problem (a declaration of that name already exists, in several places), and
nothing points at the remedy (pick a different name, or make this an Edit card).

Suggested fix: give `StatusAmbiguous` its own `case` in `createFindings`, raising
`create-already-exists` with a detail naming the candidates — the same disposition the found/multipart
arm already has, which is what create.go's own comment says the situation actually is. That keeps the
fail-closed `default:` arm for genuinely unreadable answers and stops it from lying about a status
quarry documents. `unreadableStatusDetail` then only ever renders answers that really are unreadable.

`manifest/designs/quarry-glyph-plan-alphabet.md`'s Create-inversion paragraph enumerates
`found`/`multipart`, `not_found` + `unit: found`, and `not_found` + `unit: not_found` — and is
**silent on `ambiguous`**, which is precisely why the code's disposition for it has no documented
home to be checked against. The doc gets the missing row in the same change.

## What I could NOT verify, and why

Stated plainly rather than left implicit.

- **Windows path behavior** — unreachable from this Linux host, as the prompt anticipated. Named,
  never executed. Nothing in the two refactors is path-shaped in an OS-dependent way
  (`classifyRef` is pure ASCII string analysis; `Glyph.UnitPath` is quarry's), so I have no
  specific concern, but I did not run it.
- **A real pre-resolution rejection reaching a live gate** — I tried (L11) and established it is
  *structurally unreachable* through both `validate-plan` and `record-batch`, for the reason given
  above. This is a genuine finding about reachability, not a skipped test.
- **A `quarry.Open`/`Resolve` infrastructure failure mid-validate** — not driven live. Reaching
  `ErrQuarryUnavailable` from the CLI requires making quarry fail, which on this host means
  corrupting or removing the worktree mid-call; the disposition is already covered by unit tests at
  every one of its four raise sites, and neither refactor touched it. Named as not-driven rather
  than claimed.
- **`lyx webster record-batch` under a real hub + Fabric pair** — I drove it in standalone mode with
  `--plan-dir`. The code path from `RecordBatch` through `DoneChecks`/`BindHandles`/`DetectDrift` is
  identical in both modes (`postBatchChecks` takes a `Geometry`, and only `AnchorRoot` differs); the
  hub-only difference is `fabricSync`, which is downstream of every mechanic under test.
- **No LLM subprocess was spawned at any point**, and none was needed — consistent with the
  campaign's cost declaration. I did not find any reason the mission requires the full LLM-driven
  phase machine. The one place I expected to: `record-batch`'s fork audit reads Claude Code session
  transcripts, but it accepts empty `.jsonl` stubs, so the audit is satisfiable without a provider.
