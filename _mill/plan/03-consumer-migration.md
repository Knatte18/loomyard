# Batch: consumer call-site migration

```yaml
task: 'fabric: close the weft-visibility leak (slice 8)'
batch: 'consumer call-site migration'
number: 3
cards: 7
verify: go test -tags integration ./internal/buildercli/ ./internal/webstercli/ ./internal/perchcli/ ./internal/configcli/ ./internal/builderengine/ ./internal/websterengine/ ./internal/fabriccli/ && go test ./cmd/lyx/
depends-on: [1]
```

## Rename mechanic

For each `Moves:` pair the implementer MUST:

1. Run `git mv <old> <new>` FIRST, before making any other change to the moved file.
2. Make ONLY surgical edits — touch only the lines that must change after the move (helper renames, constructor retargets, comment rewords).
3. Use a full-file `Creates:` entry only for genuinely new files that have no predecessor.
4. Never write the relocated file from scratch and delete the original — that breaks git rename history and inflates review diffs.

## Batch Scope

Migrates every production call site outside `internal/fabricengine` onto the batch-01 API and applies the `consumer-renames`, `json-key-renamed`, and `diagnostics-say-fabric-detail-says-weft` decisions: `Open`/`Committed()` adoption, `weft.go` → `sync.go` file renames, `FabricResetter`/`FabricBisector`, `RefScanner` in webster's audit, `ClassFabricReference`, `configcli` string/`Long` rewords, and the `"fabricCommitted"` JSON key.
Each card also rewords every `weft`/`warp`/fabric-sense-`host` comment in the files it edits (decision `comment-fidelity`);
untouched files are batch 06's sweep.
After this batch, no production package outside the owner set calls `New`/`WeftWorktree` — the precondition for batch 04's unexport.

## Cards

### Card 7: buildercli

- **Context:**
  - `internal/fabricengine/open.go`
  - `internal/fabricengine/commit.go`
  - `internal/fabricengine/fabric.go`
  - `_mill/discussion.md`
- **Edits:**
  - `internal/buildercli/run.go`
  - `internal/buildercli/run_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:**
  - `internal/buildercli/weft.go` -> `internal/buildercli/sync.go`
  - `internal/buildercli/weft_test.go` -> `internal/buildercli/sync_test.go`
  - `internal/buildercli/weft_integration_test.go` -> `internal/buildercli/sync_integration_test.go`
- **Requirements:** In the moved `sync.go`: rename helper `weftCommit` → `fabricSync`;
  replace `fabricengine.New(l.WorktreePath(), fabricengine.WeftWorktree(l))` (the `weft.go:24,32` pattern) with `fabricengine.Open(l)`;
  replace `res.WeftCommitted` reads with `res.Committed()`;
  rename local `weftErr` → `syncErr`;
  delete any `weftWorktree` local (`Open` removes the need);
  keep the open-coded `if !opts.SkipGit { … }` guard around the constructor exactly as-is (behaviour unchanged) but reword its explanatory comment without weft/warp vocabulary.
  In `run.go`: JSON envelope key `"weftCommitted"` → `"fabricCommitted"` (line 98);
  diagnostic `"builder: run finished (%s) but the weft sync failed: %v"` → `"…but the fabric sync failed: %v"` — the wrapped `%v` stays fabricengine's own error, which may keep weft-level detail (decision `diagnostics-say-fabric-detail-says-weft`).
  In `run_test.go:150`: `"weftCommitted"` → `"fabricCommitted"`.
  In `sync_integration_test.go` (was `weft_integration_test.go:132-148`): the commit-landed-but-`RecordCorrespondence`-failed case must keep asserting `(true, err)` not `(false, err)` through the renamed helper and `Committed()`.
  In `sync_test.go`: update the `weft.go:25` fixture comment ("Neither the host worktree nor its -weft sibling…") to fabric wording while keeping any `lyxtest` owner-API references verbatim.
  Reword remaining weft/warp/host-phrase comments in all edited/moved files.
- **Commit:** `refactor(buildercli): migrate to fabricengine.Open/Committed, rename weft.go to sync.go`

### Card 8: webstercli

- **Context:**
  - `internal/fabricengine/open.go`
  - `internal/fabricengine/commit.go`
  - `internal/buildercli/sync.go`
  - `_mill/discussion.md`
- **Edits:**
  - `internal/webstercli/run.go`
  - `internal/webstercli/verbs_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:**
  - `internal/webstercli/weft.go` -> `internal/webstercli/sync.go`
  - `internal/webstercli/weft_integration_test.go` -> `internal/webstercli/sync_integration_test.go`
- **Requirements:** Mirror card 7 for webstercli: `weftCommit` → `fabricSync`, `Open(l)`, `Committed()`, `weftErr` → `syncErr` in the moved `sync.go` (was `weft.go:21,27,32`);
  `run.go:105` JSON key `"weftCommitted"` → `"fabricCommitted"` and the run-finished diagnostic reworded to `"fabric sync failed"`;
  `sync_integration_test.go` (was `weft_integration_test.go:140-157`) keeps asserting `(true, err)` on the partial-failure path.
  `verbs_test.go:633`: the negative assertion `if strings.Contains(out.String(), "weft sync failed")` must be UPDATED to assert against `"fabric sync failed"`, not deleted — after the reword the old string can never appear, so leaving it would make the test pass forever while checking nothing.
  Reword remaining weft/warp/host-phrase comments in all edited/moved files.
- **Commit:** `refactor(webstercli): migrate to fabricengine.Open/Committed, rename weft.go to sync.go`

### Card 9: perchcli

- **Context:**
  - `internal/fabricengine/open.go`
  - `internal/fabricengine/commit.go`
  - `_mill/discussion.md`
- **Edits:**
  - `internal/perchcli/run.go`
  - `tools/sandbox/SANDBOX-PERCH-SUITE.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `run.go` (inline sites at lines 322, 344, 353): `fabricengine.New(c.layout.WorktreePath(), fabricengine.WeftWorktree(c.layout))` → `fabricengine.Open(c.layout)`;
  `res.WeftCommitted` → `res.Committed()`;
  JSON key `"weftCommitted"` → `"fabricCommitted"` (line 378);
  consumer-emitted diagnostics say "fabric sync" per decision `diagnostics-say-fabric-detail-says-weft`;
  the SkipGit guard comment rewords, behaviour unchanged.
  Reword the ~20 weft/warp comments in `run.go`.
  `tools/sandbox/SANDBOX-PERCH-SUITE.md:119` updates `weftCommitted` → `fabricCommitted` in the same commit (decision `json-key-renamed`) — this is the only `tools/` edit in the task;
  the `weftURL`/`fabricWeftURL` identifiers and GitHub URLs elsewhere in `tools/` stay verbatim.
- **Commit:** `refactor(perchcli): migrate to fabricengine.Open/Committed, rename JSON key`

### Card 10: configcli

- **Context:**
  - `internal/fabricengine/open.go`
  - `_mill/discussion.md`
- **Edits:**
  - `internal/configcli/configcli.go`
  - `internal/configcli/configcli_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** `configcli.go:125,181`: `"edited _lyx/config/%s.yaml but weft sync failed: %s"` → `"…but fabric sync failed: %s"`.
  `configcli.go:233`'s cobra `Long`: `"…and syncs weft on\n"` → `"…and syncs fabric on\n"` (CLI/Cobra Invariant help-accuracy obligation — re-check the whole `Long` reads correctly after the edit).
  `configcli.go` does not import `fabricengine` (sync is injected) — no constructor change in this card.
  `configcli_test.go:187`: assertion `"weft sync failed"` → `"fabric sync failed"`.
  Reword the 4 weft/warp comment mentions in `configcli.go`.
- **Commit:** `refactor(configcli): fabric-worded sync errors and help text`

### Card 11: builderengine

- **Context:**
  - `internal/fabricengine/open.go`
  - `_mill/discussion.md`
- **Edits:**
  - `internal/builderengine/chain.go`
  - `internal/builderengine/spawn.go`
  - `internal/builderengine/runlevel.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Rename `WarpResetter` → `FabricResetter` (interface defined at `chain.go:55-59`;
  references at `chain.go:66,70`, `spawn.go:76,362`) with its doc comment reworded to fabric vocabulary — the doc comment must not say "warp-only";
  describe it as the hard-reset surface `RestartChain` drives on the fabric repo.
  `spawn.go:367`'s `fabricengine.New(…, fabricengine.WeftWorktree(…))` → `fabricengine.Open(deps.Layout)`.
  Reword the ~21 weft/warp comments in `spawn.go` and the ~5 in `chain.go`, plus `state.go`-style "host HEAD" phrases where they appear in these two files (decision `comment-fidelity`;
  `builderengine/state.go` itself is batch 06).
  `runlevel.go:399` carries the policed phrase in a comment ("…host repo while holding an absolute report path inside the…") and rewords to "the repo" — it belongs to no other card and would otherwise fail batch 07's rule (2) on first activation.
  `spawn.go:9,232`'s "plain host filesystem" is the machine/OS sense of host and stays.
- **Commit:** `refactor(builderengine): rename WarpResetter to FabricResetter, adopt Open`

### Card 12: websterengine

- **Context:**
  - `internal/fabricengine/refscanner.go`
  - `internal/fabricengine/open.go`
  - `_mill/discussion.md`
- **Edits:**
  - `internal/websterengine/integration.go`
  - `internal/websterengine/integration_test.go`
  - `internal/websterengine/runlevel.go`
  - `internal/websterengine/audit.go`
  - `internal/websterengine/audit_test.go`
  - `internal/websterengine/recordbatch.go`
  - `cmd/lyx/rawgitmutation_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Rename `WarpBisector` → `FabricBisector` (interface defined at `integration.go:28-31`;
  references at `integration.go:98,132,184`, `runlevel.go:125,813`, comment at `integration_test.go:284`) with its doc comment reworded to fabric vocabulary.
  `runlevel.go:817`'s `fabricengine.New(…, fabricengine.WeftWorktree(…))` → `fabricengine.Open(deps.Layout)`.
  In `audit.go`: delete `weftReferencePattern(layout)` and the `internal/weftname` import;
  `CheckFork` and `CheckParent` take `*fabricengine.RefScanner` where they took `weftRef *regexp.Regexp`;
  construction moves to `fabricengine.NewRefScanner` at the existing sites `runlevel.go:706` and `recordbatch.go:125`, still once per audit.
  Rename `ClassWeftReference` → `ClassFabricReference`, value `"weft-reference"` → `"fabric-reference"` (`audit.go:47`);
  reword the violation `Detail` prose at `audit.go:134,203`.
  Grep for consumers of the old `"weft-reference"` value before committing (it is a string;
  batch-01 grep found only `audit.go:47`, re-verify).
  In `audit_test.go`: the three `fabricengine.WeftWorktree(layout)` calls and the `fakeLayout` helper move onto the scanner;
  the violation-class assertion updates to `ClassFabricReference`;
  `CheckFork`/`CheckParent` must still flag a fabric-referencing Bash command (behavioural contract).
  Reword comments: `audit.go` (~27 weft/warp mentions, plus `audit.go:156,159`'s OS-sense host lines reworded as hygiene to "the running OS's native sense" / "regardless of the workdir's platform"), `runlevel.go` (~13), `recordbatch.go` (~6 including `:40`'s "host repo checkout" → "the repo checkout").
  `cmd/lyx/rawgitmutation_test.go:10,45`: comments naming `WarpBisector`/`WarpResetter` update to the new names (card 11 already landed `FabricResetter`).
- **Commit:** `refactor(websterengine): FabricBisector, RefScanner adoption, ClassFabricReference`

### Card 13: fabriccli constructor migration

- **Context:**
  - `internal/fabricengine/open.go`
  - `_mill/discussion.md`
- **Edits:**
  - `internal/fabriccli/weft_verbs.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** `weft_verbs.go:102`'s `fabricengine.New(l.WorktreePath(), fabricengine.WeftWorktree(l))` → `fabricengine.Open(l)`.
  Nothing else changes: `fabriccli` is a vocabulary owner — `weft_verbs.go:244`'s `spawnPush(fabricengine.WeftWorktree(l))` keeps calling the exported `WeftWorktree`, the file keeps its name, and its weft-facing verbs/comments stay verbatim.
- **Commit:** `refactor(fabriccli): construct via Open(l)`

## Batch Tests

`verify:` runs the seven touched internal packages under `-tags integration` (the moved `sync_integration_test.go` files, `verbs_test.go`, and perchcli/configcli integration suites are tagged) plus untagged `./cmd/lyx/` (help-tree and raw-git-mutation guards over the renamed identifiers and `configcli`'s `Long`).
The regression pins: partial-failure `(true, err)` in both moved sync integration tests, the audit behavioural contract in `audit_test.go`, `"fabricCommitted"` in `run_test.go:150`, and `"fabric sync failed"` in `configcli_test.go:187` / `verbs_test.go:633`.
