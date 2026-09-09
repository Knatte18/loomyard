# Discussion: Bump quarry to v0.2.0 and adopt Status.Known()/Rejected()

```yaml
task: Bump quarry to v0.2.0 and adopt Status.Known()/Rejected()
slug: quarry-bump-v0-2-0-status-helpers
status: discussing
parent: main
```

## Problem

`internal/planglyph` consumes `quarry.ResolveResult.Status`, a closed four-value vocabulary (`found`, `not_found`, `ambiguous`, `multipart`) plus the zero value that marks quarry's pre-resolution rejection of the target string itself.
Every consumer in the package hand-rolls its own guard against that vocabulary: literal four-case switches, a `r.Status == ""` rejection test, and booleans derived from status comparisons.
Two separate crucible rounds (`opus-high-r9` R9-6 and `fable-high-r10` F1) each found one call site that failed OPEN on an unreadable answer and fixed it in isolation, and `status_enforcement_test.go` now exists purely as an AST tripwire forcing every future `.Status` reader through review — the tripwire is the standing admission that the guard is duplicated per call site rather than owned in one place.

**Why now:** quarry released v0.2.0 carrying exactly the fail-closed primitive round 10 of `crucible-loom-glyph-hardening` was circling before it was deferred out of `centralize-glyph-shape-enum`'s scope: `Status.Known()`, `ResolveResult.Rejected()`, and the exported `Statuses` enumeration.
lyx's `go.mod` still pins `github.com/Knatte18/quarry v0.1.0`, so none of it is reachable.
The bump plus a targeted adoption pass replaces the hand-rolled vocabulary guard with quarry's own, and — critically — lets a widening of quarry's vocabulary be caught at build time by an enumeration-completeness test instead of silently at runtime.

## Scope

**In:**

- Bump `go.mod`'s `github.com/Knatte18/quarry` pin from `v0.1.0` to `v0.2.0` (and the resulting `go.sum` update).
- `internal/planglyph/donecheck.go` — replace `doneCheckVerdicts`' literal four-case vocabulary guard with `r.Status.Known()`.
- `internal/planglyph/resolve.go` — replace `unreadableStatusDetail`'s `r.Status == ""` test with `r.Rejected()`.
- A new enumeration-completeness test in `internal/planglyph` driven by `quarry.Statuses`, so a future widening of quarry's vocabulary fails the build rather than passing silently through `Known()`.
- Comment/prose updates at the sites that hand-enumerate the four values, naming `Known()` / `quarry.Statuses` as the source of truth.

**Out:**

- `internal/quarrycli` — its `describeRejectedResolve` (`internal/quarrycli/resolve.go:80`) tests `r.Error != ""` where `r.Rejected()` is the correct spelling; recorded as a follow-up below, deliberately not changed here (see Decision: scope-planglyph-only).
- `internal/planparser` — imports quarry for glyph/handle grammar, never reads `.Status`.
- `internal/planglyph/handle.go` — `renameDeclSource`'s `r.Status != quarry.StatusFound` is Found-only by design (documented at `handle.go:86-91`), not a vocabulary guard; `CanonicalizeHandles`' `res.Error != ""` at `handle.go:222` is on `quarry.NameResult`, a different type with no `Rejected()` method.
- The `statusFindings` / `createFindings` / `resolveContainment` switch *structures* — each already gives every status a distinct disposition behind a `default` arm; only their prose changes.
- `status_enforcement_test.go`'s allowlist — no new `.Status` consumer function is introduced, so the allowlist entries are unchanged.
- `manifest/roadmap.md`, `CONSTRAINTS.md`, `docs/overview.md`, `manifest/designs/*` — no doc changes (see Decision: no-doc-changes).
- Any behavioral change to what findings lyx emits for the four readable statuses or for the zero value.

## Decisions

### known-in-donecheck-only

- Decision: adopt `Status.Known()` at exactly one site — `doneCheckVerdicts` (`internal/planglyph/donecheck.go:162-171`), whose `switch r.Status { case StatusFound, StatusMultipart, StatusAmbiguous, StatusNotFound: /* fallthrough to the booleans */ default: glyph-rejected }` is a pure vocabulary guard and nothing else.
  It becomes `if !r.Status.Known() { ...glyph-rejected finding...; continue }` ahead of the `resolved`/`stillExists` booleans.
- Rationale: that switch's four `case` values carry no per-status behavior — every one of them falls through to the same two booleans below. It is a hand-rolled `Known()` and nothing more, so the swap is behavior-preserving today and removes the duplicated enumeration.
- Rejected: restructuring `statusFindings` (`resolve.go:81`), `createFindings` (`create.go:150`) and `resolveContainment` (`containment.go:132`) the same way. Each of those gives `found`/`multipart`, `ambiguous`, and `not_found` genuinely different dispositions, so a `Known()` pre-guard would sit in front of a switch that still needs every case spelled out — dead code plus a second place to keep in sync. Their `default` arms already deliver the identical fail-closed outcome.
- Rejected: leaving `doneCheckVerdicts` literal too (i.e. bumping the pin and adopting nothing). That makes the bump pointless and leaves the duplication the task exists to remove.

### statuses-completeness-test

- Decision: add a test in `internal/planglyph` that ranges `quarry.Statuses` and asserts each enumerated value receives an explicit, named disposition from `doneCheckVerdicts`, alongside cases for the zero value and a synthetic out-of-vocabulary status.
  A status quarry adds later, which `Known()` would newly admit but `doneCheckVerdicts`' `resolved`/`stillExists` booleans have never been taught, must make this test fail.
- Rationale: `Known()` moves the definition of "readable" from lyx to quarry. That is the point of the primitive, but it means a widened vocabulary now passes lyx's guard and lands on booleans derived only from `StatusFound`/`StatusMultipart`/`StatusNotFound` — a new status would silently read as `resolved == false, stillExists == true`, producing a plausible-looking `create-not-done` / `delete-not-done` verdict instead of the honest `glyph-rejected` diagnostic. Ranging the newly-exported `Statuses` converts that runtime fail-open into a build-time failure, and it is the one place `Statuses` earns its export.
- Rejected: keeping the literal four-case switch as the mitigation (Q1 option 2). That preserves fail-closed behavior but keeps the duplication, and it leaves quarry's vocabulary and lyx's copy of it free to drift with nothing detecting it.
- Rejected: accepting the hazard undetected. The whole `status_enforcement_test.go` tripwire exists because this exact family of defect shipped twice already.

### rejected-in-unreadable-detail

- Decision: `unreadableStatusDetail` (`internal/planglyph/resolve.go:139`) swaps `if r.Status == ""` for `if r.Rejected()`.
- Rationale: it is the only `ResolveResult` pre-resolution-rejection test in the package, and quarry v0.2.0 made the identical swap in its own renderer (`quarry/text.go`), so the two sides read the same. `Rejected()`'s doc comment records why `Status == ""` — not `Error != ""` — is the primary marker, which is exactly the invariant this function relies on.
- Rejected: also touching `handle.go:222`. That `res.Error != ""` is on `quarry.NameResult` from `quarry.Name`, which has no `Rejected()` method; the resemblance is superficial.

### scope-planglyph-only

- Decision: confine production changes to `internal/planglyph` and `go.mod`/`go.sum`. Record `internal/quarrycli`'s `describeRejectedResolve` as a follow-up, do not change it here.
- Rationale: the task brief names `internal/planglyph` as the audit surface. `quarrycli` renders quarry's answer verbatim to the operator rather than making a plan/gate decision from it — `status_enforcement_test.go`'s own doc comment already explains why it sits outside this validation surface.
- Follow-up recorded (not this task): `internal/quarrycli/resolve.go:80` spells the rejection test `r.Error != ""`. quarry's `Rejected()` doc explicitly names that spelling as the rejected alternative — it "would read false for a rejection whose message happened to be empty, where `Status == ""` still holds", in which case `describeRejectedResolve` falls through and prints an empty status. Cosmetic today, one line to fix, worth its own small task.
- Rejected: folding the `quarrycli` one-liner into this commit. It is a real (if minor) correctness deviation, but widening a bump-and-adopt commit past its stated surface is the operator's call, not this task's.

### comment-prose-updates

- Decision: update the prose that hand-enumerates the four values so it names `Known()` / `quarry.Statuses` as the source of truth — `doneCheckVerdicts`' guard comment (`donecheck.go:154-161`) and `status_enforcement_test.go`'s file doc comment. Leave `allowedStatusConsumers` itself unchanged.
- Rationale: the comments currently assert lyx's own four-value enumeration as the standard. After the swap that is quarry's job, and a comment that still recites the list is the next thing to drift.
- Rejected: leaving the comments. `status_enforcement_test.go`'s doc comment explicitly instructs a future author to write "a switch with a default arm, or a boolean derived only after a vocabulary guard"; `r.Status.Known()` is now the canonical spelling of that guard and the instruction should say so.
- Note: no `.Status` *consumer function* is added or renamed, so `allowedStatusConsumers`' six `(file, function)` pairs stay exactly as they are. `unreadableStatusDetail` keeps its entry even though its body no longer names `.Status` directly — `r.Rejected()` is not a `Status` selector, so the AST tripwire simply stops hitting that function; an unused allowlist entry is harmless and removing it would only invite re-adding it later.

### no-doc-changes

- Decision: no changes to `manifest/roadmap.md`, `CONSTRAINTS.md`, `docs/overview.md`, or `manifest/designs/`.
- Rationale: per `CLAUDE.md`, the roadmap moves only on completing or adding a planned item — this is a hardening/polish pass, covered by git history. No new cross-cutting invariant is introduced (the `.Status` tripwire already exists and is unchanged in kind). No module table or execution-stack change, and no observable CLI behavior change. `manifest/designs/quarry-glyph-plan-alphabet.md` is marked **Done** and defers as-built detail to each package's own documentation, which is where these comment updates land.
- Rejected: adding a `CONSTRAINTS.md` entry for "vocabulary guards go through `Known()`". The `status_enforcement_test.go` tripwire plus the new completeness test enforce it mechanically; a prose constraint would add a third place to keep in sync for no added enforcement.

### bump-mechanics

- Decision: `go get github.com/Knatte18/quarry@v0.2.0` followed by `go mod tidy`; commit both `go.mod` and `go.sum`.
- Rationale: hand-editing `go.mod` leaves `go.sum` stale and the build broken.
- Verified during exploration: the v0.1.0 → v0.2.0 diff across the `quarry` facade package and `internal/engine/answer.go` is **purely additive** — `Statuses`, `Status.Known()`, `ResolveResult.Rejected()` added; no exported identifier removed, no signature changed, no struct field removed or renamed. Every one of the 29 `quarry.*` identifiers lyx names (`Declaration`, `DeltaAnswer`, `DirAnswer`, `ExpandAnswer`, `GitDeltaAnswer`, `GlyphsAnswer`, `GlyphsOptions`, `ModifiedSymbol`, `Name`, `Open`, `RenameCandidate`, `RenameCandidateEntry`, `RenamedPair`, `RenameSignals`, `RenderExpandJSON`, `RenderGlyphsJSON`, `RenderJSON`, `RenderResolveJSON`, `Repo`, `Resolve`, `ResolveResult`, `Status`, `Status{Found,NotFound,Ambiguous,Multipart}`, `Symbol`, `TOC`, `TOCOptions`) is still present at v0.2.0. The bump is therefore expected to be a no-op for every call site except the ones this task deliberately changes.
- Rejected: waiting for a quarry release that also carries the `quarrycli`-side fix. Nothing is pending there; the deviation is on lyx's side.

## Technical context

**The dependency.** `go.mod:6` pins `github.com/Knatte18/quarry v0.1.0`. v0.2.0 is published and resolvable from the module proxy (`go list -m -versions` confirms both). The new API, as it reaches lyx through the `quarry` facade package:

- `var quarry.Statuses = []Status{StatusFound, StatusNotFound, StatusAmbiguous, StatusMultipart}` — the engine's own slice, not a copy. Note quarry's own doc warns it is a mutable exported slice; the completeness test ranges it, and nothing in lyx may mutate it.
- `func (s quarry.Status) Known() bool` — a literal switch over the four constants with a `default: return false`. False for the empty string, deliberately: the empty string is the pre-resolution rejection marker, not a resolution outcome. quarry explicitly rejected implementing it by ranging `Statuses`, for the mutability reason above.
- `func (r quarry.ResolveResult) Rejected() bool { return r.Status == "" }` — reads `Status`, not `Error`, because the struct's documented rule is that `Status` and `Error` are never both set.

**The change sites.**

- `internal/planglyph/donecheck.go:162-171` — `doneCheckVerdicts`' vocabulary guard, immediately before `resolved := r.Status == StatusFound || r.Status == StatusMultipart` and `stillExists := r.Status != StatusNotFound` (lines 180-181). Those two booleans stay exactly as they are; only the guard above them changes. The `glyph-rejected` finding it emits on the `default` arm (check ID `glyph-rejected`, `SeverityBlocking`, detail from `unreadableStatusDetail("done-check target", e.display, r)`, then `continue`) must be preserved verbatim.
- `internal/planglyph/resolve.go:132-145` — `unreadableStatusDetail(noun, target string, r quarry.ResolveResult) string`, the single renderer shared by all four fail-closed policies. Its `r.Status == ""` branch becomes `r.Rejected()`; both format strings are unchanged.

**Deliberately untouched, for the plan's benefit.**

- `internal/planglyph/resolve.go:81-125` `statusFindings` — `found`/`multipart` pass silently, `ambiguous` → `glyph-ambiguous` listing `r.Candidates` IDs, `not_found` → `glyph-not-found` with detail branching on `r.Unit`, `default` → `glyph-rejected`. Per-status behavior throughout.
- `internal/planglyph/create.go:150-181` `createFindings` — `found`/`multipart` → `create-already-exists`, `not_found` with `r.Unit == StatusNotFound` → informational `create-new-unit`, `default` (which deliberately swallows `ambiguous`, documented in place) → `glyph-rejected`.
- `internal/planglyph/containment.go:132-158` `resolveContainment` — `found`/`multipart` collect `r.Symbols` files, `not_found`/`ambiguous` skip silently, `default` → `glyph-rejected`.
- `internal/planglyph/handle.go:94` `renameDeclSource` — `r.Status != quarry.StatusFound`, Found-only by design; `handle.go:86-91` documents why `multipart` is excluded (a multipart answer would mean arbitrarily choosing `r.Symbols[0]` to rename from).

**The two AST tripwires in the package.** Both parse the package's own production `.go` files via `runtime.Caller(0)` and must keep passing:

- `status_enforcement_test.go` — flags any `ast.SelectorExpr` named `Status` outside `allowedStatusConsumers`. The `Known()` swap keeps the selector inside `doneCheckVerdicts` (already allowlisted); the `Rejected()` swap removes the last `Status` selector from `unreadableStatusDetail` (also already allowlisted, so no failure either way).
- `chokepoint_enforcement_test.go` — pins every `Resolve` call to `resolveTargets` and every `quarry.Name` call to `CanonicalizeHandles`. Untouched by this task; named here so the plan does not accidentally introduce a call site.

**Test seam.** `doneCheckVerdicts(entries []doneCheckEntry, index map[string]quarry.ResolveResult) ([]Finding, error)` was split out of `DoneChecks` specifically so unreachable-through-a-real-`quarry.Repo` conditions can be unit-tested (documented at `donecheck.go:135-137`). It takes a caller-supplied `map[string]quarry.ResolveResult`, so a synthetic `ResolveResult` carrying any `Status` string — including one outside the vocabulary — can be fed straight in. `donecheck_test.go` already exercises this seam; the new completeness test belongs alongside it.

**Build prerequisite.** Per `CLAUDE.md`, lyx links quarry's tree-sitter grammars through cgo: building and testing requires `CGO_ENABLED=1` and a C compiler on `PATH`.

## Constraints

From `CONSTRAINTS.md` (only the ones this task can touch):

- **Told-Geometry Invariant** — `internal/planglyph` is a bound package: it is handed absolute paths and derives none of its own, and must not import `internal/lyxcwd`. This task adds no path handling, so it is satisfied by not regressing.
- **Ref-Shape Registry Invariant** — the kind-policy registry (`internal/planparser/shape.go`) and the exported handle vocabulary are the only ways `planparser`/`planglyph` act on a ref's shape. Untouched: this task is about `Status`, not ref shape.
- Package-local invariant, enforced by `status_enforcement_test.go`: every production `.Status` read in `internal/planglyph` sits inside an allowlisted function and fails closed. Preserved — no new consumer function, no new `.Status` read outside the six allowlisted pairs.
- Package-local invariant, enforced by `chokepoint_enforcement_test.go`: one `Resolve` call site (`resolveTargets`) and one `quarry.Name` call site (`CanonicalizeHandles`). Untouched.
- Discovered during discussion: `quarry.Statuses` is a mutable exported slice shared with the engine — read it, never assign into it, and never derive a runtime predicate from it (quarry itself rejected doing so in `Known()`).

## Testing

`internal/planglyph` is the only package with test changes. TDD is a natural fit for the completeness test: it can be written and made to fail against the current literal switch before the `Known()` swap lands.

**TDD candidate — vocabulary completeness (new test, `internal/planglyph`).**
Drive `doneCheckVerdicts` through its exported-for-test seam with a synthetic `index map[string]quarry.ResolveResult`, once per entry in `quarry.Statuses`, plus two extra cases: the zero-value `Status` (`""`, quarry's pre-resolution rejection, with `Error`/`Reason` populated) and a synthetic out-of-vocabulary status string. Assert:

- every value in `quarry.Statuses` produces a verdict from the create/delete rules and never a `glyph-rejected` finding;
- the zero value and the out-of-vocabulary string both produce exactly the `glyph-rejected` finding, with the detail `unreadableStatusDetail` renders for each (the two branches differ);
- the test's own coverage is keyed off `len(quarry.Statuses)` rather than a hard-coded 4, so a widened vocabulary that nobody taught `doneCheckVerdicts` fails here.

**Regression scenarios that must stay green (existing tests in `donecheck_test.go`, `donecheck_integration_test.go`).**

- `create-not-done` fires for `not_found` and for `ambiguous`, and does not fire for `found`/`multipart`.
- `delete-not-done` fires for `found`, `multipart` **and** `ambiguous` (the `fable-high-r10` F1 fix — `stillExists` is `!= not_found`, so ambiguous blocks a deletion verdict), and does not fire for `not_found`.
- A pre-resolution rejection (`Status == ""`) yields `glyph-rejected`, not a create/delete verdict — the fail-closed behavior both crucible rounds installed.
- An index missing a key the caller asked about still returns the `ErrQuarryUnavailable` infrastructure error, unchanged.

**`unreadableStatusDetail` rendering.** Existing assertions on the two detail strings (rejected-before-resolution vs unrecognized-status) must be unchanged by the `Rejected()` swap — that is the proof the swap is behavior-preserving.

**Tripwires.** `TestStatusEnforcement_NoOutOfAllowlistConsumer` and the `chokepoint_enforcement_test.go` pins must pass without allowlist edits.

**Verify command.** `CGO_ENABLED=1 go build ./... && CGO_ENABLED=1 go test ./...` — the full suite, not just `./internal/planglyph/...`: the dependency bump touches every package that imports quarry (`internal/planparser`, `internal/quarrycli`, `cmd/lyx`), and the point of the additive-only API check is confirming those compile and pass untouched.

## Q&A log

- **Q:** Which `internal/planglyph` sites should adopt `Status.Known()`? **A:** [auto-pick] `doneCheckVerdicts` only. **Why:** its four-case switch has no per-status behavior — every case falls through to the same two booleans — so it is a hand-rolled `Known()`; the other three switches give each status a distinct disposition and would keep every case anyway.
- **Q:** `Known()` moves the definition of "readable" from lyx to quarry, so a future fifth status would pass the guard and be mis-read by `resolved`/`stillExists`. How is that mitigated? **A:** [auto-pick] add a `quarry.Statuses`-driven completeness test that fails the build on a widened vocabulary. **Why:** it converts a silent runtime fail-open into a build-time failure and is the one place the newly-exported `Statuses` earns its keep; keeping the literal switch instead would preserve safety but leave quarry's vocabulary and lyx's copy free to drift undetected.
- **Q:** Where does `ResolveResult.Rejected()` apply? **A:** [auto-pick] `unreadableStatusDetail`'s `r.Status == ""` only. **Why:** it is the package's only `ResolveResult` rejection test, and quarry v0.2.0 made the identical swap in its own `text.go`; `handle.go:222`'s `res.Error != ""` is on `NameResult`, which has no such method.
- **Q:** Should the audit extend past `internal/planglyph`? **A:** [auto-pick] no — planglyph only, with `quarrycli`'s deviation recorded as a follow-up. **Why:** the brief names planglyph as the audit surface and `quarrycli` renders quarry's answer verbatim rather than gating on it; widening a bump-and-adopt commit is the operator's call.
- **Q:** What about prose that hand-enumerates the four values? **A:** [auto-pick] update `doneCheckVerdicts`' guard comment and `status_enforcement_test.go`'s file doc comment to name `Known()`/`quarry.Statuses` as the source of truth; leave `allowedStatusConsumers` alone. **Why:** a comment reciting a list that is no longer lyx's to own is the next thing to drift; no consumer function is added or renamed, so the allowlist itself is correct as-is.
- **Q:** Testing approach? **A:** [auto-pick] table test over `quarry.Statuses` plus the zero value and a synthetic unknown, through the existing `doneCheckVerdicts` seam. **Why:** that seam exists precisely so conditions unreachable through a real `quarry.Repo` can be unit-tested.
- **Q:** Bump mechanics? **A:** [auto-pick] `go get ...@v0.2.0` then `go mod tidy`, committing `go.mod` and `go.sum`. **Why:** hand-editing `go.mod` leaves `go.sum` stale; the v0.1.0→v0.2.0 diff was verified purely additive, so no other call site should need touching.
- **Q:** Any doc updates? **A:** [auto-pick] none. **Why:** hardening/polish does not move `manifest/roadmap.md` per `CLAUDE.md`; no new cross-cutting invariant, no module-table change, no observable CLI behavior change.
