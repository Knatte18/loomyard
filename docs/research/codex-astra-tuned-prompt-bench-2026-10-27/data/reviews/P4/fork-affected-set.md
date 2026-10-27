Affected-set coverage:

- Machine identity: the new package and enforcement tests are on card 1; the existing source search found no other `os.Hostname` reader.
- Board record shape: Task, stored forms, replay, normalization and run-lock comparison are on card 2; `decodedEntry` embeds `taskFields`, so the new Task field propagates without another decoder edit.
- Board writes and views: payload checks, parent resolution, CLI help and owner output are on cards 3–4; existing batch help delegates its element fields to upsert.
- Reconcile outcome: classification, movement, push retry and common seam construction are on cards 5–6; consuming batten, loom, landing, hibernate, orch and cleanup paths are covered by cards 8–13.
- Pause label and coverage: record and tender surfaces are on card 7; InnerRun, wake and adoption consumers are on cards 10–13 and 16.
- Start/resume claims: production hooks and the direct claim test are on card 8; additional direct-wiring integration fixtures are outside its targets.
- Child refusal envelope: producer and reader are on cards 9–11.
- Finalize ownership and repair: production seams, producer tests and documentation are on card 12.
- Tender owner output: engine, CLI wiring and status tests are on card 14.
- Window reopening: reed API, batten pre-run, help and all three existing “opens no window” documentation sites are on card 15.
- Adopt registration: root command, sandbox exclusion, destruction tables and module overview are on card 16; the CLI architecture’s named deviation list is uncovered.
- Orch interpretation and owner invariant: stencil and new PATTERN background are on card 17.

Findings:

1. **BLOCKING — consistency — `_lyx/plan/16-adopt-module.md:52`**  
   **The new engine-free CLI is missing from PATTERN-cli-cobra’s permitted deviations.**  
   Card 16 expressly creates `adoptcli` with no engine, following `cleanupcli`, but neither it nor card 17 updates `pattern/PATTERN-cli-cobra.md`. That background’s line 27 requires `<module>cli` to import `<module>engine`; lines 28–35 enumerate deviations, including cleanup by name. The new module therefore requires its corresponding background change. This is a PATTERN-fit finding, not a request to invent an adopt engine contrary to D5.

2. **MEDIUM — design — `_lyx/plan/08-batten-owner-check.md`, claim wiring and test coverage**  
   **The claim-seam conversion leaves direct-wiring integration callers without an owner setter.**  
   Card 8 initializes `ownerSeams` in `Command` and `armAt`, while replacing CreateWorktree’s existing board claim with `c.owner.Set`. Existing integration callers construct `battenCLI` and invoke `wire` directly, bypassing both initialization sites: `wireForHub` at `internal/battencli/lifecycle_integration_test.go:105–113`, `TestBattenIntegration_RunReconcilesTheHubBeforeAndThePairAfterCreating` at lines 237–242, and `stepRealReadStatus_IgnoresPrimesCommittedStatusForTheSameSlug` at `internal/battencli/recordleak_integration_test.go:118–129`. They execute the real CreateWorktree closure. The card changes only `TestWire_CreateWorktreeClaimsTheBoardEntry`, leaving these callers with a nil setter under the prescribed replacement. Its statement that an unarmed receiver has “no owner check” does not specify a safe claim path. Resolve that behavior explicitly and cover the affected callers together.

Tool-call ledger:

1. Searched hostname readers, board migration paths and Open-Window text consumers; the manifest did not enumerate these dependent sites.
2. Read migration embedding, Task patching, seam drift guards and test-tier registries; these decoder and assertion dependencies were absent from inherited excerpts.
3. Read direct-wiring integration fixtures, CLI registration/signature guards and hub cloning; the manifest lacked the fixture construction paths and registration consumers.
4. Searched status/payload assertions and reconcile-state consumers; read pattern package documentation and additional direct-wiring call sites to complete those affected sets.
5. Read board payload help/contract coupling and searched module-pattern registrations; the inherited context did not show how batch payload help inherits upsert fields.
6. Read the exact direct-wiring integration callers, board configuration entry point and CLI deviation lines; these supplied the remaining evidence and locations.
