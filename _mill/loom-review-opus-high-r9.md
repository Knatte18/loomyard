# `loom` — round 9 independent review (`opus-high-r9`) — glyph-plan-format surface only

> Filled instance of the round-9 prompt `_mill/loom-review-prompt.md`.
> Worktree: `/home/knatte/Code/loomyard/wts/crucible-loom-glyph-hardening`, branch `crucible-loom-glyph-hardening`, HEAD at review start `464516014`.
> Clean-room: every finding below was formed by reading the live code and the pinned spec ONLY.
> No `_mill/loom-review-*` file — no prior round's review, no fixer report, no `loom-review-HANDOFF.md` — was opened before this findings list was complete and committed.

## Executive summary

**Verdict on convergence of the glyph-plan-format surface: NOT converged.**
Round 9 found 8 findings — 1 BLOCKING, 2 MEDIUM, 3 LOW, 2 NIT — every one of them genuinely new, and the BLOCKING one is a live-confirmed wedge that makes a whole, spec-admitted ref class unrecordable.

The round-9 prompt's own hypothesis was that PG-1/PG-2 were instances of "a specific enumerated case was missed in an otherwise-general mechanism", and asked whether they were the last two of that shape or the first two of a batch.
They were the first two of a batch. Every finding below is that same shape:

- **R9-1** (BLOCKING): the shape classifier's own **rule 4** — the extensionless repository-root filename (`LICENSE`, `Makefile`, `Dockerfile`), a rule `classify.go` documents as "required rather than tidy" — is admitted by the classifier and by all 28 pure checks, but is never canonicalized to a glyph, so every glyph-backed layer downstream hands quarry a string it rejects **pre-resolution**. A `Create` card creating `LICENSE` validates 100% clean and then can *never* be recorded done.
- **R9-2** (MEDIUM): `checkRenamePairShape` enumerates the offending shapes POSITIVELY on both of its halves, and both enumerations omit `refKindPath`.
- **R9-3** (MEDIUM): `handle-collision` counts only `Create` declarations while `handle-dangling` already counts `Rename` to-sides as declarations, so a handle claimed by both sources is silently, arbitrarily rewritten by `CanonicalizeHandles`.
- **R9-4/R9-5/R9-6** (LOW): a last-wins map in `renameCardPairs`, an over-broad `handle-malformed` file-unit rule that misdiagnoses a `Rename` to-side, and a fail-OPEN `switch` over `quarry.ResolveResult.Status` in `createFindings`.
- **R9-7/R9-8** (NIT): a round-8 off-by-one cross-reference in the pinned spec, and a `ReadDir` fired before its own guard.

Round 8's own two fixes (PG-1's `glyph-malformed`, PG-2's `BindHandles` Rename path) are, in themselves, **correct and complete within their own dimension** — see "High-yield focus item 1/3" below.
What round 9 found are their siblings in *adjacent* dimensions, not regressions in them.

### High-yield-focus items — what was attempted, how far each got

| Item | Attempted | Outcome |
|---|---|---|
| 1. Adversarially re-examine PG-1/PG-2 for siblings of the same gap shape | Yes, in full | **Found 3.** PG-1's sibling is R9-1 (a ref class invisible to the glyph layers, exactly PG-1's "validates clean, wedges at the done-check" shape) and R9-2 (a positively-enumerated shape switch that leaks one `refKind`). PG-2's sibling is R9-3 (the second handle-declaration source `handle-collision` does not count). PG-1's own new `glyph-malformed` check is correctly scoped (`Targets`+`Uses`, card-generic, language-gated — byte-for-byte the same scoping as `bare-symbol-target`); PG-2's `cardOwnHandles` covers exactly the two handle sources the format admits (`Create` declarations + `Rename` to-sides), and no other `parseTypeLabelCase` branch can own a handle. |
| 2. Full adversarial pass over the 28-check pipeline and the resolve/containment/drift machinery | Yes, in full | **Found 4** (R9-1's `prosa-symbol-target` half, R9-2, R9-4, R9-6). Every one of the 28 pure checks was read against the pinned spec row-by-row; all 28 IDs and both entry-point splits agree across `validate.go`'s package comment, `doc.go`, `contracts/specs/loom-plan-spec.md`, `contracts/stencils/loom/loom-rubric-plan-review.md` and `contracts/recipes/loom-recipe.yaml` — the only disagreement found is R9-7, a cross-reference row number. `internal/planglyph/doc.go`'s enumerated resolve-backed ID list was mechanically diffed against every `Check:` literal in the package: exactly 16 IDs, complete, no drift. |
| 3. `BindHandles` × `DetectDrift`'s rename-pair gate, across every way a rename can be declared | Yes, in full | **Found 2** (R9-4, R9-5). Drove all four shapes the prompt named: a **rename chain** is structurally inexpressible and the spec says so explicitly (`loom-plan-spec.md` "A `Rename`'s `Old` side can never be a symbol the same plan creates") — not a defect; **two Rename cards on the same symbol** is R9-4; a **Rename whose New side collides with an existing symbol** is a real coverage asymmetry recorded as an out-of-scope-of-fix observation below (OBS-1); and the **file-rename** path is correct. `BindHandles`' own `bound()` two-source acceptance is correct given quarry may report a rename as `Renamed` or as `Created`+`Deleted`. |
| 4. General adversarial pass over the rest of the surface | Yes | **Found 2** (R9-5, R9-8). `RewriteRefs`' surface-lexeme bridge, the arrow-bullet collapse rule, `syntacticContainment`, `resolveContainment`, `ScopeGuard`, `targetCards`/`writingTargetCards`, `renameSignature`'s receiver-clause rule, `draftHandleIdentifier`'s owner strip, `AppendAmendment`, `extractPlanSections` and the frontmatter/index parsers were all read adversarially with nothing further found. |

### Merge bar

The merge bar is correctness in the NORMAL single-instance flow. **R9-1 fails that bar as it stands**: an ordinary plan whose `Create` card creates a repository-root extensionless file (`LICENSE`, `Makefile`, `Dockerfile`, `CODEOWNERS`) is unrecordable, with the failure appearing only at the record-batch boundary and naming the wrong cause.
With R9-1 through R9-8 fixed, the surface meets the bar.

---

## Findings

Severity-ranked. `CONFIRMED` means proven by reading the exact code path end to end, and — where marked — by driving the real substrate.

### R9-1 — BLOCKING — CONFIRMED (live-confirmed)

**A rule-4 extensionless repository-root filename validates 100% clean and is then rejected by quarry pre-resolution in every glyph-backed layer that consumes it.**

`internal/planparser/classify.go:70-74` (rule 4), `internal/planparser/normalize.go:118-129` + `146-167` (`hasFileExtension` / `canonicalizeCard`), `internal/planglyph/donecheck.go:59-126` (`DoneChecks`), `internal/planparser/validate.go:970-1006` (`checkProsaSymbolTarget`).

`classifyRef` rule 4 deliberately admits a slash-free, extensionless token (`LICENSE`, `Makefile`, `Dockerfile`) as `refKindPath`, and its own doc comment states the rule is "required rather than tidy — without it, such a filename would fall to rule 5's `refKindSymbol` with no legal spelling left".
`canonicalizeCard`, however, gates canonicalization on `hasFileExtension`, so such a ref is **never** turned into its self glyph and stays the bare token `LICENSE` in the parsed model.
Every pure check then passes it: `bare-symbol-target` skips it (wrong shape), `directory-target` skips it (no `/`), `card-path-malformed` finds it clean, `path-missing` finds it on disk, `glyph-malformed` skips it (no `#`). The plan validates 100% clean.

Downstream, three consumers hand that bare token to quarry, which rejects it **before resolution** (`ResolveResult.Status == ""`, `Error` set):

1. `DoneChecks` (`donecheck.go:74-108`) puts `resolveKeyFor(ref)` — the bare `LICENSE` — into the batched resolve for every `Create`, `Delete` and `Rename` group ref. `doneCheckVerdicts` (`donecheck.go:149`) computes `resolved := r.Status == StatusFound || r.Status == StatusMultipart`, which is **false for a rejection**. So:
   - a `**Create:**` group ref `LICENSE` yields a blocking `create-not-done` **on a card that did create the file**, permanently — the batch can never be recorded, on every retry, forever;
   - a `**Rename:**` pair endpoint spelled that way yields a blocking `rename-not-done` for the same reason;
   - a `**Delete:**` group ref `LICENSE` yields `delete-not-done` **never firing**, i.e. it silently auto-passes whether or not the file was removed.
2. `checkProsaSymbolTarget` (`validate.go:981-985`) requires `parseGlyph(lang, t)` to succeed and `IsSelf()` to report true under a glyph-enabled language. `parseGlyph(glyph.Go, "LICENSE")` fails (`ReasonNoSeparator`), so a `**Prosa:**` group targeting `LICENSE` is a **false** blocking `prosa-symbol-target` — the very spelling rule 4 exists to make legal.
3. `collectGlyphTargets` drops it (correctly, by construction), so no resolve-backed finding names it either.

**Live confirmation**, driven in this worktree against the real quarry engine (zero LLM cost):

```
$ lyx quarry resolve LICENSE
{"error":"LICENSE: glyph: parse \"LICENSE\" as go: a glyph needs a \"#\"; a path is
 addressed as its own glyph by appending one to its repository-relative form (LICENSE#)","ok":false}
$ lyx quarry resolve 'LICENSE#'
{"target":"LICENSE#","id":"LICENSE#","status":"found","listing":{"dir":".","files":[{"name":"LICENSE"}]}}
```

`LICENSE` exists in this repository. The bare spelling is a pre-resolution rejection; the self-glyph spelling resolves `found`.
Bare root directories behave the same way and their self glyphs also resolve `found` (`docs#`, `internal#`, `contracts#`, `tools#` all verified `status: found`).

**This is PG-1's exact failure shape, one ref class over.** PG-1's own rationale — "the plan would validate 100% clean while carrying a target no execution engine can ever act on, discovered only deep into a batch's own done-check, not at Plan-Validate up front" — describes R9-1 word for word.

**Suggested fix.** Narrow `canonicalizeCard`'s gate rather than patching three consumers: canonicalize a path-shaped ref that carries a file extension **or** carries no `/` at all, and leave only the *slashed* extensionless ref (`internal/foo`) as a plain path.
That preserves `hasFileExtension`'s entire stated purpose — it is load-bearing solely so `directory-target` still has something to classify, and `directory-target` **only ever fires on a ref containing `/`** (`validate.go:459`), so the slashed case is untouched.
It fixes all three symptoms at the root, it is the spelling quarry itself recommends in its own rejection message, and it needs no new check ID.
Docs: `contracts/specs/loom-plan-spec.md`'s canonicalization paragraph and its `directory-target` row, plus the `hasFileExtension`/`classifyRef` rule-4 comments.

### R9-2 — MEDIUM — CONFIRMED

**`checkRenamePairShape` enumerates the offending shapes positively on both halves, and both enumerations omit `refKindPath` — so a path-shaped Rename endpoint escapes both checks the format's own contract exists to enforce.**

`internal/planparser/validate.go:747-788`.

```go
if k := classifyRef(p.New); k == refKindGlyph || k == refKindSymbol {   // rename-to-not-handle
if k := classifyRef(p.Old); k == refKindSymbol || k == refKindHandle {  // rename-from-not-glyph
```

`classifyRef` returns one of four kinds. For a pair `isFileRenamePair` has already declined to exempt, the contract is absolute — `loom-plan-spec.md`: "the `Old` side must be a glyph … and the `New` side must be a `plan:` handle" — so the correct predicates are `New != refKindHandle` and `Old != refKindGlyph`.
Written as positive enumerations, `refKindPath` falls through **both**.

Concrete scenario (with R9-1 unfixed, and with `root:` absent or `.`): a card carrying

```markdown
**Rename:**
- `internal/a#Old` -> `LICENSE`
```

produces **no** `rename-to-not-handle` finding, no `rename-from-not-glyph` finding, no `directory-target` finding (no `/`), no `bare-symbol-target` finding, and `path-missing` never checks a `Rename` group's `New` side by design — so the pair is entirely unvalidated and surfaces only as a `rename-not-done` at the record-batch boundary.
The mirror case, `` `LICENSE` -> `plan:internal/a#X` ``, likewise escapes `rename-from-not-glyph`.

Fixing R9-1 removes the *most reachable* trigger (rule-4 tokens become glyphs), but it does not close the enumeration: a path-shaped ref whose `glyph.Self` conversion fails — an extension-carrying path containing a space or a backslash, e.g. `my file.go` — stays `refKindPath` after canonicalization (`normalize.go:151-153` leaves it untouched on a `glyph.Self` error) and leaks through both halves exactly the same way. The two fixes are independent and both are needed.

**Suggested fix.** Invert both predicates to fail closed: for a non-file-rename pair, `New` must classify `refKindHandle` and `Old` must classify `refKindGlyph`; anything else is the finding. Keep the existing shape-naming detail text and extend it to name the path shape.

### R9-3 — MEDIUM — CONFIRMED

**`handle-collision` counts only `Create` declarations, while `handle-dangling` already treats a `Rename` to-side as a declaration — so a draft handle claimed by both sources produces two competing canonical forms that `CanonicalizeHandles` resolves by silent map overwrite, rewriting the `Create` card's own declaration to the rename's destination.**

`internal/planparser/validate.go:553-563` (`renameToHandles`), `validate.go:599-618` (the `handle-collision` half), `internal/planparser/handle.go:59-67` (`declaredHandles`), `internal/planglyph/handle.go:179-272` (`CanonicalizeHandles`).

The plan format has **two** sources that bring a handle into existence, and `handle-dangling` already knows it: `checkHandleConsistency` treats a handle as declared when `len(declared[handle]) > 0 || renameTo[handle]` (`validate.go:584`).
`handle-collision`, one loop below, ranges over `declaredHandleNames` alone — the `Create`-declaration map — so it cannot see a collision whose second claimant is a `Rename` to-side.

Scenario, entirely legal under all 28 pure checks:

```markdown
# card 1
**Create:**
- `plan:internal/a#Foo` -> `func Foo() error`

# card 2
**Rename:**
- `internal/b#Old` -> `plan:internal/a#Foo`
```

- `handle-collision`: `declared["plan:internal/a#Foo"]` has length 1 → no finding.
- `handle-dangling`: declared → no finding. `handle-unreferenced`: card 2 references it externally → no finding.
- `CanonicalizeHandles` builds **two** `declSource` entries for the one draft handle: card 1's, with `Unit` from `planparser.HandleUnit` (`internal/a`); and card 2's, from `renameDeclSource`, whose `Unit` is deliberately the **resolved Old side's** unit (`internal/b`, `planglyph/handle.go:147`).
- `quarry.Name` answers two different IDs, so `canonicalOwners` holds two distinct canonicals each with exactly **one** owner — `handle-canonical-collision` (which fires on `len(owners) > 1`) does **not** trigger.
- Both then write into the one substitution map keyed by the shared draft handle: `subs[owners[0]] = canonical` (`planglyph/handle.go:271`). The second iteration of the sorted-canonical loop silently overwrites the first.
- `RewriteRefs` then rewrites **card 1's own `Create` declaration bullet** to point at `internal/b#Foo` — the rename's destination — with no finding raised anywhere.

The same hole admits two `Rename` to-sides claiming one handle (`a#X -> plan:z#Y` on one card, `b#X -> plan:z#Y` on another): `declared` is empty, `handle-collision` cannot fire, and the identical overwrite occurs whenever the two derived units differ.

`BindHandles`' own comment (`planglyph/handle.go:346-350`) asserts "the same handle spelling could not legitimately appear in both a `Declarations` entry and a `Rename` pair on one card, so this never masks a real mismatch" — an assertion nothing currently enforces.

**Suggested fix.** Count the union for the collision half only: build the per-occurrence claim list as `Create` declarations **plus** every `Rename` pair's handle-shaped `New` side, and flag when one handle carries more than one claim.
Do **not** fold `Rename` to-sides into `declaredHandles` wholesale — `handle-unreferenced` reads the same map, and a rename destination that no *other* card references is the ordinary case, so a wholesale merge would fire a false `handle-unreferenced` on essentially every `Rename` card.
Update `loom-plan-spec.md` row 14 and its "Six checks keep this mechanism internally consistent" paragraph, which today scopes all four handle checks to "a `Create`-declared handle".

### R9-4 — LOW — CONFIRMED

**`renameCardPairs` is a last-wins `map[old]new`, so when two declared `Rename` pairs share one `Old` side, `DetectDrift`'s gate one stops recognizing one card's own expected outcome and auto-repairs it as drift.**

`internal/planglyph/drift.go:31-44`, consumed at `drift.go:104`.

```go
pairs[p.Old] = resolveKeyFor(p.New)
```

Gate one is `if want, ok := renamePairs[oldID]; ok && want == newID { continue }` — a single expected `New` per `Old`.
With card 1 declaring `a#X -> plan:a#Y` and card 3 declaring `a#X -> plan:a#Z`, whichever pair the plan-order walk visits last is the only one gate one can match.
When the delta reports the *other* card's rename, gate one misses, the rename is treated as drift, and the exact tier auto-repairs: a plan-wide `RewriteRefs` plus an `AppendAmendment` recording a "repair" of a rename the plan itself declared.
Nothing anywhere flags two cards renaming the same symbol, so this reaches production silently.

**Suggested fix.** Index every declared `New` per `Old` (`map[string][]string`) and pass gate one when **any** declared destination matches. That is the semantically correct question ("is this rename a declared card's own expected outcome?") and needs no new check ID.

### R9-5 — LOW — CONFIRMED

**`handle-malformed`'s file-unit rule fires on a handle that is only a `Rename` to-side, whose unit half is never read — the finding's own stated consequence is false for that case, and it blocks a plan that canonicalizes correctly.**

`internal/planparser/validate.go:696-705`, against `internal/planglyph/handle.go:116-148` (`renameDeclSource`).

The rule's rationale (`validate.go:654-661`) is specific and correct **for a `Create` declaration**: `CanonicalizeHandles` takes that handle's `Unit` from `planparser.HandleUnit(d.Handle)` (`planglyph/handle.go:182`), so a `.go` file unit propagates into `quarry.Name`, which echoes an ID `Resolve` will never answer.

A `Rename` pair's to-side handle takes a different path: `renameDeclSource` sets `Unit: sym.Glyph.Unit` — the **resolved Old side's** package directory — and reads only the bare identifier from the draft handle (`draftHandleIdentifier`). Its own doc comment says so explicitly: "never the draft handle's own unit half, so a draft that misspells the unit is corrected by canonicalization rather than propagated."

So for `` `internal/a#Old` -> `plan:internal/a/x.go#New` `` the machinery works perfectly and the plan is nevertheless refused with a blocking finding asserting "a file-unit member spelling can never resolve" — which is untrue for this handle.

**Suggested fix.** Apply the file-unit rule only to a handle that is not a `Rename`-to-side-only handle: skip it when the handle appears as some `Rename` pair's `New` side and in no card's `Declarations`. A handle claimed by both sources stays flagged (and, after R9-3, is separately a `handle-collision`).

### R9-6 — LOW — CONFIRMED

**`createFindings` and `statusFindings` switch over `quarry.ResolveResult.Status` with no default arm, so an unrecognized — or, in `createFindings`' case, an absent — status passes with no finding. It fails open, and R9-1 proves the absent-status case is reachable.**

`internal/planglyph/create.go:126-144`, `internal/planglyph/resolve.go:98-129`, exclusion at `internal/planglyph/planglyph.go:216-224`.

`createFindings` handles `StatusFound`/`StatusMultipart` and `StatusNotFound` and nothing else. A result carrying **no** `Status` — quarry's pre-resolution rejection, `Error`/`Reason` set — therefore produces no finding at all, and it cannot be caught by `statusFindings`' own `glyph-rejected` branch either, because `resolvePass` deliberately excludes every `createTargets` member from `nonCreateResults` before calling it. A rejected `Create` target is invisible on both sides of that split.

`statusFindings` does handle `Status == ""` explicitly, but its `switch` over the four-value vocabulary still has no default arm, so a status value outside quarry's current closed set would pass silently.

This repository already has the precedent for the opposite disposition: crucible round 6's R6-27 made an unrecognized plan-finding **severity** fail closed rather than open.

**Suggested fix.** Give both switches an explicit default that reports the unrecognized/absent status as a blocking finding under the check ID that already exists for it (`glyph-rejected` semantics), so the vocabulary can only ever be widened deliberately.

### R9-7 — NIT — CONFIRMED

**`contracts/specs/loom-plan-spec.md:285` cross-references `prosa-symbol-target` as "row 24 below"; it is row 25.**

Round 8 inserted `glyph-malformed` as row 11, shifting every later row by one. Row 24 is `impact-summary-multiline`; `prosa-symbol-target` is row 25.
The rest of the round-8 renumbering is correct — all 28 IDs, their order, and the 27/28 entry-point split agree across `validate.go`'s package comment, `planparser/doc.go`, the spec, `loom-rubric-plan-review.md` and `loom-recipe.yaml`.

### R9-8 — NIT — CONFIRMED

**`checkIndexFileConsistency` calls `os.ReadDir(plan.Dir)` before the `plan.Dir == ""` guard that exists to say there is nothing on disk to scan.**

`internal/planparser/validate.go:191-204`. The `switch` reads `entries, err := os.ReadDir(plan.Dir)` and only then tests `case plan.Dir == "":`. Every in-memory `*Plan` (the shape the whole unit-test corpus and every hand-built caller uses) therefore issues a guaranteed-failing `ReadDir("")` syscall whose result is immediately discarded. Harmless, but it inverts the guard's own stated intent and reads as if the empty-`Dir` case were an error branch.

---

## Out-of-scope-of-fix observations (recorded, not acted on)

**OBS-1 — a `Rename` pair's New side is never checked for already existing, where the `Create` inversion checks exactly that.**
`internal/planglyph/create.go:112-150` inverts the resolve verdict for every `Create` group target, handle-shaped ones included, and reports blocking `create-already-exists` when the target already resolves. A `Rename` pair's to-side handle names something equally new, and after `CanonicalizeHandles` its expected glyph is equally resolvable — but it is excluded from `statusFindings` (`renameNewTargetSet`), it is not a `Create` group ref so `createFindings` never sees it, and no other pass looks. A plan renaming `a#Old` onto an already-existing `a#Existing` therefore validates clean; worse, its own `rename-not-done-new` done-check passes trivially, because the destination resolved before the card ever ran.
This is a genuine coverage asymmetry and the natural closing move would be a `rename-to-already-exists` blocking finding in `planglyph`. It is recorded rather than fixed because it **adds a new resolve-backed check ID and a new batched resolve** to a round explicitly narrowed to hardening, not to extending, the surface — and because, unlike R9-1, the failure is caught (late, by a build error in the fork), not silently passed. Recommended as its own small follow-up task.

**OBS-2 — a completed symbol-`Rename` card's on-disk pair trips `rename-to-not-handle` in any WHOLE-plan validation after PG-2's binding lands.**
Once `BindHandles` binds a `Rename` pair's New-side handle, `rewriteBulletLine` (`internal/planparser/rewrite.go:149-166`) writes the pair back as `` - `a#Old` -> `a#New` `` — two glyphs, which `isFileRenamePair` does not exempt (neither is a self glyph), so `rename-to-not-handle` fires on re-parse.
Every mid-execution consumer already scopes this away: `ValidateDispatch` drops findings for completed cards, and `webstercli/validate.go` switches to it as soon as a batch lands. `loomcli`'s `validate-plan` does not scope, but it is a pre-execution parity verb. So this is contained today and is noted only so a future caller does not add an unscoped whole-plan validation mid-run.

**OBS-3 — a slash-free extensionless *directory* at the repository root loses its narrow `prosa-symbol-target` fallback under R9-1's fix.**
`checkDirectoryTarget`'s comment records that such a ref "falls to `path-missing` and, on a `Prosa` group, to `prosa-symbol-target`, narrower coverage than the slashed case and accepted rather than papered over". R9-1's fix canonicalizes it to a unit self glyph (`docs#`), which `prosa-symbol-target` explicitly admits as the legal whole-package spelling. The nudge toward writing `docs#` by hand is lost; the ref itself is valid either way, `path-missing` still checks it, and quarry resolves it `found` (verified). This is a deliberate, documented consequence of R9-1's fix, not a separate regression.

---

## What was tested

All commands run from `/home/knatte/Code/loomyard/wts/crucible-loom-glyph-hardening` on branch `crucible-loom-glyph-hardening`.

### Baseline, before any edit (HEAD `464516014`)

| Command | Result |
|---|---|
| `CGO_ENABLED=1 go build ./...` | clean |
| `CGO_ENABLED=1 go vet ./internal/planparser/... ./internal/planglyph/... ./internal/loomcli/... ./internal/websterengine/... ./internal/webstercli/...` | clean |
| `CGO_ENABLED=1 go test -count=5 ./internal/planparser/... ./internal/planglyph/... ./internal/loomcli/... ./internal/websterengine/... ./internal/webstercli/...` | 5/5 `ok` for all five packages |
| `CGO_ENABLED=1 go test -tags integration ./internal/planparser/... ./internal/planglyph/... ./internal/websterengine/... ./internal/loomcli/... ./internal/webstercli/...` | `ok` for all five packages |

### Live substrate driven (zero LLM cost, foreground, one at a time)

The round's cost declaration allows one live confirmation where a finding genuinely needs it. R9-1 needed one — the whole finding turns on what quarry actually answers for a bare path — so quarry's own resolve verb was driven directly through `go run ./cmd/lyx`, which is strictly cheaper than `lyx webster validate` and involves no `claude` subprocess, no tmux session and no fixture.

| Command | Answer | What it establishes |
|---|---|---|
| `go run ./cmd/lyx quarry resolve LICENSE` | `{"error":"LICENSE: glyph: parse \"LICENSE\" as go: a glyph needs a \"#\" …","ok":false}` | A rule-4 extensionless root filename is a **pre-resolution rejection** (`Status == ""`), for a file that exists. R9-1's core. |
| `go run ./cmd/lyx quarry resolve 'LICENSE#'` | `status: found`, listing `dir: "."`, file `LICENSE` | The self-glyph spelling resolves. R9-1's fix direction is the spelling quarry itself recommends in its own rejection text. |
| `go run ./cmd/lyx quarry resolve Makefile` | pre-resolution rejection (no `Makefile` in this repo) | The rejection is a grammar verdict on the target string, independent of disk existence. |
| `go run ./cmd/lyx quarry resolve 'docs#' / 'internal#' / 'contracts#' / 'tools#'` | all `status: found` | A bare root **directory** canonicalized to a unit self glyph resolves `found`, so R9-1's fix introduces no new `glyph-not-found` for the directory case (OBS-3). |

No tmux session, no `lyx` daemon, no `claude` subprocess and no fixture hub was started at any point in this round, so there is no substrate to tear down. Confirmed at the end of the round.

### Reading performed (Job 1)

- `internal/planparser/`: `classify.go`, `glyphref.go`, `handle.go`, `plan.go`, `parse.go`, `normalize.go`, `containment.go`, `validate.go` (all 28 checks, line by line, against the spec), `rewrite.go`, `amendment.go`, `sections.go`, `doc.go`.
- `internal/planglyph/`: `planglyph.go`, `resolve.go`, `create.go`, `containment.go`, `handle.go`, `drift.go`, `donecheck.go`, `scope.go`, `delta.go`, `repo.go`, `doc.go`.
- Consumer boundary, read to establish reachability only: `internal/websterengine/recordbatch.go`'s `postBatchChecks`, and the caller inventory of every `planglyph` entry point across `internal/` and `cmd/`.
- Upstream contract, read to establish ground truth rather than to review: `github.com/Knatte18/quarry@v0.1.0`'s `glyph/{parse,golang,self,unitpath,glyph}.go`, `quarry/{name,quarry,repo}.go`, `internal/engine/name.go`, and `ResolveResult`'s own field contract.
- Docs: `contracts/specs/loom-plan-spec.md` (in full), `manifest/designs/quarry-glyph-plan-alphabet.md`, `contracts/stencils/loom/loom-rubric-plan-review.md`, `contracts/recipes/loom-recipe.yaml`, `CLAUDE.md` (root + global), `CONSTRAINTS.md` (in full).

---

## Fix log

Filled in during Job 2. See `_mill/loom-review-opus-high-r9-fixer-report.md` for the full per-finding table.
