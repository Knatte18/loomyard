Recovery-path list:

- Card 1, invalid scope or forbidden `card` scope: edit the gate configuration from the prime with the role hand-off — progresses.
- Card 1, retired list shape: `lyx fabric reconcile` from the prime — progresses; the offered `lyx config reconcile` alternative does not migrate the hub file as written.
- Card 2, conflicting flags: drop the incompatible flag — progresses.
- Card 2, missing card IDs: supply `--card <id>` — progresses.
- Card 2, unknown gate key: repair the hub gate configuration — progresses.
- Cards 4–5, non-string `gate_key`: supply a string or remove the override — progresses.
- Card 7, unreadable record, lock or cleanliness failure: run without carry; failed writes leave the test result intact — progresses.
- Card 8, no eligible green ancestor: use the parent merge base, then whole scope — progresses.
- Card 10, malformed date: use `YYYY-MM-DD` — progresses.
- Card 14, wrong guard tier: move the guard to tier0 or tier1 — progresses.
- Card 14, missing tier or command placeholders: repair `gate.yaml` — progresses.
- Card 15, absent tier tag: repair the tier command through `lyx config gate` — progresses.
- Card 17, evaluation error: leave the row unentered and status failed, then resume after correction — progresses subject to card 19’s recovery-message defect.
- Card 17, excessive skip chain: correct settings or use goto, then resume — progresses.
- Card 19, malformed check or unknown operand: correct the check/module/key/segment — progresses.
- Card 19, nil lookup seam: fail-loud wiring guard — exempt from ordinary recovery requirements.
- Card 19, non-bool bare setting: the prescribed config-only repair is not sufficient for valid numeric settings, and its default-layer command selects the wrong layer.
- Card 20, leftover `switch`: move it to `entry_gate` — progresses.
- Card 24, missing tier tags or retained `covers`: orch repairs hub config from the prime and reruns the card — progresses.
- Card 33, invalid test constraint: give the file its tier tag — progresses.

Findings:

1. **BLOCKING · consistency · `_lyx/plan/19-recipe-entry-gate.md:43` — The non-bool refusal prescribes recovery that cannot fix all admitted inputs.**  
   A recipe containing bare `loom.review_max_bounces` passes the planned known-key check but reads the legitimate integer `3` and fails evaluation.
   Editing that value to a bool is not a valid repair: `internal/loomengine/config.go:470` declares an integer, and `LoadConfig` unmarshals into that typed configuration at line 1227.
   The remedy must include correcting the recipe to a comparison or using the appropriate budget operand.
   Separately, the same refusal tells users to run `lyx config <module>` from the prime for a default-layer value, but `internal/configcli/hubwide.go:102–109` selects the worktree layer for per-worktree modules unless `--layer default` is given; batten and loom are per-worktree registrations at `internal/configreg/configreg.go:120,129`.
   This breaks `PATTERN-refusal-way-forward`: following the named recovery can leave the offending state unchanged or replace it with an upstream load failure.
   Fix the refusal prose to distinguish recipe misuse from invalid config values and name the actual layer-selecting command.

2. **MEDIUM · consistency · `_lyx/plan/01-gate-key-shape.md:53` — The optional `lyx config reconcile` recovery does not migrate the hub’s gate file.**  
   The card offers bare `lyx config reconcile` as equivalent to fabric reconcile.
   `internal/configcli/configcli.go:527–561` shows that no-layer reconcile addresses the worktree’s own/local files; the hub branch requires `--layer default`.
   The command also defaults to dry-run (`internal/configcli/configcli.go:710–734`).
   The primary `lyx fabric reconcile` recovery is valid (`internal/fabriccli/fabric.go:290–299`), but the alternative as prescribed makes no repair.
   Either retain only fabric reconcile or specify `lyx config reconcile --layer default --apply` with the existing prime/operator hand-off.

Tool-call manifest:

1. Read gate-tier recovery references, setting lookup references, shed goto references and refusal PATTERN backgrounds; these recovery implementations were absent from the inherited manifest.
2. Read `TierList`, `GateCall`, `SettingLookup` and the shed goto verb; checked the destinations and admission semantics of the proposed recovery commands.
3. Read config resolved/reconcile handlers, hub-layer selection, lookup wiring and the refusal contract; checked whether diagnostics identify the offending layer and whether repair commands reach it.
4. Read numbered card recovery clauses, batten/loom setting wiring and fabric reconcile help; established precise locations and the available repair route.
5. Read numbered layer-selection and reconcile implementations, module registrations and existing gate role clauses; checked per-worktree versus hub configuration recovery.
6. Read the loom numeric configuration declaration, decode site and numbered entry-gate clauses; checked whether a valid numeric operand can be repaired by the prescribed bool-setting action.
