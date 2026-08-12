# lyxtest builds real fabric hubs — invert the dependency

> **Status: not built.**
> Depends on `fabric-live-state-harness` (slice 13), which creates the `fabrictest` package this task needs as a landing zone — but sequenced behind the **whole** fabric chain (`12 → 13 → 14 → 15`), not just slice 13.
> It moves `fabricengine`'s 14 in-package `lyxtest` callers and migrates their assertions, which is the same package slice 14 rewrites every verb's result path in;
> only one of those may be in flight at a time.
> The full chain has now landed, so that sequencing constraint is satisfied.
> Deleted once landed, per the [documentation lifecycle](../../docs/overview.md#documentation-lifecycle);
> the durable half becomes the rewritten lyxtest invariant in `CONSTRAINTS.md` and `internal/lyxtest`'s own package doc.

## Why

`internal/lyxtest`'s fixtures are **hand-assembled approximations of a fabric hub**, never produced by `fabricengine.CloneHub`.
`buildWarpHub` runs `git init` plus one commit;
`buildWeftPrime` makes a `<name>-weft` sibling holding a placeholder `_lyx/config`.
Neither has `_board`, `_portals`, `_launchers`, a hub-level `.lyx`, junctions, a `.lyx-anchor` marker, the warp-URL binding on `weft:main`, or a repo-wide `fabric.yaml`.

Every test built on those fixtures therefore asserts against **a shape someone wrote down**, not the shape fabric produces.
Nothing detects drift between the two.
That is the same class of blindness the fabric v2 crucible campaign found at the tier level — see [internal/fabricengine](../../internal/fabricengine/doc.go)'s package doc, "The destruction chokepoint" section, where the hermetic suite stayed green through eight data-loss defects because it could not express the state that exposed them.

The fix is to invert the dependency: `lyxtest` imports `fabricengine` and builds hub fixtures by really cloning.
Every fixture in the repo then *is* a hub, and drift becomes impossible by construction rather than by discipline.
A side benefit, not the motive: fabric's clone path gets exercised on every suite run.

## Why the obvious objections do not hold

Both objections raised against this when it was first proposed were measured and both failed.

### The import cycle is small and mostly already handled

In-package test files that actually call `lyxtest.*` **and** sit in a package `fabricengine` imports:

| package | files | usage |
|---|---|---|
| `fabricengine` | 14 | 101× `MustRun`, 43× `CopyWeft`, 5× `CopyWarpHub`, 4× `HermeticGitEnv`, 2× `CopyPairedLocal` |
| `gitrepo` | 1 (`gogit_test.go`) | 18× `MustRun`, nothing else |
| `lyxcwd` | 1 (`gate_test.go`) | one call |

Everything else — `reedcli` (8), `cmd/lyx` (5), `shuttlecli` (4), `treadleengine`, `scoutengine`, `perchcli`, `boardengine`, … — lies **outside** `fabricengine`'s dependency set and is untouched.
The 44 external (`*_test` package) files are safe regardless: Go compiles external test packages separately, so `fabricengine_test` → `lyxtest` → `fabricengine` is a legal chain.

So the collateral is two files needing only `MustRun`, a ~15-line "run git or fail the test" helper.
`fabricengine`'s own 14 move to `fabrictest`, which slice 13 creates anyway.

### The performance cost is ~2-3%, measured

Benchmarked 2026-08-10 on Linux (WSL2, ext4 on the VM disk), Intel Core Ultra 7 155U, 14 logical CPUs, Go 1.26.x, hermetic git env.
Method: throwaway scripts and one throwaway Go benchmark, each run repeatedly;
sequential and concurrent variants measured by two independent methods that agree.

| operation | serial | concurrent (14) |
|---|---|---|
| `fabricengine.CloneHub`, in-process | 60–66 ms | 15–16 ms |
| `lyx fabric clone` via CLI | ~101–110 ms | 19 ms |
| **full fixture** (own bares + hub clone) | — | **24 ms** |
| copy prebuilt bares only | — | 2 ms |
| `lyxtest.CopyPaired` (today) | 13.3 ms | 2.3 ms |
| `lyxtest.CopyPairedLocal` (today) | 8.7 ms | 1.9 ms |

Concurrency scales 5.2× on 14 cores (37% of linear) — clone is `fork`/`fsync`-bound, not CPU-bound, so linear scaling was never available.

Repo-wide there are **167 `Copy*` call sites** (55 `CopyPaired`, 51 `CopyWeft`, 33 `CopyPairedLocal`, 23 `CopyWarpHub`, 5 `Copy`), all integration-tier since the [Test Tier Purity Invariant](../../CONSTRAINTS.md#test-tier-purity-invariant) bars `Copy*` from untagged tests.

167 × 24 ms ≈ **4.0 s**, against today's 167 × 2.3 ms ≈ 0.4 s.
Delta ≈ **+3.6 s** on Tier 2's ~132 s — about **2.7%**.

`CloneHub` is git/geometry-only (the CLI layer drives config materialization, the `weft:main` commit and junction wiring from `CloneResult`), so the in-process figures are a floor;
the 24 ms full-fixture number is the one to plan against.

## The design: copy the bares, clone the hub

The template-and-copy model is **not** discarded — it moves one level down.

- **Bare repos contain zero symlinks**, so `copyDirRecursive` handles them exactly as it does today. Copying a seeded warp bare plus an empty weft bare costs ~2 ms.
- **A hub cannot be copied.** Its junctions carry **absolute** targets — a fixture hub links `warp/_lyx → <hub>/warp-weft/_lyx`, `warp/.lyx → …`, `warp/_board → <hub>/_board` — so a filesystem copy would leave every link pointing back at the template. `copyDirRecursive`'s existing refusal of symlinks in templates stays correct and keeps protecting the trees that really are plain files.
- So: **copy the two bares per fixture, then clone the hub from them.** ~2 ms + ~22 ms.

### Local bare repos are real remotes — push/pull/sync need no GitHub

Git is distributed;
there is no privileged server concept.
A bare repo reached by path is a first-class remote, with identical refs, fast-forward rules, rejections and rebase-retry behaviour.
Fabric cannot tell the difference: its push goes `gitrepo.Push` → `gitexec` → the git CLI, and the remote's nature is opaque to it.

This is not a proposal — **the repo already does it**:

- `pull_integration_test.go:73,78` clones the bare a second time and `git push --force origin main`, producing the force-pushed upstream `Fabric.Pull`'s ancestry detection and safe re-anchor exist to handle.
- `coalesce_integration_test.go:128-138` advances the bare from a second clone so the next push is "a genuine non-fast-forward", driving `gitrepo.Push`'s rebase-retry.
- `bolt_integration_test.go:40` drives Bolt's push/pull against `git init --bare`.

Two recipe gotchas belong in the factory rather than in each call site: `git init --bare` leaves `HEAD` on `master` even when the pushed branch is `main` (fix with `git -C <bare> symbolic-ref HEAD refs/heads/main`), and the weft remote must be genuinely empty or clone's bootstrap guard refuses it.
Setting `protocol.file.allow always` in the hermetic git env is worth doing defensively.

What a local bare does **not** cover: transport (https/ssh), authentication, and server-side policy (protected branches, hooks, `receive.denyNonFastForwards`).
None of that is fabric's code.
If that surface is ever wanted it belongs as a handful of opt-in smoke tests against one real repo pair — never as the fixture substrate for 167 fixtures, which would make the suite slow, flaky and credential-dependent for no gain in fabric coverage.

The sandbox Hub is unaffected and stays the operator's dogfooding bench.
Because every fixture owns a temp-dir hub, the suite never touches `lyx-test-HUB` and the two can run concurrently — the isolation `fabric-live-state-harness` already requires for correctness buys this for free.

## The real cost is assertion migration, not runtime

This is the actual work, and it is not the milliseconds.

A real hub is ~155 files against the current templates' ~36, and carries `_board`, junctions, an anchor marker and config that today's fixtures lack.
Some fraction of the 167 call sites will have assertions that break on that difference — directory listings, file counts, "this path should not exist" checks.

Every one of those breaks is worth reading rather than silencing: it marks a place that is currently asserting against an invented shape instead of the real one.

## What needs to happen

1. **Extract the shared primitive.** Move `MustRun` (and `SeedConfig` if it proves equally load-bearing) into a zero-import leaf both `lyxtest` and low-level packages can use, freeing `gitrepo`'s `gogit_test.go` and `lyxcwd`'s `gate_test.go`.
2. **Move `fabricengine`'s 14 in-package lyxtest callers** into `fabrictest` (created by slice 13). `clone_test.go` keeps its own setup if it needs unexported access.
3. **Invert the import.** `lyxtest` imports `fabricengine`;
   hub fixtures are built by copying prebuilt bares and calling `CloneHub` plus the wiring the CLI layer performs.
4. **Migrate the assertions** that break on the real hub shape.
5. **Rewrite the lyxtest Leaf Invariant** in `CONSTRAINTS.md` into its inverse, and note the enforcement change: the new rule is **self-enforcing**. An in-package `fabricengine` test importing `lyxtest` becomes a hard compile error, not silent drift — strictly better than today's guard-test-dependent rule.

### Scoping rule, load-bearing

**`gitrepo` and `lyxcwd` must not get hub fixtures.**
Both sit *below* fabric and have no business needing one — `gogit_test.go` wants `MustRun` and nothing more.
The rule is "lyxtest offers hub fixtures via fabric", not "every lyxtest fixture goes through fabric".

A second reason: once every package's fixtures route through clone, a fabric clone bug fails the whole suite at once and localisation gets harder.
Keeping the low-level packages on primitive fixtures preserves a layer that still fails independently.

## Open question

**Windows is unmeasured**, and it is where this would be felt daily.
Clone forks many more git processes than a file copy, process spawn is expensive there, and [fixture-copy.md](../../docs/benchmarks/fixture-copy.md) records that Cortex XDR dominated the copy cost on this very machine.
Its own rule — compare down each machine's column, never across — applies here.

The comparison is clone-versus-copy, not clone-versus-nothing, and copying many small files is what an aggressive EDR punishes hardest, so the delta may be smaller than the raw platform ratio suggests.
Worth measuring before landing, not worth blocking the design on: even a 5× worse ratio puts this at roughly +15 s on a 132 s Tier 2 run.

Related: the Someday [fabric-windows-verification.md](fabric-windows-verification.md) item, which carries the same platform gap for correctness rather than for speed.
