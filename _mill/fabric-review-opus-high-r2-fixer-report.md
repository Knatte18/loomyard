# `fabric` — fixer report, round 2 (`opus-high-r2`)

Companion to `_mill/fabric-review-opus-high-r2.md`.
Every finding that round recorded is fixed — 0 BLOCKING, 3 MEDIUM, 4 LOW, 5 NIT, **12 of 12**,
nothing deferred.

## Merge-readiness verdict

**READY TO MERGE**, with the standing limit below.

Every gate is green from a clean run after the last commit: `go build`, `go vet` (both tiers),
the hermetic suite at `-count=5`, the full `integration` suite, the whole-repo hermetic suite,
4× concurrent integration suites with no corruption marker, and all four named guard tests.
Every live scenario that produced a finding was re-driven against a freshly deployed binary and
now behaves correctly; the seeded residual's own 4-way racing repro settles clean three times
running.

**Standing limit, unchanged from every prior round:** Windows path behaviour (junctions rather
than symlinks, open-handle rename failures, `git worktree remove --force` against a held handle)
is a genuine environment gap on a Linux host and was not executed. Every `fslink` path exercised
here took the symlink branch. The one place this round's changes could behave differently there
is `resolveAncestorSymlinks`'s `filepath.EvalSymlinks` over a directory junction — which is why
its fallback is the pre-existing lexical form, so a Windows failure to resolve degrades to the
behaviour that shipped before this round rather than to a refusal.

## What was implemented

Commit per fix, on `fabric-crucible-hardening`, **not pushed**. Eleven commits (four review-note
commits during Job 1, then one per finding group).

| Finding | Sev | Commit | What landed |
| --- | --- | --- | --- |
| M3 | MEDIUM | `b0aa40b4` | The gate's containment check and its one lexical-only ownership predicate now resolve symlinks. |
| M1 | MEDIUM | `8e84a9c5` | `.lyx` adoption merges a directory present on both sides instead of refusing forever. |
| M2 | MEDIUM | `97052a7a` | `reconcile` reports a pair it failed to repair as a failure. |
| L1 | LOW | `fa6d53d3` | `--force` propagates into both destructive fallback requests. |
| L2 | LOW | `28e5f735` | Request-shape validation runs before the absent-target short-circuit. |
| L3 | LOW | `55ba1e6e` | `prune` emits `"entries":[]`, never `null`. |
| L4 | LOW | `ec37b7be` | `clone` refuses a warp remote whose HEAD names a nonexistent ref. |
| N1-N4 | NIT | `524bfb33` | Read-only-result-type count corrected in three places; two honest limits stated. |
| N5 | NIT | `01b6a38d`, `86be53d4` | A pair that vanished mid-walk is named as such, decided by git's registration. |

### M3 — the round's primary-target finding

`internal/fabricengine/ancestors.go`, `internal/fabricengine/destroy.go`.

`refuseUncontainedPath` and `pathAtOrBelow` both compared nominal paths through `filepath.Rel`.
Two new helpers in `ancestors.go` — `resolveAncestorSymlinks` (full `EvalSymlinks` with an
ancestor-walk fallback for a not-yet-existing path) and `containmentPath` (resolves the target's
PARENT chain only) — now sit in front of both.

Three design points worth naming, because each was a way to get this wrong:

- **The target's final component stays unresolved.** Every junction the gate removes IS a link,
  living in the warp worktree and pointing into weft. Resolving the leaf would relocate the target
  into weft and make the warp-worktree container refuse every legitimate `unwire` — the fix would
  have broken the verb it was meant to protect. There is a dedicated subtest for exactly this.
- **Both sides are resolved, never one.** A container legitimately reached through a symlink
  (macOS's `/var` → `/private/var` is the everyday case) must not start refusing its own real
  children. Also covered by its own subtest.
- **`ownedUnderGeometryRoot`'s base-name test runs against the RESOLVED root**, so a link standing
  where `_launchers` belongs is refused rather than masquerading as the geometry root.

A separate wart the fix surfaced was fixed in the same commit: `checkPathRequest` wrapped
`refuseUncontainedPath`'s whole message as its refusal reason, which rendered
`"refusing to remove remove launcher script"` in a real refusal. `containmentFailure` was split
out so the gate attributes its own refusal; the two non-gate callers keep the prefixed wrapper,
where it reads correctly.

### M1 — root cause established before patching, as the prompt required

The writer is `internal/logger/sink.go`'s `ensureDurableSink`, whose only suppression is
`testing.Testing() && os.Getenv("LYX_TRACE") != "1"`. A deployed binary therefore runs
`os.MkdirAll(<anchor>/.lyx/logs)` on the first `Info`-or-above record — `durableHandler.Enabled`
accepts Info+ unconditionally, never consulting `levelVar`, so the default silent `Warn`
verbosity does not suppress it — or on `NotifyExit(code != 0)`. `LogsDir` is warp-anchored, so in
the post-`unwire` window that creates a real warp-side directory.

The fix is at the layer the prompt asked for (making the window survivable rather than only
improving the message): `adoptDotLyxContent` now merges a dir/dir collision recursively, via a
whole-tree pre-check (`refuseUnmergeableAdoption`) that runs to completion before
`mergeAdoptionTree` moves anything, so a refusal still leaves both sides untouched. Any other
collision shape — file/file, file/dir, link/dir — still refuses, because resolving those would
mean choosing a winner between two files, which fabric never does on the operator's behalf.

I deliberately did NOT change `internal/logger`. Gating the durable sink on wiring state would
make lyx's own observability depend on fabric's topology, inverting a dependency the repo keeps
one-way, and would silently lose the trace records for exactly the failing invocations that most
need them.

`cmd/lyx/destructiveguard_test.go`'s `junction.go` allowlist reason was rewritten to name BOTH
audited `os.Remove` sites (the `.lyx` root and each drained subdirectory), since the entry
explicitly says it is not a blanket exemption for future removals in that file.

### M2 and its counterweight

`runReconcile` now emits through a new `errWithRecordFields` when any pair carries an `Error`,
keeping the `pairs` array on the failure path so a caller gains the honest verdict without losing
the report it needs to act on.

`prune` and `cleanup` deliberately keep their current shape, and the reason is recorded in
`doc.go`: their per-entry `Error` doubles as the explanation for a DESIGNED refusal
(`Protected`'s "commit them or re-run with --force"), so treating it as a failure would report a
documented outcome as one.

**This fix had a consequence I had to chase, and it is the most important thing in this report.**
Making a per-pair error drive the exit code exposed reconcile to a race it cannot avoid:
`git worktree list` is read once, before the per-pair loop, so a concurrent `remove`/`prune` can
delete a pair mid-walk. Before the follow-up, **8 out of 8** live remove-vs-reconcile races exited
1. That is a regression I introduced, and shipping M2 without N5's follow-up would have traded one
dishonest exit code for a noisy one. After the follow-up: **8 out of 8 exit 0** with
`action=vanished_mid_walk` and no pair error.

### N5's follow-up — why the first cut was not enough

The first cut decided "vanished" by `os.Stat` alone and missed the common interleaving twice:
the two early-`continue` sites bypassed the check entirely, and — less obvious — reconcile's own
repair steps RECREATE the directory on their way past, because wiring a junction creates the
link's parent. Deciding by git's own worktree registration covers both. The stat stays as a cheap
short-circuit and the registration read only runs for a pair already reporting a problem, so an
ordinary reconcile pays nothing.

## Tests added, and every one sabotage-proved

Each new test was run against the reverted production hunk and confirmed to fail at the intended
assertion, then the hunk was restored and the test confirmed green.

| Test | File | Sabotage result |
| --- | --- | --- |
| `TestGate_ContainmentResolvesSymlinkedAncestors` (4 subtests) | `internal/fabricengine/destroy_test.go` | with BOTH hunks reverted: `err = <nil>; want a *destructiveRefusal` — i.e. the gate allowed it and `removePath` deleted the victim |
| `TestGate_ZeroValueDeclarationsAreRefusals` (+4 subtests) | `internal/fabricengine/destroy_test.go` | order swapped back: 3 subtests fail with `err = <nil>` |
| `TestDotLyxJunction_AdoptionMergesADirectoryPresentOnBothSides` | `internal/fabricengine/dotlyxjunction_integration_test.go` | dir/dir predicate inverted: fails with the refusal message |
| `TestRemoveWarpWorktreeDir_FallbackHonoursForce` (2 subtests) | `internal/fabricengine/destructivegaps_integration_test.go` | fallback `force` removed: `worktree removed = false; want true` |
| `TestReconcile_ReportsAPairThatVanishedMidWalkAsSuch` | `internal/fabricengine/destructivegaps_integration_test.go` | check removed: `Action = "unmanaged_reported"` + raw chdir error |
| `TestCloneHub_RefusesAWarpRemoteWhoseHeadNamesANonexistentBranch` (2 subtests) | `internal/fabricengine/clone_adopt_test.go` | call removed: `= nil; want a refusal` |
| `TestRunCLI_ReconcileReportsAFailedPairAsAFailure` | `internal/fabriccli/envelopecontract_integration_test.go` | branch removed: exit `0`, `"ok":true` with the error buried in `pairs[]` |
| `TestRunCLI_PruneEmitsAnEmptyArrayNotNull` | `internal/fabriccli/envelopecontract_integration_test.go` | seed reverted: `"entries":null` |
| `TestRunCLI_ReconcileDoesNotFailOnAPairThatVanishedMidWalk` | `internal/fabriccli/envelopecontract_integration_test.go` | counter-assertion to the row above; both must be read together |

Every new test is deterministic by construction — each builds the failing state directly (a
planted symlink, a deleted directory with the registration left, a `git worktree lock`) rather
than racing for it. There is no `time.Sleep` anywhere in them. They were run under the 4×
concurrent-copies pattern with no failures.

**One pre-existing test depended on a defect and was retargeted.**
`driveDirtinessGateRefusal` (`livestate_refusal_selftest_test.go`) drove `Remove(force=true)`
against a locked, tracked-dirty worktree and relied on the fallback request silently dropping
`--force` — L1's defect — to produce a gate dirtiness refusal. Its own doc comment described the
dropped flag as if it were a property. It now drives `Fabric.ResetHard`, the one gate site whose
`force` is a constant in production source, and the comment records why a helper must never be
built on a bug.

## Docs updated in the same change as the fix

- `internal/fabricengine/doc.go` (the module doc): the destruction chokepoint's new
  "why containment resolves symlinks" and "the checks are not atomic with the acts" sections;
  the `.lyx` adoption merge rule and its root cause; the per-item-outcome envelope obligation and
  why `prune`/`cleanup` are exempt; `ReconcileActionVanishedMidWalk`; the corrected read-only
  result-type count.
- `CONSTRAINTS.md`: Fabric Destruction Chokepoint Invariant gains the resolved-containment bullet
  and the request-shape-check-runs-first clause; Mutation Record Invariant's read-only count
  corrected.
- `internal/fabricengine/destroy.go`, `ancestors.go`, `junction.go`, `remove.go`, `prune.go`,
  `reconcile.go`, `clone.go`, `internal/fabriccli/envelope.go`: file/function doc comments.
- `cmd/lyx/destructiveguard_test.go`: the `junction.go` allowlist reason and two table comments.
- `manifest/roadmap.md`: **deliberately untouched**, per the prompt and CLAUDE.md — this is
  hardening, not a planned-item move.
- `docs/overview.md`: **no change needed** — no module added, no CLI verb added or removed, no
  movement in the module table or execution stack.

## Exact commands run, and results

```
go build ./...                                                              rc=0
go vet ./internal/... ./cmd/...                                             rc=0
go vet -tags integration ./internal/fabricengine/... ./internal/fabriccli/...
       ./internal/gitexec/... ./internal/gitrepo/...                        rc=0
go test ./internal/fabricengine/... ./internal/fabriccli/... ./internal/gitexec/...
        ./internal/gitrepo/... ./cmd/lyx/... -count=5                       rc=0
go test -tags integration ./internal/fabricengine/... ./internal/fabriccli/...
        ./internal/gitexec/... ./internal/gitrepo/... -count=1              rc=0
go test ./... -count=1                                                      rc=0

go test ./cmd/lyx/ -run 'TestHermeticGitEnv_GitSpawningPackagesHaveTestMain|
  TestTierPurity_UntaggedTestsSpawnNothing|TestNoDestructiveBypass_FabricengineProductionSource|
  TestMutationRecord_FabricengineProductionSource'                          all 4 PASS

go test -c -tags integration -o $SCRATCH/fabric.integration.test.exe ./internal/fabricengine/
for i in 1 2 3 4; do ( $SCRATCH/fabric.integration.test.exe -test.count=1 -test.v \
    -test.parallel=8 > $SCRATCH/intF_$i.txt 2>&1; echo "run$i rc=$?" ) & done; wait
  -> run1..run4 rc=0; marker grep -> "no markers"
```

Live driving, all against a freshly `./deploy-dev`-built binary, on throwaway hubs built from
local bare `git init` remotes:

| Scenario | Before | After |
| --- | --- | --- |
| symlink at `<Hub>/_launchers/<slug>`, then `remove --force` | 2 files outside the hub DESTROYED, `ok:true`, exit 0 | containment refusal, exit 1, all 4 files SURVIVED |
| `unwire` → failed command → `reconcile`, twice | permanent refusal, `junction_healthy:false`, warp permanently untracked-dirty | `reconcile` rc=0 both cycles, `junction_healthy:true`, `git status` clean |
| seeded 4-way `(unwire; reconcile)` racing repro | the stuck state | settles to `junction_healthy:true`, clean warp, 3/3 attempts |
| `reconcile` on an unrepairable pair | `ok:true`, exit 0, `partial:false` | `ok:false`, exit 1, `pairs` report retained |
| `remove t2 --force` on a locked, dirty worktree | "use --force" refusal, half-torn pair | removed, exit 0 |
| `clone` against a dangling-HEAD warp remote | `ok:true`, contentless hub fully wired | actionable refusal, hub torn down, no residual |
| `remove` racing `reconcile`, 8 fresh hubs | 8/8 exit 1 (after M2, before N5's follow-up) | 8/8 exit 0, `vanished_mid_walk` |
| 4× concurrent `remove --force` on one target | 1 winner, 3 correct refusals, no marker | unchanged — still correct |
| `prune --apply --force` racing `add` | correct | unchanged |
| all 16 verbs, foreground, one at a time | — | 15 rc=0, 1 rc=1 (my own bad `-m` flag) |

## Deferred: nothing

No finding was left unfixed. There was no case where fixing one required something I could not do
alone this round.

## Observations recorded but NOT fixed, with reasons

These are not deferred findings — they were surfaced while verifying fixes, after the review was
frozen and committed, so recording them here rather than retro-fitting them into the review is
the honest placement. Each is stated so the next round can grade it independently rather than
having to rediscover it.

1. **A `remove`-vs-`reconcile` race can leave an inert directory at `<Hub>/<slug>`.** Reconcile's
   junction wiring recreates the link's parent, so if the teardown lands mid-wiring, an empty
   directory holding one or two dangling symlinks survives a completed `remove`. Verified
   inert: `git worktree list` never yields it (no registration), `reconcile` ignores it, `pairs`
   and `list` do not report it, `junction_healthy` stays true, and no verb refuses because of it.
   Remedy is `rmdir`. Not fixed because closing it needs a re-verification of registration
   between the health check and each wiring step — a narrower TOCTOU chasing a residue that
   loses nothing and blocks nothing. Explicitly different in kind from the seeded residual, which
   was permanently non-self-healing and blocked the documented remedy.
2. **`.gitrepo-push.lock` accumulates at `<Hub>/_board`.** This is the documented
   `PushCoalesced` artifact (`lock.FileLock.Release` unlocks without deleting) that `doc.go`
   already names — `Bolt.Push` is a non-fabric-async-push caller of `PushCoalesced`, exactly the
   remaining case `doc.go` says can still produce one. It is on the weft side, so `Fabric.Status`
   never surfaces it. Behaving as documented; not a defect.
3. **`SANDBOX-FABRIC-SUITE.md` was not extended.** None of this round's findings surfaced a
   live/visual behaviour the suite lacks: every one is a JSON-envelope or filesystem-state
   assertion, and all nine are now covered by deterministic automated tests, which is a stronger
   home for them than a human-driven scenario.

## What I could NOT verify, and why

- **Windows path behaviour.** Genuine environment gap (Linux host). See the merge verdict for the
  one place this round's changes touch it and how the fallback is chosen to degrade safely.
- **The registration branch of `markVanishedMidWalk` as a DETERMINISTIC test.** The
  directory-gone branch is covered deterministically; the registration branch is only reachable
  when a teardown lands inside reconcile's own repair steps, which cannot be constructed without
  intervening mid-call. It is covered by the live 8/8 race loop instead, which is real evidence
  but not a regression test — stated plainly rather than papered over with a test that would only
  appear to exercise it.
- **The gate's dirtiness TOCTOU as a live repro** (review finding N4). The window is real and
  derivable from source, but the only injection seam sits after the check, so driving it would
  prove the seam's position rather than the race. Fixed as documentation, which is what the
  finding asked for.

## Teardown

Every throwaway hub, temp directory and compiled test binary created during this round was
removed. Verified afterward: **zero** stray `git` processes owned by this user, **zero** lock
files anywhere under the scratch directory, **zero** lock files in the repo worktree, and
`git status --porcelain` empty (everything committed).

Nothing was pushed, on this branch or any other.
