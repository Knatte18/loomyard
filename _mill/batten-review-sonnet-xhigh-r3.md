# batten review — round sonnet-xhigh-r3

Follow-up campaign, round 3. Clean-room: written before consulting any prior review file except
the round prompt itself (`_mill/batten-review-prompt.md`).

## Status

IN PROGRESS — Job 1 (review). Findings and test log below are provisional and are being appended
to as testing proceeds, per the round prompt's "Log as you go" rule.

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
`git branch -D typed-loom` for the rollback's own leftover); a third attempt was launched and is
IN PROGRESS as of this checkpoint (backgrounded, being watched) — result appended once it lands.
This is already the second live confirmation of F-CLEANUP-REMOTE-ORPHAN regardless of the third
attempt's outcome, this time via the done-slug remedy rather than the leftover-branch one — same
root cause, two remedy texts.

**`lyx shed seed`'s per-recipe location rule, every checkout kind, both recipes.** `RefuseSeedAt` is
`nil` for `loom` (seeds wherever its verbs drive, confirmed by
`internal/shedcli/seed_test.go`'s `TestWriteSeed_RecipeLocationRuleGatesTheWrite`/`NilRuleWrites`
and the integration test's own `writeSeed(taskLocation, ..., RecipeLoom, ...)` case) and
`battencli.RefuseUnlessPrime` for `batten` (prime-only). Live-drove the batten case directly:
`lyx shed seed <run> --recipe batten` from the weft prime and from `_board` both refuse with the
same `RefuseUnlessPrime` text already confirmed correct under the Batten Bookend checks below; from
a task worktree it refuses via the prime-name comparison (proven by the existing integration test,
`TestWriteSeed_BattenSeedIsRefusedOutsidePrime`, re-read after my own findings were complete — see
Sweep 5). No live gap found in this rule.

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

(More findings to follow as the remaining sweeps — focus 2's full error-remedy table, focus 5's
re-confirmation of the CLOSED-AND-VERIFIED list, the youngest fix's `--child-driver llm` check — are
completed.)
