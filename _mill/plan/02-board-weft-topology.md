# Batch: fabricengine+fabriccli: _board as second weft worktree

```yaml
task: 'board: move storage to weft:main'
batch: 'fabricengine+fabriccli: _board as second weft worktree'
number: 2
cards: 6
verify: go test ./internal/fabricengine/... ./internal/fabriccli/... && go test -tags integration ./internal/fabricengine/... ./internal/fabriccli/...
depends-on: []
```

## Rename mechanic

Not applicable — this batch contains no `Moves:` entries (the new `boardweft.go` is a `Creates:`, not a relocation of existing code — `_mill/discussion.md`'s Decisions section explicitly rules out `weftwiring.go` as the host for this logic, since that file's own header comment states every branch argument it handles is always a pre-suffixed weft branch name, which `_board`'s deliberately-unsuffixed branch would falsify).

## Batch Scope

This batch retargets hub bootstrap (`lyx fabric clone`) so `<Hub>/_board` is materialized as a second `git worktree add` inside the already-cloned weft repo, replacing today's separate `.wiki.git` clone. `CloneHub` drops its `boardURL` parameter and return value entirely; `suffixWeftPrimaryBranch`'s signature changes to return the host branch name it already reads (so `_board`'s worktree-add can reuse it instead of an incorrect post-rename re-read); a new file `boardweft.go` holds the create-or-adopt-or-orphan worktree-add logic. `internal/fabriccli`'s `clone` command becomes 2-arg (`<host-url> <weft-url>`). This batch is root (no dependency on batch 1's `CommitWeftAt`) — hub bootstrap and board's own runtime git-commit path are independent concerns; `boardengine`'s runtime code (batch 3) never calls `CloneHub`, it only assumes `_board` already exists at `hubgeometry.BoardDir(hub)`, which this batch preserves byte-for-byte. The external interface this batch hands to later batches: `CloneHub(cwd, hostURL, weftURL string) (hubPath string, err error)` (2 URL args, 1 return value) and `lyx fabric clone <host-url> <weft-url>` (2 positional args, no `[board-url]`).

## Cards

### Card 4: `suffixWeftPrimaryBranch` returns the host branch it read

- **Context:**
  - `internal/hubgeometry/hubgeometry.go`
- **Edits:**
  - `internal/fabricengine/clone.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Change `suffixWeftPrimaryBranch(weftPath string) error`'s signature to `suffixWeftPrimaryBranch(weftPath string) (hostBranch string, err error)`. Every existing early `return fmt.Errorf(...)` becomes `return "", fmt.Errorf(...)`; the function already computes a local `hostBranch` variable via `git branch --show-current` before the rename — reuse that same variable name as the named return, and change the final `return nil` to `return hostBranch, nil`. No other logic in the function changes (the adopt-vs-create branch-name derivation, the `WeftBranchName(hostBranch)` call, and the `checkout -b`/`origin/<suffixed>` git invocations are untouched). Update the function's doc comment to add: "Returns the host branch name it read (before the rename) so `CloneHub`'s `_board`-worktree-add step can reuse it directly — re-reading `git branch --show-current` at `weftPath` after this function returns would incorrectly see the already-renamed `<hostBranch>-weft`, not `hostBranch`."
- **Commit:** `refactor(fabricengine): suffixWeftPrimaryBranch returns the host branch it read`

### Card 5: `CloneHub` drops the board-clone step, materializes `_board` as a weft worktree

- **Context:**
  - `internal/hubgeometry/hubgeometry.go`
  - `internal/fabricengine/boardweft.go`
- **Edits:**
  - `internal/fabricengine/clone.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Change `CloneHub`'s signature from `CloneHub(cwd, hostURL, weftURL, boardURL string) (hubPath, resolvedBoardURL string, err error)` to `CloneHub(cwd, hostURL, weftURL string) (hubPath string, err error)`. Update the call site of `suffixWeftPrimaryBranch` (step 6b) to capture its new return value: `hostBranch, err := suffixWeftPrimaryBranch(weftPath)`, propagating the error through `teardownHub` exactly as today. Delete step 7's board-URL resolution block (`board := boardURL; if board == "" { board = deriveBoardURL(weftURL) }`) and step 8's `cloneRepo(board, hubgeometry.BoardDir(hubPath))` call entirely. Replace them with a single call to the new `ensureBoardWorktree(weftPath, hostBranch, hubgeometry.BoardDir(hubPath))` (from Card 6's new `boardweft.go`), teardown-and-return on error exactly like every other phase. Change the final success return from `return hubPath, board, nil` to `return hubPath, nil`. Delete the `deriveBoardURL` function (lines ~264-276) entirely — no caller remains once step 7's board-URL resolution is removed. Rewrite `CloneHub`'s doc comment: it orchestrates host+weft cloning, then materializes `<Hub>/_board` as a second weft worktree on the weft primary's unsuffixed default branch; the phase list becomes: (1) derive host name, (2) compute Hub path, (3) check Hub doesn't exist, (4) create Hub dir, (5) clone host, (6) clone weft + 6b suffix the weft primary's branch (capturing `hostBranch`), (7) materialize `_board` as a second weft worktree via `ensureBoardWorktree` (adopted onto `hostBranch` if it already exists locally from step 6's clone, freshly orphan-created otherwise), (8) return the Hub path and nil error. State that any clone OR worktree-add failure triggers `teardownHub`.
- **Commit:** `feat(fabricengine): CloneHub drops board-clone, materializes _board as a weft worktree`

### Card 6: new file boardweft.go — `_board` worktree-add (adopt-or-orphan)

- **Context:**
  - `internal/fabricengine/weftwiring.go`
  - `internal/fabricengine/clone.go`
- **Edits:** none
- **Creates:**
  - `internal/fabricengine/boardweft.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** New file `boardweft.go`, package `fabricengine`, importing `fmt` and `internal/gitexec`. Add `ensureBoardWorktree(weftRepoRoot, hostBranch, boardPath string) error`: (1) check whether `hostBranch` already exists as a local branch in the weft repo via `gitexec.RunGit([]string{"rev-parse", "--verify", "--quiet", "refs/heads/" + hostBranch}, weftRepoRoot)` (mirrors `weftwiring.go`'s `weftBranchExists` check shape, but inlined rather than calling `weftBranchExists` directly — that function takes a `*hubgeometry.Layout`, which `CloneHub` does not have at this point in bootstrap, since no worktree/config exists yet to resolve one from); (2) if the exit code is 0 (branch exists — the ordinary case: `cloneRepo`'s plain `git clone` already created and checked out a local `hostBranch` ref, and `suffixWeftPrimaryBranch`'s subsequent `checkout -b <hostBranch>-weft` leaves that ref intact), run `git worktree add <boardPath> <hostBranch>` in `weftRepoRoot`; (3) otherwise (a genuinely empty weft remote, where clone left no local `hostBranch` ref at all), run `git worktree add --orphan <hostBranch> <boardPath>` in `weftRepoRoot` (git ≥2.42 syntax — branch name before path). Both worktree-add invocations wrap non-zero exit / spawn error into a descriptive `fmt.Errorf`. Add a file-level doc comment stating: this file materializes `<Hub>/_board` as a second worktree of the weft repo on the host's unsuffixed default branch (never the `WeftBranchName`-suffixed pairing every other weft worktree uses); it never derives a branch name itself (`hostBranch` always arrives pre-computed from `suffixWeftPrimaryBranch`), mirroring `weftwiring.go`'s own stated rule for pre-suffixed branch names — `_board`'s deliberately-unsuffixed branch is exactly the case that rule exists to keep out of that file. Note in the function's own doc comment that `boardPath` must always be supplied by the caller via `hubgeometry.BoardDir(hub)` (per the Hub Geometry Invariant) — this file never constructs the `"_board"` literal itself.
- **Commit:** `feat(fabricengine): add ensureBoardWorktree for _board's adopt-or-orphan worktree-add`

### Card 7: `lyx fabric clone` becomes 2-arg

- **Context:**
  - `internal/fabricengine/clone.go`
- **Edits:**
  - `internal/fabriccli/clone.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `runCloneWithReset`, delete the `var boardURL string; if len(args) >= 3 { boardURL = args[2] }` block. Change the arg-count guard from `if len(args) < 2` to `if len(args) != 2`, and update its usage message from `"usage: lyx fabric clone [--reset] <host-url> <weft-url> [board-url]"` to `"usage: lyx fabric clone [--reset] <host-url> <weft-url>"`. Change the `CloneHub` call from `hubPath, resolvedBoard, err := fabricengine.CloneHub(cwd, hostURL, weftURL, boardURL)` to `hubPath, err := fabricengine.CloneHub(cwd, hostURL, weftURL)`. Remove the `"board": resolvedBoard` entry from the returned `output.Ok` map — the success envelope now returns only `{"hub": hubPath}`.
- **Commit:** `feat(fabriccli): fabric clone drops the board-url argument`

### Card 8: fabric.go clone command help text

- **Context:**
  - `internal/fabriccli/clone.go`
- **Edits:**
  - `internal/fabriccli/fabric.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In the `cloneCmd` cobra command definition, change `Use` from `"clone [--reset] <host-url> <weft-url> [board-url]"` to `"clone [--reset] <host-url> <weft-url>"`. Rewrite `Short` (currently `"bootstrap a new hub (host prime + board passenger + weft prime)"`) to drop the "board passenger" phrase, e.g. `"bootstrap a new hub (host prime + weft prime, with _board as a second weft worktree)"`. Rewrite `Long`: delete the `_board                 — board passenger (task-tracker wiki)` bullet from the repository list (now only host prime and weft prime are cloned); delete the `"The board URL defaults to <weft-url>.wiki.git when omitted."` sentence entirely; add a sentence stating that `_board` is materialized as a second worktree of the weft repo, on the host's unsuffixed default branch (adopted if the weft remote already carries board history, freshly orphan-created otherwise) — immediately after the existing paragraph about the weft prime's own suffixed-branch adoption-or-creation, since both follow the identical adopt-or-create shape. Update the trailing `Example:` block's command line if it lists 3 args (it currently lists only 2: `lyx fabric clone https://github.com/user/repo https://github.com/user/repo-weft` — confirm this line needs no change).
- **Commit:** `docs(fabriccli): update fabric clone help text for _board's new provenance`

### Card 9: update CloneHub tests for the 2-arg signature and `_board`-as-worktree

- **Context:**
  - `internal/hubgeometry/hubgeometry.go`
  - `internal/fabricengine/boardweft.go`
- **Edits:**
  - `internal/fabricengine/clone_adopt_test.go`
  - `internal/fabricengine/clone_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `clone_test.go`, delete `TestDeriveBoardURL` entirely (tests the now-deleted `deriveBoardURL` function) — this also removes the file's last use of any board-URL concept; leave `TestDeriveHostName` and `TestCloneRepo_InvalidURLFails` untouched. In `clone_adopt_test.go`, every `CloneHub` call site currently destructures 3 return values (`hubPath, resolvedBoard, err :=` or `_, _, err :=`); `CloneHub` now returns only 2 (`hubPath, err`), so every call site's LEFT-hand destructuring must also shrink by one variable, not just the argument list — literal "drop the trailing argument" alone leaves a 3-variables-but-2-values compile error. (1) In `TestCloneHub_AdoptsExistingRemoteWeftPrimaryBranch`, delete the `boardBare := makeBareRemote(t, fixtures, "adopt-board")` line; change `hubPath, _, err := fabricengine.CloneHub(cloneParent, filepath.ToSlash(hostBare), filepath.ToSlash(weftBare), filepath.ToSlash(boardBare))` to `hubPath, err := fabricengine.CloneHub(cloneParent, filepath.ToSlash(hostBare), filepath.ToSlash(weftBare))` (2 call args, 2 destructured return values); add new assertions after the existing upstream check: resolve `_board`'s path via `hubgeometry.BoardDir(hubPath)`, run `git -C <_board path> rev-parse --git-common-dir` and `git -C weftPrime rev-parse --git-common-dir`, resolve both outputs to absolute paths (`filepath.Abs`, cleaned), and assert they are equal — proving `_board` is a linked worktree of the same weft repo, not a separate clone; also assert `git -C <_board path> rev-parse --abbrev-ref HEAD` equals `"main"` (the unsuffixed host branch, adopted from the local ref `cloneRepo` already created). (2) In `TestCloneHub_CreatesFreshWeftPrimaryBranch`, delete the `boardBare := makeBareRemote(t, fixtures, "fresh-board")` line; change `hubPath, _, err := fabricengine.CloneHub(cloneParent, filepath.ToSlash(hostBare), filepath.ToSlash(weftBare), filepath.ToSlash(boardBare))` to `hubPath, err := fabricengine.CloneHub(cloneParent, filepath.ToSlash(hostBare), filepath.ToSlash(weftBare))` (same 2-arg/2-return-value shrink as (1)); delete the existing `if _, err := os.Stat(filepath.Join(hubgeometry.BoardDir(hubPath), ".git")); err != nil { t.Fatalf("board clone missing .git: %v", err) }` block (a `.git` file existing is no longer a meaningful assertion — a linked worktree also has a `.git` file) and replace it with the same git-common-dir-equality and `HEAD == "main"` assertions as (1). (3) In `TestCloneHub_StrictAbortRemovesHubOnFailure`, change `_, _, err := fabricengine.CloneHub(cloneParent, filepath.ToSlash(hostBare), filepath.ToSlash(nonExistentWeft), "")` to `_, err := fabricengine.CloneHub(cloneParent, filepath.ToSlash(hostBare), filepath.ToSlash(nonExistentWeft))` (drop both the trailing `""` argument AND the third destructured `_`). (4) Add a new test `TestCloneHub_BoardWorktreeOrphanBranchOnEmptyWeftRemote`: add a new fixture helper `makeEmptyBareRemote(t *testing.T, dir, name string) string` in this file that runs only `git init --bare -b main <dir>/<name>.git` (no seed-and-push — a genuinely empty remote, unlike `makeBareRemote`); the new test clones a hub using `makeBareRemote` for the host (needs a real commit for `suffixWeftPrimaryBranch` to read a checked-out branch) and `makeEmptyBareRemote` for the weft remote; call `CloneHub`; assert the weft prime ends up on branch `main-weft` (via `currentBranch`) with no commits (`git -C weftPrime log` exits non-zero or returns empty — an unborn HEAD); assert `_board` (via `hubgeometry.BoardDir(hubPath)`) exists as a worktree sharing the weft prime's git-common-dir (same assertion helper as (1)/(2)), is checked out on branch `main`, and also has no commits (`git -C <_board path> log` exits non-zero or returns empty) — proving the orphan branch shares no history with `main-weft`.
- **Commit:** `test(fabricengine): cover _board worktree-add adopt and orphan paths, update CloneHub call sites for 2-arg signature`

## Batch Tests

`go test ./internal/fabricengine/... ./internal/fabriccli/...` (both tagged and untagged). Card 9's updated `clone_adopt_test.go` (integration-tagged, real git) is the primary coverage for Cards 4-6: it exercises `CloneHub` end-to-end across all three post-clone branch states `_mill/discussion.md` names — local `hostBranch` already present (adopt), and a genuinely empty remote (orphan) — plus the pre-existing strict-abort-teardown path. `internal/fabriccli/cli_test.go` already exists and covers other `fabric` verbs; its `TestRunCLI_NoArgs` asserts `"clone"` is among the listed subcommand names, which is unaffected by Cards 7-8's argument-count change (the subcommand name itself does not change) — no test in that file drives `runCloneWithReset`/`CloneHub` end-to-end, so Cards 7-8's changes are exercised indirectly via `internal/fabricengine`'s own `CloneHub` tests (the CLI layer is a thin argument-parsing wrapper with no independent logic beyond what `go build`/`go vet` already catch); no new `internal/fabriccli` test file is added by this batch.
