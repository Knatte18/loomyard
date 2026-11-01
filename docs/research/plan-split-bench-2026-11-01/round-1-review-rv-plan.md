---
verdict: BLOCKING
findings:
  - id: F1
    severity: BLOCKING
    class: design
    origin: lens:guard-reach
    location: _lym/plan/14-verbs-refuse-by-rows.md:48
    summary: "Card 14 adds a loomServes refusal to the circling group's pre-run but names no test that reaches it, breaking PATTERN-refusal-way-forward"
    fix: >-
      In card 14, add `internal/loomcli#TestCirclingCLI_PreRunResolvesTarget` under **Edit:** and replace the sentence
      that says the circling text is pinned by TestLoomServes and the call by the decision verb's batten row with:
      `TestCirclingCLI_PreRunResolvesTarget` gains a row whose resolved target worktree holds a seed recording batten,
      and asserts that `lym loom circling accept <slug>` exits 1 naming the recipe and the `lym shed` verbs, with no
      circling record written.
      Name that row in the card's Tests list.
  - id: F2
    severity: BLOCKING
    class: consistency
    origin: lens:repo-invariants
    location: _lym/plan/05-record-write-row.md:74
    summary: "Card 5 nests the new by-value struct RecordWriteDeps in shedrecipe.Env but adds no lym:seam marker, so TestSeamMarkers_ReflectiveTestsAreMarked fails at card 5's own gate"
    fix: >-
      In card 5, add `internal/loomcli#TestWire_BeamRecipe` and `internal/testkit/envkit#TestNilSeams_SkipsNilLegalPaths`
      under **Edit:**.
      In the Tests list, state that every test reaching `envkit.NilSeams` gains the marker
      `//lym:seam internal/loomshed#RecordWriteDeps`: `TestFullEnv_FillsEverySeam`, `TestNilSeams_ReportsExactPaths`,
      `TestNilSeams_SkipsNilLegalPaths`, `TestWire_BeamRecipe` and both `TestWire_EverySeamFilled`.
      Card 7 keeps its own targets for the `DarnDeps` path fields it adds.
      Checked with `lym loom validate-plan --overlay`: no new blocking finding.
  - id: F3
    severity: MEDIUM
    class: design
    origin: lens:guard-reach
    location: _lym/plan/05-record-write-row.md:66
    summary: "Record-Write's validation halt tells the operator to remove the failing file, which drops committed Added after Discussion entries when the row runs again after a goto"
    fix: >-
      In card 5's `Call` step 4, replace the way forward with: when the `_lym` weft repository's HEAD holds the failing
      file, restore it to its committed copy in the `_lym` weft repository, then `lym loom resume`; otherwise remove it
      from the `_lym` discussion directory, then `lym loom resume`, which writes it again.
      Drop the claim that the directory holds no `Added after Discussion` entry before the first commit, and add a
      `RecordWriteDeps` seam answering whether HEAD holds each file, filled from `fabricengine.CommittedAnchoredFile`.
      In `TestRecordWrite_Call`, add a row in which a damaged record that HEAD holds is Stuck, naming the restore and
      not the removal.
  - id: F4
    severity: MEDIUM
    class: design
    origin: handler
    location: _lym/plan/05-record-write-row.md:90
    summary: "Card 5 adds TestDriftBaseline and TestDriftAwait rows for driftengine behaviour that existing rows already pin, and no card changes that behaviour"
    fix: >-
      In card 5, drop `internal/driftengine#TestDriftBaseline` and `internal/driftengine#TestDriftAwait` from **Edit:**
      and drop the Tests bullet about the goto-route rows.
      The behaviour is already covered:
      - baseline_test.go `awaited_at set takes no capture`
      - baseline_test.go `a scope record without baseline.md spawns the agent with the scope note`
      - baseline_test.go `a scope record with baseline.md spawns no agent`
      - baseline_test.go `a passed record is left alone`
      - await_test.go `awaited_at set is Done with HEAD moved`
      The walk from Record-Write to Darn-Write stays in `TestDarnRouting`.
      Checked with `lym loom validate-plan --overlay`: no new blocking finding.
  - id: F5
    severity: MEDIUM
    class: scope
    origin: lens:affected-set
    location: _lym/plan/18-plan-keys-move-to-tapestry.md:90
    summary: "Card 18 moves implementer and the plan keys but misses readers of the loom spelling, the Implementer refusals and a FullEnv-backed build test"
    fix: >-
      In card 18, add `internal/shedrecipe#implementerEntry` and `internal/shedbuild#engineMinimalConfig` under **Edit:**.
      In the Intent's list of texts that follow, state:
      - `implementerEntry`'s three way-forward examples name `tapestry.implementer`;
      - `engineMinimalConfig`'s Implementer config and its doc name `tapestry.implementer`, which FullEnv answers once
        its settings move under tapestry.
      Add `internal/shedrecipe#TestImplementerEntry_RefusesAnUnusableSetting` when its rows assert the example text.
      Checked with `lym loom validate-plan --overlay`: no new blocking finding.
  - id: F6
    severity: MEDIUM
    class: scope
    origin: lens:affected-set
    location: _lym/plan/04-preflight-loads-batcher.md:74
    summary: "Card 4 removes Tapestry-Preflight and CheckSeed but covers only part of the prose that describes them, a pattern background file included"
    fix: >-
      In card 4, add under **Edit:**:
      - `//internal/shedrun/doc.go`
      - `//internal/loomrecipe/doc.go`
      - `//internal/hubreconcile/doc.go`
      - `//pattern/PATTERN-cwd-resolution.md`
      - `internal/loomshed#websterProducer` (its doc, webster.go:30)
      - `internal/loomshed#websterProducer.Call` (its doc and body comment, webster.go:55 and 69-72)
      - `internal/loomcli#dispositionForHandshake` (its doc, bootstrap.go:142)
      - `internal/loomrecipe#TestResume_CrashRecoveryRecallsUnconditionally` (its doc, resume_test.go:121)
      In the Intent's list of comments that follow, state that:
      - PATTERN-cwd-resolution's three tiers end at `preflight.Check`, Shed's step-1 status read being the only seed check;
      - `loomrecipe`'s doc maps `Loom-Preflight` and `Tapestry-Preflight` onto Preflight;
      - Webster's comments no longer compare against Tapestry-Preflight.
      Checked with `lym loom validate-plan --overlay`: no new blocking finding.
      Withheld:
      - `//internal/reportengine/doc.go` (line 130): `uses-later-target` refuses it on card 4 while card 3 Uses it.
      - The `internal/loomshed/gatefindings_test.go` header: `redundant-file-target` and `edit-test-file` refuse the
        file beside the card's deleted test member.
  - id: F7
    severity: MEDIUM
    class: consistency
    origin: lens:repo-invariants
    location: _lym/plan/06-one-discussion-validator.md:30
    summary: "pattern/PATTERN-gate-slot.md:29 still names the Darn-Write discussion gate and ValidateDarnDiscussion, which cards 5 and 6 remove"
    fix: >-
      In card 6, add `//pattern/PATTERN-gate-slot.md` under **Edit:**.
      Add an Intent line saying its parity list names the one `discussion` gate and `lym loom validate-discussion`
      calling `loomshed.ValidateLoomDiscussion` on every loom record.
      Checked with `lym loom validate-plan --overlay`: no new blocking finding.
  - id: F8
    severity: LOW
    class: consistency
    origin: handler
    location: _lym/plan/14-verbs-refuse-by-rows.md:19
    summary: "Card 14 makes recipeRow map retired rows through the row's own recipe, which needs a recipe parameter, but its glyph carries no re-sign arrow"
    fix: >-
      In card 14, replace `- internal/loomcli#recipeRow` with
      `- internal/loomcli#recipeRow` -> `func recipeRow(status *string, recipe string, rows []string) int`.
      Checked with `lym loom validate-plan --overlay`: no new blocking finding.
  - id: F9
    severity: LOW
    class: consistency
    origin: lens:repo-invariants
    location: _lym/plan/19-shed-keys-in-shed-alone.md:56
    summary: "internal/loomcli/doc.go:148 says the arbiter keys apply under the recipe override from darn.yaml or loom.yaml, which card 19 removes, and card 19 only Uses that doc"
    fix: >-
      Make `internal/loomcli/doc.go:148` say the arbiter keys come from `shed.yaml` and its profile block.
      The direct fix, moving `//internal/loomcli/doc.go` from card 19's **Uses:** to **Edit:**, is withheld:
      `uses-later-target` refuses it because cards 15 and 18 Use that doc.
      The fixer chooses an admissible route, for example dropping that **Uses:** line from cards 15 and 18 and then
      editing the doc on card 19.
  - id: F10
    severity: LOW
    class: decision
    origin: lens:guard-reach
    location: _lym/plan/15-decision-add-darn-ways.md:29
    summary: "The not-committed decision add refusal assumes only a crash inside Record-Write leaves an uncommitted record, but an old Darn-Write session in flight can too"
    fix: >-
      In card 15, key the not-committed text on the current row.
      At Record-Write (or with no status), name `lym loom resume`.
      On a later row, name `lym fabric commit` in the `_lym` weft repository, then re-running the verb.
      Add a `TestDecisionVerb_Refusals` row at Darn-Write for the second text.
      This narrows D4's wording, so the parent records it with `lym loom decision add` before the card changes.
  - id: F11
    severity: LOW
    class: design
    origin: lens:guard-reach
    location: _lym/plan/07-darn-write-guards.md:51
    summary: "Darn-Write reads the stop marker only when the session returns no error, and removes it before the next spawn, so a stop followed by a session error is lost"
    fix: >-
      In card 7's `Call`, when the session returns an error and the stop marker is present, halt Stuck with the
      description as the reason and the decision-add way forward instead of returning the error.
      Add a `TestDarnWrite_Call` row for a marker written by a session that then errors.
  - id: F12
    severity: LOW
    class: scope
    origin: lens:affected-set
    location: _lym/plan/05-record-write-row.md:78
    summary: "Once wire drops its darn Drift-subject branch, driftengine.SubjectBoardEntry and its branches have no production user and no card removes them"
    fix: >-
      In card 5:
      - add `internal/driftengine#SubjectBoardEntry` under **Delete:**;
      - add `internal/driftengine#subjectFields.reading`, `internal/driftengine#TestSeatTables`,
        `internal/shedrecipe#driftSubject` and `internal/shedrecipe#TestDriftEntries` under **Edit:**;
      - state in the Intent that the board-entry subject goes, `subject` accepting `decision-record` alone, and that the
        board-entry rows of `TestSeatTables` and `TestDriftEntries` go with it.
      Checked with `lym loom validate-plan --overlay`: no new blocking finding.
  - id: F13
    severity: LOW
    class: consistency
    origin: handler
    location: _lym/plan/10-recipe-branches-read-rows.md:31
    summary: "Card 10 keys wireLightweight's PATTERN record path on a DiscussionWrite row, which drops it for pawl and contradicts the card's no-behaviour-change claim and D2"
    fix: >-
      In card 10's `wireLightweight` and `wire` bullet, key `wireLightweight`'s PATTERN record path on the recipe having
      no `DarnWrite` row, as today's `c.recipe != darn` does, so pawl keeps it.
      Keep `wire`'s rule of a recipe with a `DiscussionWrite` row.
      Alternatively, state the pawl change and why D2's exception admits it.
  - id: F14
    severity: NIT
    class: consistency
    origin: lens:affected-set
    location: _lym/plan/09-one-describe.md:19
    summary: "The file header at internal/landingshed/describe.go:2 names DarnDescribeTable, which card 9 deletes, and no target covers the header"
    fix: >-
      Make the header of `internal/landingshed/describe.go` name only `DescribeTable`.
      Adding `//internal/landingshed/describe.go` to card 9 is withheld: `redundant-file-target` refuses the file
      beside the card's members of it.
      The fixer either lists the file in place of those members, when the card changes the whole file, or states the
      header line in the Intent of a member the header documents.
  - id: F15
    severity: NIT
    class: consistency
    origin: lens:guard-reach
    location: _lym/plan/10-recipe-branches-read-rows.md:28
    summary: "Card 10 says card 13's refusal keeps an unknown recipe from the wiring, but that check runs only on lym loom's own path, not on lym shed's ArmAt path"
    fix: >-
      In card 10's Intent, replace the parenthetical with: card 13's refusal on the `lym loom` path, and `lym shed`'s
      recipe lookup on the `ArmAt` path, keep one from reaching the wiring.
  - id: F16
    severity: NIT
    class: consistency
    origin: handler
    location: _lym/plan/11-tapestry-drift-before-review.md:18
    summary: "Card 11 says no Go code changes while it edits test code, and adds tapestry rows to a test still named TestNewDarn_Routing after card 3 deletes NewDarn"
    fix: >-
      In card 11, change the ImpactSummary to: reorders three edges of the tapestry recipe; only test tables change.
      Drop the blank line inside its **Edit:** list.
      In card 3, rename the darn-named routing and builder tests it rewrites so that their names name `Routing` and `New`.
---

### [BLOCKING] Circling's non-loom refusal has no reaching test

**Location:** `_lym/plan/14-verbs-refuse-by-rows.md:48`

**Issue:**
Card 14 adds a `loomServes` refusal to `circlingState.preRun`.
It argues the text is pinned by `TestLoomServes` and the call by the decision verb's batten row.
`TestLoomServes` calls only the shared function, and the decision row reaches `decisionVerb`, never circling's pre-run.
PATTERN-refusal-way-forward requires a new loom refusal to land with a test that reaches it (`pattern/PATTERN-refusal-way-forward.md:8`).
The rubric does not accept a test that calls only the refusal's constructor.
A reaching test exists to extend: `TestCirclingCLI_PreRunResolvesTarget` (`internal/loomcli/circling_integration_test.go:17`) drives the real pre-run over a resolved target.

### [BLOCKING] Record-Write's new Env struct breaks the seam-marker test at card 5

**Location:** `_lym/plan/05-record-write-row.md:74`

**Issue:**
Card 5 adds `Env.RecordWrite`, holding `loomshed.RecordWriteDeps` by value.
`envSeamStructs` walks every struct `shedrecipe.Env` nests by value (`internal/testkit/envkit/envkit_test.go:200-224`).
`TestSeamMarkers_ReflectiveTestsAreMarked` reports a test that reaches `NilSeams` without a marker for each of them.
`TestWire_BeamRecipe` and `TestNilSeams_SkipsNilLegalPaths` reach `NilSeams`, are not card 5 targets, and carry markers for `DarnDeps` only (`internal/loomcli/wiring_beam_test.go:24-38`).
Card 5's own tier0 gate therefore fails, and its Intent never mentions the marker on the tests it does list.
Card 7 lists these tests, but card 7 adds no Env struct, so they sit on the wrong card.

### [MEDIUM] Record-Write's validation halt names a removal that loses committed entries

**Location:** `_lym/plan/05-record-write-row.md:66`

**Issue:**
Step 4 has a damaged file removed, and justifies it by saying the directory holds no `Added after Discussion` entry before the pair is first committed.
Record-Write also runs after its first commit: on any goto back to the row, a route card 5 itself tests (`05-record-write-row.md:88`).
On that path the record holds committed entries.
Removing it and writing it again from the board entry drops them, which D5 warns against and the history check of cards 6 and 7 then refuses (`decision-record.md:123-124`).

### [MEDIUM] Card 5's driftengine test rows duplicate existing rows

**Location:** `_lym/plan/05-record-write-row.md:90`

**Issue:**
No card changes driftengine behaviour, yet card 5 plans new `TestDriftBaseline` and `TestDriftAwait` rows for the goto route.
Each behaviour is already pinned:
- `awaited_at set takes no capture` (`internal/driftengine/baseline_test.go:220`)
- the scope-record-without-`baseline.md` row that spawns the agent (`:196`)
- the scope-record-with-`baseline.md` row that spawns none (`:210`)
- `a passed record is left alone` (`:241`)
- `awaited_at set is Done with HEAD moved` (`internal/driftengine/await_test.go:41`)

### [MEDIUM] Card 18 misses the loom.implementer readers in shedrecipe and shedbuild

**Location:** `_lym/plan/18-plan-keys-move-to-tapestry.md:90`

**Issue:**
The three refusals of `implementerEntry` give `loom.implementer` as the example way forward (`internal/shedrecipe/entries_implementer.go:30,33,41`).
That key no longer exists in `loom.yaml` after the move.
`engineMinimalConfig` sets `implementer_setting: loom.implementer` and documents that FullEnv's template lookup answers it (`internal/shedbuild/build_engines_test.go:22,62`).
Card 18 moves FullEnv's plan settings under `tapestry`, so that lookup no longer holds, and card 18 lists neither glyph.

### [MEDIUM] Card 4 leaves Tapestry-Preflight and CheckSeed prose behind

**Location:** `_lym/plan/04-preflight-loads-batcher.md:74`

**Issue:**
Card 4 covers every code reader of the removed row and check, but only part of the prose readers.
Its Intent names updates to `internal/shedrun/doc.go:6` and the `gatefindings_test.go` header, yet neither is a target.
These still describe the removed row or check, with no card listing them:
- `pattern/PATTERN-cwd-resolution.md:21`, a PATTERN background file whose three tiers end at `loomengine.CheckSeed`
- `internal/loomrecipe/doc.go:27`, which maps `Loom-Preflight` onto Tapestry-Preflight
- `internal/hubreconcile/doc.go:85`
- the doc and body comments of `websterProducer` and `websterProducer.Call` (`internal/loomshed/webster.go:30,55,69-72`)
- the doc of `dispositionForHandshake` (`internal/loomcli/bootstrap.go:142`)
- `internal/reportengine/doc.go:130`, held only under Uses
- the doc of `TestResume_CrashRecoveryRecallsUnconditionally` (`internal/loomrecipe/resume_test.go:121`)

`lym quarry resolve` confirmed the line spans of each named member.

### [MEDIUM] PATTERN-gate-slot's background still names the darn discussion gate

**Location:** `_lym/plan/06-one-discussion-validator.md:30`

**Issue:**
`pattern/PATTERN-gate-slot.md:29` lists the Darn-Write row's `discussion` gate and `validate-discussion` on a darn run as calling `loomshed.ValidateDarnDiscussion`.
Card 5 drops that gate and card 6 deletes the function, and no card edits the background file.

### [LOW] recipeRow's new recipe input has no re-sign arrow

**Location:** `_lym/plan/14-verbs-refuse-by-rows.md:19`

**Issue:**
`recipeRow` is today `func recipeRow(status *string, rows []string) int` (`internal/loomcli/peers.go:83`).
Card 14 has it map a retired producer through `RetiredRows` of the row's recipe, which it cannot do without a recipe input.
The glyph carries no arrow.

### [LOW] loomcli's doc keeps the removed arbiter override

**Location:** `_lym/plan/19-shed-keys-in-shed-alone.md:56`

**Issue:**
`internal/loomcli/doc.go:148` says the arbiter keys apply under the run's recipe override, `darn.yaml`'s for a darn run and `loom.yaml`'s otherwise.
Card 19 deletes that override, yet holds the doc under Uses, and the Intents of cards 13 and 14, which edit the doc, do not cover the line.

### [LOW] The not-committed refusal's premise misses an in-flight Darn-Write session

**Location:** `_lym/plan/15-decision-add-darn-ways.md:29`

**Issue:**
The old `darnWrite.Call` commits only after its session returns (`internal/loomshed/darnwrite.go:85-92`).
A darn run in flight inside that session can therefore leave a record written but uncommitted with its current row at Darn-Write.
After card 9, the new Darn-Write commits through the description commit, so `lym loom resume` never commits that record.
The refusal then repeats the same way forward.
The card follows D4's text, so the change needs the parent's decision.

### [LOW] A stop marker followed by a session error is lost

**Location:** `_lym/plan/07-darn-write-guards.md:51`

**Issue:**
Step 3 reads the marker only after the session returns with no error.
On a session error, Shed re-steps the row, and step 2 removes the marker before the next spawn.
A no-parent writer that wrote its stop report and marker and then timed out is spawned again on the entry it judged outdated.

### [LOW] The board-entry drift subject becomes dead code

**Location:** `_lym/plan/05-record-write-row.md:78`

**Issue:**
After card 5, `internal/loomcli/wiring.go:787` is the last production user of `driftengine.SubjectBoardEntry` (`internal/driftengine/seats.go:40`), and card 5 removes that branch.
The subject's case in `subjectFields.reading` (`seats.go:64`) and its acceptance in `driftSubject` (`internal/shedrecipe/entries_drift.go:48`) stay, unused.

### [LOW] Card 10 changes pawl's lightweight wiring while claiming no change

**Location:** `_lym/plan/10-recipe-branches-read-rows.md:31`

**Issue:**
Today `wireLightweight` sets the PATTERN record path for every recipe but darn, pawl included (`internal/loomcli/wiring.go:300`).
Card 10 sets it only for a recipe with a `DiscussionWrite` row, which pawl lacks.
Card 10 nevertheless says no behaviour changes for pawl, and D2 keeps every wiring path pawl uses today.

### [NIT] describe.go's file header keeps the deleted darn builder

**Location:** `_lym/plan/09-one-describe.md:19`

**Issue:**
`internal/landingshed/describe.go:2` names `DescribeTable and DarnDescribeTable`.
Card 9 deletes the second, and its targets are members of that file only.

### [NIT] Card 10 credits card 13 for a check lym shed's path never runs

**Location:** `_lym/plan/10-recipe-branches-read-rows.md:28`

**Issue:**
Card 13 runs `loomServes` only when `loomOnly` is set, which the `lym shed` path through `ArmAt` never sets (`13-loom-verbs-loom-only.md:25`).
On that path, `lym shed`'s own recipe lookup refuses an unknown recipe.

### [NIT] Card 11's impact line and the darn-named routing test

**Location:** `_lym/plan/11-tapestry-drift-before-review.md:18`

**Issue:**
Card 11 edits `wantProducerTable` and `TestNewDarn_Routing`, yet its ImpactSummary says no Go code changes.
A blank line splits its **Edit:** list.
After card 3 deletes `NewDarn`, a test named `TestNewDarn_Routing` that holds tapestry rows misnames what it covers.

## Rejected

- Handler candidate: card 7 builds Darn-Write's guard with `DiscussionWritten` false in `wireDarn` instead of reading `RecordEngine`. Rejected: card 10 selects `wireDarn` by the recipe's `DarnWrite` row, not its name, and darn's record row is `RecordWrite`, so D5's rule holds.

## Notes

The focus questions are settled below.

**Fidelity**
- D1 is carried by cards 2 and 5.
- D2 by card 13.
- D3 by card 14, with card 7 for the darn stencil's PATTERN bullets.
- D4 by cards 1, 5, 7 and 15:
  - the missing-record halt with its goto and restore-from-HEAD routes: card 7;
  - `decisionWayNoRecord` by current row and the three darn refusals: card 15.
- D5 by cards 6 and 7, with card 7 carrying the validate-only guard and its halt text.
- D6 by card 7.
- D7 by card 5.
- D8 by cards 2, 3, 8, 9 and 10.
- D9 by card 11.
- D10 by card 4.
- D11 by cards 12 and 13.
- D12 by cards 17 and 18.
- D13 by cards 16 and 19, with one Spec text through `RetiredWayForward` and both the `ValidateLocal` and `PrepareRunSettings` refusals.
- D14 by the overview's shared decision.

Every Constraint has a carrying card.

**Card order**
No card uses a symbol only a later card creates:
- `RecordWrite` is a name in card 2's engine set, and its constructor lands in card 5, the card that first declares a recipe whose record row runs it.
- `Routing` (card 3) precedes every caller.

**Scope**
Cards 16 to 19 stay inside D12 and D13:
- no card moves `batten.yaml`'s overrides;
- no card copies a retired key into `shed.yaml`;
- card 18 edits the shipped `internal/runprofile/template.yaml`, never the hub's `profiles.yaml`.

**Research steps and the board lock**
No card leaves a fact to be found while it runs, edits this task's board entry, or files or promotes a task.
