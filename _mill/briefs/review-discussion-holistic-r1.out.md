MILL_REVIEW_BEGIN
# Review: native clients: migrate gitrepo to go-git + selfreport gh-CLI to go-github

```yaml
verdict: GAPS_FOUND
reviewer_model: opushigh
reviewed_file: _mill/discussion.md
date: 2026-07-28
```

## Findings

### [GAP] Linked-worktree topology dismissed on a false premise
**Section:** Technical context → Probe artifacts
**Issue:** The discussion waves off the probe's third unknown ("junctions and `.git`-as-a-file worktrees, not exercised") with "every operation they concern stays on the CLI" — but that is the *only* topology the migrated read surface runs in: this very checkout's `.git` is the file `gitdir: C:/Code/loomyard/wts/loomyard/.git/worktrees/native-clients-migration`, `fabricengine/add.go:198,220` creates both host and weft as linked worktrees, and `refs/loomyard/snapshot/*` lives in the common dir while HEAD is per-worktree, so `SnapshotSHA`/`CurrentSHA`/`remoteName` all depend on go-git's untested `commondir` handling.
**Fix:** State the linked-worktree/junction case as in-scope evidence and add a parity fixture built with `git worktree add` (today `gitrepo_test.go:26` only does `git init` in `t.TempDir()`, so the lifted harness would never exercise it).

### [GAP] CLI-bound methods silently become mixed-backend
**Section:** Decisions → gitexec-seam-unchanged / gogit-boundary-local-vs-remote
**Issue:** "The CLI-bound methods are not edited at all" is true textually but false behaviourally: `StageAndCommit` and `StageAllAndCommit` end by calling `r.CurrentSHA()` (`gitrepo.go:159,211`), `SetSnapshotSHA` calls `r.remoteName()` and `r.isStrictDescendant()` (`snapshot.go:175,197`), `SnapshotSHA` calls `r.remoteName()`, and `PushCoalesced` gates on `r.hasUnpushed()` (`push.go:141`) — all five callees are in the migrate list, so every one becomes a go-git read-after-CLI-write on a handle cached before that write.
**Fix:** Name this interleaving explicitly and state the required guarantee (a cached `*git.Repository` sees refs/objects the CLI wrote in the same process afterwards, including after a repack from `pull --rebase`), with a parity case per site rather than only the `SetSnapshotSHA`→`SnapshotSHA` ref test already listed.

### [GAP] Cached handle has no invalidation or lifetime story
**Section:** Decisions → lazy-cached-repo-handle
**Issue:** `sync.Once` pins the open error permanently, which contradicts `New`'s "does not care whether the checkout exists yet" contract for a `Repo` outliving a topology change — `fabricengine` holds `Fabric.Warp`/`Weft` handles (`fabric.go:66-67`) while `remove.go`/`weftwiring.go`/`prune.go` run `worktree remove`/`add` at those same paths; on Windows an open packfile handle can also block that removal.
**Fix:** Decide and record handle lifetime — never invalidated (and why that is safe given fabric's remove/add), or re-openable — and whether go-git handles are closed before any topology mutation.

### [GAP] selfreport help text names gh as transport and prerequisite
**Section:** Constraints → CLI / Cobra Invariant
**Issue:** The claim that `selfreportcli`'s `Short`/`Long` are unchanged is wrong: `cli.go:38` ("…via gh"), `cli.go:48` ("…via gh"), `cli.go:49-50` ("via the gh CLI. The gh CLI must be installed and authenticated before running this command."), and the package doc at `cli.go:9-10` all describe the transport being deleted — stale help is a review-blocking defect under the help-accuracy obligation.
**Fix:** Move `selfreportcli`'s `Short`/`Long`/package doc into scope and state the new prerequisite wording (`GH_TOKEN`/`GITHUB_TOKEN` or a `gh auth` login, not a `gh` binary at call time).

### [GAP] No named replacement for the deleted RunGH seam
**Section:** Decisions → httptest-seam-replaces-rungh
**Issue:** `selfreportengine` has zero test files of its own; all coverage lives in `selfreportcli/cli_test.go`, which drives the *exported* `selfreportengine.RunGH` (`cli_test.go:26-31`) — deleting it while declaring `CreateIssue`'s signature unchanged and the CLI unchanged leaves no stated exported injection point for the `httptest` base URL, and no statement that the tests bypass token resolution rather than shelling out to `gh auth token` or reading the operator's real cache.
**Fix:** Name the replacement exported seam and its owner (`selfreportengine` vs `githubclient`), and state that it injects the whole authenticated client so no test resolves a real token.

### [GAP] GitHub Auth grep guard is unimplementable as specified
**Section:** Decisions → constraints-github-auth-invariant
**Issue:** "a grep guard on the `gh` literal in non-test files" cannot work as a raw substring the way its cited precedent does — `pathresolve_guard_test.go:27-31` bans specific spellings (`lookPath("lyx")`, `exec.Command("lyx"`) inside one directory, whereas a bare `gh` matches "through"/"right"/"highlight" repo-wide; the scan root and `githubclient`'s own allowlist entry are also unstated.
**Fix:** Pin the banned token list (`exec.Command("gh"`, `exec.CommandContext("gh"`, `LookPath("gh")`), the scan root (module-root walk vs single package), and the allowlisted file, plus vacuous-scan protection.

### [NOTE] SHAExists loses the `^{commit}` peel
**Section:** Testing → differential parity
**Issue:** `gitrepo.go:229` runs `rev-parse --verify --quiet <sha>^{commit}`, so a tree/blob SHA is false; the poc uses bare `ResolveRevision` (`gitnativepoc/read.go:107`), and the listed parity cases cover only real/absent/non-hex — no non-commit-object case.
**Fix:** Add a tree-or-blob-SHA parity case, or state that the peel divergence is accepted.

### [NOTE] hasUnpushed's no-upstream contract is untested
**Section:** Testing → differential parity
**Issue:** `push.go:156-166` documents "no upstream configured ⇒ true" as load-bearing for the first push, but the enumerated cases name only the strictly-behind case.
**Fix:** Add no-upstream-configured and configured-but-never-fetched cases to the parity list.

### [NOTE] ChangedFilesSince ordering left unstated
**Section:** Testing → differential parity
**Issue:** The harness being lifted sorts both sides before comparing (`harness_test.go:187-190`), so an order change between `--name-only` and `object.DiffTree` passes parity unnoticed.
**Fix:** State whether output order is contract; if not, say so in the godoc the task is already rewriting.

## Verdict

GAPS_FOUND
Production worktree topology, mixed-backend call sites, stale gh help, and the guard spec need resolving.
MILL_REVIEW_END
