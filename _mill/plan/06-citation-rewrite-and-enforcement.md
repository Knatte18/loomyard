# Batch: citation-rewrite-and-enforcement

```yaml
task: Deploy cited spec/design docs to target repos like stencils
batch: citation-rewrite-and-enforcement
number: 6
cards: 9
verify: go test ./contracts/stencils/... ./internal/loomengine/... ./internal/websterengine/... ./internal/shedadapters/... ./internal/burlerengine/... ./internal/lyxcwd/...
depends-on: [5]
```

## Batch Scope

This batch is the deliverable: the enforcement test that closes the class, the thirteen citation rewrites it exists to police, and the documentation collateral those rewrites falsify.
Everything before it was mechanism.

It is one batch because the enforcement test and the rewrites are one TDD unit — the test is written first and must fail on every audited occurrence, then go green as the rewrites land — and because every rewrite in it edits a file the same test scans.
Splitting the rewrites away from the test would leave a batch that is red by design with no batch able to make it green.

It depends on batch 5 and nothing less: the moment a stencil body carries `{{.specs_dir}}`, every route that renders it must already supply the value, or the render fails.

Batch-local decisions.
The rubric no-marker constraint is relaxed to a one-marker allowlist rather than deleted — a rubric may carry `{{.specs_dir}}` and nothing else, so a second marker, which would still be invisible to the fill at the value site, fails exactly as loudly as the old rule did.
The two unguarded `CONSTRAINTS.md` instructions are fixed by hand AND covered by a narrow companion assertion, because the scanner's prefix rule structurally cannot see a repo-root token and must not be widened to include one: `CONSTRAINTS.md` is a legitimate target-repo path a stencil is supposed to name, and the defect is the missing guard, not the token.
`docs/code-comment-conventions.md` is not deployed and not deleted; its citation is reworded away and its own retention note is rewritten to rest on its remaining standing.

## Cards

### Card 24: The citation enforcement test, written first

- **Context:**
  - `CONSTRAINTS.md`
  - `contracts/stencils/stencils.go`
  - `contracts/stencils/registry_test.go`
  - `internal/stencil/stencil.go`
- **Edits:** none
- **Creates:**
  - `contracts/stencils/citation_enforcement_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Create `contracts/stencils/citation_enforcement_test.go` in `package stencils`.
  Write it BEFORE any stencil body in this batch is touched: it must fail first, on every occurrence the rewrites are about to remove, and that failure set is the proof the scan works.

  `TestStencils_NoBareCrossRepoCitations` iterates `Registry().Names()`, reads each name's default bytes via `Default`, applies `stencil.StripLeadingComment` to get the agent-facing body, and scans that body for path tokens.
  The leading banner is stripped by that same function before any agent ever sees a stencil, so a banner-internal reference is not an agent-facing citation and must not be flagged — scanning the stripped body rather than the raw bytes is what makes that true, and the test's own comment must say so, because it is the difference between a handful of findings and forty.

  The token rule has three conjunctive parts, and each exists because a bare prefix match over-fires on real content:

  1. the token sits under one of the four repository-relative prefixes `contracts/`, `manifest/`, `docs/`, `internal/`;
  2. the token ends in `.md` or `.go`;
  3. the token contains no `#` and is not `plan:`-prefixed.

  Part 2 is what keeps a bare package reference such as the one at the end of the implementer body out of scope while keeping the Go-file citations in it.
  Part 3 excludes the two forms that make a token a glyph rather than a path — and it must be applied to the FULL surrounding token, not to a regex match that begins at the prefix: a `plan:`-handle's prefix sits to the LEFT of the matched path, so a scanner that matches only from `internal/` onward would strip the very evidence part 3 tests for.
  Extract the maximal non-whitespace, non-backtick run around each candidate before applying parts 2 and 3.

  An occurrence passes when it is `{{.specs_dir}}`-prefixed, or when `(stencil name, token)` appears on a named allowlist declared at the top of the file.
  Each allowlist entry carries its justification as a struct field, not as a comment, so the justification cannot drift away from the entry — the same `(file, target)`-keyed, owner-naming shape the Markdown Link Integrity invariant already uses.
  Seed the allowlist with exactly one entry: the stencil `loom-template-plan` and the token `internal/boardcli/list.go`, justified as a glyph-grammar example rather than a citation.
  A small number of deliberate entries is the design here, not a workaround.

  Add `TestStencils_AllowlistHasNoStaleEntries`, asserting every allowlist entry's token still appears in the named stencil's stripped body.
  Without it the allowlist silently accumulates dead rows that would re-permit a reintroduced citation.

  Add the companion assertion `TestStencils_ConstraintsCitationsAreGuarded`: any stencil whose stripped body mentions `CONSTRAINTS.md` must carry the phrase `if present` in the same sentence as that mention.
  This is a separate test, not a row of the prefix rule, and the file comment must say why: `CONSTRAINTS.md` is a repository-root token under none of the four prefixes, so the prefix rule structurally cannot see it — and it must not simply be added to the prefix set, because it is a legitimate target-repo path a stencil is supposed to name.
  The defect this assertion closes is the missing guard, not the token itself.

  Scenario coverage the file comment should name, since a future editor will otherwise not know what the test is for: a rewritten normative citation passes; a bare re-added one fails; an allowlisted entry passes; an allowlist entry whose token has vanished fails as stale.
- **Commit:** `test(stencils): add the bare-cross-repo-citation enforcement scan`

### Card 25: Rewrite the eight normative citations

- **Context:**
  - `contracts/stencils/citation_enforcement_test.go`
- **Edits:**
  - `contracts/stencils/loom/loom-template-plan.md`
  - `contracts/stencils/loom/loom-rubric-plan-review.md`
  - `contracts/stencils/loom/loom-rubric-webster-review.md`
  - `contracts/stencils/webster/webster-body-implementer.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Rewrite every normative citation to point at the deployed copy, preserving each sentence's surrounding prose and its meaning exactly — only the path token changes.
  The two replacement spellings are `` `{{.specs_dir}}/loom/loom-plan-spec.md` `` for the format contract, and `` `{{.specs_dir}}/loom/loom-plan-card-format.md` `` for the Card-model design doc.
  Note the second is not a straight path substitution: the deployed basename follows the registered name, which differs from the source file's own basename.

  Eight occurrences across four files, listed by their current line numbers as a locating aid — find them by their surrounding text, not by line number, since earlier edits in the same file shift them:

  - `contracts/stencils/loom/loom-template-plan.md` line 138, the `card-field-overlap` sentence citing the format contract's Card fields section and complete validation-check set.
  - `contracts/stencils/loom/loom-template-plan.md` line 143, the Verify-model tier-definitions sentence.
    Leave its trailing clause "this file does not restate them" intact — that clause is the reason the citation has to resolve at all.
  - `contracts/stencils/loom/loom-rubric-plan-review.md` line 15, which cites BOTH docs in one sentence; rewrite both tokens.
  - `contracts/stencils/loom/loom-rubric-plan-review.md` line 31, the twenty-eight-check-IDs sentence.
  - `contracts/stencils/loom/loom-rubric-plan-review.md` line 38, the per-type `ImpactSummary` sentence.
  - `contracts/stencils/loom/loom-rubric-webster-review.md` line 16, which cites BOTH docs in one sentence; rewrite both tokens.
  - `contracts/stencils/loom/loom-rubric-webster-review.md` line 54, the per-type mechanical-check table sentence.
  - `contracts/stencils/webster/webster-body-implementer.md` line 27, the full target and `Uses:` grammar sentence.

  Touch no other line in any of these four files, and touch no leading banner comment: the banners are stripped before an agent sees the text, so a banner edit changes nothing the enforcement scan reads while still churning the diff.
  Do not add a `{{.specs_dir}}` mention anywhere the enforcement scan does not require one — every marker added here must be one this card's own list names, so the rubric allowlist rewrite in card 27 stays true.
- **Commit:** `fix(stencils): point the eight normative citations at the deployed specs directory`

### Card 26: Drop the path from the four background citations

- **Context:**
  - `contracts/stencils/citation_enforcement_test.go`
  - `internal/shedadapters/bouncerfiles.go`
- **Edits:**
  - `contracts/stencils/bouncer/bouncer-template-judge.md`
  - `contracts/stencils/bouncer/bouncer-template-seed.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Four sentences name the Go parser that enforces a file format — three in the judge template (currently lines 61, 90, 116) and one in the seed template (currently line 50).
  Each currently reads along the lines of "This format is enforced by the parser in `internal/shedadapters/bouncerfiles.go`."

  Drop the path from all four, keeping the enforcement substance: the fact that changes behaviour is that the format is parsed mechanically and that deviating from it fails the parse, not the name of a source file the agent cannot open.
  Rewrite each to say so directly — the format is parsed mechanically, and any deviation from it fails the parse — preserving each site's existing trailing punctuation and any clause that follows the citation on the same line.

  Naming a loomyard source file tells an agent working in some other repository nothing it can act on, which is the whole class this task closes.
  Do not replace the path with `{{.specs_dir}}`: these files do not travel, and pointing at a deployed path that does not contain them would be a worse dead reference than the current one.
  Do not add an allowlist entry for them either — an allowlist entry would preserve exactly the defect being removed.

  Touch no leading banner comment in either file, and no other line.
- **Commit:** `fix(stencils): drop the unreachable parser path from the four background citations`

### Card 27: Guard the two unguarded CONSTRAINTS.md instructions

- **Context:**
  - `CONSTRAINTS.md`
  - `contracts/stencils/loom/loom-template-discussion.md`
  - `contracts/stencils/citation_enforcement_test.go`
- **Edits:**
  - `contracts/stencils/webster/webster-template-master.md`
  - `contracts/stencils/webster/webster-prefix-recovery.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Two stencils instruct an agent to read `CONSTRAINTS.md` in full with no guard, which is a dead instruction in a repository that has none.

  In `contracts/stencils/webster/webster-template-master.md` (currently line 25), the clause reading "read `CONSTRAINTS.md` in full" gains the guard.
  In `contracts/stencils/webster/webster-prefix-recovery.md` (currently line 20), the numbered step reading "Read `CONSTRAINTS.md` in full." gains it too.

  Copy the wording model the repository already uses rather than inventing a third phrasing: `contracts/stencils/loom/loom-template-discussion.md` line 42 already says "read `CONSTRAINTS.md` at the repo root if present", and the plan template carries the same form.
  The phrase `if present` must land in the same sentence as the `CONSTRAINTS.md` mention, because that is exactly what card 24's companion assertion checks.

  `CONSTRAINTS.md` stays a bare path in both, deliberately: it is a target-repo path a stencil is supposed to name, unlike the loomyard-internal paths the other cards remove, so it neither moves under `{{.specs_dir}}` nor gets dropped.

  Preserve each site's surrounding instruction exactly — the master template's sentence continues into reading the plan overview, and the recovery prefix's step sits in a numbered list whose numbering must not shift.
- **Commit:** `fix(stencils): guard both unguarded CONSTRAINTS.md instructions with "if present"`

### Card 28: Rescope the comment-convention check to the target repo

- **Context:**
  - `docs/code-comment-conventions.md`
- **Edits:**
  - `contracts/stencils/loom/loom-rubric-webster-review.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** The Webster-Review rubric's comment-convention bullet (currently line 51) reads "Any new or changed doc comment follows `docs/code-comment-conventions.md`."
  That document is explicitly Go-only and is loomyard's own house style, so shipping it into an arbitrary target repository would have a reviewer enforce the wrong conventions there — a worse defect than the dangling path.
  The citation is the thing to fix.

  Reword the bullet so the reviewer checks any new or changed doc comment against the **target repository's own** conventions — the repository's own constraints document if it has one, and the conventions the surrounding code already follows — dropping the path entirely.
  Keep the bullet's existing "no unnecessary symbol cross-references" substance if it is carried on that line, since that is a convention-independent property worth keeping.

  The document is not deployed and not deleted; card 30 is where its own retention note is repaired.
  Touch no other line in this file — the two normative citations in it are card 25's, and the per-type mechanical-check sentence is card 25's too.
- **Commit:** `fix(stencils): rescope the comment-convention check to the target repo's own conventions`

### Card 29: Relax the rubric marker rule to a one-marker allowlist

- **Context:**
  - `contracts/stencils/loom/loom-rubric-plan-review.md`
  - `contracts/stencils/loom/loom-rubric-webster-review.md`
  - `contracts/stencils/loom/loom-template-plan.md`
  - `contracts/stencils/webster/webster-body-implementer.md`
  - `internal/stencil/stencil.go`
  - `internal/shedadapters/rubric.go`
- **Edits:**
  - `contracts/stencils/rubric_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Three tests in `contracts/stencils/rubric_test.go` currently assert a rubric's bytes contain no `{{.` substring at all — one per rubric, at lines 54, 98, and 143 — and the file's own header comment encodes that rule as the marker-value-not-template constraint.
  Card 25 puts `{{.specs_dir}}` into two of the three rubrics, so these assertions must be rewritten rather than deleted: deleting them would forfeit the guarantee entirely, when what changed is its shape, not its existence.

  Rewrite each of the three `CarriesNoStencilMarkers` tests into an allowlist form, renamed to reflect what it now asserts: parse the rubric's marker set with `stencil.TopLevelMarkers` and assert that set is a subset of `{"specs_dir"}`.
  A second marker fails exactly as loudly as the old rule did, which is the property worth keeping: a marker sitting inside a value is invisible to the fill at the interpolation site, so any marker other than the one the render helper knows how to fill would ship literally into a judge prompt.
  Apply the same rewrite to the discussion-review rubric's test even though that rubric gains no marker — its allowed set is the same, and an asymmetric rule across the three is how the next author loses the invariant.

  A marker-set assertion is not sufficient on its own, and this is the half most likely to be skipped.
  Because the render helper now runs the rubric through `stencil.Fill`, a bare `{{` an author writes in prose — not `{{.`, so there is no marker name to inspect — becomes a runtime parse-template error no marker-name check can see.
  Add `TestLoomRubrics_ParseUnderTheRenderHelper`, asserting each of the three rubrics renders successfully through `shedadapters.ReadRubric` given a non-empty specs directory.
  That is the assertion that actually covers the new failure mode.
  If importing `internal/shedadapters` from this package creates an import cycle or an unwanted dependency, assert against `stencil.Fill` over the stripped bytes with the same single value instead, and say in a comment that the helper is the production path this stands in for.

  Add `TestStencils_SpecsDirMarkerIsPresent`, asserting each of the four stencils that carry a normative citation — the plan template, the two rubrics card 25 rewrites, and the implementer body — contains the literal `{{.specs_dir}}`.
  Without it a future edit could quietly revert a citation to a bare path and only the enforcement scan would notice, and only if the bare path happened to match its prefix rule.

  Fix the now-false assertion at line 123: it pins the literal `code-comment-conventions.md`, which card 28 removes.
  Replace the pinned phrase with a distinctive substring of the reworded bullet, keeping the test row's intent — that the rubric still carries a comment-convention check — rather than dropping the row.

  Update the file's header comment for all of the above: the marker-value-not-template constraint is now a one-marker allowlist, and the file additionally pins the specs-directory marker's presence.
- **Commit:** `test(stencils): relax the rubric marker rule to a one-marker allowlist and pin specs_dir`

### Card 30: Repair the documentation collateral the rewrites falsify

- **Context:**
  - `contracts/stencils/loom/loom-rubric-webster-review.md`
  - `contracts/stencils/rubric_test.go`
- **Edits:**
  - `docs/code-comment-conventions.md`
  - `manifest/designs/loom.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Two documents assert something card 28 makes untrue, and both are repaired here rather than left to drift.

  `docs/code-comment-conventions.md`'s status blockquote (currently line 5) keeps that document out of the documentation lifecycle's deletion class specifically because "live producer rubrics still cite" it, naming the rubric content test as the guard.
  Card 28 falsifies that retention rationale, not merely one sentence of it.
  Rewrite the sentence so the retention rests on the document's remaining standing — it is the standing rationale for a cross-cutting rule this repository's own code still follows, and the operative rule itself lives in the skill the blockquote already names.
  Remove the claim that a producer rubric cites it and the claim that the rubric test guards it, since neither will be true.
  The document is kept, not deleted, and no deletion decision is deferred to a later task — say that plainly so a future lifecycle audit does not re-open it.

  `manifest/designs/loom.md`'s Webster-Review rubric section (currently line 276) is that rubric's durable transcription record, explicitly kept in step with the stencil.
  Its comment-convention bullet carries the inline markdown link to the conventions document — the very item card 28 rewords away.
  Reword the bullet in step with the stencil, so the durable record and the shipped file continue to say the same thing, and remove the inline link along with the claim it supports.
  Both halves matter: the project convention requires the module doc to land with the change, and the Markdown Link Integrity invariant binds every inline link in a document under this directory, so a link left behind pointing at a rationale the bullet no longer makes is stale even though it still resolves.

  Change nothing else in either document.
  Do not touch the design document that travels as a deployed spec, and do not touch the roadmap: this closes a filed bug, not a planned roadmap item.
- **Commit:** `docs: repair the retention note and the durable rubric record the rescope falsifies`

### Card 31: Update the invariant and the module description

- **Context:**
  - `contracts/specs/specs.go`
  - `internal/stencilstore/reconcile.go`
  - `internal/stencilcli/cli.go`
- **Edits:**
  - `CONSTRAINTS.md`
  - `docs/overview.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Update the Stencil Ownership Invariant in `CONSTRAINTS.md` for the second embed site and the second seeded directory this task adds.

  Its first bullet currently reads that `//go:embed` in the stencils package is seed defaults only.
  Widen it to cover both embedded-default registries — the producer prompts and the deployed normative specs — naming the specs package as the second registry's home and stating that the same sole-owner rule applies to both: one package owns seeding, hashing, reading, and validation for either baseDir, a hash-mismatched file is never overwritten in either, and the seed/refresh pass runs once per process pre-run for both, never lazily.
  Keep the existing skip-annotation sentence intact and make it cover both passes, since a command that reads no stencils reads no specs either.
  Do not weaken the hash-mismatch rule for specs: the deployed copies deliberately reuse the existing policy unchanged, with no force-sync carve-out.

  In `docs/overview.md`, update the `stencil` module's description where it enumerates the five verbs, so it states which of them cover deployed specs and which do not: list and sync cover them, validate and diff and promote do not.
  State the reason in the same breath for the three that do not — a spec declares no markers for validate to compare, and diff and promote both need a worktree source directory that specs deliberately do not have — so the asymmetry reads as a decision rather than an omission.
  Keep the entry's existing shape and its implemented marker; this is an amendment to one entry, not a new row.

  The module table gains no row and the execution stack does not change, since no new lyx module is added.
  Do not touch the roadmap.
- **Commit:** `docs: record the second embed site in the Stencil Ownership Invariant`

### Card 32: Pin that no unrendered marker reaches a composed prompt

- **Context:**
  - `internal/websterengine/render.go`
  - `internal/loomengine/plan.go`
  - `internal/shedadapters/bouncer.go`
  - `internal/shedadapters/rubric.go`
  - `contracts/stencils/loom/loom-rubric-plan-review.md`
- **Edits:**
  - `internal/websterengine/render_test.go`
  - `internal/loomengine/plan_test.go`
  - `internal/shedadapters/bouncer_judge_test.go`
  - `internal/shedadapters/bouncer_seed_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** The rubric case is the sharpest trap in this task, and a test that only checks a rubric file would pass while the shipped prompt still carried an unrendered marker.
  Add assertions at the point each composed prompt is actually produced.

  In `internal/loomengine/plan_test.go`, extend the Plan prompt coverage so it asserts the composed prompt contains the told specs directory and contains no literal `{{.specs_dir}}` substring afterwards.
  Add a second assertion that composing with an empty specs directory returns an error rather than a prompt with a blank path — the required-marker guarantee, asserted at the composer rather than inferred from the fill helper.

  In `internal/websterengine/render_test.go`, add the same pair for both implementer prompts: the in-session fork prompt and the cold recovery prompt each render the told specs directory, neither leaves a literal marker behind, and each errors on an empty value.
  Both matter because the marker lives in the shared job body that both prompts compose.

  For the two rubrics, assert at the point the composed Bouncer prompt is produced, not at rubric-read time.
  Extend the existing judge-pass and seed-pass coverage in `internal/shedadapters/bouncer_judge_test.go` and `internal/shedadapters/bouncer_seed_test.go` in place rather than adding a new file: each already drives its pass against a fixture rubric, so the composed prompt is already in reach there.
  Assert on that composed prompt that the told specs directory appears in it and that no literal `{{.specs_dir}}` survives.
  A rubric is interpolated as a value into the template's rubric marker and is never itself run through the fill at that site, which is exactly why the assertion has to sit on the composed prompt rather than on the rubric bytes.

  Assert the rendered specs directory is absolute in at least one of these tests.
  A deployed spec lives outside the agent's own worktree, so an agent can only reach it by absolute path — a relative spelling would render, pass every substring assertion, and still be unopenable.

  Do not weaken any existing assertion in either file to make room.
- **Commit:** `test: pin that specs_dir renders and no literal marker survives into a composed prompt`

## Batch Tests

`verify: go test ./contracts/stencils/... ./internal/loomengine/... ./internal/websterengine/... ./internal/shedadapters/... ./internal/burlerengine/... ./internal/lyxcwd/...`

- `contracts/stencils` is the batch's centre: the new enforcement scan from card 24, the rewritten rubric content test from card 29, and the registry tests that must stay green across thirteen body edits.
- `internal/loomengine` and `internal/websterengine` carry card 32's composed-prompt assertions, and are also where a stencil body edit that broke a required marker would surface as a render failure.
- `internal/shedadapters` covers the rubric render path the two rewritten rubrics now travel through.
- `internal/burlerengine` is in scope because the same two rubrics reach the Burler round prompt as a value; a body edit that broke that interpolation shows up nowhere else.
- `internal/lyxcwd` is in scope for the documentation link scan, since card 30 removes an inline link from a document under the scanned directories.

Card 24 is written first and is expected to fail on sixteen occurrences: the fifteen the audit tables enumerate, plus the one glyph-grammar example that satisfies the token rule and is resolved by an allowlist entry rather than by a rewrite.
Do not treat thirteen — the audit's row count — as the expected failure count: two rows cite two paths each, and one row's path appears at three separate lines.
The two constraints-document rows contribute zero to that number; they are covered by the separate companion assertion instead.

No integration-tagged file is edited in this batch, so no `-tags integration` invocation is needed.
The repository-wide sweep, including the tagged half, is the configured done gate's job at the end of the run.
