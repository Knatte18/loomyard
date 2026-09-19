# Plan: Rename loom CLI run/drive/step for verb/engine symmetry, plus rename ly-supervise

```yaml
task: "Rename loom CLI run/drive/step for verb/engine symmetry, plus rename ly-supervise"
slug: "loom-cli-rename"
approved: false
started: "20260918-193929"
parent: "main"
root: ""
verify: null
skip_checks: ["move-target-collision"]
discussion_sha: fc59b52c42dafc4cd3f1d7c356cd59db5217d703
```

## Batch Index

_The fenced yaml block below is the authoritative DAG mill-go reads to schedule batches.
Every batch lives at `NN-<batch-slug>.md` in this directory and is mirrored as one entry here._

```yaml
batches:
  - number: 1
    name: go-cli-rename
    file: 01-go-cli-rename.md
    depends-on: []
    verify: go test ./internal/loomcli/... ./cmd/lyx/... && go test -tags smoke -run XXX_NONE ./internal/loomcli/...
  - number: 2
    name: launchers
    file: 02-launchers.md
    depends-on: [1]
    verify: go test ./internal/fabricengine/
  - number: 3
    name: go-comment-sweep
    file: 03-go-comment-sweep.md
    depends-on: [2]
    verify: go test ./internal/loomengine/... ./internal/loomshed/... ./internal/shuttleengine/... ./internal/webstercli/... ./internal/websterengine/... ./internal/frictionengine/... ./internal/friction/... ./internal/shedadapters/...
  - number: 4
    name: skill-rename
    file: 04-skill-rename.md
    depends-on: [3]
    verify: null
  - number: 5
    name: docs-and-manifest
    file: 05-docs-and-manifest.md
    depends-on: [4]
    verify: go test ./internal/lyxcwd/ ./contracts/... ./tools/...
```

## Shared Decisions

### Decision: final-verb-set

- **Decision:** `drive` becomes `run`; today's `run` becomes `start`; `step` is unchanged.
  The final loom verb set is `start|run|step|status|pause|validate-discussion|validate-plan`, and the bare root alias becomes `lyx start`.
- **Rationale:** `run` then names the verb that calls `shedengine.Shed.Run`, which is the task's whole point;
  `start` names the bootstrap accurately, since it starts a session rather than running a machine.
- **Applies to:** all batches

### Decision: no-back-compat-aliases

- **Decision:** hard rename.
  No hidden alias, no deprecation shim, and no warning path for the retired `lyx loom run` bootstrap sense or for `lyx loom drive`.
  The old names survive nowhere in the tree, including in historical prose.
- **Rationale:** pre-release with one operator, and the whole repo is renamed in the same task.
  `run` is being reused with a new meaning, so an alias for the old `run` is not even expressible.
- **Applies to:** all batches

### Decision: read-and-classify-never-blind-replace

- **Decision:** every hit is read and classified into rename / leave / structural before it is touched.
  A repo-wide textual substitution is banned.
  "run" as a *noun* meaning "an execution" is left alone — the known sites are `internal/burlercli/wiring.go:131,190`, `internal/githubclient/doc.go:84`, `internal/landingshed/deps.go:88`, `internal/selfreportengine/selfreport.go:84`, and `internal/webstercli/wiring.go:218`.
  The `run` verbs on other modules (`lyx webster run`, `lyx burler run`, `lyx shuttle run`) and their `runCmd` identifiers are untouched.
- **Rationale:** a blind replace turns `drive.go`'s contrast prose into self-references and rewrites four unrelated files' noun usage.
- **Applies to:** all batches

### Decision: cards-name-landmarks-not-boundaries

- **Decision:** a file in any card's `Edits:` is swept whole.
  The sites a card's `Requirements:` names are landmarks — they pin the hard judgment calls, the sentences that must be rewritten rather than substituted, and the identifiers that change — and are explicitly not an exhaustive list.
  A hit not named is still classified and fixed.
  Batches 1, 3, and 5 each restate this as a `## Per-file sweep rule` section naming the hit classes concretely.
- **Rationale:** this mirrors the discussion's own Scope section, which states scope as an enumeration *rule* rather than a file whitelist, for the reason its Q&A log records: a whitelist cannot be both complete and maintainable at this hit count, and a silently-short one is worse than none because it reads as authoritative.
  Plan review rounds 1 and 2 demonstrated the same failure one level down — every finding in both rounds was a `scope` miss where a card enumerated some of a file's hits and a literal implementer would have left the rest.
- **Applies to:** all batches

### Decision: contrast-prose-rewritten-as-sentences

- **Decision:** prose whose meaning turns on the two verbs being *different* is rewritten as sentences, never token-substituted.
- **Rationale:** a token swap collapses both sides of such a sentence onto one name and silently inverts what it asserts.
- **Applies to:** go-cli-rename, go-comment-sweep, docs-and-manifest

### Decision: historical-prose-rewritten-not-glossed

- **Decision:** retrospective records — roadmap Done entries, shipped-status design docs, and Go comments narrating past incidents — are rewritten to the new names outright, with no "(formerly `ly-supervise`)" gloss and no preserved old name.
- **Rationale:** these documents describe the tree as it stands, and a Done entry pointing at `plugins/ly/skills/ly-supervise/SKILL.md` after the rename names a path that no longer exists.
  A gloss would also keep the retired name grep-discoverable, which is what rejected back-compat aliases.
- **Applies to:** go-comment-sweep, docs-and-manifest

### Decision: meta-trees-excluded

- **Decision:** `_mill/`, `crucible/`, and `docs/research/` are out of scope wholesale and are never edited by any batch, even though `_mill/` is tracked.
- **Rationale:** these files record what a task decided at the time it decided it;
  rewriting them falsifies the record rather than updating it.
- **Applies to:** all batches

### Decision: launcher-filename-unchanged

- **Decision:** the per-worktree launcher script keeps its filename `run<ext>`.
  Only the command embedded in it changes, and `removeLaunchers`' explicit name list keeps `"run"+ext`.
- **Rationale:** the filename names the operator's action, not the CLI verb, and `writeLauncherScriptIfChanged` rewrites existing files in place so every hub picks up the corrected command with no migration.
  Renaming it would orphan a stale `run<ext>` that invokes the new foreground driver and would break `removeLaunchers`' non-recursive directory removal.
- **Applies to:** launchers

### Decision: rename-chain-skips-move-target-collision

- **Decision:** the plan is validated with `move-target-collision` skipped, and `skip_checks: ["move-target-collision"]` is persisted in this overview's frontmatter.
  The skip covers exactly one finding: batch 1's `internal/loomcli/drive.go` -> `internal/loomcli/run.go` pair, whose destination is occupied at validation time by the file the *same batch's other pair* relocates first.
- **Rationale:** `_check_move_target_collision`'s condition 1 tests whether the destination "already exists on disk before the plan runs", which cannot model an intra-plan rename chain (`run.go` -> `start.go`, then `drive.go` -> `run.go`).
  No plan restructuring can clear it — the check reads disk state at validation time, so the collision is reported regardless of card order, batch split, or move ordering.
  The chain itself is correct and is what the discussion's `identifiers-and-filenames-follow-the-verbs` decision requires;
  batch 1's `## Rename mechanic` section pins the two `git mv` calls in the only order that works and states what breaks if they are run the other way round.
  The two genuine hazards this check exists to catch are both absent here: no second batch targets either destination, and neither destination appears as a `Creates:` token anywhere in the plan.
- **Applies to:** go-cli-rename

### Decision: no-behavioural-change

- **Decision:** this is a pure rename.
  No verb gains, loses, or reorders a step, and no flag is added or removed.
  The only new executable assertions are the three guards the Testing section calls for.
- **Rationale:** keeps the diff reviewable and keeps the completion gate meaningful — a rename that compiles and passes the suite has landed.
- **Applies to:** all batches

### Decision: verify-scope-and-smoke-tag

- **Decision:** each batch's `verify:` is scoped to the packages it touches.
  Batch 1 chains a second `go test -tags smoke -run XXX_NONE ./internal/loomcli/...` invocation because it edits two `//go:build smoke` files that the untagged default gate never compiles.
  `-run XXX_NONE` type-checks those files without running the multi-minute process-spawning smoke suite.
- **Rationale:** without the chained tagged invocation a broken smoke test compiles nowhere in either the batch verify or `pipeline.done_gate` (`go test ./... && go test -tags integration ./...`), so the argv and refusal-assertion edits would ship unverified.
- **Applies to:** go-cli-rename

## All Files Touched

- `CONSTRAINTS.md`
- `README.md`
- `cmd/lyx/helptree_test.go`
- `cmd/lyx/main.go`
- `cmd/lyx/retiredverbs_test.go`
- `cmd/lyx/sandbox_coverage_test.go`
- `contracts/specs/loom-status-spec.md`
- `docs/overview.md`
- `internal/fabricengine/launcher_content_test.go`
- `internal/fabricengine/launchers.go`
- `internal/friction/friction.go`
- `internal/frictionengine/spec.go`
- `internal/loomcli/bootstrap.go`
- `internal/loomcli/bootstrap_test.go`
- `internal/loomcli/cli.go`
- `internal/loomcli/cli_test.go`
- `internal/loomcli/friction_test.go`
- `internal/loomcli/pause.go`
- `internal/loomcli/run.go`
- `internal/loomcli/seedinput.go`
- `internal/loomcli/sharedbootstrap.go`
- `internal/loomcli/smoke_bootstrapwiring_test.go`
- `internal/loomcli/smoke_test.go`
- `internal/loomcli/start.go`
- `internal/loomcli/status.go`
- `internal/loomcli/step.go`
- `internal/loomcli/wiring.go`
- `internal/loomengine/config.go`
- `internal/loomengine/seed.go`
- `internal/loomengine/seedownership_test.go`
- `internal/loomshed/seed.go`
- `internal/loomshed/seed_test.go`
- `internal/shedadapters/bouncer_seed_test.go`
- `internal/shuttleengine/attach.go`
- `internal/webstercli/wiring.go`
- `internal/websterengine/strand.go`
- `manifest/designs/logger-coverage.md`
- `manifest/designs/loom-step.md`
- `manifest/designs/loom.md`
- `manifest/designs/reed-born-as-strand.md`
- `manifest/designs/reed-header-selvage.md`
- `manifest/designs/reed-mailbox.md`
- `manifest/designs/self-report-tier1.md`
- `manifest/designs/self-report-tier2.md`
- `manifest/designs/shed-generic-watchdog.md`
- `manifest/designs/shed-recipe.md`
- `manifest/designs/shed.md`
- `manifest/designs/worktree-lifecycle-shed-producers.md`
- `manifest/roadmap.md`
- `plugins/ly/skills/INDEX.md`
- `plugins/ly/skills/ly-drive/SKILL.md`
- `tools/sandbox/SANDBOX-CORE-SUITE.md`
