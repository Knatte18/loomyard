MILL_REVIEW_BEGIN
# Review: native clients: migrate gitrepo to go-git + selfreport gh-CLI to go-github — holistic

```yaml
verdict: REQUEST_CHANGES
reviewer_model: sonnetmax
reviewed_file: plan/
date: 2026-07-28
```

## Findings

### [BLOCKING] cache.go mixes a Windows-only import into an untagged file
**Location:** Batch 6 / Card 26
**Issue:** `internal/githubclient/cache.go` is the card's only `Creates:` file (no build tag), yet it must import `golang.org/x/sys/windows` for the ACL-stripping step; that package is itself `//go:build windows`-gated, so `GOOS=linux go build ./...` fails — exactly the gate `cmd/lyx/crosscompile_test.go`'s `TestCrossCompileLinux` runs on every Tier-2 suite (and batch 9's card 46 whole-repo verification) — contradicting the card's own citation of `fslink_windows.go`'s "build-tag shape" as the model to follow.
**Fix:** Split into `cache.go` (cross-platform: dir resolution, schema, atomic rename), `cache_windows.go` (`//go:build windows`, the ACL syscalls), and a `!windows` counterpart, mirroring `internal/fslink`'s existing three-file split (fslink.go/fslink_windows.go/fslink_linux.go) already named in Context.

### [BLOCKING] Leaf-enforcement test's Context omits the files it must scan
**Location:** Batch 6 / Card 29
**Issue:** The test needs the exact, full import-path strings of `cache.go`, `githubclient.go`, `token.go`, and `transport.go` for its allowlist map (exact-string match, per the `modelspec`/`tokenvocab` pattern it explicitly copies) — but none of those four files is in Card 29's Context, only the two reference leaf-tests and `proc_windows.go` are. The card's own prose names abbreviated paths ("golang.org/x/sys", "github.com/google/go-github") that do not literally match what the production code actually imports (`golang.org/x/sys/windows`; go-github's importable package lives at `.../v75/github`, not the bare module path) — an implementer copying the prose verbatim into the allowlist ships a guard that fails on the very code the plan requires.
**Fix:** Add the four `internal/githubclient` production files to Card 29's Context so the implementer reads and copies the real import strings rather than the prose's shorthand.

### [BLOCKING] hasUnpushed CLI-reversal can't touch the files its own instruction requires
**Location:** Batch 5 / Card 21
**Issue:** If the perf measurement reverts `hasUnpushed` to the CLI, Requirements say to delete its parity cases — the five states from Card 8, the linked-worktree run from Card 9, and the mixed-backend/cross-instance case from Card 19 — all of which live in `internal/gitrepo/gogit_test.go` per Card 9's own placement rule ("the unexported ones must be in the internal file"). Card 21's `Edits:` names only `internal/gitrepo/push.go`, and `gogit_test.go` appears in neither its Edits nor its Context, so the card as scoped cannot execute the deletion it mandates — leaving exactly the CLI-vs-CLI "oracle trap" tautology the card itself warns against.
**Fix:** Add `internal/gitrepo/gogit_test.go` to Card 21's Edits, conditioned on the reversal branch actually firing.

### [NIT] "All Files Touched" omits every deleted path
**Location:** 00-overview.md / All Files Touched
**Issue:** The list contains no deletions: the 8 `internal/gitnativepoc/*` files (deleted by Card 22) and `manifest/designs/native-clients-migration.md` (deleted by Card 45) are dropped by the plan but absent from the manifest.
**Fix:** Add the 9 deleted paths to the list, or note explicitly that the list is Creates/Edits-only.

## Verdict

REQUEST_CHANGES
Three cross-batch Context/build-tag gaps in the githubclient cache plus one conditional-branch Edits gap in batch 5.
MILL_REVIEW_END
