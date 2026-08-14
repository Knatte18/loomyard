# Plan: Relocate producer prompt files into a stencils/ directory

```yaml
task: "Relocate producer prompt files into a stencils/ directory"
slug: "stencils-directory-reorg"
approved: true
started: "20260814-105122"
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
    name: stencilstore-foundation
    file: 01-stencilstore-foundation.md
    depends-on: []
    verify: go test ./internal/stencil/... ./internal/stencilstore/... ./internal/fabricengine/...
  - number: 2
    name: stencils-package-and-loom
    file: 02-stencils-package-and-loom.md
    depends-on: [1]
    verify: go build ./... && go test ./stencils/... ./internal/loomengine/... ./internal/lyxcwd/...
  - number: 3
    name: seeding-trigger
    file: 03-seeding-trigger.md
    depends-on: [2]
    verify: go build ./... && go test ./internal/fabricengine/... ./internal/boardengine/... ./cmd/lyx/... ./tools/...
  - number: 4
    name: burler-runtime-read
    file: 04-burler-runtime-read.md
    depends-on: [3]
    verify: go build ./... && go test ./stencils/... ./internal/burlerengine/... ./internal/burlercli/... ./internal/perchcli/... ./internal/lyxcwd/... && go vet -tags smoke ./internal/burlerengine/...
  - number: 5
    name: treadle-runtime-read
    file: 05-treadle-runtime-read.md
    depends-on: [4]
    verify: go build ./... && go test ./stencils/... ./internal/treadleengine/... ./internal/perchengine/... ./internal/perchcli/... ./internal/lyxcwd/...
  - number: 6
    name: webster-runtime-read
    file: 06-webster-runtime-read.md
    depends-on: [5]
    verify: go build ./... && go test ./stencils/... ./internal/websterengine/... ./internal/webstercli/... ./internal/lyxcwd/...
  - number: 7
    name: diff-base-recovery
    file: 07-diff-base-recovery.md
    depends-on: [3]
    verify: go build ./... && go test ./internal/gitrepo/... ./internal/fabricengine/... ./cmd/lyx/...
  - number: 8
    name: stencil-cli
    file: 08-stencil-cli.md
    depends-on: [6, 7]
    verify: go build ./... && go test ./internal/stencilcli/... ./cmd/lyx/...
  - number: 9
    name: reed-rename-and-docs
    file: 09-reed-rename-and-docs.md
    depends-on: [8]
    verify: go build ./... && go test ./internal/reedengine/... ./internal/lyxcwd/... ./cmd/lyx/... && go vet -tags integration ./internal/websterengine/...
```

## Shared Decisions

_Cross-cutting decisions every batch inherits._

### Decision: runtime-read-not-embed

- **Decision:** Producers read their prompt from a file on disk at call time via `stencilstore.Read`, which is a plain `os.ReadFile` with no caching. `//go:embed` is retained solely to carry the shipped default bytes used to seed that file, and is never the path a live read takes.
- **Rationale:** An edited `.md` must take effect with no rebuild, and the instructions driving every LLM must be readable as ordinary files rather than hidden inside a binary. Reading on every call is what "immediate effect" means, and one `os.ReadFile` per prompt render is negligible beside the LLM call that follows it.
- **Applies to:** all batches

### Decision: missing-board-is-a-hard-error

- **Decision:** When the board is unavailable, the producer read path fails loudly. There is no silent fallback to the embedded default at runtime, in any engine, ever.
- **Rationale:** A silent in-memory fallback would mean a producer quietly running against different instructions than the ones on disk — exactly the invisibility this task exists to remove. This costs nothing in tests, because `stencilstore` takes an explicit base directory and tests seed a `t.TempDir()`.
- **Applies to:** stencilstore-foundation, stencils-package-and-loom, burler-runtime-read, treadle-runtime-read, webster-runtime-read

### Decision: told-never-derives

- **Decision:** `internal/stencilstore` takes a fully resolved absolute `baseDir` and never joins `_board` or `_lyx` itself. Resolution lives in `fabricengine.StencilsDir(hub)`, and every engine is handed the resulting directory by its caller.
- **Rationale:** Duplicating either geometry literal inside `stencilstore` would trip `TestEnforcement_GeometryLiterals`, since `internal/fabricengine` owns `_board` and `internal/lyxdirs` owns `_lyx`. Keeping `baseDir` explicit is also what makes `stencilstore`'s tests hermetic against a bare `t.TempDir()`, and what makes treadle's import-allowlist amendment defensible rather than a hole in the invariant.
- **Applies to:** all batches

### Decision: hash-over-the-lf-normalised-stripped-body

- **Decision:** Every hash is taken over the file's body **after** the leading `<!-- ... -->` comment is stripped and after LF normalisation, and every comparison normalises both sides first.
- **Rationale:** A hash stored inside the file cannot cover itself, and stripping the leading comment is what removes the self-reference — it also gives the right semantics, since editing banner prose is not editing the instructions while editing the instructions always changes the hash. LF normalisation is not optional: on a machine with `core.autocrlf=true` the board copy is a git checkout, so an LF file seeded by lyx comes back as CRLF and hashes to neither the stamp nor the shipped default, which would classify *every* stencil as human-edited forever. The base-recovery path needs the same normalisation for the mirror-image reason: go-git performs no CRLF conversion and returns stored blob bytes untouched, while the working-tree copy it is compared against was written by CLI git, which does convert.
- **Applies to:** stencilstore-foundation, seeding-trigger, diff-base-recovery, stencil-cli

### Decision: edit-detection-fails-toward-not-overwriting

- **Decision:** The five-row edit-detection table is evaluated in a fixed order — absent, stamp-matches, body-equals-shipped-default, stamp-mismatch, stamp-missing — and every ambiguous state resolves to "leave it alone".
- **Rationale:** The mechanism must always fail toward not destroying the operator's work. The third row is load-bearing rather than an optimisation: without it, a file whose body has legitimately caught up with the shipped default — after a `promote` and the deploy that follows, or after a hand-reverted edit — keeps a stamp naming the *old* default forever, is classified edited forever, is skipped by every future refresh forever, and never returns to a clean state.
- **Applies to:** stencilstore-foundation, seeding-trigger, stencil-cli

### Decision: dev-builds-seed-but-do-not-refresh

- **Decision:** A `-dev` build performs row 1 (seed when absent) and skips row 2 (refresh when untouched), warning once instead. It performs rows 3, 4, and 5 unchanged — in particular the reconciliation restamp. A production build, and any unstamped binary, performs the full table. An explicit `lyx stencil sync` performs the refresh row even from a `-dev` build.
- **Rationale:** The repo deliberately keeps two binaries with different embedded defaults, and alternating dev and prod runs against the same hub would otherwise rewrite and re-commit the same untouched file in opposite directions on every run — and that alternation *is* the prescribed test-live-then-deploy loop, so it would be the normal case rather than a corner. Row 3 cannot reintroduce that thrash: it writes only the stamp line, which the hash excludes by construction, and it fires only when the body already equals *that* binary's shipped default. `sync` is exempt because an operator who types it is asking for exactly that write.
- **Applies to:** stencilstore-foundation, seeding-trigger, stencil-cli

### Decision: no-automatic-merge

- **Decision:** lyx never merges a newer default into an operator-edited file. When an edited file falls behind, it emits one non-blocking `logger.Warn` and provides `lyx stencil diff <name>`; porting changes across is a human act.
- **Rationale:** A merged LLM instruction carrying conflict markers that nobody read is a producer that misbehaves inexplicably. The diff carries almost all the value at a fraction of the risk.
- **Applies to:** stencilstore-foundation, seeding-trigger, diff-base-recovery, stencil-cli

### Decision: drift-notification-is-logger-warn-and-never-blocks

- **Decision:** Both drift signals — an edited file falling behind a newer default, and a board copy diverging from the worktree's `stencils/` source — are a single `logger.Warn` line that never blocks and never affects an exit code.
- **Rationale:** `logger.Warn` reaches the durable Info+ trace sink and respects existing verbosity configuration, inventing no new channel; the CLI/Cobra Invariant reserves the JSON envelope for errors, and a drift notice is not an error. The port-back warning in particular *cannot* block, because the comparison is inherently cross-worktree: the board copy is one per hub while `stencils/` is per worktree, so the moment one worktree promotes and deploys, every other worktree's older source differs through no fault of its own. That blast radius is accepted precisely because it only prints.
- **Applies to:** seeding-trigger, stencil-cli

### Decision: seeding-commits-under-board-lock-with-a-positive-pathspec

- **Decision:** The seeding commit is a new `internal/fabricengine` verb that acquires `board.lock` and commits the stencils subtree via `gitrepo.StageAndCommit` with an explicit positive pathspec. It commits only and never pushes. It writes only on change, so the common case fires no commit at all.
- **Rationale:** `Bolt` is wrong on both counts: `Bolt.Sync` takes `board.push.lock` while board's own file writes take `board.lock`, so seeding under `Bolt` would not exclude a concurrent `boardCriticalSection`; and `Bolt.Commit` delegates to `StageAllAndCommit`, which would sweep an unrelated dirty board file into the seeding commit. Pushing per run would fire a push on nearly every lyx invocation, so the commit rides board's next push through the existing coalescing path instead.
- **Applies to:** seeding-trigger, stencil-cli

### Decision: seed-at-the-composition-root-never-lazily

- **Decision:** The seed/refresh pass runs once per process at `cmd/lyx`'s root pre-run, never inside `stencilstore.Read`. `stencilstore` writes files and returns the list of paths it wrote; the composition root hands that list to the `fabricengine` commit verb and logs the mutation record.
- **Rationale:** This is load-bearing, not tidiness. A lazy pass inside `Read` would drag `fabricengine` onto treadle's stack through `runCircling`/`runMilestone`/`runTriage`/`runTargeting`, which is exactly what the Runner-Seam allowlist amendment is being justified against. Running it at the root means treadle's new dependency really is one file-reading package. `stencilstore` therefore never imports `fabricengine`, and the registry stays a `Reconcile` parameter rather than a package-level import.
- **Applies to:** seeding-trigger, treadle-runtime-read

### Decision: mutation-record-is-logged-not-enveloped-at-the-pre-run

- **Decision:** The seeding verb's `MutationRecord` is surfaced in `lyx stencil sync`'s envelope like any other mutating verb outcome, and logged rather than enveloped when the pass fires from the root pre-run.
- **Rationale:** The Mutation Record Invariant scopes its fixed `mutations`/`partial` keys to every envelope emitted from a **verb outcome**; a pre-run seed emits no verb-outcome envelope at all. Without this reading, a `lyx board list` that happened to seed fifteen files would either report nothing or silently grow `board`'s envelope — and widening every command's key set was already rejected on its own merits.
- **Applies to:** seeding-trigger, stencil-cli

### Decision: renames-go-through-git-mv

- **Decision:** All sixteen file relocations are expressed as `Moves:` pairs and executed with `git mv <old> <new>` before any other edit to the moved file. Never a `Creates:` plus `Deletes:` pair.
- **Rationale:** Encoding a rename as create-plus-delete destroys git rename history and inflates the review diff on a task that is 60% relocation by file count. Each moved prompt then takes only surgical banner edits.
- **Applies to:** stencils-package-and-loom, burler-runtime-read, treadle-runtime-read, webster-runtime-read, reed-rename-and-docs

### Decision: content-asserting-tests-keep-the-shipped-default-as-their-subject

- **Decision:** `internal/burlerengine/template_test.go`, `internal/treadleengine/template_test.go`, and `internal/websterengine/template_test.go` keep testing the shipped default, switching from the deleted package-private embed vars to the top-level `stencils` package's exported ones. Each also gains one new test asserting an on-disk edit is what reaches `stencil.Fill`.
- **Rationale:** Those files enforce real invariants — burler's enforce the Review Round Invariant, and CONSTRAINTS.md's "Enforced by" pointer names that file by path — so the subject stays the shipped prompt and the file stays where it is. What changes is a cross-package import, not a rename of the variables: the files are in-package (`package burlerengine`, `package treadleengine`) and read vars declared in `template.go`, which these batches delete. Importing the top-level `stencils` package from a `_test.go` file carries no allowlist consequence, since the allowlist rules police production imports only.
- **Applies to:** burler-runtime-read, treadle-runtime-read, webster-runtime-read

### Decision: verify-scope-is-per-batch-with-a-build-guard

- **Decision:** Every batch's `verify:` names only the packages it touches, plus `go build ./...`. The repo-wide regression gate is `pipeline.done_gate`, already configured as `go test ./... && go test -tags integration ./...`, and is not duplicated into any batch.
- **Rationale:** `verify:` runs after every implementer and fixer round, so a repo-wide suite would cost minutes per round. `go build ./...` is the exception worth paying for in eight of nine batches: this task deletes six `//go:embed`-declaring files and changes seven function signatures across package boundaries, so a cross-package compile break is the most likely failure mode and it does not surface in a scoped test run. Integration-tagged tests do not run under any batch `verify:`; they run under `done_gate`.
- **Applies to:** all batches

### Decision: the-wiki-task-body-rewrite-is-out-of-scope-for-the-implementer

- **Decision:** The discussion's scope item "rewriting the wiki task's body" is performed by no card in this plan.
- **Rationale:** `CLAUDE.md` states that all wiki interaction goes through mill's wiki module — the daemon client or the `/mill-*` skills — and that raw `git`, `Edit`/`Write`, or `cp` on wiki files is never permitted. An implementer card cannot satisfy that, so it is left as an operator action to take after this task merges. Every other scope item in the discussion is covered by a card.
- **Applies to:** all batches

## All Files Touched

- `.gitattributes`
- `CLAUDE.md`
- `CONSTRAINTS.md`
- `cmd/lyx/helptree_test.go`
- `cmd/lyx/main.go`
- `cmd/lyx/seamsignature_test.go`
- `cmd/lyx/stencilenvelope_integration_test.go`
- `cmd/lyx/stencilseed.go`
- `docs/overview.md`
- `internal/boardengine/sync.go`
- `internal/burlercli/cli.go`
- `internal/burlerengine/doc.go`
- `internal/burlerengine/engine.go`
- `internal/burlerengine/engine_test.go`
- `internal/burlerengine/prompt.go`
- `internal/burlerengine/prompt_test.go`
- `internal/burlerengine/smoke_cluster_test.go`
- `internal/burlerengine/smoke_round_test.go`
- `internal/burlerengine/template_test.go`
- `internal/fabricengine/junctionnames.go`
- `internal/fabricengine/stencilcommit.go`
- `internal/fabricengine/stencilcommit_integration_test.go`
- `internal/fabricengine/stencilhistory.go`
- `internal/fabricengine/stencilhistory_integration_test.go`
- `internal/fabricengine/stencilsdir_test.go`
- `internal/gitrepo/blobread.go`
- `internal/gitrepo/blobread_integration_test.go`
- `internal/loomengine/discussion.go`
- `internal/loomengine/discussion_test.go`
- `internal/loomengine/plan.go`
- `internal/loomengine/plan_test.go`
- `internal/loomengine/prompt.go`
- `internal/loomengine/prompt_test.go`
- `internal/lyxcwd/enforcement_test.go`
- `internal/perchcli/cli.go`
- `internal/perchcli/run.go`
- `internal/perchengine/engine.go`
- `internal/perchengine/run_test.go`
- `internal/reedengine/console-header.md`
- `internal/reedengine/headertemplate.go`
- `internal/stencil/export_test.go`
- `internal/stencil/stencil.go`
- `internal/stencilcli/cli.go`
- `internal/stencilcli/cli_integration_test.go`
- `internal/stencilcli/diff.go`
- `internal/stencilcli/promote.go`
- `internal/stencilcli/testmain_test.go`
- `internal/stencilstore/doc.go`
- `internal/stencilstore/reconcile.go`
- `internal/stencilstore/reconcile_test.go`
- `internal/stencilstore/stencilstore.go`
- `internal/stencilstore/stencilstore_test.go`
- `internal/stencilstore/validate.go`
- `internal/stencilstore/validate_test.go`
- `internal/treadleengine/engine.go`
- `internal/treadleengine/engine_test.go`
- `internal/treadleengine/judge.go`
- `internal/treadleengine/judge_test.go`
- `internal/treadleengine/run.go`
- `internal/treadleengine/seam_enforcement_test.go`
- `internal/treadleengine/targeting.go`
- `internal/treadleengine/template_test.go`
- `internal/websterengine/integration.go`
- `internal/websterengine/integration_test.go`
- `internal/websterengine/render.go`
- `internal/websterengine/runlevel.go`
- `internal/websterengine/template_test.go`
- `manifest/designs/loom.md`
- `manifest/designs/scout-plan-symbol-fields.md`
- `manifest/designs/shed-followups.md`
- `stencils/burler/burler-step-1-explore.md`
- `stencils/burler/burler-step-2-review.md`
- `stencils/burler/burler-step-3-fix.md`
- `stencils/burler/burler-template-round-orchestrator.md`
- `stencils/loom/loom-template-discussion.md`
- `stencils/loom/loom-template-plan.md`
- `stencils/registry_test.go`
- `stencils/stencils.go`
- `stencils/treadle/treadle-template-judge-circling.md`
- `stencils/treadle/treadle-template-judge-milestone.md`
- `stencils/treadle/treadle-template-targeting.md`
- `stencils/treadle/treadle-template-triage.md`
- `stencils/webster/webster-body-implementer.md`
- `stencils/webster/webster-prefix-fork.md`
- `stencils/webster/webster-prefix-recovery.md`
- `stencils/webster/webster-template-integration.md`
- `stencils/webster/webster-template-master.md`
- `tools/deploy/main.go`
- `tools/sandbox/SANDBOX-CORE-SUITE.md`
