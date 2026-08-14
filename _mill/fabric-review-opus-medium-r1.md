# fabric — independent crucible review, round `opus-medium-r1`

Reviewer: fresh clean-room round agent (Opus, medium effort).
Worktree: `/home/knatte/Code/loomyard/wts/fabric-crucible-hardening`, branch `fabric-crucible-hardening`, at `08520a1b`.
Scope: full independent review of `internal/fabricengine`, `internal/fabriccli`, `internal/gitexec`, `internal/gitrepo`,
weighted toward the never-independently-reviewed commits `74e6a6bb` (gitexec checked/raw split), `f4ce0188` (hubforge/fabric fixture inversion), `16c0cfcc` (`t.Parallel` unblock).

Clean-room discipline: no `.scratch/` prior-round material was read at any point (this is round 1 of the segment; none exists).
The pre-count file was never opened.
All findings below were formed from the code, the docs, and driving the real substrate, and this file was saved in full before a single production or test file was edited.

---

## Executive summary

The gitexec migration (`74e6a6bb`) is, on the evidence, a genuinely careful piece of work.
I set out expecting to find the classic migration failure — a call site that used to branch on an exit code or string-match stderr, silently re-shaped into "an error came back" — and drove every higher-risk site against real git to force it.
I did not find one.
Every mixed probe (`rev-parse --verify --quiet`, `rev-parse @{u}`, `rev-parse --abbrev-ref HEAD`, `ls-tree`, `diff --cached --quiet`) recovers its answer arm through `errors.As(err, &gitexec.GitError)` and keeps a separate arm for the exec-level failure, and each one carries a comment naming which arm is the answer and which is the failure.
The one surviving stderr string-match in the fabric tree (`weftgit.go:219`'s `"did not match any files"`) still works, because `*GitError.Error()` renders trimmed stderr into the message the wrapper chain preserves — I confirmed the composed string live.

The destruction chokepoint survived being touched.
I drove a path-traversal containment attempt, an unowned-path removal, a prime-worktree removal, a dirty-tracked removal without `--force` and an untracked-debris removal without `--force`, all against a real hub; every one was refused, and the one that was not (`--force`) removed exactly what it should.
Every raw destructive primitive still outside `destroy.go` is on `destructiveguard_test.go`'s per-file allowlist with a written reason.
I counted the pinned raw gitexec sites from the source myself — `internal/gitrepo` 3 (`run`'s body, `Pull`, `Fetch`), `internal/fabricengine` 2 (`weftRepoExists`, `weftBranchExists`) — and they match both CONSTRAINTS.md's prose and `checkedcall_test.go`'s pinned map, which does enforce the exact numbers rather than merely existing.

`t.Parallel` safety holds under real pressure: six concurrent copies of the compiled integration binaries produced zero markers, and eight concurrent `lyx fabric add`/`sync`/`reconcile` invocations against one live hub produced zero failures, an uncorrupted `.git/info/exclude`, and one correctly coalesced weft commit carrying all eight files.
`internal/fabricengine/coalesce_integration_test.go` is confirmed the only remaining fabric test file doing a raw `t.Chdir`/`os.Chdir`, and its exemption is genuine — the cwd mutation is the assertion.

What I did find is a coherent cluster of **documentation-and-message drift left behind by the migration**: seven findings, all LOW or NIT, no BLOCKING and no MEDIUM.
Four are doc statements that the migration falsified (a doc naming `gitexec.RunGit` at a site that now calls `gitexec.Run`; an owner-set enumeration that predates `hubforge`; a tolerance attributed to the wrong function).
One is a user-visible message-quality regression the migration introduced at roughly seven sites: `*GitError.Error()` now renders the git command itself, so wrappers that were written to supply that command when the underlying error was a bare exit code now emit it twice.
One is an operability gap in `Remove`'s refusal path.
One is a field carried and test-pinned but never rendered or read.

**Top risks:** none that block.
The residual risk in this module is not in what I found but in what I structurally cannot reach — Windows path/junction behavior (see limits below).

**Merge-readiness opinion:** MERGE-READY once the seven findings below are fixed.
None of them changes fabric's behavior in the normal single-instance flow; all of them make the module's own documentation match the code that shipped, or make a failure message legible.

---

## Scope assessment — plan-vs-shipped

The three unreviewed commits each delivered what their message claims, and I found no silently-dropped requirement and no over-reach.

- **`74e6a6bb` (gitexec checked/raw split).** Shipped: `internal/gitexec` with `Run` (checked, `*GitError` on non-zero exit, raw unwrapped error on exec failure) and `RunGit` (raw), both over one `runCore`; ~19 `fabricengine` files and `internal/gitrepo` migrated; a two-token guard (`checkedcall_test.go`) pinning per-package raw-site counts. The `//gitexec:raw` marker discipline is real — I verified each of the five markers sits adjacent to the call it justifies, and each justification is substantive rather than a rubber stamp. Delivered as intended.
- **`f4ce0188` (fixture-dependency inversion).** `internal/lyxtest` is gone; `internal/hubforge` builds every hub fixture through `fabriccli.CloneAndWire`. The inversion is real (no package in `fabriccli`'s dependency set imports `hubforge`, which is compile-enforced). The fixture geometry a real operator gets is unchanged in shape — I drove `lyx fabric clone` at both a root anchor and a `--subpath backend` anchor and confirmed junctions wired at the anchored directory, `.lyx`/`_lyx`/`_board` present, anchored exclude entries (`/backend/_lyx`, not a bare `_lyx`), and the anchor and warp-binding records both written and committed onto `weft:main`. Delivered as intended — but it left `doc.go`'s owner-set enumeration stale (finding F2).
- **`16c0cfcc` (`t.Parallel` unblock).** The `RunCLIIn`/`WithCwd` seam is in place, `cwdmutation_test.go` guards the migrated file set with a non-vacuous matcher and a scanned-count floor, and the single allowlist entry is justified and proven to still contain the token it exempts. Delivered as intended.

Nothing here reads as deferred-that-should-have-shipped-in-v1, and nothing reads as shipped-beyond-scope.

---

## Code findings, severity-ranked

No BLOCKING findings. No MEDIUM findings.

### F1 — LOW (CONFIRMED) — migrated error wrappers now render the git command twice

**Where:** `internal/fabricengine/dirtiness.go:60`, `internal/fabricengine/reconcile.go:574` (`readBranch`'s fallback arm), `internal/fabricengine/index.go:59`, `internal/fabricengine/index.go:86`, `internal/fabricengine/weftgit.go:164`, `internal/fabricengine/pull.go:432`, `internal/fabricengine/status.go:183` region.

**Scenario.** Before the migration, the error these sites wrapped was a bare exit code, so each wrapper had to name the git command itself for the message to be diagnosable at all.
`*GitError.Error()` now renders `git <args>: exit <code>: <stderr>`, so the wrapper's command name is emitted a second time.

Reproduced live, twice:

```
$ lyx fabric remove notapair
"error":"check warp worktree status at .../notapair: git status --porcelain in .../notapair:
         git status --porcelain: exit 128: fatal: not a git repository (or any parent up to mount point /)..."

$ lyx fabric pairs      # weft .git moved aside
"drift_reason":"read weft branch: rev-parse exited 128 and branch --show-current:
                git branch --show-current: exit 128: fatal: not a git repository ..."
```

The command appears twice and, in the first case, the directory appears twice as well.
This is not cosmetic-only: the doubled prefix pushes git's own actionable stderr — the part the operator needs — to the far right of a long line, which is exactly the failure mode `gitexec`'s doc comment says the split exists to avoid.

**Fix.** Drop the now-redundant command name (and, where `GitError` already carries it, the directory) from the wrapper, keeping the wrapper's *semantic* contribution — what fabric was trying to do — and letting `*GitError` supply the mechanical detail. Do not change `GitError.Error()` itself; several sibling packages pin its current shape.

### F2 — LOW (CONFIRMED) — `doc.go`'s fabric-vocabulary owner set omits `internal/hubforge`

**Where:** `internal/fabricengine/doc.go:544-548`.

**Scenario.** The package comment enumerates the owner set as `internal/fabricengine`, `internal/fabriccli`, `internal/weftname`, `internal/gitkit`, `internal/boardengine`, `internal/configsync`.
CONSTRAINTS.md's Fabric Vocabulary Invariant (line 199) and `internal/lyxcwd/enforcement_test.go`'s `fabricVocabularyOwners` map (line 607) both additionally include `internal/hubforge`, added when `f4ce0188` made it the repo-wide hub-fixture factory.
`doc.go` also still describes `internal/gitkit` as "the test-fixture leaf that builds real paired worktrees" with no mention that hubforge now builds the hubs.

A reader trusting fabric's own module doc — which the review prompt correctly calls the closest thing fabric has to a living spec — would conclude `hubforge` is in violation of an invariant it is explicitly permitted by.
`doc.go` does defer to CONSTRAINTS.md for "the authoritative list", which limits the damage, but an enumeration that is present and wrong is worse than one that is absent.

**Fix.** Add `internal/hubforge` to the enumeration with the same one-clause justification the other entries carry, and name the inversion in the gitkit clause.

### F3 — LOW (CONFIRMED) — refused `lyx fabric remove` strips the pair's portal and launchers with no remedy pointer

**Where:** `internal/fabricengine/remove.go:65-79`.

**Scenario.** `Remove` runs `removePortal` and `removeLaunchers` before the `!force` dirtiness gate.
An operator who runs `lyx fabric remove <slug>` against a pair with uncommitted work is refused — correctly — but the portal junction and every launcher script for that pair are already gone:

```
$ lyx fabric remove feat-b        # feat-b has an uncommitted tracked change
{"error":"worktree has uncommitted changes; use --force",
 "mutations":[{"kind":"link_removed","target":"_portals/feat-b"},
              {"kind":"path_removed","target":"_launchers/feat-b/ide.sh","detail":"single"},
              ...],
 "ok":false,"partial":true}
```

The ordering is deliberate and documented (`remove.go`'s file header: the teardown must still run when the worktree directory is already gone), and the loss is fully self-healing — I confirmed `lyx fabric reconcile` restores both, reporting `portal_restored` with detail `"portal junction restored; launcher scripts restored"`.
The `partial: true` envelope is doing its job.
What is missing is the last step: the operator reads `"worktree has uncommitted changes; use --force"`, has no reason to suspect their launchers just vanished, and is given no pointer to the one-command remedy.
This is the exact class the mutation record exists to make visible, stopping one sentence short of actionable.

**Fix.** Name the remedy in the refusal message when the portal/launcher teardown actually recorded something, so the operator is told what was torn down and that `lyx fabric reconcile` restores it. Do not reorder the teardown — the header's rationale for its position is sound.

### F4 — NIT (CONFIRMED) — `cloneRepo`'s doc comment names `gitexec.RunGit`; the code calls `gitexec.Run`

**Where:** `internal/fabricengine/clone.go:517` (doc) vs `clone.go:542` (code).

The comment reads "The clone is executed via gitexec.RunGit with the parent directory of dest as the cwd".
It has called `gitexec.Run` since the migration.
Under the Checked-Call Invariant, `RunGit` in a comment is not merely stale prose — it names the raw entry point at a site that is now checked, which is precisely the distinction the invariant exists to keep legible.
(The guard does not flag it, correctly: it skips pure-comment lines.)

**Fix.** Name `gitexec.Run`, and say what the checked form buys here.

### F5 — NIT (CONFIRMED) — `weftwiring.go`'s file header claims all its git operations are raw

**Where:** `internal/fabricengine/weftwiring.go:6`.

"All git operations use gitexec.RunGit with explicit cwd (WeftRepoRoot or WeftWorktreePath)."
After the migration exactly two of this file's calls are raw — the two `//gitexec:raw`-marked bool predicates, `weftRepoExists` and `weftBranchExists` — and the rest (`createWeftWorktree`, `pushWeftBranch`, `removeWeftWorktree`'s prune) are checked.
This is the single most misleading of the four doc drifts, because this file is one of only two in the repo that legitimately holds raw sites, so its header is where a reader goes to learn which ones and why.

**Fix.** State the split: two pinned raw predicates with the reason, everything else checked.

### F6 — NIT (CONFIRMED) — `doc.go` attributes the "did not match any files" tolerance to `StageAndCommit`

**Where:** `internal/fabricengine/doc.go:292-294` vs `internal/fabricengine/weftgit.go:218-224`.

`doc.go` says a pathspec that resolves to nothing by the time `git add` runs is "which `StageAndCommit`'s 'did not match any files' tolerance absorbs".
`gitrepo.StageAndCommit` has no such tolerance — it returns the wrapped error from `git add`.
The tolerance is `commitWeftLocked`'s, in `weftgit.go`, which string-matches the message and folds it into `("", false, nil)` or into `commitEmptySnapshot` when snapshot tags are pending.

This matters more than a misplaced attribution usually would, because it points a future maintainer at the wrong package when reasoning about the empty-commit rule's four triggering cases, and because the real implementation is a stderr string-match — the one construct in the fabric tree whose correctness genuinely depends on `*GitError.Error()` continuing to render stderr.
That dependency deserves to be recorded where the rule is explained.

**Fix.** Attribute the tolerance to `commitWeftLocked`, and record that it rests on git's own message text reaching it through `*GitError`.

### F7 — NIT (CONFIRMED) — `gitexec.GitError.Dir` is populated and test-pinned but never rendered or read

**Where:** `internal/gitexec/gitexec.go:38-63`.

`GitError` documents itself as carrying "exactly what every merged failure message needs: the command that was run, the directory it ran in, its exit code, and its stderr".
`Error()` renders three of those four; `Dir` is never rendered.
No production caller reads it either — across `internal/`, the recovered-error field accesses are nine `gitErr.ExitCode` and five `gitErr.Stderr`, and zero `gitErr.Dir`.
Its only reader is `gitexec_test.go:130`, which pins it.

The field is not wrong to exist — a caller that wants the directory should not have to re-derive it — but "exactly what every merged failure message needs" overstates a field no merged failure message uses, and the mismatch is what leads call sites to hand-append `in %s` themselves (finding F1).

**Fix.** Keep the field; correct the doc comment to say plainly that `Dir` is carried for callers and deliberately not rendered by `Error()`, with the reason (the caller's own wrapper owns the "where", so rendering it here would double it — the same doubling F1 removes).

---

## Confirmed-still-accepted (explicitly not re-litigated)

**The correspondence index's two-phase `RebuildIndex` residual.**
`doc.go`'s "The correspondence index's write path" section grades this LOW and self-healing: the scan → `record()` writes → rebuild writes interleaving still loses the recorded entry, because `scanWarpSHATrailers` reads git outside the file's lock.
I re-verified the characterization holds post-migration rather than assuming it.
`74e6a6bb` did not touch `internal/fabricengine/corrindex.go`, `internal/state`, or `internal/lock` at all; its only change to `index.go` was 8 insertions / 16 deletions collapsing the git-call exit-code branches into `gitexec.Run` error handling, which changes no timing and no locking around that window.
The weft commit trailers remain the sole source of truth and the index remains a rebuildable cache.
**Unchanged, still accepted, no new finding.**

**The `Snapshot:`/tag accumulation cost.** Out of scope per the prompt; I found no polling consumer, so it stays a documented tradeoff.

---

## Docs & operability findings

Findings F2, F4, F5, F6 and F7 are the documentation half of this review and are listed above with the code they contradict.
Beyond those:

- `docs/overview.md` is accurate for this change set: the module tree lists `internal/gitexec` and `internal/gitrepo` with the go-git/gitexec split named correctly (line 233), and `internal/hubforge` is described correctly as the repo-wide real-hub fixture factory built through `fabriccli.CloneAndWire` (line 380). The vocabulary owner set at line 80 correctly includes `hubforge` — it is `doc.go` that is out of step, not overview.
- CONSTRAINTS.md's pinned raw-site counts are correct as written; I counted them from source rather than from the prose.
- Operability of the CLI envelope is sound and matches `doc.go`'s rule: every mutating verb carried `"mutations"` (array, never null) and `"partial"` (bool, never absent) on both success and failure, and a handler that failed before reaching its verb (`lyx fabric remove` with no `git` on PATH) carried neither.
- Refusal messages are, F3 aside, genuinely actionable: the prime-worktree refusal names the alternative, the branch-already-exists refusal names both ways forward, and the `git switch` failure carries git's own `'main' is already used by worktree at ...` stderr straight through.

---

## What was tested

Every command below was run in this worktree against real git. Raw results, not summaries.

### Hermetic

```
$ go build ./...
exit 0

$ go vet ./internal/fabricengine/... ./internal/fabriccli/... ./internal/gitexec/... ./internal/gitrepo/...
exit 0, no output

$ go test ./internal/fabricengine/... ./internal/fabriccli/... ./internal/gitexec/... ./internal/gitrepo/... ./cmd/lyx/... -count=5
ok  internal/fabricengine  0.233s
ok  internal/fabriccli     0.003s
ok  internal/gitexec       0.002s
ok  internal/gitrepo       0.003s
ok  cmd/lyx                0.622s
exit 0
```

### Live integration

```
$ go test -tags integration ./internal/fabricengine/... ./internal/fabriccli/... ./internal/gitexec/... ./internal/gitrepo/... -count=1
ok  internal/fabricengine  15.018s
ok  internal/fabriccli      0.939s
ok  internal/gitexec        0.014s
ok  internal/gitrepo        0.962s
exit 0;  grep -ciE 'FAIL|panic:' over the output => 0
```

`cmd/lyx`'s tier-purity guards (`TestHermeticGitEnv_GitSpawningPackagesHaveTestMain`, `TestTierPurity_UntaggedTestsSpawnNothing`) are inside the `./cmd/lyx/...` run above and stayed green throughout; no fix in this round touches either.

### N× concurrent integration suites (the amplifier)

Compiled once, six copies run concurrently (four `fabricengine`, two `fabriccli`):

```
$ go test -c -tags integration -o $SCRATCH/fabric.integration.test.exe ./internal/fabricengine/
$ go test -c -tags integration -o $SCRATCH/fabriccli.integration.test.exe ./internal/fabriccli/
$ for i in 1 2 3 4; do ( $SCRATCH/fabric.integration.test.exe -test.count=1 -test.parallel=8 > $SCRATCH/int_$i.txt 2>&1; echo "run$i rc=$?" ) & done
$ for i in 5 6; do ( $SCRATCH/fabriccli.integration.test.exe -test.count=1 -test.parallel=8 > $SCRATCH/int_$i.txt 2>&1; echo "cli-run$i rc=$?" ) & done
$ wait
cli-run5 rc=0
cli-run6 rc=0
run2 rc=0
run4 rc=0
run3 rc=0
run1 rc=0

$ grep -hiE 'FAIL|being used by another process|permission denied|dangling|panic:' $SCRATCH/int_*.txt || echo "no markers"
no markers
```

### Live driving — my own local hubs, no launcher

`./deploy-dev` run first (`Deployed lyx @ 08520a1b`). Two throwaway hubs built with plain `git init` in a scratch temp dir, with an isolated `GIT_CONFIG_GLOBAL` so nothing depended on my own gitconfig:

- **hub1** — root anchor, remotes `demoapp.git` + `demoapp-weft.git`.
- **hub2** — `--subpath backend` anchor, remotes `mono.git` + `mono-weft.git`.

All 16 verbs driven directly, foreground, each waited on.

**Bootstrap and topology (hub1).** `clone` produced the full mutation record and correct geometry (`_board`, `demoapp`, `demoapp-weft`, `.lyx`; anchor and `.lyx-warp` written and committed to `weft:main`; junctions `_lyx`/`.lyx`/`_board` wired; exclude entries `/_lyx`, `/.lyx`, `/_board`).
`status`, `list`, `pairs`, `add feat-a`, `list`, `pairs`, `reconcile`, `prune`, `cleanup` all rc=0 with the expected envelopes; `pairs` reported `in_sync: true`, `junction_healthy: true` for every pair; `cleanup` correctly reported `main` as `protected`.

**Destructive refusals (hub1).** Each confirmed refused, nothing destroyed:

| scenario | result |
|---|---|
| `remove feat-a`, tracked change, no `--force` | refused: `worktree has uncommitted changes; use --force` |
| `remove feat-a`, untracked file, no `--force` | refused, same message |
| `remove ../../../etc` | refused: `invalid slug ... must be a single path component` |
| `remove ../evil` | refused, same |
| `remove /etc` | refused, same |
| `remove demoapp` (prime) | refused: `it is this hub's prime worktree — the warp repository itself` |
| `remove notapair` (unowned, non-git dir) | refused; directory verified still present afterwards |
| `remove feat-a --force` | succeeded, removed exactly the pair (worktrees, branch, 3 links) |

`--force` answered dirtiness only: it did not make the containment refusals (`../evil`, `/etc`) or the prime/ownership refusals reachable.

**Forced git failures at migrated sites.**

- `pull` against a remote path that does not exist → `weft pull succeeded, warp fetch failed: gitrepo: fetch in <path>: git exited 128 (run 'git -C <path> fetch' for git's own diagnosis)`. Correct: this is `gitrepo/pull.go`'s pinned raw site, whose `//gitexec:raw` marker says stderr is deliberately withheld and the reproduction pointer stands in for it. Behaves exactly as the marker claims.
- weft repo's `.git` moved aside → `add` reported `no weft repo at <path>; create the hub with "lyx fabric clone" first` (the `weftRepoExists` predicate collapsing correctly); `status` reported the go-git-level `repository does not exist`; `pairs` degraded to `in_sync:false` with a `drift_reason` carrying git's real stderr — surfaced, not swallowed. This is where finding F1's doubled prefix was reproduced.
- **no `git` on PATH at all** (`env PATH=<empty dir>`) → every verb (`add`, `status`, `remove`) failed identically with `not a git repository: exec: "git": executable file not found in $PATH`, rc=1, and destroyed nothing. This is the direct answer to the prompt's question about the two pinned bool predicates: an exec-level failure is **not reachable** through the CLI at `weftRepoExists`/`weftBranchExists`, because cwd/location resolution spawns git and fails first. Their collapse-to-false is therefore safe at both call sites in practice, not merely by argument. Confirmed, not assumed.
- broken loose ref planted in the warp common dir → `reconcile` and `pairs` both unaffected (nothing in fabric enumerates all refs), rc=0.

**Content flow (hub1, `feat-b`).** `status` → `push` (weft commit + branch push) → `pull` → `sync` (`push_spawned` / `detached`) → `status` clean. `diff <warp-sha>` returned `no_weft_correspondence:true` before any weft commit existed and `false` after — correct in both directions.

**Coordinated switch (hub1).** `checkout main` from `feat-b` correctly refused with git's own stderr (`'main' is already used by worktree at ...`) and left `pairs` unchanged — no half-switched pair. `checkout feat-b` succeeded, recording both `worktree_switched` entries. `checkout no-such-branch` refused with `fatal: invalid reference`. `checkout main` with a dirty weft refused before touching anything.

**Unwire/reconcile round trip (hub1).** `unwire` removed `.lyx`, `_lyx`, `_board`, reported `weft_content: "preserved"` and `git_exclude: "unchanged"`, with no `gitignore` key in the envelope — matching `doc.go`'s recorded envelope change exactly. `reconcile` restored all three, reporting `junction_repointed`. Weft content verified preserved on disk.

**Portal/launcher loss and recovery (hub1)** — finding F3. Refused `remove` left `_portals/feat-b` and `_launchers/feat-b/` gone; `reconcile` restored both with action `portal_restored`, detail `"portal junction restored; launcher scripts restored"`.

**Subpath anchoring (hub2).** `clone --subpath backend` wired junctions at `mono/backend/` (not the repo root) and wrote anchored exclude entries `/backend/_lyx`, `/backend/.lyx`, `/backend/_board` — the slash-prefixed anchored form `doc.go` requires, not bare names. `status` reported weft paths as `backend/_lyx/config/*.yaml`.

**Pull reconciliation, all four arms (hub2).**

- Upstream force-push that rewrote away the only commit carrying a recorded correspondence → `warp history rewritten and no recorded correspondence survives; aborting, no changes`, rc=1, both repos verified untouched. Correct `ErrNoSurvivingAnchor`.
- Dirty tracked warp change + upstream advance → `warp worktree has uncommitted changes; commit or stash them, then re-run pull; aborting, no warp changes`, rc=1; `git status --porcelain` confirmed the change still present, i.e. the `reset --hard` never ran.
- Unpushed local warp commit + diverged remote → `warp remote diverged and local warp has unpushed commits; aborting, no changes`, rc=1; local commit verified still at HEAD.
- Clean fast-forward → `weft_pulled:true, warp_fetched:true`, rc=0.

**Concurrency against one live hub (hub1).**

```
8 × concurrent `lyx fabric add conc-N`   → 8/8 ok:true, rc=0
   .git/info/exclude after: 3 entries (/_board /_lyx /.lyx), zero duplicate lines
8 × concurrent `lyx fabric sync` on one pair, each writing a distinct file
   → 8/8 ok:true; weft log shows ONE "weft sync" commit
   → git show --stat: all 8 files (_lyx/raddle/n1.md … n8.md) in that one commit
   → git status --porcelain in the weft worktree: empty
6 × concurrent `lyx fabric reconcile`    → 6/6 ok:true, rc=0
```

The push coalescing lock, the weft write lock and `mutateGitExclude`'s repo-wide flock all held. No lost update, no torn exclude file, no duplicate commit.

**Invariant counts, derived from source rather than prose.**

```
$ grep -rn "gitexec\.RunGit\|r\.run(" --include=*.go internal cmd   # non-test, comment lines excluded
internal/fabricengine/weftwiring.go:73   (weftRepoExists)
internal/fabricengine/weftwiring.go:90   (weftBranchExists)
internal/gitrepo/pull.go:20              (Pull)
internal/gitrepo/pull.go:35              (Fetch)
internal/gitrepo/gitrepo.go:61           (run's own body)
=> internal/fabricengine 2, internal/gitrepo 3
```

Matches `checkedCallPinnedRawSites` exactly, and each of the five carries an adjacent `//gitexec:raw` marker with a substantive justification.
`checkedcall_test.go` asserts both the marker adjacency and the exact per-package counts (treating any unlisted package as pinned zero), and `TestCheckedCallInvariant_TokenSpellingsDoNotCollide` pins the token spellings that make the guard non-inverted — so the numbers are genuinely enforced, not decorative.

**Destruction chokepoint completeness.** Every raw `os.RemoveAll`/`os.Remove`/`fslink.Remove`/`git branch -D`/`git worktree remove`/`ResetHard`/`createdToken{` outside `destroy.go` was enumerated and cross-checked against `destructiveguard_test.go`'s allowlist: `ancestors.go`, `junction.go`, `launchers.go`, `warpprobe.go`, `index.go`, `hook.go`, `gitexclude.go` — all seven present with written per-file reasons. No new unallowlisted destructive primitive snuck in during the migration.

**Chokepoint call-site error recovery.** Every call site of `removeGitWorktree` and `deleteBranch` was read: `remove.go:205`, `prune.go:279`, `weftwiring.go:212`/`:225`, `add.go:278`/`:296`, `cleanup.go:286`, `checkout.go:203`. The two that need git's exit code and stderr to decide whether the directory-removal fallback is permitted (`remove.go`, `prune.go`) both recover via `errors.As(err, &gitErr)` and both re-check `isRegisteredLinkedWorktree` before falling back — so a gate refusal and a git refusal stay distinguishable, and neither becomes licence to delete. The rest propagate the `*GitError` unwrapped, so its stderr reaches the operator through the envelope.

**Teardown.** Both scratch hubs and all scratch temp dirs removed. `pgrep -a git` → none. No lock file survives outside a torn-down temp dir; within the hubs (before teardown) the lock files present were the expected, documented artifacts (`.weft/fabric.push.lock`, `.weft/weft.write.lock`, `exclude.lyx.lock`, `fabric-corrindex.json.lock`, `.gitrepo-push.lock`), which `lock.FileLock.Release` unlocks without deleting by design.

---

## What I could NOT verify, and why

- **Windows path and junction behavior.** Unreachable from this Linux host — `internal/fslink` uses directory junctions on Windows and symlinks here, so every junction assertion above exercised the symlink path only. This is the permanent, never-executed gap the prior campaign named every round; it remains true, and I did not fabricate coverage for it. It is a stated limit of my merge verdict, not a finding.
- **A GitHub-hosted remote.** Out of scope per the prompt, and not a real gap: every scenario above ran against local-filesystem git remotes, which exercise fabric's own logic identically. The one behavior I therefore did not observe is real network failure modes (auth rejection, timeout) inside `PushRebaseFree`/`Fetch` — but those surface as ordinary non-zero git exits through the same `*GitError` path I did drive.
- **A genuine exec-level `gitexec` failure reaching the two pinned bool predicates.** Not a gap in my testing but a structural fact I established rather than assumed: cwd resolution spawns git first and fails the command before either predicate is reached. I could not construct a CLI-reachable path to it, which is itself the answer to the prompt's question.
