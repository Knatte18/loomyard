# Discussion: fabric: audit and migrate all remaining direct git mutations onto Fabric

```yaml
task: 'fabric: audit and migrate all remaining direct git mutations onto Fabric'
slug: webster-bisect-fabric-migrate
status: discussing
parent: main
```

## Problem

CONSTRAINTS.md's Fabric Git Invariant (warp + weft) is unconditional: every git operation LYX/LoomYard's own code performs, on either the weft repo or the warp/host repo, must go through `internal/fabricengine` — no exception for internal code, regardless of purpose. Today that invariant has a documented, tracked gap: `internal/websterengine`'s bisect/verify path (`CheckoutDetached`/`RestoreBranch` in `integration.go`) mutates warp directly, bypassing fabric. This task's own audit (grep across the whole tree for `gitexec.RunGit`, `gitrepo.`, and raw `exec.Command("git", ...)` outside the three mechanism packages) found a second, previously-undocumented instance: `internal/builderengine/gitquery.go`'s `ResetHard` (consumed only by `chain.go`'s `RestartChain`), which runs `git reset --hard <sha>` directly against the host worktree via raw `gitexec.RunGit`, never touching `gitrepo` at all.

The task's own brief assumed the dependency task `fabric-rebase-reconcile` (slice 6, landed as commit `50851542`) already "provides the Fabric API surface for detached checkout + branch restore." It does not: that task's actual deliverable was `Fabric.Pull`'s remote-reconcile auto-detection/re-anchor logic (`internal/fabricengine/pull.go`), which internally calls `f.Warp.ResetHard` but exposes no public verb usable by an arbitrary caller like bisect or chain-restart. This task therefore builds that API surface from scratch — it is not extending an existing one.

## Scope

**In:**

- Four new methods on `*fabricengine.Fabric`: `CheckoutDetached(sha string) error`, `RestoreBranch(ref string) error`, `CurrentBranch() (string, error)`, `ResetHard(sha string) error` — each a thin delegation to the corresponding existing `f.Warp.X()` method (`internal/gitrepo/gitrepo.go:503,520,477`, `internal/gitrepo/reset.go:21`), doc-commented as warp-only, with no "Warp"/"Weft" in the public method name.
- Migrate `internal/websterengine`'s bisect path (`bisect`, `checkoutAndVerify`, `BisectAndEscalate` in `integration.go`, fed by `runlevel.go:845`'s `gitrepo.New(deps.WorktreeRoot)` construction) onto a `*fabricengine.Fabric` handle constructed from the `*hubgeometry.Layout` already on `RunDeps`.
- Migrate `internal/builderengine`'s `ResetHard` (`gitquery.go:76`, consumed by `chain.go`'s `RestartChain`, itself called from `spawn.go:389`) onto the same kind of `*fabricengine.Fabric` handle constructed from `SpawnDeps.Layout`.
- Narrow consumer-side interfaces in `websterengine` and `builderengine` (structurally satisfied by `*fabricengine.Fabric`) so the migrated functions depend on an interface, not the concrete `*Fabric` type, and existing tests can keep using lightweight fakes.
- A new Tier-1 substring-scan guard test (same mechanism as `cmd/lyx/tierpurity_test.go`) banning literal mutating-verb call tokens in `internal/websterengine` and `internal/builderengine` production source, to prevent regression back to raw mutation.
- New Tier-2 (`//go:build integration`) tests in `internal/fabricengine` directly covering the four new methods.
- Final re-grep of the whole tree (the same command used for this task's own audit) to confirm zero mutating git call sites remain outside `internal/fabricengine`, `internal/gitexec`, `internal/gitrepo`.
- CONSTRAINTS.md updates in the same commit: narrow/close the Fabric Git Invariant's "Known gap, tracked" bullet (both now-fixed instances), note the module-ownership half is now machine-checked for these two packages specifically (the general case stays a review obligation, unchanged).

**Out:**

- `internal/fabriccli` — Fabric's own CLI layer; its git calls are part of Fabric itself, not a bypass of it. Untouched (e.g. `fabric.go:398`).
- `internal/hubgeometry`'s `rev-parse --show-toplevel` (`hubgeometry.go:138`) and `worktree list --porcelain` (`worktreelist.go:35`) — read-only, exempt per the invariant's own read-only carve-out. Untouched.
- `internal/websterengine/gitwrap.go`'s `CurrentSHA` (`:27`) and `status --porcelain` (`:40`) — explicitly grandfathered read-only exemptions already named in CONSTRAINTS.md. Untouched.
- `internal/builderengine/gitquery.go`'s `HeadSHA`, `ChangedFiles`, `Dirty` (`rev-parse HEAD`, `diff --name-only`, `status --porcelain`) — read-only. Untouched.
- `internal/lyxtest` — test-fixture git plumbing, not production runtime. Untouched.
- `tools/deploy` — standalone deploy tooling, outside the lyx/loomyard git-operation surface. Untouched.
- No weft-side interaction is added to bisect or chain-restart — both stay warp-only, exact same behavior, only the call path changes.
- No general "route every git verb through Fabric" widening — scope stays limited to the specific verbs this audit actually found in use (`CheckoutDetached`, `RestoreBranch`, `ResetHard`) plus `CurrentBranch` (bisect's paired read against the same handle).
- The new regression guard is scoped to `internal/websterengine` and `internal/builderengine` only — the general "every other fabricengine caller" case CONSTRAINTS.md already flags as an unmachine-checked review obligation stays that way; this task does not attempt a repo-wide guard.

## Decisions

### fabric-api-naming

- Decision: the four new warp-mutating methods are named after the git verb only — `CheckoutDetached`, `RestoreBranch`, `CurrentBranch`, `ResetHard` — with no "Warp"/"Weft" anywhere in the public signature.
- Rationale: `manifest/designs/fabric-unified-view.md` frames Fabric as "the one-repo illusion portal" — no external consumer should need to know the backend is two repos. Verified this holds today: grepped every call site of the `Fabric.Warp`/`Fabric.Weft` exported fields and of the one existing Warp-named method (`SnapshotWarpSHA`) — zero external callers exist outside `internal/fabricengine` itself (including its own test files). The illusion has never been broken at the public API boundary; these new methods must not be the first to do so.
- Rejected: `WarpCurrentBranch`/`CheckoutWarpDetached`/`RestoreWarpBranch`/`ResetWarpHard` — leaks the internal two-repo split into the public API, and was the initial (wrong) proposal, corrected mid-discussion.

### fabric-handle-construction

- Decision: `websterengine` and `builderengine` construct a `*fabricengine.Fabric` handle inline at point of use — `fabricengine.New(deps.Layout.WorktreeRoot, deps.Layout.WeftWorktree())` — in `websterengine.runIntegrationStage` (`runlevel.go`) and `builderengine.SpawnBatch` (`spawn.go`), using the `*hubgeometry.Layout` already present on `RunDeps`/`SpawnDeps`.
- Rationale: no new plumbing required. `internal/webstercli/weft.go:125` and `internal/buildercli/weft.go:128` already show the identical `fabricengine.New(layout.WorktreeRoot, weftWorktree)` pattern for weft-commit purposes, one layer up (CLI, not engine) — this reuses the established idiom at the engine layer instead.
- Rejected: threading a pre-built `*Fabric` handle down from the CLI layer through `Deps` — would mean either a second independently-constructed handle per run (duplicated `New` calls/stat checks) or refactoring the existing weft-commit call sites to share one handle across the whole run, which is a broader change than this task's scope.

### test-seam-interfaces

- Decision: define narrow consumer-side interfaces — one in `builderengine` covering `ResetHard(sha string) error`, one in `websterengine` covering `CurrentBranch() (string, error)`, `CheckoutDetached(sha string) error`, `RestoreBranch(ref string) error` (exact names/shapes are mill-plan's to finalize) — that `*fabricengine.Fabric` satisfies structurally with no adapter code. Production wires the real `*Fabric`; tests wire a lightweight fake.
- Rationale: today's tests for both migration targets use bare single-repo fixtures with no weft pairing — `TestRestartChain` (`chain_test.go:69`) uses `newScratchRepo(t)` directly; `TestIntegrationStage_FailingForkTriggersBisectAndEscalates` (`integration_test.go:199`) and its sibling `runFixture` (`runlevel_test.go:258`) build `&hubgeometry.Layout{WorktreeRoot: worktree, Cwd: worktree}` with `Hub` left empty. `fabricengine.New` stat-checks both the warp and weft paths exist on disk before returning a handle, and `Layout.WeftWorktree()` (`hubgeometry.go:790`) resolves from `Hub` — empty `Hub` means the derived weft path won't exist. Routing these tests through a real `*Fabric` would force every one onto a full paired warp+weft fixture for an operation that never touches weft. The interface seam avoids that, and mirrors the `Starter`/`MasterStarter` idiom both packages already use for exactly this kind of test-vs-production swap.
- Rejected: upgrading all affected tests to real paired warp+weft fixtures — heavier fixture cost for zero behavioral gain, since weft is never touched by these operations.

### warp-only-scope

- Decision: `ResetHard`, `CheckoutDetached`, `RestoreBranch`, `CurrentBranch` operate on warp exclusively. No weft-side interaction is added to bisect or chain-restart.
- Rationale: nothing in the current bisect or chain-restart logic reads or writes weft state today; CONSTRAINTS.md's known-gap note describes this purely as a warp-mutation bypass. This task relocates the call path, not the behavior. Confirmed both known instances already target warp specifically: `websterengine`'s `RunDeps.WorktreeRoot` is documented as "the host repo checkout," and `builderengine.ResetHard`'s own doc comment says "resets worktree's host repo."
- Rejected: emitting a weft-side record of the rollback alongside the warp mutation — an unstated requirement, out of scope creep.

### regression-guard

- Decision: add a Tier-1 substring-scan guard test, using the exact mechanism `cmd/lyx/tierpurity_test.go` already uses (a `bannedTokens []string` list + `strings.Contains` over raw file source, no AST/parsing), scoped to production `.go` files (non-`_test.go`) under `internal/websterengine` and `internal/builderengine`. Banned tokens: `.CheckoutDetached(`, `.RestoreBranch(`, `.ResetHard(`, plus the rest of the gitrepo Client Boundary Invariant's CLI-bound verb set for forward safety (`.StageAndCommit(`, `.CommitEmpty(`, `.StageAllAndCommit(`, `.Push(`, `.PushCoalesced(`, `.PushRebaseFree(`, `.Pull(`, `.Fetch(`). Read-only tokens (`.CurrentSHA(`, `status --porcelain`) are deliberately NOT banned.
- Rationale: CONSTRAINTS.md's Fabric Git Invariant already flags the general case ("every other `fabricengine` caller") as "a candidate for a future import/grep guard, not machine-checked today." This task is the natural place to close that gap for the two packages it actually fixes, so the bypass it removes cannot silently regress. A raw-substring scan over specific method-call tokens — not a package-level import ban — is required because both packages have standing, legitimate `gitrepo`/`gitexec` imports for their grandfathered read-only verbs (`gitwrap.go`'s `CurrentSHA`/`status --porcelain`; `gitquery.go`'s `HeadSHA`/`ChangedFiles`/`Dirty`); banning the import outright would break those.
- Rejected: a package-level import-ban guard (breaks the grandfathered read-only exemptions — this was the initial, incorrect proposal, corrected mid-discussion). Rejected: leaving it review-only, matching CONSTRAINTS.md's current general-case wording — the specific gap this task closes could quietly reopen with nothing catching it.

### new-fabric-method-tests

- Decision: add a new `//go:build integration` Tier-2 test file in `internal/fabricengine` (e.g. `checkout_detached_test.go`) exercising the four new methods directly: detach+verify+restore round-trip, restore-on-invalid-ref error path, reset-hard discarding local commits/uncommitted changes, `CurrentBranch` erroring on an already-detached HEAD (matching `gitrepo.Repo.CurrentBranch`'s own documented rejection of a detached HEAD).
- Rationale: gives the thin wrappers direct coverage of their own delegation/error paths, independent of whatever `websterengine`/`builderengine`'s fakes exercise once the interface seam is in place — those fakes will never call the real `*Fabric` methods at all.
- Rejected: relying solely on `websterengine`/`builderengine`'s existing bisect/chain-restart tests for coverage — once routed through the fake interfaces, the real `Fabric` methods would have zero direct test coverage.

## Technical context

- **Audit results (this session, re-verify at implementation time per the task's own step 3 caveat):** grepping the whole tree for `gitexec.RunGit`, `gitrepo.`, and raw `exec.Command("git", ...)` / `exec.CommandContext(..., "git", ...)` outside `internal/fabricengine`, `internal/gitexec`, `internal/gitrepo` themselves, then filtering to mutating-verb call sites, found exactly two production instances outside the already-documented exclusions:
  - `internal/websterengine/integration.go:146` — `repo.RestoreBranch(branch)`, inside `bisect`'s deferred cleanup.
  - `internal/websterengine/integration.go:171` — `repo.CheckoutDetached(sha)`, inside `checkoutAndVerify`.
  - `internal/websterengine/runlevel.go:845` — `gitrepo.New(deps.WorktreeRoot)`, the construction feeding both of the above via `BisectAndEscalate`.
  - `internal/builderengine/gitquery.go:76-77` — `ResetHard`, via raw `gitexec.RunGit([]string{"reset", "--hard", sha}, worktree)` (not even routed through `gitrepo`), called only from `internal/builderengine/chain.go`'s `RestartChain` (called from `spawn.go:389`).
  - No other mutating call sites were found. `internal/fabriccli/fabric.go:398` is a read call (`rev-parse`-shaped, inside fabriccli itself — out of scope per the exclusion list regardless).
- `internal/gitrepo/gitrepo.go:503` (`CheckoutDetached`), `:520` (`RestoreBranch`), `:477` (`CurrentBranch`), `internal/gitrepo/reset.go:21` (`ResetHard`) are the existing low-level methods the new `Fabric` methods delegate to unchanged — all already SHA-validated (`validSHA`/`ErrInvalidSHA`) where relevant; no new validation needed at the Fabric layer.
- `internal/fabricengine/fabric.go:57` — `func New(warpPath, weftPath string) (*Fabric, error)` stat-checks both paths exist before returning a handle. Relevant to the test-seam-interfaces decision above.
- `internal/hubgeometry/hubgeometry.go:790` — `Layout.WeftWorktree()` derives from `l.Hub` + `filepath.Base(l.WorktreeRoot)`; needs `Hub` set to resolve to a real, existing path.
- `internal/webstercli/weft.go:125` and `internal/buildercli/weft.go:128` are the reference pattern for constructing a `*fabricengine.Fabric` from a `*hubgeometry.Layout` plus its derived weft path.
- CONSTRAINTS.md's Fabric Git Invariant (lines 91-100) is the primary invariant this task closes a gap in; its "Known gap, tracked" bullet names this task by slug today and needs rewriting once both instances are fixed. The gitrepo Client Boundary Invariant (lines 177-183) already names `CheckoutDetached`, `RestoreBranch`, `ResetHard` in its exhaustive CLI-bound verb list — no change needed there, since the new `Fabric` methods reuse these already-pinned `gitrepo` methods rather than adding new ones.
- `cmd/lyx/tierpurity_test.go` and `cmd/lyx/gitrepoboundary_test.go` are the two existing guard-test precedents to model the new regression-guard test on (substring-scan and pinned-method-set-equality respectively — this task's new guard follows the former).

## Constraints

- **Fabric Git Invariant (warp + weft)** — CONSTRAINTS.md lines 91-100. The unconditional rule this whole task exists to satisfy: no LYX package other than `internal/fabricengine` runs raw/mutating git against warp or weft. Read-only verbs are exempt.
- **gitrepo Client Boundary Invariant** — CONSTRAINTS.md lines 177-183. `internal/gitrepo` splits local-vs-remote by client (go-git for reads, `gitexec` for the enumerated CLI-bound mutating set). The four verbs this task's new `Fabric` methods wrap are already on that pinned, exhaustive list — confirms no widening of `gitrepo`'s own CLI-bound surface is needed.
- **Test Tier Purity Invariant** — CONSTRAINTS.md lines 125-134. The new Tier-2 test file for the four `Fabric` methods must carry a `//go:build integration` constraint (it spawns real git). The new regression-guard test itself is Tier-1 (it only reads file source as text, spawns nothing) and needs no such constraint — but note `tierpurity_test.go`'s own precedent of self-exempting its guard file from its own banned-token scan (via `allowedSpawners`/similar) if the new guard's token list would otherwise trip on its own literal string constants.
- **Documentation Lifecycle** — per this repo's CLAUDE.md, a task introducing cross-cutting infrastructure (the new `Fabric` API surface, the new guard) updates CONSTRAINTS.md in the same commit. This task's Handoff must include that edit.

## Testing

- **`internal/fabricengine`** (new): Tier-2 (`//go:build integration`) test file covering `CheckoutDetached`, `RestoreBranch`, `CurrentBranch`, `ResetHard` directly — see the `new-fabric-method-tests` decision above for scenarios.
- **`internal/websterengine`** (update existing): `bisect`/`checkoutAndVerify`/`BisectAndEscalate`'s signatures change from taking `*gitrepo.Repo` to the new consumer-side interface. `TestIntegrationStage_FailingForkTriggersBisectAndEscalates` (`integration_test.go:199`) and `TestBisectAndEscalate_EmptySHAsDegradesGracefully` (`integration_test.go:288`) need their fixture updated to inject a fake satisfying the new interface instead of relying on a real `gitrepo.Repo` over `newScratchRepo`. `TestBisectAndEscalate_EmptySHAsDegradesGracefully`'s own doc comment already notes bisect returns before touching `repo` when `shas` is empty — confirm this still holds with the new interface type (a nil fake, matching today's "nil `*gitrepo.Repo` never dereferenced" comment).
- **`internal/builderengine`** (update existing): `RestartChain`'s signature changes from taking `worktree string` to the new `WarpResetter`-shaped interface (alongside whatever other params it still needs `worktree` for, e.g. clearing report files — check whether `worktree` is used for anything beyond the reset call before removing it entirely). `TestRestartChain` (`chain_test.go:69`), `TestRestartChain_ChainlessErrors` (`chain_test.go:124`), `TestRestartChain_UnrecordedAnchorErrors` (`chain_test.go:144`) need updating to construct/inject the fake.
- **`cmd/lyx`** (new): the regression-guard test — see the `regression-guard` decision above.
- **Manual verification step (not a persistent test):** re-run this task's own step-1 grep across the whole tree at the end of implementation, confirming zero mutating git call sites remain outside `internal/fabricengine`/`internal/gitexec`/`internal/gitrepo` (beyond the documented exclusions). The new `cmd/lyx` guard test mechanizes this going forward for the two packages it covers; the manual re-grep is the one-time confirmation this task's step 4 asks for across the whole tree.

## Q&A log

- **Q:** What did `fabric-rebase-reconcile` actually land, and does it already provide the warp-checkout/restore API this task's brief assumes? **A:** It landed `Fabric.Pull`'s remote-reconcile logic (ancestry detection + correspondence re-anchor), which calls `f.Warp.ResetHard` internally but exposes no public verb. This task builds the API surface from scratch.
- **Q:** Should the new `Fabric` methods be named `WarpCurrentBranch`/`CheckoutWarpDetached`/etc., exposing which internal repo they hit? **A:** No — verified zero external callers of `Fabric.Warp`/`Fabric.Weft` or of the one existing Warp-named method (`SnapshotWarpSHA`) exist anywhere in the codebase today. Named after the git verb only (`CheckoutDetached`, `RestoreBranch`, `CurrentBranch`, `ResetHard`), preserving the one-repo illusion at the public API boundary.
- **Q:** How do `websterengine`/`builderengine` obtain a `*Fabric` handle? **A:** Construct inline via `fabricengine.New(deps.Layout.WorktreeRoot, deps.Layout.WeftWorktree())`, reusing the `*hubgeometry.Layout` already on `RunDeps`/`SpawnDeps` — same pattern `webstercli`/`buildercli` already use one layer up.
- **Q:** Do existing bisect/chain-restart tests need to move to real paired warp+weft fixtures? **A:** No — narrow consumer-side interfaces let tests keep their current single-scratch-repo fixtures via a fake; only production wires the real `*Fabric`.
- **Q:** Should chain-restart/bisect gain any weft-side interaction as part of this migration? **A:** No — stays warp-only, exact same behavior, only the call path changes.
- **Q:** Should this task add a machine-enforced guard against regression, or leave it as a review obligation like CONSTRAINTS.md's current general-case wording? **A:** Add one, scoped to the two packages this task fixes — but as a substring-scan over specific mutating-verb tokens (matching `tierpurity_test.go`'s mechanism), not a package-level import ban, since both packages have legitimate standing imports for grandfathered read-only verbs that an import ban would break.
- **Q:** Should the four new `Fabric` methods get their own dedicated tests, or rely on webster/builder's tests via the fake interfaces? **A:** Dedicated Tier-2 tests in `internal/fabricengine` — once the interface seam is in place, webster/builder's tests never call the real methods at all.
