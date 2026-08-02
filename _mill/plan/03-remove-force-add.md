# Batch: remove-force-add

```yaml
task: 'fabric: collapse external API surface onto Commit — stop leaking warp/weft'
batch: remove-force-add
number: 3
cards: 2
verify: go test -tags integration ./internal/gitrepo/ ./internal/fabricengine/
depends-on: [2]
```

## Batch Scope

Remove the now-dead `git add -f` / `hasPathspecMagic` branch from `gitrepo.StageAndCommit` so force-add becomes structurally impossible, record the "never force-add" invariant in `CONSTRAINTS.md` in the same commit, and add a machine-checked grep guard against its re-introduction. This is safe only because batch 2 removed the last caller that passed `:(exclude)` pathspec magic — no caller now trips `hasPathspecMagic`, so no `-f` path is ever taken. `gitrepoboundary_test`'s `TestGitrepoBoundary_PinnedRunCallSites` is unaffected (it pins `StageAndCommit` by method name, which still contains its `r.run(` calls regardless of the `-f` branch). Because removing `-f` changes runtime behaviour for any `commitWeft` call that fed a `:(exclude)` pathspec over `.git/info/exclude`-ignored paths, this batch also reconciles the `fabricengine` integration tests that directly exercised that force-add-backed `:(exclude)` capability, and its `verify:` runs the `fabricengine` integration suite (not just `gitrepo`) so any such break is caught in-batch rather than surfacing as a spurious failure in a later batch.

## Cards

### Card 14: Remove the force-add branch and record the invariant

- **Context:**
  - `cmd/lyx/gitrepoboundary_test.go`
  - `internal/fabricengine/weftgit.go`
- **Edits:**
  - `internal/gitrepo/gitrepo.go`
  - `CONSTRAINTS.md`
  - `internal/fabricengine/weftgit_pathspec_integration_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `internal/gitrepo/gitrepo.go`, delete the `hasPathspecMagic(files)` conditional in `StageAndCommit` (`gitrepo.go:171-193`) so the stage always uses `addArgs := []string{"add", "--"}` (never `"add", "-f", "--"`), and delete the now-unused `hasPathspecMagic` function (`gitrepo.go:340-347`, its only caller was that branch). `StageAndCommit` otherwise keeps its three `r.run(` calls (add, diff --cached, commit) unchanged, so the `gitrepoboundary_test` pinned set does not change — do not edit that test. Also rewrite `StageAndCommit`'s own doc comment (`gitrepo.go:142/159-160`, which describes the `:(exclude)`-entry / `-f` / `hasPathspecMagic` behaviour being removed) so it no longer references the deleted magic/force-add path — trim it to the `golang-comments` shape describing plain `git add -- <files>` staging. Add a new "Never force-add" invariant section to `CONSTRAINTS.md` in this same commit: fabric/gitrepo never runs `git add -f`; transients are kept out of the index by each repo's own `.git/info/exclude` (warp: `seedGitExclude`; weft: `seedWeftArtifactExcludes`), never by force-adding past them and never by per-call `:(exclude)` pathspec magic; enforced structurally (no `-f` code path exists) plus the machine-checked grep guard added in card 15. One line per paragraph, no hard-wrap. Finally, reconcile `weftgit_pathspec_integration_test.go` with the force-add removal: its `:(exclude)`-pathspec cases directly exercise `CommitWeft` passing `:(exclude)` magic through to `git add` and rely on the deleted `-f` to stage past `.git/info/exclude`-ignored paths (the "passthrough" and "exclude-only-no-positive" cases around `weftgit_pathspec_integration_test.go:145-225`, e.g. `:167` `CommitWeft([]string{"_lyx", ":(exclude)_lyx/*.lock"})` and `:212`). Run the `fabricengine` integration suite (this batch's `verify:` now includes it) and, for each such case that fails because it depended on `-f` to force past an exclude-ignored path, delete or rewrite it — that force-add-backed `:(exclude)` capability is being retired (matching card 12's retirement of the "Anchored exclusions" invariant); keep the positive-pathspec cases (`:90`, `:133`) unchanged. Do NOT change `CommitWeft`'s name here (it is unexported later, in batch 6).
- **Commit:** `refactor(gitrepo): remove dead force-add branch; add never-force-add invariant`

### Card 15: Machine-checked never-force-add grep guard

- **Context:**
  - `cmd/lyx/rawgitmutation_test.go`
  - `internal/gitrepo/gitrepo.go`
- **Edits:** none
- **Creates:**
  - `internal/gitrepo/noforceadd_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Create `internal/gitrepo/noforceadd_test.go` — an untagged Go test modeled on `cmd/lyx/rawgitmutation_test.go`'s token-ban structure. It scans the `gitrepo` package's own non-test `.go` source (at minimum `internal/gitrepo/gitrepo.go`) and fails if the banned tokens reappear: the string literal `"-f"` inside an `add` argv context and any reference to `hasPathspecMagic`. Keep it a pure substring/AST-light grep guard (no git spawn) so it stays a Tier-1 untagged test — its first non-empty line must not be a build tag, and it must not call `gitexec.RunGit`/`exec.Command` (Test Tier Purity Invariant). Name the test e.g. `TestNoForceAdd_GitrepoSourceHasNoForceAddBranch`. Include a vacuous-scan floor so an empty scan fails loudly.
- **Commit:** `test(gitrepo): guard against git add -f reintroduction`

## Batch Tests

`verify: go test -tags integration ./internal/gitrepo/ ./internal/fabricengine/` runs the existing `StageAndCommit` real-git tests (proving plain `git add --` still stages correctly and still skips exclude-ignored paths), the new untagged `noforceadd_test.go` guard, and the `fabricengine` integration suite — the last is included specifically so that any `weftgit_pathspec_integration_test.go` case that depended on the removed `-f` (the `:(exclude)`-magic passthrough cases reconciled in card 14) fails here, in the batch that caused it, rather than as a spurious break in batch 4. `CONSTRAINTS.md` is doc-only. The module-wide `go build ./...` boundary check confirms no production package relied on `hasPathspecMagic`.
