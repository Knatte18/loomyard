# `fabric` — independent review, round 2 (`opus-high-r2`)

> Clean-room independent review+fix round per `_mill/fabric-review-prompt.md`.
> Primary target: the destruction chokepoint (`internal/fabricengine/destroy.go`), adversarially.
> This file is built incrementally during Job 1; the executive summary is written last.
> Job 1 is COMPLETE as of this revision — no production or test file has been touched yet.
> Findings: 0 BLOCKING, 3 MEDIUM (M1-M3), 4 LOW (L1-L4), 5 NIT (N1-N5).

## Executive summary

Round 2's assigned primary target was the destruction chokepoint (`destroy.go`), with a full
round's adversarial budget aimed at making it perform a destructive primitive it should have
refused. **It does — via symlink-mediated containment (M3), reproduced live against real git
and the real CLI.** The gate's containment check and its one lexical-only ownership predicate
both compare nominal paths through `filepath.Rel` and never resolve a symlink, so a link
planted at an intermediate segment of a gate target lets `removeLaunchers`' gated `removePath`
delete files outside the hub — with `ok:true`, exit 0, and a mutation record naming paths that
were never the inodes removed. Every OTHER attack on the chokepoint failed as designed:
traversal slugs, prime-worktree/`_board`/`-weft` targets, a symlinked slug directory, an
imposter link, 4-way concurrent `remove --force` on one target, `remove` vs `reconcile`,
`prune` vs `add`, and `clone --reset` through a symlink all refused correctly with no data
loss. The `--force`-answers-dirtiness-only property and the `createdToken{` ban were both
re-derived from source and hold.

The seeded residual is **root-caused and re-graded up**: the writer is `internal/logger`'s
durable sink, which is unconditional in a deployed binary, and the residual needs **no racing
at all** — `unwire`, one failed command, `reconcile`, twice, sticks the hub permanently and
also leaves the warp worktree permanently untracked-dirty (M1).

Two more genuine defects fell out of the same driving: `reconcile` reports `ok:true` / exit 0 /
`partial:false` for a pair it failed to repair (M2), and the operator's `--force` is silently
dropped by the two fallback `pathRequest`s, producing a refusal that tells the operator to
"use --force" when they already did (L1).

**Top risks, ranked:** (1) M3 — the chokepoint's containment property is weaker than its own
documentation claims; (2) M1 — a documented two-verb operator round-trip permanently bricks a
pair's wiring; (3) M2 — a repair verb's success signal is unreliable to every scripted caller.

**Counts:** 0 BLOCKING, 3 MEDIUM, 4 LOW, 5 NIT (12 total).

**Merge-readiness (pre-fix):** NOT ready — M3 and M1 both need fixing before merge.
**Merge-readiness (post-fix):** READY — all 12 findings fixed, every gate green, every live
scenario re-driven. See `_mill/fabric-review-opus-high-r2-fixer-report.md` for the verdict in
full, the sabotage-proof table, and three observations recorded after this review was frozen.

## Scope assessment — plan vs shipped

All 16 `lyx fabric` verbs were driven live against a real hub built from local bare git
remotes (`list`, `pairs`, `status`, `add`, `commit`, `push`, `sync`, `pull`, `diff`,
`checkout`, `prune`, `cleanup`, `remove`, `clone`, `reconcile`, `unwire`) — every one is
present, wired into cobra with a `Short`, and does what its help text says.

`destroy.go`'s consolidation is real and complete: the raw-primitive inventory was
re-derived from source (below) and matches the guard's allowlist exactly, with no undeclared
raw site and no `createdToken{` composite literal outside the gate. The eight executors, the
eight `pathOwnershipKind`s and the two `branchOwnershipKind`s all exist as documented, and
the ownership predicates that CAN cross-check against an independent authority (git's own
worktree registration, `fslink.RawTarget`) do. Nothing is shipped beyond scope, and nothing
plan-promised is missing.

The one scope-shaped gap is a boundary rather than an omission: the gate owns *which* checks
run, but the correctness of `refuseUncontainedPath` (M3) and the `force` field's propagation
(L1) live at each call site's request construction, and `pathRequest`'s own doc claims "every
field is required — a zero-value ownership or dirtiness is refused by the pipeline" while
`force` is exactly the field a site can silently omit to the zero value. L1 is that claim
coming true.

## What was tested

### Hermetic baseline (before any edit)

```
go build ./...                                      -> rc=0
go vet ./internal/fabricengine/... ./internal/fabriccli/... \
       ./internal/gitexec/... ./internal/gitrepo/...  -> rc=0
go test ./internal/fabricengine/... ./internal/fabriccli/... ./internal/gitexec/... \
        ./internal/gitrepo/... ./cmd/lyx/... -count=5 -> rc=0 (no FAIL lines)
```

### Raw destructive-primitive inventory, re-derived from source (not trusted from a prior round)

`grep -rn -F` over each of `destructiveguard_test.go`'s eight banned tokens across
`internal/fabricengine/*.go` minus `_test.go`:

| token | production hits outside `destroy.go` |
| --- | --- |
| `RemoveAll(` | `warpprobe.go:58` |
| `os.Remove(` | `junction.go:284`, `hook.go:157`, `launchers.go:242`, `gitexclude.go:107/111/115/119`, `index.go:307`, `ancestors.go:52` |
| `"worktree", "remove"` | none |
| `"branch", "-D"` | none |
| `warp.ResetHard(` | none |
| `weft.ResetHard(` | none |
| `fslink.Remove(` | none |
| `createdToken{` | `doc.go` (prose only) |

Every hit maps to an existing `destructiveGuardAllowlist` row, and every allowlist row is
still justified by the code at that line. The inventory MATCHES — no undeclared raw site.

### Live substrate — hub construction

Scratch hub built from plain local bare git remotes (`scratchpad/mkhub.sh`), root anchor,
`lyx fabric clone <weft.git> <warp.git>` → `ok:true`, junctions `_lyx`/`.lyx`/`_board` all
wired as symlinks into the weft prime. Verified `git -C adv status` clean on `main`.

Incidental fixture note: with `git init --bare` (no `-b main`) the warp remote's `HEAD`
points at a nonexistent `master`; `lyx fabric clone` reported `ok:true` on a hub whose warp
prime was an unborn `master` with zero files. Recorded as finding L4 below.

### Live substrate — seeded residual (`unwire` leaving a real `.lyx`)

`scratchpad/serial_unwire.sh`: plain `unwire` → `reconcile` on a hub where nothing has ever
logged is CLEAN (`already_healthy`, `junction_healthy:true`). So the junction teardown alone
is not the mechanism.

`scratchpad/residual.sh` — **root cause established, and the residual reproduced with NO
racing at all**, two serial operator actions plus one failed command:

```
cycle 1:  lyx fabric unwire                                   -> ok
          lyx fabric diff deadbeef...   (exits 1)             -> creates REAL adv/.lyx/logs/trace-*.log
          lyx fabric reconcile                                 -> adopts logs into weft, ok
cycle 2:  lyx fabric unwire                                    -> ok
          lyx fabric diff deadbeef...   (exits 1)             -> creates REAL adv/.lyx/logs again
          lyx fabric reconcile                                 -> PERMANENTLY REFUSES:
   "re-point junction: adopt <warp>/.lyx into <weft>/.lyx: logs already exists at the weft
    target; an earlier adoption already ran — delete the warp-side copy at
    <warp>/.lyx/logs and re-run `lyx fabric reconcile`"
          lyx fabric reconcile (again)                         -> same refusal, not self-healing
          lyx fabric pairs                                     -> junction_healthy:false,
                                                                  "warp .lyx is not a junction"
```

The writer is `internal/logger/sink.go`'s `ensureDurableSink` (see finding M1). Its only
suppression is `testing.Testing() && os.Getenv("LYX_TRACE") != "1"` — a `go test`-time gate
only. A DEPLOYED binary opens the durable sink, and therefore runs
`os.MkdirAll(<anchor>/.lyx/logs)`, on the first `Info`-or-above record
(`durableHandler.Enabled` accepts Info+ unconditionally, never consulting `levelVar`, so the
default silent `Warn` verbosity does not suppress it) **or** on `NotifyExit(code != 0)`.
`LogsDir` is warp-anchored (`l.AnchorPath()/.lyx/logs`), so with the junction wired the
MkdirAll resolves through it into weft — and in the post-`unwire` window it creates a real
warp-side directory instead. CONSTRAINTS.md's Live-Substrate Spawn Observability invariant
documents only the `go test` half of this, which is why inferring from its prose alone would
have pointed at the wrong layer.

Two extra observations from the same run, both findings of their own:
- `lyx fabric reconcile` printed `"ok":true` and exited **rc=0** while carrying a per-pair
  `"error"` naming a junction it failed to repair (finding M2).
- the same envelope carried `"partial":false` while `"mutations"` was non-empty and a pair
  had failed (finding M2).

### Live substrate — chokepoint scenario 1: symlink-mediated containment bypass (`scratchpad/symlink_containment.sh`)

The gate's containment check (`ancestors.go:20` `refuseUncontainedPath`) and the one
lexical-only ownership kind (`destroy.go:475` `pathAtOrBelow`, used by
`ownedUnderGeometryRoot`) both work on NOMINAL paths via `filepath.Rel` — no
`filepath.EvalSymlinks` anywhere. So I planted a symlink at an intermediate segment of a
gate target and drove the real CLI:

```
lyx fabric add t1                       -> <Hub>/_launchers/t1/{ide.sh,fabric-checkout.sh}
rm -rf <Hub>/_launchers/t1
ln -s <outside-the-hub>/VICTIM <Hub>/_launchers/t1
lyx fabric remove t1 --force
```

Result — the gate DESTROYED two files outside the hub and reported success:

```
{"links_removed":3,"mutations":[ ...,
  {"kind":"path_removed","target":"_launchers/t1/ide.sh","detail":"single"},
  {"kind":"path_removed","target":"_launchers/t1/fabric-checkout.sh","detail":"single"},
  {"kind":"path_removed","target":"_launchers/t1","detail":"single"}, ... ],
 "ok":true,"partial":false,...}
remove rc=0

  ide.sh:             *** DESTROYED OUTSIDE CONTAINER ***
  fabric-checkout.sh: *** DESTROYED OUTSIDE CONTAINER ***
  unrelated.txt:      SURVIVED
  victim dir itself:  SURVIVED
```

`unrelated.txt` and the victim directory survived only because `removeLaunchers` names
exactly two files and then `os.Remove`s the directory entry (which removed the SYMLINK, not
its target) — a bound on blast radius that comes from the launcher script list, not from
anything the gate checked. See finding M3.

### Re-derivation: `--force` satisfies dirtiness and nothing else

Re-derived from source, not from the doc comment. `grep -rn "\.force"` over
`internal/fabricengine/*.go` minus tests yields exactly two non-comment reads:
`destroy.go:593` (inside `checkPathDirtiness`, where it makes the check PASS) and
`destroy.go:692` (inside `removeGitWorktree`, where it appends `--force` to the git argv —
not a check). The `Check` enum (`destroy.go:58-68`) has exactly three members and no
`CheckForce`. **Claim holds.**

### Live substrate — concurrent destructive races (`scratchpad/races.sh`)

Combinations round 1 and the orchestrator did NOT drive:

**(a) 4× concurrent `lyx fabric remove ra --force` on the SAME target.** Exactly one winner
(rc=0), three correct refusals: one from the gate's OWNERSHIP check ("is not a registered
linked worktree of …") because the registration had already gone, two from git's own check
with fabric correctly declining the directory fallback. Final state: `ra`/`ra-weft` gone,
`git worktree list` clean on both repos, `ra-weft` branch deleted exactly once, no
substrate-corruption marker, `lyx fabric pairs` → `junction_healthy:true`. **The chokepoint
behaved exactly as designed under a race.**

**(b) `remove rb --force` racing `reconcile`.** No corruption; the pair was removed; the
serial `reconcile` afterwards settled to `already_healthy`. The racing `reconcile` reported
the vanishing pair as `unmanaged_reported` carrying a raw
`read current branch via the unborn-branch fallback: chdir …: no such file or directory` —
`os/exec`'s own message for a `cmd.Dir` that disappeared (verified: `gitexec.runCore` sets
`cmd.Dir`, and there is NO `os.Chdir` anywhere in production fabric code). Finding N5.

**(c) `prune --apply --force` racing `add`.** Both succeeded, the stale pair was removed, the
new pair was created and wired, `pairs` reported both remaining pairs healthy, no marker.

### Live substrate — other chokepoint attacks (`scratchpad/more_adversarial.sh`, `force_fallback.sh`)

| attack | result |
| --- | --- |
| `remove ../../etc`, `../evil`, `a/../../b`, `.`, `..` | all refused by slug validation, `mutations:[]` |
| `remove adv` (the prime warp worktree) | refused by name before any teardown |
| `remove _board` | refused — reserved hub-geometry name |
| `remove adv-weft` | refused — reserved `-weft` suffix |
| `remove <slug>` where `<slug>` is a symlink to a live directory | refused by OWNERSHIP (git's registration records a resolved path, so the symlinked nominal path does not match) — victim file survived, link left in place |
| `clone --reset` where `<cwd>/<name>-HUB` is a symlink to a real hub | removed the SYMLINK only (`removePath`'s `os.Lstat` sees a link, so it takes the `os.Remove` branch, never `RemoveAll`) — the real hub's contents survived |
| `reconcile` when warp `.lyx` is a real dir holding a name colliding with a FILE at the weft target | refused, correctly and permanently — the genuinely-ambiguous case M1's fix must keep refusing |
| `cleanup --apply --force` against `handcrafted-weft` (a `-weft` branch fabric never created) | deleted — by design, `WeftWarpSlug` accepts the name; documented in `cleanup.go` |

**`--force` does NOT reach the gate's fallback request** (`scratchpad/force_fallback.sh`):
with `t2` untracked-dirty and `git worktree lock`ed so `git worktree remove --force` fails,
`lyx fabric remove t2 --force` returns

```
"error":"refusing to remove warp worktree: dirtiness check failed for <hub>/t2:
         worktree has uncommitted changes; use --force"
"ok":false,"partial":true, refusal.check="dirtiness"
```

— an unactionable remedy (the operator DID pass `--force`), leaving a half-torn-down pair
(portal, launchers and all three junctions already removed). Finding L1.

### Live substrate — all 16 verbs driven (`scratchpad/all_verbs.sh`)

`list`, `pairs`, `status`, `add`, `push`, `sync`, `pull`, `diff`, `checkout` (both
directions), `prune`, `cleanup`, `remove`, `clone`, `reconcile`, `unwire`, `commit` — all
exit 0 and produce the documented envelope shape, except: `lyx fabric prune` emits
`"entries":null` rather than `[]` (finding L3), and `lyx fabric commit -m …` correctly
rejects an unknown shorthand (my invocation error, not a defect).

### Hermetic + integration gates, and the N× concurrent amplifier

```
go test -tags integration ./internal/fabricengine/... ./internal/fabriccli/... \
        ./internal/gitexec/... ./internal/gitrepo/... -count=1        -> all ok, rc=0, no markers

go test -c -tags integration -o $SCRATCH/fabric.integration.test.exe ./internal/fabricengine/
for i in 1 2 3 4; do ( $SCRATCH/fabric.integration.test.exe -test.count=1 -test.v \
    -test.parallel=8 > $SCRATCH/int_$i.txt 2>&1; echo "run$i rc=$?" ) & done; wait
  -> run1 rc=0  run2 rc=0  run3 rc=0  run4 rc=0
  -> grep -hiE 'FAIL|being used by another process|permission denied|dangling|panic:'
     matches only test NAMES containing "Fail"; no real FAIL, no corruption marker.
```

### Re-derivation: `createdToken{` outside `destroy.go`

`grep -rn -F 'createdToken{'` over production source: hits only in `destroy.go` (the two
minters plus its own doc prose) and `doc.go` prose. **No same-package composite literal has
snuck past the guard.**

## Findings

### M1 (MEDIUM, CONFIRMED) — a deployed `lyx` materialises a real warp-side `.lyx/logs`, permanently sticking `reconcile` after one `unwire`

`internal/logger/sink.go:75-108` (`ensureDurableSink`) + `internal/fabricengine/junction.go:254-269`
(`adoptDotLyxContent`).

Scenario (fully reproduced above, race-free): `lyx fabric unwire` removes the `.lyx`
junction. Any subsequent `lyx` invocation in that warp worktree that logs at Info+ or exits
non-zero runs `os.MkdirAll(<warp>/.lyx/logs)` and writes a trace file there. `lyx fabric
reconcile` — the documented remedy — then hits `seedLyxJunction`'s real-directory branch,
calls `adoptDotLyxContent`, and refuses the moment a same-named entry (`logs`) already
exists at the weft target, which it always does from the second cycle onward. The pair is
then permanently non-self-healing: every later `reconcile` repeats the identical refusal, and
`pairs`/`status` report `junction_healthy:false`.

Re-graded from the seeded LOW-MEDIUM to **MEDIUM**: no work is lost and the diagnosis is
honest, but reachability is far worse than the seeded "deliberate 4-way racing" — it is two
documented operator verbs with one ordinary failed command in between, and `unwire`'s own
doc comment advertises `reconcile` as the re-wire path.

One consequence the seeded description did not name, and which makes the stuck state worse
than "an inert real directory": because `seedLyxJunction` aborts before `seedGitExclude` runs,
the `.git/info/exclude` entries `unwire` reverted are never re-seeded, so the warp worktree
stays permanently untracked-dirty. Verified in the stuck hub:

```
$ git -C <hub>/adv status --porcelain
?? .lyx/
?? _lyx
$ cat <hub>/adv/.git/info/exclude   # (lyx-managed lines only)
/_board
```

That dirt trips `Remove`'s no-force dirty gate and `prune`'s tracked/untracked protection on a
pair the operator never dirtied — so the stuck state propagates into the destruction gate's own
decisions.

Fix (right layer, per the prompt): make `adoptDotLyxContent` MERGE a same-shaped directory
rather than refuse it. A colliding *directory* on both sides is exactly the state repeated
adoption produces, and `.lyx` is by contract lyx's own disposable machine-local scratch
(`seedLyxJunction`'s own comment says so). Recurse into a dir/dir collision, keep refusing a
collision that is not dir/dir (a file, or a type mismatch) since that is genuinely ambiguous,
and remove the emptied warp-side subdirectory on the way back out. A clearer remedy message
alone would leave the hub just as stuck.

### M2 (MEDIUM, CONFIRMED) — `lyx fabric reconcile` reports `ok:true` / exit 0 / `partial:false` when a pair fails to repair

`internal/fabriccli/fabric.go:680-687` (`runReconcile`'s tail) always exits through
`okWithRecord`. `ReconcilePairResult.Error` (`internal/fabricengine/reconcile.go:121-122`) is
serialized into the `pairs` array and consulted by nothing else.

Scenario (observed in the M1 run above): with the hub in the stuck state, `lyx fabric
reconcile` printed
`{"mutations":[...3 entries...],"ok":true,"pairs":[{...,"error":"re-point junction: adopt ..."}],"partial":false}`
and exited **0**. A scripted caller — mill, an agent, a CI step — that checks `$?` or reads
`ok` sees an unqualified success for a repair that did not happen. Worse, `"partial":false`
is emitted in exactly the state `doc.go`'s own definition of `partial` says the key exists to
name: "did this call leave the hub in a state some but not all of the intended change landed
in" — mutations landed, one pair failed.

Note this is NOT the documented carve-out: `runReconcile`'s doc comment explicitly exempts
only the warp-binding backfill's commit/push ("a convenience repair may never downgrade a
reconcile verdict"). A junction it failed to re-point is the verb's primary job, not a
convenience.

Fix: after the pair loop, if any pair carries a non-empty `Error`, emit through
`output.ErrFields` (keeping the `pairs` array and `mutations` intact, adding `partial` =
record-non-empty) so `ok` is false and the exit code is non-zero. `runPrune` has the same
shape and is checked below.

### M3 (MEDIUM, CONFIRMED) — the gate's containment check is lexical, so a symlinked intermediate segment lets a destructive primitive act outside its container

`internal/fabricengine/ancestors.go:20-29` (`refuseUncontainedPath`) and
`internal/fabricengine/destroy.go:475-484` (`pathAtOrBelow`, `ownedUnderGeometryRoot`'s
predicate).

Both compare NOMINAL paths through `filepath.Rel`. `destroy.go`'s own file header and its
`pathRequest.container` field comment both say the target must "**resolve** strictly below its
declared container" — the code never resolves anything. Reproduced live above: a symlink
planted at `<Hub>/_launchers/<slug>` makes `removeLaunchers`' gated `removePath` delete
`<somewhere-else>/ide.sh` and `<somewhere-else>/fabric-checkout.sh`, with `ok:true`, exit 0,
and a `mutations` record naming hub-relative paths that were never the inodes removed — so
the record's own commission guarantee ("every entry corresponds to a real, completed effect"
at the named target) is violated too.

`ownedUnderGeometryRoot` is the ONLY ownership kind with no independent resolved-path
cross-check, which is why this site is the one that falls: `ownedRegisteredLinkedWorktree`
and `ownedWarpCheckout` compare against git's own worktree registration (git records
resolved paths, and `filepath.Clean` of a symlinked nominal path will not match it, so those
sites refuse); `ownedWiredJunction` compares `fslink.RawTarget`; the two token kinds compare
a gate-minted path.

Blast radius today is bounded to the two launcher script names, and reachability requires a
symlink planted inside fabric's own hub geometry — which no fabric code path creates. Hence
MEDIUM, not BLOCKING. But it falsifies the property the chokepoint's own documentation calls
the one thing `--force` can never override ("a containment failure — the class of defect that
once destroyed an entire hub — can never be overridden by a flag"): it can be bypassed
without a flag at all.

Fix: resolve both sides consistently before comparing — `filepath.EvalSymlinks` the
container, and resolve the target by walking up to its deepest EXISTING ancestor, resolving
that, and re-appending the remaining components (so an absent target and a target that is
itself a symlink both still work, and a hub path that legitimately contains a symlinked
ancestor is not falsely refused). Apply it in `refuseUncontainedPath` (which every gated
path request runs) and in `pathAtOrBelow`.

### L1 (LOW, CONFIRMED) — the operator's `--force` is silently dropped by both fallback `pathRequest`s

`internal/fabricengine/remove.go:263-269` and `internal/fabricengine/prune.go:297-303`.

Both fallback requests are built without a `force:` field at all, so it takes Go's zero value
`false`, while the PRIMARY request each falls back from declares `force: force` (remove.go:231)
and `force: true` (prune.go:277) respectively.

Reproduced live (above): `git worktree lock` makes `git worktree remove --force` fail on a
registered linked worktree; the fallback then refuses with `worktree has uncommitted changes;
use --force` — the one remedy the operator has already applied — and leaves a half-torn-down
pair. `prune --apply --force` has the identical shape against a tracked-dirty weft worktree
(its own `applyStalePairProtection` gate is what `force: true` at prune.go:277 already
answers).

This is exactly the failure mode `pathRequest`'s doc comment names for the OTHER fields
("a zero-value ownership or dirtiness is refused by the pipeline rather than silently passed,
which is what makes an omitted check a loud failure instead of a forgotten one") — `force` is
a bool with no unset state, so it is the one field that CAN be silently forgotten.

Fix: propagate the primary request's `force` into both fallback requests. This preserves the
protection the fallback exists for in the no-`--force` case (where git refused on untracked
files and the fallback must not delete them), and honours the flag in the `--force` case
(where git was already invoked WITH `--force`, so it refused for some other reason). Add a
comment at both sites naming why `force` must travel, so the omission is not reintroduced.

### L2 (LOW, CONFIRMED by source) — the gate's absent-target short-circuit runs BEFORE its declaration-validity checks

`internal/fabricengine/destroy.go:549-562`. `checkPathRequest` returns nil on
`os.Lstat` → `IsNotExist` before it ever tests `req.ownership.kind == pathOwnershipUnset` or
`req.dirtiness.kind == pathDirtinessUnset`.

Consequence: a request that declares NO ownership and NO dirtiness passes the gate vacuously
whenever its target happens to be absent. The property the file's own doc claims — "an
omitted check is a loud failure instead of a forgotten one" — therefore holds only when the
target exists, which is precisely the case a new call site's first test is least likely to
cover. No current call site is malformed (all sixteen were read); this is a
guard-strength defect, not a live bug.

Fix: run the two declaration-validity checks (and `dirtinessNA`'s non-empty-reason check)
BEFORE the absent-target early return. A malformed request is malformed regardless of what is
on disk, and validating a struct costs no syscall.

### L3 (LOW, CONFIRMED) — `lyx fabric prune` emits `"entries":null`

`internal/fabriccli/fabric.go:709` returns `r.Entries` directly, and `PruneResult.Entries` is
a nil slice when nothing is stale, so the envelope carries `"entries":null`. Observed live:
`{"entries":null,"mutations":[],"ok":true,"partial":false}`. `cleanup` and `reconcile` both
emit real arrays in the same situation. A consumer must special-case `null` for one verb only
— the exact asymmetry `envelope.go`'s "always a JSON array, never null" rule exists to
prevent for `mutations`.

Fix: normalise to an empty slice before serialising, as `mutations` already is.

### L4 (LOW, CONFIRMED) — `lyx fabric clone` reports success on a warp remote whose HEAD names a nonexistent branch

`internal/fabricengine/clone.go:253-265` (step 5's warp clone).

Scenario, hit accidentally while building the very first fixture: a warp remote created with
plain `git init --bare` has `HEAD → refs/heads/master`; push only `main` to it. `git clone`
warns on stderr ("remote HEAD refers to nonexistent ref, unable to checkout") and exits 0,
leaving an unborn `master` with zero files. `cloneRepo` reports success, and `lyx fabric
clone` goes on to report `ok:true`, `warp_binding_recorded:true`, and a full mutation record
naming every junction it wired — against a warp prime with NO content at all. Verified:
`git -C adv status` → "No commits yet on master…origin/master [gone]"; `git ls-files` empty.

An unborn warp HEAD IS a legitimate documented state elsewhere (a fresh `git init` before the
first commit — see `doc.go`'s `commitWeft` warpHeadSHA rule), so the refusal must be narrow:
unborn warp HEAD **and** the warp remote actually has at least one branch. Under that
conjunction the operator's hub can never work, and reporting success is strictly worse than
aborting.

Fix: after step 5's clone lands, detect that conjunction and abort through the existing
`teardownHub` path with an actionable message, exactly as clone's twelve other strict-abort
sites do.

### N1 (NIT, CONFIRMED) — "the four read-only verbs' result types" is wrong in three places

`internal/fabricengine/doc.go` (the mutation-record section), `CONSTRAINTS.md:281`, and
`cmd/lyx/destructiveguard_test.go:175-176`.

Re-derived from source: there are exactly TWO read-only result types, and neither pairing is
what the prose claims. `Topology.Status` → `StatusResult` (`status.go:61/71`) is the **pairs**
verb; `Fabric.Diff` → `DiffResult` is `diff`; `Fabric.Status` (`diff.go:102`) returns a bare
`[]ChangeEntry` and `List` (`worktreelist.go:27`) returns a bare `[]WorktreeEntry` — the
`status` and `list` verbs have no result TYPE at all. So `doc.go`'s "`StatusResult`,
`DiffResult`, and their siblings for `list`/`pairs`" names siblings that do not exist and
mis-assigns `StatusResult`, and the guard's own table comment says "four" while declaring two
rows.

Fix: say two, name which verb each type serves, and state plainly that `list`/`status` have
no result type so there is nothing for the guard to pin — a reader must not conclude the
guard is under-populated.

### N2 (NIT, CONFIRMED) — the gate's documented check order is not the order the code runs

`internal/fabricengine/destroy.go:8-9`, `CONSTRAINTS.md:263`. Both say the pipeline is
"four checks, always in this fixed order … containment, ownership, dirtiness, force".
`checkPathRequest` actually runs: absent-target short-circuit, then the ownership/dirtiness
DECLARATION-validity checks, then slug validation, then containment, then ownership, then
dirtiness. The declaration checks reporting `CheckOwnership`/`CheckDirtiness` before
containment has ever run means a refusal's `check` field can name ownership for a request that
also fails containment — the opposite of what "stopping at the first failure" implies.

Fix: state the real order in both places (declaration validity is a request-shape check, not
one of the four), and — with L2's fix — the order becomes deterministic and documentable.

### N3 (NIT, CONFIRMED) — `destroy.go`'s removal primitive is an exported mutable package var

`internal/fabricengine/destroy.go:805`: `var RemoveAll = os.RemoveAll`. Inside the one file
whose entire purpose is that no other code may reach a destructive primitive, the primitive
itself is a mutable, EXPORTED package-level variable: any package in the module can replace
the gate's own removal function, and two tests assigning it concurrently are a data race the
bypass guard cannot see. Nothing outside the package assigns it today (verified by grep).

Fix: document the exposure honestly in the declaration's doc comment (the seam has real value
and removing it would cost error-injection coverage), and name it as a known limit alongside
the invariant's other honestly-stated blind spots.

### N4 (NIT, CONFIRMED by source) — the dirtiness probe is check-then-act, and the doc does not say so

`internal/fabricengine/destroy.go:589-605` runs `git status --porcelain` and returns; the
executor then acts. Nothing holds a lock across the two, so a write landing in that window is
destroyed by `removePath`'s `RemoveAll` with no further check. The exposure is genuinely
narrow — `removeGitWorktree` re-checks through git itself, `resetHardTo` delegates to git, and
the only `RemoveAll` sites carrying a real dirtiness scope are the two fallbacks that fire
only after git has already declined — but `destroy.go`'s header claims the pipeline runs
"before performing its act" without ever saying the two are not atomic.

Fix: state the limit in `destroy.go`'s header, in the same honest register the file already
uses for `createdToken`'s guard-not-type-system caveat. (Closing the window would need a
lock spanning probe and act across every executor — a much larger change than this round's
budget, and not obviously worth it for a window only reachable after git has already refused.)

### N5 (NIT, CONFIRMED) — a pair whose worktree vanishes mid-`reconcile` is reported as `unmanaged_reported` with a raw `chdir` error

Observed in race (b) above. `Topology.Reconcile`'s per-pair loop reads the warp branch via
`readBranch` (`reconcile.go:534`), whose failure surfaces `os/exec`'s raw
`chdir <path>: no such file or directory`. The action reported is `unmanaged_reported`, which
means something quite different ("this pair is not fabric's to manage").

Fix: check for the warp worktree directory having vanished before reading its branch, and
report that distinctly — a directory that disappeared between `git worktree list` and the
per-pair step is a concurrent `remove`/`prune`, not an unmanaged pair.

### Deliberate non-findings, with reasons (so the next round need not re-derive them)

- **`prune`/`cleanup` do not fail on a per-entry `Error` either, and that is CORRECT.** Their
  `Error` field doubles as a REPORT channel: `prune`'s `Protected` entry sets `Error` to
  "commit them or re-run with --force" as its designed explanation, and `Unowned` likewise.
  Making either verb exit non-zero on any entry error would turn a documented advisory outcome
  into a failure. `reconcile` (M2) is different in kind: every one of its six `pr.Error` sites
  is a genuine repair failure, never a report.
- **`ownedDriftedWiredJunction` removes a user's own symlink sitting at a wired junction
  path.** Verified from source and deliberate: the resolved target is not compared because
  drift is the precondition for repair. `_lyx`/`.lyx`/`_board` are names fabric reserves, only
  the link itself is removed (never its target), and a tracked symlink is restorable with
  `git checkout`. Not a defect.
- **`cleanup --apply --force` deletes a `-weft`-suffixed branch fabric never created.** Driven
  live; `WeftWarpSlug` accepts the name by design and `cleanup.go:51` already documents the
  `--force` consequence.
- **The correspondence index's `RebuildIndex` two-phase residual.** Out of scope per the
  prompt; re-read the code and confirmed unchanged.
- **Windows path behaviour.** Permanent, never-executed gap — unreachable from a Linux host.
  Stated as a limit in the merge-readiness verdict, as in every prior round.

## Docs & operability findings

Consolidated above rather than duplicated: N1 (three places claim four read-only result
types), N2 (documented check order ≠ executed order), N3 and N4 (two honest limits the gate's
own documentation currently omits), and the doc half of M3 (`destroy.go` says the target must
"resolve strictly below its declared container" while the code resolves nothing). Every one
of these is a case of the documentation being STRONGER than the code — the direction that
matters, since a reader who trusts it writes the next defect.

## What I could NOT verify, and why

- **Windows path behaviour** (junctions vs symlinks, open-handle rename failures,
  `git worktree remove --force` against a held handle). Genuine environment gap: the host is
  Linux. Every `fslink` path here exercised the symlink branch only.
- **The TOCTOU window in N4 as a live repro.** The window is real and derivable from source,
  but hitting it deterministically from outside the process would require an injection point
  that sits between `checkPathDirtiness` and the executor; the only such seam (`RemoveAll`)
  sits after the check, so driving it would prove the seam's position rather than the race.
  Recorded as CONFIRMED-by-source, PLAUSIBLE as an operational event.
- Nothing else. No scenario was skipped for cost, time, or convenience.
