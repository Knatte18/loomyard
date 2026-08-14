# fabric — fixer report, round 3 (`fable-high-r3`)

Companion to `_mill/fabric-review-fable-high-r3.md`. Job 2: fixes for the findings that review recorded.

## Summary

One finding required a code fix (M1, the primary target — the containment TOCTOU). The recorded residuals (N4 dirtiness-probe TOCTOU, the launcher-dir direct `os.Remove`) are left as-is with justification below. No BLOCKING findings. No LOW/NIT findings beyond those residuals.

## M1 (MEDIUM) — containment TOCTOU — FIXED

**What was wrong:** the destruction gate's containment check resolved symlinks at one instant, and the two arbitrary-path executors (`removePath`, `removeLink`) then acted on the nominal path at a later instant. A symlink planted at an intermediate segment of a gate target — dangling when `checkPathRequest` ran (so its absent-target short-circuit skipped every check) and flipped live-and-escaping before the executor's `os.Lstat`+unlink — carried a gated `remove --force` outside the hub. CONFIRMED live: 3/3 trials deleted a file outside the hub, reported as (partial) success (see review report for the reproduction).

**What I implemented:**

- Added `removeContainedPath(container, target string, recursive bool) (removed, wasDir bool, err error)` in `internal/fabricengine/destroy.go`. It removes the container-relative form of `target` through an `os.Root` (Go 1.26) rooted at `container`, so each path component is resolved and unlinked as one `openat` chain that atomically refuses any component escaping the container ("path escapes from parent") at removal time. A final-component junction link is still removed as a link (`os.Root` never follows the target's last component). Idempotent on an absent target and on an absent container.
- Rewrote `removePath` and `removeLink` to remove through `removeContainedPath` instead of `os.RemoveAll`/`os.Remove`/`fslink.Remove` on the nominal path. Mutation-record details (`"recursive"`/`"single"` for `path_removed`; `link_removed` only when actually present) preserved.
- Removed the now-dead `var RemoveAll = os.RemoveAll` seam (no consumer once removal routes through `os.Root`; grep confirmed zero external readers/assigners).
- Updated `internal/fabricengine/doc.go`'s "The gate's checks are not atomic with its acts" section to distinguish the DIRTINESS window (still a stated limit) from the CONTAINMENT dimension (now bound to the act, needing no lock — the atomicity is the kernel's `openat` escape refusal).
- Updated `CONSTRAINTS.md`'s Fabric Destruction Chokepoint Invariant "Containment is resolved, not lexical" bullet → "…, AND bound to the act", naming the R3 window and the `removeContainedPath`/`os.Root` closer.
- Updated `cmd/lyx/destructiveguard_test.go`'s explanatory comment for the `"RemoveAll("` banned token (the seam it referenced is gone; the bare token now guards `root.RemoveAll(`/`os.RemoveAll(`).

**Why this layer, not a second `EvalSymlinks`:** re-resolving the nominal path immediately before the act only narrows the same class of window (same gap, smaller). Rooting the unlink at the container makes resolution and act one atomic `openat` chain, so there is no window to time. Verified the exact `os.Root` semantics the fix depends on with standalone probes before writing it (intermediate escape refused + outside preserved; escaping-leaf link removed as a link, target preserved; recursive tree with escaping children removes children as links + preserves targets; absent → IsNotExist; `..`-escape refused).

**Tests added:**

- `internal/fabricengine/destroy_toctou_test.go` (hermetic, `package fabricengine`): pins `removeContainedPath`'s act-time escape refusal directly — escaping intermediate refused + outside preserved; legitimate nested file/dir removed with correct `wasDir`; final-component link removed as a link with target preserved; absent target no-op. **Sabotage-proved:** temporarily reverting `removeContainedPath` to a nominal-path `os.RemoveAll`/`os.Remove` made `TestRemoveContainedPath_RefusesEscapingIntermediate` FAIL ("got nil error", outside file deleted); restoring the fix passed it.
- `internal/fabricengine/destroy_containment_toctou_integration_test.go` (`//go:build integration`): drives the real `Topology.Remove(force=true)` verb end-to-end against a live escaping symlink planted at the per-slug launcher directory, and asserts the outside files are preserved — a full-stack guard that the whole `remove` call never deletes outside the hub through the escaping segment.

The unit test is the deterministic authoritative guard for the act-time property; the integration test is the end-to-end companion. An external toggle-race test is deliberately NOT added: the fix makes the act refuse escape regardless of timing, so a race test would pass on every run post-fix and only flakily catch a regression — the deterministic unit test is strictly stronger. This is stated in the test file headers.

**Verification:**
- `go build ./...` rc=0; `go vet` (fabricengine/fabriccli/gitexec/gitrepo) rc=0.
- `go test ./internal/fabricengine/... ./internal/fabriccli/... ./internal/gitexec/... ./internal/gitrepo/... ./cmd/lyx/... -count=5` all `ok`.
- Guard tests: `TestNoDestructiveBypass_FabricengineProductionSource`, `TestMutationRecord_FabricengineProductionSource`, `TestEnforcement_MarkdownLinks`, `TestEnforcement_FabricVocabulary`, `TestHermeticGitEnv_*`, `TestTierPurity_*` all `ok`.
- `go test -tags integration` (all four fabric pkgs) `ok`; 4× concurrent full integration suites: all rc=0, no FAIL/permission/dangling/panic markers.
- Redeployed the fixed binary (`./deploy-dev`, no instrumentation) and re-drove the live escape scenario: `remove --force` against a live escaping launcher symlink refuses at containment and preserves the outside canaries; a normal `add`+`remove --force` still succeeds (ok=true, links_removed=3, clean teardown).

**Changed files:** `internal/fabricengine/destroy.go`, `internal/fabricengine/doc.go`, `internal/fabricengine/destroy_toctou_test.go` (new), `internal/fabricengine/destroy_containment_toctou_integration_test.go` (new), `CONSTRAINTS.md`, `cmd/lyx/destructiveguard_test.go`.

## Deliberately NOT fixed (with reasons)

- **N4 (dirtiness-probe TOCTOU):** left as-is. It is the accepted, documented narrow residual in `doc.go` ("the gate's DIRTINESS check is not atomic with its act"). This round's dedicated attempt (source-traced in the review report) could not improve it beyond PLAUSIBLE / CONFIRMED-by-source: the only reachable probe→recursive-removal paths are either pre-checked (`Remove`'s own `worktreeDirty` before `removeWarpWorktreeDir`) or bypass the probe entirely via `force` (both teardown fallbacks pass `force:true`). No evidence this round shows it worse than documented, and it is orthogonal to containment (the M1 fix neither addresses nor needs to address it). Fixing it would require a lock held across probe and act at every executor — a larger claim about every future call path than the residual risk warrants, exactly as `doc.go` states.
- **Launcher-dir direct `os.Remove(launcherDir)`** (`launchers.go:242`, allowlisted): left as-is. It removes the FINAL-component link only (never follows it), so a symlink planted AT `_launchers/<slug>` is removed as a link — safe. An escape would require `_launchers` itself (the container / hub geometry) to be replaced by a symlink, a materially higher bar than the confirmed `_launchers/<slug>` intermediate the M1 fix closes. Routing it through a `destroy.go` helper would move an allowlisted call for no gain proportional to the narrower exposure.

## Sandbox suite

No `SANDBOX-FABRIC-SUITE.md` change: the M1 behavior (a containment escape via a concurrently-mutated intermediate symlink) is not a visual/interactive scenario the black-box suite exercises — it is fully covered by the deterministic unit test and the integration test against real git.

## Merge-readiness

MERGEABLE. The chokepoint's core promise — containment is "the one thing `--force` can never override" — is restored for the arbitrary-path executors and regression-guarded. Standing limit, unchanged from prior rounds: Windows path/junction behavior is out of scope (unreachable from a Linux host); `os.Root`'s Windows junction semantics are a stdlib cross-platform contract this review could not execute, consistent with the module's existing Windows-verification posture.
