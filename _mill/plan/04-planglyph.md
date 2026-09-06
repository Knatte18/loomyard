# Batch: planglyph

```yaml
task: "Adopt quarry's glyph alphabet as the plan alphabet"
batch: "planglyph"
number: 4
cards: 7
verify: go test ./internal/planglyph/ ./internal/planparser/ ./internal/lyxcwd/
depends-on: [3]
```

## Batch Scope

This batch creates `internal/planglyph`, the sole owner of every `quarry.Repo` call and of the resolve-backed validation pass, and gives it the two entry points the Gate Self-Check Parity Invariant will name in batch 5: `planglyph.ValidateFormat(plan, worktreeRoot)` and `planglyph.Validate(plan, worktreeRoot)`.
It is one batch because every card here is a finding family layered onto the same composed entry point — a second batch boundary inside it would ship a package whose exported pass answers only half the question it claims to answer.
The external interface batches 5, 6 and 7 consume is those two entry points plus the `openRepo` seam card 16 introduces, which batch 7 extends with the `DeltaGit` call site.

Batch-local decisions, beyond `## Shared Decisions`:

- **Composition, not duplication.** `planglyph.ValidateFormat` calls `planparser.ValidateFormat` and appends resolve findings; `planglyph.Validate` calls `planparser.Validate` and appends the same set. No check is implemented twice, so the two entry points cannot drift and parity holds by construction rather than by convention.
- **One batched `Resolve`, one batched `Name`.** Every glyph the plan references is resolved in a single `(*quarry.Repo).Resolve` call, and every `Create` declaration is named in a single `quarry.Name` call. Neither ever runs inside a planning loop.
- **Under `language: none` both entry points still run** and simply skip the resolve pass, so the parity pair is one function in every mode rather than branching by plan language.

## Cards

### Card 16: create internal/planglyph and its composed validation entry points

- **Context:**
  - `internal/planparser/validate.go`
  - `internal/planparser/plan.go`
  - `internal/planparser/glyphref.go`
  - `internal/loomshed/planvalidate.go`
  - `CONSTRAINTS.md`
- **Edits:** none
- **Creates:**
  - `internal/planglyph/doc.go`
  - `internal/planglyph/repo.go`
  - `internal/planglyph/planglyph.go`
  - `internal/planglyph/repo_test.go`
  - `internal/planglyph/planglyph_test.go`
  - `internal/planglyph/testmain_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Create the package and its two exported entry points.
  In `doc.go`, document the package as the sole owner of every `quarry.Repo` call and of the resolve-backed validation pass, state that it derives no path of its own and never imports `internal/lyxcwd`, and state that it composes `internal/planparser`'s pure checks rather than reimplementing any of them.
  In `repo.go`, add `openRepo(worktreeRoot string) (*quarry.Repo, error)` as the package's one call to `quarry.Open`, wrapping its error with a `planglyph:` prefix, and `resolveTargets(repo *quarry.Repo, targets []string) ([]quarry.ResolveResult, error)` as the package's one call to `(*quarry.Repo).Resolve`.
  `quarry.Open` takes an absolute root, which is exactly why the root is told rather than derived: the caller supplies `worktreeRoot`, mirroring `planparser.Validate(plan, worktreeRoot)`'s own shape.
  Add `Finding` as this package's own finding type carrying the same `Check`, `Card` and `Detail` fields `planparser.ValidationError` carries plus a `Severity` field whose values are `blocking` and `informational`, and a conversion from `planparser.ValidationError` that stamps `blocking`.
  In `planglyph.go`, add `ValidateFormat(plan *planparser.Plan, worktreeRoot string) []Finding` calling `planparser.ValidateFormat(plan, worktreeRoot)`, converting its findings, and appending the resolve-backed findings; and `Validate(plan *planparser.Plan, worktreeRoot string) []Finding` doing the same over `planparser.Validate`.
  Factor the resolve-backed half into one unexported `resolvePass(plan, worktreeRoot)` both call, so the two exported functions differ only in which pure function they delegate to — that single shared body is what makes the parity pair one function in every mode.
  When `plan.Language` is `none`, `resolvePass` returns nil immediately without opening a repository, so `language: none` degrades to today's path-only behaviour with no quarry call at all. No file read needed.
  Collect the plan's distinct glyph refs across every card's `Targets`, `Uses` and both `Pairs` endpoints into one sorted, deduplicated slice and issue exactly one `resolveTargets` call for it; card 17 consumes the results.
  In `testmain_test.go`, add a `TestMain` calling `gitkit.HermeticGitEnv()` before `m.Run()`, per the Hermetic Git Test Environment Invariant, because batch 7 adds `integration`-tagged tests to this package that spawn git.
  Cover in tests: `ValidateFormat` returning every `planparser.ValidateFormat` finding unchanged apart from the severity stamp; `Validate` additionally returning the `plan-unapproved` finding; `language: none` producing no quarry call, asserted by pointing the pass at a non-repository directory and observing success; and the batched-resolve collection deduplicating a glyph referenced by two cards into one target.
- **Commit:** `16: feat(planglyph): add the package and its composed validation entry points`

### Card 17: apply the resolve status policy

- **Context:**
  - `internal/planglyph/repo.go`
  - `internal/planglyph/planglyph.go`
  - `internal/planparser/plan.go`
  - `internal/planparser/classify.go`
- **Edits:** none
- **Creates:**
  - `internal/planglyph/resolve.go`
  - `internal/planglyph/resolve_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Turn each `quarry.ResolveResult` into the finding its status calls for, mirroring quarry's own contract wording rather than inventing a policy.
  Add `statusFindings(plan *planparser.Plan, results []quarry.ResolveResult) []Finding` applying four rules by `ResolveResult.Status`: `found` passes with no finding; `multipart` **passes** with no finding, because it marks a legitimate single symbol the language lets be declared in several places — rejecting it would reject a Go package's several `func init()` and every C# partial type as defects, which the contract says they are not; `ambiguous` is a blocking finding with check ID `glyph-ambiguous` listing every entry of `ResolveResult.Candidates` by its `ID`, because downgrading it to a warning would let a plan proceed against a target nobody has picked; and `not_found` is a blocking finding with check ID `glyph-not-found` whose detail **branches on `ResolveResult.Unit`** — a `Unit` of `found` means the unit is there and only the member is missing, so the message names a misspelled member, while a `Unit` of `not_found` means the message names a misspelled unit.
  A result carrying no `Status` at all is a pre-resolution rejection carried by `ResolveResult.Error` and `ResolveResult.Reason` instead; report it as check ID `glyph-rejected`, blocking, with both fields in the detail.
  Attribute each finding to the card that references the target: build the target→cards index once from the plan and emit one finding per referencing card, so a glyph two cards name produces a finding on each rather than one unattributed finding.
  This card deliberately does not special-case a `Create` group's targets — the inversion for those is card 18's, and it must be layered on top rather than woven in here, so the ordinary policy stays readable on its own.
  Cover in tests, table-driven against a small fixture repository built under `t.TempDir()`: `found` clean, `multipart` clean, `ambiguous` listing its candidates, `not_found` with `unit: found` producing the member-misspelled message, `not_found` with `unit: not_found` producing the unit-misspelled message, and a rejection carrying `Error`/`Reason`.
- **Commit:** `17: feat(planglyph): apply quarry's four-status resolve policy as findings`

### Card 18: invert the verdict for Create groups

- **Context:**
  - `internal/planglyph/resolve.go`
  - `internal/planglyph/planglyph.go`
  - `internal/planparser/plan.go`
- **Edits:** none
- **Creates:**
  - `internal/planglyph/create.go`
  - `internal/planglyph/create_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Apply the per-group inversion a `Create` group requires, and surface the one case the inversion would otherwise hide.
  Add `createFindings(plan *planparser.Plan, results []quarry.ResolveResult) []Finding` operating over every `planparser.TargetGroup` whose `Type` is `planparser.CardTypeCreate`.
  A `Create` group's targets must resolve `not_found`, and **both** a `Unit` of `found` and a `Unit` of `not_found` pass: demanding `unit: found` is not merely stricter but incoherent in Go, because a package does not exist apart from its files, so the separate unit-creating card it would require has nothing to target — its own unit being equally absent — and the regress never grounds.
  Creating a package is creating its first symbol.
  A `found` or `multipart` verdict on a `Create` target is the blocking finding, check ID `create-already-exists`, whose detail names the target and the status that contradicts it.
  Because `unit: not_found` is ambiguous between a deliberate new package and a typo in the unit path, add the rider that keeps it honest: every `Create` group whose target resolves `not_found` with a `Unit` of `not_found` produces an **informational** finding, check ID `create-new-unit`, reading that this card introduces a new unit and naming it, so each new package name appears explicitly in the validation report and a misspelled unit cannot silently create a package nobody intended.
  Arrange the composition so a `Create` group's targets are excluded from card 17's `statusFindings` and handled only here — otherwise the same target would produce both a `glyph-not-found` finding and a pass, which is a contradiction in one report.
  A `plan:` handle in a `Create` group is never sent to `Resolve` at all: quarry never sees a handle, and card 19 owns handles.
  Note in the file comment that this check also discharges the plan spec's own card-types table obligation for `Create` — "none — check nothing equivalent exists first" — which had no mechanical implementation before.
  Cover in tests: a `Create` target that resolves `found` producing `create-already-exists`; one that resolves `multipart` producing the same; one that resolves `not_found` with `unit: found` producing no finding at all; one that resolves `not_found` with `unit: not_found` producing exactly one informational `create-new-unit`; and a handle target producing neither and never reaching `Resolve`.
- **Commit:** `18: feat(planglyph): invert the resolve verdict for Create groups and flag new units`

### Card 19: canonicalize Create handles through one batched Name call

- **Context:**
  - `internal/planglyph/repo.go`
  - `internal/planglyph/planglyph.go`
  - `internal/planparser/handle.go`
  - `internal/planparser/plan.go`
  - `internal/planparser/rewrite.go`
- **Edits:** none
- **Creates:**
  - `internal/planglyph/handle.go`
  - `internal/planglyph/handle_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Turn each draft handle into its canonical `plan:<expected-glyph>` form, mechanically, at the validation boundary and never inside the planning loop.
  Add `CanonicalizeHandles(plan *planparser.Plan, planDir string) ([]Finding, error)` doing three things in order.
  First, collect every `planparser.CardDeclaration` across every card into one `[]quarry.Declaration`, taking `Unit` from `planparser`'s own handle-unit accessor and `Decl` from the declaration head verbatim, and issue **one** `quarry.Name` call for the whole slice. No file read needed.
  `quarry.Name` is positional and always returns a slice the same length as its input, with `Unit` and `Target` echoing the input verbatim, `ID` and `Kind` set only on success, and `Error` and `Reason` set only on failure — so match results to declarations by index and verify the echo, and let one bad declaration fail its own entry rather than the batch. No file read needed.
  Second, emit a blocking finding, check ID `handle-name-failed`, for each result carrying a non-empty `Error`, with `Reason` in the detail.
  Third, build the substitution map from each successful result's draft handle to `planparser.HandlePrefix + result.ID` and hand it to `planparser.RewriteRefs(planDir, subs)`, which rewrites every occurrence across the whole plan in one pass — the draft spelling on the left, the canonical spelling on the right.
  A collision between two successful results' canonical forms is a blocking finding, check ID `handle-canonical-collision`, and suppresses the rewrite for both entries so the plan is never left half-rewritten.
  Never call `quarry.Name` from a planning-loop caller and never expose it through the CLI: `name` in an agent's hands is a glyph-spelling machine, which is the one thing the hard rule exists to prevent.
  Under `plan.Language` `none` this function returns nil findings and performs no call and no rewrite. No file read needed.
  Cover in tests: one batched call covering three declarations; positional matching verified by an out-of-order fixture; one failing declaration producing exactly one `handle-name-failed` and leaving the other two rewritten; a canonical collision producing `handle-canonical-collision` and rewriting neither; and the rewrite landing on every card that referenced the draft handle, not only the declaring card.
- **Commit:** `19: feat(planglyph): canonicalize Create handles via one batched quarry.Name call`

### Card 20: add the resolve-backed tier of the containment check

- **Context:**
  - `internal/planglyph/resolve.go`
  - `internal/planglyph/planglyph.go`
  - `internal/planparser/containment.go`
  - `internal/planparser/plan.go`
- **Edits:** none
- **Creates:**
  - `internal/planglyph/containment.go`
  - `internal/planglyph/containment_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Add the half of the containment check that string prefixes cannot see, reporting its own finding ID distinct from the syntactic tier's.
  A card targeting a member glyph and a card targeting a file self glyph overlap physically when the member lives in that file, and there is no string equality between the two refs — so no DAG edge, blind parallel dispatch, and a merge conflict.
  A package's symbols are spread across its files, which is exactly why the syntactic tier cannot answer this and why the motivating example needs the resolve answer.
  Add `resolveContainment(plan *planparser.Plan, results []quarry.ResolveResult) []Finding` emitting check ID `containment-file-overlap`: for each member glyph's `ResolveResult`, read the owning file from the answer — `ResolveResult.Symbols` carries a `File` per symbol, filled by `Resolve` because its entries span files — and match it against every file self glyph on every other card.
  Take the file from the answer rather than deriving it, so no glyph↔path conversion happens here.
  A `multipart` result carries several symbols and therefore several files; report the overlap when any of them matches, since editing any part of a multipart symbol touches that file.
  Findings are blocking, matching the syntactic tier's severity: a physical overlap with no edge is a merge conflict waiting to happen, not a judgment call.
  Wire the call into `resolvePass` after card 17's and card 18's passes, so it sees the same single batched `Resolve` result set rather than issuing a second call.
  Cover in tests, against a fixture repository: a member glyph and the file self glyph of its own file on two cards producing exactly one finding; the same two refs on one card producing none; a member glyph and an unrelated file self glyph producing none; and a `multipart` member whose second part lives in the other card's file producing the finding.
- **Commit:** `20: feat(planglyph): add the resolve-backed tier of the containment check`

### Card 21: give an infrastructure error its own disposition

- **Context:**
  - `internal/planglyph/resolve.go`
  - `internal/planglyph/create.go`
  - `internal/planglyph/doc.go`
- **Edits:**
  - `internal/planglyph/repo.go`
  - `internal/planglyph/planglyph.go`
  - `internal/planglyph/repo_test.go`
  - `internal/planglyph/planglyph_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Make a non-nil `error` from `quarry.Open` or `(*quarry.Repo).Resolve` a category distinct from any per-target verdict, so it can never be silently read as a clean answer.
  quarry's own contract draws exactly this line: the failure envelope's `ok` key marks that quarry could not answer at all and never that the answer is negative, with `not_found` and `ambiguous` being ordinary payloads carrying a status word.
  Conflating the two is the specific disaster this card prevents — a transport failure read as a clean `not_found` is, under card 18's inversion, a **pass**, so a quarry outage would silently mark every `Create` card done.
  Declare `var ErrQuarryUnavailable = errors.New("planglyph: quarry could not answer")` in `repo.go` and wrap every `openRepo`/`resolveTargets` error with it, so a caller distinguishes it with `errors.Is` rather than by string matching.
  Change `ValidateFormat` and `Validate` to return `([]Finding, error)` rather than a bare slice, returning the wrapped error alongside whatever pure findings were already collected — the gate cannot certify a plan as valid against code it failed to read, and the error must report as a gate/infrastructure failure rather than as a plan finding so nobody mistakes "quarry broke" for "the plan is wrong".
  Update `doc.go` to state the three-way split: pure findings, resolve findings, and the infrastructure error that is neither.
  Rejected and worth stating in the file comment so it is not reintroduced: degrading to format-only validation with a warning, which is the exact failure mode where a plan looks validated and was not; and making the error informational everywhere, which makes the outage invisible at precisely the boundaries whose whole job is to be mechanical.
  Batch 5 wires the returned error onto the gate's own failure disposition, and batch 7 wires it onto the two webster boundaries.
  Cover in tests: `openRepo` against a directory that is not a repository returning an error satisfying `errors.Is(err, ErrQuarryUnavailable)`; `Validate` returning that error together with the pure findings it had already collected; and `language: none` returning a nil error against the same non-repository directory, since it opens nothing.
- **Commit:** `21: feat(planglyph): separate a quarry infrastructure error from every per-target verdict`

### Card 22: record planglyph in the invariant list, the module table and the design doc

- **Context:**
  - `internal/planglyph/doc.go`
  - `internal/planglyph/planglyph.go`
  - `internal/planparser/validate.go`
  - `_mill/discussion.md`
- **Edits:**
  - `CONSTRAINTS.md`
  - `docs/overview.md`
  - `manifest/designs/quarry-glyph-plan-alphabet.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Record the new module in the three places this repository's task-completion rule requires, in the same commit that finishes creating it.
  In `CONSTRAINTS.md`, add `planglyph` to the `## Told-Geometry Invariant` section's bound-packages bullet.
  Membership is unconditional and is settled by the package's own shape: `internal/planparser` is already on that list carrying a plain `worktreeRoot string`, which proves membership turns on deriving no paths rather than on carrying a `Geometry` struct.
  Do not hand `planglyph` a `Geometry` from `internal/hubgeom` or `internal/standalonegeom` — uniform with the engines, but heavier than `planparser`'s own precedent requires for a package that needs two path strings.
  In `docs/overview.md`, add a `planglyph` entry to the module list beside the existing `planparser` entry, describing it as the sole owner of every `quarry.Repo` call and of the resolve-backed validation pass, composing `planparser`'s pure checks rather than reimplementing them.
  In `manifest/designs/quarry-glyph-plan-alphabet.md`, replace the pre-implementation design sketch with the shipped design: the alphabet, the `package-ownership` seam between `planparser` and `planglyph`, the `language:` mode key, the handle lifecycle from draft through canonicalization to binding, the resolve status policy and the `Create` inversion, the two containment tiers, and the infrastructure-error disposition.
  Point at `CONSTRAINTS.md`'s Glyph Conversion Chokepoint Invariant rather than restating its rules, per the Producer Pointer-Rule Invariant's spirit.
  Do not edit `manifest/roadmap.md` in this card — the roadmap moves only on completing the item, which is card 39.
- **Commit:** `22: docs(planglyph): record the new module in CONSTRAINTS, the module table and its design doc`

## Batch Tests

`verify: go test ./internal/planglyph/ ./internal/planparser/ ./internal/lyxcwd/` runs the new package's whole test set plus the parser package it composes, because every card here layers onto `planparser`'s pure checks and a regression there surfaces as a `planglyph` failure that is easier to read with both packages in one run.
The `internal/planglyph` tests added in this batch are untagged and tier1-safe: they build a small fixture repository under `t.TempDir()` and call `quarry.Open`/`Resolve`, which read files and spawn no process — only `DeltaGit` spawns `git`, and its first call site arrives in batch 7 under an `integration` tag.
`testmain_test.go`'s `gitkit.HermeticGitEnv()` call from card 16 is in place ahead of those tagged tests, satisfying the Hermetic Git Test Environment Invariant before the first test that needs it exists.
The three-package scope is right rather than module-wide: no package outside these two imports `planglyph` until batch 5, and the overview's module-wide `go build ./...` catches a compile break from the card 21 signature change at this batch's own boundary.
`./internal/lyxcwd/` is in scope for one reason: the Markdown Link Integrity guard lives in `internal/lyxcwd/docslink_test.go` and scans `manifest/` and `docs/`, which this batch rewrites — without it a broken link would surface only at the hub's end-of-task done gate, several batches after the edit that caused it.
</content>
