# Fabric crucible follow-ups — slices 12-15

> **Status: none of the four slices is built.**
> This file is the durable, versioned source of truth for what each must do.
> Per the [documentation lifecycle](../../docs/overview.md#documentation-lifecycle) it is deleted once all four have landed, with their durable rationale folded into `internal/fabricengine`'s package doc.

The fabric v2 crucible campaign (slice 11, landed 2026-08-09) ran **six serial model-rotating review+fix rounds** against the slice 1-10 rewrite: R1 Opus high, R2 Fable high, R3 Opus high, R4 Opus medium, R5 Opus medium, R6 Fable high.
It produced **81 findings, 9 BLOCKING, 8 of them data-loss**, and every round's verdict was independently verified by an orchestrator that re-ran the gates itself, sabotage-proved each new test, and re-drove BLOCKING fixes live — four of five verified rounds had something material the round's own report did not surface.

Every individual defect is fixed and merged.
What is *not* fixed is the four shapes that kept producing them, which the orchestrator filed as GitHub issues #146, #143, #144 and #148 rather than as more per-instance fixes.
This file folds those issues into the manifest as buildable slices;
the issues themselves are closed, pointing here.

The finding count per round did not converge.
That is the single most important fact about this campaign, and the reason these four slices exist: the campaign was draining a class instance by instance, and the class has more instances than any one round's method can reach.

## The eight data-loss defects — the shared evidence table

Referenced by every slice below;
stated once here.

| round | verb | what it destroyed |
|---|---|---|
| R1 | `reconcile` | a tracked symlink in the warp worktree |
| R2 | `pull` | uncommitted tracked warp work, via `ResetHard` on every advance path — and returned `ok:true` |
| R3 | `remove` | the warp worktree directory, via an `os.RemoveAll` fallback past a git refusal |
| R3 | `cleanup` | the primary weft branch |
| R3 (found by the orchestrator, not the round) | `prune` | a path git had just refused to remove |
| R4 | `prune` | any hub child whose name ends in the weft suffix — an ordinary user directory, or an unrelated git clone parked there |
| R4 | `clone --reset` | any directory named `<derived>-HUB`, hub or not, on a name fabric *derives* rather than one the operator types |
| R5 | `remove ..` | the entire hub — warp clone, weft clone, `_board`, every pair, all uncommitted work — then reported `{"error":"failed to check worktree status","ok":false}`, i.e. claimed it had done nothing |

Two facts about this table drive the whole ordering below.

**All eight were found by driving real `git` against a real filesystem in hostile or dirty state.**
`go build ./...`, `go vet ./...` and `go test ./...` were green before, during and after each of them existed in the tree.

**The eight are one shape, not eight mistakes** — a destructive operation acting on a path it does not own, or without checking whether there is uncommitted work to lose — found in six different files across five different rounds.

## Build order and why

`12 → 13 → 14`, with `15` independent and parked at the tail.
The slice numbers are assigned in build order, so numeric order is always build order.

The root-cause fix (the destructive-operation chokepoint) is **slice 13, not slice 12** — it is second, not first, and the single sentence justifying that is:

> Slice 13 is a consolidating refactor of the exact code paths that destroyed something eight times, and the only test tier that can observe destruction is the one slice 12 builds.

Doing the refactor first means rewriting fabric's highest-blast-radius code with a suite that provably cannot see the failure mode it is being rewritten to prevent.
That is how bug nine happens.
Slice 12 is deliberately kept small for this reason — see its scope note: it must cover the destructive verbs before slice 13 starts, and only needs to grow to the full cross product afterwards.

The rest of the order:

- **Slice 12 first** — the instrument.
  Additive (`//go:build integration`), touches no production code, and its hostile-state matrix is likely to surface further instances of the slice-13 class *before* the gate is designed, which is worth more than the same instances surfacing after.
  Note what it does and does not prove: proving that no *call site bypasses* the gate is the job of slice 13's own static guard test, not of the harness;
  the harness proves the gate *behaves* correctly once reached, against real git in dirty and hostile state.
  Both are needed and they are different mechanisms.
- **Slice 13 second** — the safety fix, landing as early as it can be landed with cover.
  Its steps 1-4 (containment, ownership, dirtiness, force semantics) are what actually stop destruction and depend on nothing but slice 12.
  Its step 5 (honest reporting) is truthfulness, not safety — `remove ..` would have been stopped dead by step 1 alone — so step 5 is allowed to land in each verb's existing error shape and be generalised by slice 14.
  That is a deliberate, bounded churn cost, paid to get the safety fix in earlier.
- **Slice 14 third** — the envelope.
  It generalises slice 13's per-verb refusal reporting into one accumulate-as-you-mutate shape, and it completes slice 12: a harness cell is only fully meaningful when it asserts both "the operator's file is still on disk" *and* "the report was truthful", because case after case in the table above returned an error **and** destroyed something.
- **Slice 15 last** — LOW, self-healing, unrelated to the destruction class, and gated on a locking decision rather than on any of the others.

Nothing here runs usefully in parallel.
12 and 14 both concern the same result shape from opposite sides, and 13 sits between them by design.

## Slice 12 — live-state integration harness

Issue #144 (`enhancement`).

### Why

Every one of the eight data-loss defects was found by driving real git against a real filesystem with hostile or dirty state.
**Not one was found by the hermetic suite, which was green throughout.**

That is not a gap in any individual test.
It is a gap in what the suite is *able* to express: it does not build a real hub, so it cannot put one into a dirty or hostile state, so it cannot observe a verb destroying something.

The reason is structural and correct.
The hermetic tier is bound by CONSTRAINTS.md's [Test Tier Purity Invariant](../../CONSTRAINTS.md#test-tier-purity-invariant): untagged test files must not call `gitexec.RunGit`, `exec.Command*`, or `lyxtest.Copy*`.
That invariant stays — it is what keeps the fast tier fast.
But it means the fast tier can never observe a verb's behaviour against real git, and today the `//go:build integration` tier covers that ground **per-verb and ad hoc**, only where someone thought to write a test.

The campaign proved the alternative works.
R3 drove an explicit 40-cell matrix — every mutating verb (`pull`, `sync`, `checkout`, `reconcile`, `remove`, `cleanup`, `unwire`, `prune`) × dirty warp / dirty weft / both — and found a BLOCKING defect.
The orchestrator's note at the time, carried into the campaign handoff:

> Both data-loss bugs in this campaign are destructive git operations with no dirty-worktree check, and both were invisible until someone ran the verb with uncommitted work on disk.
> Neither round covered that axis *systematically* — each hit it once by luck of scenario choice.
> **A third lucky hit is not a convergence signal; an exhaustive clean sweep of that matrix is.**

R5 then extended the idea to axes nobody had driven — concurrency between worktrees, the hook surface, `_portals`/`_launchers` — and found 2 BLOCKING and 5 MEDIUM in 24 scenarios across 18 independent hubs.
The pattern is consistent: point a systematic live matrix at fabric and it finds something;
the hermetic suite finds none of it.

### What needs to happen

An integration-tier harness (`//go:build integration`) providing:

1. **A hub factory.**
   Build a real hub from local bare remotes.
   The campaign's recipe is known and has two gotchas worth encoding rather than rediscovering: `git init --bare` leaves `HEAD` on `master` while the pushed branch is `main` (needs `git -C <bare> symbolic-ref HEAD refs/heads/main`), and the weft remote must be genuinely empty or clone's bootstrap guard refuses it.
   One independent hub per destructive scenario, never shared.
2. **A state matrix.**
   Named hostile states applied to a fresh hub: clean, dirty warp (tracked), dirty warp (untracked), dirty weft, both dirty, tracked symlink present, foreign directory at a fabric-owned path, unrelated git clone parked at a fabric-named path, stale portal link, non-executable user hook, `core.hooksPath` set.
3. **A verb table.**
   Every exported verb with its arguments, including hostile inputs — `""`, `.`, `..`, `../x`, `-weft`-suffixed, reserved hub names, a leading `-`.
4. **The cross product driven, with per-cell assertions on what must survive.**
   The critical assertion is **not** "the verb returned an error" but "the operator's file is still on disk" — case after case in the evidence table returned an error *and* destroyed something.
   Once slice 14 lands, each cell also asserts the report was truthful.
5. **Subpath coverage.**
   Every cell runnable on a `--subpath backend` hub as well as a `.`-anchored one.
   The campaign's number one concern throughout was the anchor/subpath mechanism, and it is the axis most likely to differ.

The point is the **cross product**, not the individual cells.
A new verb added to the table inherits every state;
a new state inherits every verb.
That is the property the current per-verb integration tests do not have, and it is precisely how `remove` escaped four consecutive review rounds.

### Scope note — build the minimum that covers slice 13, then grow

This slice does **not** have to ship the full cross product before slice 13 starts, and deliberately should not.
The blocking subset is: the hub factory, the dirty/hostile states that the evidence table's eight defects actually exercised, and the hostile-input row of the verb table driven against every destructive verb.
That is enough to refactor the destructive paths under cover, which is the whole reason this slice is sequenced first.

The remaining axes — concurrency between worktrees, the hook surface, `_portals`/`_launchers`, and full subpath coverage of every cell — are additive and land after slice 13, alongside slice 14's truthfulness assertions.
Growing the matrix is cheap once the factory exists;
holding slice 13 hostage to a complete matrix is not.

### Scope, cost and risk

Fabric is unusually well suited to this: it spawns no LLM subprocess, its substrate is real git plus the real filesystem, so live driving is cheap and there is no concurrency ceiling.
The campaign ran 18 independent hubs in a single round without trouble.

Runtime is the real cost.
This belongs behind the `integration` tag and must not be in the default `go test ./...` path.
Measure wall-clock before the matrix grows large;
parallelising per-hub is straightforward since every cell owns its own hub.

**Known limitation, stated up front rather than discovered later:** Windows path behaviour (junctions vs symlinks, case-insensitive compare in `lyxcwd.samePath`) cannot be exercised on a Linux host.
The campaign carried this as a permanent known gap across all six rounds rather than pretending to have verified it, and this harness inherits that gap honestly — see the Someday `fabric: Windows path behaviour is unverified` item and [fabric-windows-verification.md](fabric-windows-verification.md).

## Slice 13 — route every destructive operation through one ownership-and-dirtiness gate

Issue #146 (`bug`).
This is the root-cause slice.

### Why

The eight data-loss defects are eight instances of one shape spread across six files: **a destructive operation acting on a path it does not own, or without checking whether there is work there to lose.**

The structural cause is that fabric has **no common chokepoint for destruction**.
Every call site implements its own ownership check and its own dirtiness check, or forgets to.
That is why each round found a new one.

R4 enumerated **28 destructive sites** across the fabric packages and closed the two live defects in that enumeration.
R5 then found two more in regions R4's sweep did not cover — `remove`'s slug door and the shared `.git/info/exclude` — which is the evidence that enumerate-and-fix does not terminate here.

### What needs to happen

Every ingredient already exists in the tree.
They are just scattered, and applied by whichever call site remembers to apply them:

- `refuseUncontainedPath(container, target, what)` — `internal/fabricengine/ancestors.go`, added by R5.
- `isRegisteredLinkedWorktree` / `isRegisteredLinkedWorktreeIn` — "is this git's, and this hub's?"
- `applyStalePairOwnership` — `prune.go`, R4's ownership gate, deliberately NOT bypassed by `--force`.
- `looksLikeHub` — `clone.go`, R4's hub predicate.
- `refuseDirtyWeftWorktree` / `applyStalePairProtection` — the tracked-changes probes.
- `validateWorktreeSlug` — `slug.go`, which currently has **two** call sites (`add.go`, `remove.go`) against **eight** exported `Topology` verbs.
- `var RemoveAll = os.RemoveAll` — `clone.go:32`, an existing test seam that shows the routing idea already works.

Consolidate these into one path every destructive operation must go through, enforcing in a fixed order:

1. **Containment** — the target resolves strictly below the container it is supposed to be inside.
2. **Ownership** — the target is something fabric created and this hub owns, not merely something whose name matches a pattern.
3. **Dirtiness** — there is no uncommitted tracked work being discarded, unless `--force` was given.
4. **Force semantics** — `--force` answers "discard work I might want", and must **never** answer "delete something that was never fabric's".
   R4 established this seam and verified it live;
   the chokepoint makes it structural rather than per-site discipline.
5. **Honest reporting** — a refusal names which of the four gates refused and why, and a failure never reports success.
   This step, and this step only, may land in each verb's existing error shape — slice 14 generalises it into one envelope afterwards.
   Steps 1-4 are the safety half and must not be deferred that way.

Then **prove no call site bypasses it**.
That is the part that turns this from another fix into a closed class:

- A graded sweep (the crucible's form) showing every one of R4's enumerated destructive sites routes through the gate.
- A guard test that fails when a new raw `os.RemoveAll` / `os.Remove` / `git worktree remove --force` appears outside the gate.
  The cheapest shape matching how this repo already enforces structure is a test that walks the tree, in the manner of the Test Tier Purity Invariant's guard — see the open question below.
- Slice 12's harness driving the hostile-input row of its verb table against every verb, which is what keeps proving it.
- A **CONSTRAINTS.md invariant in the same commit**.
  This is exactly the cross-cutting kind that file exists for, and CLAUDE.md requires it in the same commit anyway.

### Fold in R6's validation-asymmetry class

`validateWorktreeSlug`'s two-of-eight call-site ratio was enumerated by crucible R6 as a separate class — validation asymmetry across entry points.
Whatever R6 recorded is folded in **here** rather than fixed twice: the two classes meet at exactly the door where `remove ..` got in.

### Open questions

- **Does the gate belong in `internal/fabricengine`, or lower** — beside `internal/fslink` / `internal/gitrepo` — so non-fabric callers get it too?
  Leaning fabricengine first, since all eight defects were fabric's, and generalising later is cheaper than a premature abstraction.
- **The dirtiness probe is deliberately tracked-only today** (`git status --porcelain --untracked-files=no`).
  That decision is reasoned and documented in `prune.go`;
  the chokepoint inherits it rather than silently widening it, because refusing on untracked files would make `prune` useless on exactly the debris it exists to clear.
- **How to enforce "no new raw destructive call" mechanically.**
  Options: a test walking the AST, a `golangci-lint` forbidigo rule, or the existing grep-the-tree pattern.
  The last is cheapest and matches the repo's existing guards — but see the Someday `lyx has ~15 home-grown static-analysis guards` item (issue #135) before adding a sixteenth hand-rolled walk.

## Slice 14 — accumulate the result envelope from mutations, not from control flow

Issue #143 (`bug`).

### Why

Fabric's verbs assemble their JSON result envelope from control flow at the end of the call, rather than accumulating it from mutations as those mutations actually succeed.
The consequence is that the envelope can contradict what happened on disk — and it has done so twice, in opposite directions.

This is the only defect class the campaign found where the failure **actively misleads the operator**.
The other classes produce bad help;
this one produces false information, in the exact situation where the operator most needs the truth.

**1. `pull` reported success after destroying uncommitted work** (R2, finding `R2-1`, BLOCKING).
Every warp advance went through `ResetHard` — the fast-forward path and the re-anchor path alike.
A routine `lyx fabric pull` with a dirty tracked file in the warp worktree discarded that file and returned `{"ok":true, ...}`.
The operator's own verification, recorded in the campaign handoff: with the dirty gate neutered, `TestPull_DirtyWarpRefusesBeforeMovingWarp` fails at `Pull() error = <nil>; want ErrWarpDirty`.
Live re-drive after the fix: pull refuses, and the uncommitted line survives on disk.

**2. `remove ..` reported failure after destroying an entire hub** (R5, finding `B0`, BLOCKING).
`validateWorktreeSlug` rejected the empty string, path separators, the `-weft` suffix and every reserved hub name — but not `.` and `..`.
`removeLaunchers` then resolved `<hub>/_launchers/./..` to `<hub>` and `os.RemoveAll`'d it.
It then returned `{"error":"failed to check worktree status","ok":false}` — the envelope claiming *nothing happened* immediately after the most destructive act the tool can perform.
The observation in R5's report: `[ -d $HUB ]` was false afterwards.

The two cases invert in opposite directions.
A single missing check cannot produce both.
What produces both is that **the envelope is derived from where control flow ended up, not from what was done**:

- `ok:true` is the default for "we reached the end without returning an error", so an operation that destroyed something and then succeeded structurally reports success.
- `ok:false` with a message is derived from whichever error happened to be in hand when the function returned, so a late failure overwrites the record of an earlier successful (and destructive) step.

Neither path consults what actually changed on disk, because nothing tracks that.

### What needs to happen

Make the result value accumulate as work completes, instead of being composed at the end:

1. Each verb's result type gains a record of what was actually mutated — worktrees removed, directories deleted, branches moved, files rewritten.
   Appended at the point the mutation succeeds, never inferred afterwards.
2. `ok` becomes a statement about that record plus the error, not a synonym for "no error was returned".
   A verb that removed three of five pairs and then failed reports the three, the failure, and `ok:false` — which is the honest answer and is currently unrepresentable.
3. A verb that returns an error while its mutation record is non-empty must say so explicitly.
   Today that combination is silently indistinguishable from a clean refusal, which is exactly what made case 2 so dangerous.

**Prior art already in the tree:** `PruneEntry` does roughly the right thing — it carries `Removed`, `Protected`, `Unowned` and `Error` per entry, so a dry run reports exactly what `--apply` would do, and a refusal names which gate refused.
That per-entry honesty is what the top-level envelope lacks.
Generalise it rather than inventing a second vocabulary.

### Scope and risk

Confined to `internal/fabricengine` and `internal/fabriccli`.
It changes JSON output shape, so anything parsing fabric's output is affected — enumerate the consumers before starting;
within loomyard they are known, and `internal/boardengine` is the one to check first since it routes through `CommitWeftAt`/`PushWeftAt`.

Sequenced **after** slice 13, not before: the gate's own refusal reporting (its step 5) lands in each verb's existing error shape first, and this slice is what generalises that into one accumulate-as-you-mutate envelope.
Deferring the envelope is a deliberate, bounded churn cost, paid so the safety half of the gate lands a slice earlier.

## Slice 15 — corrindex two-phase read-modify-write races an unlocked RebuildIndex

Issue #148 (`bug`), severity **LOW — self-healing**.
Independent of slices 12-14;
parked at the tail so it does not delay the class-closing work.

### The mechanism

`internal/fabricengine/corrindex.go:48` — `record()` builds `next` from `ix.recs`, an in-memory snapshot loaded earlier by `loadCorrIndex` under a read lock **that has already been released**.
It then writes the whole array via `state.WriteJSON`.
That write is itself flock'd, so the *write* is atomic;
the problem is the window between load and write.
Another process that wrote in that window has its entry clobbered, because `next` was composed from a base that no longer reflects the file.

`internal/fabricengine/index.go:416` — `RebuildIndex` writes the same file, flock'd on the file's own lock but **not** holding the weft write lock, so it does not serialise against `record()`'s two-phase window.

A `commit` racing a `diff`/`revert` on one pair can therefore transiently drop an index entry.

### Why it is LOW

The commit trailers are the sole source of truth;
the index is an explicitly rebuildable cache.
A dropped entry is reconstructed by the next stale-hit rebuild.
The worst observable effect is a single spurious `no_weft_correspondence` from `lyx fabric diff` that a re-run clears.

### Why R6 did not fix it

The clean fix is for `RebuildIndex` and `refreshCorrIndexAfterSwitch` to acquire the weft write lock so they serialise against `record()`.
That is only safe given that no already-under-lock caller reaches `RebuildIndex` — a whole-package deadlock analysis.
The known call ordering, as traced by R6: `pull.go:253` calls `RebuildIndex` **before** taking its own lock at `pull.go:299` (safe today), and `commitWeftLocked` / `commitEmptySnapshot` never call it.

So the change looks tractable, but it is a cross-path locking decision rather than a mechanical edit, and its payoff is closing a self-healing LOW race.

**Preferred shape:** make `record()` single-phase by re-reading under the write lock it already takes.
That is local to `corrindex.go`, needs no cross-path analysis at all, and closes the clobber window without touching `RebuildIndex`'s locking.
Weigh it against the weft-lock version before choosing.

### Verification status

**CONFIRMED by code inspection, NOT reproduced as a runtime failure.**
R6 traced it;
the orchestrator independently read both call sites and confirmed the two-phase structure and the missing weft lock.
Neither party made it fail on demand.
Stated plainly because the campaign's own standard is that a race reasoned about but never driven is not a finding — this is filed as a characterised risk, not as a proven defect.
R6 recorded it as incidental observation O1, outside that round's two-part assignment, and deliberately did not fix it.

## Related

- [fabric-unified-view.md](fabric-unified-view.md) — the slice 1-10 campaign these four slices follow;
  slice 11 was the crucible hardening pass itself.
- [fabric-windows-verification.md](fabric-windows-verification.md) — the Someday platform gap slice 12 inherits honestly rather than closing.
- [gitexec-error-shape.md](gitexec-error-shape.md) — the fifth class the campaign surfaced, scoped out of these slices because its blast radius is every module that touches git, not fabric.
- [CONSTRAINTS.md](../../CONSTRAINTS.md) — where slice 13's invariant goes;
  its [Fabric Git Invariant](../../CONSTRAINTS.md#fabric-git-invariant-warp--weft) and [Test Tier Purity Invariant](../../CONSTRAINTS.md#test-tier-purity-invariant) are the two these slices sit next to.
- `internal/fabricengine` package documentation — where the durable rationale folds when this file is deleted.
- The campaign's own artifacts — `.scratch/fabric-review-HANDOFF.md` and the per-round orchestrator verifications (`fabric-review-r3-orchestrator-verification.md`, `-r4-`, `-r5-`, and `fabric-review-opus-medium-r4.md`'s 28-site destructive enumeration) — lived in the `fabric-v2-crucible` worktree and were never merged.
  Everything durable from them that these slices need is restated above;
  treat that worktree as gone.
