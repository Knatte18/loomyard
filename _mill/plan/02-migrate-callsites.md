# Batch: migrate-callsites

```yaml
task: "Extract internal/fslink cross-OS link primitive"
batch: "migrate-callsites"
number: 2
cards: 6
verify: go test -tags integration ./internal/fslink/... ./internal/worktree/... ./internal/weft/... ./internal/paths/...
depends-on: [1]
```

## Batch Scope

Migrates every hand-rolled junction/symlink call site to the `internal/fslink` API
created in batch 1, then deletes the now-dead worktree link files and updates the weft
status test. This is a single behaviour-preserving pass over all six call sites
(`createPortal`, `removePortal`, `seedLyxJunction`, `removeHostJunction`, the
`removeLinks` sweep in `remove.go`, and `checkJunction`), so no half-migration remains.
After this batch, `internal/worktree` contains zero build-tagged link files. Caller-visible
error/reason strings and the `remove.go`/`add.go` removal ordering are preserved
verbatim (see Shared Decision `preserve-behaviour-and-messages`). Batch-local note: the
surviving worktree/weft tests keep `//go:build integration`; the migration's correctness
is verified by running them with `-tags integration`.

## Cards

### Card 6: migrate portals.go

- **Context:**
  - `internal/fslink/fslink.go`
- **Edits:**
  - `internal/worktree/portals.go`
- **Creates:** none
- **Deletes:** none
- **Requirements:** In `createPortal`, replace `createJunction(link, target)` with
  `fslink.Create(link, target)`. In `removePortal`, replace the `os.Remove(link)` call
  and its `os.IsNotExist` branch with `fslink.Remove(link)` (idempotent): on a non-nil
  error return `fmt.Errorf("remove portal %s: %w", link, err)`; otherwise call
  `pruneEmptyAncestors(filepath.Dir(link), l.PortalsDir())` and return nil — preserving
  today's behaviour that empty ancestors are pruned whether or not the link existed.
  Add the `internal/fslink` import; drop `os` if it becomes unused (keep `filepath` for
  `pruneEmptyAncestors`). Update the doc comment that says `createPortal` "Delegates to
  createJunction" to reference `fslink.Create`.
- **Commit:** `refactor(worktree): migrate portals.go to fslink`

### Card 7: migrate weft.go

- **Context:**
  - `internal/fslink/fslink.go`
- **Edits:**
  - `internal/worktree/weft.go`
- **Creates:** none
- **Deletes:** none
- **Requirements:** Rewrite `seedLyxJunction` to use `fslink`, preserving every
  caller-visible message and the idempotent semantics:
  When `os.Lstat(link)` succeeds (link exists), preserve the original check ORDER:
  (1) resolve the expected target first — if `filepath.EvalSymlinks(target)` errors
  (target missing), return `"weft _lyx directory does not exist at %s; cannot validate
  junction target"`. (2) Otherwise, if the link is a link (`fslink.IsLink(link)` true)
  AND `fslink.PointsTo(link)` equals the resolved target, return nil (idempotent).
  (3) Otherwise — covering both the not-a-link case and the points-elsewhere case —
  return `"host repo already contains a real _lyx at %s; it predates weft — migrate via
  the hub-creator"`. This ordering keeps the edge-case message (real dir + absent
  target) byte-identical to the original per the `preserve-behaviour-and-messages`
  decision.
  When `os.Lstat(link)` returns `os.IsNotExist`: call `fslink.Create(link, target)` and
  return its result.
  Other `os.Lstat` error: wrap as `"lstat %s: %w"` (unchanged).
  In `removeHostJunction`, replace `os.Remove(link)` and its `os.IsNotExist` branch with
  `fslink.Remove(link)`; on a non-nil error return `fmt.Errorf("remove host junction
  %s: %w", link, err)`, else nil. Add the `internal/fslink` import; keep `os`/`filepath`
  (still used by `seedGitExclude`, `seedLyxJunction`'s Lstat, etc.).
- **Commit:** `refactor(worktree): migrate weft.go junction logic to fslink`

### Card 8: migrate remove.go sweep

- **Context:**
  - `internal/fslink/fslink.go`
- **Edits:**
  - `internal/worktree/remove.go`
- **Creates:** none
- **Deletes:** none
- **Requirements:** In `Worktree.Remove`, replace `removeLinks(target)` (step 6, the
  root-level link safety net) with `fslink.RemoveLinksIn(target)`, keeping the
  `linksRemoved, err :=` assignment and error handling unchanged. Do NOT reorder steps
  — `removeHostJunction` still runs first (step 5) before the sweep (step 6). Add the
  `internal/fslink` import. Update the step-5/step-6 doc comments that mention
  `removeLinks` to say `fslink.RemoveLinksIn`.
- **Commit:** `refactor(worktree): use fslink.RemoveLinksIn for link sweep`

### Card 9: migrate weft/status.go checkJunction

- **Context:**
  - `internal/fslink/fslink.go`
- **Edits:**
  - `internal/weft/status.go`
- **Creates:** none
- **Deletes:** none
- **Requirements:** Rewrite `checkJunction(hostLink, weftLyxDir) (bool, string)` to use
  `fslink`, preserving all reason strings. Keep `os.Lstat(hostLink)` to distinguish
  existence: `os.IsNotExist` → `(false, "host _lyx junction missing")`; other lstat
  error → `(false, "lstat error: %v")`. Replace the `info.Mode()&os.ModeSymlink == 0`
  test with `fslink.IsLink(hostLink)`: when it returns false (or errors) →
  `(false, "host _lyx is not a junction")`. Replace `filepath.EvalSymlinks(hostLink)`
  with `fslink.PointsTo(hostLink)`, preserving the `"EvalSymlinks(hostLink) error: %v"`
  reason on failure; keep `filepath.EvalSymlinks(filepath.Clean(weftLyxDir))` with its
  `"EvalSymlinks(weftLyxDir) error: %v"` reason; compare the two cleaned paths and
  return `(false, "host _lyx junction points elsewhere")` on mismatch, `(true, "")` on
  match. Add the `internal/fslink` import; keep `os`/`filepath`.
- **Commit:** `refactor(weft): migrate checkJunction to fslink`

### Card 10: delete dead worktree link files and tests

- **Context:**
  - `internal/worktree/portals.go`
  - `internal/worktree/weft.go`
  - `internal/worktree/remove.go`
- **Edits:** none
- **Creates:** none
- **Deletes:**
  - `internal/worktree/junction_windows.go`
  - `internal/worktree/junction_other.go`
  - `internal/worktree/links.go`
  - `internal/worktree/junction_test.go`
  - `internal/worktree/links_test.go`
- **Requirements:** After cards 6–8 removed every reference to `createJunction` and
  `removeLinks`, delete the now-dead source files `junction_windows.go`,
  `junction_other.go`, and `links.go`, plus their tests `junction_test.go` and
  `links_test.go` (whose coverage moved to `internal/fslink/fslink_test.go` in batch 1).
  Confirm via the Context files that no remaining reference to `createJunction` or
  `removeLinks` exists in package `worktree` before deleting.
- **Commit:** `refactor(worktree): delete hand-rolled junction/symlink files`

### Card 11: update weft/status_test.go

- **Context:**
  - `internal/fslink/fslink.go`
  - `internal/weft/status.go`
- **Edits:**
  - `internal/weft/status_test.go`
- **Creates:** none
- **Deletes:** none
- **Requirements:** Keep the file's `//go:build integration` tag (it uses
  `lyxtest.CopyWeft` git fixtures). In `TestStatus_JunctionOk_Windows`, replace the
  `exec.Command("cmd", "/c", "mklink", "/J", hostLink, weftLyxDir)` junction creation
  with `fslink.Create(hostLink, weftLyxDir)` — the suite must not depend on the removed
  double-spawn. Now that detection goes through `fslink.IsLink`, make the test expect
  success for a real junction: remove the `"Windows junction not recognized by os.Lstat
  (ModeSymlink not set)"` skip branch. Keep a `t.Skip` only for the genuine
  can't-create-link case (non-Windows / no privilege — i.e. when `fslink.Create`
  returns an error), mirroring the probe-then-skip pattern. Add the `internal/fslink`
  import; drop `os/exec` if it becomes unused. Leave the `TestStatus_Junction` table
  (`Missing`/`PlainDir`/`ValidSymlink`) and its reason-string assertions unchanged.
- **Commit:** `test(weft): build status-test junction via fslink, expect success`

## Batch Tests

`verify: go test -tags integration ./internal/fslink/... ./internal/worktree/...
./internal/weft/... ./internal/paths/...`. The `-tags integration` flag is required
here (unlike batch 1) because the surviving tests that prove the migration preserved
behaviour — `portals_test.go`, `remove_test.go`, `weft_test.go`, `status_test.go` — are
integration-tagged for their `lyxtest` git fixtures; without the tag they would not run
and the migration would be unverified. `./internal/worktree/...` exercises
`createPortal`/`removePortal`/`seedLyxJunction`/`removeHostJunction` and the
`Remove`-path sweep; `./internal/weft/...` exercises `checkJunction` via `Status`
including the updated `TestStatus_JunctionOk_Windows`; `./internal/fslink/...` re-runs
the package tests (cheap, catches cross-package breakage); `./internal/paths/...` keeps
the path-invariant enforcement green. The scope is bounded to the four packages this
batch touches rather than `./...`.
