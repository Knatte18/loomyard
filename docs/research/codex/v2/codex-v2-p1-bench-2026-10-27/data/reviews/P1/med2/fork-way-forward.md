I’ll read the instructions in order, check the target against the fasit and repository code, and write only the review file.

I’ve read all 33 cards and the decision record. I’m now tracing the gate and entry-gate call paths, including config migration, resume behavior, history readers, and tests.

Reading manifest:
- Plan and criteria: `_lyx/plan/00-overview.md`, cards 1–33, `_lyx/discussion/decision-record.md`, `.bench/instr/focus.md`, and `.bench/stencils/loom/loom-template-plan.md`.
- Repository rules: `PATTERN.md` and the testing, recipe lookup, verified-tree, tolerant-records, failure visibility, gate-slot, config ownership, parser, CLI, and refusal backgrounds.
- Gate execution: `internal/gatecli/run.go`, `scope.go`, `tiers.go`, and `counters.go`; `internal/gateslot/tiers.go` and `runlog.go`.
- Gate callers: `internal/webstercli/beginbatch.go`, `internal/loomcli/roundgate.go`, and `darnwiring.go`; references to gate lists, commands, guard tiers, and tree-scope settings.
- Shed state: `internal/shedengine/run.go` entry, routing, budget, and persistence functions; history-reader references across shed, loom, reports, batten, and the driver.

Refusal/recovery list:

- Card 1, invalid scope or gate key/list: edit `gate.yaml` from the prime; progresses.
- Card 1, retired list shape: `lyx fabric reconcile` progresses; the offered bare `lyx config reconcile` alternative does not migrate the hub file.
- Card 2, incompatible flags: remove the conflicting flag; progresses.
- Card 2, missing cards: supply `--card`; progresses.
- Cards 4–5, non-string `gate_key`: correct/remove the row setting; progresses.
- Card 10, malformed date: use `YYYY-MM-DD`; progresses.
- Card 12, barred raw test tags: use the existing gate route; progresses.
- Card 14, wrong guard tier: move the marked test to tier0/tier1; progresses.
- Card 14, undeclared/malformed guard tier command: repair the hub gate configuration; progresses.
- Card 15, empty tagged tier: repair the command’s tag through `lyx config gate`; progresses.
- Card 17, skip cycle: repair settings or goto an admitted row and resume; progresses.
- Cards 17/19, entry evaluation error: retry evaluates again, but card 19’s prescribed config repairs do not reliably change the value the resumed run reads.
- Card 19, malformed check, unknown setting/segment, missing wiring: correct the recipe or wiring; progresses.
- Card 20, leftover `switch`: move it to `entry_gate`; progresses.
- Card 24, missing tier tag/remaining covers: orch applies the hub command changes, then reruns the card; progresses.

Findings:

1. **MEDIUM / consistency — `_lyx/plan/19-recipe-entry-gate.md:43` — The non-bool refusal directs repairs to sources the resumed run does not read.**  
   For a value seeded from `run_settings`, editing the board entry does not update the running pair’s local layer: `internal/boardcli/runsettings.go:77–79` explicitly says the change applies at the next Seed-Child and prescribes `lyx config <module> --layer local --set` in the pair.  
   Seed-Child writes those files in `internal/battenshed/seamchild.go:144–163`, and the entry-gated Command rows follow it (`contracts/recipes/batten-recipe.yaml:44–64`), so resuming at Open-Window/Seed-IDE does not reseed them.  
   The same sentence’s `lyx config <module>` from the prime does not edit a per-worktree module’s board default: `selectedLayer` selects worktree without `--layer` (`internal/configcli/hubwide.go:102–109`); batten is per-worktree (`internal/battenshed/config.go:49–55`).  
   **Proposed fix:** In card 19’s Intent, prescribe fixing the provenance-selected layer explicitly: `--layer local` in the running pair for a seeded run-settings override, `--layer default` from the prime for a board default, and the appropriate worktree file for a worktree override, then resume. Updating board `run_settings` may also preserve the correction for future runs, but is not the live repair. Extend the already planned refusal coverage to check this actionable recovery text.

2. **LOW / consistency — `_lyx/plan/01-gate-key-shape.md:53` — The alternative reconcile command cannot perform the promised migration.**  
   Bare `lyx config reconcile` defaults to dry-run (`internal/configcli/configcli.go:729`) and addresses worktree/local files; hub-wide files are only reported there (`internal/configsync/configsync.go:405–414`).  
   The command’s own help requires `--layer default --apply` to write the hub files (`internal/configcli/configcli.go:722–726`).  
   **Proposed fix:** Remove the parenthetical alternative and retain `lyx fabric reconcile`, or spell the alternative as `lyx config reconcile --layer default --apply` from the prime, including the existing role clause.

Tool-call reading log:

1. Read shed CLI arming, generic goto, and review format; inherited context lacked the recovery verb’s entry point and restrictions.
2. Searched config provenance/migration and arming paths; inherited context lacked how proposed config remedies select files.
3. Read `SettingLookup`, provenance references, and goto admission; inherited context lacked setting snapshots and live read behavior.
4. Read config dispatch, layer selection references, wiring, and migration references; inherited context lacked whether editor commands reach the refused value.
5. Read config geometry, batten setting wiring, and reconcile dispatch; inherited context lacked where task run settings become readable config.
6. Read Seed-Child references and loom resume; inherited context lacked whether resume reapplies board settings.
7. Read board run-settings handling, Seed-Child writes, and card 19 line locations; established the live-versus-next-run recovery distinction.
8. Read exact layer selection, board notice, Seed-Child, lookup, and card 1 locations; pinned evidence and locations for the findings.
9. Read reconciliation selection/help and batten recipe/config declarations; established the incorrect alternative’s behavior and the per-worktree default-layer case.
