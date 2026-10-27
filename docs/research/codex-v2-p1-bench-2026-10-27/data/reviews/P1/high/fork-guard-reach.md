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

Guard reachability:

| Guard or branch | Result |
|---|---|
| Card 1: invalid gate scope, `card` scope on another key, retired list shape | Reachable through a hub configuration; named `TestLoadConfig` cases cover them. |
| Card 2: incompatible `--gate` flags | Reachable before hub resolution; named CLI refusal cases cover them. |
| Card 2: unknown key and missing cards for card scope | Reachable after config loading; named scope cases cover them. |
| Cards 4–5: non-string row `gate_key` | Reachable through row config; named resolver/entry cases cover them. |
| Card 7: clean-tree carry and dirty/changed-tree rejection | Reachable; the proposed scenario cases distinguish carry, changed command, changed tree and dirty state. |
| Card 10: malformed `--since` | Reachable before hub resolution; named CLI case covers it. |
| Card 12: tier2/tier3 raw-test guards | Reachable through the named static command strings. |
| Card 14: wrong-tier guard marker and invalid guard-tier configuration | Reachable; existing `TestPlanTierRuns` already has both missing-placeholder cases. |
| Card 15: empty tier and filtered-run exception | Reachable. The card explicitly excludes guard runs from the exception; its `tierRun` argument supplies the existing `guardStep` distinction. |
| Card 17: false gate, evaluation error, pause conditions, resume and goto bypass | Reachable. Multi-row done cycles can reach the skip-chain bound; direct self-`OnDone` is rejected upstream, but the card does not require that input. |
| Card 19: malformed check, unknown setting/segment and non-bool setting | Reachable. `SettingLookup` returns YAML-decoded values without typed module decoding, so `no` and `1` can reach evaluation. Nil seams are explicitly wiring guards. |
| Card 20: retired switch config and forced Open-Window | Reachable through Command config and the hidden flag’s told lookup. |
| Card 24: pre-retag hub-command check | **Bypassable for valid configuration serialization; finding below.** |
| Card 33: missing/multiple tier tags and unmarked transitional term | Reachable through the specified fixture sources. |

**Finding — MEDIUM, design**

- **Location:** `_lyx/plan/24-retag-cmd-contracts-tools.md:31`
- **Summary:** The first-retag Verify can accept a tier command with no tier tag because it only validates standalone, exactly indented `command:` lines.
- **Evidence:** A valid hub tier written as a flow mapping, such as `tier0: {command: "go test -json {targets} {args}", all_targets: ./..., timeout_min: 60, counter: go-json}`, sets `seen` through the tier-heading pattern but never matches `/^        command:/`; with no `covers`, the script exits successfully. The command consequently lacks `-tags tier0`, although the card’s Intent at lines 28–29 promises this input fails. `printResolved` delegates to provenance and writes its YAML verbatim (`internal/configcli/configcli.go:70–80`). `tiers` is an open map (`internal/gateslot/config.go:59–62`); its nodes are copied intact by the layer merge (`internal/yamlengine/layers.go:73–86`) and annotation re-marshals the nodes without clearing their styles (`internal/yamlengine/layers.go:526–541`). No upstream typed-command check requires a tier tag (`internal/gateslot/tiers.go:120–124`).
- The same Verify also accepts `-tags nottier0` for tier0: `[^ ]*` before the tier name supplies no left tag-token boundary.
- **Required correction:** Validate every tier’s actual command and exact tag membership, rejecting tiers whose command was never inspected. This is D13’s only installed-binary protection against vacuous retag gates. I have not proposed an executable replacement or run an overlay, because this fork is read-only and may run only reading commands.

Tool-call manifest:

1. Read gate CLI parsing, flag-shape checks, config decoding and config-output references; inherited context lacked the full CLI entry path.
2. Read resolved-output tests, guard scanning, Goto and Relaunch; inherited context lacked complete guard and bypass admission paths.
3. Read resolved rendering, setting lookup, darn recipe construction and numbered target passages; inherited context lacked the concrete output/setting paths and line references.
4. Read existing guard-placeholder refusal cases, YAML annotation/merge, batten reopen wiring and setting tests; checked upstream reachability and whether the new guards repeat existing coverage.
5. Read numbered guard-argument scenario, config open-map declaration, gate template and flag checks; established concrete accepted inputs and exact evidence locations.
6. Read YAML set-style handling, numbered card/decision passages and shed-test inventory; checked whether flow style is forcibly normalized and whether test inputs survive upstream checks.
7. Read shed validation/cycle references and YAML merge/annotation locations; checked skip-chain reachability and finalized serialization evidence.
