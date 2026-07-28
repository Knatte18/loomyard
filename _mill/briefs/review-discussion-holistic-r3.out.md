MILL_REVIEW_BEGIN
# Review: native clients: migrate gitrepo to go-git + selfreport gh-CLI to go-github

```yaml
verdict: GAPS_FOUND
reviewer_model: opushigh
reviewed_file: _mill/discussion.md
date: 2026-07-28
```

## Findings

### [GAP] Lifted parity harness has no CLI oracle left
**Section:** Testing → "differential parity (the TDD candidate)"; `delete-gitnativepoc-lift-harness`
**Issue:** The harness's reference side *is* `gitrepo`'s CLI implementation — `read_test.go:57,100,264` call `gitrepo.New(dir).CurrentSHA()/ChangedFilesSince()/SnapshotSHA()` as the oracle — so once those methods become go-git, the lifted harness compares go-git against go-git and asserts nothing.
**Fix:** State that the lift includes writing a test-only CLI-oracle layer (raw `gitexec.RunGit` plus the `-z` NUL-split and `--verify --quiet` exit-code parsing production is deleting), and note that `remoteName`/`hasUnpushed`/`isStrictDescendant` parity must live in an internal `package gitrepo` test file, since every git-spawning test file today is `package gitrepo_test`.

### [GAP] Three migrated methods cannot carry the lazy-open error
**Section:** `lazy-cached-repo-handle`; `linked-worktree-and-interop-evidence` (worktreeConfig risk)
**Issue:** "The open error … returned by whichever method needed it" is unimplementable for `remoteName` (returns bare `string`, snapshot.go:60), `isStrictDescendant` (bare `bool`, snapshot.go:252) and `SHAExists` (bare `bool`) — a failed open silently degrades to `"origin"`/`false`, so the `extensions.worktreeConfig` refusal cannot "surface as a clear typed error" on the path that matters: `SetSnapshotSHA` (snapshot.go:175,197) calls both and would run its adopt-loop on fabricated values, returning nil.
**Fix:** Name the per-method fallback for each error-less migrated method and state which method is guaranteed to surface an open failure loudly, or give the handle accessor a caller-visible error path for `SetSnapshotSHA`.

### [GAP] Two decisions each own the Authorization header
**Section:** `go-github-version-and-auth-construction` vs. `token-resolution-and-cache`
**Issue:** `github.NewClient(nil).WithAuthToken(token)` bakes a fixed token into go-github's own transport wrapper, while the 401 rule requires a `githubclient` RoundTripper that "adds the `Authorization` header … re-resolves once, then replays"; with the token captured in `WithAuthToken`'s closure the replay re-sends the stale token unless the layering (custom RT strictly *inner*, overwriting the header) is specified.
**Fix:** Pick one owner of the header and state the transport nesting order explicitly, including that the replay must rewind the request body via `req.GetBody` (issue-create is a POST with a JSON body).

### [GAP] The new gh guard's own file will fail the tier-purity guard
**Section:** `constraints-github-auth-invariant` (guard specification)
**Issue:** The guard must carry `exec.Command`/`exec.CommandContext` + `"gh"` literals as its own scan data; `cmd/lyx/tierpurity_test.go:37-41` bans `exec.Command` as a raw substring in every untagged `*_test.go`, so the new guard breaks `go test` unless added to `allowedSpawners` — exactly the entry `tools/sandbox/pathresolve_guard_test.go` already holds (line 28). The discussion allowlists the guard's own file only against *its own* scan.
**Fix:** Name the guard file's home package and require an `allowedSpawners` entry with a reason in the same commit.

### [GAP] Stale `gh` and gitnativepoc docs outside selfreportcli
**Section:** `docs-lifecycle`; Scope (in)
**Issue:** `docs-lifecycle` claims `docs/overview.md` "most likely" needs no change, but its tree lists library packages, so `internal/gitnativepoc/` (overview.md:141) goes dangling and `internal/githubclient/` is missing; overview.md:168 still says selfreport files issues "via the `gh` CLI"; `README.md:65` repeats it and `README.md:109` lists "`gh` CLI authenticated (`gh auth login`)" as a hard requirement, which this task demotes to a fallback.
**Fix:** Add overview.md and README.md to the doc list; also extend the four cli.go sites to cover cli.go:32-33 (`gh CLI +` in `Command`'s godoc) and cli.go:96,128-129, which name `buildCreateArgs` and "the gh output URL's trailing path segment" — both deleted by this task.

### [NOTE] `hasUnpushed`'s failure-swallowing contract has no parity case
**Section:** Testing → three added parity cases
**Issue:** CLI `hasUnpushed` returns `(true, nil)` on *any* non-zero exit (push.go:161-165), while the poc returns `(false, err)` when `Head()`/`CommitObject` fails (read.go:300-311) — an unreadable or unborn-HEAD repo turns `PushCoalesced` (boardengine/sync.go:68, weftgit.go:313,332) from "attempt the push" into a hard error.
**Fix:** Add a parity case for the swallow-into-true failure path alongside the no-upstream and never-fetched cases.

### [NOTE] Two small factual slips in otherwise-verified claims
**Section:** Scope (in); `mixed-backend-call-sites`
**Issue:** The `pathresolve_guard_test.go` change is called "a one-line fix", but that guard matches whole-file substrings (`strings.Contains(content, token)`, line 68), so the mandated line-based match is a restructure; and `SnapshotSHA` calls `remoteName()` at snapshot.go:94, *before* its fetch at line 98, not after.
**Fix:** Reword both so the plan writer sizes the guard change correctly and the "exhaustive" interop list is accurate.

## Verdict

GAPS_FOUND
Parity oracle, error-less open path, auth-header ownership, guard collision, and doc scope unresolved.
MILL_REVIEW_END
