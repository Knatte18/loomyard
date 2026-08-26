# Discussion: loom: Plan-Write/Plan-Validate approval deadlock (F7)

```yaml
task: 'loom: Plan-Write/Plan-Validate approval deadlock (F7)'
slug: loom-plan-approval-gate
status: discussing
parent: main
```

## Problem

loom's seventeen-row pipeline cannot get past row 8, and it never could.
Three shipped contracts contradict each other.
`contracts/stencils/loom/loom-template-plan.md` orders the plan writer to emit `approved: false` and states "Always write `approved: false` — you never self-approve";
`internal/planparser`'s `checkFormatAndApproval` emits the `plan-unapproved` finding whenever `!plan.Approved`;
and `contracts/recipes/loom-recipe.yaml` routes `Plan-Write --on_done--> Plan-Validate --on_stuck--> Plan-Write`.
No row anywhere in the recipe ever flips `approved` to `true`.
So `Plan-Write` writes a correct plan, `Plan-Validate` rejects it for the one thing the writer was ordered to do, the run bounces back to `Plan-Write`, and the loop spins until the bounce budget is spent and the run blocks to a human.
The plan is never wrong;
only the check is unsatisfiable.

**Why now.** This is Crucible round 1's finding F7 against loom (the module merged to `main` in `a0612b30`;
the report is preserved at git tag `archive/loom-crucible-hardening~1`, path `_mill/loom-review-opus5-high-r1.md`, and its disposition in the sibling `-fixer-report.md`).
It was confirmed live: four bounce cycles observed, each spending a real LLM session, each leaving another `_lyx/plan/archive-<stamp>/` behind.
The round deliberately did not fix it, because the fix is a feature addition across seven packages rather than a commit-per-fix hardening change, and because it carries one design question a hardening round should not settle alone.
Everything downstream of row 8 — `Plan-Revalidate`, `Batchifier`, `Webster`, `Webster-Review`, `Publish`, `Finalize` — is unverifiable until this lands.
It is the single blocker keeping loom from being crucible-merge-ready.

## Scope

**In:**

- A plan-format writer in `internal/planparser` that sets `approved: true` in `00-overview.md`.
- A split of `internal/planparser`'s validation entry point into a format-only function and the existing full function that adds the approval check.
- A new optional `Approve func() error` seam on `internal/shedadapters`' `BouncerConfig`, called on the APPROVED branch of `settle` before `Commit`.
- A new `approve_seam` recipe config key on the `Bouncer` registry row in `internal/shedrecipe`, plus a new `require_approved` key on the `PlanValidate` registry row.
- A new `Env.ApprovePlan` closure field in `internal/shedrecipe`, filled in `internal/loomcli`'s `wire()`.
- A mode parameter on `internal/loomshed`'s `NewPlanValidate` so the two rows sharing the one engine differ.
- `contracts/recipes/loom-recipe.yaml`: `approve_seam: plan` on `Plan-Bouncer`, `require_approved: true` on `Plan-Revalidate`, and the `Plan-Burler` row's `fasit.instructions` prose corrected — see the Technical context entry for that string.
- A `--require-approved` flag on `lyx loom validate-plan`, and the parity test extended to cover both rows.
- `internal/loomrecipe/fixture_test.go`'s fake `Plan-Write` corrected to write the unapproved plan the real writer is required to write, and `fakeLoomBurler`'s injected regression re-pointed to a format fault the new seam cannot mask — see the Testing section's keystone paragraph;
  the first of those is the regression test for F7 itself.
- Docs in the same commit: `manifest/designs/loom.md`, `contracts/specs/loom-plan-spec.md`, `contracts/stencils/loom/loom-template-plan.md`, `contracts/stencils/loom/loom-rubric-plan-review.md`, `internal/loomengine/plan.go`'s package doc, `internal/planparser/validate.go`'s package doc, `internal/loomshed/planvalidate.go`'s package doc and `Call` doc, `internal/loomcli/validate.go`'s `Long` text, and two `CONSTRAINTS.md` entries — the **Gate Self-Check Parity Invariant** (a gate's verb reaches every mode its row set uses) and the **Planparser Sole-Parser Invariant** (widened to sole parser *and* sole writer of the plan format).

**Out:**

- **`Plan-Write` still writes `approved: false`.**
  The stencil's "you never self-approve" rule is the whole point and does not change;
  only the sentence naming *who* flips it is corrected.
- **Webster, `webstercli`, and `Batchifier` are untouched.**
  They are consumers running after approval and keep calling the full, approval-enforcing `Validate`.
- **`Discussion-Bouncer` / `Webster-Bouncer` are untouched.**
  Neither has an artifact with an approval flag;
  both simply omit the new `approve_seam` key, which leaves the seam nil exactly as an omitted `commit_seam` already does.
- **No new recipe row.**
  The seventeen row names stay as they are, so the coverage guard pinning them against `internal/loomshed`'s `Name*` constants needs no change.
- **No live end-to-end loom run in this task.**
  Verification is unit plus wiring tests;
  proving the full pipeline runs to `Finalize` is the next task's job, and it is exactly what this change unblocks.
- **F2's findings-surfacing work, F3's fabric clone bug, F12's attach redraw** — separate findings from the same round, separate tasks.

## Decisions

### approval-write-lives-on-the-bouncer-settle

- **Decision:** the approval write is an injected `Approve func() error` closure on `shedadapters.BouncerConfig`, invoked on `settle`'s `verdictApproved` branch immediately **before** `b.cfg.Commit`, selected by a new `approve_seam: plan` key on the `Plan-Bouncer` recipe row.
- **Rationale:** `Plan-Bouncer`'s approved settle is the only point in the list that knows the plan passed review, and it is the only point that can guarantee write-then-commit ordering in one step — the `approved: true` byte must be inside the commit `Commit` makes, not left dirty in the weft working tree afterwards.
  It also reuses a seam pattern already shipped and already proven on this exact producer: `Commit`/`commit_seam` is the same shape, an opaque `func() error` resolved by a recipe key, so the generic `Bouncer` gains no plan-specific knowledge.
  No new row means no new `Name*` constant, no coverage-guard churn, and one commit in git history rather than two.
- **Rejected:** a dedicated `Plan-Approve` row between `Plan-Bouncer` and `Plan-Revalidate`.
  It would run *after* `Plan-Bouncer`'s `Commit` already fired, so it would need a second commit of its own to make the flag durable, producing two consecutive `loom: plan artifacts for <slug>` commits.
  Its one genuine advantage — an auditable approval event in the shed status history — is already served by the commit itself and by `Plan-Bouncer`'s own recorded `done`.
- **Rejected:** having `Plan-Revalidate` write it.
  That makes the row that *enforces* the check also the row that *satisfies* it, which is not a gate.

### validate-splits-into-two-named-functions

- **Decision:** `internal/planparser` exposes two entry points.
  `ValidateFormat(plan, worktreeRoot) []ValidationError` runs the fourteen checks `checkIndexFileConsistency` through `checkCommitSubjectMismatch` plus `format-unrecognized` — fifteen IDs in all.
  `Validate(plan, worktreeRoot) []ValidationError` keeps its current signature and current meaning: `ValidateFormat`'s fifteen findings plus the `plan-unapproved` check, sixteen IDs in all.
  `checkFormatAndApproval` splits into two helpers, `checkFormatRecognized` and `checkApproved`.
  **`Validate` splices the approval finding at position two, preserving `contracts/specs/loom-plan-spec.md:200-220`'s fixed sixteen-row order byte-for-byte — it does not append it last, and that ordered list is not renumbered.**
  The two exported functions are thin wrappers over one unexported `validate(plan, worktreeRoot string, requireApproved bool)` that emits `checkFormatRecognized`, then `checkApproved` when `requireApproved`, then the remaining fourteen in their existing order.
  The unexported bool is an implementation detail of ordering and does not contradict the two-named-functions choice, which is about the package's exported seam.
- **Rationale:** two named functions read better than a boolean and, decisively, no `planparser.Validate` call site needs a signature change.
  There are three production call sites outside `internal/loomshed`: `internal/websterengine/runlevel.go:332` and `internal/webstercli/validate.go:74` keep both their call and their behaviour unchanged, and `internal/loomcli/validate.go:97` keeps its call shape but switches which of the two functions it names, per the verb-mode decision below.
  `contracts/specs/loom-plan-spec.md` already frames `plan-unapproved` as "`approved: true`; else refuse to **run**", a consumer guard, which is exactly the split this makes explicit.
- **Rejected:** a `ValidateOptions` struct parameter on `Validate`.
  It changes every call site for one boolean and makes the default mode a thing every caller must state.
- **Rejected:** deleting `plan-unapproved` from `planparser` entirely and enforcing approval only in Webster.
  That loses `Plan-Revalidate`'s post-segment integrity check and leaves the format spec's own sixteen-ID list wrong.

### planvalidate-row-mode

- **Decision:** `loomshed.NewPlanValidate` gains a fourth parameter, `requireApproved bool`, giving the signature `NewPlanValidate(name, anchorPath, worktreeRoot string, requireApproved bool) shedengine.ShedProducer`.
  A plain `bool`, not a named mode type: the producer has exactly two modes and no third is foreseeable, and the field is stored as `requireApproved bool` on the unexported `planValidate` struct.
  `shedrecipe`'s `planValidateEntry` reads a new optional `require_approved` bool config key (absent ⇒ `false`) to supply it.
  The signature change touches **every `loomshed.NewPlanValidate` construction in the repo** — all of them in tests;
  let the build enumerate them rather than trusting a written count.
  The ones found during exploration are `internal/loomrecipe/shape_test.go` (two constructions, `:49` and `:52`, inside `reflect.TypeOf`), `internal/loomcli/parity_test.go:197`, `internal/loomshed/cancellation_test.go:86`, `internal/loomshed/gatefindings_test.go:93`, and four in `internal/loomshed/planvalidate_test.go`.
  `contracts/recipes/loom-recipe.yaml` sets `require_approved: true` on the `Plan-Revalidate` row and leaves the key absent on the `Plan-Validate` row.
- **Rationale:** the two rows deliberately share one engine, and the recipe is where their difference already lives (their `on_done` targets already differ there).
  `Plan-Validate` runs before review and must not demand a flag only the review can produce;
  `Plan-Revalidate` runs after the segment settles and must confirm the flag is there, catching both a fixer-introduced regression and a failed approval write.
- **Rejected:** two registry engines.
  The rows are the same producer in two modes, and the existing recipe comment on `Plan-Revalidate` already documents the shared-engine choice as deliberate.

### planparser-writer-shape

- **Decision:** `planparser.SetApproved(planDir string) error`.
  It reads `00-overview.md`, rewrites the frontmatter's `approved:` line to `approved: true` in place and leaves every other byte of the file — remaining frontmatter keys, key order, the framing paragraph, the Card Index, every plan-level section — untouched.
  It is idempotent: an already-`true` plan is a successful no-op.
  When the `approved:` key is absent from an otherwise well-formed frontmatter block, it inserts `approved: true` rather than failing.
  A missing overview file, an unreadable file, or a file with no frontmatter block is an error.
- **Rationale:** the Planparser Sole-Parser Invariant reserves plan-format writes to this package, so the writer cannot live anywhere else.
  Surgical line rewriting rather than parse-and-re-serialize is required because a YAML round-trip would not preserve the body at all and would reorder or requote the frontmatter.
  Insert-if-absent costs one branch and makes the seam total over every plan `ParsePlan` accepts — `Approved` is a `*bool`, so an absent key parses fine today and would otherwise be a silent deadlock of exactly the kind this task exists to remove.
- **Rejected:** a `WriteApproved(overviewPath string, approved bool) error` two-way setter.
  Nothing needs to un-approve a plan;
  `Plan-Write` rewriting the overview from scratch is the only path back to `false`.
- **Rejected:** full parse-and-re-serialize.

### validate-plan-verb-gets-a-mode-flag

- **Decision:** `lyx loom validate-plan` gains a `--require-approved` bool flag, default `false`.
  With the flag absent it calls `planparser.ValidateFormat`, matching the `Plan-Validate` row;
  with the flag set it calls `planparser.Validate`, matching the `Plan-Revalidate` row.
  `internal/loomcli/parity_test.go`'s `TestGateParity_PlanValidate` is extended to drive both modes against both rows.
- **Rationale:** the Gate Self-Check Parity Invariant requires the row and the verb to call the same package function with neither re-implementing the other.
  Two rows now share the engine in two modes, so a single-mode verb would leave one row without a self-check.
  The default is `false` because the verb's documented user is the writer agent calling it before handoff, which is pre-review.
- **Rejected:** no flag, pre-review mode only — leaves `Plan-Revalidate` unmirrored.
- **Rejected:** a second verb (`validate-plan-approved`) — two verbs for one gate contradicts the invariant's one-verb-per-gate shape.

### approve-failure-is-an-error-not-a-stuck

- **Decision:** in `Bouncer.settle`, a non-nil error from `Approve` is returned as `settle`'s own error, never routed through `degrade`, exactly as the existing `Commit` failure already is.
  `Approve` runs first;
  if it fails, `Commit` is not attempted.
- **Rationale:** `degrade` only ever returns `shedengine.Stuck`, so sending an approval-write failure through it would silently convert an approval into a rejection — the identical reasoning the existing `Commit` branch already carries in its own comment.
  A failed write also means the commit would capture a plan still marked unapproved, which `Plan-Revalidate` would then correctly reject, turning an I/O fault into a wasted review generation.
- **Rejected:** degrade to `Stuck`.

### a-failed-settle-seam-costs-one-review-generation-and-that-is-accepted

- **Decision:** when `Approve` or `Commit` fails, `settle` returns the error, `shedengine.Run` persists the run `failed`, and the operator resumes.
  `internal/shedengine/run.go:95` deliberately does not short-circuit `StateFailed` — it re-calls `current_producer` — so the resume re-enters `Plan-Bouncer.Call`, which finds its own APPROVED verdict still on disk, archives the run directory, re-seeds, and **spends a complete new LLM review generation** before settling again.
  That cost is accepted for this task rather than engineered away.
  No retry loop is added inside `settle`, and no short-circuit is added to the clear-and-re-seed trigger.
- **Rationale:** the state converges without intervention, and every intermediate state is benign.
  `Approve` runs before `Commit`, so the only reachable inconsistency is a plan carrying `approved: true` in the working tree but not in git.
  The re-run's judge never reads the approval flag — the Plan-Review rubric forbids re-deriving anything `Plan-Validate` checks, and the sixteen check IDs are named there explicitly — so the second generation judges the same plan on the same grounds and reaches the same verdict.
  On its APPROVED settle, `SetApproved` is an idempotent no-op over the already-flipped file and `CommitPlan` picks up the uncommitted change, so the run heals itself.
  The trigger is an I/O fault on a local file write or a local git commit, which is rare, and the alternative costs more than it saves.
- **Rejected:** a durable `settled` marker file written by `settle` after both seams succeed, with the clear-and-re-seed trigger narrowed to "APPROVED verdict **and** marker present", so an APPROVED-without-marker run directory means "my own settle failed part-way" and is retried rather than re-judged.
  This is the correct long-term fix and it would eliminate the wasted generation for all three segments, not just this one — `Discussion-Bouncer` and `Webster-Bouncer` have the identical exposure today.
  It is rejected here because it changes the generic `Bouncer`'s durable on-disk contract for two segments this task otherwise does not touch, which is exactly the scope growth F7's own fixer report warned against.
  **It should be filed as its own follow-up task** once this one lands.
- **Rejected:** retrying `Approve`/`Commit` in a bounded loop inside `settle`.
  It hides a genuine I/O fault from the operator and does nothing for the crash-between-the-two-seams case, which is the one that actually costs a generation.

### approve-seam-accepts-only-plan

- **Decision:** `approve_seam`'s only accepted value is `"plan"`, resolving to `Env.ApprovePlan` through the existing `requireSeam` guard.
  An absent key leaves `BouncerConfig.Approve` nil, which means "approve nothing" and keeps every existing `Bouncer` row valid unchanged.
  Any other value is a construction error naming the accepted value.
- **Rationale:** this mirrors `commit_seam`'s shipped shape one-for-one, including the `requireSeam` guard that stops a nil `Env` closure from silently reproducing the no-seam condition the key exists to eliminate.
  There is no discussion-side or webster-side approval flag, so inventing a second accepted value now would be a hypothetical.
- **Rejected:** mirroring `commit_seam`'s two-value set.

### stencil-and-writer-doc-keep-approved-false

- **Decision:** `contracts/stencils/loom/loom-template-plan.md` keeps emitting `approved: false` in both its frontmatter block and its minimal skeleton, and keeps the "Always write `approved: false` — you never self-approve" rule verbatim.
  Only the trailing clause "a future review gate flips it to `true`" (`loom-template-plan.md:80`) is corrected to name `Plan-Bouncer`'s approved settle as the row that does it.
  `internal/loomengine/plan.go`'s package doc gets the matching correction, dropping "not built here".
  The stencil's **Step 5 — Self-check before ending your turn** (`:152-164`) stays **verbatim**, including "The verb takes no arguments" and "re-run it until it exits 0 before ending your turn".
- **Rationale (Step 5 specifically):** Step 5 is the writer-facing half of the same deadlock and today it is an impossible instruction — the writer is told to write `approved: false` and then to re-run `lyx loom validate-plan` until it exits 0, which it never can.
  The verb's new default mode is precisely what makes that instruction satisfiable, so the wording needs no change: bare `lyx loom validate-plan` still takes no arguments, and it now exits 0 on the unapproved plan the writer was ordered to produce.
  Adding `--require-approved` to the stencil would be actively wrong — it would re-impose the deadlock on the one agent that must never satisfy it.
- **Rationale:** the writer must not self-approve;
  that separation is what makes the review gate mean anything.
  The stencil was never wrong about the rule, only about the tense.
- **Rejected:** having the stencil omit the key entirely and relying on `SetApproved`'s insert-if-absent path.
  An explicit `approved: false` is a readable, greppable record that the plan has not been reviewed;
  making absence the normal state would make the insert path the normal path and hide the state.

### consumers-keep-enforcing-approval

- **Decision:** `internal/websterengine/runlevel.go`, `internal/webstercli/validate.go`, and anything else consuming a finished plan keep calling `planparser.Validate` and keep refusing an unapproved plan.
  `Batchifier` is unchanged and still parses nothing.
- **Rationale:** they run after `Plan-Bouncer` has settled, so the flag is genuinely there by then, and the refusal is the standalone-invocation guard the spec's "else refuse to run" wording describes.
- **Rejected:** moving them to `ValidateFormat`, which would delete the guard for the one class of caller it was written for.

## Technical context

**The three contradicting contracts, verbatim locations.**

- `contracts/stencils/loom/loom-template-plan.md:71` (`approved: false` in the frontmatter block), `:79` (the "never self-approve" rule), `:127` (the minimal skeleton).
- `internal/planparser/validate.go:79` `checkFormatAndApproval`, whose second half emits `plan-unapproved: plan frontmatter approved: is not true` at `:88-93`.
  It is the second of sixteen check IDs;
  the package doc at the top of the file lists all sixteen and names `checkFormatAndApproval` as the source of the first two, so that doc must be updated by the split.
  The dispatch list in `Validate` itself is `validate.go:60-73`: fourteen `check*` calls after `checkFormatAndApproval`, which is why `ValidateFormat` is fourteen checks plus `format-unrecognized`, fifteen IDs, and `Validate` is sixteen.
- `contracts/recipes/loom-recipe.yaml`, rows `Plan-Write`, `Plan-Validate`, `Plan-Bouncer`, `Plan-Burler`, `Plan-Revalidate`.

**`contracts/stencils/loom/loom-rubric-plan-review.md`.**
The Plan-Review rubric tells the judge not to re-derive "the sixteen check IDs … enforced deterministically upstream" (`:20`, `:31`), naming `format-unrecognized` through `commit-subject-mismatch`.
After the split only **fifteen** are enforced upstream of the judge — `plan-unapproved` moves downstream to `Plan-Revalidate`, which runs *after* the segment.
The contiguous-range phrasing is what makes a bare count edit impossible: `plan-unapproved` sits at position two *inside* "`format-unrecognized` through `commit-subject-mismatch`", so "fifteen … through `commit-subject-mismatch`" would be self-contradictory.
Disposition: keep the range and carve out the exception — "the sixteen check IDs `format-unrecognized` through `commit-subject-mismatch` are enforced deterministically outside this round: fifteen of them upstream by `Plan-Validate`, and `plan-unapproved` downstream by `Plan-Revalidate`".
The don't-re-derive instruction itself stays exactly as it is, since the judge must not re-derive the approval flag either — it is not the judge's business in either direction, upstream or downstream.

**`contracts/recipes/loom-recipe.yaml`'s `Plan-Burler` `fasit.instructions` (`:158-161`).**
That string tells the overlay fixer round "the mechanical checks over that format are already enforced upstream by `Plan-Validate`, so re-deriving them in this round is duplicated work", which stops being true for `plan-unapproved` alone.
Disposition: carve out the same exception the rubric gets, and **add the self-approval prohibition explicitly** — the fixer round may never write `approved:` in `00-overview.md`.
The "never self-approve" rule binds `Plan-Burler` exactly as it binds `Plan-Write`, and for a sharper reason: the fixer runs *inside* the review segment, so a fixer that set the flag would be approving the very artifact the round is judging.
This is not a hypothetical the recipe can leave unsaid — `Plan-Burler` runs `fix-scope: overlay` with `_lyx/plan` in `target.paths`, so `00-overview.md` is a file it is already permitted to write.
The prohibition is prose in the row's instructions, not a mechanical guard;
the mechanical backstop is `Plan-Bouncer`'s own clear-and-re-seed plus `Plan-Revalidate`, and a fixer that flipped the flag early would gain nothing, since `Approve` writes it moments later anyway.

Note the neighbouring `Webster-Burler` `fasit.instructions` needs **no** change: it names "`Plan-Validate` and `Plan-Revalidate`" together, and together those two rows do still enforce all sixteen.

**`internal/planparser`.**
Read-only today — the package has no writer of any kind (`parse.go`, `validate.go`, `sections.go`, `normalize.go`, `classify.go` contain no `os.WriteFile`).
`SetApproved` is the package's first write path, so it also introduces the package's first write-side test fixtures.
Frontmatter is modelled by the unexported `overviewFrontmatter` struct (`parse.go:83-87`) with pointer fields, so absent and `false` are already distinguishable at parse time.
`splitFrontmatter` (`parse.go:180`) is the existing helper that separates the `---`-delimited block from the body and is the natural thing for the writer to reuse.
`PlanDir(anchorPath)` and `PlanOverview(anchorPath)` (`parse.go:66`, `:74`) are the package's own path declarers;
the writer must take a `planDir` (or reuse `PlanOverview`) and must never resolve cwd, per the Planparser Sole-Parser Invariant.

**`internal/shedadapters/bouncer.go`.**
`BouncerConfig.Commit` (around `:52-57`) is the field to mirror — read its doc comment for the "nil means commit nothing" phrasing to match.
`NewBouncer` validates every field before returning;
`Approve` needs no validation there beyond being permitted nil.
`settle` (`:~320-360`) is the single call site: the `case verdictApproved:` branch currently calls `Commit` and returns `shedengine.Done` with the round's ledger as the pointer.
Its doc comment already explains at length why a `Commit` failure is not routed through `degrade`;
that paragraph should be extended to cover `Approve` rather than duplicated.
Note the surrounding contract that the approved branch performs its side effects even under an already-cancelled context — a parsed verdict is never retracted for cancellation — so `Approve` inherits that too.

**`internal/shedrecipe`.**

- `entries_bouncer.go`: `configString(cfg, "commit_seam", false)` at `:44`, the `switch commitSeam` at `:75-90`, and the `configRejectUnknown(cfg, ...)` allowlist at `:64` are the three places `approve_seam` must be added.
  `requireSeam` is the existing helper the `plan` case must use.
- `entries_simple.go:118` `planValidateEntry` currently calls `configRejectUnknown(cfg)` with an **empty** allowlist, so adding `require_approved` means adding it to that call as well as reading it.
  `configBool(cfg, key, required)` already exists in `config.go:84`.
- `recipe.go`: `Env.CommitDiscussion` (`:92-94`) and `Env.CommitPlan` (`:103-105`) are the fields `ApprovePlan` sits beside;
  `Env`'s field docs name which producer reads each, so `ApprovePlan`'s doc must name `Bouncer`.
- `seam_enforcement_test.go` holds this package's import allowlist.
  No new import is needed — `ApprovePlan` is a `func() error` and is built by the caller.

**`internal/loomshed/planvalidate.go`.**
`NewPlanValidate(name, anchorPath, worktreeRoot)` gains the trailing `requireApproved bool` parameter, and `Call` (`:60-81`) picks between the two `planparser` functions.
Two doc comments in this file pin the old contract as "a thin wrap over `planparser`'s own parse and validate steps" naming `planparser.Validate` specifically — the package doc at `:1-3` and `Call`'s own doc at `:47-54` — and both move in the same commit to describe the two-mode wrap.
The existing `logger.Warn("loomshed: plan failed validation", ...)` line at `:76` already distinguishes the two rows by producer name and needs no change.
`formatPlanFindings` is shared and unchanged.

**`internal/loomcli`.**

- `wiring.go`: `CommitPlan` is filled around `:200-210` as a closure over `fabricengine.CommitAnchoredPaths`.
  `ApprovePlan` goes beside it, as `func() error { return planparser.SetApproved(planparser.PlanDir(location.AnchorPath())) }`.
  The file already imports `planparser` (it uses `planparser.PlanDirRel()` in the `CommitPlan` closure), so no new import.
- `validate.go:72-111` `validatePlanCmd` — the flag lands here;
  `Args: cobra.NoArgs` stays, and the **whole `Long` paragraph** (`:76-82`) is rewritten to describe both modes, not just its "it takes no arguments and no flags" sentence: it also claims the verb "runs `planparser.Validate` against it — the identical checks the Plan-Validate mechanical gate runs", and both halves of that become false in the new default mode.
  Per the CLI/Cobra Invariant every command carries a `Short`, which this one already does.
- `parity_test.go:156-200` `TestGateParity_PlanValidate` and its `planFixture(t, anchorPath, worktreeRoot, approved bool)` helper at `:169`.
  The existing `Stuck_Unapproved` case (`:176-181`) asserts that an unapproved plan maps to `stuck`;
  under the new split that is true only in `require_approved` mode, so the case must be re-keyed by mode rather than deleted.

**Existing tests that encode the old behaviour and must move with it.**

- `internal/loomshed/planvalidate_test.go` — `seedPlanValidateFixture(t, anchorPath, approved bool)` at `:19`, and the case at `:65` that seeds unapproved and expects the `plan-unapproved` stuck.
- `internal/loomshed/gatefindings_test.go:73-107` — builds a plan whose *only* validation failure is `approved: false` and asserts the warn line contains `plan-unapproved`.
  Disposition: **re-point the fixture, keep the default mode.**
  Seed a format-clean, approved plan plus one unindexed orphan `.md` file, so the single finding is `index-file-mismatch`, and assert the warn line on that ID instead.
  The test's subject is that exactly one finding reaches the log line — that is plumbing, not mode behaviour — so it should not become the one test that depends on `require_approved`, which `planvalidate_test.go`'s mode table already covers properly.
  It also reuses the same orphan-file corruption the `revalidate_test.go` disposition below settles on, so one verified idea covers both.
- `internal/planparser/validate_test.go:110-140` — `TestValidate_FormatAndApproval`, the direct test of the check being split.
- `internal/loomcli/validate_test.go:166-175` — `planFixture`'s own `approved` parameter.
- `internal/loomcli/parity_test.go:176` — `Stuck_Unapproved`.
- `internal/loomrecipe/fixture_test.go:393` — `fakeLoomShuttle`'s `"plan"` branch writing `planFixtureOverview(true)`, the self-approving `Plan-Write` stand-in that is why the nineteen-row `sequence_test.go` never caught F7.
  Its sibling `:545` `seedPlanValidateFixture(t, dir, true)` is inert at row 8 (rotated away) but flips for honesty.
  See the Testing section's keystone paragraph.
- `internal/loomrecipe/revalidate_test.go` and `fakeLoomBurler.corruptPlanOverview` (`fixture_test.go:114`, `:131-135`) — **`TestSequence_PlanRevalidateCatchesPostSegmentRegression` breaks under this change and must be re-pointed.**
  The fake burler injects its regression solely as `planFixtureOverview(false)`, i.e. by clearing the approval flag, and the test depends on `plan-unapproved` firing at `Plan-Revalidate`.
  The new `Approve` seam runs on `Plan-Bouncer`'s APPROVED settle, which is *after* the burler round, so the flag is flipped straight back to `true` and the bounce the test asserts stops happening.
  Disposition: change the fake to inject a regression that no approval write can undo.
  **The replacement must be parseable-but-invalid, and that constraint is sharp**: `planValidate.Call` maps a `ParsePlan` failure to a *returned error*, never to `Stuck` (`planvalidate.go:61-67`, and its own doc comment says why — "a plan that will not parse is not a plan the Plan-Write bounce target can be asked to improve").
  An unparseable corruption therefore aborts the whole run and the test dies at its `Run() error` fatal before reaching the `Stuck` → `Plan-Write` assertion it exists to make.
  Two corruptions are verified to satisfy the constraint:
  - **Preferred — an orphan card file.** Leave the overview and `01-first-card.md` exactly as they are and additionally write an unindexed `99-orphan.md` into the plan directory.
    `ParsePlan` only opens files the Card Index names, so it succeeds;
    `checkIndexFileConsistency` reads the directory and reports `index-file-mismatch` for the file no card names (`validate.go:99-130`).
    Purely additive, with no plan-format grammar to get right.
    The fake's field is renamed from `corruptPlanOverview` to match — it no longer writes an overview.
  - **Alternative — a card missing its `Intent:`.** Rewrite `01-first-card.md` with its heading and type label intact but no `**Intent:**`;
    the parser records `HasIntent` false rather than failing, and `checkCardMissingField` reports `card-missing-field`.

  Two corruptions that do **not** work, both of which look plausible: a Card Index entry naming an absent card file (`parseCardFile` hard-errors `card file not found`, `parse.go:279-297`), and dropping the Card Index entry while leaving the file on disk (`parseCardIndex` errors `no card index entries found` on an empty index, `parse.go:262`).
  Neither is an `index-file-mismatch` finding;
  both abort the run.
  The test's own subject and name are unchanged;
  only the corruption's flavour moves, and it moves to something the row could always catch and the seam can never mask.
- `internal/loomrecipe/shape_test.go:49`, `internal/loomshed/cancellation_test.go:86` — mechanical `NewPlanValidate` call-site updates for the new parameter.

**Pipeline behaviour after the fix, end to end.**
`Plan-Write` (writes `approved: false`, commits) → `Plan-Validate` (`ValidateFormat`, passes) → `Plan-Bouncer` seeds, `Plan-Burler` rounds, `Plan-Bouncer` judges → APPROVED settle: `Approve` writes `approved: true`, then `Commit` commits the whole plan directory → `Plan-Revalidate` (`Validate`, approval present, passes) → `Batchifier` → `Webster`.
The regression path stays coherent: if `Plan-Revalidate` finds a fixer-introduced format fault it bounces to `Plan-Write`, which rewrites the overview with `approved: false` again, and the segment re-enters — `Bouncer.Call`'s clear-and-re-seed archives the settled generation and judges afresh, so the stale APPROVED verdict cannot replay over a re-approved plan.

**Idempotence, and what it is actually for.**
Both seams are idempotent by construction: `SetApproved` over an already-approved plan is a no-op success, and `CommitAnchoredPaths` reports `committed == false` over an already-clean tracked path, which the `CommitPlan` closure already discards.
This is **not** because `settle`'s approved branch runs twice within one generation — it cannot.
`Bouncer.Call` (`bouncer.go:182-193`) archives the run directory and resets `n` to 0 the moment it finds an APPROVED verdict on disk at entry, so the approved branch of `settle` is reached at most once per generation.
Idempotence matters for the two paths that genuinely re-run the seams over an already-approved working tree:
a later generation approving a plan `Plan-Write` did not in fact rewrite, and the failed-settle resume described in the `a-failed-settle-seam-costs-one-review-generation-and-that-is-accepted` decision above, where the second generation's settle meets a file `SetApproved` already flipped and a commit `CommitPlan` may or may not have made.

## Constraints

From `CONSTRAINTS.md`:

- **Planparser Sole-Parser Invariant.**
  `internal/planparser` is the sole parser of `_lyx/plan/` and the sole declarer of the plan directory's path;
  it never resolves cwd and never imports `internal/lyxcwd`, taking the anchor path from its caller.
  This is what forces the approval writer into `planparser` and forbids `shedadapters`, `loomshed`, or `loomcli` from touching `00-overview.md` directly.
  The shipped entry's wording is parse-only, and this task introduces the format's first writer, so **the entry itself is edited in this commit** — `internal/planparser` becomes the sole parser *and* the sole writer of the on-disk plan format, with the same "no other package parses or writes `00-overview.md`/`NN-<card-slug>.md`" bullet and the same review-obligation enforcement note.
  Leaving the sole-writer property as an unwritten reading of a parse-only invariant is how the next task grows a second writer somewhere else.
- **Gate Self-Check Parity Invariant.**
  A mechanical gate's `ShedProducer` row and its CLI self-check verb call the same package function, and neither re-implements the other's check;
  the verb's envelope distinguishes a findings failure from an I/O fault structurally, by the presence of the `findings` key.
  Enforced by `internal/loomcli/parity_test.go`.
  This task turns one gate into a two-mode gate, so the invariant's entry must be updated to say the verb reaches every mode its row set uses, and the parity test must cover both.
- **Cwd Resolution Invariant.**
  `internal/lyxcwd` owns cwd resolution alone;
  every module owns its own relative subpath.
  The new `ApprovePlan` closure must take its anchor from the already-resolved `location`, exactly as the neighbouring `CommitPlan` closure does.
- **Lyxdirs Single-Declarer Invariant.**
  No hand-built `filepath.Join` naming the `_lyx` literal in production path construction — `SetApproved` must reach the overview through `PlanDir`/`PlanOverview`, which already compose `lyxdirs.LyxDirName`.
- **Shed Recipe Registry Invariant / told geometry.**
  `internal/shedrecipe` takes every absolute path from its caller and has no production import of `internal/lyxcwd`;
  its import allowlist lives in `seam_enforcement_test.go`.
- **CLI/Cobra Invariant.**
  Module `Command()`/`RunCLI` seam, a `Short` on every command, help-tree tests.
  Adding a flag to an existing command keeps this satisfied but the help-tree fixture may need regenerating.
- **Fabric Git Invariant.**
  Committing weft content is the loop owner's job, never an agent's.
  The approval write is loop-owner code, not an agent action, so it is on the right side of this line — and it is precisely why the write must be a Go seam rather than something the fixer round is told to do.
- **Documentation Lifecycle.**
  Docs land in the same commit — `manifest/designs/loom.md` (row 8 and row 10's table entries, plus the Plan-Validate detail subsection), `contracts/specs/loom-plan-spec.md` (the `plan-unapproved` row's framing and the sixteen-ID list's grouping), the plan stencil, and `CONSTRAINTS.md`.
  `manifest/roadmap.md` does **not** move: this is a defect fix on a shipped module, not a planned-item completion.

Discovered during exploration:

- `internal/planparser/validate.go`'s package doc names the sixteen check IDs and attributes the first two to `checkFormatAndApproval`.
  Splitting the check means that doc line changes with it, in the same commit.
- `Bouncer.settle`'s approved branch performs its side effects even under an already-cancelled context, by explicit design.
  `Approve` inherits that and must not add its own cancellation check.
- `shuttleengine` classifies a bouncer judge run complete only when every declared output file exists, which is why the verdict/ledger/focus output list is unconditional.
  Nothing about the approval write touches that list.

## Testing

**`internal/planparser` — the strongest TDD candidate in this task.**
`SetApproved` is new, pure-ish, and file-shaped, so write its table test first.
Cases: `approved: false` flips to `true`;
already-`true` is an idempotent no-op with the file byte-identical afterwards;
the key absent from an otherwise valid frontmatter block gets it inserted;
every other frontmatter key and its ordering survive;
a `root:` key and a multi-section body survive byte-for-byte;
a missing overview file errors;
a file with no `---` frontmatter block errors.
Round-trip the result through `ParsePlan` in at least the flip and insert cases and assert `plan.Approved` is `true`, so the writer and the parser are pinned against each other rather than against a hand-written expectation.

**`internal/planparser` — the validate split.**
`TestValidate_FormatAndApproval` becomes two tests: one over `ValidateFormat` asserting `format-unrecognized` fires and `plan-unapproved` never does regardless of the flag, one over `Validate` asserting `plan-unapproved` fires exactly when `!Approved`.
Add one case asserting `Validate`'s finding order still matches the spec's fixed order with `plan-unapproved` in position two, since the split moves where it is appended.

**`internal/shedadapters` — the `Approve` seam.**
Extend the existing `bouncer_commit_test.go` pattern rather than inventing a new harness.
Cases: an APPROVED settle with a non-nil `Approve` calls it exactly once and calls it **before** `Commit` (assert ordering with a shared call-log slice, not two independent booleans);
a nil `Approve` on an APPROVED settle commits normally and is not an error;
an `Approve` returning an error makes `Call` return that error with `shedengine.Done` **not** returned and `Commit` never called;
a BLOCKING settle never calls `Approve`.

**`internal/shedrecipe` — the two new config keys.**
Extend `entries_bouncer_test.go`: `approve_seam: plan` with a non-nil `Env.ApprovePlan` builds;
`approve_seam: plan` with a nil `Env.ApprovePlan` is a construction error;
an unknown `approve_seam` value errors and the message names `"plan"`;
an absent key builds with a nil seam;
and `approve_seam` is accepted by `configRejectUnknown` while a typo like `approve-seam` still is not.
Extend `entries_simple_test.go` for `require_approved`: absent, `true`, `false`, and a rejected unknown key.

**`internal/loomshed`.**
`planvalidate_test.go` grows a mode dimension: for each of the two modes, an unapproved-but-format-clean plan (stuck only in `require_approved` mode), a format-invalid plan (stuck in both), a clean approved plan (done in both), and an unparseable plan (returned error in both, never stuck).
`gatefindings_test.go`'s single-finding fixture is re-pointed to an approved, format-clean plan plus one unindexed orphan `.md` file, asserting the warn line carries `index-file-mismatch`, and stays in the default mode — see its entry in Technical context for why the mode table, not this test, is where `require_approved` belongs.

**`internal/loomcli`.**
`TestGateParity_PlanValidate` becomes a two-dimensional table: {clean approved, clean unapproved, format-invalid, absent plan directory} × {flag absent, `--require-approved`}, each cell asserting the verb's mapped verdict equals the matching row's.
The `clean unapproved` × `flag absent` cell is the one that proves the deadlock is gone — it must map to `done`.
Keep the envelope's structural `findings`-key discrimination, which is what the parity comparison keys off.
Add a help-tree/flag-registration assertion for `--require-approved` if the existing help-tree test does not pick it up automatically.

**Recipe-level wiring — the keystone, and the mechanism is already in the repo.**
Assert `contracts/recipes/loom-recipe.yaml` still builds against a fully-populated `Env` with the two new keys present, and that a recipe naming `approve_seam` on a row whose `Env` seam is nil fails to build rather than building silently.

The keystone assertion needs no new harness: `internal/loomrecipe/fixture_test.go`'s `buildSequenceFixture` is exactly the vehicle.
It is an untagged Tier-1, fully offline fixture that builds the **real** producer list from the embedded recipe via `New`, over a temp anchor, with `fakeLoomShuttle` and `fakeLoomBurler` standing in for every LLM call, a real stencils directory supplied through `contracts/stencils` + `stencilstore` (which is what satisfies `NewBouncer`'s eager rubric probe), and `fakeAlwaysDoneProducer` substituted for row 1.
`sequence_test.go`'s `wantSequenceOrder` already drives that fixture through all nineteen history rows, scripting each segment's APPROVED verdict through the fake shuttle, and `revalidate_test.go` already re-uses it to prove the `Plan-Revalidate` → `Plan-Write` bounce.
So the seed-verdict-report machinery the assertion needs is shipped and working.

**The one thing that fixture gets wrong is the whole reason F7 escaped CI, and the offending line is the fake `Plan-Write`.**
`fakeLoomShuttle`'s `spec.Role == "plan"` branch — the stand-in for row 6, `Plan-Write` — writes `planFixtureOverview(**true**)` at `fixture_test.go:393`.
The fake writer **self-approves**, which is exactly what `loom-template-plan.md:79` forbids the real writer from doing.
The fixture therefore hands row 8 a plan the production `Plan-Write` can never produce, the nineteen-row sequence test passed continuously, and the live pipeline could not clear row 8.

Note the line that is *not* to blame, because the round-2 draft of this section named it wrongly: `fixture_test.go:545`'s `seedPlanValidateFixture(t, dir, true)` never reaches `Plan-Validate` at all.
`loomshed.NewPlanWrite`'s rotation archives every top-level `.md` file in the plan directory before the shuttle runs — `fixture_test.go:283-287` documents this as the reason the `"plan"` branch rewrites the whole directory rather than only `spec.OutputFiles` — so the seeded overview is gone by row 8 and its `approved` value is inert there.
Flip it to `false` as well for honesty, but the regression rides entirely on `:393`.
mill-plan must check whether any other test in `internal/loomrecipe` (e.g. `resume_test.go`) depends on that seed's value before flipping it.

**The regression test is one character.**
Change `fixture_test.go:393` to `planFixtureOverview(false)`, so the `Plan-Write` stand-in obeys the stencil it is standing in for, and fill `env.ApprovePlan` with a closure running the real `planparser.SetApproved` over the fixture's own plan directory.
`sequence_test.go`'s `wantSequenceOrder` must then still hold unchanged — all nineteen rows, `Plan-Validate` Done through `Plan-Revalidate` Done to `Batchifier`.
Under today's code that flip makes `Plan-Validate` report Stuck and the sequence bounce;
under the fix it passes.
It is the assertion that would have caught F7 on the day row 8 landed.
Add one further assertion in the same test that the fixture's `00-overview.md` carries `approved: true` after the run, so a fixture that silently stopped exercising the seam cannot pass.

**The negative case, by a mechanism that is actually reachable.**
Removing `approve_seam` from the `Plan-Bouncer` row is *not* reachable through `buildSequenceFixture`: `loomrecipe.New` parses the embedded `recipes.LoomRecipe` unconditionally at `loomrecipe.go:86`, and leaving `env.ApprovePlan` nil against the shipped recipe makes `New` fail at `requireSeam` before the run starts.
Express it two ways instead:

- **Dynamic**, through the shipped fixture: substitute `env.ApprovePlan` with a **non-nil no-op** closure.
  `requireSeam` only checks non-nil, so construction succeeds, the segment approves, nothing writes the flag, and the run must halt at `Plan-Revalidate` with `plan-unapproved` and bounce to `Plan-Write` rather than reaching `Batchifier`.
  This pins that `Plan-Revalidate` is genuinely enforcing the approval it is now the only row to check.
- **Static**, over a hand-authored recipe document: assert the two new keys are wired as shipped, and that mis-wirings are rejected, by parsing modified YAML through `shedbuild.Parse([]byte(...))` exactly as `internal/loomrecipe/overlay_seam_guard_test.go:270` already does.
  Cases: the shipped `recipes.LoomRecipe` carries `approve_seam: plan` on `Plan-Bouncer` and `require_approved: true` on `Plan-Revalidate` and on no other row;
  a document naming `approve_seam` on a row whose `Env` seam is nil fails to build;
  an unknown `approve_seam` value fails to build.

**Not covered here.**
A live driven loom run through `Finalize`.
That is the follow-on task this change exists to unblock.

## Q&A log

- **Q:** Where should the approval write live — on `Plan-Bouncer`'s APPROVED settle, or a new dedicated row? **A:** [auto-pick] `Approve func() error` seam on `BouncerConfig`, called before `Commit`, selected by an `approve_seam: plan` recipe key. **Why:** it is the only point that guarantees the flag lands inside the commit `Commit` makes, and it reuses the already-shipped `commit_seam` pattern instead of adding a row, a `Name*` constant, and a second commit.
- **Q:** How should `Plan-Validate` stop demanding approval while `Plan-Revalidate` keeps enforcing it? **A:** [auto-pick] split `planparser` into `ValidateFormat` and `Validate`, with a `require_approved` recipe key on the shared `PlanValidate` engine selecting between them. **Why:** two named functions leave the four consumer call sites untouched and match the spec's own consumer-guard framing of `plan-unapproved`.
- **Q:** What shape should the `planparser` writer take? **A:** [auto-pick] `SetApproved(planDir string) error` — surgical frontmatter-line rewrite, idempotent, inserting the key when absent. **Why:** a YAML round-trip would not preserve the overview body, and insert-if-absent makes the seam total over every plan `ParsePlan` already accepts.
- **Q:** How does `lyx loom validate-plan` pick a mode without breaking Gate Self-Check Parity? **A:** [auto-pick] a `--require-approved` bool flag defaulting to `false`, with the parity test extended to both rows. **Why:** two rows now share the engine in two modes, so a single-mode verb would leave one row with no self-check;
  the default matches the verb's documented pre-handoff user.
- **Q:** Does the plan stencil keep ordering `approved: false`? **A:** [auto-pick] yes, unchanged — only the clause promising "a future review gate" is corrected to name `Plan-Bouncer`'s approved settle. **Why:** the writer must not self-approve;
  the rule was right and only its tense was wrong.
- **Q:** What happens when the `Approve` closure fails inside `settle`? **A:** [auto-pick] returned as `settle`'s own error, never through `degrade`, and `Commit` is not attempted. **Why:** `degrade` only returns `Stuck`, so routing an I/O fault through it would silently convert an approval into a rejection — the same reasoning the existing `Commit` branch already carries.
- **Q:** Which values does `approve_seam` accept? **A:** [auto-pick] `"plan"` only;
  absent leaves the seam nil, anything else is a construction error. **Why:** there is no discussion-side or webster-side approval flag, so a second value would be hypothetical.
- **Q:** Do Webster, `webstercli`, and `Batchifier` change? **A:** [auto-pick] no — they keep calling the full `Validate` and keep refusing an unapproved plan. **Why:** they run after approval, and the refusal is the standalone-invocation guard the spec's "else refuse to run" wording was written for.
- **Q:** (review round 2 gap) What happens when `Approve` or `Commit` fails and the operator resumes — `shedengine.Run` re-calls `Plan-Bouncer`, which archives its own APPROVED verdict and spends a whole new LLM review generation? **A:** [auto-pick] accepted as-is and recorded, with no retry loop and no short-circuit;
  the state converges because both seams are idempotent and the judge never reads the approval flag. **Why:** the durable `settled`-marker short-circuit that would eliminate the cost changes the generic `Bouncer`'s on-disk contract for two segments this task does not otherwise touch, which is the scope growth F7's own fixer report warned against — filed as a follow-up instead.
- **Q:** (review round 2 gap) How does the keystone routing assertion reach an APPROVED settle without an LLM run — the shuttle, the rubric-stencil probe, and the verdict/ledger/report set all have to come from somewhere? **A:** [auto-pick] reuse `internal/loomrecipe/fixture_test.go`'s existing `buildSequenceFixture`, which already supplies all three offline. **Why:** tracing the mechanism found the actual reason F7 survived CI, and round 3 corrected which line carries it — `fakeLoomShuttle`'s `"plan"` branch at `:393` writes `planFixtureOverview(true)`, so the fake `Plan-Write` self-approves in a way the real one is forbidden to.
  Flipping that one argument to `false` is both the fixture fix and the regression test.
  The round-2 draft blamed `:545`'s seed instead, which `NewPlanWrite`'s rotation discards before row 8 ever reads it.
- **Q:** (review round 3 gap) How is the negative case expressed, given `loomrecipe.New` parses the embedded recipe unconditionally and a nil `Env.ApprovePlan` fails at `requireSeam` before the run starts? **A:** [auto-pick] two ways — dynamically by substituting a non-nil **no-op** `env.ApprovePlan` in the shipped fixture, and statically by parsing hand-authored recipe YAML through `shedbuild.Parse` as `overlay_seam_guard_test.go:270` already does. **Why:** the no-op closure passes `requireSeam`'s non-nil check while writing nothing, which is exactly the condition `Plan-Revalidate` must catch;
  the static half pins the shipped recipe's own key wiring, which no dynamic run asserts.
- **Q:** (review round 3 gap) `TestSequence_PlanRevalidateCatchesPostSegmentRegression` injects its regression as `approved: false`, which the new `Approve` seam undoes — what happens to it? **A:** [auto-pick] re-point `fakeLoomBurler.corruptPlanOverview` to a genuinely format-invalid corruption, an `index-file-mismatch` from a Card Index entry naming an absent card file. **Why:** the seam runs after the burler round, so any approval-flag corruption is masked by construction;
  the test's subject is that `Plan-Revalidate` catches a post-segment regression, and a format fault tests that subject better than the flag ever did.
- **Q:** (review round 2 gap) Does the plan stencil's Step 5 self-check block change? **A:** [auto-pick] it stays verbatim. **Why:** Step 5 is the writer-facing half of the same deadlock, and the verb's new default mode is exactly what makes "re-run it until it exits 0" satisfiable;
  adding `--require-approved` there would re-impose the deadlock on the one agent that must never satisfy it.
- **Q:** (review round 4 gap) The round-3 replacement corruption for `revalidate_test.go` — a Card Index entry naming an absent card file — makes `ParsePlan` hard-error, so the row returns an error and the test aborts instead of bouncing. What replaces it? **A:** [auto-pick] an **orphan card file**: leave the overview and `01-first-card.md` alone and additionally write an unindexed `99-orphan.md`, which parses cleanly and yields `index-file-mismatch`. **Why:** `planValidate.Call` maps a parse failure to a returned error and only a *validation* finding to `Stuck`, so the corruption must be parseable-but-invalid;
  the orphan file is purely additive with no plan-format grammar to get wrong.
  The reviewer's own alternative — dropping the index entry — fails the same way, since `parseCardIndex` errors on an empty index.
- **Q:** How deep does verification go in this task? **A:** [auto-pick] unit tests per package plus a recipe-level routing assertion that an approved plan reaches `Batchifier` with no bounce;
  no live LLM run. **Why:** a live end-to-end run is the thing this change unblocks and belongs to the follow-on task, while the routing assertion is what would have caught F7 in the first place.
