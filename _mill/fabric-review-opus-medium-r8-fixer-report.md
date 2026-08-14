# `fabric` — fixer report, round 8 (`opus-medium-r8`)

Companion to `_mill/fabric-review-opus-medium-r8.md`.
Every finding recorded in the review was fixed — 4 of 4, all severities. Nothing deferred.

## Summary

| ID | Severity | Status | Commit |
|----|----------|--------|--------|
| M1 | MEDIUM | FIXED | `4b0f7d25` |
| L2 | LOW | FIXED | `6cbb759e` |
| L1 | LOW | FIXED | `f543021e` |
| L3 | LOW | FIXED | `563557cf` |
| — | docs | module doc narrative for M1/L1/L2 | `14cf3a2b` |

Review report committed at `26c33757`, before any production or test file was touched (A-before-B).

## What was implemented

### M1 — `removeLaunchers`' launcher-directory removal now binds containment to the act

`internal/fabricengine/launchers.go`. The directory removal ran the gate's `checkPathRequest` and then
performed a raw, unrooted single-entry removal of the nominal path, making it the third arbitrary-path
removal in the package and the only one still carrying R3's check-then-act window.

It now calls `removeContainedPath(launchersDir(l), launcherDir, false)`. Choosing the existing helper's
non-recursive branch is what makes this surgical: `os.Root.Remove` is refused by the OS on a non-empty
directory exactly as the previous call was, so `TestRemoveLaunchers_PreservesForeignContent`'s
preserve-operator-content property — the reason this site deliberately avoids `removePath`'s `RemoveAll`
branch — is unchanged, while component resolution and the unlink become one rooted `openat` chain.

**The regression guard is mechanical, and that is deliberate.** `launchers.go` is now OFF
`cmd/lyx/destructiveguard_test.go`'s allowlist, so reintroducing a raw removal there fails
`TestNoDestructiveBypass_FabricengineProductionSource`. I sabotage-proved this rather than asserting it:
reintroducing the old call made the guard fail with
`internal/fabricengine/launchers.go: contains banned destructive-bypass token "os.Remove("`, and restoring
the fix made it pass again.

Two behavioural tests were added alongside (`internal/fabricengine/portallauncher_test.go`):
`TestRemoveLaunchers_DirRemovalIsContained` (an escaping intermediate component is refused and the
out-of-container victim survives — the victim is an EMPTY directory on purpose, so a nominal removal
would have succeeded on it and the assertion is real rather than a vacuous OS refusal) and
`TestRemoveLaunchers_EmptyDirRemovedAndRecorded` (the success path and the unrecorded idempotent
second call, pinning that `removed=false` still means "record nothing").

**Honest limit on M1's evidence.** I did **not** reproduce M1's escape live. The review's scenario S3
ran a purpose-built toggler against 60 foreground `add`/`remove` cycles and saw no escape; the window is
two adjacent statements and my toggler was not inotify-triggered the way R6's verification harness was.
The finding rests on the structural argument — the module's own `doc.go` and `CONSTRAINTS.md` state that a
containment check resolved at one instant and acted on later is insufficient, which is precisely why
`removePath`/`removeLink` were rewritten — plus the live confirmation (S2) that the *static* form is
refused, which is what makes it race-only. A future round should treat "M1 was fixed without a live
repro" as the honest status, not read it as a closed live-proved chain.

### L2 — the pre-gate containment refusal is now the gate's own refusal type

`internal/fabricengine/ancestors.go`. `refuseUncontainedPath` returned a bare `fmt.Errorf`, and all four
best-effort call sites (`Remove` ×2, `Prune` ×2) wrap it in `surfaceRefusal`, which by design returns
`nil` for anything that is not a `*destructiveRefusal`. It now returns
`&destructiveRefusal{Check: CheckContainment, …}`, so every site propagates it with **no call-site change**
and `RefusalOf` answers for it.

This one was **reproduced live with no race at all** (review scenario S2): before the fix,
`lyx fabric remove task-s --force` against a static escaping symlink at the `_launchers` container
reported `{"links_removed":3,…,"ok":true,"partial":false}` and exit 0 with the pair's `ide.sh` and
`fabric-checkout.sh` still on disk. After the fix, the same scenario reports
`refusing to remove launcher dir: containment check failed for …` and exits non-zero, with the
out-of-hub victim still untouched.

`TestRemoveLaunchersAndPortal_ContainmentRefusalSurvivesSurfaceRefusal` covers both helpers. It asserts
on the `surfaceRefusal` round-trip specifically, not merely on a non-nil error, because a non-nil error
was never the missing half. Sabotage-proved: reverting the hunk fails it at exactly that assertion for
both the launchers and the portal case.

### L1 — `pruneEmptyAncestors`' sweep is rooted at its stop directory

`internal/fabricengine/ancestors.go`. The walk related `cur` to `stop` with a lexical `filepath.Rel` over
nominal strings and removed the nominal path. The removal now goes through an `os.Root` opened at `stop`;
the lexical `Rel` survives only as the loop's termination condition, where it can stop the walk early but
never widen it. `ancestors.go` is likewise now OFF the destructive guard's allowlist.

**The sabotage run upgraded this finding's confidence and is worth recording plainly:** reverting the
production hunk makes `TestPruneEmptyAncestors_RefusesEscapingIntermediate` report the out-of-container
directory *destroyed*. So L1 was a **deterministic** escape needing no race — only a static symlink at an
intermediate `AnchorRel` component and a multi-segment anchor. It stays graded LOW because the blast
radius is genuinely small (a single-entry removal is refused by the OS on a non-empty directory, so only
an EMPTY out-of-hub directory could go, and this path records nothing to the mutation record), but a
future round should read it as confirmed rather than traced.

### L3 — every fabric verb declares its positional-argument arity

`internal/fabriccli/fabric.go`, `internal/fabriccli/weft_verbs.go`. `diff` was the only command setting an
`Args` validator; every other left it nil, which cobra defaults to `ArbitraryArgs`. `cobra.NoArgs` now
covers `list`, `pairs`, `reconcile`, `prune`, `cleanup`, `unwire`, `status`, `commit`, `push`, `pull`,
`sync`; `cobra.MaximumNArgs(1)` covers `add`, `remove`, `checkout`.

`MaximumNArgs` rather than `ExactArgs` is deliberate for the latter three: each already hand-rolls a
`usage: lyx fabric …` message for the too-few case, and `ExactArgs` would have replaced those with cobra's
generic text. Only the newly-caught too-many case changes.

`clone` is deliberately left WITHOUT a validator and recorded as a documented exemption in the test, with
its reason: `clone.go`'s own `len(args) != 1 && != 2` check already refuses both directions and carries
the full usage line. I discovered this the right way round — adding `MaximumNArgs(2)` broke an existing
integration test that pins that message, which is the test doing its job. The finding's suggested fix in
the review named `RangeArgs` for clone; that suggestion was wrong and the test caught it.

`internal/fabriccli/argsarity_test.go` (new, untagged — pure cobra-tree inspection, no git spawn, per the
Test Tier Purity Invariant) walks the live cobra tree and requires every verb to appear in one of the two
tables, so a newly added verb cannot quietly inherit `ArbitraryArgs`.

Live re-verified after redeploy: `unwire bogus-typo`, `commit <file>`, `list bogus` and
`add slug-one slug-two` all now refuse on the JSON envelope with exit 1; `clone a b c` keeps its original
message; valid invocations are unaffected (exit 0).

## Docs updated

- `CONSTRAINTS.md` — Destruction Chokepoint Invariant now records the third arbitrary-path removal and
  the non-recursive rooting rule (landed with M1, `4b0f7d25`).
- `internal/fabricengine/doc.go` — destruction-chokepoint section gains the narrative half of M1, L1 and
  L2 (`14cf3a2b`). This landed one commit after the fixes rather than folded into each, which is a
  deviation from the docs-in-the-same-commit rule worth naming rather than hiding; the rules half
  (`CONSTRAINTS.md`) did land with its fix.
- `manifest/roadmap.md` — deliberately untouched, per the prompt and `CLAUDE.md`.
- `tools/sandbox/SANDBOX-FABRIC-SUITE.md` — not extended. None of the four findings surfaces a
  live/visual behaviour the suite is the right home for: M1/L1 are symlink-containment properties covered
  by hermetic tests, L2's observable is an exit code and an error string, and L3 is argument validation.

## Verification

All commands run from the worktree root, after `./deploy-dev` following every source change.

- `go build ./...` — PASS
- `go vet ./internal/fabricengine/... ./internal/fabriccli/... ./internal/gitexec/... ./internal/gitrepo/... ./cmd/lyx/...` — PASS
- `go test ./internal/fabricengine/... ./internal/fabriccli/... ./internal/gitexec/... ./internal/gitrepo/... ./cmd/lyx/... -count=5` — PASS (all five packages `ok`)
- `go test -tags integration ./internal/fabricengine/... ./internal/fabriccli/... -count=1` — PASS
- Named invariant guards, explicitly: `TestNoDestructiveBypass_FabricengineProductionSource`,
  `TestNoUncontainedWrite_FabricengineProductionSource`, `TestMutationRecord_FabricengineProductionSource`,
  `TestHermeticGitEnv_GitSpawningPackagesHaveTestMain`, `TestTierPurity_UntaggedTestsSpawnNothing` — all PASS.
- **4× concurrent integration suites** (compile once, 4 copies, `-test.parallel=8`): all four `rc=0`, no
  substrate-corruption marker. The marker grep matches only test *names* containing "Fail"
  (`…FailsClosed…`, `…Failure…`), every one of which reports PASS.
- Live: full 16-verb sweep re-driven against the redeployed binary; scenarios S1/S2/S3 from the review
  re-run after each relevant fix.

### Determinism

No new test sleeps or polls — every test added is either pure cobra-tree inspection (L3) or a
filesystem-state assertion with no timing component (M1, L1, L2). The two containment tests skip rather
than fail where symlinks are unsupported, so they stay correct on a Windows runner. All new tests ran
clean under `-count=5` and inside the 4× concurrent integration runs.

## Deferred

Nothing. All 4 findings fixed.

Two standing limits are restated rather than deferred, because neither is this round's to close:

- **Windows path behaviour** — permanently unverifiable from a Linux host.
- **N4's dirtiness-probe TOCTOU** — accepted, documented residual; the prompt explicitly instructed not
  to re-attempt it, and I did not.

## Teardown

All scratch substrate I created is removed: the throwaway `myapp-HUB` hub, both bare origins, the victim
directories, the toggler binary, and the compiled integration test binary. Verified **zero stray `git`
processes**, and zero leftover lock files belonging to anything this round built.

One honest note rather than a clean-sweep claim: the session scratchpad still holds several hub trees
from EARLIER rounds of this campaign (`fresh/`, `drv/`, `drive1/`, `drive3/`, `verify2/`, `verify5/`, all
`warp-HUB`-named, none of them mine — my hub was `myapp-HUB`), each carrying the ordinary
`.gitrepo-push.lock` and `exclude.lyx.lock` artifacts a live fabric hub leaves behind. I deliberately did
not delete them: they are another agent's scratch, outside what this round created, and sweeping them
would be exactly the kind of unilateral cleanup of someone else's state this repo's isolation rules warn
against. They are inert (no process holds them) and live entirely under `/tmp`.

## Merge-readiness

**MERGEABLE.** 0 BLOCKING, 1 MEDIUM and 3 LOW found and all fixed; every hermetic, integration,
concurrent and live gate green; no data-loss defect in the normal single-instance flow; both machine
guards accurate and now covering two more files than before this round.
