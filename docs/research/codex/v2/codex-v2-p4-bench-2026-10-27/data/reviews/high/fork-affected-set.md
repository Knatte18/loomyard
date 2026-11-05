Affected sets and card coverage:

- Machine identity: new leaf package, machine-name callers and environment/test registration are covered by cards 1, 6, 8, 9, 11–14 and 16.
- Board owner fields: Task/store serialization, migration embedding, replay, payload validation, effective-owner lookup, README, JSON list/get and compact text rendering; cards 2–4 cover all except compact text rendering.
- Reconcile ownership: origin readers, classification, movement, replay retry and pairteardown seams; cards 5–6 cover the discovered callers.
- Owner pause state: run coverage, pause writing, wake, orch and tender status; cards 7 and 13–14 cover the discovered readers.
- Batten/loom refusal behavior: command entry points, worktree claim, child spawning and integration tests; cards 8–11 cover production callers, but card 9 places the new private-helper consumer in an inaccessible external test package.
- New InnerRun and Landing function fields: constructors, production wiring, shared Env fixtures, recursive nil detection and wiring-contract tests; cards 10–12 cover constructors/production wiring but miss the shared nil-seam policy and its consumers.
- Resume windows: batten command text, startup/resume entry points, reed attached-client query and existing documentation; card 15 covers the discovered readers.
- Adopt: module registration, sandbox registration, fabric cleanup, test fixture cloning, resume entry point and module documentation; card 16 covers the discovered consumers.

Findings:

1. **MEDIUM — design — `_lyx/plan/10-innerrun-owner-check.md` and `_lyx/plan/12-finalize-owner-checks.md`: New optional function fields omit the reflective seam-contract affected set.**
   Card 10 adds `InnerRun.OwnerCheck`, `PauseOwned`, `ParentHoldsTaskWork` and `Restepped`; card 12 adds `Landing.ParentHoldsTaskWork`, `ParentOwner`, `OriginParentHoldsWork` and `ParentCommitsOriginLacks`.
   Both cards explicitly permit nil defaults, but no card updates `envkit.nilLegal` or the affected shared fixtures/allowlists.
   `NilSeams` recursively reports every nil exported function unless its dotted path is in `nilLegal` (`internal/testkit/envkit/envkit.go:44`, `:502`, `:519`, `:526`).
   Consequently `TestFullEnv_FillsEverySeam`, `TestNilSeams_ReportsExactPaths` and `TestNilSeams_SkipsNilLegalPaths` fail with the added fields (`internal/testkit/envkit/envkit_test.go:14`, `:57`, `:73`).
   Loom’s ordinary and darn wiring tests also see the new InnerRun fields left nil, while both CLI wiring tests see the new Landing fields left nil until landing-specific construction (`internal/loomcli/wiring_seams_test.go:13`, `:102`; `internal/loomcli/wiring_darn_test.go:61`; `internal/battencli/wire_seams_test.go:35`, `:114`).
   Card 12’s update to `TestLandingDeps_EveryFieldPopulated` does not cover those Env-level assertions.
   This is one recurring affected-set omission: account for shared nil policy and these reflective consumers when each field set is introduced, including card 10’s gate before card 11 wires batten.

2. **MEDIUM — consistency — `_lyx/plan/09-loom-owner-check.md:16`: The new integration-test consumer cannot access its private helper or machine seam from the declared package.**
   The card declares `runOwnerVerdict` and its result type inside `internal/loomcli` (`:13`, `:32`), but declares `TestRunOwnerVerdict` in `internal/loomcli_test` (`:16`) and requires it to exercise that helper using a fixed machine (`:39`).
   No export bridge exists or is covered by a card.
   Existing landing dependency integration coverage instead uses the internal package (`internal/loomcli/landingdeps_integration_test.go:8`).
   Put this integration test in the package that owns the helper, or explicitly cover the test bridge needed by the external consumer.

3. **MEDIUM — design — `_lyx/plan/04-board-owner-views.md`: Board owner visibility omits the existing compact listing consumer.**
   D9 requires every status view to show ownership and specifically requires board list to carry each task’s effective owner (`_lyx/discussion/decision-record.md:206`, `:208`).
   Card 4 covers BriefTask, JSON list/get and README, but not `RenderCompact` or its golden expectations.
   `writeListing` selects `RenderCompact` for `list --text` and `find --text` (`internal/boardcli/cli.go:961`, `:966`); the renderer constructs its own columns and only appends status, so adding `BriefTask.OwnerMachine` alone cannot show it (`internal/boardcli/text.go:20`, `:44`, `:47`).
   Cover this renderer and `TestRenderCompact` (`internal/boardcli/text_test.go:18`) so the existing text status view exposes the same effective owner.

Tool-call audit:

1. Searched board CRUD/render definitions and whole-tree owner/window strings to discover readers missing from the manifest.
2. Read board CRUD, constructors, loadStore, skip settings and Config to trace payload and parent-resolver callers beyond the pre-read.
3. Searched Board/NewStore callers and read migration, README golden and BriefTask/replay tests to inspect uncovered persistence/view consumers.
4. Read compact rendering, Task embedding, payload construction and BriefTask tests to check storage and text-view consequences.
5. Read batten claim/InnerRun/spawn/window callers and searched window documentation and reed readers to map resume behavior’s affected set.
6. Searched CLI/engine imports, reflection and registration assertions and read associated pause/status/wiring tests to discover cross-package consumers.
7. Searched loom export bridges/test packages and read landing integration setup and batten wiring tests to check helper accessibility and seam assertions.
8. Read batten wiring allowlists, envkit nil policy/detection and loom landing assertions to identify reflective consumers outside the cards.
9. Read envkit tests, loom/darn wiring assertions and plan seam references to confirm missing coverage of the shared nil policy.
10. Retrieved exact references for nil-policy consumers and the external integration-test declaration; the attempted board `list.go` path did not exist.
11. Checked compact rendering and D9 wording and searched all cards for coverage; attempted guessed card filenames did not exist.
12. Located actual card filenames and read line-numbered recursive nil checks, wiring allowlists and listing dispatch to pin the evidence.
