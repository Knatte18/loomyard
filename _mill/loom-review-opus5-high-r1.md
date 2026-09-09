# `loom` crucible review — round 1, tag `opus5-high-r1`

> Independent clean-room review + fix of the two behavior-preserving refactors
> (`centralize-glyph-shape-enum`, `quarry-bump-v0-2-0-status-helpers`) driven LIVE through the real
> built `cmd/lyx` binary. Per `_mill/loom-review-prompt.md`.
> Clean-room constraint honored: nothing under `_mill/loom-review-*` was opened before this file's
> own findings list was complete.

## Status

- Job 1 (review): IN PROGRESS — notes appended as each command/scenario returns.
- Job 2 (fix): not started.

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

## Findings

(Appended provisionally as formed; severity ordering finalized last.)

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
