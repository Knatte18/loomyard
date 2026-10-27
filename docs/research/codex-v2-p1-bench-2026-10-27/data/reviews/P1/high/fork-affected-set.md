I’ll read the exploration instructions first, then inspect the target and its code paths before writing the review.

I’ve read the plan through the first retagging card and the main decisions. I’m now tracing the gate, recipe and history readers to check coverage and card ordering.

The exploration now covers gate rendering, scope selection, test counters, recipe construction and shed persistence. I’m checking the tag guards and transition tests next, then I’ll apply the review rules.

Reading manifest:

- Plan and answer key: `_lyx/plan/00-overview.md`, cards 1–33, and `_lyx/discussion/decision-record.md`.
- Review contracts: `PATTERN.md`, the focus directive, plan spec and plan-writer stencil.
- Gate execution: `gatecli/scope.go`, `tiers.go`, `tierrun.go`, `run.go`, `counters.go`; scope derivation, tier planning, execution, reporting and counters.
- Gate configuration: `gateslot/tiers.go`, `runlog.go`; tier declarations, gate lookup/rendering and run records.
- Verify wiring: `loomshed/verifygate.go`, `shedrecipe/entries_gate.go`, `loomcli/roundgate.go`, `loomcli/darnwiring.go`, `webstercli/beginbatch.go`.
- Shed state: `shedengine/run.go`; entry, pause, routing, budget and persistence paths.
- Recipe construction: `shedbuild/build.go`, `shedbuild/parse.go`; Build, Parse and shape checks.
- Tag guards: `cmd/lyx/tierpurity_test.go`, `llmtier_test.go`, `treesitterlink_integration_test.go`; tag classification and package loading.
- Test support: `testkit/tmuxkit/allow_none.go`, `allow_tmux.go`; tmux admission.
- Background: test tiers, speed, economy, isolation, testkit, hub fixtures, gate slots, verified trees, tolerant records, recipe settings, shed verbs, failure visibility, sole parsers, config layers/strictness, hub config, state placement, geometry, CLI wiring, refusals, hub GC, machine ownership, pane resolution, temporary files, leaf packages, documentation lifecycle and review rounds.

Affected-set list:

- Gate configuration shape and retired names: `Config`, config decoder/migration, template, gate renderers, loom/landing sources and test fixtures are covered by cards 1–6; `gateslot.templateDefaults` is omitted.
- Rendered gate calls: direct renderers and several direct tests are covered by card 3; downstream card-list, begin-batch, recovery and CLI prompt assertions are omitted.
- Green records, carried summaries and launch records: execution/reporting, `RunRecord`, summary parsing and counter readers are covered by cards 7–10.
- Package-loading tag sets: impactset and planglyph are covered by card 11; the tree-sitter linkage guard’s package load is omitted.
- Tier-sensitive guards: planparser/shellguard, testtiming, purity/LLM/isolation guards and test headers are covered by cards 12–16 and 24–33; tmuxkit’s non-test build-tag switch is omitted.
- Per-tier guard results: production callers and direct tests are covered by card 14; `TestDerive_OverARepository` still asserts the old result type after its earlier card-11 edit.
- Entry gates and history: persistence, goto/relaunch/rename, recipe construction and the named checklist/report/coherence readers are covered by cards 17–19.
- Command switches and zero-budget routing: producer, recipe, batten wiring/reopen path and darn routing are covered by cards 20–21.
- Retag package enumeration: comparing repository test-package directories with cards 24–32 found no missing package.

Finding — **BLOCKING**, **design**, location: `_lyx/plan/01-gate-key-shape.md`, `_lyx/plan/03-gate-call-renders-key.md`, `_lyx/plan/11-package-load-tags.md`, `_lyx/plan/14-guard-step-per-tier.md`, `_lyx/plan/16-tier-purity-tier-names.md` and the retag shared decision.

**The affected-set enumeration misses existing fixtures, downstream assertions and tag consumers, so the planned cards do not preserve their own gates or complete the tag migration.**

Concrete members omitted by every applicable card:

1. `internal/gateslot#templateDefaults` initializes `Config.Gates` as `map[string][]int` at `internal/gateslot/config_test.go:21–25`. Card 1 changes that field to `map[string]GateKey` but neither targets nor describes changing this initializer. The package’s tests consequently do not compile after card 1.

2. Card 3 changes rendered gate strings but omits:
   - `internal/websterengine#TestRenderCardGates_OneBulletPerCardWithAStepSubBulletEach`, whose exact expected strings remain `--tier`, at `internal/websterengine/cardgate_test.go:64–83`;
   - `internal/websterengine#TestRenameBoundary_BatchesCheckAndGateTheirOwnTree`, at `internal/websterengine/cardgate_test.go:141–155`;
   - `internal/websterengine_test#TestBeginBatch_PromptFile`, at `internal/websterengine/beginbatch_test.go:558–562`;
   - `internal/websterengine_test#TestRecoverSpawnOrAttach`, at `internal/websterengine/recoverbatch_test.go:1710–1718`;
   - `internal/webstercli#TestRebaselineCmd_EditedCardOfFailedBatchThenRecover`, at `internal/webstercli/verbs_test.go:987–989`.

   These exercise actual Go-rendered marker values, not static stencil prose. Their old assertions fail when card 3 lands; subsequent retag cards change headers only.

3. Card 14 changes `Targets.Guards` to `map[string][]GuardTest`, but `internal/impactset#TestDerive_OverARepository` compares it through `reflect.DeepEqual` with `map[string][]string` at `internal/impactset/derive_integration_test.go:102` and `:162`. Card 11’s earlier edit adds a tag-import case and cannot update these assertions to a type card 14 has not created. Card 14 omits the test, leaving both assertions false.

4. `cmd/lyx#TestTreeSitterLink_OnlyAllowedTestBinariesLinkIt` launches `go list -test -deps -tags integration,tmux,llm` at `cmd/lyx/treesitterlink_integration_test.go:40–41`. Card 11 describes all package loads moving to both spellings but does not cover this load; card 24 changes its header only. The linkage guard loses visibility of retagged test imports, contrary to its all-tier contract and `PATTERN-test-speed`’s tree-sitter enforcement.

5. `internal/testkit/tmuxkit/allow_tmux.go:1–9` enables `tmuxAllowed` only under `tmux || llm`. The default remains false in `allow_none.go:7`, and `afterRun`/`checkSockets` fail remaining registered tmux sockets when it is false (`tmuxkit.go:105–110`, `:138–149`). Card 29 retags `_test.go` files only; no card updates this non-test tag consumer. A `tier2`-only or `tier3`-only binary therefore misclassifies legitimate tmux servers, and card 33’s follow-up removes the old command tags without naming this prerequisite.

These are one recurring enumeration defect rather than separate findings per missed reader. The strongest instances are the immediate compilation failure in card 1 and stale assertions in cards 3 and 14.

Tool-call manifest:

1. Searched non-test tag consumers and tools: the inherited reading lacked the complete runtime tag-consumer set.
2. Searched config/verify test fixtures and read tmuxkit teardown: the inherited reading lacked field-shape fixtures and the switch’s caller.
3. Read config defaults, darn fixture helpers and tmux socket checks: the inherited reading lacked the omitted initializer and its consequences.
4. Read remaining history consumers, rename predicates and status shape: the inherited reading lacked these one-hop readers.
5. Searched old rendered commands and read downstream assertions: the inherited reading lacked exact-string consumers outside direct renderer tests.
6. Read setting lookup, darn recipe/budget assembly and Command references: the inherited reading lacked these direct callers/callees.
7. Read concrete downstream prompt assertions and setting wiring: the inherited reading lacked their containing test members and lookup lifetimes.
8. Compared test-package directories with retag targets and searched guard-result assertions: the inherited reading lacked package enumeration and indirect result consumers.
9. Read tree-sitter-importing tests and `TestDerive_OverARepository` assertions: the inherited reading lacked evidence of excluded imports and the guard-result type comparisons.
