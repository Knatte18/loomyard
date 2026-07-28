# Plan: PATTERN wiring: conditional constraint-injection into every agent

```yaml
task: 'PATTERN wiring: conditional constraint-injection into every agent'
slug: pattern-wiring
approved: false
started: '20260728-093403'
parent: main
root: ""
verify: null
```

## Batch Index

_The fenced yaml block below is the authoritative DAG mill-go reads to schedule batches. Every batch lives at `NN-<batch-slug>.md` in this directory and is mirrored as one entry here._

```yaml
batches:
  - number: 1
    name: stencil-optional
    file: 01-stencil-optional.md
    depends-on: []
    verify: go test ./internal/stencil/...
  - number: 2
    name: hubgeometry-pattern-surface
    file: 02-hubgeometry-pattern-surface.md
    depends-on: [1]
    verify: go test -tags integration ./internal/hubgeometry/... ./cmd/lyx/...
  - number: 3
    name: fabric-junction-generalisation
    file: 03-fabric-junction-generalisation.md
    depends-on: [2]
    verify: go test -tags integration ./internal/fabricengine/... ./internal/initengine/... ./internal/initcli/... ./internal/loomengine/... ./cmd/lyx/...
  - number: 4
    name: weft-pathspec-tolerance
    file: 04-weft-pathspec-tolerance.md
    depends-on: [3]
    verify: go test -tags integration ./internal/fabricengine/... ./internal/fabriccli/... ./cmd/lyx/...
  - number: 5
    name: pattern-junction-flip
    file: 05-pattern-junction-flip.md
    depends-on: [4]
    verify: go test -tags integration ./internal/hubgeometry/... ./internal/fabricengine/... ./internal/initengine/... ./internal/initcli/... ./internal/loomengine/... ./cmd/lyx/...
  - number: 6
    name: pattern-package
    file: 06-pattern-package.md
    depends-on: [5]
    verify: go test -tags integration ./internal/pattern/... ./internal/hubgeometry/... ./cmd/lyx/...
  - number: 7
    name: prompt-wiring
    file: 07-prompt-wiring.md
    depends-on: [6]
    verify: go test -tags integration ./...
```

## Shared Decisions

_Cross-cutting decisions every batch inherits._

### Decision: batches form a strict linear chain

- **Decision:** every batch depends on exactly its predecessor. There is no parallelism in this DAG.
- **Rationale:** two independent reasons. First, the pieces genuinely serialise: `hubgeometry` cannot declare a second junction until `fabricengine` can materialise, unwire and health-check one generically, and `fabricengine`'s widened weft pathspec cannot land until `CommitWeft` tolerates a non-matching entry. Second, several docs (`CONSTRAINTS.md`, `docs/overview.md`, `manifest/designs/pattern.md`) are edited from more than one batch because this repo's rule is that docs land in the same commit as the change that invalidates them; a linear chain is what keeps those shared edits from being concurrent writes to the same file.
- **Applies to:** all batches

### Decision: `_pattern` is a geometry token owned solely by `internal/hubgeometry`

- **Decision:** the literal `"_pattern"` may appear in a path-construction context (a `filepath.Join` argument, a `+` operand, or a string `const` value) only inside `internal/hubgeometry`. Every other package obtains `_pattern` paths from a `hubgeometry` accessor. Comparisons and git-pathspec slice literals are exempt by the Hub Geometry Invariant's own carve-out, which is what permits `fabricengine/status.go`'s new `git ls-files` pathspec entry and its `strings.HasPrefix` check.
- **Rationale:** the Hub Geometry Invariant, machine-enforced by `internal/hubgeometry/enforcement_test.go` (`TestEnforcement_GeometryLiterals`) on every `go test`. Batch 2 adds `_pattern` to that test's token list, so from batch 2 onward a stray literal fails the build. This is forced, not stylistic: it is why `internal/pattern` cannot build its own `filepath.Join(root, "_pattern", "PATTERN.md")`.
- **Applies to:** all batches

### Decision: every new git-spawning or fixture-copying test file carries `//go:build integration`

- **Decision:** any new `*_test.go` whose source contains `gitexec.RunGit`, `exec.Command`, `exec.CommandContext` or `lyxtest.Copy*` — including in a comment or string literal — carries `//go:build integration` as its first non-empty line. `internal/pattern`'s own tests stay untagged: they use `os.Stat` and `t.TempDir()` only and spawn nothing.
- **Rationale:** the Test Tier Purity Invariant, enforced by `cmd/lyx/tierpurity_test.go` on every plain `go test`. The match is a raw substring, so even a comment mentioning one of those tokens in an untagged file trips it. Separately, the Hermetic Git Test Environment Invariant (`cmd/lyx/hermeticenv_test.go`, which scans every test file regardless of build tag) requires any *new* git-spawning package to carry a `TestMain` calling `lyxtest.HermeticGitEnv()`; `fabricengine` and `initengine` already have one, and this plan adds no new git-spawning package.
- **Applies to:** all batches

### Decision: `internal/gitrepo` is not touched

- **Decision:** no card in this plan edits `internal/gitrepo`. The pathspec-tolerance filter lands in `internal/fabricengine/weftgit.go` even though `gitrepo.StageAndCommit` is where the underlying `git add` runs.
- **Rationale:** the concurrent `native-clients` task owns `internal/gitrepo`. Keeping this task off that package is what makes the two parallel-safe. `CommitWeft` already owns a `did not match any files` special case for exactly this reason (`weftgit.go`), so the filter belongs beside it.
- **Applies to:** batch 4

### Decision: docs land in the batch that invalidates them, never in a trailing docs batch

- **Decision:** `CONSTRAINTS.md`, `docs/overview.md`, `docs/shared-libs/*.md`, `internal/fabricengine/doc.go`, the affected cobra `Short`/`Long` strings, and each template's leading banner comment are edited by the batch whose code change makes them stale — not collected at the end. The single exception is `manifest/designs/pattern.md`, whose six corrections are one coherent card in the final batch because they describe the finished design rather than any one step of it.
- **Rationale:** this repo's `CLAUDE.md` rule ("Task completion — docs land in the same commit"). A trailing docs batch means every intermediate commit ships prose that contradicts the code.
- **Applies to:** all batches

### Decision: markdown is one line per paragraph, never hard-wrapped

- **Decision:** every `.md` file this plan touches — the five prompt templates, `CONSTRAINTS.md`, `docs/overview.md`, `manifest/designs/pattern.md`, `manifest/roadmap.md`, `docs/shared-libs/*.md` — is written with one continuous line per paragraph or list item.
- **Rationale:** the repo's `CLAUDE.md` markdown rule. A hard-wrapped paragraph diffs badly: an edit anywhere in it touches every wrapped line.
- **Applies to:** all batches

### Decision: `PatternDirName` is the only new geometry constant; `PATTERN.md` is not one

- **Decision:** `hubgeometry` gains `PatternDirName = "_pattern"` as an exported const. The filename `"PATTERN.md"` is *not* a geometry token and is not added to the enforcement list, but the whole path is still composed inside `hubgeometry` (`PatternFile`, `PatternFileHere`) so no consumer joins the two halves itself.
- **Rationale:** keeping the full path in one accessor is what lets `internal/pattern` stay a leaf that never constructs a path.
- **Applies to:** batches 2, 6

## All Files Touched

- `CONSTRAINTS.md`
- `docs/overview.md`
- `docs/shared-libs/hubgeometry.md`
- `docs/shared-libs/stencil.md`
- `internal/builderengine/implementer-template.md`
- `internal/builderengine/spawn.go`
- `internal/builderengine/template_test.go`
- `internal/burlerengine/engine.go`
- `internal/burlerengine/prompt.go`
- `internal/burlerengine/review-prompt-template.md`
- `internal/burlerengine/template_test.go`
- `internal/fabriccli/fabric.go`
- `internal/fabriccli/weft_verbs.go`
- `internal/fabricengine/doc.go`
- `internal/fabricengine/drift.go`
- `internal/fabricengine/junction.go`
- `internal/fabricengine/junction_pattern_integration_test.go`
- `internal/fabricengine/junction_repoint_test.go`
- `internal/fabricengine/reconcile.go`
- `internal/fabricengine/reconcile_stale_registration_test.go`
- `internal/fabricengine/remove_junctions_integration_test.go`
- `internal/fabricengine/status.go`
- `internal/fabricengine/template.yaml`
- `internal/fabricengine/template_test.go`
- `internal/fabricengine/weftgit.go`
- `internal/fabricengine/weftgit_pathspec_integration_test.go`
- `internal/fabricengine/weftwiring.go`
- `internal/hubgeometry/enforcement_test.go`
- `internal/hubgeometry/hubgeometry.go`
- `internal/hubgeometry/hubgeometry_test.go`
- `internal/hubgeometry/pattern_test.go`
- `internal/hubgeometry/weft_test.go`
- `internal/initcli/initcli.go`
- `internal/initcli/initcli_test.go`
- `internal/initengine/init.go`
- `internal/initengine/init_test.go`
- `internal/initengine/undo.go`
- `internal/initengine/undo_test.go`
- `internal/loomengine/plan.go`
- `internal/loomengine/plan-template.md`
- `internal/loomengine/plan_test.go`
- `internal/loomengine/preflight.go`
- `internal/loomengine/preflight_integration_test.go`
- `internal/pattern/doc.go`
- `internal/pattern/leaf_enforcement_test.go`
- `internal/pattern/pattern.go`
- `internal/pattern/pattern_test.go`
- `internal/stencil/stencil.go`
- `internal/stencil/stencil_test.go`
- `internal/webstercli/beginbatch.go`
- `internal/websterengine/beginbatch.go`
- `internal/websterengine/fork-template.md`
- `internal/websterengine/master-template.md`
- `internal/websterengine/recoverbatch.go`
- `internal/websterengine/render.go`
- `internal/websterengine/runlevel.go`
- `internal/websterengine/template_test.go`
- `manifest/designs/pattern.md`
- `manifest/roadmap.md`
