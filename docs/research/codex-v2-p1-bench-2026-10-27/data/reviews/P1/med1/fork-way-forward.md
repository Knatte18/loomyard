I’ll read the review instructions in order, check the target against the repository and its rules, and write only the review file.

I’ve read the 33 cards, the decision record, and the relevant gate and recipe rules. I’m checking the existing callers and test fixtures to trace the effects of the proposed changes.

Reading manifest:
- Plan and answer key: `_lyx/plan/00-overview.md`, cards 1–33, and `_lyx/discussion/decision-record.md`.
- Review rules: step 1, focus directive, and step 2.
- Repository rules: `PATTERN.md` and backgrounds for test tiers, speed, economy, recipe settings, tolerant records, gate slots, fabric git, verified trees, and CLI wiring.
- Gate execution: `internal/gatecli/run.go`, `scope.go`, `tiers.go`, and `tierrun.go`; flag validation, scope resolution, tier planning, slot acquisition, guard execution, and reporting.
- Gate callers: search results for `GateCall`, `GuardTier`, and `TreeScopeVerify` in gate, loom, Webster, and landing packages.

Refusal/recovery inventory:

- Card 1, invalid scope or gate declaration → repair `gate.yaml` from the prime: progresses.
- Card 1, retired list-shaped gate → `lyx fabric reconcile`: progresses; the offered alternative `lyx config reconcile` does not migrate the hub file as written.
- Card 2, incompatible flags or missing cards → remove conflicting flags or supply card IDs: progresses.
- Cards 4–5, non-string `gate_key` → set a string or remove the override: progresses.
- Card 7, unreadable green record or recording failure → run tiers normally: progresses.
- Card 10, malformed date → use `YYYY-MM-DD`: progresses.
- Card 14, guard in the wrong tier → move the test; undeclared or unsuitable guard tier → repair gate config: progresses.
- Card 15, missing tier tag → repair the tier command through `lyx config gate`: progresses.
- Card 17, entry evaluation failure → fix the value and resume; cyclic skip chain → fix settings or goto an admitted row and resume: progresses, provided the value is fixed in the effective layer.
- Card 19, malformed check, unknown operand, or missing wiring → correct the recipe/check/wiring: progresses.
- Card 19, non-boolean setting → the prescribed repair can edit an ineffective source and leave the resumed row failing.
- Card 20, retired `switch` → move it to `entry_gate`: progresses.
- Card 24, unsuitable hub tier commands or retained `covers` → orch changes hub config and reruns the card: progresses.
- Card 33, invalid test tags → assign the appropriate tag: progresses.

Findings:

1. **MEDIUM · consistency · `_lyx/plan/19-recipe-entry-gate.md:43` — The non-boolean-setting recovery edits sources that do not change the stopped run’s effective value.**

   Card 19 tells the caller to correct task `run_settings` and resume, but Seed-Child copies those settings into local layer files; an entry-gate failure occurs later, and resuming that row does not repeat Seed-Child.
   `internal/battenshed/seamchild.go:144–164` reads and writes the settings only during `writeRunSettings`, while `internal/battencli/wire.go:1351–1356` reads the task’s existing config stack.
   The plan also prescribes bare `lyx config <module>` from the prime for a default-layer value, but `internal/configcli/hubwide.go:102–111` selects the prime’s **worktree** layer for a per-worktree module.

   **Proposed fix:** Change card 19’s Intent to prescribe `lyx config <module> --layer local` in the task worktree for a local value, plus correction of the board’s `run_settings` for future seeding; prescribe `--layer default` from the prime for a board-default value, and the task worktree’s worktree layer when that is the source.
   Extend the planned non-boolean refusal assertions to name these effective-layer repairs.
   This changes Intent only, not targets or Verify.

2. **MEDIUM · consistency · `_lyx/plan/01-gate-key-shape.md:53` — The offered config-reconcile alternative cannot repair the hub gate file.**

   The card permits the new refusal to recommend `lyx config reconcile` from the prime as an alternative to fabric reconcile.
   `internal/configcli/configcli.go:525–565` shows that without `--layer default` it reconciles worktree/local files, and without `--apply` it writes nothing.
   Thus following this offered recovery leaves the old hub gate list and the refusal unchanged.

   **Proposed fix:** Replace the alternative in card 1’s Intent with `lyx config reconcile --layer default --apply` from the prime, or prescribe only `lyx fabric reconcile`; have the planned retired-shape refusal test assert the chosen actionable command.
   This changes Intent only, not targets or Verify.

Tool-call manifest:

1. Located config reconciliation and shed goto/relaunch paths; read refusal-way-forward background, absent from inherited reading.
2. Read config resolution/reconcile dispatch and goto admission; inherited context did not establish whether recovery commands can run from failed status.
3. Read config decoding, existing gate refusals, and hub-edit validation references; inherited context lacked these recovery dependencies.
4. Searched run-settings propagation and read hub config edit/set handling; needed to establish whether invalid values can be repaired.
5. Read layer selection geometry, setting lookup, and run-profile lifecycle; needed to trace effective setting sources.
6. Located Seed-Child run-settings writes and inspected exact card recovery wording and setting wiring; needed evidence for stopped-run recovery.
7. Read selected-layer behavior, Seed-Child writes, and batten lookup implementation; confirmed which repair changes the value read on resume.
