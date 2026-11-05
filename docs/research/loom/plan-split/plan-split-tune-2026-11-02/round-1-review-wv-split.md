---
verdict: APPROVED
findings:
  - id: F1
    severity: MEDIUM
    class: decision
    origin: handler
    location: _lym/plan/00-overview.md (Shared Decisions, Part fabric-worktree-seam, Part bolt-replay no-ledger-no-replay, Part heddle hub-only-cli, Part bolt-dir-name); cards 8, 9, 11, 13, 20, 33, 34
    summary: Four plan rulings depart from decision-record steps with no Added after Discussion entry recording them
    fix: >-
      Through the parent (lym creel post --parent --expect-reply), ask for one Added after Discussion entry per departure, recorded with lym loom decision add:
      (1) the Constraints line wiring lym heddle through internal/cliwire becomes hub-only resolution through lymcwd.CwdFrom and lymcwd.Resolve with no cliwire.Module (cards 33, 34);
      (2) D5's and D6's CloneHub seeding and made event move to fabriccli's CloneAndWire (card 20);
      (3) D9's third bullet, that what lives in Bolt beside the board takes its place from BoltDirName and never from BoardDirName, is narrowed so fabricengine.BoardDir calls outside internal/fabricengine stay as they are (cards 8, 9);
      (4) D7's rule that a commit whose claimed paths have no replayer fails becomes a failure only when no run ledger is registered, with any path no registered replayer claims cherry-picked (cards 11, 13).
      Once the record carries the four entries, no card text changes.
      For any entry the parent declines, change the named cards to carry the decision as the record states it.
  - id: F2
    severity: MEDIUM
    class: decision
    origin: handler
    location: _lym/plan/00-overview.md (Part prompt-seam, Decision seam-error-refuses-launch); _lym/plan/25-claude-prompt-passages.md; _lym/plan/30-heddle-passages.md
    summary: The plan makes a prompt-seam error refuse every Claude launch in a hub, a failure mode D4 does not license
    fix: >-
      Either ask the parent to record the launch refusal as an Added after Discussion entry under D4, through lym loom decision add,
      or change card 25 so that a StandingPassages error is logged through logger.Warn, naming the hub's Bolt checkout and lym heddle list, and the launch proceeds with lym's line alone.
      In the second case, card 25's error row in TestPrepare_AppendsPromptPassages asserts that Prepare succeeds and the append-system-prompt file holds lym's line alone, in place of the refusal and the absent artifacts.
      Then drop the Decision seam-error-refuses-launch from 00-overview.md, or rewrite it to state the warning.
  - id: F3
    severity: MEDIUM
    class: consistency
    origin: lens:guard-reach
    location: _lym/plan/13-replay-through-registry.md (Intent, the by-entry step paragraph)
    summary: The ended-run set-aside reads as vacuously true for a commit whose ledger files name no run, which would set aside every heddle-only commit
    fix: >-
      In card 13's Intent, change "When those runs are all ended and live" to
      "When TouchedRuns names at least one run and every one of them is ended and live, as today's len(slugs) > 0 test requires".
      Add the sentence: "A commit whose ledger-claimed files name no run, such as one touching only heddle.json or only README.md, is never set aside for an ended run."
      Add a TestRecordsReplay row: a commit touching only notes.json, with ended naming an unrelated slug, is replayed through notesReplayer and tagged nothing.
  - id: F4
    severity: MEDIUM
    class: design
    origin: lens:guard-reach
    location: _lym/plan/29-heddle-render.md (TestRender paragraph)
    summary: The TestRender row that releases the Bolt lock within a few-millisecond BoltWait depends on scheduling and is not deterministic
    fix: >-
      In card 29's TestRender paragraph, give the row "a render while the test holds the Bolt lock and swaps heddle.json ... renders the new entries whole" its own BoltWait of several seconds, in place of the table's few milliseconds.
      The test swaps heddle.json and releases the lock in a fixed order once the render goroutine has started, so the row waits only as long as the test holds the lock.
      Keep the few-millisecond BoltWait for the row whose lock stays held past the wait.
  - id: F5
    severity: MEDIUM
    class: consistency
    origin: lens:guard-reach
    location: _lym/plan/33-heddle-cli.md (Intent, actingHub, and the TestVerbs refusal rows)
    summary: The actingHub refusal for a missing Bolt checkout has no test that reaches it, because the outside-a-hub row is refused earlier by lymcwd.Resolve
    fix: >-
      Add a TestVerbs row to card 33: a verb run with RunCLIIn in a plain git repository made under t.TempDir() through gitkit, whose parent directory holds no Bolt checkout.
      The row asserts the refusal naming the missing Bolt checkout and running the verb inside a hub's worktree as the way forward.
      State that row in card 33's Intent as the input and test that reach the BoltDir branch.
  - id: F6
    severity: LOW
    class: consistency
    origin: lens:repo-invariants
    location: _lym/plan/15-bolt-seam-swap.md (Intent, the internal/boardengine/doc.go bullet)
    summary: After card 15 deletes FabricReplay.Apply, boardengine doc.go's Replay section still opens with Apply re-applies and the installed board seam
    fix: >-
      In card 15's Intent, replace "internal/boardengine/doc.go's # Replay section drops any sentence naming BoardReplay or InstallBoardReplay" with:
      "internal/boardengine/doc.go's # Replay section opens with FabricReplay as the board's Bolt replayer and run ledger, registered through fabricengine.RegisterBoltReplayer.
      Its sentence beginning 'Apply re-applies' begins 'Replay re-applies', and it names neither BoardReplay nor InstallBoardReplay."
  - id: F7
    severity: LOW
    class: consistency
    origin: lens:affected-set
    location: _lym/plan/21-pull-took-in.md (Edit list and Intent)
    summary: A seam failure on a lym fabric pull --hub never reaches the printed result, since HubPullSide carries no warning
    fix: >-
      Under card 21's Edit, add internal/fabricengine#HubPullSide, internal/fabricengine#pullPairSide and internal/fabriccli#hubPullSidesMap.
      Add an Intent bullet: "HubPullSide gains Warning string, which pullPairSide sets from its pair's PullResult.Warning, and hubPullSidesMap adds a side's warning when non-empty, so lym fabric pull --hub prints it too."
      Change the ImpactSummary's closing clause to "and lym fabric pull's envelope and its --hub sides gain a warning key".
      Checked with lym loom validate-plan --overlay together with F8: no new finding.
  - id: F8
    severity: LOW
    class: scope
    origin: handler
    location: _lym/plan/04-fabric-bolt-dir.md (Create list and Intent)
    summary: TestBoltDir repeats what TestStencilsDir, TestSpecsDir and TestHubScratchAndLogsDir already assert, and pins a constant re-export and a one-line join
    fix: >-
      Remove the Create bullet plan:internal/fabricengine#TestBoltDir from card 4.
      Remove the Intent sentence beginning "TestBoltDir, a tier0 test in the package, pins".
      TestStencilsDir and TestSpecsDir, which card 4 already edits, and TestHubScratchAndLogsDir, which card 7 edits, keep covering the paths under the Bolt checkout.
      Checked with lym loom validate-plan --overlay together with F7: no new finding.
---

### [MEDIUM] Plan rulings depart from decision-record steps without a recorded entry

**Location:** `_lym/plan/00-overview.md` (Shared Decisions); cards 8, 9, 11, 13, 20, 33, 34

**Issue:** The rubric requires every Decision and Constraint of the decision record to be carried by some card. A dropped step is accepted only through an `Added after Discussion` entry, and only for a step on this task's own board entry. The overview records four departures as the chair's rulings, and the decision record carries none of them:

- The Constraints line "`lym heddle` is one cobra subtree, wired through `internal/cliwire`" is replaced by hub-only resolution (`hub-only-cli`). The ruling itself states that it departs.
- D5 and D6 name `CloneHub` as a seeding site and as a firer of "made". Card 20 moves both into `fabriccli.CloneAndWire`.
- D9's third bullet says what lives in Bolt beside the board "takes its place from `BoltDirName`, never from `BoardDirName`". The Part bolt-dir-name ruling keeps every `fabricengine.BoardDir` call outside fabric. My grep shows 79 such non-test calls, many of which mean the Bolt checkout: `NewBolt`, `parentlock.Dir`, `ReadShortname` and the `hubreconcile` geometry.
- D7 says "A commit whose claimed paths have no replayer registered fails as `ErrBoardReplayMissing` fails today". The `no-ledger-no-replay` decision fails only without a run ledger and cherry-picks a path no registered replayer claims. So a `heddle.json` commit in a process that registered only the board's replayer is cherry-picked, not refused.

Each ruling reads as well-reasoned. For example, `CloneAndWire` does wire and materialise config after `CloneHub` returns (`internal/fabriccli/clone.go:46-111`). Only the parent can record them, though, and the plan cannot depart from the record on its own authority.

### [MEDIUM] A prompt-seam error refuses every hub launch, which D4 does not license

**Location:** `_lym/plan/00-overview.md` (Decision seam-error-refuses-launch); cards 25 and 30

**Issue:** D4 fixes the seam's absence ("a missing seam reads as no passage") but says nothing about a seam error. The plan rules that an error refuses the launch.

`Passages` reads `heddle.json` under the Bolt write lock with `DefaultBoltWait` (30 seconds). A Bolt push holds that lock across its fetch and push retries (D3), and a malformed `heddle.json` also fails the read. In either case every Claude launch in the hub is refused, including the run producers'.

This is a behaviour change across all launches that the answer key does not license. It also stops agent work over text the decision record calls standing passages, which LoomYard's own sentences do not depend on (D1).

### [MEDIUM] The ended-run set-aside is vacuously true for a commit that names no run

**Location:** `_lym/plan/13-replay-through-registry.md`

**Issue:** The card sets a commit aside when the runs `TouchedRuns` names "are all ended and live". It never states that at least one run must be named. Today's code requires `len(slugs) > 0` (`internal/fabricengine/replay.go:769`), and card 12's `TouchedRuns` names no run for an overview file or for a commit with no ledger file. A literal implementation would therefore set aside every local commit touching only `heddle.json`, under an empty slug.

### [MEDIUM] A TestRender row depends on scheduling

**Location:** `_lym/plan/29-heddle-render.md`

**Issue:** `TestRender`'s `BoltWait` is "a few milliseconds". One row has the test hold the Bolt lock, swap `heddle.json` and release "within the wait". Whether the release lands inside a few-millisecond window depends on goroutine scheduling, so the row can time out instead of rendering the new entries. `lock.AcquireWriteLockWithin` returns not-acquired at its deadline (`internal/lock/lock.go:54-66`). That breaks the testing rule that no test passes only most of the time.

### [MEDIUM] actingHub's missing-Bolt-checkout refusal is never reached

**Location:** `_lym/plan/33-heddle-cli.md`

**Issue:** `actingHub` refuses in two cases: outside a hub's worktree, and where `fabricengine.BoltDir(location.HubPath)` is no directory. The only refusal row runs in a `t.TempDir()`, which is no git worktree, so `lymcwd.Resolve` refuses before the `BoltDir` check is reached. A plain git checkout that `Resolve` accepts, with no Bolt checkout beside it, reaches the second branch, and no test drives it.

### [LOW] boardengine's package doc keeps describing the deleted Apply

**Location:** `_lym/plan/15-bolt-seam-swap.md`

**Issue:** `internal/boardengine/doc.go:324-325` says `FabricReplay` "implements the board seam ... installed process-wide" and that "Apply re-applies a local board commit". Card 15 deletes `Apply` and drops only the sentences that name `BoardReplay` or `InstallBoardReplay`, which this one does not. Card 12 adds the Bolt-replayer sentence but does not retire the `Apply` sentence, so the doc goes on naming a method that no longer exists.

### [LOW] lym fabric pull --hub drops the seam warning

**Location:** `_lym/plan/21-pull-took-in.md`

**Issue:** `PullHub` runs each pair's `Fabric.Pull` in `pullPairSide` (`internal/fabricengine/hubpull.go:141`) but keeps only an outcome and a detail on `HubPullSide`. `hubPullSidesMap` prints those (`internal/fabriccli/hubpull.go:56-63`). A pair's `PullResult.Warning` is therefore lost in a hub pull, although that verb prints a result, and D6 has the move's result carry the warning.

### [LOW] TestBoltDir repeats existing path tests

**Location:** `_lym/plan/04-fabric-bolt-dir.md`

**Issue:** `TestBoltDir` pins three things:

- `BoltDirName` equals `lymcwd.BoltDirName`, a re-export the compiler already ties.
- `BoltDir` is `filepath.Join(hub, BoltDirName)`, which restates the body.
- `StencilsDir`, `SpecsDir` and `HubScratchDir` sit under `BoltDir`, which `TestStencilsDir` (`internal/fabricengine/stencilsdir_test.go:14`), `TestSpecsDir` (`specsdir_test.go:15`) and `TestHubScratchAndLogsDir` (`hubscratch_test.go:34`) already assert.

What it adds beyond them is per-symbol and redundant under PATTERN-test-speed's test economy.

## Rejected

- Lens affected-set, finding 2 (no case runs `heddleengine.Replayer` through `replayBolt`): rejected.
  - Card 13's `notesReplayer` rows pin fabric's per-replayer walk, the joint commit and the `set-aside/replayer/<name>/<sha>` tag built from `Name()`.
  - Card 31's `TestReplayer` pins heddle's `Name`, its `Claims` on `heddle.json` and its merge.
  - An end-to-end row would repeat both.
- Lens guard-reach, finding 4 (take-in sites in `syncSideBeforeMerge` and `DropLocal`): rejected. D6 enumerates its take-in sites after its colon, and cards 21 and 22 carry exactly that list. Firing elsewhere would be scope the answer key does not license.
- Handler consideration, card 13 leaving the `lym` binary without a registered replayer until card 15: rejected. The repo-invariants lens found no `fabricengine`, `boardengine` or `boardkit` tier1 test that builds a `lym` binary, so no gate between cards 13 and 15 breaks.
- Handler consideration, the store's `Save` through `state.WriteJSON` merging stale unknown keys by index: rejected. `Entry` and `File` implement `json.Marshaler`, which `internal/state` writes whole as opaque (`internal/state/doc.go`, "# Unknown keys").
- Handler consideration, the `heddle.json.swaplock` file outside `.lym`: rejected. `board.json.swaplock` already sits there, and the Bolt `.gitignore` ignores `*.swaplock`.
- Handler consideration, the plan's names `SeedExcludeList` and `ExcludeList` departing from D3's and D5's `SeedWarpExclude`: rejected. PATTERN-fabric-vocabulary forbids a side-token spelling in a non-owner file such as `heddlecli`, and the constraint wins over the record's name.

## Focus departures

- Directive: "The plan's recorded rulings that depart from a decision-record Constraint (hub-only CLI without `cliwire`) are the plan's own; check that cards 33 and 34 and PATTERN card 36 stay consistent with them."
  - Departed: the rubric's fidelity bullet keeps an uncarried Decision or Constraint a finding unless the decision record holds an `Added after Discussion` entry, and a directive never suppresses a finding the rubric requires.
  - F1 raises the cliwire departure with three other rulings of the same kind.
  - On the consistency the directive asked about: cards 33 and 34 follow the hub-only ruling (`beamcli`'s resolution, no `cliwire.Module`), and card 36 fences no cliwire text, so the three are consistent with it.
- The board entry could not be read: `lym board get` returned `task:null` for `warp-visibility` and for `psr-wv`. So the fidelity check is against the decision record alone, and no finding rests on the entry.
