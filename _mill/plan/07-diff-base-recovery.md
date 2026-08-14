# Batch: diff-base-recovery

```yaml
task: "Relocate producer prompt files into a stencils/ directory"
batch: "diff-base-recovery"
number: 7
cards: 3
verify: go build ./... && go test ./internal/gitrepo/... ./internal/fabricengine/... ./cmd/lyx/...
depends-on: [3]
```

## Batch Scope

`lyx stencil diff <name>` needs the default its on-disk file was forked from, and that text exists nowhere except the board repo's own git history.
Because `_lyx` is tracked content and lyx commits its own `_lyx` writes, every default-refresh lands as a commit in the board repo, so the board repo's history is the archive of every default version that hub has ever seen — which is what makes `diff` possible with no historical versions embedded in the binary and no base copies on disk.

This batch builds the two verbs that recover it, and nothing else: a go-git read-blob-at-revision plus path-history walk in `internal/gitrepo`, and the board-scoped accessor in `internal/fabricengine` that wraps them.
It depends only on batch 3 (which established `StencilsDir` and the board-commit path) and touches no engine, so it is disjoint from the batch 4-5-6 chain.

Invariant notes.
The gitrepo Client Boundary Invariant assigns commit/tree/blob lookups and ref reads to go-git, so these verbs add no `gitexec` call site and require no update to that invariant's pinned `gitexec` method list; the gitexec Checked-Call Invariant is likewise untouched because no raw call is added.
The Fabric Git Invariant routes every git operation on either repo through `internal/fabricengine`, which is why the board-scoped accessor lives there rather than being called directly from `internal/stencilcli`.
Line endings must be normalised on both sides before comparison: go-git performs no CRLF conversion at all and returns stored blob bytes untouched, while the working-tree copy it is compared against was written by CLI git, which does convert.

## Cards

### Card 29: Add a go-git blob-read-at-revision and path-history walk to `internal/gitrepo`

- **Context:**
  - `internal/gitrepo/gitrepo.go`
  - `internal/gitrepo/doc.go`
- **Edits:** none
- **Creates:**
  - `internal/gitrepo/blobread.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  New file in `package gitrepo` adding two read-only methods on `*Repo`, both built on go-git and neither touching `r.run` or `r.runChecked`:

  - `func (r *Repo) FileAtRevision(rev, relPath string) ([]byte, error)` — resolves `rev` to its commit's tree and returns the named path's blob contents.
    Model it on `ChangedFilesSince`'s existing shape: guard `rev` with `validSHA`, obtain the repository via `r.goGit()`, resolve the tree through `lookupObjectRetrying(r, repo, func() (*object.Tree, error) { return treeForRev(repo, rev) })`, then read the file's contents from that tree.
    `relPath` is slash-separated and repo-relative, matching what `ChangedFilesSince` returns.
    Return a distinguishable sentinel error — declare `var ErrPathNotAtRevision = errors.New("gitrepo: path not present at revision")` — when the path is absent from that revision's tree, so a caller can tell "not there yet" from a real failure.
    Wrap every other failure as `fmt.Errorf("gitrepo: ...: %w", err)`, matching the package's existing error style.

  - `func (r *Repo) PathRevisions(relPath string, limit int) ([]string, error)` — returns the SHAs of the commits that touched `relPath`, newest first, capped at `limit` when `limit > 0` and uncapped when `limit <= 0`.
    Build it on go-git's log with a filename filter, iterating commits and collecting `commit.Hash.String()`.
    The package has no history iteration today, so this is new surface rather than a refactor.
    Return an empty slice and no error when the path has no history — an unrecoverable base is a normal outcome the caller reports explicitly, not an error condition.

  Take `r.goGitMu.RLock()`/`RUnlock()` around any direct use of the `*git.Repository` value, exactly as `ChangedFilesSince` does around its `repo.Head()` call.
  Do not add any `gitexec` call to this file: reads that resolve state already on disk belong to go-git, and adding one would require amending the gitrepo Client Boundary Invariant's pinned list in the same commit.
- **Commit:** `feat(gitrepo): add go-git blob-at-revision read and path-history walk`

### Card 30: Add the board-scoped forked-from-default accessor to `internal/fabricengine`

- **Context:**
  - `internal/gitrepo/blobread.go`
  - `internal/fabricengine/junctionnames.go`
  - `internal/fabricengine/stencilcommit.go`
  - `internal/stencilstore/stencilstore.go`
  - `internal/lyxdirs/dirs.go`
- **Edits:** none
- **Creates:**
  - `internal/fabricengine/stencilhistory.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  New file in `package fabricengine` declaring one read-only accessor:

  - `func StencilBaseByStamp(hub, name, stampHash string) (base []byte, rev string, found bool, err error)`

  Behaviour, which is the lookup key the design turns on: walk the stencil's path history in the board repo newest-first and take the first revision whose stripped-and-LF-normalised body hashes to `stampHash` — that revision is by definition the default this file was forked from.

  - Build the board-repo-relative path as `path.Join(lyxdirs.LyxDirName, "stencils", stencilstore.RelPath(name))` and open the board repo with `gitrepo.New(BoardDir(hub))`.
  - Call `PathRevisions` on that path, then for each SHA in order call `FileAtRevision` and compute `stencilstore.BodyHash` over the returned bytes. `BodyHash` already strips the leading comment and LF-normalises, which is what makes this comparable against a working-tree copy CLI git may have written with CRLF.
  - Return the first match's contents with `found == true` and its SHA in `rev`.
  - When no revision matches — history pruned, the stamp hand-written, or the file predating its own history — return `found == false` with a nil error. The caller must report this explicitly rather than rendering an empty diff, which would read as "no upstream changes" when the truth is "base not found".
  - Skip a revision whose `FileAtRevision` returns `ErrPathNotAtRevision` rather than aborting the walk.

  This accessor is read-only and mutates nothing, so it takes no `rec *Mutations` and its return is a plain tuple with no `MutationRecord` embed — the Mutation Record Invariant requires the embed of mutating verbs' result types and forbids it on read-only ones.
  It lives in `fabricengine` because the Fabric Git Invariant routes every git operation on either repo through this package; `internal/stencilcli` must never call `gitrepo` directly.
- **Commit:** `feat(fabricengine): recover a stencil's forked-from default by stamp hash`

### Card 31: Test the blob read, the history walk, and stamp-keyed base recovery

- **Context:**
  - `internal/gitrepo/blobread.go`
  - `internal/fabricengine/stencilhistory.go`
  - `internal/fabricengine/stencilcommit.go`
  - `internal/stencilstore/stencilstore.go`
  - `internal/gitkit/hermetic.go`
  - `internal/hubforge/hub.go`
- **Edits:** none
- **Creates:**
  - `internal/gitrepo/blobread_integration_test.go`
  - `internal/fabricengine/stencilhistory_integration_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Both files carry a `//go:build integration` constraint as their first non-empty line — they spawn real git — and both live in packages that must have a `TestMain` calling `gitkit.HermeticGitEnv()`; verify each and add one only if missing.

  `blobread_integration_test.go` asserts, against a `gitkit` primitive repo fixture:
  - `FileAtRevision` returns a file's exact stored bytes at an older commit, unaffected by a later change to the working-tree copy
  - `FileAtRevision` returns `ErrPathNotAtRevision` for a path absent from that revision's tree, distinguishably from a malformed-revision error
  - `FileAtRevision` returns an error for an invalid SHA
  - `PathRevisions` returns the touching commits newest-first, respects a positive `limit`, is uncapped at `limit <= 0`, and returns an empty slice with no error for a path that has no history

  `stencilhistory_integration_test.go` asserts, against a `hubforge` hub whose board has been seeded and re-seeded with a changed default:
  - `StencilBaseByStamp` finds the forked-from revision for a file stamped from an older default, returning that older default's body
  - it returns `found == false` and a nil error when no revision's body matches the stamp, which is the case `diff` must report explicitly instead of rendering an empty diff
  - **hash normalisation**: a board copy whose working-tree bytes were written with CRLF line endings still matches the LF-stored blob, in the base-recovery path specifically. go-git returns stored blob bytes untouched while CLI git converts on checkout, so the two sides can differ by line ending alone — this assertion is what keeps the mechanism working on a machine with `core.autocrlf=true`, where a regression would silently disable base recovery entirely.
- **Commit:** `test(gitrepo,fabricengine): cover blob-at-revision reads and stamp-keyed base recovery`

## Batch Tests

`verify: go build ./... && go test ./internal/gitrepo/... ./internal/fabricengine/... ./cmd/lyx/...`

`internal/gitrepo` and `internal/fabricengine` are the two packages this batch adds files to.
`cmd/lyx` is included for its guard tests specifically, not its behaviour: `gitrepoboundary_test.go` pins gitrepo's `gitexec` method set, `checkedcall_test.go` pins the raw call sites, and `destructiveguard_test.go` pins the `fabricengine` mutation-record and destruction rules — all three react to new files in those two packages, and all three run cheaply.
The two new integration-tagged tests do not run under this untagged `verify:`; they are exercised by `pipeline.done_gate`, which already runs `go test -tags integration ./...`.
