# Batch: remote and push helpers

```yaml
task: 'landing: Publish + Finalize producers'
batch: 'remote and push helpers'
number: 2
cards: 5
verify: go test ./internal/gitrepo/... ./internal/githubclient/... ./internal/fabricengine/... ./cmd/lyx/... && go test -tags integration ./internal/gitrepo/... ./internal/fabricengine/...
depends-on: []
```

## Batch Scope

Three small, independent helpers `Publish` needs and the repo does not have today: reading the `origin` remote URL, parsing an owner/repo pair out of it, and pushing the GitHub-visible branch without rebasing.
They are one batch because each is a handful of lines with its own focused test and none of them has an interesting relationship with the others beyond being `Publish`'s prerequisites — splitting them into three batches would be three round-trips for what one session holds trivially.
It shares no file with batch 1, so the two are genuinely independent roots of the DAG.

The external interfaces batch 4 consumes are `githubclient.ParseOwnerRepo` (called directly by `Publish`) and `gitrepo.RemoteURL` plus the new rebase-free push wrapper (called by the CLI/orchestrator layer that fills `landingshed.Deps`, which the next roadmap item builds).

Batch-local decisions beyond `## Shared Decisions`:

- **`RemoteURL` is go-git-backed, deliberately.** It resolves state already on disk and spawns no process, which keeps it on go-git's side of the gitrepo Client Boundary Invariant and adds no entry to that invariant's pinned method list. Implementing it with `gitexec` would produce the same string, spawn a process for nothing, and force a list update nobody asked for.
- **The push wrapper never becomes `PushWarpAt`.** `PushWarpAt` routes to `gitrepo.PushCoalesced` → `pushWithRebaseRetry`, which rewrites the pushed side's SHAs on a rejected push while the other side is not rebased, desynchronizing the pair and invalidating the correspondence index; it also writes a push-lock file the warp repo has no exclude entry for. The new wrapper routes to `gitrepo.PushRebaseFree`, which does neither.

## Cards

### Card 7: gitrepo.RemoteURL

- **Context:**
  - `internal/gitrepo/gogit.go`
  - `internal/gitrepo/gitrepo.go`
  - `internal/gitrepo/doc.go`
- **Edits:** none
- **Creates:**
  - `internal/gitrepo/remote.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Create `internal/gitrepo/remote.go` in `package gitrepo` with a file-header comment in this package's established style, declaring `func (r *Repo) RemoteURL(name string) (string, error)`.

  It obtains the cached handle via `r.goGit()`, then — holding `r.goGitMu` for the whole duration of its use of that handle, exactly as `CurrentSHA` does with `RLock`/`RUnlock` — calls `repo.Remote(name)` and returns `remote.Config().URLs[0]`.
  A remote that exists but carries an empty `URLs` slice is an error, not an index panic: guard the length before indexing.
  Wrap a lookup failure as `fmt.Errorf("gitrepo: read remote %q URL in %s: %w", name, r.path, err)`.

  Its godoc states that this is a local config read backed by go-git, that it spawns no git process, and that it therefore does not belong on the gitrepo Client Boundary Invariant's pinned `gitexec` method list.
  Do not add `RemoteURL` to `gitrepoPinnedRunBoundMethods` — it reaches no CLI and the guard is set-equality, so adding it would fail the build.
- **Commit:** `feat(gitrepo): add go-git-backed RemoteURL`

### Card 8: RemoteURL integration coverage

- **Context:**
  - `internal/gitrepo/remote.go`
  - `internal/gitrepo/merge_integration_test.go`
  - `internal/gitrepo/testmain_test.go`
  - `internal/gitkit/gitkit.go`
- **Edits:** none
- **Creates:**
  - `internal/gitrepo/remote_integration_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Create `internal/gitrepo/remote_integration_test.go` opening with the `//go:build integration` constraint as its first line, followed by a file-header comment, in whichever package this directory's existing integration test files use.
  It builds a real repository with `gitkit`'s helpers and covers: a repo with an `origin` remote returns that remote's configured URL verbatim; a repo with no such remote returns an error rather than an empty string; and a repo whose remote name is misspelled by the caller returns an error naming the requested remote.
  The `//go:build integration` tag is mandatory here rather than optional — the Test Tier Purity Invariant forbids an untagged test file from spawning git or calling `gitkit.MustRun`, and constructing a repo with a configured remote requires exactly that.
- **Commit:** `test(gitrepo): cover RemoteURL against real repositories`

### Card 9: githubclient.ParseOwnerRepo

- **Context:**
  - `internal/githubclient/githubclient.go`
  - `internal/githubclient/doc.go`
  - `internal/githubclient/leaf_enforcement_test.go`
- **Edits:** none
- **Creates:**
  - `internal/githubclient/parseownerrepo.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Create `internal/githubclient/parseownerrepo.go` in `package githubclient` with a file-header comment, declaring `func ParseOwnerRepo(remoteURL string) (owner, repo string, err error)`.

  It is a pure standard-library string function — no network, no go-github call, no process — so it stays inside this package's own leaf allowlist (stdlib, go-github, `golang.org/x/sys`, `internal/proc`).
  It accepts exactly these shapes, each with an optional `.git` suffix and an optional single trailing slash:

  - the SSH form, `git@github.com:owner/repo.git`;
  - the HTTPS form, `https://github.com/owner/repo.git`.

  Anything else is an error with a distinct message: a non-GitHub host, a URL with no owner/repo segment pair, and unparseable garbage each produce their own wrapped error text so `Publish` can report which failure it hit.
  An empty `owner` or an empty `repo` after parsing is an error, never a silently returned empty string.
  The godoc states the accepted forms explicitly and states that the parser belongs here because this package owns GitHub knowledge.
- **Commit:** `feat(githubclient): add ParseOwnerRepo remote-URL parser`

### Card 10: ParseOwnerRepo table test

- **Context:**
  - `internal/githubclient/parseownerrepo.go`
  - `internal/githubclient/githubclient_test.go`
- **Edits:** none
- **Creates:**
  - `internal/githubclient/parseownerrepo_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Create `internal/githubclient/parseownerrepo_test.go` as a table test over `ParseOwnerRepo`, in whichever package this directory's existing non-integration test files use.
  Cases: SSH form with and without the `.git` suffix; HTTPS form with and without the `.git` suffix; HTTPS form with a trailing slash; a non-GitHub host; a URL with no owner/repo pair; the empty string; and outright garbage.
  Each error case asserts an error is returned and that both returned strings are empty, and the distinct-message cases assert their message differs from the others rather than asserting exact prose.
  This file stays untagged, which is correct and required: it spawns nothing, so the Test Tier Purity Invariant leaves it in tier 1.
- **Commit:** `test(githubclient): table-test ParseOwnerRepo across every accepted form`

### Card 11: rebase-free push wrapper

- **Context:**
  - `internal/fabricengine/weftgit.go`
  - `internal/fabricengine/fabric.go`
  - `internal/gitrepo/push.go`
- **Edits:**
  - `internal/fabricengine/spawn.go`
- **Creates:**
  - `internal/fabricengine/pushrebasefree_integration_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Add a new exported function to `internal/fabricengine/spawn.go`, declared immediately after `PushWarpAt`, named `PushWarpRebaseFreeAt`, with the signature `func PushWarpRebaseFreeAt(warpPath string, opts SyncOptions) (res PushResult, err error)`.

  Its body mirrors `PushWarpAt`'s exactly except for the push call: construct the recorder with `NewMutations(filepath.Dir(warpPath))` and the snapshot defer, return an empty result and a nil error when `opts.SkipGit || opts.SkipPush`, sample `repo.HasUnpushed()` before pushing, call `repo.PushRebaseFree()` instead of `repo.PushCoalesced()`, and record via the existing `recordPushIfAdvanced` helper on success.

  Its godoc must state the two hazards this routing discharges rather than merely mitigates, because they are the entire reason it exists beside `PushWarpAt`: `PushRebaseFree` never runs `git pull --rebase`, so it never rewrites this side's SHAs while the paired side is not rebased and never invalidates the correspondence index; and it never takes the push lock, so it leaves no untracked lock-file residue in the operator's own repo, which is the undischarged precondition `PushWarpAt`'s own doc comment names.
  It also states that a rejected push surfaces as `gitrepo.ErrPushRejected`, which the caller is expected to treat as a human-decidable condition rather than retrying.
  Do not edit `PushWarpAt` itself — its "no production caller" doc comment stays true, and no warp-side exclude seeding enters this task.
  Update this file's own header comment so it names the new function alongside the two it already describes.

  Also create `internal/fabricengine/pushrebasefree_integration_test.go`, opening with the `//go:build integration` constraint as its first line, in `package fabricengine_test`, covering: a real pair with a commit ahead of its upstream pushes successfully and the returned record reports the branch push; `SkipGit` and `SkipPush` each return an empty result with a nil error and push nothing; and no untracked push-lock file is left behind afterwards, which is the residue property this function exists to guarantee.
- **Commit:** `feat(fabricengine): add rebase-free warp push wrapper`

## Batch Tests

`verify:` runs the fast tiers of all three touched packages plus `cmd/lyx`, then the two integration tiers this batch creates tagged files in.
`cmd/lyx` is in the untagged half specifically to prove the two negative guard properties this batch depends on: `TestGitrepoBoundary_PinnedRunCallSites` must stay green with `RemoteURL` *absent* from its pinned map (go-git reads do not belong there), and `TestGHGuard_NoShellOutOutsideGithubclient` plus `internal/githubclient`'s own leaf test must stay green with the new stdlib-only parser added.
The `-tags integration` half is required because cards 8 and 11 each create a tagged test file; an untagged-only run would compile neither, and card 8's scenarios cannot exist untagged at all without violating the Test Tier Purity Invariant.
