# Fabric crucible follow-ups — slices 12-15

> **Status: slice 12 landed (2026-08-11); slices 13-15 not yet built.**
> This file is the durable, versioned source of truth for what each must do.
> Per the [documentation lifecycle](../../docs/overview.md#documentation-lifecycle) it is deleted once all four have landed, with their durable rationale folded into `internal/fabricengine`'s package doc — slice 12's own share of that rationale already lives there (see doc.go's "The destruction chokepoint" section).

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

`12 → 13 → 14 → 15`, strictly serial — **one fabric slice in flight at a time**.
The slice numbers are assigned in build order, so numeric order is always build order.

The chain is serial for two independent reasons, and both have to hold before two slices may overlap:

- **Logical** — each slice asserts on behaviour the previous one changes (argued per slice below).
- **Mechanical** — every one of these slices edits `internal/fabricengine`, and slices 12 and 14 each rewrite it package-wide: 12 rewires roughly 29 destructive call sites and lands a static guard over the whole tree, 14 rewrites every verb's result path.
  Two agents in that package at once is a merge conflict, not a schedule win.

Slice 15 is the one whose serialisation rests only on the mechanical reason.
It is logically independent of 12-14 — a locking race in `corrindex.go`, touching nothing the other three touch — and an earlier draft of this file therefore declared it free to pick up at any point.
That was wrong: logical independence does not make it safe to edit the same package alongside a package-wide refactor.
It is LOW and self-healing, so it loses nothing by waiting, and the tail is where it already sat.

**The root-cause fix — the destructive-operation chokepoint — is slice 12, and it goes first.**
It is the only slice that stops anything being destroyed;
everything else is instrumentation, truthfulness, or a self-healing race.

An earlier draft of this file put the harness first, on the argument that the chokepoint is a consolidating refactor of the exact paths that destroyed something eight times, and that the only tier able to observe destruction was the one the harness builds.
**That argument was wrong, and the correction matters enough to record rather than quietly delete**, because it is the kind of reasoning that sounds right and delays a safety fix by a whole slice.

It conflated two different jobs:

- **Regression cover for the refactor** — "did I break a guarantee that already holds?" — *already exists*.
  The campaign fixed each of the eight defects **with a named, sabotage-proved test**: `TestPull_DirtyWarpRefusesBeforeMovingWarp`, `TestPrune_RefusesHubDirectoryItDoesNotOwn`, `TestPrune_RefusesUnrelatedGitCloneInHub`, `TestPrune_ProtectsDirtyWeftWorktreeUntilForced`, `TestCleanup_ProtectsPrimaryWeftBranchAfterCheckout`, `TestCleanup_PrimaryBranchSurvivesForceWhenNotCheckedOut`, `TestAdd_RejectsReservedHubNameSlug`, and the `remove_guard`/`remove_reserved` integration files — inside roughly **29 integration test files** covering the destructive verbs.
  A consolidating refactor is exactly the change those tests are good at policing.
- **Discovery of instance number nine** — that is the harness's job, and it is a *finding* task, not a refactor-safety one.

And the chokepoint's own completeness proof — the static guard showing no call site bypasses the gate — is a tree walk that needs no fixtures at all.

Two further reasons the chokepoint leads:

- Slice 14 rewrites **every verb's result path**, the destructive ones included.
  Landing the gate first means that work happens on already-gated code.
- The class is open in the sense that a future change can add a ninth instance — and there are three more fabric slices immediately after this one.

The rest of the order:

- **Slice 13 second, and genuinely after 12 rather than merely beside it** — the instrument.
  Additive (`//go:build integration`), touches no production code.
  Its first job is to validate the gate slice 12 built, against real git in dirty and hostile state;
  its second is to find instances nobody has thought of.
  It **depends on** slice 12 rather than running alongside it, for the same reason slice 14 does: its cells assert on *refusal* behaviour — that a verb refuses instead of destroying, and which of the four checks refused — and slice 12 is precisely what changes that behaviour and those messages.
  Cells written before the gate would be rewritten after it.
  The hub factory, state matrix and verb table are admittedly gate-independent and could be built in parallel, but that is about a third of the task;
  the valuable two thirds is not.
  Note the division of proof: *no call site bypasses the gate* is slice 12's static guard;
  *the gate behaves correctly once reached* is the harness.
  Different mechanisms, both needed.
- **Slice 14 third, and after 13 rather than beside it** — the envelope.
  It generalises slice 12's per-verb refusal reporting into one accumulate-as-you-mutate shape, and it completes slice 13: a harness cell is only fully meaningful when it asserts both "the operator's file is still on disk" *and* "the report was truthful", because case after case in the table above returned an error **and** destroyed something.
  That is the same argument that puts 13 after 12, applied one slice later: 14 changes the report shape 13's cells assert on, so cells written beside it would be rewritten after it.
  It also rewrites every verb's result path, which is the whole package 13's harness is built to drive.
- **Slice 15 last** — LOW, self-healing, unrelated to the destruction class, and gated on a locking decision rather than on any of the others.
  Serialised behind 14 on the mechanical reason alone (see above), not because it needs anything 14 produces.

The chain is strict and total: `12 → 13 → 14 → 15`, one in flight at a time.
Nothing here may be picked up beside an in-flight fabric slice.

## Slice 12 — route every destructive operation through one ownership-and-dirtiness gate

**Landed 2026-08-11.**
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

### What "one entry point" must mean — three precisions

**One shared pre-flight in front of several destructive primitives, never one delete function.**
Destruction in fabric is not one operation.
It is at least five, spread across a dozen files:

| primitive | current sites |
|---|---|
| `os.RemoveAll` / `os.Remove` | `remove.go`, `prune.go`, `clone.go` (via the `RemoveAll` seam), `launchers.go`, `junction.go`, `index.go`, `hook.go` |
| `git worktree remove [--force]` | `remove.go`, `prune.go`, `add.go`, `weftwiring.go` |
| `git branch -D` | `cleanup.go`, `checkout.go`, `add.go`, `weftwiring.go` |
| `ResetHard` | `pull.go`, three call sites |
| exclude rewrite / link removal | `gitexclude.go`, `junction.go` |

If the chokepoint is read as "one function that deletes a directory", R2's `ResetHard` defect walks straight past it — and that is exactly how R2's defect worked.
The gate is the four checks;
the primitives are what it fronts.

**The gate executes, it does not merely approve.**
A gate the caller consults and then deletes on its own behalf is a rule someone must remember to follow, which is the failure mode this whole slice exists to end — 28 destructive sites each remembering their own checks is how the class was produced.
A gate that performs the act makes `os.RemoveAll` outside one file mechanically bannable, which is what reduces the bypass guard to a trivial file-scoped scan.
This is [overview.md](../../docs/overview.md#principles)'s principle 6 — make the correct path the path of least resistance and make drift detectable — applied to fabric's own internals.
`clone.go:32`'s `var RemoveAll = os.RemoveAll` already demonstrates the routing works.

**One file in `internal/fabricengine`, not a sub-package — at least first.**
A sub-package (`internal/fabricengine/destroy`, or a lower leaf beside `internal/fslink`) is the shape that first suggests itself, and it has a concrete blocker: the predicates the gate needs — `isRegisteredLinkedWorktree`, `looksLikeHub`, `applyStalePairOwnership`, `refuseDirtyWeftWorktree`, weft path construction — are `fabricengine`-private.
A sub-package importing `fabricengine` for them while `fabricengine` imports the sub-package for the gate is an import cycle Go forbids.
A sub-package therefore only works if the gate is **told** its predicates and never derives them — the [Treadle Runner-Seam Invariant](../../CONSTRAINTS.md#treadle-runner-seam-invariant) pattern, which is a real option but a larger design.

Nothing is lost by starting in-package.
Enforcement does not need a package boundary: `internal/scoutengine/lspclient_guard_test.go` already polices a **single file**, so "the only file allowed to call `os.RemoveAll`" is exactly as machine-checkable as "the only package".
Extract to a told-everything leaf later if a non-fabric caller appears — which is the open question below, and the reason this slice does not settle it up front.

### Where the gate lives vs where the primitives live

**The gate is fabric's, and that follows from its own checks rather than from habit.**
Of the four, two are irreducibly fabric-domain: *ownership* ("fabric created this and this hub owns it" — hub geometry, the weft suffix, registered linked worktrees, `looksLikeHub`) and *force semantics* (whose whole content is "never delete something that was never fabric's").
`gitrepo.Repo` has no concept of a hub, a pair, a weft sibling or a prime, and must not acquire one.
Moving the gate down would drag fabric's domain model with it, which is not a move.

The **primitives** it fronts split three ways, and only the middle row is an open call:

| primitive | home | why |
|---|---|---|
| `ResetHard` | **already `gitrepo`** | `gitrepo/reset.go`, SHA-validated, with its data-loss surface documented at `gitrepo/doc.go:227`; `fabricengine.ResetHard` is thin delegation. The precedent that makes this question worth asking. |
| tracked-only dirtiness probe | **candidate for `gitrepo`** | see below |
| `git worktree remove --force`, `git branch -D` | **stays in `fabricengine`** | `gitrepo.Repo` models *one local checkout*; both act on a path other than the repo they run from, so they are topology, not checkout, operations. Pushing them down grows gitrepo a worktree-topology surface for a single consumer, against its own documented boundary. |
| `os.RemoveAll` / `os.Remove` | **`fabricengine`** | not a git operation at all. |

**The one deferred decision, with its criterion stated so the implementing agent applies a rule rather than a preference.**
`gitrepo` nearly has the dirtiness probe already: `WorktreeChangedFiles()` (`gitrepo/worktree.go`) returns every uncommitted change but **includes untracked files**, while this gate's probe is deliberately tracked-only.
A tracked-only variant is trivial in go-git — skip `git.Untracked` entries — so it needs no `gitexec` call and therefore no update to the [gitrepo Client Boundary Invariant](../../CONSTRAINTS.md#gitrepo-client-boundary-invariant)'s pinned list.

Meanwhile `fabricengine` hand-rolls `git status --porcelain` at **eight** sites — `add.go`, `checkout.go`, `prune.go`, `pull.go`, `remove.go` (twice), `warpclean.go`, `reconcile.go` — four of them with `--untracked-files=no`.
That is the same "every call site rolls its own check" disease this slice exists to cure, one layer down.

Promote the probe into `gitrepo` **if** the gate ends up needing it against both warp and weft through a `Repo` handle it already holds;
keep it fabric-local **if** the gate only ever probes paths it resolves itself, since a `gitrepo` method would then need a `Repo` constructed solely to answer one question.
Either way the eight hand-rolled probes collapse to one implementation — that part is not optional, and is the smaller half of this slice.

### Prove no call site bypasses it

That is the part that turns this from another fix into a closed class:

- A graded sweep (the crucible's form) showing every one of R4's enumerated destructive sites routes through the gate.
- A guard test that fails when a new raw `os.RemoveAll` / `os.Remove` / `git worktree remove --force` appears outside the gate.
  The cheapest shape matching how this repo already enforces structure is a test that walks the tree, in the manner of the Test Tier Purity Invariant's guard — see the open question below.
- Slice 13's harness driving the hostile-input row of its verb table against every verb, which is what keeps proving it — it arrives after this slice lands, so it validates the gate rather than gating it.
- A **CONSTRAINTS.md invariant in the same commit**.
  This is exactly the cross-cutting kind that file exists for, and CLAUDE.md requires it in the same commit anyway.

### Fold in R6's validation-asymmetry class

`validateWorktreeSlug`'s two-of-eight call-site ratio was enumerated by crucible R6 as a separate class — validation asymmetry across entry points.
Whatever R6 recorded is folded in **here** rather than fixed twice: the two classes meet at exactly the door where `remove ..` got in.

### Open questions — resolved on landing

- **Does the gate belong in `internal/fabricengine`, or lower?**
  Resolved: it stays in `internal/fabricengine`.
  All eight defects were fabric's, and every predicate the gate resolves — hub geometry, the weft suffix, registered linked worktrees, `looksLikeHub` — is `fabricengine`-private;
  generalising later, if a non-fabric caller ever appears, is cheaper than a premature abstraction now.
- **~~The dirtiness probe is deliberately tracked-only today~~ — stale, corrected on landing.**
  That earlier sentence over-generalised one verb's comment to the whole package, and the verified site-by-site split contradicts it: four of the eight `git status --porcelain` sites the gate consolidates are tracked-only and four are untracked-inclusive.
  The decision actually taken: dirtiness scope is a caller-declared member of a closed sum type (`dirtyScopeTracked` / `dirtyScopeAll` / `dirtinessNA`), and every site keeps the scope it already had.
  Normalising every site to tracked-only would have opened a new data-loss path: git's own untracked-file refusal on `git worktree remove` would then route into a directory-removal fallback with no equivalent protection.
- **How to enforce "no new raw destructive call" mechanically?**
  Resolved: the existing grep-the-tree pattern, matching the repo's other guards (`cmd/lyx/destructiveguard_test.go`).
  The broader consolidation question — a shared static-analysis-guard framework rather than a sixteenth hand-rolled walk (issue #135) — is noted, not resolved, by this slice.

## Slice 13 — live-state integration harness

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
   Build a real hub from local bare remotes, **by running clone** rather than by assembling the layout in test code — a hand-built hub tests a shape the author asserted, not the one fabric produces, which is the same blindness this slice exists to remove.
   The campaign's recipe has two gotchas worth encoding rather than rediscovering: `git init --bare` leaves `HEAD` on `master` while the pushed branch is `main` (needs `git -C <bare> symbolic-ref HEAD refs/heads/main`), and the weft remote must be genuinely empty or clone's bootstrap guard refuses it.
   One independent hub per destructive scenario, never shared.

   **This is more extraction than new construction, and `internal/lyxtest` is not where it goes.**
   Read this paragraph before reaching for either;
   both mistakes are easy to make and one of them fails the build.

   - `internal/lyxtest` **does** build real hermetic git repos — `buildWarpHub` (warp repo + bare remote), `buildWeftPrime` (`<name>-weft` sibling with a placeholder `_lyx/config` + bare), and `CopyPaired`/`CopyWarpHub`/`CopyWeft` on a template-built-once + copy-per-test pattern.
     Reuse that machinery: `HermeticGitEnv`, the bare-remote builders, the origin-URL rewriting, the per-test isolation.
   - What it builds is **not a fabric hub**.
     Every fixture is assembled by hand, never through `CloneHub`, so there is no `_board`, no `_portals`/`_launchers`, no hub-level `.lyx`, no junctions, no `.lyx-anchor` marker, no warp-URL binding on `weft:main` and no repo-wide `fabric.yaml`.
     Its bare remotes are left empty and never pushed to, which is why the `symbolic-ref HEAD` gotcha above does not arise there — it appears only once a hub is built by really cloning.
   - **Naming trap:** `lyxtest.WarpFixture.Hub` is the *warp repo*, not a fabric hub.
     "lyxtest already gives me a Hub" is a misreading that produces the wrong fixture.
   - **The factory cannot live in `internal/lyxtest`.** The [lyxtest Leaf Invariant](../../CONSTRAINTS.md#lyxtest-leaf-invariant) bars it from importing `fabricengine`, machine-enforced by `internal/lyxtest/leaf_enforcement_test.go`, because feature packages' own tests import lyxtest and a reverse import closes a test-build cycle.
     Anything that drives `CloneHub` is therefore out of bounds there.
   - **Nor in an in-package (`package fabricengine`) test file that imports a shared helper package**, for the same cycle reason one level down.

   **Where it does go: `internal/fabricengine/fabrictest`, mirroring `internal/boardengine/boardtest`.**
   The repo already has this exact pattern — `boardtest` is the black-box package holding board's benchmarks, concurrency stress and git-backed integration suites, with its own `doc.go` and a `testmain_test.go` calling `lyxtest.HermeticGitEnv` (required by the [Hermetic Git Test Environment Invariant](../../CONSTRAINTS.md#hermetic-git-test-environment-invariant)), and `docs/overview.md`'s Tests section names it as the convention.
   Nothing imports `boardtest`;
   nothing will import `fabrictest`.

   The cycle only ever bites **in-package** test files.
   `fabricengine_test` → `fabrictest` → `fabricengine` is a legal chain, because Go compiles external test packages separately for exactly this reason.
   `fabrictest` may also import `lyxtest` (for `HermeticGitEnv` and the bare-remote builders) without risk, since `lyxtest` imports neither.

   - **Measured cost of a hub fixture** (2026-08-10, Linux/WSL2 ext4, 155U, 14 cores, hermetic git env;
     two independent methods agreeing).
     `CloneHub` in-process: 60–66 ms serial, 15–16 ms concurrent.
     Via the CLI: ~101–110 ms serial, 19 ms concurrent.
     **Full fixture — own bares plus hub clone — 24 ms concurrent**, of which copying two prebuilt bares is ~2 ms and the clone ~22 ms.
     Concurrency scales 5.2× on 14 cores (37% of linear): clone is `fork`/`fsync`-bound, not CPU-bound.
     For comparison, today's `lyxtest.CopyPaired` is 13.3 ms serial / 2.3 ms concurrent.
     Cheap enough that per-scenario hubs are not a cost concern on this platform;
     Windows is unmeasured (see [fabric-windows-verification.md](fabric-windows-verification.md)).
   - **Copy the bares, clone the hub.** Bare repos hold zero symlinks, so the existing copy helper handles them;
     a hub cannot be copied at all, because its junctions carry **absolute** targets (`warp/_lyx → <hub>/warp-weft/_lyx`, `warp/.lyx → …`, `warp/_board → <hub>/_board`), so a filesystem copy would leave every link aimed at the template.
   - **Local bares are real remotes — no GitHub needed for `push`/`pull`/`sync`.** The repo already proves this: `pull_integration_test.go:73,78` force-pushes from a second clone to produce the diverged upstream `Fabric.Pull` re-anchors from, and `coalesce_integration_test.go:128-138` advances the bare from a second clone to force a genuine non-fast-forward through `gitrepo.Push`'s rebase-retry.
     Give each scenario its **own** bare pair so cells can push independently without racing;
     that isolation is already required for correctness, and it also means the suite never touches the sandbox Hub, so both can run at once.
   - **The consolidation half, and it is cheaper than it looks.** Fabric's own tests already call `CloneHub` **101 times across 7 files**, with no shared factory and a scattering of ad-hoc local helpers — `gitStatusPorcelain` is defined twice.
     **Six of those seven files are already `package fabricengine_test`** (only `clone_test.go` is in-package), and `fabricengine` runs 38 external test files against 45 in-package ones.
     So a `fabrictest` factory serves the existing call sites immediately, with **no export-for-test shim** — the conversion problem `lyxtest`'s own package doc flags as "a slice of its own" does not arise here.
     `clone_test.go` stays in-package and keeps its own setup;
     that is not a defect to fix in this slice.
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

### Scope note — build the minimum that validates slice 12's gate, then grow

Ship the tranche that validates slice 12's gate first, then grow.
This is why the slice depends on 12 rather than running beside it — that tranche cannot be written against behaviour the gate has not yet changed.
That tranche is: the hub factory, the dirty/hostile states the evidence table's eight defects actually exercised, and the hostile-input row of the verb table driven against every destructive verb.
It is what turns "the gate refuses correctly" from a claim into an assertion.

The remaining axes — concurrency between worktrees, the hook surface, `_portals`/`_launchers`, and full subpath coverage of every cell — are additive and can land alongside slice 14's truthfulness assertions.
Growing the matrix is cheap once the factory exists.

### Scope, cost and risk

Fabric is unusually well suited to this: it spawns no LLM subprocess, its substrate is real git plus the real filesystem, so live driving is cheap and there is no concurrency ceiling.
The campaign ran 18 independent hubs in a single round without trouble.

Runtime is the real cost.
This belongs behind the `integration` tag and must not be in the default `go test ./...` path.
Measure wall-clock before the matrix grows large;
parallelising per-hub is straightforward since every cell owns its own hub.

**Known limitation, stated up front rather than discovered later:** Windows path behaviour (junctions vs symlinks, case-insensitive compare in `lyxcwd.samePath`) cannot be exercised on a Linux host.
The campaign carried this as a permanent known gap across all six rounds rather than pretending to have verified it, and this harness inherits that gap honestly — see the Someday `fabric: Windows path behaviour is unverified` item and [fabric-windows-verification.md](fabric-windows-verification.md).

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

Sequenced **after** slices 12 and 13, not before: the gate's own refusal reporting (its step 5) lands in each verb's existing error shape first, and this slice is what generalises that into one accumulate-as-you-mutate envelope.
Deferring the envelope is a deliberate, bounded churn cost, paid so the safety half of the gate lands a slice earlier.
Waiting for 13 as well is the mechanical half of the chain: this slice rewrites every verb's result path, which is the whole surface 13's harness drives.

## Slice 15 — corrindex two-phase read-modify-write races an unlocked RebuildIndex

Issue #148 (`bug`), severity **LOW — self-healing**.
Logically independent of slices 12-14, but sequenced **after** them all the same:
it edits `internal/fabricengine`, and slices 12 and 14 rewrite that package wholesale, so overlapping it buys a merge conflict rather than a schedule win.
Parked at the tail, where it costs nothing to wait.

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
- [fabric-windows-verification.md](fabric-windows-verification.md) — the Someday platform gap slice 13 inherits honestly rather than closing.
- [gitexec-error-shape.md](gitexec-error-shape.md) — the fifth class the campaign surfaced, scoped out of these slices because its blast radius is every module that touches git, not fabric.
- [CONSTRAINTS.md](../../CONSTRAINTS.md) — where slice 12's invariant goes;
  its [Fabric Git Invariant](../../CONSTRAINTS.md#fabric-git-invariant-warp--weft) and [Test Tier Purity Invariant](../../CONSTRAINTS.md#test-tier-purity-invariant) are the two these slices sit next to.
- `internal/fabricengine` package documentation — where the durable rationale folds when this file is deleted.
- The campaign's own artifacts — `.scratch/fabric-review-HANDOFF.md` and the per-round orchestrator verifications (`fabric-review-r3-orchestrator-verification.md`, `-r4-`, `-r5-`, and `fabric-review-opus-medium-r4.md`'s 28-site destructive enumeration) — lived in the `fabric-v2-crucible` worktree and were never merged.
  Everything durable from them that these slices need is restated above;
  treat that worktree as gone.
