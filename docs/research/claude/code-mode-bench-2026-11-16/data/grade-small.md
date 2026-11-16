# Grade: three plans against keys L1-L9 (base 0604f2ea)

Grades: R repeated, P partly, A avoided, n/a other route. Pairs: sa, sb, sc.

## Key table

| Key | sa | sb | sc |
|---|---|---|---|
| L1 helper tests | A: "TestNameShow ... TestNameRefusesWithoutWriting ... TestNameSet ... TestNameSweep" (verb-level only) | A: "`TestName` ... is one table over `runWith`" | R: "`TestNextFile` (tier0) is a table over `nextFile`" |
| L2 competing-write subtest on replace set | A: no such test (only "ReplaceSetSelectsAnOwnerOnlyOriginHeld", a different case) | A: no such test (one subtest, origin-only owner pushed by `pushFromOther`) | A: no such test ("ReplaceSetSweepsAnOwnerOnlyOriginHolds"; the review rejected a Lost row as restating CompetingWriteLosesToOrigins) |
| L3 three doc sentences | P: "`SetOwners`'s doc comment states the replace-set case" and the doc.go Takeover sentence is edited; `OwnerRequest.Takeover` comment and doc.go:174-175 left (review F6) | R: "`internal/boardengine/doc.go` ... **Extends:**" only; Takeover and SetOwners comments untouched (review F2) | A: "`SetOwners`' and `OwnerRequest`'s doc comments state the replace set; `doc.go`'s Owners section states that `SetOwners` takes another machine's owner with `Takeover` ... or through the replace set" (doc.go:174-175 subject lines still stale, review F2) |
| L4 unpushed read error | R: "reads `fabricengine.NewBolt(...).HasUnpushed()` before the write", no error outcome | A: "A `HasUnpushed` error leaves the hub reported by its write alone." | R: "`sweepHub` reads `hasUnpushed`, then calls `setOwners`", no error outcome |
| L5 failed-hub way forward branches | A: "reports `failed` with the write's error and the way forward, a re-run of `lym machine name <name>`" | A: "a write error: the hub, the error and the way forward `lym machine name <name>`" | A: "`failed`, with the error and the way forward `lym machine name <name>`" |
| L6 seam counted in prose | A: no such count | P: "`cmd/lym/seamsignature_test.go` gains ... in its two pin lists" (counts test pin lists, not a fake) | A: no such count |
| L7 hostname export bound / doc by comparison | R: "The doc comment says it ignores the machine file, unlike `Name` and `Names`." no caller bound | P: "It reads the hostname on every call, whatever the machine file holds."; doc line "for a caller that needs it beside the file's names"; no bound on callers | P: doc sentence "`HostnameName` gives the hostname rule's name regardless of the machine file, for the verb that records it among the machine's names"; names the one caller, states no bound |
| L8 TestName_Set row without input / repeated assertion | A: each `TestNameSet` row names its input ("`osl46` over `{"name":"osl-1046","names":["osl-1046"]}`") | A: rows name inputs ("`osl46` over `osl-1046` writing `names` `osl-1046` then `osl46`") | A: `TestNextFile` rows name inputs ("a file with only `{"name": "osl46"}` adds the hostname name") |
| L9 tier1 real hubs for re-run pinned elsewhere | P: "with the origin restored, a re-run with the same name sweeps it" inside tier1 `TestNameSweep`; boardengine already pins the rewrite | A: tier1 `TestName_SweepsRealHubs` is one rename; re-run is a tier0 row ("a re-run with the name already set writes no file yet sweeps again") | A: tier1 `TestSetName_SweepsRealHubs` is a rename plus an origin-only collision; the re-run is a tier0 row |

Totals: sa 2R, 2P, 5A. sb 1R, 2P, 6A. sc 2R, 1P, 6A.

## Per-plan figures

Landed change: commit 380101d7 "Add lym machine name to show and set the machine name" (23 files). Cards name new machinecli code by `plan:` symbol handles, so only files named outright are compared; machinecli source files cli.go, current.go, hubs.go, set.go are not named by any plan.

### sa
- Cards: 4 (01-machinename-hostname-name, 02-owner-replace-set, 03-machine-name-verb, 04-owner-sweep).
- Source files named: internal/machinename/machinename.go, machinename_test.go, doc.go (doc.go under Uses only); internal/boardengine/ownerset.go, ownerset_integration_test.go, doc.go; PATTERN.md, pattern/PATTERN-owner-machine.md, pattern/PATTERN-cli-cobra.md; internal/machinecli/doc.go, testmain_test.go, cli_test.go, name_integration_test.go (plus a tier1 file for TestNameSweep); cmd/lym/main.go.
- Named and landed: cmd/lym/main.go; boardengine ownerset.go, ownerset_integration_test.go, doc.go; machinename.go, machinename_test.go; pattern/PATTERN-owner-machine.md, pattern/PATTERN-cli-cobra.md; machinecli/doc.go, testmain_test.go, cli_test.go.
- Named, not landed: PATTERN.md; internal/machinename/doc.go; machinecli/name_integration_test.go (landed as sweep_integration_test.go).
- Landed, not named: cmd/lym/sandbox_coverage_test.go; cmd/lym/seamsignature_test.go; docs/overview.md; internal/loomcli/smoke_test.go; internal/sheddriver/drive.go, driverules.go; machinecli cli.go, current.go, hubs.go, set.go, set_test.go, sweep_integration_test.go.
- Review: verdict BLOCKING; 12 findings: 1 BLOCKING, 4 MEDIUM, 6 LOW, 1 NIT.
  - F1 BLOCKING: mounting `machine` fails TestSandboxCoverage_AllModulesCoveredOrExcluded; no excludedModules entry.
  - F2 MEDIUM: sweptHubs drops the cwd hub when lymcwd.Resolve fails off the worktree root; no row.
  - F3 MEDIUM: D5's collision-check bounds and shared-name sweep bound not carried.
  - F4 MEDIUM: no test reaches the Lost path of a replace-selected entry.
  - F5 MEDIUM: no row reaches the `pushed` report.
  - F6 LOW: card 2's doc.go edit misses lines 174-175.
  - F7 LOW: Replace with Prime can report the prime both recorded and lost.
  - F8 LOW: three error branches (ReadOwners, ReadFile read error, FilePath error in show) have no outcome.
  - F9 LOW: TestNameSweep row 1 repeats card 2's SetOwners rows.
  - F10 LOW: a row tests the Review-marked recoveries wording.
  - F11 LOW: the collision refusal's hand-write way forward is conditional on state checkCollision cannot see.
  - F12 NIT: TestNameSet's hostname unstated.
- Contradicts the record: none found. Gaps only (seamsignature_test, sandbox coverage, docs/overview).

### sb
- Cards: 4 (01-owner-replace-set, 02-machinename-hostname-name, 03-machine-name-verb, 04-machine-name-sweep).
- Source files named: internal/boardengine/ownerset.go, ownerset_integration_test.go, doc.go; PATTERN.md, pattern/PATTERN-owner-machine.md, pattern/PATTERN-cli-cobra.md; internal/machinename/machinename.go, machinename_test.go, doc.go; internal/machinecli/doc.go, testmain_test.go, name_test.go, sweep_integration_test.go; cmd/lym/main.go, cmd/lym/seamsignature_test.go; docs/overview.md.
- Named and landed: cmd/lym/main.go, seamsignature_test.go; docs/overview.md; boardengine ownerset.go, ownerset_integration_test.go, doc.go; machinename.go, machinename_test.go; pattern/PATTERN-owner-machine.md, pattern/PATTERN-cli-cobra.md; machinecli/doc.go, testmain_test.go, sweep_integration_test.go.
- Named, not landed: PATTERN.md; internal/machinename/doc.go; machinecli/name_test.go (landed as cli_test.go and set_test.go).
- Landed, not named: cmd/lym/sandbox_coverage_test.go; internal/loomcli/smoke_test.go; internal/sheddriver/drive.go, driverules.go; machinecli cli.go, current.go, hubs.go, set.go, cli_test.go, set_test.go.
- Review: verdict BLOCKING; 14 findings: 1 BLOCKING, 5 MEDIUM, 8 LOW, 0 NIT.
  - F1 BLOCKING: show reads through machinename.Names, so show rows read the developer's file (D8) and no source is reported.
  - F2 MEDIUM: card 1 lists boardengine/doc.go under Extends while its change falsifies doc.go:174/183 and the Takeover and SetOwners comments.
  - F3 MEDIUM: card 4 leaves owner seeding for the tier1 test to be researched.
  - F4 MEDIUM: card 3 refusals (FilePath error, ReadOwners error, registry read error) have no row.
  - F5 MEDIUM: hand-write way forward rests on a lost-file state the verb cannot see; no row.
  - F6 MEDIUM: card 3 states no bound for the new verb.
  - F7 LOW: research step for the board directory.
  - F8 LOW: empty name in Replace never meets an unowned beam in the subtest.
  - F9 LOW: Replace combined with Slugs/Prime can write twice; no row.
  - F10 LOW: step 3 conflates read error with decode error.
  - F11 LOW: several prescribed branches have no TestName row.
  - F12 LOW: osl46 row's hostname unstated.
  - F13 LOW: tier1 sweep test's deps unstated.
  - F14 LOW: a row tests Review-criterion wording.
- Contradicts the record: show through `machinename.Names` conflicts with D8 (tests reach the file only through seams). Bad-file show "exits 0" is unspecified by the record, not contradicted. Nothing else.

### sc
- Cards: 4 (01-owner-replace-set, 02-hostname-name, 03-machine-name-show, 04-machine-name-set).
- Source files named: internal/boardengine/ownerset.go, ownerset_integration_test.go, doc.go; pattern/PATTERN-owner-machine.md, PATTERN.md, pattern/PATTERN-cli-cobra.md; internal/machinename/machinename.go, machinename_test.go (TestHostnameName), doc.go; internal/machinecli/doc.go, testmain_test.go; cmd/lym/main.go. Other machinecli code is by `plan:` handle.
- Named and landed: boardengine ownerset.go, ownerset_integration_test.go, doc.go; pattern/PATTERN-owner-machine.md, pattern/PATTERN-cli-cobra.md; machinename.go, machinename_test.go; machinecli/doc.go, testmain_test.go; cmd/lym/main.go.
- Named, not landed: PATTERN.md; internal/machinename/doc.go.
- Landed, not named: cmd/lym/sandbox_coverage_test.go; cmd/lym/seamsignature_test.go; docs/overview.md; internal/loomcli/smoke_test.go; internal/sheddriver/drive.go, driverules.go; machinecli cli.go, current.go, hubs.go, set.go, cli_test.go, set_test.go, sweep_integration_test.go.
- Review: verdict BLOCKING; 13 findings: 1 BLOCKING, 5 MEDIUM, 5 LOW, 2 NIT.
  - F1 BLOCKING: card 3 mounts `machine` with no sandbox Covers tag or excludedModules entry.
  - F2 MEDIUM: replace-only commit subject unpinned; example needs Prime; row has no wantSubject; doc.go:174-175 stale.
  - F3 MEDIUM: three setName refusals (relative variable, registry error, readOwners error) have no row.
  - F4 MEDIUM: D5 repair paths tested only by TestNextFile below the verb.
  - F5 MEDIUM: cwdHub maps every Resolve failure to no hub; a subdirectory run silently skips the hub.
  - F6 MEDIUM: tier1 test reads the host's hostname and cwd.
  - F7 LOW: `pushed` needs a second hasUnpushed read.
  - F8 LOW: seamsignature_test.go misses machinecli.
  - F9 LOW: docs/overview.md lacks machinecli.
  - F10 LOW: written-once rule guards an unsent request shape.
  - F11 LOW: showName with a failing hostname unprescribed.
  - F12 NIT: ImpactSummary misstates the new fixture entries' reach.
  - F13 NIT: cli-cobra deviation omits standalonestate.
- Contradicts the record: none found. D5's bounds on the collision check are not stated in the cards (doc.go carries only the write bound); an omission.
