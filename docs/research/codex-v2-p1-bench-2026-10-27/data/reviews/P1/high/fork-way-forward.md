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

Per-refusal recovery list:

- Card 1, invalid scope or missing gate/tier: edit `gate.yaml` from prime; progresses.
- Card 1, retired list shape: `lyx fabric reconcile` progresses; the offered bare `lyx config reconcile` alternative does not migrate the hub file.
- Card 2, conflicting flags: drop the conflicting flag; progresses.
- Card 2, card scope without IDs: supply `--card`; progresses.
- Cards 4–5, non-string `gate_key`: replace it with a string or remove it; progresses.
- Card 6, retired tree-scope refusals: migration and editor changes remove the obsolete restrictions; progresses.
- Card 7, green-record failures: warn and run normally; progresses.
- Card 10, malformed date: supply `YYYY-MM-DD`; progresses.
- Card 12, prohibited tier tags: retain the existing gate-command steer; progresses.
- Card 14, misplaced guard or invalid guard-tier configuration: move the guard to an admitted tier or repair the tier configuration; progresses.
- Card 15, zero-test tier: repair the tier command through `lyx config gate`; progresses.
- Card 17, skip cycle: correct settings or goto an admitted row, then resume; progresses.
- Card 17, evaluation failure: status becomes failed and entry remains unevaluated, permitting reevaluation on resume.
- Card 19, malformed checks/unknown operands: correct the recipe operand or check form; progresses.
- Card 19, nil wiring seams: fail-loud wiring guard; exempt from ordinary recovery requirements.
- Card 19, non-bool setting: the prescribed default-layer and `run_settings` repair routes do not necessarily change the value the halted run reads.
- Card 20, obsolete `switch` key: move it to `entry_gate`; progresses.
- Card 24, missing tier tags or remaining `covers`: orch changes hub config and reruns the card; progresses.
- Card 33, one-tag scan failure: retag the named test file; ordinary test failure, not a refusal.

Findings:

1. **MEDIUM — consistency — `_lyx/plan/19-recipe-entry-gate.md:43` — The non-bool refusal directs repairs to layers that do not update the halted run.**  
   The card prescribes editing a default value with bare `lyx config <module>` from prime, or changing the task’s `run_settings`, then resuming. For per-worktree modules such as batten, bare config selects prime’s worktree layer rather than the board default (`internal/configcli/hubwide.go:106–116`). Run settings are copied into the task’s local layer by Seed-Child (`internal/battenshed/seamchild.go:139–166`); changing the board entry afterward does not rewrite that layer. `prepareRunSettings` explicitly reports that the change applies to the next Seed-Child and directs running pairs to `lyx config <module> --layer local --set` (`internal/boardcli/runsettings.go:74–79`). Batten’s entry lookup reads the task’s files, not the board entry (`internal/battencli/wire.go:1350–1356`), so resuming after the prescribed repair can encounter the same invalid value.  
   Proposed correction: prescribe the actual provenance layer—`--layer default` from prime for a per-worktree module’s default, and `--layer local` in the task pair for a seeded run setting—then resume. Updating the board setting can additionally preserve the correction for subsequent runs.

2. **LOW — consistency — `_lyx/plan/01-gate-key-shape.md:53` — The alternative reconcile recovery omits the flags that migrate the hub’s file.**  
   The card permits the retired-list-shape refusal to name bare `lyx config reconcile` from prime. That command defaults to a dry run and, without `--layer default`, operates on worktree/local configuration rather than the hub’s board configuration (`internal/configcli/configcli.go:525–565`, `729–730`). It leaves the refused `gate.yaml` unchanged. The other offered route, `lyx fabric reconcile`, does apply hub reconciliation (`internal/fabriccli/fabric.go:720`).  
   Proposed correction: retain `lyx fabric reconcile` alone, or spell the alternative `lyx config reconcile --layer default --apply`.

Tool-call manifest:

1. Read refusal contract, setting lookup, goto implementation and resume references; inherited context lacked recovery requirements and admission paths.
2. Read setting wiring, window reopening, config migration registration and denied-recovery rules; inherited context lacked where repaired values are read.
3. Read resolved-config provenance and resume flow; inherited context lacked provenance output and resumption behavior.
4. Searched run-setting writers/readers and read batten’s lookup; inherited context lacked whether board changes update an existing pair.
5. Read run-setting validation, Seed-Child writes, existing board tests and plan refusal locations; confirmed the repair behavior and finding locations.
6. Read config editor rollback/validation and reconcile defaults; inherited context lacked whether the proposed configuration commands can repair the refused state.
7. Read hub reconcile’s apply path, config reconcile routing and setting validation; confirmed the alternate reconcile command and scalar-setting reachability.
