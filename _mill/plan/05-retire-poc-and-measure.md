# Batch: retire-poc-and-measure

```yaml
task: 'native clients: migrate gitrepo to go-git + selfreport gh-CLI to go-github'
batch: retire-poc-and-measure
number: 5
cards: 4
verify: go test -tags integration -race -count=1 ./internal/gitrepo/...
depends-on: [4]
```

## Batch Scope

Closes out the gitrepo migration: deletes `internal/gitnativepoc` now that production carries the real implementation and `internal/gitrepo` carries the harness, proves the parity cases can actually fail, and resolves the one open performance question the spike left with no evidence at all.

The performance card is a genuine decision point, not a formality. `hasUnpushed` walks the upstream tip's entire ancestor set with no early exit and sits on a hot path — `PushCoalesced` calls it at every round and sync boundary. The probe measured nothing beyond a handful of commits, so the spike's MIGRATE verdict for it carries no performance evidence. The reversal criterion and its full blast radius are specified below so this stays a scoped change rather than a judgment call made under deadline.

## Cards

### Card 20: Falsification pass

- **Context:**
  - `internal/gitrepo/gitrepo.go`
  - `internal/gitrepo/push.go`
  - `internal/gitrepo/parity_test.go`
  - `internal/gitrepo/gogit_test.go`
- **Edits:** none
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Verification-only card, no diff. For each parity case covering a migrated method, confirm it **fails** when the go-git implementation is temporarily reverted to a plausible-but-wrong form, then restore the correct implementation. Two shapes are mandatory because both were real spike findings rather than invented ones: (a) `ChangedFilesSince` using `Tree.Diff`/`DiffTreeWithOptions` instead of `object.DiffTree`, which folds a rename into one entry and must break the delete-plus-add case; (b) `hasUnpushed` seeding `NewCommitPreorderIter`'s `seenExternal` with only the upstream tip instead of the full ancestor set, which must break the strictly-behind case. Also confirm the `SHAExists` tree/blob case fails under a bare `ResolveRevision` with no commit peel, and that the cross-instance reindex case from card 19 fails when the fingerprint gate is replaced with a per-`Repo` call counter. A parity case that still passes under its known-wrong shape is asserting nothing and must be fixed here, not later. Record the outcome in the batch's own commit message trail — there is no file to write.
- **Commit:** none

### Card 21: Measure hasUnpushed and apply the reversal criterion

- **Context:**
  - `internal/gitrepo/gogit.go`
  - `internal/gitexec/gitexec.go`
  - `internal/boardengine/sync.go`
  - `internal/fabricengine/weftgit.go`
- **Edits:**
  - `internal/gitrepo/push.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Time the migrated `hasUnpushed` against the CLI form it replaced, on the **real loomyard checkout** rather than a fixture — a fixture with a handful of commits is exactly the condition under which the walk looks free. Compare against the CLI's single `rev-list --count` spawn, measuring both the equal-hash shortcut path and the walking path. Record the measured numbers in `hasUnpushed`'s godoc, in the same concrete style the token-resolution timing uses elsewhere in this task, so the next reader can re-check rather than re-derive. **If go-git is slower than the spawn it replaces, revert `hasUnpushed` to the CLI** — its CLI form is one cheap command and no principle here is worth a regression on the package's hottest path. The reversal's blast radius, so it is scoped in advance: `hasUnpushed` rejoins the pinned `r.run` list that batch 8's boundary guard asserts, joins the CLI-bound set the `CONSTRAINTS.md` entry names, and **loses its parity cases entirely** — the no-upstream, never-fetched, failure-swallowing, strictly-behind and linked-worktree runs all become CLI-versus-CLI comparisons that assert nothing, which is the same oracle trap the whole batch-2 design exists to avoid. Delete them rather than keeping them as self-checks; its existing CLI behaviour stays covered by the tests that cover it today. State the measured outcome and which branch was taken in the commit message.
- **Commit:** `perf(gitrepo): measure hasUnpushed go-git walk against CLI baseline`

### Card 22: Delete internal/gitnativepoc

- **Context:**
  - `internal/gitrepo/parity_test.go`
  - `internal/gitrepo/oracle_test.go`
  - `internal/gitrepo/gogit_test.go`
- **Edits:** none
- **Creates:** none
- **Deletes:**
  - `internal/gitnativepoc/doc.go`
  - `internal/gitnativepoc/gitnativepoc.go`
  - `internal/gitnativepoc/read.go`
  - `internal/gitnativepoc/write.go`
  - `internal/gitnativepoc/harness_test.go`
  - `internal/gitnativepoc/read_test.go`
  - `internal/gitnativepoc/write_test.go`
  - `internal/gitnativepoc/testmain_test.go`
- **Moves:** none
- **Requirements:** Delete the whole package directory. Before deleting, confirm three things: no non-test file anywhere in the module imports `internal/gitnativepoc`; every implementation this task needed has been lifted (all of `read.go`'s migrating methods, and `write.go` contributes nothing since every write method stays CLI-bound); and every harness case worth keeping now lives in `internal/gitrepo`'s parity files. Its `doc.go` findings are **not** carried into this batch — they are folded into `internal/gitrepo`'s package doc in batch 9, which is where a future reader will look. Keeping the package as a permanent oracle is not an option: its copy of the go-git logic would have no consumer to keep it honest, and the real oracle is now the CLI layer in `oracle_test.go`.
- **Commit:** `chore(gitnativepoc): delete spike package, superseded by gitrepo migration`

### Card 23: Confirm dependent packages are untouched

- **Context:**
  - `internal/boardengine/sync.go`
  - `internal/fabricengine/fabric.go`
  - `internal/fabricengine/weftgit.go`
  - `internal/websterengine/gitwrap.go`
  - `internal/websterengine/runlevel.go`
  - `internal/fabriccli/spawn.go`
- **Edits:** none
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Verification-only card, no diff. Confirm that no consumer of `internal/gitrepo` required an edit across batches 1–5. The consumers are `boardengine/sync.go`, `fabricengine/{fabric,weftgit}.go`, and `websterengine/{gitwrap,integration,runlevel}.go`; `fabriccli/spawn.go` reaches the same checkout through a detached child process. The public surface was required to be unchanged, so a caller needing an edit means the boundary was drawn wrong and must be raised rather than patched at the call site. Confirm these packages' own test suites pass with the migrated `gitrepo` underneath them by running the module-wide integration suite once — this is the first point where the cross-package effect of swapping the git foundation is observable.
- **Commit:** none

## Batch Tests

`verify:` runs `go test -tags integration -race -count=1 ./internal/gitrepo/...`, unchanged in scope, and must pass with `internal/gitnativepoc` gone — a build failure there would mean the harness lift was incomplete.

Two cards in this batch are verification-only and produce no diff. Card 20 is the batch's real quality gate: a parity suite that passes against known-wrong implementations is worth nothing, and this is the only point in the plan where that is checked directly. Card 23 runs the module-wide integration suite (`go test -tags integration ./... -count=1`) once, since batches 1–5 replace the git foundation that `boardengine`, `fabricengine`, and `websterengine` all sit on, and their suites are the first place a cross-package regression would show.
