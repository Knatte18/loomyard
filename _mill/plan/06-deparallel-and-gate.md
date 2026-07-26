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
  behaviour note). Also sweep every OTHER comment in this file that names a deleted module
  (`warpengine`/`weftengine`/`warpcli`/`weftcli`, full-path or bare) -- repoint to the fabric
  equivalent -- so the file carries no deleted-module reference (per the tree-wide
  comment-sweep Shared Decision). Every command keeps a non-empty `Short`. Do NOT touch the
  `t.Parallel()`-style test-concurrency mention elsewhere in the file -- that is not a
  parallel-build reference; and keep fabric's own `Warp`/`Weft`/`Warp-SHA` API terms.
- **Commit:** `docs(fabriccli): reframe help as sole git-coordination module`

### Card 23: sweep every fabricengine + fabriccli deleted-module comment

- **Context:**
  - `CONSTRAINTS.md`
- **Edits:**
  - `internal/fabriccli/cli_test.go`
  - `internal/fabricengine/ancestors_test.go`
  - `internal/fabricengine/clone_test.go`
  - `internal/fabricengine/clone_adopt_test.go`
  - `internal/fabricengine/config_test.go`
  - `internal/fabricengine/fabric_test.go`
  - `internal/fabricengine/hook_test.go`
  - `internal/fabricengine/launcher_content_test.go`
  - `internal/fabricengine/template_test.go`
  - `internal/fabricengine/add.go`
  - `internal/fabricengine/ancestors.go`
  - `internal/fabricengine/checkout.go`
  - `internal/fabricengine/cleanup.go`
  - `internal/fabricengine/clone.go`
  - `internal/fabricengine/config.go`
  - `internal/fabricengine/doc.go`
  - `internal/fabricengine/drift.go`
  - `internal/fabricengine/fabric.go`
  - `internal/fabricengine/hook.go`
  - `internal/fabricengine/hostclean.go`
  - `internal/fabricengine/junction.go`
  - `internal/fabricengine/launcher_content.go`
  - `internal/fabricengine/launchers.go`
  - `internal/fabricengine/list.go`
  - `internal/fabricengine/portals.go`
  - `internal/fabricengine/prune.go`
  - `internal/fabricengine/reconcile.go`
  - `internal/fabricengine/reconcile_stale_registration_test.go`
  - `internal/fabricengine/remove.go`
  - `internal/fabricengine/status.go`
  - `internal/fabricengine/topology.go`
  - `internal/fabricengine/weftgit.go`
  - `internal/fabricengine/weftwiring.go`
  - `internal/fabriccli/clone.go`
  - `internal/fabriccli/spawn.go`
  - `internal/fabriccli/weft_verbs.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Sweep EVERY `//` comment in these files that names a deleted module
  (`warpengine`/`weftengine`/`warpcli`/`weftcli`, full import-path or bare word) so that after
  this card `grep -rnw -E 'warpengine|weftengine|warpcli|weftcli' --include='*.go'
  internal/fabricengine/ internal/fabriccli/` returns nothing -- **including `_test.go`
  files** (the nine fabricengine/fabriccli test files in Edits -- including
  `clone_adopt_test.go`, added to this card's scope because it also carries a
  "mirroring warpengine's clone_integration_test.go" / "alongside a warpengine
  comparison" provenance comment, discovered while sweeping `clone_test.go` -- carry
  "Adapted from warpengine's X_test.go" / "mirroring weftengine..." provenance comments;
  all are comment-only, no code import of a deleted engine, verified;
  `reconcile_stale_registration_test.go` is also added to this card's scope, discovered
  while sweeping `reconcile.go`, because its `TestCleanup_NonSuffixedBranchNeverDeleted`
  doc comment carries a "parallel-build period" phrase caught by card 27's Tier 2b gate).
  This covers two comment kinds, both of which name a now-deleted module:
  - **Provenance/mirror comments** -- "Adapted from warpengine's X.go", "distinct from
    warpengine's", "mirroring warpengine.Y", "matching warpengine's shape", and similar in
    `add.go`, `ancestors.go`, `checkout.go`, `clone.go`, `config.go`, `drift.go`,
    `hostclean.go`, `junction.go`, `launcher_content.go`, `launchers.go`, `list.go`,
    `portals.go`, `prune.go`, `reconcile.go`, `remove.go`, `status.go`, `topology.go`,
    `weftwiring.go`, and `fabriccli/{clone.go,spawn.go,weft_verbs.go}`. Reword to drop the
    deleted-module name: state the behaviour directly, or refer to fabric's own file, without
    citing `warpengine`/`weftengine` as a live module. Do not invent provenance; if a comment
    only exists to say "adapted from warpengine", delete the provenance clause.
  - **Parallel-build-period comments** -- `doc.go` (delete the "fabric is built parallel to
    the existing, shipped warpengine/weftengine modules -- not replacing them yet ... a later,
    separate cutover task ..." paragraph; reword the opening "unifies the warp ... and weft ...
    modules" line and the "today's warp/weft, which mirror identical branch names" contrast to
    past/neutral tense), `cleanup.go` (three "parallel-build period" comments about
    warp-created weft branches / two modules sharing a branch), `fabric.go` ("serialize
    against the same test/CI bypass during the parallel-build period"), `hook.go` ("parallel
    build"), `weftgit.go` (two "parallel-build period" locking/racing comments).
  Keep fabric's own API terms untouched everywhere: `Warp`/`Weft` struct fields,
  `Warp-SHA`/`WarpSHATrailerKey`, `WeftBranchName`, `CommitWeft`/`PushWeft`/`PullWeft`/
  `StatusWeft`, `WeftSuffix`, and `-weft` geometry. Do NOT touch `t.Parallel()` or "parallel
  to Add's logic" mentions -- those describe Go test concurrency, not the parallel build (they
  do not match the `warpengine|weftengine` word grep either). This is a mechanical sweep and a
  script may drive the uniform replacements, but confirm each reworded comment still reads
  correctly (a blind `warpengine`->`fabricengine` substitution would produce nonsense like
  "Adapted from fabricengine's clone.go", so provenance clauses need a genuine reword or
  deletion, not a token swap).
- **Commit:** `docs(fabric): drop deleted-module comment references across the module`

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
  - `internal/hubgeometry/siblinglayout_test.go`
  - `internal/codeintelcli/cli.go`
  - `docs/shared-libs/hubgeometry.md`
  - `manifest/designs/loom.md`
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
  - `internal/hubgeometry/siblinglayout_test.go`: sweep any `//` comment naming a deleted
    module (comment-only, verified -- no code import) to the fabric equivalent.
  - `docs/shared-libs/hubgeometry.md`: line 100 cites "Used by `warpengine/prune.go`" as a
    consumer of `WeftHostSlug` -- reword to name `internal/fabricengine`'s equivalent hub-scan
    (`internal/fabricengine/prune.go`), which now owns that scan.
  - `manifest/designs/loom.md`: the Preflight module-table row says the package "builds on
    ... internal/warpengine, internal/state" -- reword to `internal/fabricengine`. Pre-existing
    doc scope gap surfaced by holistic review r4 (neither file was in any batch's original
    Edits list); folded into this card because it is the same "repoint stale full-path
    deleted-module comment refs" pattern the rest of the card already covers.
- **Commit:** `docs: repoint stale warp/weft package comments to fabric`

### Card 26: bare-name comment review sweep

- **Context:**
  - `internal/fabricengine/doc.go`
- **Edits:**
  - `internal/perchengine/doc.go`
  - `internal/perchengine/engine.go`
  - `internal/reedengine/config.go`
  - `internal/reedengine/config_test.go`
  - `internal/lyxtest/hermetic.go`
  - `internal/websterengine/audit.go`
  - `internal/clihelp/exec.go`
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
  - `internal/reedengine/config_test.go`: sweep any `//` comment naming a deleted module
    (e.g. "mirroring warpengine..." ; comment-only, verified -- no code import) to
    `fabricengine`.
  - `internal/clihelp/exec.go`: the `GroupRunE` doc comment's example list "(e.g. \"lyx
    warp\", \"lyx board\")" cites a deleted CLI verb as an illustrative example -- reword to
    an example pair that is still live, e.g. "(e.g. \"lyx fabric\", \"lyx board\")".
- **Commit:** `docs: repoint bare warp/weft module comments to fabricengine`

### Card 27: final acceptance grep-clean gate

- **Context:**
  - `internal/fabricengine/doc.go`
- **Edits:**
  - `internal/buildercli/weft.go`
  - `internal/webstercli/weft.go`
  - `internal/perchcli/run.go`
  - `cmd/lyx/registration_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Primarily a verification-only gate, but running it surfaced four files
  the earlier batches' sweeps missed (each belonging to an already-committed card, per the
  Tier 2 rule that a surviving hit is swept "wherever it is found" rather than left for a
  closed batch to reopen): `buildercli/weft.go`, `webstercli/weft.go`, and `perchcli/run.go`
  each carry a "mirroring weftengine.Commit's own top-level short-circuit" comment (batch A
  rewired the calls to fabricengine but missed this provenance clause) -- reword to name
  `fabricengine.CommitWeft`/`CommitWeft` instead. `cmd/lyx/registration_test.go` still
  carries the `"warpcli": true, "weftcli": true` temporary allowlist entries batch C added
  explicitly as a bridge "until batch D1" deletes the packages (see that card's own comment);
  batch D1 deleted the packages but never removed the now-dead allowlist entries and their
  comment -- delete both entries and reword the comment, since `discovered` can no longer
  contain those package names (the packages no longer exist on disk), so removing the
  allowlist changes no test behavior. Run and confirm the remaining checks (zero diff beyond
  the four sweeps above):
  - **Tier 1 (hard zero-match, acceptance blocker):** no `.go` file imports
    `github.com/Knatte18/loomyard/internal/{warpengine,warpcli,weftengine,weftcli}`
    (`grep -rn -E 'loomyard/internal/(warp|weft)(cli|engine)"' --include='*.go' .` returns
    nothing); no `warpcli.Command()`/`weftcli.Command()` registration remains; no `warp`/
    `weft` config-module identifiers remain in `internal/configreg`.
  - **Tier 2 (deleted-module names, now zero after the tree-wide sweep):** the sweep Shared
    Decision + cards 22-26 + the in-file sweeps folded into every batch-A/B/C editing card
    remove every comment that names a deleted module, so both
    `grep -rnw -E 'warpengine|weftengine|warpcli|weftcli' --include='*.go' .` (excluding the
    deleted packages, which are gone) and `grep -rn -E 'internal/(warp|weft)(cli|engine)' .`
    over `.go` comments should now return nothing -- **including `_test.go` files**: the
    surviving test files that carried provenance comments are swept by card 23 (fabricengine +
    fabriccli tests), card 25 (`hubgeometry/siblinglayout_test.go`), card 26
    (`reedengine/config_test.go`), card 17 (`lyxtest/leaf_enforcement_test.go`), and the
    batch-A cards (`initengine/undo_test.go`, `loomengine/testmain_test.go`,
    `perchcli/run_integration_test.go`), so the tree-wide `--include='*.go'` grep is genuinely
    reachable. Any surviving `.go` hit is an unswept deleted-module reference -- treat it as a
    gate failure and sweep it (it belongs to whatever card owns that file). In non-`.go` docs,
    a remaining hit is only acceptable if it is a legitimate weft-repo/role description, not a
    deleted-module name.
  - **Tier 2b (fabric's own module):** `grep -rn -iE 'parallel[- ]build'
    internal/fabricengine/ internal/fabriccli/` returns nothing -- cards 22/23 cleared it; the
    `t.Parallel()`/"parallel to Add's logic" mentions do not match `parallel[- ]build`.
  If any Tier-1 match appears, it is an acceptance failure -- fix the offending file (which
  belongs to an earlier batch's scope) before the batch can pass. Tiers 2 and 2b were expected
  to be clean from cards 22-26 and the batch-A/B/C in-file sweeps alone; running the gate found
  the four-file residue documented in Edits above, swept here instead of reopening the closed
  batches. The full-suite `verify` runs alongside this gate.
- **Commit:** `docs: sweep gate-discovered deleted-module residue from batch A/C`

## Batch Tests

This is the final batch, so `verify` runs the full acceptance gate:
`go build ./... && go test ./... -tags integration` -- the whole tree compiles and every
integration test passes with the old modules gone. This is the one deliberate full-suite run
(all other batches scoped their tests); justified because the cutover's correctness guarantee
is tree-wide (no dangling importer, no broken help-tree/coverage guard, no regressed weft
path) and only a full run proves it. Card 27's two-tier grep-clean gate runs in the same
batch as a structural (non-`go test`) acceptance check.
