# Batch: D3 -- de-parallel-build prose + final gate

```yaml
task: 'fabric: cutover -- rewire consumers onto fabric, delete warp/weft'
batch: D3 -- de-parallel-build prose + final gate
number: 6
cards: 6
verify: go build ./... && go test ./... -tags integration
depends-on: [5]
```

## Batch Scope

De-parallel-build fabric's own module and sweep stale deleted-module comment references
across the tree, then run the full-suite acceptance gate. After batches A-D2, fabric is the
sole git-coordination module, but its own CLI help + code comments still frame it as
"coexisting with the live warp/weft modules during the parallel-build period," and several
production comments elsewhere still name the deleted packages by their full import path or
bare name. These are comment/help edits (build-safe) but required by the CLI/Cobra
help-accuracy obligation, the Documentation-Lifecycle "no rot" rule, and the Tier-1/Tier-2
grep gate. Card 27 is the final acceptance gate. Depends on batch 5. This is the last batch,
so `verify` runs the full integration suite once as the acceptance gate (see Batch Tests).

## Cards

### Card 22: rewrite fabriccli/fabric.go help to sole-module framing

- **Context:**
  - `internal/fabricengine/doc.go`
  - `docs/overview.md`
- **Edits:**
  - `internal/fabriccli/fabric.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Rewrite the parallel-build framing in this file's cobra help and package
  comment so fabric reads as the sole host<->weft git-coordination module: the package
  comment ("fabric ... coexists with warp and weft during the parallel-build period (see
  docs/overview.md); it is not yet the default"), the command `Short` ("unified host<->weft
  git-coordination (parallel build alongside warp/weft)" -> drop the parenthetical), the
  `Long` sentence "fabric exists alongside warp and weft during the parallel-build period; it
  is not yet the default. See docs/overview.md and manifest/designs/fabric.md." (rewrite to
  present-tense sole-module framing and DROP the `manifest/designs/fabric.md` reference --
  the file is deleted; keep the `docs/overview.md` link), and the `Long` line "During the
  parallel-build period the weft repo also holds warp-created weft branches ..." (reword to
  drop the parallel-build/warp-created framing while keeping any still-true weft-branch
  behaviour note). Every command keeps a non-empty `Short`. Do NOT touch the
  `t.Parallel()`-style test-concurrency mention elsewhere in the file -- that is not a
  parallel-build reference.
- **Commit:** `docs(fabriccli): reframe help as sole git-coordination module`

### Card 23: sweep fabricengine own-module parallel-build + provenance comments

- **Context:**
  - `internal/fabricengine/fabric_test.go`
- **Edits:**
  - `internal/fabricengine/doc.go`
  - `internal/fabricengine/clone.go`
  - `internal/fabricengine/cleanup.go`
  - `internal/fabricengine/fabric.go`
  - `internal/fabricengine/hook.go`
  - `internal/fabricengine/weftgit.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Rewrite the comments that describe fabric as coexisting with the live
  warp/weft modules, keeping fabric's own `Warp`/`Weft`/`Warp-SHA` API terms intact:
  - `doc.go`: delete the "fabric is built parallel to the existing, shipped
    `warpengine`/`weftengine` modules -- not replacing them yet ... a later, separate cutover
    task rewires consumers onto fabric and deletes the old modules." paragraph (this cutover
    IS that task); reword the opening line "unifies the `warp` ... and `weft` ... modules" and
    the "today's warp/weft, which mirror identical branch names" contrast to past/neutral
    tense that does not present warp/weft as live modules. Keep the `Warp-SHA` trailer and
    `Warp *gitrepo.Repo`/`Weft *gitrepo.Repo` descriptions unchanged (fabric's own API).
  - `clone.go`: reword the "Adapted from warpengine's clone.go" provenance comment and the
    "distinct from warpengine's: each module's clone orchestration and its differential test
    tear down through its own module's RemoveAll" comment (the differential tests are deleted
    and warpengine is gone) so neither names a deleted module.
  - `cleanup.go`: reword the three "parallel-build period" comments that reference
    warp-created weft branches / the two modules sharing a weft branch, keeping any still-true
    branch-cleanup behaviour.
  - `fabric.go`: reword the "serialize against the same test/CI bypass during the
    parallel-build period" comment.
  - `hook.go`: reword the "parallel build" comment.
  - `weftgit.go`: reword the two "parallel-build period" comments about locking/racing between
    the two modules.
  Do NOT touch `t.Parallel()` or "parallel to Add's logic" mentions -- those describe Go test
  concurrency, not the parallel build.
- **Commit:** `docs(fabricengine): drop parallel-build/deleted-module comment references`

### Card 24: de-parallel-build sandbox prose

- **Context:**
  - `internal/fabricengine/doc.go`
- **Edits:**
  - `tools/sandbox/main.go`
  - `tools/sandbox/SANDBOX-FABRIC-SUITE.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Rewrite the now-false isolation/parallel-build prose (the functional
  `cloneRun` flip already landed in batch C card 14; this card is comments/docs only):
  - `tools/sandbox/main.go`: reword the dedicated-fabric-hub comments that justify isolation
    as "warp/weft must stay untouched" / "parallel-build" (near the fabric hub constants and
    the `decideFabricClone`/fabric-suite plumbing) to state the dedicated hub simply hosts
    fabric's stricter `main-weft` branch-naming suite. Keep the plumbing itself.
  - `tools/sandbox/SANDBOX-FABRIC-SUITE.md`: reword the suite's parallel-build / "isolate from
    warp-weft" framing to describe FABRIC-SUITE as fabric's own dedicated suite; drop any
    inbound `manifest/designs/fabric.md` link (repoint to `internal/fabricengine/doc.go` if a
    rationale link is wanted).
- **Commit:** `docs(sandbox): reframe dedicated fabric hub prose post-cutover`

### Card 25: sweep stale full-path deleted-module comment refs

- **Context:**
  - `internal/fabricengine/doc.go`
- **Edits:**
  - `internal/lyxtest/doc.go`
  - `internal/hubgeometry/hubgeometry.go`
  - `internal/codeintelcli/cli.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Repoint comments that name the deleted packages by import path or file:
  - `internal/lyxtest/doc.go`: the package comment lists "test suites across
    internal/warpengine, internal/warpcli, internal/weftengine, internal/weftcli, and
    internal/hubgeometry" -- replace the four deleted packages with `internal/fabricengine`/
    `internal/fabriccli` (keep `internal/hubgeometry`).
  - `internal/hubgeometry/hubgeometry.go`: reword the three warpengine references -- the
    "seeders in internal/warpengine" comment, "warpengine's Status/Reconcile scans", and
    "matching the skip condition in warpengine/prune.go's hub scan" -- to name
    `internal/fabricengine` (the equivalent scan/seeder logic now lives there). These are
    comments; do not touch any geometry token/const.
  - `internal/codeintelcli/cli.go`: reword the "(see internal/weftcli.Command)" comment to
    reference `internal/fabriccli` instead.
- **Commit:** `docs: repoint stale warp/weft package comments to fabric`

### Card 26: bare-name comment review sweep

- **Context:**
  - `internal/fabricengine/doc.go`
- **Edits:**
  - `internal/perchengine/doc.go`
  - `internal/perchengine/engine.go`
  - `internal/reedengine/config.go`
  - `internal/lyxtest/hermetic.go`
  - `internal/websterengine/audit.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Repoint ONLY the comments that name a deleted *module*; leave comments
  that describe the weft *repo/role* concept or the `lyx weft`/`lyx warp` guard behaviour:
  - `internal/perchengine/engine.go`: "never imports weftengine/warpengine" -> name the
    git-coordination engine `fabricengine`.
  - `internal/perchengine/doc.go`: "never imports weftengine or warpengine" and "via
    weftengine directly" -> `fabricengine`.
  - `internal/reedengine/config.go`: "mirroring warpengine.LoadConfig" and "matching
    warpengine's shape" -> `fabricengine.LoadConfig` / `fabricengine`'s shape.
  - `internal/lyxtest/hermetic.go`: "warpengine test run" example -> `fabricengine`.
  - `internal/websterengine/audit.go`: the comment "weftengine.Commit/Push run in-process
    inside webstercli's verbs" -> `fabricengine`'s `CommitWeft`/`PushWeftAt` (webstercli now
    uses fabricengine after batch A). LEAVE the `weftRef` regex and the `lyx weft`/`lyx warp`
    detection strings and the `ClassWeftReference`/`weft-reference` concept names untouched --
    that guard describes the weft-touching concept (defense-in-depth), not a live module, and
    is functional code, not a stale reference.
- **Commit:** `docs: repoint bare warp/weft module comments to fabricengine`

### Card 27: final acceptance grep-clean gate

- **Context:**
  - `internal/fabricengine/doc.go`
- **Edits:** none
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Verification-only gate (zero diff). Run and confirm:
  - **Tier 1 (hard zero-match, acceptance blocker):** no `.go` file imports
    `github.com/Knatte18/loomyard/internal/{warpengine,warpcli,weftengine,weftcli}`
    (`grep -rn -E 'loomyard/internal/(warp|weft)(cli|engine)"' --include='*.go' .` returns
    nothing); no `warpcli.Command()`/`weftcli.Command()` registration remains; no `warp`/
    `weft` config-module identifiers remain in `internal/configreg`.
  - **Tier 2 (reviewed soft sweep):** `grep -rn -E 'internal/(warp|weft)(cli|engine)' .` and
    `grep -rnw -E 'warpengine|weftengine|warpcli|weftcli' --include='*.go' .` over comments/
    docs -- every remaining hit must be a legitimate weft-repo/role description, not a named
    deleted module (the sweeps in cards 22-26 should have cleared all module-naming hits).
  - **Tier 2b (fabric's own module):** `grep -rn -iE 'parallel[- ]build'
    internal/fabricengine/ internal/fabriccli/` returns nothing except the intended
    exclusions -- there should be none left after card 22/23; the `t.Parallel()`/"parallel to
    Add's logic" mentions do not match `parallel[- ]build` and are irrelevant here.
  If any Tier-1 match appears, it is an acceptance failure -- fix the offending file (which
  belongs to an earlier batch's scope) before the batch can pass. If a Tier-2/2b hit that
  names a deleted module survives, sweep it. The full-suite `verify` runs alongside this gate.
- **Commit:** none

## Batch Tests

This is the final batch, so `verify` runs the full acceptance gate:
`go build ./... && go test ./... -tags integration` -- the whole tree compiles and every
integration test passes with the old modules gone. This is the one deliberate full-suite run
(all other batches scoped their tests); justified because the cutover's correctness guarantee
is tree-wide (no dangling importer, no broken help-tree/coverage guard, no regressed weft
path) and only a full run proves it. Card 27's two-tier grep-clean gate runs in the same
batch as a structural (non-`go test`) acceptance check.
