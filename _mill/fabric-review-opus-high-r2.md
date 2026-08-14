# `fabric` — independent review, round 2 (`opus-high-r2`)

> Clean-room independent review+fix round per `_mill/fabric-review-prompt.md`.
> Primary target: the destruction chokepoint (`internal/fabricengine/destroy.go`), adversarially.
> This file is built incrementally during Job 1; the executive summary is written last.

## Status

Job 1 (review) in progress.

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

### L4 (LOW, CONFIRMED) — `lyx fabric clone` succeeds against a warp remote whose HEAD names a nonexistent branch

`lyx fabric clone` reported `ok:true`, `warp_binding_recorded:true`, and wired every
junction against a warp prime that was an unborn `master` with zero tracked files (the warp
remote's `HEAD` pointed at `master`; only `main` existed). The hub is structurally complete
and completely non-functional, with nothing in the envelope saying so. To be re-checked and
sited before fixing.
