# Discussion: Producer-agnostic final-summary artifact + wire Finalize

```yaml
task: Producer-agnostic final-summary artifact + wire Finalize
slug: final-summary-artifact
status: discussing
parent: main
```

## Problem

The prose summary artifact that becomes a landing's PR title and body — `_lyx/webster/summary.md`, first line `# <title>` then free-form prose — is today owned end to end by `internal/websterengine`.
Both of its consumers in `internal/landingshed` reach it through webster's own accessors: `Publish` calls `websterengine.ParseSummary(websterengine.SummaryPath(p.deps.WebsterDir))` (`internal/landingshed/publish.go:169`), and `landingshed.Deps` carries a `WebsterDir` field for that one purpose (`internal/landingshed/deps.go:42-44`).
That wiring means the landing producers know *which producer wrote the artifact*, not merely *that a final-summary artifact exists*.
A future last-content producer — Tenter is the named candidate — could not satisfy the same contract without `Finalize`/`Publish` growing a second branch, or without webster's package being dragged in as a dependency of a landing path that has nothing to do with webster.

The second half is a plain gap rather than a design problem: `Finalize` builds its parent-side merge options as `fabricengine.MergeOptions{Squash: fz.deps.Config.Squash}` (`internal/landingshed/finalize.go`), leaving `Message` unset.
`fabricengine` already plumbs that field end to end — `merge.go:453` stores `opts.Message` into the merge-state record and `mergelifecycle.go:40` applies the precedence `explicit msg → st.Message → git's prepared MERGE_MSG/SQUASH_MSG` — so a squash-merged landing commit today carries only git's own generated `SQUASH_MSG`, despite the composed title and body sitting on disk a few lines away.

**Why now:** the artifact contract is about to acquire a second producer, and the moment it does, the current shape becomes a per-producer branch in the consumer. Fixing the ownership and closing the unset-`Message` gap in one pass is cheaper than doing it under a Tenter task's own deadline.

## Scope

**In:**

- New stdlib-only leaf package `internal/summaryparser` owning the artifact's *read* contract: the `summary.md` filename constant, `Path(dir string) string`, the `Summary` struct (`Title`, `Body`), `Parse(path string) (*Summary, error)`, and a `Summary.CommitMessage() string` method returning `Title + "\n\n" + Body`.
- Deletion of `websterengine.SummaryFileName`, `websterengine.SummaryPath`, `websterengine.Summary`, and `websterengine.ParseSummary`; every caller updated to the new package. No compatibility wrappers.
- `landingshed.Deps`: `WebsterDir string` replaced by `FinalSummaryPath string`, a told absolute path to the artifact. `NewPublish` and `NewFinalize` each reject an empty value with a distinct error.
- `internal/loomcli/landingdeps.go` fills the new field from `summaryparser.Path(geom.WebsterDir)`.
- `Publish` reads `summaryparser.Parse(p.deps.FinalSummaryPath)` instead of the webster-rooted call.
- `internal/landingshed/seam_enforcement_test.go`'s `landingshedAllowedImports` drops `internal/websterengine` and gains `internal/summaryparser`. Dropping that entry is this task's own enforcement of producer-agnosticism: the allowlist is a positive membership list, so leaving `websterengine` in it would compile and pass while still authorising the exact producer import the task exists to eliminate.
- `Finalize` parses the artifact at the top of `Call` (after `entryErr`, before the status commit) and sets `MergeOptions.Message` to `summary.CommitMessage()`, unconditionally — for both the squash and non-squash merge shape, and on the step-5 retry merge as well, which reuses the same `mergeOpts` value.
- New `contracts/specs/final-summary-spec.md` pinning the artifact producer-agnostically; `contracts/specs/webster-spec.md`'s summary section reduced to a pointer plus webster's own writer-side additions.
- `docs/overview.md` at two named sites: the kept-durable-contract-docs enumeration (`docs/overview.md:98`, today `loom-status-spec.md`, `webster-spec.md`, `llm-model-spec.md`) gains `final-summary-spec.md`, and the module list gains a `summaryparser` row beside the existing `planparser` and `discussionparser` rows (`docs/overview.md:303-306`). The same edit also adds the missing `loom-plan-spec.md` to that enumeration: the file exists in `contracts/specs/` and is already linked from `docs/overview.md:304`, so its absence from the list is pre-existing drift. Repairing it is a one-word addition on a line this task is editing anyway, and leaving a knowingly incomplete list while adding to it would make the next reader trust it less, not more.
- `CONSTRAINTS.md` updated in the same commit; `manifest/roadmap.md`'s "producer-agnostic final-summary artifact" Planned item marked complete.
- Tests: Tier 1 only (see **Testing**).

**Out:**

- Any Tenter implementation, or any second producer that writes the artifact. This task makes the contract satisfiable by one; it does not add one.
- The artifact's *format*. `# <title>` heading plus free-form body stays exactly as pinned; no YAML frontmatter, no schema, no change to what Master writes or to `contracts/stencils/webster/`.
- The artifact's *location*. `_lyx/webster/` remains webster's, and `websterengine.Dir()` remains the sole declarer of that path segment. Only the filename and the parse move.
- `websterengine.ArchiveStaleSummary` and `websterengine.AppendIntegrationFailure`. Both are producer-specific *write* policy — the first reuses `archive.go`'s `firstFreeArchivePath` and `archiveTimestampFormat`, the second is written by the integration bisect — and the roadmap item is a read-side requirement. They stay in `websterengine` and call into `summaryparser` for the base path.
- `fabricengine.MergeOptions`. No new fields; the existing single `Message string` is used as-is.
- Any change to `shedengine`. The Shed status history does persist a producer's output pointer (`internal/shedengine/run.go:158`), but this task does not make `landingshed` a reader of that history.
- Integration-tier test coverage for the composed commit message.

## Decisions

### summaryparser-leaf-package

- Decision: the artifact's read contract moves to a new package `internal/summaryparser`, importing the standard library and nothing else. It owns `FileName`, `Path`, `Summary`, `Parse`, and `CommitMessage`. Both `websterengine` and `landingshed` import it.
- Rationale: this is the only placement under which neither consumer depends on a producer. It follows the shipped `internal/discussionparser` / `internal/planparser` precedent exactly — a small sole-parser leaf that the format's readers and its writer both depend on.
- Rejected: keeping the code in `websterengine` and telling only the path — `landingshed` would still import `websterengine`, so `Finalize`/`Publish` would still know who wrote the file, which is the whole point of the roadmap item. Also rejected: moving it into `landingshed`, which would make `websterengine` import `landingshed` and invert the layering.

### package-name-summaryparser

- Decision: the package is named `summaryparser`, not `summaryartifact` or `finalsummary`.
- Rationale: it joins the `planparser`/`discussionparser` sole-parser family and reads as one of them. `planparser` already carries a non-parsing write path (`approve.go`), so `CommitMessage` living in a `*parser` package is precedented rather than novel.
- Rejected: `summaryartifact` (recommended during discussion on the grounds that the package owns more than parsing) and `finalsummary` (the roadmap's own wording). The operator chose consistency with the existing family.

### told-final-summary-path

- Decision: `landingshed.Deps.WebsterDir` is deleted and replaced by `FinalSummaryPath string`, the told absolute path to the artifact itself. `internal/loomcli/landingdeps.go` — the layer that legitimately resolves geometry — fills it as `summaryparser.Path(geom.WebsterDir)`.
- Rationale: this is the "shed-level told path" the roadmap item asks for. It satisfies the Told-Geometry Invariant (`landingshed` is a bound package and derives no path of its own) and leaves webster the sole declarer of the `_lyx/webster` directory segment. The consumer now knows a path, not a producer.
- Rejected: keeping `WebsterDir` alongside the new field — `deps.go`'s own ScratchDir comment already argues against carrying a derived near-duplicate. Also rejected: having `Finalize`/`Publish` read the previous producer's output pointer out of the Shed status history, which is genuinely producer-agnostic but makes `landingshed` a reader of shed run state and couples it to the history layout.

### no-compat-wrappers

- Decision: `websterengine.SummaryFileName`, `SummaryPath`, `Summary`, and `ParseSummary` are deleted outright. Every caller moves to `summaryparser` in the same commit.
- Rationale: two names for one thing is exactly how the next producer picks the wrong one. The caller set is small and fully enumerated (see **Technical context**).
- Rejected: keeping thin deprecated wrappers in `websterengine`.

### commit-message-composition

- Decision: the merge commit message is `Title + "\n\n" + Body`, produced by `Summary.CommitMessage()` in `summaryparser`.
- Rationale: git's subject / blank line / body convention. `Summary.Body` is already the artifact's remaining lines verbatim, so an appended `## Integration suite failed` section rides along into the landing commit exactly as it already rides into the PR body. Putting the join behind a method means it is tested once and a future Tenter inherits it.
- Rejected: title only (discards the narrative that is the artifact's entire reason for existing); the file's raw bytes including the `# ` heading (a leading `# ` line is a comment to git's own message handling in some paths and reads wrong in `git log`); explicit `Subject`/`Body` fields on `fabricengine.MergeOptions` (a wider change touching the existing `Message` field, its precedence chain, and the persisted merge-state record, for no gain at one call site); inlining the join in `Finalize.Call`.

### commitmessage-body-trim

- Decision: `CommitMessage` trims leading whitespace from `Body` before joining, and returns the bare `Title` with no trailing blank line when `Body` is empty or whitespace-only. Written out, the exact output is: `Title` when `strings.TrimSpace(Body) == ""`, otherwise `Title + "\n\n" + strings.TrimLeft(Body, " \t\r\n")`. Trailing whitespace is left alone — git strips it from a commit message itself, so trimming it here would be a second implementation of a normalization that already happens.
- Rationale: `Parse` sets `Body` to `strings.Join(lines[headingIdx+1:], "\n")`, so a conventionally formatted artifact — `# Title`, blank line, prose — yields a `Body` whose first character is `\n`. Without the trim, `CommitMessage` emits `Title\n\n\nprose`: a subject, then two blank lines, which reads as a malformed message in `git log` and is not the git subject/blank/body convention this composition exists to follow. Trimming inside `CommitMessage` rather than inside `Parse` is what keeps `Publish`'s PR body byte-identical to today.
- Rejected: no trim at all (emits the double blank line described above); trimming inside `Parse` instead, which would silently change the PR body `Publish` has produced since the artifact shipped and is explicitly out of scope; trimming both ends, which duplicates git's own trailing-whitespace normalization.

### finalize-parse-fails-loud

- Decision: a missing or malformed artifact at `Finalize` time is a hard error returned from `Call` — never `Stuck`, never a silent fallback to an unset `Message`.
- Rationale: `contracts/specs/webster-spec.md` already makes the artifact required and fail-loud on `outcome: done`, and a run that reaches `Finalize` without the artifact is broken, not blocked on something a human resolves by editing the branch — which is the distinction `landingshed`'s existing stuck-vs-error split turns on.
- This is a deliberate tightening, not parity with `Publish`. `Publish` reaches its parse only on a narrow path: the parent branch must be in `Config.RequirePRToBase` (`publish.go:98` returns `Done` immediately otherwise) *and* the existing-PR query must come back empty (`publish.go:168`). `Finalize`'s parse is unconditional and at the top of `Call`, so the artifact becomes mandatory on task-to-task landings where nothing reads it today. That is intended: `Finalize` is the last row of every landing, PR-gated or not, and a landing commit with no composed message is the gap this task exists to close. Only the *disposition* on failure — a returned error rather than `Stuck` — is borrowed from `Publish`.
- Rejected: `Stuck` with a written reason; falling back to today's unset `Message`; gating the parse on `Config.RequirePRToBase` to match `Publish`'s reachability exactly, which would leave the non-PR landing path with the unset message this task is fixing.

### finalize-parse-position

- Decision: the parse happens at the top of `Finalize.Call`, immediately after the `entryErr` check and *before* step 1b's status commit.
- Rationale: the failure then lands before any commit, any catch-up merge-in, and any parent-side mutation, so a broken run never half-lands. The alternative placement reads the artifact only after the catch-up merge-in has already run and mutated the task worktree.
- Rejected: parsing immediately before step 4 where `mergeOpts` is built.

### message-set-unconditionally

- Decision: `Message` is set whether or not `Config.Squash` is true.
- Rationale: `opts.Message` is the conclude-commit message for both merge shapes in `fabricengine`, so gating on `Squash` would leave the non-squash landing commit with today's unset message and create a second code path to reason about for no benefit.
- Rejected: setting it only when `Config.Squash`.
- Reach: the composed message lands on the **warp** conclude commit only. `concludeMergeSides` (`mergelifecycle.go`) does compute one `effectiveMsg` for both arms, but `Merge` writes `WeftOutcome: mergeOutcomeAlreadyUpToDate` into the merge-state record's initial literal (`merge.go:449-460`), and the weft arm's own `st.WeftOutcome != mergeOutcomeAlreadyUpToDate` guard therefore skips it on every record this binary produces — the arm is retained solely for cross-binary compatibility with records a pre-change binary left behind, as `mergelifecycle.go`'s own header states verbatim. This is stated so no one plans around a weft-side commit that is never produced; it must not be restated as an "accepted consequence" in `final-summary-spec.md`, which should say nothing about pair sides at all.

### construction-time-validation

- Decision: `NewFinalize` and `NewPublish` each reject an empty `Deps.FinalSummaryPath` up front, with a distinct error message each, exactly as they already reject a nil `OpenFabric` / `OpenParentFabric` / `PushBranch`.
- Rationale: turns a mis-wired assembly seam into a construction error instead of a first-call surprise, which is the reasoning `NewFinalize`'s own doc comment already states for its resolver.
- Rejected: letting it surface as a file-read error at call time.
- Noted deviation: `Deps`' other string fields (`WorktreeRoot`, `TaskBranch`, `ParentBranch`, `StencilsDir`, `ScratchDir`, `OriginURL`) are not validated in these constructors today. This decision adds validation for the new field only and does not retroactively validate the others — widening that check is out of scope here.

### error-prefix-summaryparser

- Decision: errors originating in the moved code are prefixed `summaryparser: `, not `webster: `. Callers wrap with their own prefix, e.g. `landingshed: Finalize: parse summary artifact: %w`.
- Rationale: the owning package names itself, as every other package in this tree does. A `webster:` prefix on an error raised by a producer-agnostic parser would state the opposite of what this task establishes.
- Rejected: preserving the `webster:` strings verbatim to avoid rewriting existing assertions in `websterengine/summary_test.go`. Those assertions move with the code and get rewritten.

### spec-file-split

- Decision: a new `contracts/specs/final-summary-spec.md` pins the artifact contract producer-agnostically — the format, the fail-loud rules, and both consumers (`Publish`'s PR title/body, `Finalize`'s merge message). `contracts/specs/webster-spec.md`'s "The summary artifact" section shrinks to a pointer at that file plus webster's own writer-side additions: the archive-never-refuse discipline and the appended `## Integration suite failed` section.
- The retained `## Integration suite failed` sentence is corrected while it is being moved, not copied across as-is. `webster-spec.md:43` today reads "because Finalize dumps `summary.md` verbatim into the PR body" — wrong on both counts: the PR body is `Publish`'s (`publish.go:169-182`, which passes `summary.Title` and `summary.Body` as separate `github.NewPullRequest` fields), and nothing dumps the file verbatim. Rewrite it to name `Publish` and drop "verbatim". Pinning that split is what this task is for, so leaving the sentence would contradict the spec it now points at.
- Rationale: the contract stops being webster's the moment a second producer can satisfy it. The Producer Pointer-Rule Invariant already says a producer's own doc points at another's format contract rather than restating it, and leaving the pinned text under webster would leave a Tenter-era reader looking for the contract in the wrong module's doc.
- Rejected: rewording `webster-spec.md` in place (webster keeps owning a contract it no longer solely fulfils); package `doc.go` plus `CONSTRAINTS.md` only, with no spec file.

### summaryparser-invariant

- Decision: `CONSTRAINTS.md` gains a short "Summaryparser Sole-Parser Invariant" — in **production code**, `internal/summaryparser` is the sole declarer of the summary filename and the sole parser of its format; stdlib-only leaf. Kept to the length of the existing `Discussionparser Sole-Parser Invariant` entry, which is two lines.
- Rationale: this is what stops the next producer re-deriving the path or re-implementing the parse, which is the failure mode the whole task exists to prevent. It matches the `planparser`/`discussionparser` precedent.
- The production-only scope is load-bearing, not boilerplate. Bare `"summary.md"` literals already exist across the test tree — `internal/websterengine/recordbatch_test.go:156`, `internal/webstercli/smoke_test.go`, and roughly a dozen sites in `internal/websterengine/runlevel_test.go` — where a fixture writing the literal filename is the clearer test. An unscoped invariant would declare every one of those a violation. This matches how the Lyxdirs Single-Declarer Invariant already scopes itself to "no other production file".
- No declarer-scan test is added. The one enforcing test is the leaf import allowlist (`summaryparser/leaf_enforcement_test.go`), which polices imports, not string literals; the sole-declarer half is review discipline, exactly as it is for `discussionparser` today.
- Decision: `internal/summaryparser` is **not** added to the Told-Geometry Invariant's bound-package list. That list contains `planparser` but not `discussionparser`, and `summaryparser` follows `discussionparser`: its own invariant's stdlib-only leaf rule is strictly stronger than the bound-package rule — no `lyxcwd` import is possible at all, not merely discouraged — and it ships a test enforcing that, so a second listing would be redundant.
- Rejected: appending a sentence to the existing parser invariants instead; no invariant at all; adding `summaryparser` to the Told-Geometry bound-package list.

## Technical context

**The artifact and its current owner.**
`internal/websterengine/summary.go` holds all four names being moved plus the two write helpers that stay.
`SummaryPath` is `filepath.Join(websterDir, SummaryFileName)` with `SummaryFileName = "summary.md"`.
`ParseSummary` performs minimal fail-loud validation: file must be readable, must not be whitespace-only, its first non-blank line must start with `"# "`, and the title after that prefix must be non-empty — each violation its own distinct wrapped error.
`Summary.Body` is `strings.Join(lines[headingIdx+1:], "\n")`, i.e. everything after the heading line verbatim, leading blank line included.
The two helpers that stay — `ArchiveStaleSummary` and `AppendIntegrationFailure` — both currently call `SummaryPath(websterDir)`; after the move they call `summaryparser.Path(websterDir)`.
`ArchiveStaleSummary` builds its target name with its own `fmt.Sprintf("summary-%s%s.md", stamp, suffix)`, so `websterengine` keeps declaring the archive-name shape while `summaryparser` declares the live filename. That split is intentional: the archive discipline (`archive.go`'s `firstFreeArchivePath`, `archiveTimestampFormat = "20060102T150405Z"`) is webster's own and does not follow the artifact to a second producer.

**Complete caller set to update.** These are every production reference to the four moved names:

- `internal/websterengine/runlevel.go:463` — `filepath.Abs(SummaryPath(deps.Geom.WebsterDir))`.
- `internal/websterengine/runlevel.go:633` — `ParseSummary(summaryPath)` on the `outcome: done` path, where a failure is a hard error.
- `internal/websterengine/runlevel.go:651` — `ParseSummary(summaryPath)` on the stuck/paused path, where it is best-effort and the error is discarded.
- `internal/websterengine/runlevel.go:455` — `ArchiveStaleSummary` (helper stays; only its internal path call changes).
- `internal/webstercli/recordbatch.go:109` — fills `RecordBatchDeps.SummaryPath` from `websterengine.SummaryPath(c.geom.WebsterDir)`. The `SummaryPath` *field* on `RecordBatchDeps` (`internal/websterengine/recordbatch.go:54`) is a told string and keeps its name; only the expression filling it changes.
- `internal/shedadapters/webster.go:100` — the `Done` outcome's `OutputPointer{Path: websterengine.SummaryPath(p.deps.Geom.WebsterDir)}`. `shedadapters` already imports `websterengine`, so this becomes an added `summaryparser` import alongside it.
- `internal/shedadapters/doc.go:34` — prose naming `websterengine.SummaryPath`; update the reference.
- `internal/landingshed/publish.go:169` — the parse call, and `internal/landingshed/deps.go:42-44` — the `WebsterDir` field and its doc comment.
- `internal/loomcli/landingdeps.go:41` — `WebsterDir: geom.WebsterDir`.

Test files referencing the moved names: `internal/websterengine/summary_test.go`, `internal/websterengine/recordbatch_test.go`, `internal/websterengine/integration_test.go`, `internal/landingshed/publish_test.go`, `internal/landingshed/publish_integration_test.go`, `internal/shedadapters/webster_test.go`, `internal/webstercli/smoke_test.go`.

**The import allowlist that must move with the code.** `internal/landingshed/seam_enforcement_test.go` declares `landingshedAllowedImports`, a positive membership map that currently lists `internal/websterengine` among eleven permitted non-stdlib imports. Removing the `websterengine` entry and adding `internal/summaryparser` is what makes the package's producer-agnosticism enforced rather than merely intended — the test passes either way if the entry is left behind, so this is easy to miss and the reason it is called out in Scope. Its sibling `internal/shedadapters` has no equivalent allowlist to update; `shedadapters` legitimately keeps its `websterengine` import.

**`Finalize`'s current shape** (`internal/landingshed/finalize.go`). `Call` runs: `entryErr` → step 1b `deps.CommitStatus()` (nil-tolerant) → step 2 `mergeInStep` (catch-up against the parent branch) → step 3 `parentOpener()` → step 4 `parentHandle.Merge(fz.deps.TaskBranch, mergeOpts)` where `mergeOpts := fabricengine.MergeOptions{Squash: fz.deps.Config.Squash}` → step 5 on `*fabricengine.ErrMergeInRequired`, re-run `mergeInStep` and retry the same `mergeOpts` once → steps 6-7 stuck dispositions.
The parse is inserted between `entryErr` and step 1b; `mergeOpts` gains `Message: summary.CommitMessage()`.
The single `mergeOpts` value is reused by the step-5 retry, so no second assignment is needed.
`Finalize` holds the parent handle behind the package-private one-method `parentMerger` seam, which is what lets an in-package test assert on the `MergeOptions` value the producer passes.

**`MergeOptions.Message` is already wired.** `internal/fabricengine/merge.go:87-92` declares it (`Message overrides the conclude commit's message`); `merge.go:453` copies `opts.Message` into the merge-state record; `mergelifecycle.go:40-49` documents and applies the precedence `explicit msg → st.Message → empty`, and `merge.go:490`'s comment states the full chain including git's own prepared `MERGE_MSG`/`SQUASH_MSG`. No `fabricengine` change is required by this task.

**Assembly seam.** `landingshed.Deps` reaches the producers as a whole-struct passthrough: `shedrecipe.Env.Landing` (`internal/shedrecipe/recipe.go:82`) is handed unchanged to `landingshed.NewPublish`/`NewFinalize` by `entries_simple.go`. `internal/loomcli/landingdeps.go` is the sole assembler, does no I/O, and returns no error. Its drift guard (`internal/loomcli/landingdeps_test.go`) walks the returned struct by reflection rather than an enumerated field list, so it picks up the new field automatically once the test fixture stops setting `WebsterDir` and starts setting `FinalSummaryPath`.

**Leaf-enforcement precedent.** `internal/discussionparser/leaf_enforcement_test.go` is the template to copy: a `go/parser` `ImportsOnly` walk of every non-test `.go` file in the package directory, checking each import against an intentionally empty `allowedImports` allowlist, with stdlib detected as "no `.` in the first path segment". Copy it verbatim, renaming the test and the failure message to the new invariant.

**Gotchas.**

- `internal/shedengine` persists a producer's `OutputPointer.Path` into status history (`run.go:158`), so `finalize.go`'s header comment claiming the pointer is one "nobody persists" is inaccurate. This task does not depend on the pointer either way; do not correct that comment as a drive-by unless the change touches those lines anyway.
- `websterengine` sits in the Told-Geometry Invariant's bound-package list, as does `landingshed`. The new leaf takes a told directory or a told path in every exported function and resolves nothing.
- `hubgeom.WebsterDir`/`standalonegeom.WebsterDir` (`websterengine.Dir(...)`) are unchanged — the directory contract is untouched.
- `Summary.Body` retains its leading newline when the artifact has a blank line after the heading, which is why `CommitMessage` trims leading whitespace from `Body` rather than joining it raw. The exact rule is settled in the **commitmessage-body-trim** Decision above and is not a plan-time choice. `Parse`'s own `Body` semantics are unchanged, because `Publish` relies on them for the PR body.

## Constraints

From `CONSTRAINTS.md`:

- **Told-Geometry Invariant** — `landingshed` and `websterengine` are both bound packages: they are handed absolute paths and derive none. `internal/summaryparser` must never import `internal/lyxcwd`. The path is resolved by `internal/loomcli`, the layer entitled to resolve geometry.
- **Lyxdirs Single-Declarer Invariant** — `internal/lyxdirs` alone names `_lyx`/`.lyx`. `summaryparser` declares a bare filename and joins it onto a told directory; it never constructs a `_lyx` path.
- **CLI / Cobra Invariant** — no CLI surface changes in this task; `<module>cli` may import the new leaf, and no engine gains a cli/cobra import.
- **Producer Pointer-Rule Invariant** — drives the `webster-spec.md` reduction: webster's doc points at the final-summary spec rather than restating its format.
- **Markdown Link Integrity** — enforced over `docs/overview.md` only, of the files this task touches. The invariant's scan sources are `manifest/` and `docs/`; `contracts/specs/` is a link *target*, never a scanned source, so the new `final-summary-spec.md` and the edited `webster-spec.md` are outside the enforcing test. Their links must still resolve — that is review discipline here, not invariant coverage, and the distinction matters because a broken link in the new spec file will not fail `go test`.
- **Test Tier Purity Invariant** — every test this task adds is untagged Tier 1: no `gitexec.Run`/`RunGit`, no `exec.Command`, no `gitkit.Copy*`, no `hubforge.NewHub`.
- **Documentation Lifecycle** and this repo's `CLAUDE.md` rule that docs land in the same commit — this change adds cross-cutting infrastructure and touches observable behaviour, so the spec files, `docs/overview.md`, `CONSTRAINTS.md`, and `manifest/roadmap.md` all move with the code.
- **Markdown: semantic line breaks** — one sentence per line in every `.md` file this task writes or edits; no fixed-column hard wrap.

New, introduced by this task:

- **Summaryparser Sole-Parser Invariant** — in production code, `internal/summaryparser` is the sole declarer of the summary artifact's filename and the sole parser of its format; imports the standard library only. Recorded in `CONSTRAINTS.md` in the same commit, kept to the length of the existing `Discussionparser Sole-Parser Invariant` entry. See the **summaryparser-invariant** Decision for the production-only scope, the absence of a declarer-scan test, and why `summaryparser` is not added to the Told-Geometry bound-package list.

## Testing

All Tier 1, untagged.

**`internal/summaryparser` (TDD candidate — the parse rules are fully specified before any code moves).**

- A table-driven `Parse` suite, moved from `internal/websterengine/summary_test.go` and re-pointed at the new package with the `summaryparser:` prefix. Scenarios that must be covered, each already exercised today: missing file; whitespace-only file; a first non-blank line that is not a `# ` heading; a `# ` heading with an empty title; leading blank lines before the heading; and a valid artifact whose `Body` is the remaining lines verbatim.
- `Path` returns the told directory joined with the filename constant.
- `CommitMessage` returns the title, a blank line, and the trimmed body, per the **commitmessage-body-trim** Decision. Cases that must be covered: a conventionally formatted artifact whose `Body` starts with a newline, asserting exactly one blank line between subject and body; a `Body` that is empty, and one that is whitespace-only, both yielding the bare title with no trailing blank line; a `Body` with no leading blank line at all, unchanged by the trim; trailing whitespace left in place; and a `Body` carrying an appended `## Integration suite failed` section, which must survive into the message intact.
- `leaf_enforcement_test.go`, copied from `internal/discussionparser`, asserting the empty allowlist.

**`internal/landingshed`.**

- `finalize_test.go` gains coverage using the existing in-package `parentMerger` fake: a successful merge asserts that the `fabricengine.MergeOptions` the producer passed carries the composed message and the configured `Squash` value. Cover both `Squash: true` and `Squash: false` — the message must be set either way.
- A missing artifact and a malformed artifact each make `Call` return an error, with no merge attempted and no status commit performed — the fake `parentMerger` and the injected `CommitStatus` closure both record whether they were called, which is what proves the parse happens before either.
- The step-5 retry path (`ErrMergeInRequired` then a successful retry) asserts the retry merge carries the same composed message.
- `NewFinalize` and `NewPublish` each reject an empty `FinalSummaryPath` with their own error, alongside the existing nil-closure rejection tests.
- `publish_test.go` / `publish_integration_test.go` update to the new field and the new package; behaviour is unchanged there, so these are mechanical.
- `seam_enforcement_test.go`'s `landingshedAllowedImports` is edited, not merely left to pass: `internal/websterengine` out, `internal/summaryparser` in. Nothing asserts this happened, so it is a plan-level checklist item rather than a test to write.

**Existing fixtures the two changes break — carried by the plan, not discovered at `go test` time.**
The top-of-`Call` parse plus the constructors' empty-`FinalSummaryPath` rejection together make an artifact mandatory on paths that never needed one:

- **The fixture fix belongs in the shared helper.** `internal/landingshed/finalize_test.go:52`'s `newFinalizeDeps(t)` is what every in-package `&Finalize{...}` site builds its `Deps` from — the nine struct literals in `finalize_test.go` and the three in `commitstatus_test.go:52,74,99`, all of which call `Call`. Writing a real artifact into a `t.TempDir()` path and assigning it to `FinalSummaryPath` inside that one helper covers all twelve. Do not patch the call sites individually; the two new cases asserting the missing- and malformed-artifact errors override the helper's value locally instead.
- `internal/landingshed/finalize_integration_test.go:167-193` is the one in-package site that does **not** go through `newFinalizeDeps`: it builds a `landingshed.Deps` literal directly, calls `landingshed.NewFinalize(deps)`, then a real `fz.Call(ctx)` against a `hubforge` fixture. It therefore breaks on both new gates — the constructor's empty-path rejection and the top-of-`Call` parse — and needs a real artifact written to disk, not merely a non-empty path string. It is also the one place a real squash commit is produced, so it is the natural site for anyone who later wants the end-to-end message assertion this task declines to add.
- `internal/shedrecipe/entries_simple_test.go:43`, `internal/shedbuild/fixture_test.go:113`, and `internal/loomrecipe/fixture_test.go:160` each build a `landingshed.Deps` fixture that reaches `NewPublish`/`NewFinalize`, so all three must set a non-empty `FinalSummaryPath` or the constructors reject them. These three packages need no artifact on disk — they never call `Call` — so a non-empty path string is enough.

**`internal/loomcli`.**

- `landingdeps_test.go`'s fixture sets `FinalSummaryPath` instead of `WebsterDir`; the reflection drift guard needs no structural change.

**`internal/websterengine`, `internal/webstercli`, `internal/shedadapters`.**

- Mechanical re-pointing only. `summary_test.go`'s parse cases leave `websterengine` entirely; what remains there is `ArchiveStaleSummary` and `AppendIntegrationFailure` coverage, which keeps its existing shape.

**Not covered.** No integration-tier assertion that the real squash commit's message equals the composed string. `fabricengine`'s own suite already covers `opts.Message` reaching the conclude commit, and this task adds no `fabricengine` code; asserting the producer passes the right `MergeOptions` is the boundary this task owns.

## Q&A log

- **Q:** Where does the generalized path+parse contract live — a new leaf package, `websterengine` with only the path told, or `landingshed`? **A:** New leaf package.
- **Q:** What replaces `Deps.WebsterDir` — a told `FinalSummaryPath`, both fields, or reading the previous producer's Shed output pointer? **A:** Replace with a told `FinalSummaryPath`.
- **Q:** How is the merge message composed — `Title + "\n\n" + Body`, title only, or the file's raw bytes? **A:** `Title + "\n\n" + Body`. The operator first asked whether explicit fields were possible; after being shown the three readings (explicit `Subject`/`Body` on `fabricengine.MergeOptions`, a `CommitMessage()` method, or frontmatter in the artifact) they returned to the plain concatenation, with the method placement settled separately below.
- **Q:** Missing or malformed artifact at `Finalize` time — hard error, `Stuck`, or fall back to an empty message? **A:** Hard error, matching `Publish`.
- **Q:** Set `Message` when `Config.Squash` is false? **A:** Always set it.
- **Q:** Does the write side (`ArchiveStaleSummary`, `AppendIntegrationFailure`) move too? **A:** No — read side only. The operator's reasoning, verbatim in substance: the roadmap item asked specifically that `Finalize`/`Publish` be able to *read* the artifact without knowing which producer wrote it; that is a read requirement, not a write one.
- **Q:** Package name — `summaryartifact`, `summaryparser`, or `finalsummary`? **A:** `summaryparser`, for consistency with `planparser`/`discussionparser`, against the recommendation.
- **Q:** Keep `websterengine.SummaryPath`/`ParseSummary` as wrappers? **A:** No wrappers; update every caller.
- **Q:** Where does the `Title + "\n\n" + Body` join live? **A:** A `Summary.CommitMessage()` method in the new package.
- **Q:** Does this earn a `CONSTRAINTS.md` invariant? **A:** Yes — but keep it short.
- **Q:** Error-message prefix in the moved code? **A:** `summaryparser: `, with callers wrapping in their own prefix.
- **Q:** Where in `Finalize.Call` does the parse happen? **A:** At the top, after `entryErr`, before the status commit.
- **Q:** Is the new told field validated at construction? **A:** Yes — `NewFinalize` and `NewPublish` each reject an empty value with a distinct error.
- **Q:** Where does the artifact contract get documented? **A:** A new `contracts/specs/final-summary-spec.md`, with `webster-spec.md` reduced to a pointer plus webster's writer-side additions.
- **Q:** Test coverage tier? **A:** Tier 1 only; no integration-tier assertion of the real squash commit's message.
