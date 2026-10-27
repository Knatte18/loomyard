I’ll read the instructions in order, check the target against the fasit and repository code, and write only the review file.

I’ve read all 33 cards and the decision record. I’m now tracing the gate and entry-gate call paths, including config migration, resume behavior, history readers, and tests.

Reading manifest:
- Plan and criteria: `_lyx/plan/00-overview.md`, cards 1–33, `_lyx/discussion/decision-record.md`, `.bench/instr/focus.md`, and `.bench/stencils/loom/loom-template-plan.md`.
- Repository rules: `PATTERN.md` and the testing, recipe lookup, verified-tree, tolerant-records, failure visibility, gate-slot, config ownership, parser, CLI, and refusal backgrounds.
- Gate execution: `internal/gatecli/run.go`, `scope.go`, `tiers.go`, and `counters.go`; `internal/gateslot/tiers.go` and `runlog.go`.
- Gate callers: `internal/webstercli/beginbatch.go`, `internal/loomcli/roundgate.go`, and `darnwiring.go`; references to gate lists, commands, guard tiers, and tree-scope settings.
- Shed state: `internal/shedengine/run.go` entry, routing, budget, and persistence functions; history-reader references across shed, loom, reports, batten, and the driver.

Affected-set list:

- Gate config shape and names: `Config`, `configFile`, `TierList`, `GateCall`, `TiersValue`, migration, Webster callers, loom landing sources, and fixtures are covered by cards 1–5; the package-level `templateDefaults` initializer is missed.
- Tree-scope removal: config loading, migration registration, editor refusals, loom wiring, and corresponding tests are covered by card 6.
- Green carry and summaries: execution, `RunRecord`, `TierSummaryLine`, and `FailedTier` coverage are addressed by card 7; per-tier scope derivation by card 8.
- Counter and report changes: both existing counter implementations, `runTier`, records, and report consumers are covered by cards 9, 10, and 15.
- Package loads and tier-sensitive scans: impactset, planglyph, planparser, shellguard, testtiming, tier purity, LLM purity, and tmux isolation are covered by cards 11–16; the tree-sitter link guard’s package load is missed.
- Guard result shape: scan implementations, gatecli consumers, pure target tests, and module-scan tests are covered by card 14; `TestDerive_OverARepository` still asserts the old shape.
- Entry gates and history: engine routing, status persistence, goto/relaunch/rename, checklist, coherence, hand-step reports, timing reports, and notice classification are addressed by cards 17–19.
- Command switches: producer, recipe constructor, batten wiring/reopen path, and associated tests are covered by card 20.
- Darn budget routing: `NewDarn`, `DarnRouting`, removed rewrite, and routing tests are covered by card 21.
- Retag corpus: the package directories found by enumerating repository `_test.go` files outside testdata match cards 24–32.
- Documentation and final scan: cards 22, 23, and 33 carry their stated changes.

Finding:

**BLOCKING — design — `_lyx/plan/01-gate-key-shape.md:3`, `_lyx/plan/11-package-load-tags.md:3`, `_lyx/plan/14-guard-step-per-tier.md:8`**

**Summary:** The affected-set enumeration omits existing initializer, assertion, and package-load consumers, so the planned commits cannot retain passing gates.

Evidence:

1. Card 1 changes `Config.Gates` to `map[string]GateKey`, but does not cover `internal/gateslot#templateDefaults`.
   `internal/gateslot/config_test.go:21–24` initializes that field with `map[string][]int`.
   This initializer lies outside every test member card 1 targets, and remains a compile error after its field-type change.

2. Card 14 changes `Targets.Guards` to `map[string][]GuardTest`, but omits `internal/impactset#TestDerive_OverARepository`.
   `internal/impactset/derive_integration_test.go:102` and `:162` compare it using `reflect.DeepEqual` against `map[string][]string`.
   Those comparisons necessarily fail after card 14.
   Card 11 touches this test earlier solely to add a tier0 import-edge case; it neither covers this later representation change nor can use the `GuardTest` type before card 14 creates it.

3. Cards 11 and 16 omit `cmd/lyx#TestTreeSitterLink_OnlyAllowedTestBinariesLinkIt`.
   Its package load at `cmd/lyx/treesitterlink_integration_test.go:40–41` still selects only `integration,tmux,llm`.
   Retagging its own file in card 24 changes its header, not this command.
   After cards 24–32, ordinary tier-tagged test files disappear from this guard’s dependency graph, compromising its scan and potentially triggering its floor or stale-allowlist failures (`:50–60`).
   `pattern/PATTERN-test-speed.md:95–99` explicitly requires this guard to inspect every test tier.

These are one recurring enumeration defect, rather than three independent findings.
The first two are concrete build/test failures; the third is another omitted consumer of the same migration.
They are not `caller-uncovered` duplicates: that check applies to deleted/re-signed members, whereas these omissions concern a changed field representation and unchanged test-body literals.

The handler should validate any proposed target additions through the required overlay procedure before writing the finding’s `fix`.

Tool-call manifest:

1. Searched tag loaders, guard-result readers, retired tree-scope fields, and Command switch consumers omitted from the inherited searches.
2. Read the tree-sitter scanner, impactset integration assertions, target representation, and gate config literals to inspect the newly found consumers.
3. Enumerated test-package directories, read gate defaults and envkit lists, and searched history consumers to compare the retag coverage and shared fixtures.
4. Searched counter implementations and summary readers; read darn routing, timing-row construction, and row-entry counting beyond the manifest.
5. Followed Darn outcome readers and entry-gate builder callers, including routing-only projections and batten fixture construction.
6. Checked tree-sitter imports and plan coverage of the newly found members, and searched remaining retired gate-name calls.
7. Searched outcome-sensitive history readers and read Darn’s prior-work consumer.
8. Read reroute-cap counting, preflight re-entry interpretation, batten classification, and their test references to assess skipped-history reach.
9. Obtained exact line references for the three omissions and read the review-format rules.
10. Checked affected files’ headers/guard markers and the spec’s `caller-uncovered` boundary to exclude mechanically duplicated findings.
