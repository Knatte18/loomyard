# `loom` — round 9 fixer report (`opus-high-r9`) — glyph-plan-format surface only

> Companion to `_mill/loom-review-opus-high-r9.md`.
> Worktree: `/home/knatte/Code/loomyard/wts/crucible-loom-glyph-hardening`, branch `crucible-loom-glyph-hardening`.
> Review report committed at `021b21848`, before any production or test file was touched.
> No push was performed.

## Result

**8 findings, 8 fixed, 0 deferred.** Every severity, NIT included.
Every fix landed as its own commit with its own regression test and its own doc update in the same commit.

| Severity | Found | Fixed | Deferred |
|---|---|---|---|
| BLOCKING | 1 | 1 | 0 |
| MEDIUM | 2 | 2 | 0 |
| LOW | 3 | 3 | 0 |
| NIT | 2 | 2 | 0 |
| **Total** | **8** | **8** | **0** |

## Per-finding table

Every row was written as its fix landed, not reconstructed at the end.

| Commit | Finding | Severity | What changed, and why |
|---|---|---|---|
| `e455e817d` | R9-1 | BLOCKING | `canonicalizeCard`'s eligibility gate moved from `hasFileExtension` alone to the new `canonicalizablePath` (`internal/planparser/normalize.go`), which composes the extension test with a slash-free test. `classifyRef` rule 4 admits an extensionless repository-root filename (`LICENSE`, `Makefile`, `Dockerfile`) as a card ref and its own doc calls the rule required rather than tidy — but the ref then stayed a bare token through all twenty-eight pure checks and was handed verbatim to quarry, which rejects it BEFORE resolution. `DoneChecks` read the rejection as "did not resolve", so a `Create` card creating `LICENSE` was blocked by `create-not-done` permanently, on every retry; a `Delete` of the same ref auto-passed silently; and `checkProsaSymbolTarget` reported a false `prosa-symbol-target`. `directory-target`'s own load-bearing gate is preserved exactly — it only ever fires on a ref containing `/`, which the new gate leaves as a plain path. |
| `71beac1de` | R9-2 | MEDIUM | `checkRenamePairShape` (`internal/planparser/validate.go`) rewritten from a positive enumeration of forbidden shapes to the negation of the one admitted shape on both halves, plus the new `refKindName` so a finding says what the offending side actually is. `classifyRef` returns four kinds and both enumerations omitted `refKindPath`, so a pair such as `` `internal/a#Old` -> `LICENSE` `` drew no finding from either check, none from `directory-target` (no `/`), none from `bare-symbol-target` (wrong shape), and `path-missing` never checks a Rename `New` side by design. |
| `ec49d92dd` | R9-3 | MEDIUM | New `handleClaims` (`internal/planparser/handle.go`) indexes BOTH handle-declaring sources — a `Create` sub-bullet's declaration and a `Rename` pair's own to-side — with the source tagged; `handle-dangling` and `handle-collision` now key on it. `handle-collision` counted `Create` declarations alone while `handle-dangling` already accepted either source, so a handle claimed by both passed clean and `CanonicalizeHandles` then derived two DIFFERENT canonical glyphs for it (a declaration's unit comes from the handle, a rename to-side's from the resolved old side), wrote both into one substitution map keyed by the shared draft handle, and let the later one rewrite the `Create` card's own declaration bullet. `handle-unreferenced` deliberately still keys on `declaredHandles` alone. |
| `a7cf0d40d` | R9-4 | LOW | `renameCardPairs` (`internal/planglyph/drift.go`) changed from `map[old]new` to `map[old][]new`, and `DetectDrift`'s gate one now passes when ANY declared destination matches. Two cards declaring a rename of the same old symbol are legal under every plan-format check and reported by none; the last-wins map made gate one miss the other card's declared outcome, so the exact tier auto-repaired a declared rename as drift — a plan-wide `RewriteRefs` plus a spurious amendment. |
| `0303cc731` | R9-5 | LOW | `handle-malformed`'s file-unit rule gated on the new `fileUnitRuleApplies` (`internal/planparser/handle.go`). The rule's rationale — quarry's `Name` echoes a file-unit member ID its own `Resolve` never answers — binds a handle whose unit half is actually read, which is a `Create` declaration's. A `Rename` to-side's unit comes from the RESOLVED old side (`renameDeclSource`), deliberately, so the rule refused a plan that would have canonicalized correctly with a detail asserting something untrue of it. An unclaimed handle keeps the rule. |
| `8588d52a9` | R9-6 | LOW | `createFindings` (`internal/planglyph/create.go`) gained a `default` arm reporting blocking `glyph-rejected`, and `statusFindings` (`internal/planglyph/resolve.go`) had its pre-switch `Status == ""` test folded into the switch's own `default`, both sharing the new `unreadableStatusDetail` renderer. A `Create` target is excluded from `statusFindings` by `resolvePass`, which owns the only other reader of a no-`Status` pre-resolution rejection, so such a result had no reader on either side of the split and passed the Create inversion in silence — where silence reads as "does not exist yet, carry on". Same fail-closed disposition R6-27 settled for an unrecognized plan-finding severity. |
| `8af4a81a7` | R9-7 | NIT | `contracts/specs/loom-plan-spec.md` row 11's cross-reference to `prosa-symbol-target` corrected from "row 24" to "row 25". Round 8's insertion of `glyph-malformed` as row 11 shifted every later row by one; row 24 is `impact-summary-multiline`. |
| `7a93f4eb8` | R9-8 | NIT | `checkIndexFileConsistency` (`internal/planparser/validate.go`) now runs its `plan.Dir == ""` guard BEFORE the `os.ReadDir` it guards. Reading first and testing afterwards issued a guaranteed-failing `ReadDir("")` for every in-memory `*Plan` and read as if the empty-`Dir` case were an error branch. |

## Tests added or extended

Every bug got a test that would have caught it. Three of the fixes were additionally **mutation-checked** — the fix reverted, the new test confirmed failing, the fix restored — rather than only confirmed green.

| Test | File | Covers |
|---|---|---|
| `TestCanonicalizeCard_ExtensionGate` (rewritten; two new sub-tests) | `internal/planparser/normalize_test.go` | R9-1's parser half: a slash-free extensionless ref canonicalizes (`Makefile` -> `Makefile#`), including through the `//` worktree-root escape under a non-`.` `root:`, while a slashed extensionless directory still does not. |
| `TestCanonicalizeCard_RootFilenameSurfaceRefRecorded` (new) | `internal/planparser/normalize_test.go` | R9-1: the surface lexeme is recorded under the new canonical string, so `RewriteRefs` still restores the card file's own byte-form. |
| `TestValidate_RootFilenameCanonicalizesEndToEnd` (new) | `internal/planparser/validate_test.go` | R9-1 end to end through a real `ParsePlan`: a `Prosa` card targeting `LICENSE` and a `Create` card targeting `Makefile` produce ZERO findings, where the Prosa half was previously a false `prosa-symbol-target`. |
| `TestDoneChecks_RootFilenameCreateLanded` (new, `integration`) | `internal/planglyph/donecheck_integration_test.go` | R9-1's planglyph half against a real quarry repo: the canonicalized spelling passes, and the bare token is pinned as still producing the false `create-not-done` — so the parser-side canonicalization is provably the thing keeping this correct. |
| `TestValidate_RenamePairShape` (two new sub-tests) | `internal/planparser/validate_test.go` | R9-2: a path-shaped `New` side is `rename-to-not-handle`; a path-shaped `Old` side is `rename-from-not-glyph`. |
| `TestValidate_HandleConsistency` (three new sub-tests) | `internal/planparser/validate_test.go` | R9-3: one `Create` declaration plus one `Rename` to-side claiming one handle is a collision; two `Rename` to-sides claiming one handle is a collision; and a lone `Rename` to-side handle is none of dangling/collision/unreferenced — the regression guard against folding rename to-sides into `handle-unreferenced`. |
| `TestDetectDrift_GateOneSeesEveryDeclaredDestinationForOneOldSide` (new) | `internal/planglyph/drift_test.go` | R9-4. **Mutation-checked**: against pre-fix `drift.go` it fails exactly as the finding describes — card 3's ref rewritten, an `amendments.md` created, and three spurious blocking `glyph-not-found` findings. |
| `TestCheckHandleMalformed_FileUnitRuleScopedToDeclarations` (new) | `internal/planparser/handle_test.go` | R9-5: a rename-to-side-only file-unit handle draws no finding; one claimed by both sources still draws one per referencing card; an unclaimed one still draws one. |
| `TestCreateFindings_UnreadableStatusFailsClosed` (new) | `internal/planglyph/create_test.go` | R9-6: a pre-resolution rejection and an out-of-vocabulary status each produce a blocking `glyph-rejected` from the Create inversion. |
| `TestStatusFindings_UnrecognizedStatusFailsClosed` (new) | `internal/planglyph/resolve_test.go` | R9-6's sibling on the status policy. |

## Documentation updated (same commit as its fix)

| Doc | Fix | Change |
|---|---|---|
| `contracts/specs/loom-plan-spec.md` | R9-1 | Shape-classifier rule 4 gained the canonicalization consequence; the `directory-target` hard-rule bullet restated for the slash-free case; the "Ordering is load-bearing" canonicalization paragraph rewritten around the two-half eligibility rule, naming the failure it prevents. |
| `contracts/specs/loom-plan-spec.md` | R9-2 | Rows 17 and 18 restated as the negation of the admitted shape, with an explicit note that no classifier shape escapes them. |
| `contracts/specs/loom-plan-spec.md` | R9-3 | Row 14 restated over both declaring sources with the silent-overwrite failure named; the "Six checks keep this mechanism internally consistent" paragraph gained the per-check source-set split. |
| `contracts/specs/loom-plan-spec.md` | R9-5 | Row 16 gained the file-unit rule's scoping to `Create`-declared handles. |
| `contracts/specs/loom-plan-spec.md` | R9-7 | Row 11's cross-reference corrected. |
| `internal/planparser/doc.go` | R9-1 | The two canonicalization sentences retargeted from "extension-carrying" to `canonicalizablePath`. |
| `internal/planparser/classify.go` | R9-1 | Rule 4's doc comment now carries the canonicalization half of the rule. |
| `internal/planparser/normalize.go` | R9-1 | `hasFileExtension` reduced to what it is; `canonicalizablePath` carries the full rationale for both halves and the deliberate loss of the `prosa-symbol-target` nudge for a bare root directory. |
| `internal/planglyph/doc.go` | R9-6 | The enumerated `glyph-rejected` bullet now names the Create inversion's fail-closed arm as its second producer. |
| `internal/planglyph/handle.go` | R9-3 | `BindHandles`' two-source-acceptance comment no longer asserts an unenforced invariant; it now names the check that enforces it. |

No entry was added to `manifest/roadmap.md` — this round is hardening, which the roadmap explicitly does not track.
`CONSTRAINTS.md` needed no change: no new cross-cutting invariant was introduced, and every fix stays inside the Planparser Sole-Parser and Glyph Conversion Chokepoint invariants exactly as they already stood (no new `glyph.Self` call site, no new glyph→path call, no local glyph regex).

## Changed files

Production:

- `internal/planparser/classify.go`
- `internal/planparser/normalize.go`
- `internal/planparser/handle.go`
- `internal/planparser/validate.go`
- `internal/planparser/doc.go`
- `internal/planglyph/create.go`
- `internal/planglyph/resolve.go`
- `internal/planglyph/drift.go`
- `internal/planglyph/handle.go`
- `internal/planglyph/doc.go`

Tests:

- `internal/planparser/normalize_test.go`
- `internal/planparser/validate_test.go`
- `internal/planparser/handle_test.go`
- `internal/planglyph/create_test.go`
- `internal/planglyph/resolve_test.go`
- `internal/planglyph/drift_test.go`
- `internal/planglyph/donecheck_integration_test.go`

Docs:

- `contracts/specs/loom-plan-spec.md`

Reports:

- `_mill/loom-review-opus-high-r9.md`
- `_mill/loom-review-opus-high-r9-fixer-report.md`

## Gates, at the end of the round

| Command | Result |
|---|---|
| `CGO_ENABLED=1 go build ./...` | clean |
| `CGO_ENABLED=1 go vet ./internal/planparser/... ./internal/planglyph/... ./internal/loomcli/... ./internal/websterengine/... ./internal/webstercli/...` | clean |
| `CGO_ENABLED=1 go test -count=5 ./internal/planparser/... ./internal/planglyph/... ./internal/loomcli/... ./internal/websterengine/... ./internal/webstercli/...` | 5/5 `ok`, all five packages |
| `CGO_ENABLED=1 go test -tags integration ./internal/planparser/... ./internal/planglyph/... ./internal/websterengine/... ./internal/loomcli/... ./internal/webstercli/...` | `ok`, all five packages |
| `CGO_ENABLED=1 go test ./...` | whole repo clean |

Each of these was additionally run to green after every individual fix, before that fix was committed.

## Substrate teardown

No tmux session, no `lyx` daemon, no `claude` subprocess and no fixture hub was started at any point in this round.
The only live driving performed was `go run ./cmd/lyx quarry resolve <target>`, a synchronous, zero-LLM-cost process that exits on its own.
`ps aux | grep -iE 'tmux|lyx|claude'` at the end of the round lists only processes that predate it (the user's own Claude Desktop, and three `claude` sessions started 2026-08-31 through 2026-09-05).
Zero stray processes attributable to this round.

## Deferred

None. Every finding, all severities, was fixed.

Two items were deliberately recorded as observations rather than fixed, and both are stated in full in the review report's "Out-of-scope-of-fix observations" section:

- **OBS-1** — a `Rename` pair's `New` side is never checked for already existing, where the `Create` inversion checks exactly that. Closing it means a new resolve-backed check ID plus a new batched resolve, which is extending the surface rather than hardening it; and unlike R9-1 the failure is caught (late, by a build error in the fork) rather than silently passed. Recommended as its own small follow-up task.
- **OBS-2** — a completed symbol-`Rename` card's on-disk pair trips `rename-to-not-handle` in any WHOLE-plan validation once `BindHandles` has bound its New-side handle. Contained today by `ValidateDispatch`'s completed-card scoping, which every mid-execution consumer already uses; recorded so a future caller does not add an unscoped whole-plan validation mid-run.

**OBS-3** is not a deferral: it is the deliberate, documented consequence of R9-1's fix, recorded so a later reader does not mistake it for a regression.
