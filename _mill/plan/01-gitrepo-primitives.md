# Batch: gitrepo-primitives

```yaml
task: 'fabric: warp-rebase / remote-reconcile recovery'
batch: gitrepo-primitives
number: 1
cards: 4
verify: go test ./internal/gitrepo/ && go test -tags integration ./internal/gitrepo/ -run 'TestFetch|TestIsAncestor|TestPush' && go test -run TestGitrepoBoundary_PinnedRunCallSites ./cmd/lyx/
depends-on: []
```

## Batch Scope

This batch adds the three `internal/gitrepo` primitives `Fabric.Pull` needs but that do not exist today, all `gitexec`-bound (they authenticate to a remote or read remote-tracking refs): a fetch-without-merge method, an `IsAncestor` reachability primitive, and the exported promotion of `hasUnpushed`. Because all three widen the CLI-bound method set, this batch also updates the boundary guard's pinned set (`cmd/lyx/gitrepoboundary_test.go`), the **gitrepo Client Boundary Invariant** in `CONSTRAINTS.md`, and `internal/gitrepo/doc.go`, in the same batch — per CONSTRAINTS.md that update is mandatory in the same change. The external interface batch 2 consumes: `(*Repo).Fetch() error`, `(*Repo).IsAncestor(sha, ref string) (bool, error)`, and `(*Repo).HasUnpushed() (bool, error)`. Batch-local decision: `IsAncestor` maps `git merge-base --is-ancestor`'s exit codes directly (0 → true, 1 → false, any other → error) rather than treating any non-zero as failure — that tri-state exit is the primitive's whole point.

## Cards

### Card 1: Fetch — fetch-without-merge primitive

- **Context:**
  - `internal/gitrepo/gitrepo.go`
  - `internal/gitrepo/reset.go`
  - `internal/gitrepo/pull_test.go`
- **Edits:**
  - `internal/gitrepo/pull.go`
- **Creates:**
  - `internal/gitrepo/fetch_integration_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Add a new exported method `func (r *Repo) Fetch() error` to `internal/gitrepo/pull.go`, alongside the existing `Pull`. It runs `r.run("fetch")` — refreshing this repo's remote-tracking refs (e.g. `@{u}` / `origin/<branch>`) WITHOUT merging or moving the local branch, unlike `Pull`'s `git pull --ff-only`. Follow `Pull`'s exact error style: on a spawn failure wrap and return the underlying error (`fmt.Errorf("gitrepo: fetch in %s: %w", r.path, err)`); on any other non-zero exit return an error naming the repo path and git's exit code WITHOUT embedding raw stderr (no `fatal:`-prefixed leak), matching `Pull`/`ResetHard`. `Fetch` calls `r.run` exactly once, so it joins the pinned `r.run`-bound method set (card 4). In `fetch_integration_test.go` (first line `//go:build integration`, package `gitrepo`), assert that after a second clone advances a bare remote and the first checkout calls `Fetch()`, the first checkout's remote-tracking ref (`git rev-parse @{u}`) advances to the new tip while its local `HEAD` is UNCHANGED (Fetch merges nothing). Also add an error-path case (`TestFetch_NoRemoteConfigured_*`) mirroring `pull_test.go`'s existing `Pull` no-remote / error-path test: on a repo with no remote configured, `Fetch()` returns a non-nil error whose message names the repo path and does NOT leak git's raw `fatal:`-prefixed stderr (the same no-stderr-leak assertion the `Pull` error-path test makes). Model the bare-remote/second-clone fixture on the package's own existing integration tests (see `internal/gitrepo/pull_test.go` for the fixture idiom and its `Pull` error-path test; use `t.TempDir()`, `git init --bare`, `git clone`, `git remote`/`push -u` directly via `exec.Command` or the package's existing helpers).
- **Commit:** `feat(gitrepo): add Fetch fetch-without-merge primitive`

### Card 2: IsAncestor — reachability primitive

- **Context:**
  - `internal/gitrepo/gitrepo.go`
  - `internal/gitrepo/reset.go`
  - `internal/gitrepo/pull.go`
- **Edits:** none
- **Creates:**
  - `internal/gitrepo/ancestry.go`
  - `internal/gitrepo/ancestry_test.go`
  - `internal/gitrepo/ancestry_integration_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Create `internal/gitrepo/ancestry.go` (package `gitrepo`) with `func (r *Repo) IsAncestor(sha, ref string) (bool, error)`. It answers "is `sha` an ancestor of `ref`" via `r.run("merge-base", "--is-ancestor", sha, ref)`, mapping exit codes: `code == 0` → `(true, nil)`, `code == 1` → `(false, nil)`, any other non-zero → an error naming the repo path and exit code in `Pull`'s no-stderr-leak style; a spawn error (`err != nil`) wraps and returns. Guard both arguments against option-injection before spawning: validate `sha` with the existing `validSHA` (return `ErrInvalidSHA` on failure, exactly like `ResetHard`); reject `ref` only when it begins with `-` (a leading dash would be parsed as a flag) — `ref` is NOT required to be a plain hex SHA because callers pass it a resolved commit SHA in this slice but the primitive must stay usable with symbolic refs, so it uses the lighter leading-dash guard rather than `validSHA`. Return `ErrInvalidSHA` for the rejected-`ref` case too so both bad-argument shapes surface one typed error. `IsAncestor` calls `r.run` once, so it joins the pinned set (card 4). Split the tests by tier across two files, because a `//go:build` constraint is per-file: `ancestry_test.go` (untagged, Tier-1, package `gitrepo`) holds `TestIsAncestor_RejectsInvalidArgs` — assert a `validSHA`-failing `sha` and a leading-dash `ref` each return `ErrInvalidSHA` via `errors.Is`, spawning NO git (required by the Test Tier Purity Invariant for an untagged file); `ancestry_integration_test.go` (first line `//go:build integration`, package `gitrepo`) holds `TestIsAncestor_Reachability` — build a real repo where commit B descends from commit A and assert `IsAncestor(A, B)` is `(true, nil)`, `IsAncestor(B, A)` is `(false, nil)`, and a well-formed-but-absent SHA yields an error. Reuse the package's existing test fixture idiom (see `pull_test.go`) for repo setup.
- **Commit:** `feat(gitrepo): add IsAncestor reachability primitive`

### Card 3: Promote hasUnpushed to exported HasUnpushed

- **Context:**
  - `internal/gitrepo/gitrepo.go`
- **Edits:**
  - `internal/gitrepo/push.go`
  - `internal/gitrepo/gogit.go`
  - `internal/gitrepo/gogit_test.go`
  - `internal/gitrepo/push_test.go`
  - `internal/boardengine/sync.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Rename the unexported method `func (r *Repo) hasUnpushed() (bool, error)` in `internal/gitrepo/push.go` to exported `func (r *Repo) HasUnpushed() (bool, error)`, body unchanged (`r.run("rev-list", "--count", "@{u}..HEAD")` with the same no-upstream-folds-to-true handling). Update its sole internal caller `PushCoalesced` (same file) from `r.hasUnpushed()` to `r.HasUnpushed()`. Update every `hasUnpushed` identifier mention in this file's godoc/comments (the method's own reversal-criterion doc block, and the `PushCoalesced`/`Push` doc comments) to `HasUnpushed`, and add one sentence to `HasUnpushed`'s godoc noting it is exported so `fabricengine.Fabric.Pull` can check local-unpushed state before a fetch. This is the only behavioural addition — the reversal-criterion narrative and measurements stay verbatim. Also update the stale lowercase `hasUnpushed` mentions in the sibling files also in this card's Edits (comment-only, no code): in `internal/gitrepo/gogit_test.go` and `internal/gitrepo/push_test.go` (comment references), and in `internal/boardengine/sync.go` (the `// the hasUnpushed guard` comment) — each becomes `HasUnpushed`. For `internal/gitrepo/gogit.go` the fix is CONTENT-level, not a case-rename: its locking-discipline comment currently lists `hasUnpushed` among the go-git object-lookup methods routed through `lookupObjectRetrying`, which is already wrong today — the method is CLI-bound (`r.run`), takes no go-git handle, and is excluded from that locking discipline (confirmed by the boundary guard's pinned `r.run`-bound set). Remove `hasUnpushed`/`HasUnpushed` from that parenthetical go-git-lookup list entirely rather than case-renaming it into a still-incorrect entry. No test logic changes; these are comment-accuracy edits following the rename.
- **Commit:** `refactor(gitrepo): promote hasUnpushed to exported HasUnpushed`

### Card 4: Pin the new CLI-bound methods (boundary guard, invariant, doc)

- **Context:**
  - `internal/gitrepo/pull.go`
  - `internal/gitrepo/ancestry.go`
  - `internal/gitrepo/push.go`
- **Edits:**
  - `cmd/lyx/gitrepoboundary_test.go`
  - `CONSTRAINTS.md`
  - `internal/gitrepo/doc.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** (1) In `cmd/lyx/gitrepoboundary_test.go`, update the `gitrepoPinnedRunBoundMethods` map: remove `"hasUnpushed"`, and add `"HasUnpushed"`, `"Fetch"`, and `"IsAncestor"` (each now contains an `r.run(` call). Update the `gitrepoBoundaryMinScannedFiles` doc comment's file enumeration to include the new `ancestry.go` (the floor constant `5` need not change — the package now has 8 non-test `.go` files, still above the floor). The `gitexecTotal != 1` assertion is unaffected: the new methods call `r.run`, never `gitexec.` directly. (2) In `CONSTRAINTS.md`, edit the **gitrepo Client Boundary Invariant** "Statement" bullet's exhaustive CLI-bound list — replace the trailing `hasUnpushed` with `HasUnpushed`, and add `Fetch` and `IsAncestor` to the named set, each with a short clause (`Fetch` refreshes remote-tracking refs without merging; `IsAncestor` classifies fast-forward-vs-rewrite and drives the anchor-walk reachability checks) so a reviewer can check a future call against it. (3) In `internal/gitrepo/doc.go`, update the two-backend-boundary paragraph and the `# The Repo API` list: rename `hasUnpushed` → `HasUnpushed` in the CLI-bound enumeration, and add `Fetch` (fetch-without-merge, remote-tracking refresh) and `IsAncestor` (ancestry/reachability) to the CLI-bound side with a one-line description each, mirroring the existing entries' style.
- **Commit:** `test(gitrepo): pin Fetch/IsAncestor/HasUnpushed to the boundary guard`

## Batch Tests

`verify` runs three scopes: (1) `go test ./internal/gitrepo/` — the Tier-1 untagged suite, which compiles the whole package (catching any missed `hasUnpushed` reference from the rename) and runs `ancestry_test.go`'s `TestIsAncestor_RejectsInvalidArgs` guard; (2) `go test -tags integration ./internal/gitrepo/ -run 'TestFetch|TestIsAncestor|TestPush'` — the real-git behaviour of the two new primitives (`TestFetch*`, `TestIsAncestor_Reachability`) plus the `TestPush*` integration tests that exercise the promoted `HasUnpushed` through `PushCoalesced`, scoped by `-run` to avoid the package's full (slower) integration suite; (3) `go test -run TestGitrepoBoundary_PinnedRunCallSites ./cmd/lyx/` — the boundary guard, confirming the pinned set now matches the widened `r.run`-bound method set exactly and the single `gitexec.` call site is still `run`'s own body. The whole-repo `go test ./...` done-gate (configured in `mill-config.yaml`) backstops any package outside this scope.
