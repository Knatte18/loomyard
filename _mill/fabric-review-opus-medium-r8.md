# `fabric` — independent review, round 8 (`opus-medium-r8`)

> Clean-room, unprimed review of the `fabric` module per `_mill/fabric-review-prompt.md`.
> This is the operator's stated FINAL round of the campaign, with no seeded residual — a general
> confidence sweep rather than a hunt for a known gap.
> Model/effort: Opus/medium.

## Executive summary

The operator's expectation for this round — "it should not find much" — **held up, but not
completely.** After a genuine unprimed pass (read `doc.go` and `CONSTRAINTS.md` in full, read the
gate and its create/write-side minters, drove all 16 verbs live against a real hub with a subpath
anchor, and ran static and racing symlink attacks against the real deployed binary), the module is
in good shape: every chain the campaign closed is still closed, both guard tests are still accurate,
and no substrate corruption or data-loss defect surfaced anywhere.

What did surface is **one coherent cluster of four leftovers around the launcher/portal teardown
path** — the one corner that has been touched by every previous round's fix from the outside but
never audited on its own terms. R3 rewrote the gate's two arbitrary-path executors onto `os.Root`;
R7 rewrote the two hub-level container WRITERS onto `os.Root`. Nobody looked at the *third*
arbitrary-path removal (`removeLaunchers`' own `os.Remove(launcherDir)`), at the sweeper that runs
immediately after it (`pruneEmptyAncestors`), or at whether that path's containment refusal is
actually the type `surfaceRefusal` propagates. It is not — and that one is live-reproducible with a
plain static symlink and no race at all.

**Top risks, in order:**

1. `removeLaunchers` still resolves containment at one instant and unlinks at a later one (M1) —
   the pattern `destroy.go`'s own doc comment declares insufficient.
2. `Remove`/`Prune` report `ok:true` while the launcher/portal teardown was refused outright,
   leaving orphaned launcher scripts (L2) — **confirmed live**, no race needed.
3. `pruneEmptyAncestors` relates and removes purely lexically (L1).
4. Every fabric verb accepts arbitrary extra positional arguments and silently ignores them (L3) —
   `lyx fabric unwire <typo>` executes the unwire.

None is a data-loss defect in the normal single-instance flow, and none blocks merge.

**Merge-readiness: MERGEABLE**, with the fixes in this round applied, and with the standing limits
restated: Windows path behaviour remains permanently unverified from a Linux host, and N4's
dirtiness-probe TOCTOU remains an accepted, documented residual (not re-chased this round, per the
prompt).

## Counts by severity

0 BLOCKING, 1 MEDIUM, 3 LOW, 0 NIT.

## Scope assessment (plan vs shipped)

No scope gap found. All 16 verbs registered on the cobra tree are implemented and were driven live;
`doc.go`'s promises match the as-built code on every section I exercised (unified `Pull`, the
`Warp-SHA` trailer + rebuildable correspondence index, the `Snapshot:` write/read halves, the
`_board` convenience junction's wire-only-and-unmonitored contract, the three repo-wide records, and
the destruction chokepoint). Nothing is shipped beyond scope, and I found no deferred-that-should-be-v1
item. The four findings below are all correctness/operability, not scope.

## Status

Job 1 (review) COMPLETE — saved and committed before any production or test file was touched.

## What was tested

### Hermetic gates

- `go build ./...` — **PASS** (exit 0, no output).
- `go vet ./internal/fabricengine/... ./internal/fabriccli/... ./internal/gitexec/... ./internal/gitrepo/...` — **PASS** (exit 0, no output).

- `go test ./internal/fabricengine/... ./internal/fabriccli/... ./internal/gitexec/... ./internal/gitrepo/... ./cmd/lyx/... -count=5` — **PASS**, all five packages `ok`.
- `go test -tags integration ./internal/fabricengine/... ./internal/fabriccli/... ./internal/gitexec/... ./internal/gitrepo/... -count=1` — **PASS** (exit 0), no FAIL and no substrate-corruption marker.

### Guard-test accuracy spot-check

- Re-derived the write-side guard's subject set from source:
  `grep -l 'os.MkdirAll(\|os.Mkdir(\|os.WriteFile(\|os.Create(\|os.OpenFile(\|os.Symlink(\|os.Link(' internal/fabricengine/*.go`
  (non-test) returns exactly `gitexclude.go warpbinding.go clone.go hook.go junction.go doc.go weftgit.go`
  — **7 files, matching `uncontainedwrite_test.go`'s allowlist exactly, 7 for 7.** R7's guard is
  still accurate against reality; no drift.
- Both guard tests are green (they run inside the `cmd/lyx` package, covered by the `-count=5` run
  above). I did not re-derive the destructive guard's inventory from scratch — the prompt explicitly
  says rounds 2 and 7 already did that and a sanity check is enough.

### Live driving (real substrate, `./deploy-dev` binary at `696979e2`)

Scratch hub built with plain `git init`: bare warp origin `myapp` + bare weft origin `myapp-weft`,
seeded with a `backend/` subtree so the hub could be anchored at a **non-`.` subpath** — deliberately,
since a root anchor collapses `_launchers/<AnchorRel>` onto `_launchers` itself and hides exactly the
geometry M1/L1 need. All commands run foreground, waiting for each to return.

- `lyx fabric clone <weft> <warp> --subpath backend` — **ok**, `anchor:"backend"`, 23-entry mutation
  record, hub laid out as `_board`/`myapp`/`myapp-weft`/`.lyx`. Matches `doc.go`'s clone-does-everything
  description exactly.
- `lyx fabric add task-a` — **ok**; record shows the full expected sequence (warp worktree+branch,
  weft worktree+branch, portal link, launcher dir + 2 scripts + menu launcher, junctions).
- `lyx fabric list`, `pairs`, `status` — **ok**; `pairs` reports `in_sync:true`,
  `junction_healthy:true`.
- `lyx fabric commit`, `reconcile`, `prune`, `cleanup`, `pull`, `checkout main` — all **ok**.
  `reconcile` reported `already_healthy` on a healthy hub and `warp_binding:"present"`; `cleanup`
  correctly reported `main` as `protected:true`; `pull` reported `weft_pulled:true`,
  `warp_fetched:true` with no rewrite — the vacuous-success path `doc.go` documents for a hub whose
  weft branch has no upstream yet.
- `lyx fabric diff HEAD` — errors `gitrepo: invalid SHA`. Correct (the verb takes a SHA, not a rev),
  and `Args: cobra.ExactArgs(1)` is set here — the one verb that validates its arguments.

### Adversarial scenarios (my own, beyond the suite)

**S1 — static escaping symlink at the `_launchers` container, WRITE side (R7's fix).**
With `<hub>/_launchers/backend` replaced by a symlink to an out-of-hub victim directory,
`lyx fabric add slug-one` failed closed:
`mkdir launcher dir …/_launchers/backend/slug-one: mkdirat _launchers/backend/slug-one: path escape`.
The victim directory was untouched. **R7's `writeLaunchers` rooting confirmed working against a
static pre-plant.** The failed add's rollback then correctly emitted R4-F2's WARN
(`rollbackAdd's warp-branch deletion was refused by the destructive gate; the branch is left behind`)
— that behaviour is live and reporting honestly.

**S2 — static escaping symlink at the `_launchers` container, DELETE side.** Same plant, with the
victim seeded with a matching `task-s/ide.sh` and a `canary.txt`. `lyx fabric remove task-s --force`:
victim **fully intact** (`canary.txt` and `task-s/ide.sh` both survive), so containment held and
nothing escaped. **But** the command reported `"ok":true,"partial":false,"links_removed":3` and its
mutation record carried **no launcher entries at all**, while `_launchers/backend.real/task-s/`
still held `ide.sh` and `fabric-checkout.sh`. → **finding L2**, confirmed live with no race.

**S3 — racing toggle at the `_launchers/<anchor>` component (M1's window).** Purpose-built Go
toggler flipping `<hub>/_launchers/backend` between a real directory and an escaping symlink, run
against 60 foreground `add`/`remove --force` cycles. **No escape observed.** Reported honestly: I
did **not** win this race. The window is two adjacent statements (`checkPathRequest` returns →
`os.Remove`), on the order of a microsecond, and my toggler was not inotify-triggered the way R6's
verification harness was. M1 is therefore graded CONFIRMED-by-trace / **not reproduced live**, and
the report says so rather than implying a live repro. I also confirmed live (S2) that the *static*
form is refused, which is consistent with the trace: M1 is race-only.

**S4 — arbitrary positional arguments on every verb.** See L3.

### Teardown

All scratch state lives under the session scratchpad
(`…/scratchpad/r8live`, `…/scratchpad/toggler`) and is removed at the end of Job 2. Verified zero
stray `git` processes and zero lock files outside the torn-down temp tree.

### What I could NOT verify, and why

- **Windows path behaviour** — permanently out of scope from a Linux host, as in every prior round
  (junction vs symlink semantics, `EvalSymlinks` over a directory junction).
- **M1 live** — the race window exists structurally but I did not win it in 60 trials (S3 above).
  Not an environment gap; an honest budget/precision limit, stated rather than papered over.
- **N4's dirtiness-probe TOCTOU** — deliberately not re-attempted, per the prompt's explicit
  instruction that it is a settled, accepted residual.

## Findings

### M1 (MEDIUM, CONFIRMED-by-trace) — `removeLaunchers`' launcher-directory removal is check-then-act on a nominal path, the exact R3 window the gate's two arbitrary-path executors closed

`internal/fabricengine/launchers.go:284-293`.

`removeLaunchers` runs the gate's full pipeline for the launcher DIRECTORY via a direct
`checkPathRequest(dirReq)` call, and then performs the act itself as a raw, nominal-path
`os.Remove(launcherDir)`:

```go
if err := checkPathRequest(dirReq); err != nil { ... }
removeErr := os.Remove(launcherDir)
```

It deliberately does not route through `removePath`, and the stated reason is sound — `removePath`'s
directory branch is `RemoveAll`, which would destroy foreign content the operator put beside the
launchers (`TestRemoveLaunchers_PreservesForeignContent`). But avoiding `removePath` also avoided
`removeContainedPath`, and with it the entire R3 fix. This site is therefore the one surviving
arbitrary-path removal in the package that still resolves its containment at one instant and unlinks
at a later one — precisely the window `destroy.go`'s own doc comment says the check alone cannot
close ("a symlink at an intermediate segment, dangling when the check ran and flipped
live-and-escaping before the executor acted, carried a gated removal outside the hub anyway").

**Failure scenario.** `AnchorRel` is a non-`.` subpath (say `backend`), so
`launcherDir = <hub>/_launchers/backend/<slug>`. A same-UID process toggles
`<hub>/_launchers/backend` between a dangling symlink and a symlink to `/victim` during
`lyx fabric remove <slug>`. With the link dangling, `refuseUncontainedPath` and `checkPathRequest`
both pass (`checkPathRequest` short-circuits to `nil` on an absent target via `os.Lstat`; the
ancestor-walk fallback in `resolveAncestorSymlinks` resolves the dangling tail lexically). The link
then flips live-and-escaping, and `os.Remove(<hub>/_launchers/backend/<slug>)` resolves `backend`
and unlinks `/victim/<slug>` — an out-of-hub deletion. Because `os.Remove` also removes plain files
and symlinks, the escaped target need not be an empty directory. On success the site appends
`KindPathRemoved` naming the **hub-relative** path the inode removed never was — the identical
false-success shape as R2's M3 and R3's M1.

Static (no-race) exploitation is refused: a pre-planted escaping symlink is caught by
`refuseUncontainedPath`. So this needs the same toggle race R3 reproduced, against the same threat
model R3 treated as real rather than theoretical.

**Suggested fix.** Route the act through the existing `removeContainedPath(launchersDir(l),
launcherDir, false)`. The non-recursive branch is `os.Root.Remove`, which the OS refuses on a
non-empty directory exactly as `os.Remove` does — so the preserve-foreign-content property is kept
verbatim — while component resolution and the unlink become one `openat` chain rooted at the
container. This also retires `launchers.go`'s `os.Remove(` entry from the destructive guard's
allowlist, which is the correct end state: the file should no longer need one.

### L1 (LOW, CONFIRMED-by-trace) — `pruneEmptyAncestors` relates and removes purely lexically, so a planted intermediate symlink walks it out of the hub

`internal/fabricengine/ancestors.go:103-129`, reached from `launchers.go:296` and `portals.go:112`.

The sweeper's boundary guard is `filepath.Rel(stop, cur)` over **nominal** strings — the lexical
comparison `containmentFailure` was rewritten away from in R2 — and its act is a raw
`os.Remove(cur)` on the nominal path. Both call sites walk the two hub-level structural containers
R7 identified as the attacker-plantable ones (`<hub>/_launchers`, `<hub>/_portals`).

**Failure scenario.** A multi-segment `AnchorRel` (e.g. `services/api`) makes
`start = <hub>/_launchers/services/api`, `stop = <hub>/_launchers`. A symlink planted at `services`
pointing to `/victim` makes the first `os.Remove(cur)` resolve `services` and remove
`/victim/api` — an out-of-hub removal. The loop then removes the `services` link itself and halts at
`stop`.

Materially weaker than M1 and correctly graded LOW: `os.Remove` on a directory is refused by the OS
the moment it is non-empty (the reason the guard allowlist gives), so only an EMPTY out-of-hub
directory can be destroyed; every error is swallowed; and nothing is appended to the mutation
record, so there is no false-success claim. It is nevertheless the last lexical-containment +
raw-nominal-act pair left in the package, and it sits on the same two containers the campaign has
now hardened on both the write and the delete side.

**Suggested fix.** Root the sweep: open an `os.Root` at `stop` and walk with `root.Remove(rel)`, so
both the boundary and the act resolve through the container's own handle. The lexical `Rel` guard
can stay as the loop's termination condition; what changes is that the removal itself can no longer
traverse an escaping component.

### L2 (LOW, CONFIRMED LIVE) — a containment refusal from `refuseUncontainedPath` is not a `*destructiveRefusal`, so `surfaceRefusal` silently discards it and `Remove`/`Prune` report `ok:true`

`internal/fabricengine/ancestors.go:30-35` (the producer), with the loss occurring at
`internal/fabricengine/remove.go:66,69` and `internal/fabricengine/prune.go:266,270`.

`removeLaunchers` and `removePortal` each open with a `refuseUncontainedPath` guard, which returns a
plain `fmt.Errorf`. Their four best-effort call sites all wrap the call in `surfaceRefusal`, which by
design returns `nil` for anything that is not a `*destructiveRefusal`:

```go
func surfaceRefusal(err error) error {
	if errors.As(err, new(*destructiveRefusal)) { return err }
	return nil          // <-- a containment refusal lands here
}
```

So the one refusal class `CONSTRAINTS.md` says must never be discarded — "A gate refusal is never
discarded on a best-effort path" — is discarded, precisely because this particular containment guard
predates the typed refusal and was never converted to it. `surfaceRefusal`'s own doc comment states
the intent exactly ("an operational failure stays discardable while a gate refusal never does"); the
type just does not match the intent here.

**Failure scenario (reproduced live, scenario S2 above, no race required).** Plant a static symlink
at `<hub>/_launchers/<AnchorRel>` pointing out of the hub, then `lyx fabric remove <slug> --force`.
Containment correctly refuses the whole launcher teardown — nothing escapes, which is the right
outcome — but the refusal evaporates, and the operator is told:

```
{"links_removed":3,"mutations":[…no launcher entries…],"ok":true,"partial":false,…}
```

exit 0, while `_launchers/<AnchorRel>/<slug>/{ide.sh,fabric-checkout.sh}` are still on disk. The
operator has no signal that a containment guard fired, no signal that launcher artifacts leaked, and
no reason to reach for `reconcile`. This is R2-M2's dishonest-success shape on the teardown path.

**Suggested fix.** Have `refuseUncontainedPath` return a `*destructiveRefusal`
(`Check: CheckContainment`, `What: what`, `Target: target`) instead of a bare `fmt.Errorf`. That is
the minimal change and the principled one: it makes the pre-gate containment guard the same type as
the gate's own containment refusal, so all four `surfaceRefusal` sites propagate it with no call-site
change at all, and `RefusalOf` starts answering for it too.

### L3 (LOW, CONFIRMED LIVE) — every fabric verb except `diff` accepts arbitrary extra positional arguments and silently ignores them

`internal/fabriccli/fabric.go` (all topology verbs) and `internal/fabriccli/weft_verbs.go`
(`status`/`commit`/`push`/`pull`/`sync`).

`diff` is the only command in the fabric subtree that sets an `Args:` validator
(`cobra.ExactArgs(1)`, weft_verbs.go:307). Every other command leaves `Args` nil, which cobra
defaults to `ArbitraryArgs` for a command with no subcommands. Extra arguments are therefore
accepted and dropped without a word.

**Failure scenarios, all observed live:**

- `lyx fabric unwire bogus-typo` → **performs the full unwire** (`junctions_removed:[".lyx","_lyx"]`,
  `board_junction_removed:true`), exit 0. A mistyped invocation of a deactivation verb executes
  rather than being rejected.
- `lyx fabric commit backend/_lyx/note.md` → `{"committed":true,…}`, exit 0. The named file is
  ignored entirely; the whole configured pathspec is committed. `commit`'s own `Long` says staging is
  pathspec-scoped and the message is fixed, so an operator reasonably reaching for a per-file commit
  gets a silent, different action reported as success.
- `lyx fabric add slug-one slug-two` → operates on `slug-one` only, `slug-two` dropped silently.
- `lyx fabric list bogus extra`, `lyx fabric pairs bogus`, `lyx fabric status bogus`,
  `lyx fabric reconcile bogus` → all run normally, ignoring the arguments.

This is squarely inside the CLI/Cobra Invariant's "help accuracy is a review obligation" clause: the
usage strings (`Use: "list"`, `Use: "unwire"`, `Use: "add <slug>"`) advertise an argument arity the
binary does not enforce.

**Suggested fix.** Set the correct `Args` validator on each command: `cobra.NoArgs` for `list`,
`pairs`, `reconcile`, `unwire`, `prune`, `cleanup`, `status`, `commit`, `push`, `pull`, `sync`;
`cobra.ExactArgs(1)` for `add` and `remove`; `cobra.MaximumNArgs(1)` for `checkout` (its `Use` is
`checkout [branch]`); `cobra.RangeArgs(1, 2)` for `clone`. Where a verb already hand-rolls its own
`usage: …` arity error, keep that message as the authority and let the validator only catch the
too-many case, so no existing error text changes.

## Docs & operability findings

Docs are in good shape. `internal/fabricengine/doc.go`, `CONSTRAINTS.md`, and `docs/overview.md`
were read against the code and I found **no doc/code drift** — the R6 and R7 sections in both
`doc.go` and `CONSTRAINTS.md` accurately describe `containedWorktreeAdd`, `writeLaunchers`, and
`createPortal` as shipped, and `createExclusiveDir`'s doc comment carries R7-F4's corrected,
appropriately-narrow guarantee.

Two documentation consequences fall out of the findings above and are fixed with them, not
separately:

- `CONSTRAINTS.md`'s Destruction Chokepoint Invariant says "The two arbitrary-path executors
  (`removePath`, `removeLink`) therefore remove through `removeContainedPath`". There is in fact a
  **third** arbitrary-path removal (M1), and the invariant's own text is what makes it visible as an
  omission rather than a design choice. Fixing M1 makes the existing sentence true as written.
- The destructive guard's allowlist entry for `launchers.go` ("removeLaunchers runs the gate's own
  `checkPathRequest` immediately before its `os.Remove(launcherDir)` call") documents exactly the
  check-then-act shape M1 describes. Fixing M1 lets that entry be deleted outright, which is the
  right end state: the file should not need one.

Operability: the mutation-record envelope behaved correctly on every path I drove, including the
failure paths (`partial:false` with an empty record on a pre-flight failure; the record surviving
the error return on the failed `add`). `nameStrandedPortalTeardown`'s remedy text and R4-F2's
rollback WARN both fired live and read well.
