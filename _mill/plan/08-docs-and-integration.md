# Batch: docs-and-integration

```yaml
task: 'Seeded Shed core: run addressing, seed contract, batten'
batch: docs-and-integration
number: 8
cards: 5
verify: go build ./... && go test ./cmd/lyx/... && go test -tags integration ./internal/battencli/...
depends-on: [4, 6, 7]
```

## Batch Scope

This batch sweeps every prose artefact that names a retired path, a retired flag or a retired product name, and adds the one end-to-end integration test that proves the assembled shape works.
It is one batch because the prose describes the finished surface: every document here would have to be written twice if split across the batches that produced the surface, and none of it can be written correctly until run addressing, the seed contract and the renamed module all exist.
The per-`CONSTRAINTS.md` amendments are deliberately **not** here — each rode the card that falsified it, per the overview's `per-module-docs-ride-their-own-card` Shared Decision.

The sweep is driven by **literal**, not by Go caller: grepping symbols misses every prose artefact that hand-writes a path or a flag, and there are several.
The seven literals to sweep across `contracts/`, `plugins/`, `tools/`, `docs/`, `manifest/` and repo-root `CONSTRAINTS.md` are `_lyx/loom`, `.lyx/lifecycle`, `--recipe`, `lyx lifecycle`, `lifecycleshed`, `lifecyclerecipe` and `lifecyclecli`.

Batch-local decision beyond the overview's: `contracts/specs/loom-status-spec.md` is a **deployed normative spec**, and editing the in-repo copy does not update an already-deployed one.
Per the Stencil Ownership Invariant, `internal/stencilstore` never overwrites a hash-mismatched file and there is explicitly no force-sync carve-out for specs, so the deployed copy keeps the old text and is simply never refreshed.
This batch states the operator step — delete the deployed copy so the next run re-seeds it — in the commit message and in `docs/overview.md`.
It must **not** add a force-sync path, which that invariant forbids.

## Cards

### Card 32: the contracts sweep

- **Context:**
  - `internal/shedrun/paths.go`
  - `internal/shedrun/seed.go`
  - `internal/stencilstore/doc.go`
- **Edits:**
  - `contracts/specs/loom-status-spec.md`
  - `contracts/stencils/loom/loom-template-discussion.md`
  - `contracts/stencils/loom/loom-rubric-webster-review.md`
  - `contracts/recipes/loom-recipe.yaml`
  - `contracts/stencils/discussiontemplate_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Replace every `_lyx/loom/status.json` occurrence with `_lyx/shed/self/status.json` in the four prose artefacts listed, and every bare `_lyx/loom` used as the status file's directory with `_lyx/shed/self`.
  `contracts/specs/loom-status-spec.md` carries the path in four places — the status banner, the schema line, the machine-written paragraph and the seed paragraph — and its own use of the word "seed" means the t=0 contents of the status file, a different artefact from this task's `seed.json`; keep that meaning and add one sentence distinguishing the two, so a reader meeting both words does not conflate them.
  `contracts/stencils/loom/loom-template-discussion.md` and `loom-rubric-webster-review.md` carry the literal in producer prompt text, and `internal/loomcli`'s `discussiontemplate_test.go` — which lives under `contracts/stencils/` — pins it; update the pin with the text.
  `contracts/recipes/loom-recipe.yaml` names the path in a round's prompt text.
  Do not touch `internal/stencilstore` or add any force-sync path; the deployed-copy consequence is stated in card 35's `docs/overview.md` edit and in this card's commit message.
- **Commit:** `docs(contracts): retarget the loom status path onto the shed run directory`

### Card 33: the plugins and tools sweep

- **Context:**
  - `internal/shedcli/cli.go`
  - `internal/shedcli/seed.go`
  - `internal/battencli/cli.go`
  - `internal/shedrun/runid.go`
- **Edits:**
  - `plugins/ly/skills/ly-drive/SKILL.md`
  - `plugins/ly/skills/INDEX.md`
  - `tools/sandbox/SANDBOX-CORE-SUITE.md`
  - `tools/sandbox/SANDBOX-FABRIC-SUITE.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Rewrite `plugins/ly/skills/ly-drive/SKILL.md`'s whole invocation surface: the `--recipe` flag is gone, so every `lyx shed <verb> --recipe <name>` example becomes `lyx shed <verb> [<run-id>]`, and every `--recipe lifecycle` becomes a run-id address against a batten-seeded run.
  Keep the skill's five-value refusal-kind contract exactly as it is — that vocabulary did not change — and add the one new fact the skill's driver needs: a run-id that does not exist refuses with a run-id listing and **no** `kind` field, which is not a retryable condition.
  Update `plugins/ly/skills/INDEX.md`'s one-line description to match.
  In `tools/sandbox/SANDBOX-CORE-SUITE.md`, retarget the hand-written `_lyx/loom/status.json` fixture onto `_lyx/shed/self/status.json` and add the seed file beside it, since a status file with no seed is now the inconsistency batch 4 card 14 refuses.
  `tools/sandbox/SANDBOX-FABRIC-SUITE.md` carries an entire scenario written against `lyx lifecycle` and `.lyx/lifecycle/<slug>/`, including the run-lock path and the per-slug-directory refusal texts, all of which move: rewrite the scenario against `lyx batten`, `_lyx/shed/<slug>/status.json` and `.lyx/shed/<slug>/run.lock`, and add the new `lyx shed seed` verb and `lyx batten step` to the module's exercised surface, which the Sandbox Suite Coverage invariant requires of every registered module.
- **Commit:** `docs(plugins,sandbox): retarget ly-drive and the sandbox suites onto run addressing and batten`

### Card 34: the design doc and the roadmap

- **Context:**
  - `CONSTRAINTS.md`
  - `internal/shedrun/doc.go`
  - `internal/battenrecipe/names.go`
  - `contracts/recipes/batten-recipe.yaml`
- **Edits:**
  - `manifest/designs/seeded-shed.md`
  - `manifest/roadmap.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Update `manifest/designs/seeded-shed.md` to describe what shipped rather than what was proposed, correcting the three places this task's decisions diverged from its text: the run's locks live under `.lyx/shed/<run-id>/` at the mirrored subpath rather than beside the durable pair; `Run-Shed`'s still-running re-entry is a self-routed `Stuck` with a row-level `max_bounces: 1440` and a `config: {poll_interval_s: 30}` pair encoding a 12-hour window, while every non-running non-done child state is a hard error; and the recipe-name vocabulary is declared in `internal/shedrun` with `internal/shedcli`'s table pinned against it by a sync meta-test, because a reverse import from `battencli` into `shedcli` would be a cycle.
  Remove the DRAFT framing from the sections this task implemented and leave the two explicitly-out-of-scope items — `driver: llm` and relay-stepping — marked as the next roadmap item's and as rejected respectively.
  In `manifest/roadmap.md`, move the `seeded Shed core: run addressing, seed contract, batten` item from Planned to its completed section following that file's own convention.
  The Next Up item beneath it — `seeded driver choice: ly-drive strand as the child's driver` — needs no rewording: it already describes the `driver` field as selecting who steps a run, which stays true now that the field exists carrying `go` and refusing `llm`.
  Re-read it before landing and leave it alone unless the wording has since drifted.
  Every link target both files gain or change must resolve, per the Markdown Link Integrity invariant, whose allowlist is keyed by `(file, target)`.
- **Commit:** `docs(manifest): record the seeded Shed core as landed and correct the design's diverged points`

### Card 35: the overview

- **Context:**
  - `CONSTRAINTS.md`
  - `internal/shedrun/doc.go`
  - `internal/battencli/cli.go`
  - `internal/shedcli/cli.go`
  - `internal/stencilstore/doc.go`
- **Edits:**
  - `docs/overview.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Update `docs/overview.md`'s module table and execution-stack prose for the renamed and new modules: the `lifecycle` row becomes `batten`, naming `internal/battenshed` + `internal/battenrecipe` + `internal/battencli` and the verb set `lyx batten run|step|status|pause`; the `shed` entry stops describing a `--recipe` flag and describes the run-id positional and the seed-driven arming instead, and names the new `lyx shed seed` command; and `internal/shedrun` is added as a module in its own right.
  Rewrite the paragraph stating the lifecycle Shed's status is "per-machine under prime's own ephemeral tree" — it is now durable under prime's own `_lyx/shed/<slug>/`, which is the whole point of the change — and re-point its link from the renamed `Lifecycle Bookend Invariant` anchor to `#batten-bookend-invariant`.
  State the two landing preconditions this task carries, which an operator reading only this file must not miss: landing requires no lifecycle or loom run in flight, because no migration reads or moves the old layouts; and the already-deployed copy of `contracts/specs/loom-status-spec.md` must be deleted by hand so the next run re-seeds it, because `internal/stencilstore` never overwrites a hash-mismatched file and there is no force-sync carve-out for specs.
  Add a line for the new `Shed Run-Directory Invariant` wherever the file enumerates the repo's invariants.
  Every changed link target must resolve.
- **Commit:** `docs(overview): describe batten, run addressing and the shed run directory`

### Card 36: the end-to-end integration test

- **Context:**
  - `internal/battencli/wire.go`
  - `internal/battencli/arm.go`
  - `internal/battenshed/seamchild.go`
  - `internal/battenshed/innerrun.go`
  - `internal/battenrecipe/names.go`
  - `internal/shedrun/paths.go`
  - `internal/shedrun/seed.go`
  - `internal/hubforge/hub.go`
  - `internal/gitkit/gitkit.go`
- **Edits:**
  - `internal/battencli/lifecycle_integration_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Extend the existing `integration`-tagged end-to-end test to the four-row shape over a `hubforge`-built fixture: `Worktree-Create`, then `Seed-Child`, then `Run-Shed` watching a stubbed child status through to `done`, then `Worktree-Teardown`.
  Assert the child's `_lyx/shed/self/seed.json` exists, carries the recipe the fixture's Board task `type` names, and is **committed** on the child's pair — an uncommitted seed is as lost to a fresh clone as one never written, which is the machine-switch case this row exists for.
  Add a step-driven variant proving a still-running child yields a re-entrant `lyx batten step` that returns after one poll interval rather than holding for the child's whole duration.
  State plainly in the test's own comment what that variant proves and what it does not: the step **blocks for one `poll_interval_s`** and then returns, because the sleep stays inside `Call` and the row cannot see which verb drove it.
  The bounded return is the property the re-entrancy decision buys; it is not a non-blocking step, and nothing here makes it one.
  Keep the fixture's poll interval short enough that the variant does not spend 30 real seconds — inject it through the recipe fixture's own `config`, not by faking `deps.Sleep`, since this tier is exercising the assembled wiring rather than the producer in isolation.
  The file stays `integration`-tagged and its package keeps calling `gitkit.HermeticGitEnv()` in `TestMain`, per the Hermetic Git Test Environment Invariant; nothing here may move into an untagged file, per the Test Tier Purity Invariant's ban on `hubforge.NewHub` outside tagged files.
- **Commit:** `test(battencli): cover the four-row batten run and the re-entrant step end to end`

## Batch Tests

`verify: go build ./... && go test ./cmd/lyx/... && go test -tags integration ./internal/battencli/...` pairs a whole-module build with the two suites this batch's own changes are provable by.

`cmd/lyx/...` is in scope for two reasons beyond habit: `sandbox_coverage_test.go` enforces the Sandbox Suite Coverage invariant, which card 33's suite-document rewrite must keep satisfied for the renamed module and the two new verbs, and `helptree_test.go` — already updated in batch 7 — is what proves `lyx shed seed` and `lyx batten step` are both reachable with non-empty `Short` text.
The Markdown Link Integrity invariant's checker runs over `manifest/` and `docs/` and is reached by the whole-module build's own test surface, which is why cards 34 and 35 both close on "every changed link target must resolve" rather than carrying a separate verify.

The `integration`-tagged run is this batch's real deliverable beyond prose: it is the only place the assembled shape is exercised end to end, and it is deliberately not in any earlier batch's verify, because until batch 7 lands the surface it addresses does not exist.
Its two assertions that matter most are the child seed being **committed** rather than merely written, and the step variant returning after one poll interval — the first is the machine-switch property the whole durability decision rests on, and the second is the only proof that `Run-Shed`'s re-entrancy actually reaches a stepped outer run.

Cards 32, 34 and 35 add no runnable surface of their own beyond the link checker; their correctness is review discipline, which is the same footing every other prose artefact in this repo stands on.
