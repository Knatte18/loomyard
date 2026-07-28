# Batch: guards

```yaml
task: 'native clients: migrate gitrepo to go-git + selfreport gh-CLI to go-github'
batch: guards
number: 8
cards: 4
verify: go test -count=1 ./cmd/lyx/... ./tools/sandbox/... ./internal/gitrepo/...
depends-on: [5, 7]
```

## Batch Scope

Ships the machine enforcement for both new invariants, plus a fix to the guard one of them is modelled on. The `CONSTRAINTS.md` prose lands in batch 9; the tests land here, because a rule with no mechanism decays into prose and every other entry in that file names its enforcement.

Two guards are new. The **gh guard** asserts no production package outside `internal/githubclient` shells out to `gh` — without it there is nothing stopping the coming finalize module from growing its own credential path in six weeks, at which point there are two of them and only one has a timeout. The **gitrepo boundary guard** pins the set of methods that may contain an `r.run(` call, because the go-git/CLI boundary is invisible in the code — both backends are just method bodies, and without a check CLI calls seep back one bugfix at a time.

The batch depends on batch 5 (the pinned method list must match the post-migration reality, and batch 5's `hasUnpushed` measurement can still change that list) and batch 7 (the gh guard must pass, which requires `selfreportengine`'s shell-out to be gone).

Batch-local decision: both guards live in `cmd/lyx/`, alongside the repo's other cross-cutting guards, and both resolve their scan root via `go env GOMOD` so they are cwd-independent.

## Cards

### Card 36: Fix the pathresolve guard's unmatchable token

- **Context:**
  - `tools/sandbox/resolve.go`
  - `CONSTRAINTS.md`
- **Edits:**
  - `tools/sandbox/pathresolve_guard_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Fix a latent hole this task discovered in the guard the new gh guard is modelled on. Its banned-token list carries `exec.CommandContext("lyx"`, but `exec.CommandContext` takes the context **first**, so that string never appears in compilable Go — the token can never match, while the guard's own doc and the Dev/Prod Binary Separation entry both claim the `CommandContext` form is covered. Replace the unmatchable literal with a **line-based** match: a line containing `exec.Command` or `exec.CommandContext` together with the token `"lyx"`, keeping the `lookPath("lyx")` literal as-is. **Size this as a small restructure, not a token-list edit**: the guard currently matches whole-file substrings via `strings.Contains(content, token)`, so moving to a line-based match means changing how the scan iterates, not just what it looks for. Preserve its existing scan scope, its `resolve.go` self-exclusion, and its vacuous-scan floor. Leaving a knowingly-broken guard in place is the exact debt this repo's guard discipline exists to prevent, and the new guard would otherwise inherit the same hole by construction.
- **Commit:** `fix(sandbox): make pathresolve guard match the CommandContext form`

### Card 37: gh shell-out guard

- **Context:**
  - `tools/sandbox/pathresolve_guard_test.go`
  - `cmd/lyx/tierpurity_test.go`
  - `internal/githubclient/token.go`
- **Edits:** none
- **Creates:**
  - `cmd/lyx/ghguard_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Add a guard asserting that no production package outside `internal/githubclient` shells out to `gh`. A bare `gh` substring is unusable as a banned token — it matches "through", "right", "highlight" and hundreds of other words repo-wide — so follow the pathresolve precedent and ban **specific spellings**. Match on a line containing `exec.Command` or `exec.CommandContext` together with the token `"gh"`, plus the literal `LookPath("gh")`. The line-based form is not optional here: `exec.CommandContext("gh"` never appears in compilable Go, and the context-bounded shell-out is exactly the call shape this task itself introduces, so a naive literal would miss the one form that matters. Both spellings include the quoted binary name, so no English word can match. **Scan root:** a module-root walk resolved via `go env GOMOD`, cwd-independent, as the tier-purity and hermetic-env guards already do, over **non-test `.go` files only**. **Allowlist:** `internal/githubclient/` — the one package permitted to shell out. The guard's own file needs **no** self-exclusion entry: the scan covers non-test files only, so a `*_test.go` guard is already outside it by the same filter, exactly as the pathresolve guard's own comment states. The `cmd/lyx/hermeticenv_test.go` analogy does not transfer, since that guard deliberately scans test files. **Vacuous-scan protection:** fail if fewer than 20 files were scanned — this is a module-root walk, so it takes the same floor as its structural twin `tierpurity_test.go`, not the `< 3` floor the single-directory pathresolve guard uses. A mis-resolved root that still finds a handful of files must not pass. **Skip-dir set:** reuse the directories `tierpurity_test.go`'s `tierPuritySkipDirs` already excludes — `.git`, `_lyx`, `_mill`, `.scratch`, `.wiki`, `_raddle`. Without it the walk descends into gitignored trees, including this task's own `.scratch/gogitprobe*/` probe modules, which would make a local `go test` result depend on whichever untracked files happen to be lying around.
- **Commit:** `test(cmd/lyx): add guard banning gh shell-outs outside githubclient`

### Card 38: Allowlist the gh guard in tierpurity

- **Context:**
  - `cmd/lyx/ghguard_test.go`
  - `tools/sandbox/pathresolve_guard_test.go`
- **Edits:**
  - `cmd/lyx/tierpurity_test.go`
  - `internal/githubclient/githubclient_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Add `cmd/lyx/ghguard_test.go` to `tierpurity_test.go`'s `allowedSpawners` map with a one-line reason. This is required, not defensive: the Test Tier Purity Invariant bans `exec.Command` as a **raw substring** in every untagged test file, and the new guard necessarily carries that literal as its own scan data. Without the entry the guard breaks `go test` on the untagged tier the moment it is added. `tools/sandbox/pathresolve_guard_test.go` already holds exactly such an entry for exactly this reason, and `tierpurity_test.go` itself holds one for its own test data. This is the **only** allowlist entry the guard file needs — its self-exclusion from its own scan is already handled by the non-test-file filter.
- **Commit:** `test(cmd/lyx): allowlist the gh guard in tier-purity`

### Card 39: gitrepo client-boundary guard

- **Context:**
  - `cmd/lyx/longlist_test.go`
  - `internal/gitrepo/gitrepo.go`
  - `internal/gitrepo/snapshot.go`
  - `internal/gitrepo/push.go`
  - `internal/gitrepo/pull.go`
  - `internal/gitrepo/reset.go`
- **Edits:** none
- **Creates:**
  - `cmd/lyx/gitrepoboundary_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Add a **pinned-set** guard in the style of `cmd/lyx/longlist_test.go`. It walks `internal/gitrepo`'s non-test `.go` files, collects every method containing an `r.run(` call, and asserts the resulting set equals a **literal pinned list**. That list is *not* "the CLI-bound methods" — the two sets differ in both directions after this task, and conflating them ships a guard that fails on day one. Post-migration the methods containing `r.run(` are exactly: `StageAndCommit`, `StageAllAndCommit`, `CheckoutDetached`, `RestoreBranch`, `Pull`, `ResetHard`, `pushWithRebaseRetry`, `SnapshotSHA`, `advanceAndPushSnapshotRef`, and `adoptSnapshotRef` — plus `hasUnpushed` **only if** batch 5's measurement reverted it, in which case that batch already mandated adding it here. Both directions of the mismatch are deliberate: `SnapshotSHA` is a *migrating* method that still appears because it keeps its CLI fetch, while `Push`, `PushCoalesced` and `SetSnapshotSHA` are CLI-bound *by contract* yet do not appear, because they delegate to `pushWithRebaseRetry`/`advanceAndPushSnapshotRef` or lose both their own `r.run` calls to go-git. Add a **second assertion** closing the guard's other blind spot: a new `gitexec.RunGit` call written directly inside a migrated method would satisfy an `r.run(`-keyed check while violating the rule. It must operate on **code only, with comments stripped** — `gitexec.` occurs five times in the package's non-test files today and four are prose (`doc.go` twice, `gitrepo.go`'s file header, and `run`'s own doc comment), and batch 9 rewrites `doc.go` while still naming `gitexec.RunGit`, so a naive token match fails on day one. The assertion: after stripping line and block comments, the token `gitexec.` appears **exactly once** in `internal/gitrepo`'s non-test source, inside `run`'s body. Record the one remaining hole in the test's own doc comment rather than papering over it: set-equality on method names cannot see a *new* `r.run` call added inside a method already on the list, and `SnapshotSHA` is precisely such a method, hosting a migrated read and a CLI fetch side by side.
- **Commit:** `test(cmd/lyx): pin gitrepo's CLI-bound call sites`

## Batch Tests

`verify:` runs `go test -count=1 ./cmd/lyx/... ./tools/sandbox/... ./internal/gitrepo/...`. The scope covers the two directories this batch writes to plus `internal/gitrepo`, because card 39's guard asserts against that package's source and a mismatch there is the failure most worth catching immediately. No build tag and no `-race`: all three guards are untagged Tier 1 file-walkers that spawn nothing.

Each guard must be checked for the failure it is supposed to produce, not merely for passing. Confirm the gh guard fails when a `gh` shell-out is temporarily added to a production file outside `internal/githubclient`, and passes with the real `internal/githubclient` shell-out in place. Confirm the boundary guard fails when an `r.run(` call is temporarily added to a migrated method and when a bare `gitexec.RunGit` call is added outside `run`, and that it still passes with `gitexec.RunGit` named freely in comments. Confirm the fixed pathresolve guard now matches the `exec.CommandContext` form it previously could not — that is the whole point of card 36, and a fix that leaves the token unmatchable is indistinguishable from no fix.
