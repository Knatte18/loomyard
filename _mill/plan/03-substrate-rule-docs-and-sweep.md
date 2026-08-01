# Batch: substrate-rule-docs-and-sweep

```yaml
task: Formalize the Tier 1/2 substrate rule and re-tier mis-tagged tests
batch: substrate-rule-docs-and-sweep
number: 3
cards: 4
verify: go vet -tags integration ./internal/gitrepo/... ./internal/websterengine/... ./internal/webstercli/...
depends-on: [1, 2]
```

## Batch Scope

This batch writes down the substrate rule this whole task exists to formalize (`docs/benchmarks/running-tests.md`'s "## The two tiers" section, with a one-line pointer from `CONSTRAINTS.md`), fixes the single doc-staleness nit this task's planning-time sweep of all 89 `//go:build integration`-tagged files found (`internal/gitrepo/testmain_test.go`), and records the planning-time confirmation that `internal/webstercli/smoke_test.go` and `internal/websterengine`'s six `integration`-tagged files are correctly tiered. It depends on batches 1 and 2 because the substrate rule's prose names the `scout` tag as a real, existing category (introduced in batch 2) built on the known-tags-list machinery (introduced in batch 1).

No new mis-tiering fix cards appear in this batch because the sweep found none outside `internal/scoutengine` (already handled in batch 2) — see the overview's "the full sweep found zero mis-tiering" Decision for the full reasoning and the reproduction command.

## Cards

### Card 14: Add CONSTRAINTS.md's pointer to the substrate rule

- **Context:**
  - `docs/benchmarks/running-tests.md`
- **Edits:**
  - `CONSTRAINTS.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In the `## Test Tier Purity Invariant` section, add one new bullet immediately after the **Statement** bullet (Card 5 in batch 1 already edited this bullet's wording; do not re-edit it here) and before **Mechanics**, titled `**Substrate definition.**`, reading: the full substrate definition this invariant enforces — real `git` subprocess spawning, real filesystem junctions/symlinks, real `tmux` sessions, real cross-compilation, and real external-binary spawn (the `scout` tag's category) — lives in `docs/benchmarks/running-tests.md`'s "## The two tiers" section; this entry is the terse index pointer, not a duplicate of the explanation.
- **Commit:** `docs(CONSTRAINTS): add a pointer from Test Tier Purity Invariant to running-tests.md's substrate rule`

### Card 15: Rewrite running-tests.md's "## The two tiers" section with the full substrate rule

- **Context:** none
- **Edits:**
  - `docs/benchmarks/running-tests.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Rewrite the `## The two tiers` section (currently three bullets: Tier 1, Tier 2, and the ">This is not a regression" callout) to state the substrate rule explicitly rather than leaving it implicit in "real git" alone. The Tier 2 bullet must enumerate the substrate categories that legitimately earn a test the `-tags integration` (or `-tags scout`) opt-in: real `git` subprocess spawning, real filesystem junction/symlink creation, real `tmux` sessions, real cross-compilation, and real external-binary spawn (a language-server process, or similar) — the category scoutengine's own tests surfaced. State plainly, in the same section, the rule this whole task formalizes: a test earns an opt-in tag for touching one of these substrate categories, never merely for being slow. Add one line naming `smoke` as a third, pre-existing opt-in tag (`go test -tags smoke ./...`) requiring a real logged-in `claude` session, distinct from `scout`'s external-language-server-binary substrate — this section currently never mentions `smoke` at all. Add a fourth `## Commands` example line documenting `go test -tags scout ./...` (mirroring the existing Tier 1/Tier 2 example lines' style) with a one-line note that it is manual-only, hidden behind its own tag, requiring `gopls`/`pyright`/`csharp-ls` on `$PATH` depending on language — no CI wiring exists in this repo for it. Keep every existing paragraph's single-line-per-paragraph markdown style (no hard-wrapping, per this repo's own markdown convention) and keep the existing Tier 1/Tier 2 timing numbers and the `test-suite-timing.md` cross-reference unchanged.
- **Commit:** `docs(running-tests): formalize the substrate rule and document the scout and smoke tags`

### Card 16: Re-run the full sweep and fix the one doc-staleness nit found

- **Context:**
  - `internal/gitexec/testmain_test.go`
- **Edits:**
  - `internal/gitrepo/testmain_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Re-run `grep -rl "^//go:build integration" --include="*_test.go" .` from the repo root to reproduce the current file count (do not assume it is still exactly 89 — this task's own discussion notes the count moves; recount fresh and confirm no file is newly present that this task's planning-time sweep did not already classify against the substrate rule stated in Card 15). The planning-time sweep (five independent research passes covering every file this recount lists, conducted during this plan's authoring) confirmed all of them are tagged for a genuine substrate reason per that rule; the one exception is a doc-accuracy nit, fixed here. In `internal/gitrepo/testmain_test.go`'s file-header comment, change `This mirrors internal/gitexec/testmain_test.go exactly.` to accurately state that only the `TestMain` function body is identical between the two files — `internal/gitexec/testmain_test.go` carries no `//go:build` tag at all (untagged, runs in Tier 1) while this file is `//go:build integration`-tagged (Tier 2-only), because `gitrepo`'s one untagged test file (`keyvalidation_test.go`) does no git spawning and so needs no `HermeticGitEnv()` call, unlike every one of `gitrepo`'s other (tagged) test files.
- **Commit:** `docs(gitrepo): fix testmain_test.go's stale "mirrors gitexec exactly" claim about the build tag`

### Card 17: Record the confirmation for webstercli/smoke_test.go and websterengine's six integration files

- **Context:**
  - `internal/webstercli/smoke_test.go`
  - `internal/websterengine/beginbatch_test.go`
  - `internal/websterengine/gitwrap_test.go`
  - `internal/websterengine/integration_test.go`
  - `internal/websterengine/recordbatch_test.go`
  - `internal/websterengine/recoverbatch_test.go`
  - `internal/websterengine/runlevel_test.go`
- **Edits:** none
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Read each of the seven listed files and independently confirm, rather than taking this task's own discussion or plan on faith: `internal/webstercli/smoke_test.go` is `//go:build smoke`-tagged, requires a real logged-in `claude` on `$PATH` (self-skips via `smokeClaudeBin`'s `exec.LookPath("claude")` check when absent), and its tests exercise the real fork-context guard, a real Agent-tool fork's transcript audit, and the await-batch poll loop — genuine live-substrate behavior no hermetic test can cover, so its tag is correct as-is. Confirm each of the six websterengine files backs its fixture's `WorktreeRoot` with a real scratch git repo: `beginbatch_test.go` via `newScratchRepo` (real `git init`/`config` through `mustGit`/`gitexec.RunGit`) feeding `newBeginFixture`; `gitwrap_test.go` via `gitwrapNewScratchRepo`/`gitwrapMustGit` exercising the package's own `headSHA`/`dirty` helpers against that real repo; `integration_test.go` via the shared `newRunFixture` (itself backed by `newScratchRepo`) plus its own direct `mustGit(..., "symbolic-ref")` HEAD-restoration assertion; `recordbatch_test.go` via `newRecordFixture` (`newScratchRepo`/`commitFile`); `recoverbatch_test.go` via `newRecoverFixture`, with assertions reading real state through `gitrepo.New(fx.Worktree).CurrentSHA()`; `runlevel_test.go` via `newRunFixture` (`newScratchRepo`/`commitFile`), used throughout its end-to-end `Run()` tests. No re-tiering or fixture refactor is expected or permitted on this card — this is a verification-only pass; if any of the above does not hold for the code as it exists at implementation time (rather than as described here), stop and report a non-progress finding rather than silently fixing or re-tagging, since a real discrepancy here would mean this task's own discussion and plan were wrong about a load-bearing premise.
- **Commit:** none

## Batch Tests

`verify:` runs `go vet -tags integration` scoped to the three packages this batch's one code-adjacent edit (Card 16) and Card 17's read-only confirmation subjects live in (`internal/gitrepo`, `internal/websterengine`, `internal/webstercli`) — a cheap compile-sanity check for the doc-comment edit, since Cards 14, 15, and 17 have no runnable surface (pure documentation and a verification-only card respectively). The `-tags integration` flag is required here because Card 16 edits `internal/gitrepo/testmain_test.go`, itself `//go:build integration`-tagged — without the flag, `go vet` would never even compile that file. Card 17 carries `Commit: none` because it edits nothing; its "test coverage" is the confirmation read itself, recorded in this card's Requirements text and in the fixer/implementation notes for this batch, not in a new or changed test.
