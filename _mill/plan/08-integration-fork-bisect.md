# Batch: integration-fork-bisect

```yaml
task: 'webster: rewrite for flat card list'
batch: integration-fork-bisect
number: 8
cards: 4
verify: go test -tags integration ./internal/websterengine/...
depends-on: [4, 7]
```

## Batch Scope

Add the plan-level integration-suite stage: after all batches land, ONE dedicated final fork
runs the plan-level `## verify:` suite once (no commit); on failure webster runs an in-process
SHA-bisect over the captured per-card SHAs to localize the offending card, then escalates to a
human via the existing terminal-status path plus the summary document. A plan whose overview has
NO `## verify:` section skips this whole stage (`planparser.Plan.Verify == ""`) and proceeds
straight to the summary/finish path — no error, no empty fork. This batch depends on batch 7
(the retargeted engine) and batch 4 (the `gitrepo` detached-checkout/restore primitive the bisect
uses). It adds `integration.go` + its template and renderer, and wires the trigger into the
`runlevel.go` Run flow (edited again after batch 7, sequentially).

## Cards

### Card 35: integration fork prompt + template + master trigger

- **Context:**
  - `docs/reference/plan-format-v3.md`
  - `internal/planparser/plan.go`
  - `internal/websterengine/render.go`
  - `internal/websterengine/fork-template.md`
- **Edits:**
  - `internal/websterengine/render.go`
  - `internal/websterengine/master-template.md`
  - `internal/websterengine/template_test.go`
- **Creates:**
  - `internal/websterengine/integration-template.md`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Create `integration-template.md`, the prompt for the single integration-suite fork: it instructs the fork to run the plan-level `## verify:` command once at HEAD and report pass/fail — it makes NO commit and implements NO cards. Add `RenderIntegrationPrompt(plan *planparser.Plan, reportPath, worktreeRoot string) ([]byte, error)` to `render.go`, embedding `integration-template.md` and injecting the plan-level `## verify:` text (`Plan.Verify`) and the `## Shared Decisions`. Add an `IntegrationTemplate() []byte` accessor mirroring `ForkTemplate()`/`MasterTemplate()`. Edit `master-template.md` to add the final integration-fork bracket instruction: AFTER all batches reach a terminal-done state, IF the plan has a `## verify:` section, Master spawns exactly one integration fork (reusing the fork-spawn discipline), waits for it, and on failure hands off to webster's in-process bisect; if there is no `## verify:` section, Master skips straight to writing the summary. Update `template_test.go` to assert `RenderIntegrationPrompt` injects the `## verify:` text and that the integration template carries no per-card/commit instructions.
- **Commit:** `feat(websterengine): integration-suite fork prompt, template, and master trigger`

### Card 36: integration orchestration (skip-check + run-once)

- **Context:**
  - `internal/planparser/plan.go`
  - `internal/websterengine/state.go`
  - `internal/websterengine/report.go`
  - `internal/websterengine/render.go`
  - `internal/hubgeometry/hubgeometry.go`
- **Edits:** none
- **Creates:**
  - `internal/websterengine/integration.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `integration.go` add the integration-stage orchestration (Go side): `ShouldRunIntegration(plan *planparser.Plan) bool` returning `plan.Verify != ""` (the skip-check — the plan-level `## verify:` is optional; absence skips the whole stage). Add the run-once helpers webster uses to drive the single integration fork through the existing spawn/await/report mechanism (reuse `AwaitBatch`-style long-poll on an integration report path resolved via `hubgeometry.WebsterReportsDir`, and webster's `ParseReport` for the fork's OK/FAILED). The integration fork makes no commit and is triggered exactly once, after every batch has reached a terminal-done digest. Keep this stage sequential — there are no concurrent forks contending for the working tree when it runs. Do NOT execute the bisect here (card 37); this card is the trigger/skip/await plumbing only.
- **Commit:** `feat(websterengine): integration-suite orchestration and skip-check`

### Card 37: SHA-bisect + escalation

- **Context:**
  - `internal/gitrepo/gitrepo.go`
  - `internal/planparser/plan.go`
  - `internal/websterengine/state.go`
  - `internal/websterengine/summary.go`
  - `internal/websterengine/digest.go`
- **Edits:**
  - `internal/websterengine/integration.go`
  - `internal/websterengine/summary.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Add the in-process SHA-bisect to `integration.go`: `bisect(repo *gitrepo.Repo, shas []string, verifyCmd string, worktree string) (offendingIndex int, err error)` — a binary search over the ordered per-card SHAs accumulated across every batch's `BatchState.CardSHAs`. It captures the current branch (`repo.CurrentBranch`), then for each candidate `repo.CheckoutDetached(sha)`, runs the plan-level `## verify:` command IN-PROCESS via `os/exec` (webster Go exec-ing the command at that checkout — NOT a fork per candidate, preserving the no-concurrent-forks guarantee), and `repo.RestoreBranch(branch)` when done (including on error/defer, so HEAD is always restored). Localize the first SHA at which `## verify:` fails. On completion escalate via the existing terminal path: record a terminal failed/stuck status in `_lyx/webster/state.json` (the run ends non-successfully rather than proceeding to loom's finishing step) and extend the summary document (in `summary.go`) to name the localized offending card. Reuse the existing run-exit/terminal-status surfacing rather than inventing a new operator signal. Guard the bisect so an empty `shas` slice or a single-SHA plan degrades gracefully (report the sole/HEAD card).
- **Commit:** `feat(websterengine): in-process SHA-bisect and integration escalation`

### Card 38: wire integration stage into the Run flow

- **Context:**
  - `internal/planparser/plan.go`
  - `internal/websterengine/integration.go`
  - `internal/websterengine/state.go`
  - `internal/websterengine/gitwrap.go`
- **Edits:**
  - `internal/websterengine/runlevel.go`
  - `internal/websterengine/runlevel_test.go`
- **Creates:**
  - `internal/websterengine/integration_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Wire the integration stage into the `Run` flow in `runlevel.go`: after all batches reach terminal-done, call `ShouldRunIntegration(plan)`; when false, proceed straight to summary/finish; when true, drive the single integration fork (card 36) and, on its `FAILED`, run `bisect` (card 37) over the accumulated `BatchState.CardSHAs` and escalate. This is the second (sequential, post-batch-7) edit to `runlevel.go`; keep it a minimal call-site addition at the end of the run, not a rewrite of the batch loop. Add `integration_test.go` with `//go:build integration` tests (real git; reuse the package `TestMain`): a plan with no `## verify:` skips the stage; a passing integration fork finishes normally; a failing one triggers bisect that localizes a scripted offending SHA (build a temp repo with a known-bad commit) and records the terminal escalation state + summary naming the card; assert HEAD is restored to the original branch after bisect. Update `runlevel_test.go` for the added integration call-site (fake integration fork report).
- **Commit:** `feat(websterengine): wire integration-suite stage and bisect into the run flow`

## Batch Tests

`verify: go test -tags integration ./internal/websterengine/...` — the bisect and integration
wiring spawn real git (detached checkout via the batch-4 primitive) and exec the verify command,
so their tests are integration-tagged and the flag is required for them to run. Skip-path and
orchestration plumbing are covered by Tier-1 fakes. The existing hermetic `TestMain` neutralizes
the operator gitconfig.
