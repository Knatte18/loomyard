# Batch: row-removal

```yaml
task: 'Producer gates: mechanical gates before session release'
batch: 'row-removal'
number: 5
cards: 9
verify: go test ./internal/loomshed/... ./internal/shedrecipe/... ./internal/loomrecipe/... ./internal/loomcli/...
depends-on: [4]
```

## Batch Scope

This batch removes `Discussion-Validate`, `Plan-Validate`, and `Plan-Revalidate` and everything that exists only to serve them, taking the recipe from seventeen rows to fourteen.
The gates landed in batch 4 already run the identical checks on the identical artifacts, strictly earlier and un-skippably, so nothing is lost by the removal — what is gained is that a `Burler` round's own output is now checked, which no standalone row ever covered.

There is **no resume migration**: a run parked on a removed row when the new binary lands is restarted, not migrated.
Row names are durable on-disk identities and a rename breaks resume for in-flight tasks; removal is a stronger version of the same fact, and a permanent migration table mapping three retired names onto successors is carrying cost a pre-release, single-operator system does not need when the operator can simply re-run.

Batch-local decision: this batch's cards are one coordinated removal and only the batch as a whole builds.
Card 29 deletes two producer files whose constructors three other packages still reference until cards 30 through 34 land, so the intermediate per-card commits are deliberately not individually green and `verify:` is a batch-level gate here, as it is for every batch.

Batch-local decision: two standing guards lose their subject entirely and are deleted rather than retargeted, each with its replacement named in the deleting commit.
`TestSequence_NoOpApproveSeamBouncesAtRevalidate` pinned that a no-op approve seam is caught by a later row; nothing re-checks the approval flag after the seam writes it any more, so the property that survives is the sequence test's own trailing `planparser.ParsePlan` assertion that the plan is approved after the run, which is the standing guard that the seam genuinely ran.
`revalidate_test.go`'s whole subject is the removed row.

## Cards

### Card 28: Remove the three rows from the recipe and rewire the graph

- **Context:**
  - `contracts/recipes/recipes.go`
  - `internal/shedrecipe/entries_gate.go`
- **Edits:**
  - `contracts/recipes/loom-recipe.yaml`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Delete the `Discussion-Validate`, `Plan-Validate`, and `Plan-Revalidate` row blocks and rewire the three edges around them: `Discussion-Write`'s `on_done` becomes `Discussion-Bouncer`, `Plan-Write`'s becomes `Plan-Bouncer`, and `Plan-Bouncer`'s becomes `Batchifier`.
  Correct the header comment's "seventeen row names" to fourteen, and drop the removed rows' own rationale paragraphs with them.
  Rewrite the header's escalate-set paragraph for two consequences: `Plan-Write`'s `Stuck` becomes reachable for the first time, since nothing made it fire before, and `Discussion-Write` gains a second reachable path alongside the asking one the paragraph already names.
  The count of escalating rows does not change, because both rows were already in that set — say so, so a reader auditing routing from the header is not left recounting.
  Add a header sentence recording that a gated writer row's exhausted gate halts the run for a human rather than bouncing to a fresh writer, that this replaces the removed rows' cheap respawn deliberately, and that no `on_stuck` is added to either writer row: after three in-session re-prompts with the findings in hand, a cold respawn that knows nothing about the complaint is strictly worse than stopping and telling someone.
  Add a header sentence recording that an in-flight run parked on one of the three removed rows is restarted rather than migrated, and why.
  Rewrite all **three** `fasit.instructions` blocks that tell a fixer its mechanical checks are already enforced upstream — the Discussion-Burler block naming `Discussion-Validate`, the Plan-Burler block naming `Plan-Validate` and `Plan-Revalidate`, and the Webster-Burler block naming both plan rows.
  This is one more block than the surrounding discussion's own count of two, and the third is the Webster one; leaving it would tell that round its subject's format checks are enforced by rows that no longer exist.
  Each rewritten block says that the mechanical checks are enforced by the round's own gate over the round's own output, which is what makes re-deriving them in the round duplicated work — the claim is now *stronger* than the one it replaces, not merely relocated, because the old rows never checked what a fix round rewrote.
  Do not touch `max_bounces`, `segment`, `commit_seam`, `approve_seam`, or any gate key this batch's predecessor added.
- **Commit:** `feat(recipe): remove the three standalone validate rows and rewire the graph`

### Card 29: Delete the two validate producers and everything serving them

- **Context:**
  - `internal/shuttleengine/gate.go`
  - `internal/shedrecipe/entries_gate.go`
  - `internal/shedrecipe/config.go`
  - `internal/shedrecipe/recipe.go`
  - `internal/planglyph/repo.go`
  - `internal/discussionparser/validate.go`
- **Edits:**
  - `internal/loomshed/gates.go`
  - `internal/loomshed/loomshed.go`
  - `internal/loomshed/interruptpolicy.go`
  - `internal/loomshed/interruptpolicy_test.go`
  - `internal/shedrecipe/entries_simple.go`
  - `internal/shedrecipe/registry.go`
  - `internal/shedrecipe/registry_test.go`
  - `internal/shedrecipe/entries_simple_test.go`
- **Creates:** none
- **Deletes:**
  - `internal/loomshed/discussionvalidate.go`
  - `internal/loomshed/discussionvalidate_test.go`
  - `internal/loomshed/planvalidate.go`
  - `internal/loomshed/planvalidate_test.go`
  - `internal/loomshed/cancellation_test.go`
- **Moves:** none
- **Requirements:** Delete both producer files and their own tests, and move the three helpers the gate closures already call — `formatDiscussionFindings`, `formatPlanFindings`, and `hasBlockingFinding` — into the gates file, each keeping its existing doc comment verbatim, because each carries a hard-won rationale that must survive its file.
  `hasBlockingFinding`'s in particular records that `planglyph.Severity` is an open string type, so testing not-informational rather than equals-blocking is what keeps an unrecognized or zero-valued severity from silently passing.
  Delete `NameDiscussionValidate`, `NamePlanValidate`, and `NamePlanRevalidate` from the row-name constant block and their three entries from the `InterruptPolicies` map, correcting both files' "seventeen" prose to fourteen; drop the retired constant from the interrupt-policy test's table and re-point that row onto a surviving reinvoke row.
  Delete `discussionValidateEntry` and `planValidateEntry` from the simple-entries file, their `"DiscussionValidate"` and `"PlanValidate"` keys from the registry map, and correct the registry's own "complete at eighteen keys" count to sixteen; drop the two names from the registry test's expected-name list and the two rows from the simple-entries test's table.
  Correct the two `Env` field doc comments naming `DiscussionValidate` as the reader of `DecisionRecordPath`/`SupportLogPath` and `PlanValidate` as a reader of `AnchorPath`/`WorktreeRoot`, which are now read by the gate resolver instead.
  The whole cancellation test file goes with the producers: its own doc comment states that it is in this suite because it constructs those two producers, and it has no other subject.
  Keeping the producers as dead code for a release was rejected: nothing would construct them, and the coverage guard would need an exemption to tolerate that.
- **Commit:** `refactor(loomshed): delete the two validate producers and their registry entries`

### Card 30: Retarget the findings-surfacing guard onto the gate closures

- **Context:**
  - `internal/loomshed/gates.go`
  - `internal/loomshed/gates_test.go`
  - `internal/loomshed/loompreflight.go`
- **Edits:**
  - `internal/loomshed/gatefindings_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** The file's subject — that this package's mechanical gates surface the determined findings they used to discard — survives the row removal intact, and the file is carried over rather than deleted with the producers it happened to test.
  Replace its two deleted-producer cases with the equivalent assertions against `NewDiscussionGate` and `NewPlanGate`, keeping the existing Loom-Preflight case untouched.
  Rewrite the file's doc comment for the new shape: the rule still matters most exactly where it is cheapest to skip, but the reason has changed — a gated writer row's exhausted gate now **halts the run for a human** rather than bouncing to a respawned writer, so the `logger.Warn` line is not merely the best record of a refusal but the only one that outlives the run directory the findings file is deleted with.
  Where the gate-closure cases would now duplicate coverage the gates test file already carries, keep the assertion here and say in the comment which file owns which half, so the two do not silently drift into one testing the other's subject.
- **Commit:** `test(loomshed): retarget the findings-surfacing guard onto the gate closures`

### Card 31: Re-point the loomrecipe row tables at fourteen rows

- **Context:**
  - `contracts/recipes/loom-recipe.yaml`
  - `internal/loomshed/loomshed.go`
  - `internal/loomshed/interruptpolicy.go`
  - `internal/loomrecipe/loomrecipe.go`
  - `internal/loomrecipe/names.go`
  - `internal/loomrecipe/interruptpolicy_meta_test.go`
- **Edits:**
  - `internal/loomrecipe/coverage_guard_test.go`
  - `internal/loomrecipe/shape_test.go`
  - `internal/loomrecipe/recipe_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Drop the three removed rows from `loomRowEngines` and correct that var's "seventeen row names" prose to fourteen.
  In the shape test's row table, delete the three removed rows' entries and correct the two writer rows' and Plan-Bouncer's expected `on_done` targets to `Discussion-Bouncer`, `Plan-Bouncer`, and `Batchifier` respectively; correct the file's own "seventeen-row producer table" prose in its header and in the order-stability test's comment.
  Correct the recipe test's "exactly seventeen rows" assertion and prose to fourteen.
  Retarget the shape test's validate-driving test — the one whose fake shuttle deliberately writes nothing so a real producer bounces until a budget is exhausted: with the validate row gone, the non-writing fake now makes `Discussion-Write`'s own gate fail, and the row carries no `on_stuck`, so the run blocks at `Discussion-Write` rather than at the removed row.
  Update its expectation and both of its comments to that shape, keeping its actual subject — that driving `Run` exercises the unexported `validate()` indirectly and that an ordinary blocked outcome is not a validation failure — unchanged.
  The interrupt-policy meta test derives its expectations from the rows `New` assembles and therefore needs no edit; it is listed as context so a reader does not add one.
- **Commit:** `test(loomrecipe): re-point the row tables and guards at the fourteen-row recipe`

### Card 32: Shorten the sequence and drop the two rows' own tests

- **Context:**
  - `contracts/recipes/loom-recipe.yaml`
  - `internal/loomrecipe/fixture_test.go`
  - `internal/planparser/parse.go`
- **Edits:**
  - `internal/loomrecipe/sequence_test.go`
  - `internal/loomrecipe/approveseam_test.go`
- **Creates:** none
- **Deletes:**
  - `internal/loomrecipe/revalidate_test.go`
- **Moves:** none
- **Requirements:** Drop the three removed rows' entries from `wantSequenceOrder`, taking it from nineteen entries to sixteen, and rewrite the list's doc comment accordingly: the Plan-Review segment no longer carries the trailing post-segment mechanical re-check the other two segments have no counterpart for, so all three segments now have the identical three-entry shape.
  Update the paragraphs explaining why the two writer rows pass, which currently reason via the row below them: each now passes because the fixture's fake shuttle writes an artifact the row's own gate accepts, and the gate runs inside that fake's gated method rather than as a separate row.
  Keep the trailing `planparser.ParsePlan` approved-flag assertion and strengthen its comment: with no post-segment re-check row left, this assertion is the only standing guard that Plan-Bouncer's approve seam genuinely ran.
  Delete `revalidate_test.go` outright — its single subject is the removed row.
  In the approve-seam test file, delete `TestSequence_NoOpApproveSeamBouncesAtRevalidate`, whose premise is that a later row catches a no-op seam, and record in the file's own doc comment that the property now rests on the approve seam failing loudly plus the sequence test's trailing approved-flag assertion, with no row re-checking the flag by design.
  Rewrite `TestShippedRecipe_ApproveSeamWiredOnPlanBouncerOnly` to assert `approve_seam: plan` on Plan-Bouncer and `require_approved` present on **no** row at all, since the key is no longer recognized by any registry entry and a row carrying it would now fail construction outright.
- **Commit:** `test(loomrecipe): shorten the sequence to fourteen rows and drop the revalidate guards`

### Card 33: Retarget the bounce-routing and resume guards

- **Context:**
  - `contracts/recipes/loom-recipe.yaml`
  - `internal/loomshed/loomshed.go`
  - `internal/shedengine/producer.go`
  - `internal/shedadapters/burler.go`
- **Edits:**
  - `internal/loomrecipe/resume_test.go`
  - `internal/loomrecipe/fixture_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Three guards in the resume test file drive the removed discussion validate row as their bouncing producer and must be retargeted rather than deleted, because each one's actual subject survives.
  Retarget `TestBounceRouting_StuckContinuesAtDeclaredTarget` onto the Discussion-Review segment's mutual `on_stuck` pair: with the fixture's bouncer verdict scripted to a non-approving value, `Discussion-Bouncer` goes `Stuck` and the run must continue at its declared target, `Discussion-Burler`, immediately afterward — the same assertion shape over a pair that still exists.
  Retarget `TestBounceRouting_BudgetExhaustionBlocks` onto the same pair, asserting against the Bouncer row's own recipe-declared `max_bounces` rather than a small `ShedPaths.MaxBounces`, since both segment rows declare a budget of their own that the paths value no longer overrides; assert that the run halts at `Discussion-Bouncer` with that row's budget plus one `Stuck` entries.
  Record in the test's comment why the Bouncer binds rather than the Burler: its `Stuck` sequence runs one ahead of the round producer's count, so with equal budgets it exhausts first in a segment's first generation.
  Rename and retarget `TestResume_DiscussionValidateBounceRespawnsDiscussionWrite` onto the row it was always really about: plant `current_producer` at `Discussion-Write` with both discussion artifacts already present on disk — the identical on-disk shape a crash mid-interview leaves — and assert that the row respawns rather than reporting `Done` off bare file existence, that the attach probe ran first, and that the respawned run itself reports `Done`.
  The bounce half of its assertions goes away with the row that produced the bounce; the load-bearing half, which is the whole "interactive-mode trap" regression, survives and is now reached by a resume rather than by a bounce.
  Leave `TestBounceRouting_EmptyTargetBlocksInstead` untouched: it drives Batchifier, which is unaffected.
  In the fixture file, rename `seedPlanValidateFixture` to a name that does not carry a removed row's identity and update its and `fakeLoomShuttle`'s doc comments, which currently explain their write behaviour by naming the row below each writer.
- **Commit:** `test(loomrecipe): retarget the bounce-routing and resume guards onto surviving rows`

### Card 34: Rewrite the gate-parity test against the closures

- **Context:**
  - `internal/loomshed/gates.go`
  - `internal/loomcli/validate.go`
  - `internal/loomcli/validate_test.go`
  - `internal/shedrecipe/recipe.go`
  - `internal/planglyph/repo.go`
- **Edits:**
  - `internal/loomcli/parity_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Replace both parity tests' producer halves with the gate closures: `producerVerdict` becomes a mapping over `(shuttleengine.GateResult, error)` — a non-nil error is the error verdict, `Passed` false is the stuck verdict, and `Passed` true is the done verdict — and each test drives `NewDiscussionGate` or `NewPlanGate` over the same fixture the verb reads.
  The CLI half is unchanged: the verbs do not change in this batch, and the three-way comparison stays three-way so a stuck-versus-error mismatch is caught rather than collapsed onto a single fail side.
  Reduce the plan test's mode cross-product to the flag-absent mode only, since both plan gate sites run `planglyph.ValidateFormat` and neither has a `--require-approved` counterpart, and add an explicit assertion or comment recording that the verb's `--require-approved` mode has no recipe counterpart by design — the flag's guarantee rests on the approve seam failing loudly, not on a row.
  One fixture diverges deliberately and must be marked as an expected divergence rather than quietly dropped or made to pass: the absent-plan-directory fixture reaches the stuck verdict at the gate and the error verdict at the verb, because the gate's `ParsePlan` carve-out routes a missing or malformed overview to findings while the verb keeps returning it as an error.
  Its comment must state that the invariant binds the two sides to the same package *function*, which both still satisfy — `planglyph.ValidateFormat` — and that the divergence lives strictly in the `ParsePlan` pre-step, whose disposition the gate deliberately reverses because its bounce target is the live session that just wrote the file rather than a cold respawn.
  Every other fixture must still agree on both sides: clean-approved, unapproved, format-invalid, glyph-not-resolving, informational-only, and quarry-unavailable on the plan side, and all four discussion fixtures.
  Update the file's own doc comment to name the four gate sites the two pairs now cover rather than the two removed rows.
- **Commit:** `test(loomcli): rewrite the gate-parity test against the two gate closures`

### Card 35: Pin that no gated site authorizes fork subagents

- **Context:**
  - `contracts/recipes/loom-recipe.yaml`
  - `contracts/recipes/recipes.go`
  - `internal/burlerengine/engine.go`
  - `internal/loomengine/discussion.go`
  - `internal/loomengine/plan.go`
  - `internal/shuttleengine/spec.go`
  - `internal/loomrecipe/fixture_test.go`
- **Edits:** none
- **Creates:**
  - `internal/loomrecipe/gatequiescence_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** The per-attempt done-signal is the next turn boundary and nothing more, and that narrowing holds only because no gated site can spawn an async in-process subagent whose work outlives the turn.
  Batch 1 pinned the first half of that precondition — the in-process `Agent` tool is denied by the shipped shuttle template.
  Pin the second half here: parse the real embedded recipe and assert that every row carrying a `gate:` key either is a writer row whose spec source sets no `ForkSubagents`, or is a burler row carrying no `cluster-fan` key in its `profile:` sub-map — the round engine sets `ForkSubagents` from `ClusterFan != ""` and from nothing else, so an absent fan is what keeps a gated round's spec unforked.
  Assert in the same test that the Webster round, the one row that could legitimately grow a fan, carries no `gate:` key.
  The failure message must state that a gated site gaining fork subagents re-opens the compound-quiescence question this task deliberately declined — turn-idle alone would no longer mean the agent is finished — and that the decision must be re-opened rather than the test updated.
  Keep the test untagged and offline: it parses the embedded recipe and reads no worktree.
- **Commit:** `test(loomrecipe): pin that no gated row authorizes fork subagents`

### Card 36: Confirm the removal is complete in code

- **Context:**
  - `internal/loomshed/loomshed.go`
  - `internal/shedrecipe/registry.go`
  - `contracts/recipes/loom-recipe.yaml`
- **Edits:** none
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Verification only, no edit.
  Grep the whole worktree, excluding the git directory and the task's own mill directory, for the five strings naming the removed rows and their engines, and confirm that every surviving hit is prose in a file batch 6 owns — documentation, a deployed stencil, a deployed spec, or a Go doc comment — and that no hit is a live Go identifier, a registry key, a recipe row name, or a config key.
  Record the surviving hit list in the batch's own round notes so batch 6 starts from a verified inventory rather than re-deriving one.
  If any hit is a live identifier, it belongs to this batch and must be fixed here before the batch is reported complete.
- **Commit:** none

## Batch Tests

`verify: go test ./internal/loomshed/... ./internal/shedrecipe/... ./internal/loomrecipe/... ./internal/loomcli/...` covers every package holding a Go symbol that names a removed row.
`internal/loomcli` is in scope because the parity test constructs the two deleted producers directly and would otherwise fail to compile; card 34 is what makes that package green again.
The load-bearing surface is `internal/loomrecipe`, which rebuilds the real embedded recipe in every one of its guards: the coverage guard proves the fourteen row names and their engines still line up, the shape test proves each row's concrete producer type and rewired edges, the routing-graph test proves no Bouncer/Burler pair was mis-wired by the edge changes, the sequence test proves the shortened run still reaches Publish, and the new quiescence test proves the narrowing the whole gate design rests on.
The two deliberate deletions — the revalidate guard and the no-op-approve-seam guard — are named in `## Batch Scope` with the guard that replaces each.
No tagged test is edited by this batch; the smoke file batch 4 added is unaffected by the removal.
