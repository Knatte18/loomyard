# Plan: Collapse _pattern into _lyx, and un-reserve _raddle as a hub-level name

```yaml
task: "Collapse _pattern into _lyx, and un-reserve _raddle as a hub-level name"
slug: "pattern-into-lyx-consolidation"
approved: true
started: "20260808-161017"
parent: "main"
root: ""
verify: go vet -tags integration ./...
```

## Batch Index

_The fenced yaml block below is the authoritative DAG mill-go reads to schedule batches.
Every batch lives at `NN-<batch-slug>.md` in this directory and is mirrored as one entry here._

```yaml
batches:
  - number: 1
    name: pattern-path-api
    file: 01-pattern-path-api.md
    depends-on: []
    verify: go test ./internal/pattern/ ./internal/burlerengine/ ./internal/websterengine/ ./internal/loomengine/ ./cmd/lyx/ && go test -tags integration ./internal/builderengine/
  - number: 2
    name: residue-rescope
    file: 02-residue-rescope.md
    depends-on: [1]
    verify: go test -tags integration -run 'Pull|Residue' ./internal/fabricengine/
  - number: 3
    name: junction-test-retarget
    file: 03-junction-test-retarget.md
    depends-on: [2]
    verify: go test -tags integration ./internal/fabricengine/ ./internal/fabriccli/ ./internal/loomengine/
  - number: 4
    name: empty-pathspec-and-unreservation
    file: 04-empty-pathspec-and-unreservation.md
    depends-on: [3]
    verify: go test -tags integration ./internal/fabricengine/ ./internal/configsync/ ./internal/configcli/ ./internal/lyxcwd/ ./cmd/lyx/
  - number: 5
    name: pollution-scan-and-reportonly
    file: 05-pollution-scan-and-reportonly.md
    depends-on: [4]
    verify: go test -tags integration ./internal/fabricengine/ ./internal/fabriccli/
  - number: 6
    name: geometry-token-retirement
    file: 06-geometry-token-retirement.md
    depends-on: [5]
    verify: go test ./... && go test -tags integration ./...
  - number: 7
    name: docs-and-design-sweep
    file: 07-docs-and-design-sweep.md
    depends-on: [6]
    verify: go test -tags integration ./cmd/lyx/ ./internal/lyxcwd/ ./internal/fabriccli/ ./internal/fabricengine/
```

## Shared Decisions

_Cross-cutting decisions every batch inherits: naming conventions, error-handling posture, test frameworks, style/lint constraints.
One subsection per decision.
Batch-local decisions live in each batch file._

### Decision: the tree compiles and stays green at every batch boundary

- **Decision:** no batch may leave any package failing to build or failing its tests.
  The module-wide `verify: go vet -tags integration ./...` in the frontmatter above runs at every batch boundary specifically to catch a cross-package compile break the batch's own scoped `verify:` cannot see.
- **Rationale:** deleting `pattern.DirName` is a compile break across five packages and roughly thirteen files.
  Sequencing the plan so the deletion happens only after every consumer has already been migrated is what keeps each intermediate commit a working tree rather than a broken one.
- **Applies to:** all batches

### Decision: `pattern.DirName` and `pattern.Dir()` survive until batch 6

- **Decision:** batch 1 rewrites `pattern.File`/`FileHere` and the three directive constants onto `_lyx`, but leaves the `DirName` const and the `Dir(baseDir)` accessor in place, unchanged, still spelling `_pattern`.
  They are deleted in batch 6, once batches 1-3 have removed every consumer.
- **Rationale:** `pattern.DirName` has two unrelated consumer classes — PATTERN-path assertions (migrated in batch 1) and junction-name arguments in fabricengine/fabriccli/loomengine tests (retargeted to `_extra` in batch 3).
  Deleting the const in batch 1 would force both classes into one batch and would couple the `internal/pattern` work to the entire fabricengine junction suite.
  Between batch 1 and batch 6 `Dir()` is deliberately incoherent with `File()` — `Dir` still points at `_pattern` while `File` points at `_lyx/PATTERN.md`.
  That incoherence is transient, unreferenced by production code, and pinned by `patternpath_test.go`'s existing `Dir` table until batch 6 removes both.
- **Applies to:** batches 1, 3, 6

### Decision: `internal/pattern` is the single declarer of the PATTERN path segments

- **Decision:** `internal/pattern` exports `PathspecFile` and `PathspecDir`, both built from `lyxdirs.LyxDirName` by string concatenation, and `internal/fabricengine` production code imports `internal/pattern` to consume them.
  No other package spells `PATTERN.md` or the `pattern` path segment in a path-construction context.
- **Rationale:** without this the `patternDirName` duplication `pull.go` carries today is simply re-created under new spellings, and this time with no enforcement at all — `TestEnforcement_GeometryLiterals` matches whole tokens by exact equality, so `"_lyx/PATTERN.md"` is not equal to `"_lyx"` and passes unpoliced.
  Building these strings from `lyxdirs.LyxDirName` is therefore a **review obligation**, not a machine-enforced one.
- **Applies to:** batches 1, 2, 6

### Decision: `_extra` is the substitute optional-junction name

- **Decision:** every test that used `_pattern` merely as "an ordinary optional, config-driven junction name" retargets to `_extra`, the name `config_driven_junctions_integration_test.go` and `junctionnames_test.go` already use for that role.
  Such tests are never deleted — the generic multi-junction path must stay covered by a name that is not the one being removed.
- **Rationale:** `_pattern` is the exemplar in most of these tests, not the subject.
  Deleting the cases would silently drop coverage of config-driven junction wiring, repoint, unwire, stale-removal, and rollback.
- **Applies to:** batches 3, 4

### Decision: no file renames, no migration mechanism

- **Decision:** this plan performs zero `git mv` operations and adds no migration code, CLI verb, reconcile branch, or operator-facing migration paragraph.
  Files whose names reference `_pattern` (e.g. `internal/fabricengine/junction_pattern_integration_test.go`) keep their names; only their header comments are corrected.
- **Rationale:** lyx is not in production and the only fabric-wired repo is the re-clonable SANDBOX, so migration machinery would be built for a population of one disposable repo.
  Renaming test files is outside the discussed scope and would inflate the diff for wording alone.
- **Applies to:** all batches

### Decision: `internal/lyxcwd/raddle_guard_test.go` is never touched

- **Decision:** leave `internal/lyxcwd/raddle_guard_test.go` entirely untouched — all nine of its `_raddle` occurrences, and its header prose.
  The positive "`_raddle` is not a reserved hub name" guard goes in `internal/lyxcwd/lyxcwd_test.go` (`package lyxcwd_test`) instead.
- **Rationale:** the file is `package lyxcwd` and is a tree-scan guard asserting no production file in `internal/lyxcwd` names `_raddle` — an unrelated invariant that stays valid.
  Calling `fabricengine.IsReservedHubName` from it would close a `fabricengine -> lyxcwd` import cycle in the test binary.
- **Applies to:** batches 4, 6

### Decision: markdown files use semantic line breaks

- **Decision:** every `.md` file this plan edits keeps one sentence per line, with additional breaks at internal independent-clause boundaries.
  Never hard-wrap at a fixed column; never use trailing double-spaces or a backslash for the break.
- **Rationale:** repo convention (`CLAUDE.md`), applied to edited lines in existing files as well as new prose.
- **Applies to:** batches 1, 7

## All Files Touched

- `CLAUDE.md`
- `CONSTRAINTS.md`
- `README.md`
- `cmd/lyx/constructoranchoring_test.go`
- `cmd/lyx/tierpurity_test.go`
- `docs/overview.md`
- `docs/research/linux-portability-survey.md`
- `docs/shared-libs/lyxcwd.md`
- `internal/builderengine/template_test.go`
- `internal/burlerengine/template_test.go`
- `internal/configcli/configcli_integration_test.go`
- `internal/configsync/configsync_test.go`
- `internal/fabriccli/cli_test.go`
- `internal/fabriccli/fabric.go`
- `internal/fabriccli/weft_verbs.go`
- `internal/fabricengine/add_rollback_adopt_test.go`
- `internal/fabricengine/add_test.go`
- `internal/fabricengine/checkout_index_refresh_test.go`
- `internal/fabricengine/checkout_rollback_test.go`
- `internal/fabricengine/classify_test.go`
- `internal/fabricengine/cleanup.go`
- `internal/fabricengine/commit_integration_test.go`
- `internal/fabricengine/config_driven_junctions_integration_test.go`
- `internal/fabricengine/config_test.go`
- `internal/fabricengine/doc.go`
- `internal/fabricengine/dotlyxjunction_integration_test.go`
- `internal/fabricengine/fabric_test.go`
- `internal/fabricengine/hostjunction_test.go`
- `internal/fabricengine/junction.go`
- `internal/fabricengine/junction_pattern_integration_test.go`
- `internal/fabricengine/junction_repoint_test.go`
- `internal/fabricengine/junctionnames.go`
- `internal/fabricengine/junctionnames_test.go`
- `internal/fabricengine/pull.go`
- `internal/fabricengine/pull_integration_test.go`
- `internal/fabricengine/reconcile.go`
- `internal/fabricengine/reconcile_stale_registration_test.go`
- `internal/fabricengine/reconcile_stale_removal_test.go`
- `internal/fabricengine/remove_junctions_integration_test.go`
- `internal/fabricengine/status.go`
- `internal/fabricengine/structuraldirs_test.go`
- `internal/fabricengine/template.yaml`
- `internal/fabricengine/template_test.go`
- `internal/fabricengine/unwire.go`
- `internal/fabricengine/unwire_test.go`
- `internal/fabricengine/weftgit_pathspec_integration_test.go`
- `internal/fabricengine/weftwiring.go`
- `internal/loomengine/plan_test.go`
- `internal/loomengine/preflight_integration_test.go`
- `internal/lyxcwd/enforcement_test.go`
- `internal/lyxcwd/lyxcwd_test.go`
- `internal/pattern/doc.go`
- `internal/pattern/leaf_enforcement_test.go`
- `internal/pattern/pattern.go`
- `internal/pattern/pattern_test.go`
- `internal/pattern/patternpath_test.go`
- `internal/websterengine/template_test.go`
- `manifest/designs/fabric-unified-view.md`
- `manifest/designs/finalize.md`
- `manifest/designs/loom.md`
- `manifest/designs/raddle.md`
- `manifest/designs/shed.md`
- `manifest/roadmap.md`
- `tools/sandbox/SANDBOX-FABRIC-SUITE.md`
