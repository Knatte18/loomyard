I’ll read the review instructions in order, check the target against the repository and its rules, and write only the review file.

I’ve read the 33 cards, the decision record, and the relevant gate and recipe rules. I’m checking the existing callers and test fixtures to trace the effects of the proposed changes.

Reading manifest:
- Plan and answer key: `_lyx/plan/00-overview.md`, cards 1–33, and `_lyx/discussion/decision-record.md`.
- Review rules: step 1, focus directive, and step 2.
- Repository rules: `PATTERN.md` and backgrounds for test tiers, speed, economy, recipe settings, tolerant records, gate slots, fabric git, verified trees, and CLI wiring.
- Gate execution: `internal/gatecli/run.go`, `scope.go`, `tiers.go`, and `tierrun.go`; flag validation, scope resolution, tier planning, slot acquisition, guard execution, and reporting.
- Gate callers: search results for `GateCall`, `GuardTier`, and `TreeScopeVerify` in gate, loom, Webster, and landing packages.

Affected-set list:

- Gate map shape and retired names: production `GateCall`/`TierList` sites are covered by cards 1–5; fixture constructors in Webster, loom, landing, and configcli are named there. The package-level `gateslot.templateDefaults` fixture remains uncovered.
- Guard result types and `guard_tier`: impactset declarations and gatecli consumers are covered by card 14, with tagged package loads in card 11.
- Darn tree scope: production readers in darnengine, configcli, and loomcli are covered by cards 4–6; their scenarios are named.
- Command switches and `Forced`: production readers and test readers found in shedrecipe, shedadapters, and battencli are covered by card 20.
- Tier spelling: impactset/planglyph loads, planparser/shellguard guards, testtiming, and purity/isolation guards are covered by cards 11–16; test-file headers by cards 24–32. The tree-sitter package load and tmuxkit’s non-test build-tag switch remain uncovered.
- Skipped history: inspected checklist, report, shed routing, and driver consumers are covered by cards 17–18 or filter only existing stop outcomes.
- Run records: new fields preserve the existing summary regexp’s shape; cards 2, 7, 9, and 10 cover the writer and new reader.

Findings:

1. **BLOCKING — design — `_lyx/plan/11-package-load-tags.md:13`**
   **Summary:** The tier-tag migration misses two readers outside the enumerated test-file headers.
   
   `cmd/lyx/treesitterlink_integration_test.go:40` still runs `go list -test -deps -tags integration,tmux,llm`; its guard explicitly promises every tier at lines 33–34 and enforces a binary-count floor at line 57. Card 11 updates only impactset/planglyph, and card 24 changes headers only, so retagged tests disappear from this guard’s dependency graph.
   
   `internal/testkit/tmuxkit/allow_tmux.go:1` is a **non-test** Go file gated by `tmux || llm`; its `init` sets `tmuxAllowed` at line 9. Card 29 only retags test files. Pure `tier2`/`tier3` runs therefore retain `tmuxAllowed == false`; `internal/testkit/tmuxkit/tmuxkit.go:105` feeds that value into socket checking, and lines 145–146 reject even registered servers when false. Transitional commands containing old tags mask this second omission until the D13 cleanup.
   
   These are one recurring enumeration gap in migration of tag readers. The migration needs to cover the tree-sitter loader and tmuxkit’s build-time switch, retaining transitional spellings where D13 requires them. No target-changing fix was overlay-validated in this read-only fork.

2. **BLOCKING — scope — `_lyx/plan/01-gate-key-shape.md:3`**
   **Summary:** The gate-map reshape leaves the shared template fixture at the old Go type.
   
   `internal/gateslot/config_test.go:21–25` declares `templateDefaults` with `Gates: map[string][]int{...}`. Card 1 changes `Config.Gates` to `map[string]GateKey` but neither targets nor mentions this variable. Its callers include `hubWith` at line 32, `TestLoadConfig` at line 107, and `TestTemplateConfigAndLimits` at line 367. The existing literal no longer compiles after card 1, before any retagging is relevant. Both tests are named by card 1, but their shared package-level initializer is omitted from its explicit fixture migration.
   
   The fixture must adopt the new map type, keys, and scopes in card 1. No target-changing fix was overlay-validated in this read-only fork.

Tool-call manifest:

1. Searched whole-tree tag-set and removed-symbol readers; inherited context lacked tree-sitter’s load.
2. Read the tree-sitter guard and searched gate-map fixtures and Command switch consumers; inherited context lacked their definitions.
3. Searched history and verify-seam consumers and read gate fixture declarations; inherited context lacked shared initializers.
4. Searched tier classifiers, retired config prose, and history functions; inherited context lacked secondary readers.
5. Searched non-test tag-dependent code and read run-record serialization; inherited context lacked tmuxkit’s compile-time switch.
6. Read tmuxkit’s switch and searched its consumers and plan coverage; inherited context lacked the switch’s behavior.
7. Read socket enforcement and impactset result consumers; inherited context lacked the resulting failure path.
8. Checked shared-fixture callers and mechanical check exclusions; inherited context lacked whether these omissions were already gate-enforced.
