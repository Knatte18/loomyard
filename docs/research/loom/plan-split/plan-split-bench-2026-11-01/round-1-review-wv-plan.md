---
verdict: BLOCKING
findings:
  - id: F1
    severity: BLOCKING
    class: design
    origin: lens:repo-invariants
    location: _lym/plan/15-heddle-cli.md:6-7
    summary: Card 15 makes boltLockWait and spawnSync package-level vars that TestHeddleCLI swaps, breaking PATTERN-test-speed (a timeout is a field or parameter, never a package var, so tests keep t.Parallel)
    fix: In card 15's Create group, replace the bullet for plan:internal/heddlecli#spawnSync with `plan:internal/heddlecli#commandDeps` -> `type commandDeps struct`, and the bullet for plan:internal/heddlecli#boltLockWait with `plan:internal/heddlecli#newCommand` -> `func newCommand(deps commandDeps) *cobra.Command`. In the Intent, commandDeps carries SpawnSync (a func(string) error) and BoltLockWait (a time.Duration). Command() returns newCommand(commandDeps{SpawnSync - boardengine.SpawnSync}) with BoltLockWait left zero, so Render's default holds. render passes deps.BoltLockWait into the RenderRequest, and set and remove start the sync through deps.SpawnSync. TestHeddleCLI builds the subtree through newCommand with a recording SpawnSync and a short BoltLockWait, and calls t.Parallel. Every mention of spawnSync and boltLockWait in the Intent and test rows changes to the deps fields.
  - id: F2
    severity: BLOCKING
    class: design
    origin: handler
    location: _lym/plan/14-prompt-passages.md:7
    summary: heddleengine.Passages fixes the Bolt-lock wait at Render's 30-second default with no way to inject it, so TestPassages' busy-lock row waits out a production duration, against PATTERN-test-speed
    fix: In card 14's Create group, replace the bullet for plan:internal/heddleengine#Passages with `plan:internal/heddleengine#PassageReader` -> `type PassageReader struct` and `plan:internal/heddleengine#PassageReader.Passages` -> `func (r PassageReader) Passages(boltDir, engine string) ([]string, error)`. In card 14's Intent, PassageReader holds BoltLockWait, and zero means Render's default. The Intent's heddleengine.Passages becomes PassageReader.Passages, and TestPassages' busy-lock row uses a PassageReader with a short BoltLockWait. In card 15, change the Uses entry plan:internal/heddleengine#Passages to plan:internal/heddleengine#PassageReader.Passages. The root pre-run installs shuttleengine.InstallPassageSource(heddleengine.PassageReader{}.Passages), and TestRootPreRun_InstallsEverySeam checks that argument.
  - id: F3
    severity: BLOCKING
    class: design
    origin: lens:repo-invariants
    location: _lym/plan/05-bolt-dir-stencils.md:39
    summary: TestEnforcement_BoltDirNameDeclaredOnce scans the whole repository but card 5 does not give it the //lym:guard marker PATTERN-test-speed requires, so narrowed gates skip the pin
    fix: In card 5's Intent, add that TestEnforcement_BoltDirNameDeclaredOnce carries `//lym:guard` on the line directly above its func line, as TestEnforcement_GeometryLiterals does at internal/lymcwd/enforcement_test.go:202.
  - id: F4
    severity: MEDIUM
    class: scope
    origin: lens:affected-set
    location: _lym/plan/05-bolt-dir-stencils.md:27
    summary: The D9 sweep misses two file-header comments that spell _board, so card 5's new pin fails at card 5's own gate
    fix: Add `internal/fabricengine/reconcile_stale_removal_test.go` and `internal/fabricengine/remove_reserved_integration_test.go` to card 5's Prosa group, after `internal/fabricengine/livestate_doc_test.go`. In card 5's Intent, add a sentence that their file-header comments, at lines 9 and 5, say the Bolt checkout and no longer spell the literal.
  - id: F5
    severity: MEDIUM
    class: consistency
    origin: lens:affected-set
    location: _lym/plan/07-bolt-replayer-registry.md:120
    summary: Card 7 says TestRecordsReplay changes the registry in no row, but its parallel row at replay_integration_test.go:684-693 installs scribbleThenFailReplay as the process-wide replay
    fix: In card 7's Tests list, move the TestRecordsReplay row "A refused restore after a failed by-entry apply tags the starting tip and names the way back" into TestBoltReplayerRegistry. There it registers scribbleThenFailReplay through SetBoltReplayersForTest and keeps its assertions. Reword the TestRecordsReplay bullet to say every other existing row passes unchanged and that this one row moved.
  - id: F6
    severity: MEDIUM
    class: decision
    origin: lens:affected-set
    location: _lym/plan/10-warp-exclude.md:28-31
    summary: Card 10 adds a repo-wide config load inside wireJunctionSet but does not say which load it is or what a failed load does to the wiring, which decides whether the existing WireJunctions test fixtures still pass
    fix: In card 10's Intent, state which load seedWarpExclude uses, strict Load or LoadOrTemplate per PATTERN-config-layers. Also state what a failed load or seed inside wireJunctionSet does - either it fails the wiring, as the junction-line seeding does, or it logs at Warn and never fails the wiring, as the scratch step does. If it fails the wiring, list on card 10 the existing WireJunctions and WireJunctionsWith test members whose fixtures hold no loadable repo-wide fabric.yaml.
  - id: F7
    severity: MEDIUM
    class: decision
    origin: handler
    location: _lym/plan/00-overview.md:56-58
    summary: Shared Decision heddle-cli-not-cliwire departs from the decision record's Constraint that lym heddle is wired through internal/cliwire, and no Added-after-Discussion entry records the departure
    fix: Do one of the following. (a) Card 15 wires lym heddle through internal/cliwire as the Constraint states, and the Shared Decision is dropped. (b) The fixer asks the parent with lym creel post --parent to record the departure in the decision record through lym loom decision add, as an Added-after-Discussion entry citing PATTERN-cliwire-sole-wiring's scope (standalone-capable CLIs). The Shared Decision then stays as written.
  - id: F8
    severity: MEDIUM
    class: decision
    origin: handler
    location: _lym/plan/00-overview.md:33-36
    summary: Shared Decision bolt-name-scope moves only fabricengine's Bolt-meaning BoardDir uses, while D9 says what lives in Bolt beside the board takes its place from BoltDirName, never BoardDirName, and that BoltDirName is the name every other package uses
    fix: Do one of the following. (a) Add a card, through lym loom insert-card after card 4, that moves the other packages' fabricengine.BoardDir uses that mean the Bolt checkout to fabricengine.BoltDir. Examples are the parentlock stores in battencli, tendercli, loomcli and pairteardown; configengine.Dirs built from it in battencli/wire.go:1336 and cleanupcli/segments.go:128; gatecli/report.go:88; preflight/predicates.go:80; orchroute/paths.go:39; cmd/lym/stencilseed.go:146. (b) The fixer asks the parent with lym creel post --parent to record the narrowing as an Added-after-Discussion entry through lym loom decision add, and the Shared Decision stays.
  - id: F9
    severity: LOW
    class: scope
    origin: lens:guard-reach
    location: _lym/plan/11-warp-seam.md:58-64
    summary: A take-in through PullHub reaches Fabric.Pull, but HubPullSide has no place for the pull's SeamWarning, so a hub-wide pull's seam failure is only logged, against D6's rule that the move's result carries it
    fix: Add `internal/fabricengine#pullPairSide` to card 11's Edit group. In the Intent, a pair side whose pull carries a SeamWarning puts that warning in the side's Detail, so PullHub's result shows it. TestWarpSeam's failing-TookIn row also runs PullHub and asserts that the prime side's Detail names the worktree and lym heddle render.
  - id: F10
    severity: LOW
    class: design
    origin: lens:guard-reach
    location: _lym/plan/09-heddle-render.md:44
    summary: Render and Passages read heddle.json under the Bolt write lock so they never see an absent file mid-move, but Show (card 9) and the list and get verbs (card 15) read it with no lock stated
    fix: In card 9's Intent, state that Show reads heddle.json the way Render does, under the Bolt write lock with the bounded wait. In card 15's Intent, state that list and get read it the same way, and that ErrBoltBusy refuses with running the verb again once the Bolt push ends as the way forward.
  - id: F11
    severity: LOW
    class: consistency
    origin: lens:repo-invariants
    location: _lym/plan/03-bolt-dir-callers-sweep.md:37-43
    summary: Card 3 allows a test-asserted configcli message to say the Bolt checkout, which breaks TestDispatchHubWideBoard until card 4, and card 3 contradicts itself and card 4 on whether asserted texts change
    fix: In card 3's Intent, configcli's two hub-wide write messages take the directory name from fabricengine.BoltDirName, keeping their text. The option to say the Bolt checkout applies only to text no test asserts. In card 4's Intent, drop the sentence "A test asserting a help or message text card 3 reworded asserts the new text."
  - id: F12
    severity: LOW
    class: consistency
    origin: handler
    location: _lym/plan/03-bolt-dir-callers-sweep.md:13,16
    summary: Every Edit target on card 3 is a non-test Go file path, while its Intent names the symbols it changes (creelengine.Geometry.StateDir's comment, hubforge.Hub.BoardDir's comment and the teardown comment)
    fix: In card 3's Edit group, replace `internal/creelengine/geometry.go` with `internal/creelengine#Geometry`, whose span 11-30 holds line 18. Replace `internal/hubforge/hub.go` with `internal/hubforge#Hub.BoardDir` (span 227-230) and `internal/hubforge#registerTeardown` (span 372-424, holding line 415). lym quarry resolve confirmed each span.
  - id: F13
    severity: LOW
    class: decision
    origin: handler
    location: _lym/plan/06-entrymerge.md:39
    summary: Card 6 leaves open whether replayRun.addEntry, changeEntry and removeEntry stay as the hooks or are deleted, yet lists them only under Edit
    fix: In card 6's Intent, replace "shrink to the hooks or go, whichever the merge leaves them" with "become the bodies of the Add, Change and Remove hooks FabricReplay.Apply passes to Merge". The three members stay in the Edit group.
---

### [BLOCKING] Package-level vars for the Bolt-lock wait and the sync spawner in heddlecli

**Location:** `_lym/plan/15-heddle-cli.md:6-7`, and lines 45 and 56.
**Issue:** `var boltLockWait time.Duration` and `var spawnSync = boardengine.SpawnSync` are package-level vars that `TestHeddleCLI` shortens or swaps.
PATTERN-test-speed's "Time is injectable" section says a production timeout a test would otherwise wait out is a struct field or a parameter, never a package-level `var`, so tests keep `t.Parallel` (`pattern/PATTERN-test-speed.md:101-105`).
Swapping `spawnSync` is process-global state that the card neither names nor uses to justify a non-parallel test (`pattern/PATTERN-test-speed.md:107-110`).
Moving both into a deps struct fixes both and lets `TestHeddleCLI` run in parallel.

### [BLOCKING] TestPassages' busy-lock row waits out the 30-second production default

**Location:** `_lym/plan/14-prompt-passages.md:7`, 35 and 42.
**Issue:** `heddleengine.Passages(boltDir, engine)` takes the lock "with the bounded wait `Render` uses", which is 30 seconds by default per card 9.
Its signature is fixed by `PassageSource`, and nothing on the card makes that wait injectable.
The row "a Bolt write lock held past the wait returns `ErrBoltBusy`" can only pass by waiting out the production duration.
PATTERN-test-speed forbids that ("No test in any tier waits out a production duration") and requires the wait to be a field or parameter.

### [BLOCKING] Repository-scanning pin without `//lym:guard`

**Location:** `_lym/plan/05-bolt-dir-stencils.md:39`.
**Issue:** `TestEnforcement_BoltDirNameDeclaredOnce` walks every `.go` file and the stencil, spec and skill trees from the repository root.
PATTERN-test-speed requires such a test to carry `//lym:guard` directly above its `func` line (`pattern/PATTERN-test-speed.md:79-88`), and its sibling `TestEnforcement_GeometryLiterals` does (`internal/lymcwd/enforcement_test.go:202`).
Without the marker, narrowed gates skip the pin whenever `internal/lymcwd` is outside the impacted set.

### [MEDIUM] Two `_board` file headers that no sweep card lists

**Location:** `_lym/plan/05-bolt-dir-stencils.md:27`; evidence at `internal/fabricengine/reconcile_stale_removal_test.go:9` and `internal/fabricengine/remove_reserved_integration_test.go:5`.
**Issue:** Both comments sit above `package`, so no member glyph owns them, and cards 2–4 list neither file.
Card 5's new pin walks every `.go` file, so it fails at card 5's own gate.
The affected-set and repo-invariants lenses both reported this.

### [MEDIUM] TestRecordsReplay's parallel row swaps the board replay

**Location:** `_lym/plan/07-bolt-replayer-registry.md:120`; evidence at `internal/fabricengine/replay_integration_test.go:684-693`.
**Issue:** The row calls `t.Parallel()` and then `t.Cleanup(fabricengine.InstallBoardReplay(scribbleThenFailReplay{boardDir: board}))`.
Under the registry, that row would register a replacement `board` replayer inside a parallel test, which affects every concurrent row.
That contradicts card 7's claim that `TestRecordsReplay` changes the registry in no row.
The affected-set and repo-invariants lenses both reported this.

### [MEDIUM] wireJunctionSet's new config load has no stated failure behaviour

**Location:** `_lym/plan/10-warp-exclude.md:28-31`; evidence at `internal/fabricengine/junction.go:92-93,115-140`.
**Issue:** `wireJunctionSet` has never loaded config; its names are supplied by the caller.
Card 10 has it call `seedWarpExclude`, which loads the repo-wide fabric config.
The card states the failure behaviour only for `Reconcile`'s once-per-pass call.
For the wiring path it states neither strict or tolerant load nor fail or warn, so whether the many existing `WireJunctions` fixtures keep passing is left to execution.

### [MEDIUM] The cliwire Constraint is departed from without a record entry

**Location:** `_lym/plan/00-overview.md:56-58`; card 15 line 35.
**Issue:** The decision record's Constraints say `lym heddle` is "wired through `internal/cliwire`".
The plan reads PATTERN-cliwire-sole-wiring as applying only to standalone-capable CLIs, and `pattern/PATTERN-cliwire-sole-wiring.md:3` supports that reading.
`boardcli` resolves through `lymcwd` directly.
Even so, the plan departs from a stated Constraint, and the decision record holds no Added-after-Discussion entry for it.

### [MEDIUM] bolt-name-scope narrows D9 to fabricengine

**Location:** `_lym/plan/00-overview.md:33-36`.
**Issue:** D9 says `BoltDirName` "is the name every other package uses".
It also says that what lives in Bolt beside the board (the `_lym` config, the `.lym` scratch tree, the write lock) takes its place from `BoltDirName`, never from `BoardDirName`.
The plan moves only fabricengine's own uses.
Other packages still reach Bolt-resident things through `fabricengine.BoardDir`:
- parent-lock stores;
- `configengine.Dirs` in `battencli/wire.go:1336` and `cleanupcli/segments.go:128`;
- the gateslot dir in `gatecli/report.go:88`;
- `.lym` in `preflight/predicates.go:80`;
- the hub rules file in `orchroute/paths.go:39`;
- `cmd/lym/stencilseed.go:146`.

The values stay equal, so nothing breaks today.
This is still a Decision step that no card carries, and no Added-after-Discussion entry records it.

### [LOW] A hub-wide pull drops the take-in seam warning from its result

**Location:** `_lym/plan/11-warp-seam.md:58-64`; evidence at `internal/fabricengine/hubpull.go:37-42,70-76,108-152`.
**Issue:** `PullHub` runs each pair's pull through `pullPairSide` into `HubPullSide{Kind, Worktree, Outcome, Detail}`, which has no place for `PullResult.SeamWarning`.
D6 has the move's result carry the warning, so on this path the warning is only logged.

### [LOW] Show, list and get read heddle.json without the Bolt lock

**Location:** `_lym/plan/09-heddle-render.md:44`; `_lym/plan/15-heddle-cli.md:38-39`.
**Issue:** D3 reads `heddle.json` under the Bolt write lock so a read never sees the file absent while git replaces it.
The plan applies that to `Render` and `Passages` only.
`Show` previews exactly what `Render` would write, and `list` and `get` print entries, yet the plan gives none of them the lock.
During a Bolt pull, any of them can print no entries.

### [LOW] Card 3 and card 4 disagree on reworded asserted messages

**Location:** `_lym/plan/03-bolt-dir-callers-sweep.md:37-43`; `_lym/plan/04-bolt-dir-test-sweep.md:50`; evidence at `internal/configcli/configcli_test.go:705`.
**Issue:** Card 3 lets a configcli message either take the constant or say "the Bolt checkout".
The second option breaks `TestDispatchHubWideBoard` until card 4 runs.
Card 3 also says no asserted text changes, while card 4 plans to update tests for texts card 3 reworded.

### [LOW] Card 3 lists file paths for named symbols

**Location:** `_lym/plan/03-bolt-dir-callers-sweep.md:13,16`.
**Issue:** Every Edit target on card 3 is a non-test `.go` file path, while its Intent names `creelengine.Geometry.StateDir`'s comment, `hubforge.Hub.BoardDir`'s comment and the teardown comment.
The plan-writer stencil keeps a file path only for a file with no symbol to name.
`lym quarry resolve` confirmed the member spans the fix names.

### [LOW] Card 6 leaves the fate of three members open

**Location:** `_lym/plan/06-entrymerge.md:39`.
**Issue:** "`addEntry`, `changeEntry` and `removeEntry` shrink to the hooks or go, whichever the merge leaves them" leaves a design choice to execution.
The members are listed only under Edit, so if they "go", the card carries no Delete for them.

## Rejected

- guard-reach finding 1, that the `replayBolt` and owner-read `ErrBoltReplayerMissing` guards are shadowed by the ended-set read: these are fail-loud wiring guards, which take no reaching test. The rows assert the error the call returns, and that holds whichever guard fires.
- guard-reach finding 3, that no row reaches an undecodable `heddle.json` as a failing passage source: `Prepare`'s failure branch does not depend on the source, and the error-returning fake source in `TestPrepare_AppendsStandingPassages` reaches it. Inside `Passages` the decode error is a propagated error.
- Handler candidate, that the Shared Decision `replayer-missing-guard` reinterprets D7's "a commit whose claimed paths have no replayer registered fails": fabric can know a claim only through a registered replayer, since D7 and PATTERN-heddle-instructions keep every board and heddle file name out of fabric. The plan's guard is the only form consistent with the record.
- Handler candidate, that "made" fires from `CloneAndWire` rather than `CloneHub`: `CloneAndWire` is `CloneHub`'s only production caller (`internal/fabriccli/clone.go:46`), so every clone still fires, and only once its prime is wired.
- repo-invariants finding F6, a separate `spawnSync` finding, was folded into F1, whose fix covers it.
