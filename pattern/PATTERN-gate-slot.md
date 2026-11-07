# PATTERN-gate-slot

This cluster guards running and clearing a gate: in held `internal/gateslot` slots, verify via `verifytree.Verify`, and a cold fixer only via `gatefixer`, judged by Go's re-run.

## Gate slot

Rule: Running gate build, test or verify: in held `internal/gateslot` slots, verify via `verifytree.Verify`, a tier skipped only on `lym gate test`'s green record; a gate and its self-check call one function. (test)

Running a gate build or test holds `internal/gateslot` slots,
so the hub never runs more at once than its configured count.

- A gate build or test runs only holding `internal/gateslot` slots, in one of two places.
- Inside a `lym gate test` tier run, holding the slots its tier asks for;
  the tier's config sets its parallelism and environment.
- In one of the non-tier holders `internal/gateslot`'s doc lists, holding one slot;
  the holder sets its own `-trimpath` Go flag, plus `-p` from `tier0`'s parallel when the hub declares `tier0`.
- Every Go gate build keeps `-trimpath`,
  so a fresh worktree reuses the build and test cache;
  a `dotnet` build takes no Go flags.
- No Go file of the module imports `"C"`:
  the shared cache keeps a cgo package's generated files naming the source by absolute path.
  The path is that of the checkout that first built it, so `go vet` in another worktree opens that checkout's files.
  A cgo dependency in the module cache sits at the same path in every worktree and stays allowed.
  Enforced by `cmd/lym/nocgo_test.go`.
- The hub's shared build step runs once per `lym gate test` call, inside the slots of the first tier that opts into it.

### Trimpath-safe root

Code a gate run builds never locates the module root from a `runtime.Caller` path alone;
`-trimpath` strips the absolute path that call would return.

### Gate carry

A test tier is skipped as already green only on `lym gate test`'s own green record, and only over a clean tree.

### Self-check parity

A mechanical gate's **closure** and its CLI self-check verb call the same package function for every mode.

#### Pairs

- Discussion-Write's and Discussion-Burler-Fix's gates and `validate-discussion`:
  `loomshed.ValidateDiscussion` runs `discussionparser.Validate` and `discussionparser.CheckDecisionIDs`.
  It also runs the history check `discussionparser.CheckAddedDecisionsKept`.
  For a Discussion-Write record it adds the citation check `citationFindings`.
  That check runs over `discussionparser.Citations` and `discussionparser.CheckRelevant`.
- Plan-Burler-Fix's gate, a Plan-Write run's gate whose record carries no fork authorization and `validate-plan`:
  `loomshed.ValidatePlan` runs `planindex.Index.ValidateFormat`, implemented by `planglyph`, plus the done-card checks.
- A twill-spawned Plan-Write run's gate and `validate-plan --lens-review`:
  `loomshed.ValidatePlanLensReview` adds a check that `lens-review.md` is present;
  the run's recorded fork authorization selects it.
- PR-Rework's gate and `validate-plan --rework`:
  `loomshed.ValidateReworkPlan` checks the whole new plan plus the told `first_card`.
- `validate-plan --overlay`, a self-check mode no gate runs:
  it calls `loomshed.ValidatePlan`, the flag-less mode's function, over a plan parsed through a `planparser.Overlay`.
- Describe's gate and `validate-description`: `summaryparser.ValidateDescription`.
- The webster `verify` gate (`websterengine.NewVerifyGate`) and `lym webster verify`: `verifytree.Verify`.
  Webster-Burler-Fix's `verify` gate (`loomshed.NewVerifyGate`) goes through `verifytree.Verify`:
  it runs the comment lint and the derived round command.
- A gate key's `comments` lint step in `lym gate test` and `lym loom lint-comments`: `commentlint.Lint`.
  The Webster-Burler-Fix round gate (`loomshed.NewVerifyGate`) joins this pair when it runs the lint.
- A gate key's `pattern` lint step in `lym gate test` and `lym loom lint-pattern`:
  `patternlint.Lint` over `pattern.Headroom`.
  The Webster-Burler-Fix round gate (`loomshed.NewVerifyGate`) is the pair's third caller when it is told a merge base.
- Every burler round's `review` gate and `lym burler validate-review`:
  `burlerengine.CheckReviewFile`, over `ParseReview`.

#### Rules

- Adding a mechanical gate means adding its verb and its parity check in the same task.
- The verb's `--require-approved` mode, running the full check set, has no recipe counterpart by design.
  Both plan gate sites run strictly before the Plan-Review segment's approve seam writes the approval flag,
  so demanding it would fail every fix round.
  The flag's guarantee rests on that seam failing loudly, never on a row re-checking it.
- A gate's call sits in a standalone row or in a producer's own closure;
  the property this section binds holds either way.

### Verified tree

Every verify site runs the call rendered from a gate of the hub's `gate.yaml` through `verifytree.Verify`:
the `final` gate's at the webster gate and `lym webster verify`;
the `round` gate's at the Webster-Burler-Fix round gate;
the `landing` gate's at Parent-Align, Finalize, Code-Burler-Fix and Darn-Write.

- `Verify` takes no gate slot and applies no timeout:
  its command is `lym gate test --wait` calls, which take their own slots and bound themselves.
- No site verifies a dirty tree: `Verify` refuses one with its paths before running anything.
- Records are per command, each entry with its tree and its commit;
  a skip requires an entry of the same command naming HEAD's tree.
- The record is written only by `Verify`, and only after a pass; no prompt names its path.
- The sites are the webster gate, the Webster-Burler-Fix gate and `lym webster verify`.
  They also include the Code-Burler-Fix, Darn-Write and Drift-Verify gates, Parent-Align and Finalize.
- The `round` gate's sites are Webster's verify-gate fixers:
  the fixer fork and the cold fixer `lym webster fix-verify` spawns.
  They share one prompt body.
  Each runs the hub's `round` call, rendered into the prompt's `fix_gate` marker, before it reports.
- Code-Burler-Fix and Darn-Write each run their own gate key's call through `Verify`:
  the `code-burler-fix` and `darn-write` keys.
  They share no pass with Webster's end gate.
  Each lints from its own latest recorded pass or the merge base.
  Darn-Write, and Code-Burler-Fix in a darn run, lint from the merge base on every attempt.
  For a diff wholly under `darn.yaml`'s `tree_scope` they run their own key's call over the touched trees' targets;
  otherwise they run their own key's call alone.
- PR-Review runs its own `pr-review` key's call through `Verify` over each clean head its agent commits,
  and it shares no pass with another site.
- Parent-Align and Finalize run the `landing` call through `Verify` with `BaseCommand` set to the base command.
  The base command is the `final` call in a tapestry run and the `landing` call in a darn run,
  so a landing-list pass keeps the entry the next round diffs from.
  A darn Finalize runs its `finalize` key's call over the touched trees' targets instead, for a tree-scoped diff.
  The call is resolved each time it runs and is one `lym gate test --wait` call;
  an empty one, rendered when no touched tree contributes a target, runs nothing.
- Each run that spawns its command writes its own `verify-<n>.log` in the verify directory and names it on its result,
  so no later verify overwrites an earlier run's log.
  After each such run, `Verify` keeps the newest logs up to its retention bound plus the one the failure record names,
  and it removes the rest.
- The failure record in the verify directory is the landing failure record.
  Parent-Align and Finalize write it when their `landing` call fails with a non-zero exit.
  It records the failing gate as kind, the failing tier, tests and run log, HEAD and the merge commit.
  A pass or a skip of the gate removes it, and the other sites neither write nor remove it.
- Every site tells `Site.Hold`, built by `verifytree.NewPauseHold` from its wiring's pause geometry.
  Finalize's site hold reads hub entries only while its run holds the parent lock;
  Parent-Align's verify runs without the lock.
  `cardverify` and battencli's `verifyrun` rows tell none, being no verify site.
- `internal/cardverify/runner.go` reruns a card's own `**Verify:**` command through `verifyrun` directly.
  It is no plan-verify site.
- `internal/battencli/land.go` runs the landing config's `deploy` command through `verifyrun` directly.
  It is no plan-verify site: it verifies no tree.

#### Enforcement

`cmd/lym/verifiedtree_test.go`, a tripwire rather than a completeness proof.
Each site names `verifytree.Verify` and also names a `Hold` it passes;
no other production file calls `verifyrun.Run`,
and none names the retired `verify-pending` marker.

## Gate fixer

Rule: Clearing a failed gate with a cold agent: only `gatefixer`, one fixer per failed evaluation; Go's re-run of the gate judges, never the fixer; a fixer's own gate calls spawn none; a runner crash gets none.

Held by review alone; no test enforces it.
