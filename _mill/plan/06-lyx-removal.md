# Batch: lyx-removal

```yaml
task: "Extract scout into its own standalone repo"
batch: "lyx-removal"
number: 6
cards: 9
verify: go test ./...
depends-on: [5]
```

## Batch Scope

This batch removes scout from Loomyard entirely: both packages, the `lyx scout` subcommand, every test that asserts scout-shaped behaviour, the `scout` test tier tag, the module-count prose that drops from thirteen to twelve, and every documentation reference.
It is one batch because a partial deletion does not compile — the module list, the seam-signature pins, the help-tree expectations, and the sandbox exclusion list all reference the same two packages — and because splitting it would leave an intermediate commit with a broken build.

It runs only after batch 5 proved quarry equivalent.
Card 45's first act is to re-read the port log's go/no-go line, and the batch stops there if it says no-go.

Batch-local decisions:

- The four research and benchmark documents are **deleted** here, not moved. They were created in quarry in batch 1 card 4. A cross-repo relocation is not a git rename, so there is no `Moves:` pair to express and this batch carries none.
- `manifest/designs/scout-plan-symbol-fields.md` **stays**. It is a loom design that happens to consume scout, so it belongs to the consumer;
  only its links are repointed at quarry.
- No lyx-side replacement is added. No shell-out to a `quarry` binary, no optional binary detection, no vendored artifact, and no external Go module dependency on quarry in `cmd/lyx`. The subcommand is deleted outright.
- The enumeration in the discussion is a list of known high-risk sites, not a complete inventory — it was hand-built and found incomplete on review. Card 46 re-runs the enumeration and is the batch's real completeness gate.

## Cards

### Card 38: delete the two scout packages

- **Context:** none
- **Edits:** none
- **Creates:** none
- **Deletes:**
  - `internal/scoutengine/daemonstate.go`
  - `internal/scoutengine/daemonstate_test.go`
  - `internal/scoutengine/definition.go`
  - `internal/scoutengine/definition_test.go`
  - `internal/scoutengine/detect.go`
  - `internal/scoutengine/detect_test.go`
  - `internal/scoutengine/doc.go`
  - `internal/scoutengine/ensureserver.go`
  - `internal/scoutengine/ensureserver_integration_test.go`
  - `internal/scoutengine/ensureserver_test.go`
  - `internal/scoutengine/errors.go`
  - `internal/scoutengine/load.go`
  - `internal/scoutengine/load_test.go`
  - `internal/scoutengine/lspclient.go`
  - `internal/scoutengine/lspclient_guard_test.go`
  - `internal/scoutengine/lspclient_test.go`
  - `internal/scoutengine/position.go`
  - `internal/scoutengine/position_test.go`
  - `internal/scoutengine/probe.go`
  - `internal/scoutengine/refs.go`
  - `internal/scoutengine/refs_integration_test.go`
  - `internal/scoutengine/refs_test.go`
  - `internal/scoutengine/registry.go`
  - `internal/scoutengine/registry_test.go`
  - `internal/scoutengine/scoutdaemon_test.go`
  - `internal/scoutengine/seam_enforcement_test.go`
  - `internal/scoutengine/supervised_integration_test.go`
  - `internal/scoutengine/supervised_scout_test.go`
  - `internal/scoutengine/supervised_test.go`
  - `internal/scoutengine/symbol.go`
  - `internal/scoutengine/symbol_test.go`
  - `internal/scoutengine/template.go`
  - `internal/scoutengine/template.yaml`
  - `internal/scoutengine/toolchain.go`
  - `internal/scoutengine/toolchain_integration_test.go`
  - `internal/scoutengine/toolchain_test.go`
  - `internal/scoutcli/cli.go`
  - `internal/scoutcli/cli_integration_test.go`
  - `internal/scoutcli/cli_test.go`
  - `internal/scoutcli/testmain_test.go`
- **Moves:** none
- **Requirements:** Delete both directories with `git rm -r internal/scoutengine internal/scoutcli`.
  The tree does not build at the end of this card — `cmd/lyx` still imports `scoutcli` and four of its test files still import `scoutengine` — and cards 39 and 40 resolve that.
  Do not run `go mod tidy` here.
  Both packages' external dependencies (`cobra`, `yaml.v3`, `flock`) are used by other modules in this repo, so tidying would either change nothing or, if it did change something, would mix a dependency-graph edit into the deletion diff where nobody would look for it.
  If `go mod tidy` turns out to be needed at all, it belongs in card 46 after the whole deletion is in.
- **Commit:** `refactor: delete internal/scoutengine and internal/scoutcli`

### Card 39: unwire the scout subcommand from the lyx root command

- **Context:** none
- **Edits:**
  - `cmd/lyx/main.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Remove the `github.com/Knatte18/loomyard/internal/scoutcli` import, the `scoutcli.Command()` entry in the root command's subcommand registration, and the word `scout` from the module list in the root command's `Long` help text.
  The module list is prose that also feeds the help-tree test card 40 updates, so the two must agree exactly — remove the name and its separator without disturbing the order of the rest.
  Add no replacement: no shell-out, no binary detection, no vendored artifact.
- **Commit:** `refactor(cmd): remove the scout subcommand from lyx`

### Card 40: update the cmd/lyx invariant tests

- **Context:**
  - `docs/benchmarks/running-tests.md`
- **Edits:**
  - `cmd/lyx/configstrictness_test.go`
  - `cmd/lyx/constructoranchoring_test.go`
  - `cmd/lyx/helptree_test.go`
  - `cmd/lyx/hermeticenv_test.go`
  - `cmd/lyx/notransients_test.go`
  - `cmd/lyx/sandbox_coverage_test.go`
  - `cmd/lyx/seamsignature_test.go`
  - `cmd/lyx/tierpurity_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Remove every scout reference from these eight files, and adjust the counts that change without containing the word "scout".
  In `helptree_test.go`, drop `"scout"` from the module list and delete the scout help-tree case.
  In `seamsignature_test.go`, drop the `scoutcli` import and both the `scoutcli.RunCLI` and `scoutcli.RunCLIIn` pins;
  then fix the counts in its header comment and its two blank-identifier comments, which say thirteen `RunCLI` and twelve `RunCLIIn` seams — scout carries both, so both drop by one, to twelve and eleven.
  In `constructoranchoring_test.go`, drop the `scoutengine` import and the four `DaemonStateFile`/`DaemonLock` anchoring assertions across its three sites, plus the module name from the file's header comment;
  those assertions pin exactly the anchoring behaviour this task deleted, so they are removed with the module rather than migrated.
  In `notransients_test.go`, do the same for its import, its two table entries, and its header comment.
  In `sandbox_coverage_test.go`, delete the `excludedModules["scout"]` entry — it is required today by the Sandbox Suite Coverage invariant and becomes stale the moment the module leaves, so leaving it in place fails that invariant's own check.
  In `hermeticenv_test.go`, delete the `internal/scoutengine` subprocess-justification entry.
  In `configstrictness_test.go`, remove the module from the comment naming which modules call neither config entry point.
  In `tierpurity_test.go`, remove `"scout"` from `knownTierTags`, delete the two table cases that test the `scout` tag's parsing, delete the two `allowedSpawners` entries naming `internal/scoutengine` test files, and update the file's header comment and the banned-token failure message, both of which enumerate the three tags by name.
  Confirm afterwards that `go test ./cmd/lyx/...` compiles and passes;
  the help-tree, seam-signature, and sandbox-coverage tests are machine guards that will fail loudly if any of this is incomplete, so lean on them rather than re-deriving their expectations by hand.
- **Commit:** `test(cmd): drop scout from the lyx invariant tests`

### Card 41: remove scout from CONSTRAINTS.md

- **Context:** none
- **Edits:**
  - `CONSTRAINTS.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Delete the whole **Scout Engine-Seam Invariant** section, including its enforcement line naming the two guard tests — both tests moved to quarry, where the invariant is now quarry-owned.
  Remove `internal/scoutengine` from the Told-Geometry Invariant's review-obligation list.
  Remove the scout entry from the known-instrumented-call-sites list.
  Remove `scout` from the Sandbox Suite Coverage invariant's list of intentionally-never-exercised modules.
  In the Test Tier Purity section, remove `scout` from the three opt-in tiers and from the untagged-test rule that enumerates them, leaving `integration` and `smoke`.
  Remove `internal/scoutengine` and its `servers.yaml` parenthetical from the list of modules with deliberately degrading config behaviour.
  Fix the two module counts: the line saying twelve of the thirteen seam modules also carry `RunCLIIn`, and the line pinning the seam shapes across all thirteen modules and across the twelve that carry `RunCLIIn`. Both become twelve and eleven.
  Repoint the prose-mention example that cites a `scout-redesign.md` reference in the roadmap: pick a live example that is not scout-related, and cite it in a form that does not pin a line number, since the current citation is already stale by forty lines and this task's own roadmap edit shifts it again.
- **Commit:** `docs: remove the scout engine-seam invariant and scout references from CONSTRAINTS`

### Card 42: remove scout from the overview

- **Context:** none
- **Edits:**
  - `docs/overview.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Delete the scout entry from the module table and the `internal/scoutengine` entry from the package-documentation list.
  Remove scout from the prose sentence listing the modules that write `.lyx` unconditionally.
  Fix the module count in the sentence saying twelve of the thirteen modules also expose `RunCLIIn`: it becomes eleven of the twelve.
  The deleted module-table entry links the scout spike research document, which this batch also deletes;
  the entry goes away with the link, so no repointing is needed there, but check for any other link in this file to a moved document and repoint it at its `https://github.com/Knatte18/quarry` URL rather than leaving it dangling.
  `TestEnforcement_MarkdownLinks` is a machine guard on exactly that and will fail if a link is left pointing at a deleted path.
- **Commit:** `docs: remove scout from the overview module table and counts`

### Card 43: remove scout from the README, roadmap, and parallel-work notes

- **Context:** none
- **Edits:**
  - `README.md`
  - `manifest/roadmap.md`
  - `manifest/parallel-work.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `README.md`, delete the scout bullet from the module list.
  In `manifest/roadmap.md`, handle nine reference sites in three different ways.
  The Someday items about *consuming* scout — the plan-sweep stub note, the scout-backed plan symbol fields item and its design link, the plan-sweep build item and its two supporting lines, the producers-standalone item's uniformity-pass mention, and the plan-format item's deferred symbol fields — all stay, reworded so they name quarry as an external Go module dependency rather than an in-repo module.
  The scout defect item about the `"resolution":"complete"` trust marker on interface-method noise is closed out of the roadmap rather than reworded: it is the same defect already filed as quarry issue 1, and tracking it in two places guarantees drift.
  The Someday `scout` item itself — the LSP-backed code intelligence entry, including its line about two entry points and its pointer at the package documentation — is removed, since the thing it describes has shipped and left.
  The two meta-lines that use `scout` as an example of cross-referencing convention and of a firmly-committed Someday item need a different example each;
  pick items that remain in the file.
  In `manifest/parallel-work.md`, remove the note saying scout extraction is discussed but not yet a wiki task — this task resolves it.
  `manifest/roadmap.md` moves here because this task completes a planned item, which is one of the two cases that legitimately moves the roadmap.
- **Commit:** `docs: remove scout from the README, roadmap, and parallel-work notes`

### Card 44: reword the remaining prose and code-comment mentions

- **Context:** none
- **Edits:**
  - `contracts/specs/loom-plan-spec.md`
  - `internal/fabriccli/clone.go`
  - `internal/fabricengine/junction.go`
  - `internal/gitrepo/doc.go`
  - `internal/loomshed/loomshed.go`
  - `internal/lyxcwd/enforcement_test.go`
  - `internal/websterengine/doc.go`
  - `manifest/designs/fabric-unified-view.md`
  - `manifest/designs/loom.md`
  - `manifest/designs/raddle.md`
  - `manifest/designs/review-finding-classification.md`
  - `manifest/designs/scout-plan-symbol-fields.md`
  - `manifest/designs/semantic-index.md`
  - `manifest/designs/webster-parallel-execution.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Reword every prose and comment mention so it names quarry as an external tool where the statement is still true, and drops the mention where it is not.
  Two categories need care because a machine guard covers them.
  First, every markdown link pointing at the deleted scout engine package documentation or at one of the four moved documents must be repointed at its `https://github.com/Knatte18/quarry` URL — `TestEnforcement_MarkdownLinks` checks these and will fail on a dangling one. The live link in `review-finding-classification.md` to the scout-versus-grep benchmark is the specific instance the discussion names, and `loom-plan-spec.md` and `scout-plan-symbol-fields.md` each carry several more.
  Second, `internal/lyxcwd/enforcement_test.go` mentions `scoutcli/cli.go` in a comment;
  check whether that comment is decorative or whether the file name appears in a geometry-literal allowlist entry that must be dropped, and treat those two cases differently — a stale allowlist entry silently weakens the enforcement, while a stale comment only misleads a reader.
  In the four Go files, the mentions are comments describing which modules write machine-local scratch, which modules use explicit-list staging, and what the operator must stop before a junction move on Windows;
  remove scout from each list rather than rewriting the surrounding sentence.
  `manifest/designs/scout-plan-symbol-fields.md` stays in this repo — it is a loom design about consuming the tool, and it belongs to the consumer — so reword it to describe quarry as an external dependency and repoint all its links, but do not delete it.
- **Commit:** `docs: reword remaining scout mentions to name quarry as an external tool`

### Card 45: retire the scout test tier from the benchmark docs and delete the moved documents

- **Context:**
  - `docs/research/quarry-port-log.md`
- **Edits:**
  - `docs/benchmarks/running-tests.md`
  - `docs/benchmarks/test-suite-timing.md`
- **Creates:** none
- **Deletes:**
  - `docs/benchmarks/scout-vs-grep.md`
  - `docs/research/scout-agent-usage-findings.md`
  - `docs/research/scout-multilang.md`
  - `docs/research/scout-spike.md`
- **Moves:** none
- **Requirements:** Before anything else, read the port log's batch 5 go/no-go line.
  If it says no-go, stop this card and this batch and report;
  the deletion below is irreversible in practice and is authorized only by that line.
  Then, in `running-tests.md`, remove the `scout` tag's own bullet, its mention in the substrate-category definition, its mention in the sentence distinguishing it from the other two tags, and its example command block, leaving `integration` and `smoke`.
  In `test-suite-timing.md`, remove `scoutengine`'s retry timeouts from the list of tests that sit in real wall-clock windows, and pick a remaining example if the sentence needs one to stay readable.
  Delete the four research and benchmark documents with `git rm`;
  they landed in quarry in batch 1.
  Confirm afterwards with `go test ./cmd/lyx/... -run TestEnforcement_MarkdownLinks` that no surviving document links a deleted path.
- **Commit:** `docs: retire the scout test tier and delete the moved research documents`

### Card 46: two-sweep completeness gate

- **Context:**
  - `_mill/discussion.md`
  - `CONSTRAINTS.md`
  - `README.md`
  - `cmd/lyx/helptree_test.go`
  - `cmd/lyx/main.go`
  - `cmd/lyx/tierpurity_test.go`
  - `docs/overview.md`
- **Edits:** none
- **Creates:** none
- **Deletes:**
  - `docs/research/quarry-port-log.md`
- **Moves:** none
- **Requirements:** This card writes nothing;
  it is the gate that decides whether the deletion is finished, and it must be run after every other card in this batch.
  Run the token sweep: `grep -rli 'scout' --exclude-dir=.git --exclude-dir=_mill .`, which returned 73 files before this batch and must now return only files that are deliberate historical mentions.
  Examine every remaining hit and classify it as a deliberate historical mention that stays;
  the sweep is not done while an unexamined hit remains.
  This card fixes nothing.
  If a hit turns out to need a real edit, that is a gap in cards 39 through 45 rather than something this gate patches — stop and report which card should have covered it, so the fix lands in the card that owns the file and is reviewed as part of that card's diff.
  Then run the count-oriented sweep, which no `scout` grep will ever surface, because these facts encode a module count rather than the module's name: search for `thirteen`, `twelve`, and their digit forms across `CONSTRAINTS.md`, `docs/overview.md`, and `cmd/lyx/`, and confirm every seam-module count now reads twelve where it read thirteen and eleven where it read twelve.
  Also confirm the tier-tag list in `cmd/lyx/tierpurity_test.go` and in `CONSTRAINTS.md`'s Test Tier Purity section no longer names `scout`, and that the module lists in `cmd/lyx/main.go`, `cmd/lyx/helptree_test.go`, and `README.md` agree with each other.
  Finish with the whole gate: `go build ./...`, `go test ./...`, `go test -tags integration ./...`, and `golangci-lint run` if it is installed.
  Then close the port log: delete `docs/research/quarry-port-log.md`, which existed only to give each quarry-side batch a task-worktree commit and to carry batch 5's go/no-go verdict into this batch.
  That deletion is what this card commits, and its commit message is where the sweep's results are recorded durably — the surviving hit count and one line per justified historical mention, naming the file and why it stays.
- **Commit:** `chore: close the quarry port log after the completeness sweep`

## Batch Tests

`verify:` runs `go test ./...` from this worktree — the full untagged suite, deliberately unscoped.
That is the right scope here and not the usual per-batch narrowing: this batch deletes two packages and edits eight test files in `cmd/lyx` that are themselves whole-repo invariant guards, so the blast radius is the repository, not a file list.
A narrower command would compile the edited files without proving the module they were guarding is cleanly gone.

The machine guards do most of the work and the cards lean on them rather than duplicating their expectations by hand.
`TestEnforcement_MarkdownLinks` catches a link left pointing at one of the four deleted documents.
The Sandbox Suite Coverage invariant catches the stale `excludedModules["scout"]` entry.
The help-tree test catches a module list that disagrees with the root command's registration.
The seam-signature test catches a count that was not decremented.
`tierpurity_test.go` catches an untagged test that still names the retired tag.

Card 46 adds the two checks no machine guard performs: the token sweep across all file types, and the count-oriented sweep for prose that encodes thirteen modules without naming any of them.
It also runs `go test -tags integration ./...`, which `verify:` does not, and which the configured done gate runs again at task completion.
