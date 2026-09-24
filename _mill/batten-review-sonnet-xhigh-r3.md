# batten review — round sonnet-xhigh-r3

Follow-up campaign, round 3. Clean-room: written before consulting any prior review file except
the round prompt itself (`_mill/batten-review-prompt.md`).

## Status

Job 1 (review) COMPLETE. 4 findings (2 BLOCKING, 2 MEDIUM, 0 LOW, 0 NIT), all CONFIRMED live. Job 2
(fixing) COMPLETE — all 4 fixed, sabotage-proven, live-reverified against the real substrate. See
`_mill/batten-review-sonnet-xhigh-r3-fixer-report.md` for the fix-by-fix detail and the final
merge-readiness verdict.

## What was tested (running log)

### Read phase

Read in full: `internal/battenshed/*.go` (create.go, teardown.go, innerrun.go, seamchild.go,
deps.go, ctx.go, stuck.go, doc.go), `internal/battenrecipe/*.go` (battenrecipe.go, names.go,
doc.go), `internal/battencli/*.go` (arm.go, wire.go, cli.go, refusal.go, commitstatus.go, paths.go,
bootstrapverb.go — tests read selectively, see below), `contracts/recipes/batten-recipe.yaml`,
`CONSTRAINTS.md`, the recovered design doc (`git show 8ac857ce1~1:manifest/designs/seeded-shed.md`).

Read boundary code: `internal/fabricengine/add.go`, `remove.go`, `fabric.go` (RequireWarpWorktree/
RequireDrivableWorktree/PairSiblingRemnant), `junction.go` (WireJunctionsWith/seedLyxJunction —
this is what makes the SIGKILL-Add finding below legible), `commitweftpaths.go`
(CommitWeftPaths/CommitAnchoredPaths), `weftgit.go` (ensureWeftLockDirAt/seedWeftArtifactExcludes),
`origin.go` (ReadOrigin/originLockPath); `internal/shedrun/seed.go`, `paths.go`; `internal/shedcli/
seed.go`, `table.go`; `internal/loomcli/start.go`, `driverlaunch.go`, `driverspec.go`,
`sharedbootstrap.go`; `internal/gitrepo/gitrepo.go` (StageAndCommit).

Did not (yet, as of this checkpoint) do a line-by-line read of `internal/shuttleengine/{run,wait,
attach}.go` in full — prior rounds' focus areas covered that boundary heavily and this round's
focus is elsewhere (fabric Add/Remove crash windows, error-remedy text, wiring pins); spot-checked
via the loom start.go call sites instead. Recorded as a scope note, not a finding.

### Environment check (focus 6)

`~/.claude/plugins/installed_plugins.json` does not contain `ly@loomyard` (only `mill@millhouse`,
`codeguide@millhouse`, `python@millhouse`, `golang@millhouse`, `weblens@millhouse`,
`prowler@loomyard`). Per the round prompt, this is an environment gap: `--child-driver llm` cannot
be exercised on this host. No plugin was installed. Not treated as a defect.

### GitHub self-report hazard pre-check (BLOCKING instruction)

`grep -rn "CreateIssue\|selfreport create\|gh issue create"` across `internal/` and `cmd/`
(excluding tests) found exactly two production call sites reaching
`internal/selfreportengine.CreateIssue`: `internal/selfreportcli/cli.go` (`lyx selfreport create`,
the manual/underlying primitive both tiers ultimately go through) and `internal/loomcli/arm.go`
(`FileIssue: selfreportengine.CreateIssue`, Tier 1's own wiring). No third Go-level filing path
found. Tier 2 (`internal/frictionengine`) runs an LLM reflection pass that may itself invoke
`lyx selfreport create` as a CLI action, which is the same primitive, not a bypass.

### Fixture hub build

Built at `/tmp/batten-r3-fixture-1790250807` (outside the loomyard tree and `$HOME/Code`, never
used on this host before). `git init --bare` warp.git seeded with a minimal Go project (`go.mod`,
trivial `main.go`, `.gitignore`), empty bare weft.git. First clone attempt used warp default branch
`main`; local git's own `init.defaultBranch` is `master`, so `lyx fabric clone`'s
`suffixWeftPrimaryBranch` derived the weft prime's own initial branch from the WEFT repo's own
post-clone branch (client-config-dependent, independent of warp's branch name) — this produced a
`main`/`master-weft` mismatch and `Topology.Add`'s first parent-branch lookup refused with
"invalid reference: main-weft" on the very first task-worktree create. This is fabric clone's own
branch-naming behavior against an empty weft, not a batten defect, and fabric clone/merge
correctness is explicitly out of this round's scope — rebuilt the fixture with warp's default
branch also `master` (matching local git's default) and the mismatch did not recur. Recorded here
because it cost real setup time and a future round's fixture builder should seed the warp bare repo
with the SAME default branch name `git init` will give the empty weft, not "main" unconditionally.

Config overrides committed onto prime's weft ahead of any Board task or create, verified by grep:
`loom.yaml` `selfreport: false`, `friction: ""`; `landing.yaml` `require_pr_to_base: []`. Two Board
tasks created: `typed-loom` (`type: "loom"` explicit) and `typed-empty` (`type` omitted).

### Floor scenario: one full `--child-driver go` batten run to done

`lyx batten run typed-loom --child-driver go`, run from prime, backgrounded with output to a log
file, polled to completion via a Monitor watching the process exit. Reached `state: done` at
`Worktree-Teardown` in ~9.5 minutes wall clock (real discussion → plan → implementation → review →
landing/finalize round against a trivial fixture task, real opus[effort=high] agents per
`loom.yaml`, no mocking). Verified post-hoc: `typed-loom` worktree absent from the hub (teardown
removed it), `git status --porcelain` empty in both warp prime and weft prime, `_board` clean, zero
stray `lyx`/`tmux`/`reed`/`claude` processes left over. This is the merge-bar's normal
single-instance SUCCESS path and it is clean. No finding here.

### Sweep 1 (focus 1): SIGKILL inside fabric's `Topology.Add`, re-entered by batten

Enumerated every step boundary in `Add` (internal/fabricengine/add.go) by reading. Full table
below in the Findings section (F-SIGKILL-ADD's own table). Drove the two most informative
boundaries live, using a throwaway instrumented binary (a local, uncommitted `time.Sleep` inserted
at each boundary, reverted from the source tree immediately after each throwaway build — verified
`git status --porcelain internal/fabricengine/add.go` clean after each revert):

**Window A — SIGKILL immediately after the warp worktree+branch are created, before anything else
(`InstallPostCheckoutHook` onward: no weft worktree, no portal, no launchers, no junctions, no
origin record).**
Command: built `lyx-instrumented` with a `time.Sleep(20s)` right after
`rec.AppendRef(KindBranchCreated, ...)` in `Add`, gated on `BATTEN_R3_SIGKILL_WINDOW=1`. Ran
`BATTEN_R3_SIGKILL_WINDOW=1 lyx-instrumented batten run sigkill-add-test --child-driver go`
backgrounded; confirmed via `ls`/`git worktree list` that the warp worktree existed and the weft
sibling did not, then `kill -9` the process. Resumed with the real (uninstrumented) dev binary:
`lyx batten run sigkill-add-test --child-driver go`.

Observed: Worktree-Create's re-entry probe (`taskWorktreePresent`, wire.go) reported the worktree
"already present" (a bare `os.Stat` + `lyxcwd.ResolveWorktree`, which needs only a valid git
worktree root — it does not check for the weft sibling, junctions, portal, launchers, origin
record, or a push) and skipped calling `fabricengine.Topology.Add` entirely, reporting
Worktree-Create **Done**. Seed-Child then ran and called `childSeedParams` → `fabricengine.
ReadOrigin` → `originLockPath` → `ensureWeftLockDirAt(weftPath)`, which failed because the weft
worktree does not exist (`git rev-parse --git-path info/exclude: fatal: not a git repository`).
This is NOT one of the three `WriteSeed` refusal sentinels `childRecipeRefusal` recognizes, so
Seed-Child returned it as a **hard error**, and shedengine turned that into **StateFailed** at
Seed-Child, run halted. Confirmed permanent: re-running `lyx batten run sigkill-add-test
--child-driver go` again reproduces the identical error, forever — this run can never proceed and
the error text ("seed weft artifact excludes ... fatal: not a git repository") gives the operator
no indication that the actual cause is an interrupted create, and no remedy at all. See
F-SIGKILL-ADD below.

Side effect observed: each failed resume left a stray `<slug>-weft/.weft/` plain directory behind
(`ensureWeftLockDirAt`'s own `os.MkdirAll` runs before the git-exclude call that fails), which would
also need manual removal before a真 truly fresh create at that path, since `Topology.Add` refuses
when the weft worktree directory already exists.

Cleaned up by hand (git worktree remove --force, git branch -D, board remove, run-dir delete) —
this matches the "fabric rollback leaves debris" shape but is NOT the same known/deferred item (that
one is about `Add`'s own in-process rollback after an error it itself observed; this is about
`Add` never running its rollback at all because the process was killed, and about batten's own
re-entry probe misreading the result).

**Window B — SIGKILL after the weft worktree is created (a real, valid git repo) but before
`WireJunctionsWith` runs (no `_lyx` junction, no portal, no launchers, no origin record).**
Same method, sleep moved to just before `createPortal`, gated on `BATTEN_R3_SIGKILL_WINDOW2=1`,
fresh slug `sigkill-add-test2`. Confirmed via `ls` that the weft worktree existed (with its own
`_lyx`) and the warp side had no `_lyx` before killing.

Observed, resumed with the real binary: Worktree-Create again reported Done via the same
over-permissive probe. Seed-Child's `WriteSeed` closure this time succeeded (`ReadOrigin`'s
lock-path resolution now works, since the weft worktree is a real repo) — `shedrun.WriteSeed`'s
own `os.MkdirAll(RunDir(...))` **created a REAL, non-junction `_lyx` directory in the warp
worktree** (since no junction exists yet) and wrote `seed.json` into it. `CommitSeed` then failed
(`git add -- _lyx/shed/self/seed.json: pathspec did not match any files`, because the file is
physically in warp, not weft, where `CommitAnchoredPaths` looks) and Seed-Child reported **Stuck**
with that opaque git error as the only reason.

Confirmed the consequence live: `lyx fabric reconcile` (the documented self-healing command)
**refuses** to repair the pair — `"warp repo already contains a real _lyx at .../sigkill-add-test2/
_lyx; it predates weft — move its content into the paired weft worktree's own _lyx, or remove this
directory..."` — a message that is actively WRONG here (the directory does not predate weft; it is
batten's own accidental write seconds earlier) and gives no hint that the fix is specific to a
killed create. Followed the stated remedy verbatim (removed the fake `_lyx` directory by hand) and
confirmed `lyx fabric reconcile` then repairs the junction correctly. Also confirmed the origin/
provenance record is permanently missing for this pair (`_lyx/fabric` absent in weft) even after
the manual junction repair — a second-order consequence (the child's own loom bootstrap will later
hit its own "pass --parent once" refusal, outside batten's own recipe). Manually tore the pair down
afterward (`lyx fabric remove --force`, branch cleanup, board task removal) — zero stray processes
remained.

This is the most severe form of the same root cause (Window A): a warp-pristine-invariant
violation that `lyx fabric reconcile` cannot self-heal and actively misdiagnoses. See
F-SIGKILL-ADD.

### Sweep 3 (focus 3): production-wiring pin — reverted, tests still green

Per fix, since `d7ab9eca0` (subject lines only, read before this section; bodies not read before
this point). For each candidate I neutralised the PRODUCTION call site (not the helper under test)
and ran `go build ./...`, `go test ./internal/battencli/... ./internal/battenshed/...`, and
`go test -tags integration -count=1 ./internal/battencli/...`, then restored the file from a
pre-edit backup and verified `git status --porcelain` clean before moving to the next.

- **`internal/battencli/wire.go`'s `CreateWorktree` closure, `return createRefusal(err)`** (r2 F2 —
  "Worktree-Create words its own leftover-branch remedy"). Reverted to `return err`. Result: `go
  build` ok, untagged battencli tests **all pass**, `-tags integration` **all pass**. Confirmed:
  `createRefusal` is tested only as a bare function (`TestCreateRefusal_LeftoverBranchRemedyNever
  NamesCheckout`, wire_test.go) — no test exercises `env.CreateWorktree`'s own error path with a
  real or stubbed `ErrBranchExists`. **Wiring not pinned.** See F-WIRING-1.

- **`internal/battencli/wire.go`'s `SeedChild.WriteSeed` closure, the `errors.Is(err, shedrun.
  ErrDisagreeingSeed)` → wrap into `battenshed.ErrDisagreeingChildSeed` branch** (part of the
  disagreeing-child-seed handling, r1's own hardening). Reverted to a bare
  `if err := shedrun.WriteSeed(...); err != nil { return err }`. Result: `go build` ok, untagged
  and `-tags integration` battencli tests **all pass**. Confirmed: `TestSeedChild_
  DisagreeingChildSeedIsStuck` (battenshed/seamchild_test.go) stubs `deps.WriteSeed` to already
  return the wrapped sentinel and so never exercises wire.go's own rewrap; no integration test
  writes a real disagreeing child seed and drives Seed-Child over it. **Wiring not pinned.** See
  F-WIRING-2.

Checked and found PROPERLY pinned (production closure itself exercised, not just a helper):
`Worktree-Teardown`'s idempotent-absence and half-torn-pair refusal (`TestWire_
TeardownIsIdempotentAgainstAnAlreadyRemovedWorktree`, `TestWire_TeardownRefusesAHalfTornPair` call
`c.env.Teardown.Shutdown`/`Remove` directly); the done-slug remedy (`TestRunCmd_
StateDoneRefusesNamingTheRunDir` drives the real `run` command's `PreRun` hook via
`clihelp.Execute`); shedcli's batten-prime-only seed rule (`TestWriteSeed_
BattenSeedIsRefusedOutsidePrime`, seed_integration_test.go, calls `writeSeed` against the real
global `recipes` table with `battencli.RefuseUnlessPrime` wired in); shedcli's fabric-checkout
refusal (`TestWriteSeed_RefusesFabricsOwnCheckouts` calls `writeSeed` directly).

Not yet checked with a revert experiment (time-boxed; recorded as a gap): the loomcli/loomshed
fixes in the same git-log window (r1 F2, F4) — those are outside batten's own three packages and
this round's wiring-pin sweep prioritised batten-owned fixes given the scope and time budget.

(Log continues below as more scenarios are run.)

## Findings (provisional — being filled in as testing proceeds)

Severity scale: BLOCKING / MEDIUM / LOW / NIT. CONFIRMED = reproduced live. PLAUSIBLE = reasoned
from code reading, not yet reproduced.

### F-SIGKILL-ADD (BLOCKING, CONFIRMED) — Worktree-Create's re-entry probe accepts a SIGKILL-interrupted `Add` as complete

`internal/battencli/wire.go:71-102` (`taskWorktreePresent`), consumed by `wire.go:205-215`
(`CreateWorktree` closure).

`taskWorktreePresent` treats "the target path resolves as a git worktree" (a bare `os.Stat` +
`lyxcwd.ResolveWorktree`, which needs nothing beyond a valid `.git` and the hub's own recorded
anchor) as equivalent to "the task worktree's post-condition — the WHOLE pair fabric's `Add`
creates — is satisfied." `Topology.Add` is an ~11-step transactional sequence (warp worktree+branch
→ weft worktree+branch → portal → launchers → junctions → origin record+commit → warp push → weft
push) whose own in-process rollback (`rollbackAdd`) never runs at all when the process is killed —
exactly the class of defect this round's focus 1 names. A SIGKILL any time after the warp worktree
is created but before `Add` returns leaves `taskWorktreePresent` reporting `true`, so the re-entered
Worktree-Create row skips `Add` entirely and reports **Done** over a pair that is not actually
built.

Two live-reproduced consequences, depending on exactly when the kill lands (see the "What was
tested" log above for full transcripts):

- **Before the weft worktree exists**: Seed-Child's own `ReadOrigin` call fails trying to resolve a
  lock path inside a weft worktree that is not a git repository. This is a hard error (not one of
  the three sentinels `childRecipeRefusal` recognizes), so shedengine reports **StateFailed** and
  the run is **permanently wedged** — every resume reproduces the identical opaque
  `fatal: not a git repository` error, with no remedy text and no indication the root cause is an
  interrupted create.
- **After the weft worktree exists but before junctions are wired**: `shedrun.WriteSeed`'s
  `os.MkdirAll` creates a REAL (non-junction) `_lyx` directory inside the warp worktree — the exact
  "warp repo already contains a real directory predating weft" state `seedLyxJunction`'s own guard
  exists to catch, except this one is batten's own accidental write, not user content. Seed-Child
  reports Stuck with an opaque git-plumbing reason (`pathspec did not match any files`, because the
  commit step looks for the file in weft, where it never landed). Confirmed live: `lyx fabric
  reconcile`, the documented self-healing command, **refuses** to repair the pair
  ("...it predates weft — move its content... or remove this directory...") — a message that is
  factually wrong here and gives the operator zero indication the fix is specific to a killed
  create. The origin/provenance record is also permanently missing for this pair even after a
  manual junction fix.

Neither of batten's own existing remedy texts (the absent-pair remedy in `taskWorktreeLocation`,
the leftover-branch remedy in `createRefusal`) covers this state, because the worktree is neither
fully absent nor does `Add` ever return an `ErrBranchExists` — `Add` is never even called.

**Fix direction** (for Job 2): `taskWorktreePresent`'s notion of "present" needs to verify enough
of the pair's own post-condition to distinguish a genuinely-finished create from a SIGKILL-torn one
— at minimum that the weft worktree and the warp `_lyx`/`.lyx` junctions exist — and refuse with a
clear, on-its-own-terms remedy (naming the incomplete pair and the manual cleanup: remove the
orphaned warp worktree and its local branch, then resume) rather than silently reporting Done. This
extends the exact idempotency-probe pattern the module already uses elsewhere (the absent-pair
remedy, the half-torn-teardown remedy) — same shape, not a new mechanism — so it is in scope for an
inline fix, not a NOT-FIXED-THIS-ROUND item.

### F-WIRING-1 (MEDIUM, CONFIRMED) — `createRefusal`'s production call site is not test-pinned

`internal/battencli/wire.go:224` (`return createRefusal(err)` inside the `CreateWorktree` closure).
Reverting this one line to `return err` leaves `go build`, every untagged `battencli` test, and
`-tags integration` all green. `createRefusal` itself is well-tested
(`TestCreateRefusal_LeftoverBranchRemedyNeverNamesCheckout`), but nothing exercises the closure
that is supposed to call it. A future edit that silently drops the call (e.g., during an unrelated
refactor of `CreateWorktree`) regresses fabric's raw, wrong-from-prime "lyx fabric checkout" advice
back onto the operator with no test catching it. Fix: add a test that drives `env.CreateWorktree`
itself with a stubbed/real `ErrBranchExists` and asserts the reworded text comes out the other end
— the same shape `TestWire_TeardownRefusesAHalfTornPair` already uses for the sibling closure.

### F-WIRING-2 (MEDIUM, CONFIRMED) — the disagreeing-child-seed rewrap's production call site is not test-pinned

`internal/battencli/wire.go:404-406` (the `errors.Is(err, shedrun.ErrDisagreeingSeed)` branch
inside the `SeedChild.WriteSeed` closure). Reverting this branch to a bare error pass-through
leaves every battencli test, untagged and `-tags integration`, green. `TestSeedChild_
DisagreeingChildSeedIsStuck` (battenshed) stubs the already-wrapped sentinel directly and so never
touches this rewrap. Consequence if it silently regressed: a genuinely disagreeing pre-existing
child seed would surface as a hard error (StateFailed) instead of Stuck, a real behavioural
regression matching this round's focus-3 shape exactly. Fix: extend
`TestWire_WriteSeedRefusesANonLoomChildBeforeTouchingTheWorktree`'s style to cover this case, or add
a sibling test that writes a real disagreeing seed via `shedrun.WriteSeed` first and then calls
`c.env.SeedChild.WriteSeed` and asserts `errors.Is(..., battenshed.ErrDisagreeingChildSeed)`.

### Sweep 4 (focus 4): adversarial pressure on the youngest fixes

**Half-torn-pair refusal at teardown (r2 F1), followed end to end.** Created a pair directly
(`lyx fabric add half-torn-test`), hand-seeded a batten status at `Worktree-Teardown`/`running`
(the durable status format supports resuming mid-graph by design), then simulated a `Remove`
interrupted between its two halves (`git worktree remove --force` on the warp side only, weft
sibling left on disk). `lyx batten step half-torn-test` correctly refused, naming
`lyx fabric prune --apply` as the remedy. Ran it verbatim; `lyx batten step half-torn-test` then
completed the row (`outcome: done`). **Confirmed correct, no finding.**

**Leftover-branch remedy at create (r2 F2), followed end to end after a real teardown.** Created and
force-removed a pair (`lyx fabric add`/`fabric remove --force leftover-branch-test`, which leaves
the warp branch behind by fabric's own design). `lyx batten run leftover-branch-test --child-driver
go` correctly hit the reworded `createRefusal` text, naming the branch and both its local and remote
deletion commands plus `lyx fabric cleanup --apply --remote` for the weft sibling. Followed the
first two verbatim (`git branch -D`, `git push origin --delete`), then ran `lyx fabric cleanup
--apply --remote` for the weft sibling as the text says — **it silently did nothing for this
branch**, see F-CLEANUP-REMOTE-ORPHAN below (found via this exact adversarial drive, not
speculatively).

**Done-slug remedy (r2 F4), followed end to end and the slug re-run to done.** Reused the
already-completed `typed-loom` from the floor scenario. `lyx batten run typed-loom --child-driver
go` correctly refused with the done-slug text, naming the run directory and "local and remote"
branch cleanup, with `lyx fabric cleanup --apply --remote` again named as the weft-sibling remedy.
Followed the full remedy verbatim (deleted the run directory via a commit on the weft sibling,
`git branch -D typed-loom`, `git push origin --delete typed-loom`, `lyx fabric cleanup --apply
--remote`) and re-ran the slug: **Worktree-Create failed** with a raw, unrelated-looking git error
(`git push -u origin typed-loom-weft ... ! [rejected] ... non-fast-forward`), because the remedy's
`cleanup --apply --remote` step had, again, silently done nothing for the stale `typed-loom-weft`
remote branch. The failure also triggered `Add`'s own accepted, known rollback limitation (a fresh
`typed-loom` warp branch left behind, logged as
`"rollbackAdd's warp-branch deletion was refused by the destructive gate"` — matches the known
fabric-rollback deferred item, not re-reported as new). Diagnosed and fixed by hand
(`git push origin --delete typed-loom-weft` directly against the weft sibling, plus a second
`git branch -D typed-loom` for the rollback's own leftover); a third attempt was launched
(backgrounded, watched to completion) and **reached `done`** cleanly at `Worktree-Teardown` (~12
minutes wall clock for the full discussion → plan → implementation → review → landing/finalize
cycle a second time). Verified afterward: the `typed-loom` worktree is gone, `git status
--porcelain` is empty in both warp and weft prime, and zero stray processes remain. This confirms
the underlying full-lifecycle machinery is sound once the stale-remote-branch obstacle is cleared by
hand — the defect is squarely in the remedy text's own completeness, not in the rest of the
lifecycle. This is the second live confirmation of F-CLEANUP-REMOTE-ORPHAN, via the done-slug
remedy rather than the leftover-branch one — same root cause, two remedy texts.

**`lyx shed seed`'s per-recipe location rule, every checkout kind, both recipes.** `RefuseSeedAt` is
`nil` for `loom` (seeds wherever its verbs drive, confirmed by
`internal/shedcli/seed_test.go`'s `TestWriteSeed_RecipeLocationRuleGatesTheWrite`/`NilRuleWrites`
and the integration test's own `writeSeed(taskLocation, ..., RecipeLoom, ...)` case) and
`battencli.RefuseUnlessPrime` for `batten` (prime-only). Live-drove the batten case directly:
`lyx shed seed <run> --recipe batten` from the weft prime and from `_board` both refuse with the
same `RefuseUnlessPrime` text already confirmed correct under the Batten Bookend checks below; from
a task worktree it refuses via the prime-name comparison (already proven by the existing
integration test read during the wiring-pin sweep above, `TestWriteSeed_BattenSeedIsRefusedOutsidePrime`).
No live gap found in this rule.

**Batten Bookend refusals, all three non-prime standing points.** Drove `lyx batten status
typed-loom` from: the weft prime (`warp-weft`) — refused, correctly named "the weft sibling of a
pair, not a warp worktree"; the `_board` checkout — refused, correctly named "the hub's _board
checkout, not a warp worktree"; and from inside the task worktree itself (`typed-loom`, materialized
mid-rerun) — refused, correctly named "typed-loom" is not the prime worktree ("warp" is). All three
texts are accurate and none suggests a wrong-from-there remedy. **Confirmed correct, no finding.**

**`PrimeRunLock` scope across two slugs.** Created two fresh Board tasks (`concurrent-a`,
`type: "loom"`; `concurrent-b`, `type` omitted) and fired `lyx batten step concurrent-a
--child-driver go` and `lyx batten step concurrent-b --child-driver go` as two background processes
started within the same shell command (true OS-level concurrency, not sequential). One
(`concurrent-a`) won the race and created its worktree; the other (`concurrent-b`) correctly
detected `PrimeRunLock` contention and reported Stuck naming the lock path, rather than racing fabric
or corrupting either slug's state. Resuming `concurrent-b` then succeeded. Stepped both through
Seed-Child and confirmed **zero cross-contamination**: each worktree, branch, and child seed
belonged to the correct slug. This also doubled as the type-default check: `concurrent-b`'s
(`type` omitted) child seed came out `{"recipe":"loom", ...}`, byte-identical in shape to
`concurrent-a`'s explicit `type: "loom"` — **confirmed correct, no finding.** Both slugs abandoned
via direct `lyx fabric remove --force --remote` (avoiding a second/third real child driver spawn,
since the full-lifecycle machinery was already proven twice via `typed-loom`); passing `--remote`
explicitly here DID delete both weft siblings' remote copies cleanly, which is itself supporting
evidence for F-CLEANUP-REMOTE-ORPHAN's fix direction below (fabric's own `remote` flag works
correctly when a caller actually passes `true` — batten's own teardown just never does).

**`--driver llm`/`--child-driver llm` flag-level refusals**, both surfaces: `lyx batten run
some-fresh-slug --driver llm` refuses ("batten has no bootstrap verb"); `lyx shed seed some-slug
--recipe batten --driver llm` refuses with the shedcli-level wording. Both correct, no remedy
implied or needed (flat impossibilities). **Confirmed correct, no finding.**

## Findings (continued)

### F-CLEANUP-REMOTE-ORPHAN (BLOCKING, CONFIRMED) — `lyx fabric cleanup --apply --remote`, the remedy both `createRefusal` and `doneSlugRefusal` name for the weft sibling branch, never sees the branch it is supposed to clean up

`internal/battencli/wire.go`'s `Teardown.Remove` closure calls `top.Remove(location, slug, false,
false)` — always `remote: false`. `fabricengine.Topology.Remove` (`remove.go`) always deletes the
weft branch LOCALLY regardless of the `remote` flag (only whether it is ALSO deleted on the origin
remote depends on that flag) — so every batten-driven teardown leaves the task's weft branch
present on the remote and ALREADY ABSENT locally.

`internal/fabricengine/cleanup.go`'s `Topology.Cleanup` (the implementation behind `lyx fabric
cleanup`, which BOTH `createRefusal` (wire.go, r2 F2) and `doneSlugRefusal` (arm.go, r2 F4) name as
the way to remove "an orphaned sibling branch") enumerates ONLY the weft repo's own **local**
branches (`listWeftBranches` → `git branch --format=...`, no `git ls-remote` or fetch of any kind).
A weft branch that is already gone locally — which is the state EVERY normal batten teardown leaves
— is therefore invisible to `lyx fabric cleanup` in every mode, `--apply --remote` included: it is
never enumerated, so `deleteWeftBranchOnRemote` (which only runs "after `deleteWeftBranch` has
already returned true for the SAME branch") never even attempts the remote side either.

**Confirmed live, twice, via two different remedy texts** (see Sweep 4 above for full transcripts):
following `createRefusal`'s remedy for `leftover-branch-test` and `doneSlugRefusal`'s remedy for
`typed-loom`, `lyx fabric cleanup --apply --remote` reported nothing for either slug's weft branch,
which remained on the remote. For `typed-loom` this directly caused a SECOND failure on the
documented "run it again" path: `Worktree-Create`'s own weft-branch push was rejected
non-fast-forward against the stale remote branch, surfaced as raw git plumbing text with a
misleading `git pull` hint, plus a fresh leftover-warp-branch from `Add`'s own accepted rollback
limitation stacking on top. The only working fix in both cases was a manual
`git push origin --delete <slug>-weft` against the remote directly — an action neither remedy text
names.

**Fix direction** (scoped to batten's own package, in scope for an inline fix): `Teardown.Remove`
should pass `remote: true` to `top.Remove` — nothing else in the pair's lifecycle will ever use that
weft branch again once batten tears the task down, `Remove`'s own doc comment already states a
remote-deletion failure never fails the call, and the two-slug interleave test above independently
confirmed `remote: true` deletes cleanly when a caller actually passes it. This does not fix
`fabricengine.Cleanup`'s own local-only enumeration (a real, separate gap: a weft branch stranded on
the remote by any OTHER path — a failed push during a `remote: true` teardown, or a hand-run
`fabric remove --force` with no `--remote` — is still permanently invisible to `lyx fabric
cleanup`), but it removes the defect from batten's own normal, common-case teardown, which is what
both remedy texts are actually written to describe. Extending `Cleanup`'s own enumeration to reach
remote-only orphans (via `git ls-remote`) is a materially larger, network-aware change to
`fabricengine`, and is recorded as **NOT-FIXED-THIS-ROUND** — its own task, not a scoped inline fix,
though it is the more complete resolution and I record it as a punch-list item below.

## Focus-1 table: every step boundary in `Topology.Add`/`Topology.Remove`

Enumerated by reading `internal/fabricengine/add.go`/`remove.go` sequentially (each numbered step
in the source, cross-checked against `grep -n "rollbackAdd\|^func" internal/fabricengine/add.go`,
which shows every step from weft-worktree-creation through the final weft push is guarded by
`rollbackAdd` when `Add` observes its OWN error — the gap this table is about is specifically what
a SIGKILL bypasses, since a killed process never reaches any of those calls at all). This
enumeration cannot see: a SIGKILL landing mid-syscall inside a single `git` invocation itself (e.g.
half-written refs) — out of reach for a Go-level source read, and not distinguished from "the git
call fully completed" in the table below; nor can it see reed/tmux state, which is Remove's own
concern only at the Shutdown half of batten's Worktree-Teardown, not Add's.

### `Topology.Add`

| # | Step (add.go) | On-disk state after a SIGKILL landing right after this step | batten's re-entered Worktree-Create row | Verdict |
|---|---|---|---|---|
| 0-8 | Validation, dirty check, branch-name compute, existence probes, parent-branch resolve (lines 70-146) | No mutation of any kind — all reads | Re-runs `Add` from scratch, identical to a fresh create | Safe — CONFIRMED (no live drive needed; provably read-only) |
| 9 | `createGitWorktree` (warp worktree + branch) (149-157) | Warp worktree present, git-registered, on a new local-only (unpushed) branch. No weft worktree, no `_lyx`/`.lyx` junctions, no portal, no launchers, no origin record. | `taskWorktreePresent` → **true** (bare `os.Stat`+`ResolveWorktree`, no pair/junction check) → **skips `Add` entirely, reports Done** | **BLOCKING, CONFIRMED live** (Window A). Seed-Child's `ReadOrigin` hits a non-existent weft repo → hard error → `StateFailed`, permanently wedged. See F-SIGKILL-ADD. |
| 10 | `InstallPostCheckoutHook` (162-164), non-fatal even on its own failure | Same as step 9 plus maybe a hook file | Same as step 9 | Same as step 9 |
| 11 | Weft worktree create/adopt (166-187) | Weft worktree now present (a real, valid git repo with its own `_lyx`). Warp side still has no `_lyx`/`.lyx` junction. No portal, no launchers, no origin record. | `taskWorktreePresent` → **true**, skips `Add`, reports Done | **BLOCKING, CONFIRMED live** (Window B, killed just after this step). Seed-Child's `WriteSeed` now succeeds, but `os.MkdirAll` creates a REAL (non-junction) `_lyx` in warp — warp-pristine invariant violated. `CommitSeed` fails opaquely; `lyx fabric reconcile` refuses to repair it, misdiagnosing the directory as user content. See F-SIGKILL-ADD. |
| 12 | `createPortal` (189-192) | Adds a portal junction on top of step 11's state | Same as step 11 | Same as step 11 (PLAUSIBLE by extension, not independently re-driven — the portal's presence/absence does not change Seed-Child's own path) |
| 13 | `writeLaunchers` (194-197) | Adds launcher scripts on top of step 12's state | Same as step 11 | Same as step 11 (PLAUSIBLE by extension) |
| 14 | `RepoWiredNames` + `WireJunctionsWith` (199-213) — wires `_lyx`/`.lyx` and seeds `.git/info/exclude` | A SIGKILL mid-step can leave one junction wired and the other not, or a junction wired but its exclude entry unseeded | `taskWorktreePresent` still doesn't check junction state at all; Done regardless | **PLAUSIBLE, not live-driven** (time-boxed): if `_lyx` ends up wired before the kill, Seed-Child's write lands correctly (no corruption) but an unseeded exclude entry would make the junction show as untracked "dirt" in `git status`, a condition several of fabric's own dirty-worktree gates treat as a hard refusal. Reasoned from `seedLyxJunction`'s/`seedGitExclude`'s own per-junction-then-excludes ordering, not reproduced with a real kill. |
| 15 | `WriteOrigin` + `CommitWeftPaths` (215-228) | Junctions ARE wired by this point. `origin.json` may be written to disk but its commit interrupted, leaving it as an untracked/uncommitted file in the pair's weft worktree | `taskWorktreePresent` → true, Done. Seed-Child's `ReadOrigin` reads the uncommitted file directly off disk (state.ReadJSON has no commit-awareness) and gets a valid `ParentBranch` | **PLAUSIBLE, not live-driven**: the run itself likely proceeds normally (Seed-Child succeeds, child seed gets its `parent` param), but the origin record stays permanently uncommitted since nothing later in batten's own flow re-commits it — a latent dirty-weft-worktree hazard for a LATER `lyx fabric remove` on this same pair (`refuseDirtyWeftWorktree`'s own no-force gate). Lower severity than 9/11; not independently confirmed. |
| 16 | Push warp branch (231-235) | Warp branch may be unpushed | Done, proceeds normally otherwise | **PLAUSIBLE, low severity**: the task's own warp work is machine-local until something else pushes it; loom's own later landing/publish flow typically pushes branches as part of its ordinary operation, which would self-heal this in the common case. Not independently confirmed. |
| 17 | Push weft branch (238-241) | Weft branch may be unpushed | Done, proceeds normally | **PLAUSIBLE, low severity**: batten's own `CommitStatus` seam pushes the pair on every non-no-op transition, which self-heals this within one bounce. Not independently confirmed. |

### `Topology.Remove`

| # | Step (remove.go) | On-disk state after a SIGKILL landing right after this step | batten's re-entered Worktree-Teardown row | Verdict |
|---|---|---|---|---|
| 1-5 | Validation, prime-slug refusal, target-exists check, merge-in-progress checks (64-96) | No mutation | Re-enters cleanly | Safe |
| 6 | `removePortal` + `removeLaunchers` (101-106) | Portal/launcher entries gone; both worktrees still fully present | `taskWorktreePresent` → true; `Shutdown` and `Remove` both re-attempt portal/launcher removal, which is individually idempotent-absent (each helper's own no-op-if-missing contract, per this file's own header comment) | **Safe by reading** — self-healing by construction, consistent with the file's own stated design ("teardown still runs when the worktree directory is already gone"). Not independently re-driven live (time-boxed) but the idempotency is structural, not incidental. |
| 7 | No-force dirty gates (108-122) | No mutation (refusal path only) | N/A — this is an ordinary refusal, not a crash window | Out of scope for this sweep |
| 8 | Junction sweep: `scanOnDiskJunctionNames` + `removeWarpJunction` (124-138) | A SIGKILL mid-sweep can leave `_lyx` unwired while `.lyx` still is, or vice versa | `taskWorktreePresent` → true (still resolves; junction state irrelevant to that probe). `Shutdown` calls `reedengine.LoadConfig(taskLocation.AnchorPath(), "reed")`, which reads through the (now possibly-missing) `_lyx` junction | **PLAUSIBLE, not live-driven** (time-boxed): if `_lyx` was the one unwired, `Shutdown`'s own config load plausibly fails, reporting Stuck — and since `Remove` never runs until `Shutdown` succeeds (the Bookend ordering this row exists to enforce), the row could wedge permanently on a state `Remove` itself would happily clean up. This is the single most consequential PLAUSIBLE-not-CONFIRMED gap left by this round's time budget; flagged for a follow-up round's live drive rather than asserted as fact. |
| 9 | `removeWarpWorktreeDir` (139-141) | Warp worktree possibly partially removed by an interrupted `git worktree remove` | `taskWorktreePresent` → false (once the directory is actually gone) or true (if git left enough behind to still resolve) | **PLAUSIBLE, not live-driven**: ordinary git-worktree-removal atomicity question, not batten-specific; not pursued further given time budget. |
| 10 | `removeWeftWorktree` (147-155) | Warp side fully gone, weft sibling still on disk — the canonical half-torn pair | `taskWorktreePresent` → false; `PairSiblingRemnant` → true; refuses by name, pointing at `lyx fabric prune --apply` | **CONFIRMED correct, live-driven** (Sweep 4 above, r2 F1). |

## Focus-2 table: every error text batten surfaces, and whether its remedy works from prime

Enumerated by reading every `fmt.Errorf`/`reportStuck` call in `internal/battenshed/*.go` and
`internal/battencli/{arm,wire,refusal,cli}.go`, plus every error `wire.go`'s closures pass through
unchanged from `fabricengine`/`shedrun`/`boardengine`/`lyx loom start`. This enumeration cannot see:
errors from packages batten calls into but that this table did not itself open line-by-line in full
(`gitrepo`, `reedengine`'s own internals) — those surface only as opaque wrapped text in the two
CONFIRMED findings above (F-SIGKILL-ADD, F-CLEANUP-REMOTE-ORPHAN), which IS the point being made:
batten's own pass-through discipline is fine, the texts it passes through are sometimes wrong for
the state they actually describe.

| Text (site) | Names a remedy? | Remedy | Standing point | Works from there? |
|---|---|---|---|---|
| Non-prime refusal (`refusal.go`) | No command, just "re-run from there" | Re-run from prime | Weft prime, `_board`, task worktree | **Yes — CONFIRMED live**, all three standing points |
| Prime-name-unresolvable refusal (`refusal.go`) | No | — | Anywhere the hub geometry is broken | Not driven (needs a broken hub geometry, out of scope to manufacture) |
| Self-address refusals (`arm.go` `refuseSelfAddress`) | Yes — "pass the slug" / names the reservation | Type the slug | Prime | **Yes — matches cobra's own arity contract**, verified by reading `cli_test.go`'s arity tests; not independently re-driven live (low-risk, purely textual) |
| `refuseAdoptedSeed` (driver/recipe disagreement) (`arm.go`) | Yes — "drive it with `lyx shed run`" / "delete its seed to re-seed" | Two alternatives | Prime | Not independently driven live this round (unchanged since earlier rounds, no fix landed on it since `d7ab9eca0`, out of this round's wiring-pin sweep) |
| `refuseBattenOwnDriverLLM` | N/A (flat impossibility) | — | Prime | **Yes — CONFIRMED live** (`--driver llm` refused) |
| `taskWorktreeLocation`'s absent-pair refusal (`wire.go`) | Yes — two-branch remedy (restore by hand, or abandon: delete run dir + both branches) | Manual restore or full abandon | Prime | Reasoned correct by reading + the existing `TestTaskWorktreeLocation_AbsentPairIsNamed` unit coverage; not independently re-driven live this round (this exact text did not change since `d7ab9eca0`) |
| `createRefusal`'s `ErrBranchExists` rewording (`wire.go`) | Yes — delete branch local+remote, `lyx fabric cleanup --apply --remote` for the sibling | Three commands | Prime | **Local+remote warp branch deletion: CONFIRMED works. The `lyx fabric cleanup --apply --remote` step: CONFIRMED DOES NOT WORK** for the common case (F-CLEANUP-REMOTE-ORPHAN) |
| `PrimeLock` contention (`create.go`/`teardown.go` `reportStuck`) | No command, names the lock path | Wait and resume | Prime | **Yes — CONFIRMED live** via the PrimeRunLock two-slug interleave |
| `childRecipeRefusal`'s three sentinels (`seamchild.go`) | Two of three name remedies (unsupported recipe names the one legal value; disagreeing seed names delete-or-correct informally, no exact command); unknown recipe just lists valid names | Fix the Board task's type, or fix/delete the child's own seed | Prime | Not independently driven live this round (unchanged text, low risk — these are Board-data-correction remedies, not fabric commands that could be wrong-from-there) |
| Seed-Child hard errors: `ReadBoardType`/`ChildDriver`/unwrapped `WriteSeed` failures | No | — (mechanism failures by design) | Prime | **The unwrapped `WriteSeed` failure path is exactly where F-SIGKILL-ADD's opaque git error surfaces** — see above |
| `innerRunProducer`'s halted-child remedy (`innerrun.go` `haltedChildRemedy`) | Yes — "resume from inside that worktree ... e.g. `lyx loom start`" | Attach/resume inside the child | Prime (the text explicitly says go elsewhere) | Reasoned correct by reading; not independently re-driven live this round (would need to manufacture a genuinely blocked/failed/paused child, a larger live scenario than this round's time budget covered — flagged as a gap, not asserted safe) |
| `worktreeTeardownProducer`'s Shutdown/Remove Stuck reasons (`teardown.go`) | Shutdown: none (names which half failed); Remove: none directly, but the closure's own `PairSiblingRemnant` branch does | `lyx fabric prune --apply` | Prime | **CONFIRMED live** (Sweep 4, half-torn-pair) |
| `doneSlugRefusal` (`arm.go`) | Yes — delete run dir, delete branches local+remote, `lyx fabric cleanup --apply --remote` | Same three-part remedy as `createRefusal` | Prime | **Delete run dir + local/remote warp branch: CONFIRMED works. `lyx fabric cleanup --apply --remote`: CONFIRMED DOES NOT WORK** — same root cause as `createRefusal`'s case, independently reproduced (F-CLEANUP-REMOTE-ORPHAN) |
| `PauseAbsentMessage`/`MissingSeedMessage` (`arm.go`/`shedrun`) | Yes — "run `lyx batten run <slug>` first" | Start the run | Prime | **Yes — CONFIRMED live** |
| Child bootstrap spawn failure (`childSpawnError`, wire.go) | No (folds child's own diagnosis in, no remedy of its own) | — | Prime | Not independently driven live (would need a genuinely failing child bootstrap; unit-tested via `TestChildSpawnError`) |
| `lyx shed seed`'s fabric-checkout refusal (`shedcli/seed.go`) | No command, states the refusal | Seed from a drivable worktree | Weft prime / task worktree's other side | **Yes — CONFIRMED by the existing `TestWriteSeed_RefusesFabricsOwnCheckouts`** (read during the wiring-pin sweep), consistent with my own live Bookend drives against the same refusal chain |
| `lyx shed seed`'s batten-prime-only refusal (`shedcli/table.go` → `battencli.RefuseUnlessPrime`) | No command, states the refusal | Seed from prime | Task worktree | **Yes — CONFIRMED by the existing integration test** (`TestWriteSeed_BattenSeedIsRefusedOutsidePrime`, read during the wiring-pin sweep) |

## Sweep 5 (focus 5): standard re-confirmation of the CLOSED-AND-VERIFIED list

Read AFTER my own findings above were complete, per the round prompt's clean-room release:
`_mill/batten-review-HANDOFF.md` (this follow-up campaign) and
`_mill/batten-end-to-end/batten-review-HANDOFF.md` (the predecessor). Not opened before this point:
`_mill/batten-review-precount.md`, any `_mill/**/batten-review-*` file other than the prompt.

**Both of this round's headline findings turn out to be exactly this round's seeded focus, already
half-diagnosed by the orchestrator before I started:**
- The follow-up HANDOFF's R2 entry states, verbatim: *"Orchestrator-found candidate, not driven
  (seeded as R3 focus 1): R2's crash-window table says a kill inside `Topology.Add` is rolled back;
  under SIGKILL the in-process rollback never runs, and `taskWorktreePresent` probes the warp alone,
  so a re-entered create likely reports Done over a half-created pair — the create-side mirror of
  F1."* This is F-SIGKILL-ADD, reasoned but not live-driven by the orchestrator; this round supplies
  the live SIGKILL reproduction (twice, at two different boundaries) and the fix.
- The same entry also states: *"Orchestrator-found wiring gap in F2 (seeded as R3 focus 3):
  `fabricengine.Add` returning an untyped error with the same text leaves every test green,
  `-tags integration` included... The fix works today; nothing stops it being reverted silently."*
  This is F-WIRING-1, independently reproduced this round via an actual revert-and-test experiment
  (the orchestrator's note reasoned about it but did not run one).

No CLOSED-AND-VERIFIED item regressed. Spot-checked against this round's own live drives and reads:
- Follow-up R1's F1 (Run-Shed retries a failed-after-seeded bootstrap): `innerrun.go`'s
  `spawnConfirmed`/re-spawn logic read in full this round, unchanged since the fix, still exercised
  by `TestInnerRun_SpawnsUntilASpawnIsConfirmed`/`TestInnerRun_FailedSpawnIsRetriedOnTheNextCall`.
- Follow-up R1's F3 (absent-worktree advice) / R2's F1 (half-torn-pair refusal) / R2's F4 (done-slug
  remedy naming the whole abandon path): all three **re-driven live this round** (Sweep 4 above),
  not merely re-read — no regression, and the done-slug remedy's own incompleteness
  (F-CLEANUP-REMOTE-ORPHAN) is a genuinely new gap in what the remedy CLAIMS, not a regression of
  what R2 fixed (R2's own fix scope was the run-directory-and-branches text, which is correct; it
  never claimed to fix `lyx fabric cleanup`'s own remote-branch enumeration).
- Follow-up R2's F3 / R1's F7 (shedcli seed location rules): **re-driven live this round**
  (`lyx shed seed --recipe batten` from all three non-prime standing points), no regression.
- The GitHub self-report hazard (Tier 1 + Tier 2, the two-incident history in the predecessor's
  HANDOFF): re-checked this round's own grep sweep found no third path; the fixture hub's
  `selfreport: false`/`friction: ""` override was verified by grep before the first Board task,
  per the standing rule. Zero GitHub issues filed this round (not independently checked against the
  live tracker — no `gh` credential probe performed — but no code path in this round's drives ever
  reached a `selfreportengine.CreateIssue` call site, and both gate keys were confirmed off).

**Deferred items, re-evaluated:**
- Cold-machine recreate-from-branch (fabric `Topology.Add` refuses a pre-existing branch by
  design): still holds, `Add`'s own `ErrBranchExists` check is unchanged.
- `step`-mode pacing (30s poll sleep, cancellable, still elapses once per `step` call): still holds,
  `innerRunProducer.Call`'s single `p.deps.Sleep` call per invocation is unchanged.
- The design doc's two shipped residuals (dead driver strand undetected; a cleanly-finished driver's
  strand/run-dir torn down only at whole-worktree teardown): re-read `battenshed/doc.go` this round
  (see the Read phase above) — text is still accurate, no code change of mine touches either
  residual's own mechanism.
- GitHub issue #263: not touched, not re-reported.
- Teardown ending loom's post-run friction reflection: not driven, not re-reported, per the round
  prompt's own explicit instruction (Tier 2 files real issues) — `battenshed/doc.go`'s existing text
  on this point was re-read and is unchanged.
- "No `ly` plugin on this host": re-confirmed at the top of this log (Environment check).
- "Fabric rollback leaves the warp branch behind": **hit repeatedly this round** (every `Add`
  rollback after a SIGKILL-induced failure, and the rejected-push retry during the done-slug remedy
  drive) — each instance logged the same `"rollbackAdd's warp-branch deletion was refused..."`
  warning and was cleaned up by hand, per the deferred item's own instruction; not re-reported as
  new, not fixed in fabric.

## Scope assessment

Compared the shipped code against the recovered design doc (`manifest/designs/seeded-shed.md`,
`git show 8ac857ce1~1`). The implementation matches the design's stated intent closely: the
Shed-is-the-only-execution-machinery concept, run addressing (`self` default, durable `_lyx/shed/`
vs ephemeral `.lyx/shed/`), the seed contract (recipe/driver/params, closed vocabularies owned by
`shedrun`), the Driver Choice Single-Site Invariant (batten stays `go`-only because it has no
bootstrap verb — confirmed live via `--driver llm` and `lyx shed seed --driver llm` both refusing),
the one-FSM-per-worktree boundary (batten's own recipe never crosses into the child; composition is
by seed-write/status-read reference only), the four-row recipe with `Run-Shed`'s durable row identity
and its self-route/12-hour-budget shape (confirmed against `contracts/recipes/batten-recipe.yaml`
and live history), and the two consciously-shipped residuals. Relay-stepping is confirmed NOT
implemented (`Run-Shed`'s `Spawn` execs `lyx loom start --no-attach` directly, never `lyx shed step`
in the child) — correctly out of scope per the design doc's own rejection.

No scope gap found: the module delivers what the design doc promises. This round's findings are
CORRECTNESS bugs, not scope gaps, and they live specifically at a question the design doc never
addresses at all — what a SIGKILL mid-`Add()` (as opposed to an ordinary `Add()` error, which the
design doc's crash-window reasoning implicitly assumes is the only failure shape) leaves behind, and
whether the module's own idempotency probes and documented remedies are accurate for that state.
`docs/overview.md`'s own batten paragraph makes the same implicit assumption explicit: *"the state a
process killed BETWEEN THE CREATE FINISHING and its transition being persisted leaves behind"* — a
narrower window than the one this round's SIGKILL sweep actually found reachable (mid-`Add()`
itself, not just post-return-pre-persist). This doc sentence needs updating in the same commit as
the Job 2 fix.

## Docs & operability findings

- `docs/overview.md`'s batten paragraph (the "idempotent against its own post-condition" sentence)
  understates the crash window `taskWorktreePresent` must actually cover — see Scope assessment
  above. To be corrected in the same commit as the F-SIGKILL-ADD fix.
- `docs/overview.md`'s leftover-branch paragraph and both `createRefusal`'s/`doneSlugRefusal`'s own
  shipped text name `lyx fabric cleanup --apply --remote` as the weft-sibling remedy; all three need
  updating once F-CLEANUP-REMOTE-ORPHAN's fix lands (batten's own `remote: true` teardown makes the
  text true again for the common case, so the simplest correct fix is likely leaving the text as-is
  once the code changes rather than rewording it — reassessed at Job 2 time).
- `tools/sandbox/SANDBOX-FABRIC-SUITE.md`'s F22 scenario text (line ~544) claims following the
  done-slug remedy "does let the slug run again" — not automatically verified by any Go test (`grep`
  confirms `sandbox_coverage_test.go` only checks a `**Covers:**` tag's presence, not the scenario's
  own claims), and this round's live drive shows the claim is false as written for a real git-backed
  pair. To be revisited once the F-CLEANUP-REMOTE-ORPHAN fix lands (F22's own claim becomes true
  again, so likely no rewording needed there either — confirmed once the fix is live).
- No other docs/operability issues found. `CONSTRAINTS.md`'s Batten Bookend Invariant section is
  accurate against the live-confirmed refusal behavior (Sweep 4). `docs/reference/
  claude-trust-dialog-repro.md` is a loom-level (not batten) manual repro recipe; read, no batten
  content, no issue found.

## Executive summary

**4 findings, all fixed in Job 2** (see the fixer report for the fix-by-fix detail):

- **2 BLOCKING**, both CONFIRMED live, both reproducing a real crash-recovery or remedy-completeness
  gap in the normal operator flow, not a synthetic edge case:
  - **F-SIGKILL-ADD** — batten's `Worktree-Create` re-entry probe treats "a warp worktree directory
    resolves" as "fabric's `Add` finished," which is false for most of `Add`'s own multi-step
    sequence. A SIGKILL landing after the warp worktree is created but before `Add` returns either
    permanently wedges the run (`StateFailed`, opaque git error, no remedy) or — worse — corrupts
    the warp-pristine invariant by writing real files into what should be a `_lyx` junction, a state
    `lyx fabric reconcile` itself refuses to repair with a misleading "predates weft" diagnosis.
    Reproduced live at two distinct crash boundaries.
  - **F-CLEANUP-REMOTE-ORPHAN** — both of batten's own "re-run this slug" remedies
    (`createRefusal`'s leftover-branch text, `doneSlugRefusal`'s done-slug text) name
    `lyx fabric cleanup --apply --remote` as the way to clean up the task's weft sibling branch; that
    command's own enumeration is local-branches-only, and batten's own teardown always deletes the
    weft branch locally while leaving it on the remote — so the command never has anything to find.
    Reproduced live via BOTH remedy texts, and via the `typed-loom` slug specifically: following
    every documented step verbatim still leaves the slug unable to re-run (non-fast-forward push
    rejection with a misleading `git pull` hint) until a manual `git push origin --delete` no
    documented remedy names.
- **2 MEDIUM**, both CONFIRMED live via a revert-and-test experiment (this round's focus-3 sweep):
  `createRefusal`'s (F-WIRING-1) and the disagreeing-child-seed rewrap's (F-WIRING-2) own production
  call sites in `wire.go` can each be silently reverted with every test, `-tags integration`
  included, staying green — the fix works today but nothing pins it there.
- **0 LOW, 0 NIT** this round: no additional issues surfaced at that severity during this sweep.

**Merge-readiness opinion (pre-Job-2):** NOT a safety pass — this round found real, live-reproduced
defects, one of them (F-SIGKILL-ADD) a genuine data-model corruption path in the module's core
crash-recovery mechanism, squarely in scope and squarely what this round was seeded to find. Not
mergeable as-is. See the fixer report for the post-fix verdict.

## Teardown

Fixture hub at `/tmp/batten-r3-fixture-<timestamp>` (outside the loomyard tree and `$HOME/Code`,
never used on this host before this round) fully deleted after Job 2's own final live
re-verification: `find <scratch> -mindepth 1 -delete` then `rmdir`, confirmed absent from `/tmp`
afterward. `ps aux | grep -E 'lyx|tmux|reed|claude'` before and after teardown showed only this
host's pre-existing, untouched standing sessions (identical PIDs at session start and end) — zero
stray processes belonging to this round's own fixture. The operator's own standing bench
(`lyx-test-LYXHUB`) does not exist on this host and was never touched. `~/.claude.json` gained one
`projects` entry, for the fixture's prime worktree path; read to confirm, never modified, per the
live-substrate cost declaration's own instruction. `--child-driver llm` was not exercised: the `ly`
plugin is absent on this host (confirmed at the top of this log), an environment gap per the round
prompt's own focus 6, not a defect.

Job 2's fix-by-fix implementation, verification, and final merge-readiness verdict are in
`_mill/batten-review-sonnet-xhigh-r3-fixer-report.md`.
