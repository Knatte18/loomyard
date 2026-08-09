# Plan: Rename the fabric host vocabulary to warp, and name the composite repo Fabric

```yaml
task: "Rename the fabric host vocabulary to warp, and name the composite repo Fabric"
slug: "fabric-host-to-warp-rename"
approved: false
started: "20260809-055729"
parent: "main"
root: ""
verify: go build ./...
```

## Batch Index

_The fenced yaml block below is the authoritative DAG mill-go reads to schedule batches.
Every batch lives at `NN-<batch-slug>.md` in this directory and is mirrored as one entry here._

```yaml
batches:
  - number: 1
    name: wordswap-tool
    file: 01-wordswap-tool.md
    depends-on: []
    verify: go test ./tools/wordswap/...
  - number: 2
    name: pre-sweep-rewords
    file: 02-pre-sweep-rewords.md
    depends-on: [1]
    verify: go test ./internal/fabricengine/... ./internal/boardengine/... ./internal/configcli/... ./internal/builderengine/... ./tools/sandbox/... ./internal/lyxcwd/...
  - number: 3
    name: go-sweep
    file: 03-go-sweep.md
    depends-on: [2]
    verify: go test ./... && go test -tags integration ./...
  - number: 4
    name: file-renames
    file: 04-file-renames.md
    depends-on: [3]
    verify: go test ./internal/fabricengine/... && go test -tags integration ./internal/fabricengine/...
  - number: 5
    name: cli-surface-review
    file: 05-cli-surface-review.md
    depends-on: [4]
    verify: go test ./cmd/lyx/... ./internal/fabriccli/... ./tools/sandbox/...
  - number: 6
    name: docs-sweep
    file: 06-docs-sweep.md
    depends-on: [5]
    verify: go test ./cmd/lyx/...
  - number: 7
    name: constraints-and-guard
    file: 07-constraints-and-guard.md
    depends-on: [6]
    verify: go test ./... && go test -tags integration ./...
```

## Shared Decisions

_Cross-cutting decisions every batch inherits.
Batch-local decisions live in each batch file._

### Decision: the substitution is one generic case-preserving word swap, never an identifier table

- **Decision:** every mechanical `host` → `warp` change in this task is performed by `go run ./tools/wordswap -from host -to warp ...` over an explicit file list.
  No hand-maintained old→new identifier table, no per-call-site `Edit` call for anything the tool can do, and no `gofmt -r` / `gopls rename`.
  The tool swaps `host`→`warp`, `Host`→`Warp`, `HOST`→`WARP` and every unambiguously bounded embedded form (`hostBranch`→`warpBranch`, `HostJunctions`→`WarpJunctions`, `HOST_BRANCH`→`WARP_BRANCH`) in one pass.
- **Rationale:** the boundary-case survey recorded in `_mill/discussion.md` found the fabric-package exclude list is empty after one comment reword, so a per-identifier table protects against nothing while covering only what someone remembered to list.
  A generic swap covers identifiers, comments, test-function names, string literals, shell variables and markdown prose with one mechanism; `go build ./... && go test ./...` is the completeness proof.
- **Applies to:** all batches

### Decision: `host` + lowercase is AMBIGUOUS — the implementer adjudicates, the tool never guesses

- **Decision:** `wordswap` swaps only where the token boundary is unambiguous — the character after `host` is an uppercase letter, a digit, an underscore, or a non-identifier character (and symmetrically before it, or start-of-input).
  Anything of the form `host` + lowercase letters at a token start (`hostclean`, `hostlayout`, `hosthub`, and equally `hostname`, `localhost`, `conhost`) is classified AMBIGUOUS: not swapped, reported with file and line.
  The implementer resolves each reported occurrence by judgment, in one of exactly two ways — hand-edit it (so it stops appearing in the next run's report), or name it in `-skip` (so it is recorded as a deliberate keep).
- **Rationale:** `hostclean` and `hostname` are character-class-identical, so no mechanical rule can separate them.
  Reporting rather than guessing keeps the tool correct and reusable, and makes the mechanical/judgment boundary explicit rather than hidden.
- **Applies to:** all batches

### Decision: the report has two buckets and only one drives the exit code

- **Decision:** `wordswap`'s report separates **unresolved AMBIGUOUS** occurrences (no `-skip` claimed them) from **deliberately skipped** occurrences (a `-skip` pattern matched).
  A non-empty unresolved-AMBIGUOUS bucket exits non-zero.
  The deliberately-skipped bucket is printed for the audit trail and never affects the exit code.
- **Rationale:** without the split the two rules collide — `-skip` matches are "left untouched and reported", so if reporting alone drove the exit code no run could ever clear.
  A run is complete when `wordswap` exits zero with an explicit `-skip` set, and that `-skip` set is the reviewable audit record of every deliberate keep.
- **Applies to:** all batches

### Decision: verb-sense `host` in a swept file is reworded before the sweep, never skipped

- **Decision:** where an English verb-sense `host`/`hosts`/`hosting` sits in a file the sweep touches, it is reworded to a synonym in batch 2 rather than protected by `-skip`.
  The four sites are `internal/fabricengine/coalesce.go:1`, `internal/boardengine/board.go:23` and `:26`, and `tools/sandbox/main.go:32`.
- **Rationale:** rewording means the word "host" does not survive in the fabric packages in any sense, which is what makes the tightened enforcement guard in batch 7 trivially verifiable and removes all risk of the tool skipping the wrong occurrence.
- **Applies to:** pre-sweep-rewords, go-sweep

### Decision: non-owner *production* files are hand-edited and never passed to `wordswap`

- **Decision:** exactly three non-owner production files contain the token, and none of them is ever an argument to `wordswap`:
  `internal/configcli/configcli.go` and `internal/builderengine/spawn.go` are hand-reworded to a neutral phrase (drop the qualifier, or say "Fabric" — never `warp`);
  `internal/buildercli/poll.go` is left untouched entirely.
- **Rationale:** `internal/lyxcwd/enforcement_test.go:883` fails any non-owner directory whose production file carries a bare `warp` token, so swapping `configcli.go:269` to "the warp `_lyx` parent" would fail `TestEnforcement_FabricVocabulary` on the spot.
  Hand-editing `spawn.go` additionally averts a silent mis-swap: its `:9` "the plain host filesystem" is bare machine-sense `host` with a clean token boundary, which the tool would swap rather than report.
  Non-owner *test* files are swept freely — `*_test.go` is excluded from all three guard rules.
- **Applies to:** pre-sweep-rewords, go-sweep

### Decision: the ban list keeps the word `host`

- **Decision:** `CONSTRAINTS.md` and `internal/lyxcwd/enforcement_test.go` are never passed to `wordswap`.
  Both name the retired vocabulary in order to forbid it: `CONSTRAINTS.md`'s phrase and identifier lists, and `enforcement_test.go`'s `hostPhrases` slice, `hostGeometryIdentifiers` map, `fabricSenseHostPhrase` predicate and its sense-discrimination sub-tests.
  Both are hand-edited in batch 7.
- **Applies to:** go-sweep, constraints-and-guard

### Decision: vocabulary rule — Fabric outward, warp/weft where the two sides must be told apart

- **Decision:** **Fabric** (capital F) is the name of the fully wired-up composite — warp with junctions into weft inside it — and is what any external reader meaning *the repo as a whole* says.
  **warp** and **weft** are used, including in CLI help text and user-visible messages, at exactly those points where the two sides genuinely must be distinguished (`lyx fabric clone <warp-url> <weft-url>`, `fabric: warp/weft out of sync`).
  **"repo"** alone is too vague to denote warp and is never a substitute for it.
  **"host"** is never used in any of these senses.
- **Rationale:** the CLI user is the most external reader there is, and a `clone` verb taking two URLs cannot avoid naming which is which;
  forcing "Fabric" there would be wrong and a vague "repo" would be worse.
  Reserving warp/weft for genuine two-sided distinctions keeps the composite name meaningful everywhere else.
- **Applies to:** cli-surface-review, docs-sweep, constraints-and-guard

### Decision: the doc work splits in two — `wordswap` does only half of it

- **Decision:** `wordswap` is pointed at documentation only where the text cites a retired **identifier** or a genuine two-sided phrase: `docs/shared-libs/lyxcwd.md`, `manifest/designs/fabric-unified-view.md`, and the five `.claude/agents/crucible-reviewer-*.md` files.
  All consumer prose is hand-reworded file by file, asking per occurrence: does this sentence mean *the composite repo* (→ "Fabric"), or does it genuinely need to distinguish the two sides (→ warp/weft)?
- **Rationale:** a mechanical `host`→`warp` swap over consumer prose produces "the warp repo" precisely where the vocabulary rule demands "the Fabric repo", so running the tool over those files would actively violate the rule this task exists to establish.
- **Applies to:** docs-sweep

### Decision: four historical-record docs are excluded from every sweep

- **Decision:** `docs/benchmarks/test-suite-timing.md`, `docs/benchmarks/fixture-copy.md`, `docs/research/scout-spike.md` and `docs/research/linux-portability-survey.md` are not touched by this task in any way.
- **Rationale:** all four are dated records of measurements and investigations performed at specific past commits, and all four already preserve other retired names from the same era (`internal/warpengine`, `internal/worktree`, `warpcli`, `hubgeometry`).
  Renaming the symbols they cite would falsify what was actually measured.
  This is a closed list, not a rule of thumb — any *other* doc citing a retired identifier is swept.
- **Applies to:** docs-sweep, constraints-and-guard

### Decision: the JSON output field rename is the task's one observable behaviour change

- **Decision:** `json:"host_worktree"` → `json:"warp_worktree"` and `json:"host_branch"` → `json:"warp_branch"` at `internal/fabricengine/status.go:40` and `:44`, `internal/fabricengine/reconcile.go:73`, and `internal/fabricengine/prune.go:22`.
  This changes the fields emitted by `lyx fabric status --json`, `lyx fabric reconcile --json` and `lyx fabric prune --json`, and must be named as such in the sweep commit message.
- **Rationale:** preserving the tags would leave `WarpWorktree string \`json:"host_worktree"\`` — field and tag disagreeing permanently, re-teaching the retired mapping to every future reader.
  Blast radius verified nil in-repo: grep for `host_worktree`/`host_branch` finds only the four declaration sites, and `CONSTRAINTS.md`'s CLI/Cobra Invariant pins the envelope shape, not payload field names.
- **Applies to:** go-sweep

### Decision: the whole Go sweep is one atomic step

- **Decision:** every Go and shell file in the sweep set is passed to a single `wordswap` invocation in batch 3.
  The sweep is never split by package across batches.
- **Rationale:** `HostJunctions`, `HostLyxLink`, `DeriveHostName`, `HostWorktree` and `lyxtest.CopyHostHub`/`HostFixture` have callers in other packages (`internal/fabriccli/clone.go:41`, `internal/loomengine/preflight_integration_test.go:450` and `:521`, `internal/idecli/cli_test.go`, `internal/lyxcwd/anchor_test.go`), so renaming one package without its callers leaves the tree uncompilable.
  A per-package split could not be `go build`-green at its own batch boundary.
- **Applies to:** go-sweep

### Decision: commit granularity follows the discussion's four-commit shape

- **Decision:** card `Commit:` messages carry the discussion's (a)–(d) grouping — (a) rewords + tool + Go sweep, (b) file renames, (c) non-Go/user-visible surfaces, (d) docs + `CONSTRAINTS.md` + guard tightening — even though mill-go commits per card rather than per group.
- **Rationale:** each group is `go build ./... && go test ./...` green and has one readable nature;
  (a)–(c) are mechanical and (d) is the only one requiring judgment, so it is the only one a reviewer must read closely.
- **Applies to:** all batches

## All Files Touched

- `.claude/agents/crucible-reviewer-high.md`
- `.claude/agents/crucible-reviewer-low.md`
- `.claude/agents/crucible-reviewer-max.md`
- `.claude/agents/crucible-reviewer-medium.md`
- `.claude/agents/crucible-reviewer-xhigh.md`
- `CONSTRAINTS.md`
- `README.md`
- `cmd/lyx/tierpurity_test.go`
- `docs/overview.md`
- `docs/sandbox-howto.md`
- `docs/sandbox-hub.md`
- `docs/shared-libs/configengine.md`
- `docs/shared-libs/lyxcwd.md`
- `internal/boardengine/board.go`
- `internal/buildercli/poll_test.go`
- `internal/buildercli/sync_integration_test.go`
- `internal/buildercli/sync_test.go`
- `internal/buildercli/validate_test.go`
- `internal/builderengine/gitquery_test.go`
- `internal/builderengine/spawn.go`
- `internal/configcli/configcli.go`
- `internal/configcli/configcli_integration_test.go`
- `internal/fabriccli/cli_test.go`
- `internal/fabriccli/clone.go`
- `internal/fabriccli/fabric.go`
- `internal/fabriccli/pushbypass_integration_test.go`
- `internal/fabriccli/unwire.go`
- `internal/fabricengine/add.go`
- `internal/fabricengine/add_branch_exists_test.go`
- `internal/fabricengine/add_rollback_adopt_test.go`
- `internal/fabricengine/add_test.go`
- `internal/fabricengine/boardjunction_integration_test.go`
- `internal/fabricengine/boardweft.go`
- `internal/fabricengine/branchname.go`
- `internal/fabricengine/branchname_test.go`
- `internal/fabricengine/checkout.go`
- `internal/fabricengine/checkout_rollback_test.go`
- `internal/fabricengine/classify_test.go`
- `internal/fabricengine/cleanreason_integration_test.go`
- `internal/fabricengine/cleanup.go`
- `internal/fabricengine/clone.go`
- `internal/fabricengine/clone_adopt_test.go`
- `internal/fabricengine/clone_test.go`
- `internal/fabricengine/coalesce.go`
- `internal/fabricengine/coalesce_integration_test.go`
- `internal/fabricengine/config.go`
- `internal/fabricengine/config_driven_junctions_integration_test.go`
- `internal/fabricengine/diff.go`
- `internal/fabricengine/diff_integration_test.go`
- `internal/fabricengine/doc.go`
- `internal/fabricengine/dotlyxjunction_integration_test.go`
- `internal/fabricengine/drift.go`
- `internal/fabricengine/fabric.go`
- `internal/fabricengine/fabric_test.go`
- `internal/fabricengine/healthreason_integration_test.go`
- `internal/fabricengine/hook.go`
- `internal/fabricengine/hook_test.go`
- `internal/fabricengine/hostclean.go`
- `internal/fabricengine/hostjunction_test.go`
- `internal/fabricengine/hostlayout.go`
- `internal/fabricengine/index.go`
- `internal/fabricengine/index_integration_test.go`
- `internal/fabricengine/junction.go`
- `internal/fabricengine/junction_pattern_integration_test.go`
- `internal/fabricengine/junction_repoint_test.go`
- `internal/fabricengine/junction_test.go`
- `internal/fabricengine/junctionnames.go`
- `internal/fabricengine/launcher_content.go`
- `internal/fabricengine/launcher_content_test.go`
- `internal/fabricengine/open_integration_test.go`
- `internal/fabricengine/portallauncher_test.go`
- `internal/fabricengine/post-checkout.sh`
- `internal/fabricengine/prune.go`
- `internal/fabricengine/reconcile.go`
- `internal/fabricengine/reconcile_stale_registration_test.go`
- `internal/fabricengine/reconcile_stale_removal_test.go`
- `internal/fabricengine/remove.go`
- `internal/fabricengine/remove_junctions_integration_test.go`
- `internal/fabricengine/snapshot_integration_test.go`
- `internal/fabricengine/status.go`
- `internal/fabricengine/structuraldirs_test.go`
- `internal/fabricengine/unwire.go`
- `internal/fabricengine/unwire_test.go`
- `internal/fabricengine/warpclean.go`
- `internal/fabricengine/warpjunction_test.go`
- `internal/fabricengine/warplayout.go`
- `internal/fabricengine/weftgit_exclude_test.go`
- `internal/fabricengine/weftgit_unborn_warp_test.go`
- `internal/fabricengine/weftpaths_test.go`
- `internal/fabricengine/weftwiring.go`
- `internal/fabricengine/weftwiring_test.go`
- `internal/fabricengine/worktreelist_test.go`
- `internal/idecli/cli_test.go`
- `internal/loomengine/preflight_integration_test.go`
- `internal/lyxcwd/anchor_test.go`
- `internal/lyxcwd/enforcement_test.go`
- `internal/lyxcwd/lyxcwd_test.go`
- `internal/lyxtest/doc.go`
- `internal/lyxtest/lyxtest.go`
- `internal/lyxtest/lyxtest_test.go`
- `internal/perchcli/run_integration_test.go`
- `internal/webstercli/cli_test.go`
- `internal/webstercli/sync_integration_test.go`
- `internal/webstercli/verbs_test.go`
- `internal/websterengine/audit_test.go`
- `internal/weftname/weftname.go`
- `manifest/designs/fabric-unified-view.md`
- `manifest/designs/loom.md`
- `manifest/designs/warp-visibility.md`
- `manifest/roadmap.md`
- `tools/sandbox/SANDBOX-BUILDER-SUITE.md`
- `tools/sandbox/SANDBOX-BURLER-SUITE.md`
- `tools/sandbox/SANDBOX-CORE-SUITE.md`
- `tools/sandbox/SANDBOX-FABRIC-SUITE.md`
- `tools/sandbox/SANDBOX-PERCH-SUITE.md`
- `tools/sandbox/SANDBOX-REED-SUITE.md`
- `tools/sandbox/SANDBOX-SHUTTLE-SUITE.md`
- `tools/sandbox/SANDBOX-WEBSTER-SUITE.md`
- `tools/sandbox/main.go`
- `tools/sandbox/main_test.go`
- `tools/sandbox/report.go`
- `tools/sandbox/report_test.go`
- `tools/sandbox/suite.go`
- `tools/sandbox/suite_test.go`
- `tools/wordswap/main.go`
- `tools/wordswap/swap.go`
- `tools/wordswap/swap_test.go`
