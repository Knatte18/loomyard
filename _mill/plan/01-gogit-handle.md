# Batch: gogit-handle

```yaml
task: 'native clients: migrate gitrepo to go-git + selfreport gh-CLI to go-github'
batch: gogit-handle
number: 1
cards: 4
verify: go test -tags integration -race -count=1 ./internal/gitrepo/...
depends-on: []
```

## Batch Scope

Adds the go-git handle infrastructure to `internal/gitrepo` with **zero behaviour change** — no existing method switches backend in this batch. It delivers three things the rest of the gitrepo work sits on: a lazily-opened, cached `*git.Repository` reached through an unexported accessor; the pack-fingerprint-gated reindex-and-retry object-lookup helper every migrated read will route through; and a linked-worktree test fixture, because a standalone `git init` repo cannot detect the failure mode that makes this whole design necessary.

The external interface the next batches consume is two unexported methods on `*Repo`: `goGit() (*git.Repository, error)` and `lookupObject`-style retry helper (exact names in the cards). Nothing exported changes; `New` keeps its no-I/O, cannot-fail contract.

Batch-local decision: the handle open uses `git.PlainOpenWithOptions` with `EnableDotGitCommonDir: true` and **never** `PlainOpen` or `DetectDotGit`. Under `PlainOpen`, a linked worktree returns a handle with no error that reports existing refs as absent — a silent wrong answer, not a failure. This is measured evidence, not caution; see `.scratch/gogit-worktree-probe-report.md`.

## Cards

### Card 1: Lazily-opened cached go-git handle on Repo

- **Context:**
  - `internal/gitnativepoc/gitnativepoc.go`
  - `internal/gitrepo/doc.go`
- **Edits:**
  - `internal/gitrepo/gitrepo.go`
- **Creates:**
  - `internal/gitrepo/gogit.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Add unexported fields to the `Repo` struct in `gitrepo.go`: a `sync.RWMutex`, a `*git.Repository`, and a bool recording that the open succeeded. Do **not** add a `sync.Once` — only a *successful* open may be cached; a failed open must be retried on the next call, because `New`'s documented posture is that the checkout need not exist yet and a `Repo` constructed before `fabricengine` creates the worktree at that path would otherwise fail forever. `New` is unchanged: it still performs no I/O and cannot fail. In the new file `gogit.go`, add `func (r *Repo) goGit() (*git.Repository, error)`: under the write `Lock`, return the cached handle if the success flag is set, otherwise call `git.PlainOpenWithOptions(r.path, &git.PlainOpenOptions{EnableDotGitCommonDir: true})`, caching only on success. `PlainOpen` and `DetectDotGit: true` must appear nowhere — `EnableDotGitCommonDir` defaults to false, so the idiomatic call is the wrong one, and `DetectDotGit` makes the open walk *up* and silently retarget a parent repository. `KeepDescriptors` must stay at its `false` default or packfiles lock against `git worktree remove` on Windows. Wrap the open error so an `extensions.worktreeConfig` refusal (go-git declines to open such a repo outright) surfaces as a clear typed error naming the package rather than a bare library error. **State the caller's locking obligation in `goGit`'s own godoc, because the method cannot enforce it:** the write `Lock` covers only the cache-check-and-open step, so the returned handle is unprotected once it leaves, and **every caller must hold the read lock for the whole duration of its use of that handle** — not merely across the `goGit()` call. That godoc is the single point of truth every migrating card in batches 3 and 4 works from; without it the discipline exists only in the plan and not in the code.
- **Commit:** `feat(gitrepo): add lazily-opened cached go-git handle`

### Card 2: Pack-fingerprint-gated reindex-and-retry lookup helper

- **Context:**
  - `internal/gitrepo/snapshot.go`
  - `internal/gitrepo/push.go`
- **Edits:**
  - `internal/gitrepo/gogit.go`
  - `internal/gitrepo/gitrepo.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Add the shared object-lookup helper every migrated read must route its object lookups through. Behaviour: perform the caller-supplied lookup; on an `object not found` error, compute a **pack fingerprint** — the sorted `(name, size)` list of `*.idx` files in the common dir's `objects/pack` directory — and, **only if that fingerprint differs from the one recorded at the last index build**, call `Reindex()` on the storer (`r.Storer.(*filesystem.Storage)`), record the new fingerprint, and retry the lookup exactly once. If the fingerprint is unchanged, return the not-found as truth with no rescan. The gate is deliberately on-disk state and **not** a per-`Repo` call counter: one physical checkout is addressed concurrently by several live `Repo` values and by a separate OS process (`internal/fabricengine/fabric.go`'s cached `Warp`/`Weft`, `internal/fabricengine/weftgit.go`'s `PushWeftAt`, `internal/websterengine/gitwrap.go`, and `internal/fabriccli/spawn.go`'s detached child), so a counter would miss every write made through any of the others. Store the fingerprint in an unexported `Repo` field — the struct is declared in `gitrepo.go`, which is on this card's `Edits:` for that reason. Locking discipline for the whole package: **RLock for the duration of each go-git call, Lock for handle initialization and for the fingerprint-check/reindex/retry sequence as one unit** — go-git's `filesystem.Storage` builds its object index lazily on first read, so even reindex-free concurrent first reads mutate shared state. The fingerprint field is read and written only inside the write-locked sequence and needs no atomicity of its own. Document on the helper that a concurrent external repack landing between the miss and the fingerprint read can still yield one stale answer, and that this is strictly narrower than the CLI behaviour it replaces.
- **Commit:** `feat(gitrepo): add fingerprint-gated reindex-and-retry object lookup`

### Card 3: Linked-worktree and junction test fixtures

- **Context:**
  - `internal/gitrepo/gitrepo_test.go`
  - `internal/gitrepo/testmain_test.go`
  - `internal/fslink/fslink.go`
- **Edits:** none
- **Creates:**
  - `internal/gitrepo/fixtures_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Create an integration-tagged (`//go:build integration`) helper file in `package gitrepo_test` providing a fixture builder that produces a repository plus a **linked worktree** via `git worktree add`, with the two worktrees deliberately on **different branches and at different HEAD commits** so a common-dir/per-worktree confusion cannot hide behind coincidentally equal values. `refs/loomyard/snapshot/*` lives in the shared common dir while `HEAD` is per-worktree, which is exactly the split that surfaces as a wrong SHA rather than a clean error. Provide a second helper that reaches the same worktree through a Windows directory junction created via `internal/fslink.CreateDirLink`, since that is how lyx actually addresses these directories; skip that helper's cases cleanly on platforms where link creation is unavailable rather than failing. Existing fixtures in `gitrepo_test.go` stay untouched — this file adds, it does not replace.
- **Commit:** `test(gitrepo): add linked-worktree and junction fixtures`

### Card 4: Handle-open behaviour tests

- **Context:**
  - `internal/gitrepo/gogit.go`
  - `internal/gitrepo/fixtures_test.go`
  - `internal/gitrepo/keyvalidation_test.go`
  - `internal/gitrepo/testmain_test.go`
- **Edits:** none
- **Creates:**
  - `internal/gitrepo/gogit_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Create an integration-tagged test file in the **internal** `package gitrepo` (the package's only other internal test file is the untagged `keyvalidation_test.go`; every git-spawning file is external today, so this is new). It is reached by the existing `TestMain` automatically, since one `TestMain` covers both packages of a test binary. Cover: `goGit` succeeds on a standalone repo and on a **linked worktree**, and in the linked-worktree case can read a ref that lives in the common dir — the case `PlainOpen` fails silently. `goGit` on a path that is not a repository returns an error and does not retarget a parent directory. A failed open is **not** cached: after a failure, creating the checkout at that path and calling again succeeds. A successful open **is** cached: two calls return the same pointer. Concurrency: several goroutines calling `goGit` and the lookup helper at once, meaningful only under `-race`, which this batch's `verify:` enables. Also assert that holding an open handle does not block `git worktree remove` — measured to return 0 with `KeepDescriptors` at its default, and a regression here would break `fabricengine`'s topology verbs for reasons unrelated to their own code. Do not use `CurrentBranch` as a smoke test anywhere: it reads only an unresolved HEAD and therefore passes even on a completely broken handle.
- **Commit:** `test(gitrepo): cover go-git handle open, caching, and worktree topology`

## Batch Tests

`verify:` runs `go test -tags integration -race -count=1 ./internal/gitrepo/...` — the package's full Tier-2 suite. Scope is the single package this batch touches. `-race` is not optional here and stays on for every gitrepo batch: the handle is shared mutable state reached from two goroutines by an existing test (`internal/gitrepo/push_test.go`'s concurrent `PushCoalesced` case), and without `-race` this entire class of bug is invisible.

New coverage lands in `internal/gitrepo/gogit_test.go` (handle open, caching, non-caching of failures, concurrency, worktree-remove interop) on fixtures from `internal/gitrepo/fixtures_test.go`. Every existing test in the package must keep passing untouched — no method changes backend in this batch, so any existing-test edit means something went wrong.
