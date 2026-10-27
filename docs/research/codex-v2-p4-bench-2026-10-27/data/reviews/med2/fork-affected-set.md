Affected-set list:

- Machine identity: new package and production seams are covered by cards 1, 6, 8–9, 12–14 and 16.
- Board owner fields, payloads, replay and views: Task/Store serialization, replay fields, payload entry points, list/get and README rendering are covered by cards 2–4.
- Reconcile outcomes: classification, movement, push retry and the batten/loom/hibernate/orch/cleanup consumers are covered by cards 5–6 and 8–13, but the owner candidate set excludes existing pairs without batten records.
- Pause label and coverage: tender pause, coverage, InnerRun, wake and adoption readers are covered by cards 7, 10–11, 13 and 16.
- New InnerRun and landing seams: production wiring is covered by cards 10–12, but reflective fixture readers and direct batten wiring callers are omitted.
- Open-Window behavior: CLI help, battenshed docs, overview and client detection are covered by card 15.
- Adoption module: root registration, sandbox exclusion, destruction tables and overview are covered by card 16; the engine-less module exception list is omitted.
- Orch ownership interpretation: stencil changes are covered by card 17.

Findings:

1. **MEDIUM — design — cards 8, 10 and 12: incomplete enumeration of fixture consumers breaks existing tests.**
   Card 10 adds four optional `InnerRunDeps` function fields and card 12 adds four optional `landingshed.Deps` function fields, but no card updates `envkit.nilLegal`.
   `NilSeams` recursively reports every nil exported function unless its dotted path is explicitly exempt (`internal/testkit/envkit/envkit.go:502–534`).
   Consequently `TestFullEnv_FillsEverySeam` (`envkit_test.go:14`), batten’s `TestWire_EverySeamFilled` (`internal/battencli/wire_seams_test.go:99`) and loom’s corresponding test (`internal/loomcli/wiring_seams_test.go:92`) report the new optional fields.
   Card 11’s later wiring does not repair `FullEnv` or loom’s intentionally absent InnerRun fields, and card 12’s `landingDeps` wiring does not repair raw `Env.Landing` checked before assembly.
   The same missed-consumer method occurs in card 8: it initializes `c.owner` only through `Command`/`armAt`, then makes `wire`’s CreateWorktree closure call `c.owner.Set`.
   Existing `wireForHub` builds a literal receiver and calls `wire` directly (`internal/battencli/lifecycle_integration_test.go:105–113`), as does `TestBattenIntegration_RunReconcilesTheHubBeforeAndThePairAfterCreating` (`:237–246`); neither is covered by any card.
   Their CreateWorktree paths therefore reach a nil owner setter.
   Locations: `_lyx/plan/08-batten-owner-check.md:42–48`, `10-innerrun-owner-check.md:38`, `12-finalize-owner-checks.md:40`.
   Report as one recurring affected-set enumeration defect.

2. **MEDIUM — decision — card 5: owned tasks without batten records escape the no-replay rule and adopt’s local-commit stop.**
   Card 5 explicitly supplies only “recorded pairs” to `readOriginOwners` (`05-reconcile-owner-outcome.md:36`).
   Existing `recordedPairs` filters out every local pair without a prime-side batten status file (`internal/fabricengine/originreconcilemoves.go:435–454`), although `originTargets` includes those pairs and their records branches (`internal/fabricengine/originreconcile.go:563–584`).
   The mover processes their records branches (`originreconcilemoves.go:399–417`).
   D5 explicitly permits adopting a task with no run record, so an owned idle task with a local pair and diverged records is reachable.
   Such a task never enters the owners map: its records are replayed during adoption’s step 1, and card 16’s stop, conditioned on an `owned_elsewhere` entry, never runs.
   This contradicts D4’s fast-forward-only treatment of an owned-elsewhere entry and D5’s local-commit choice.
   The owner candidate set must cover these pairs independently of the ended-run candidate filter.

3. **BLOCKING — consistency — card 16: the new engine-less CLI is absent from PATTERN-cli-cobra’s exceptions.**
   Card 16 deliberately creates `internal/adoptcli` without an engine.
   `pattern/PATTERN-cli-cobra.md:27–35` requires `<module>cli` to import `<module>engine` and enumerates exceptions, including cleanupcli.
   No card changes that background file to register adoptcli’s decided exception.
   Location: `_lyx/plan/16-adopt-module.md`, Intent’s opening paragraph and target list.
   This is a missing affected architecture declaration, not a request to invent adoptengine.

4. **MEDIUM — design — card 9: the external integration test cannot access its private subject or fixed-machine seam.**
   Card 9 creates private `loomcli.runOwnerVerdict` but declares `TestRunOwnerVerdict` in `internal/loomcli_test`, and prescribes calling the verdict with a fixed machine.
   Neither this card nor another creates a test export for `runOwnerVerdict`; the production seam is also a private `loomCLI.ownerOf` field.
   The prescribed external test cannot directly call the function or inject that machine through the named private receiver.
   Location: `_lyx/plan/09-loom-owner-check.md:13–16,39`.
   The test’s package/access route must be made concrete without violating the hubforge dependency boundary.

Tool-call manifest:

1. Read board task construction, patch/upsert/batch/merge paths and searched old messages; inherited context lacked the payload consumers’ bodies.
2. Read board replay/render tests and batten pre-run/window bodies; inherited context lacked exact assertions and resume callers.
3. Searched child-refusal consumers, pause construction and detached push callers; read Pause’s request handling, absent from inherited context.
4. Read classifier test export shape and searched client parsing, docs and owner-related record consumers; inherited context lacked those adapters.
5. Read recordedPairs, originTargets, movement, push retry and claim closure; inherited context lacked candidate filtering and direct replay behavior.
6. Searched module invariant consumers, OpenHub and hubforge construction rules; inherited context lacked module exception and fixture boundaries.
7. Read network-gate setup, module seam signature assertions and tender Status; inherited context lacked those command/output consumers.
8. Searched arm/wire callers and read fake receiver setup; inherited context lacked test construction paths bypassing arm.
9. Read direct CreateWorktree integration callers and seam completeness checks; inherited context lacked the affected fixture bodies.
10. Read envkit’s nil exemption map, fixture builder and reflection walker, plus lifecycle helper callers; inherited context lacked reflective field consumers.
11. Read exact envkit assertions and loom/darn seam-completeness readers; inherited context lacked their failure contracts and final location evidence.
