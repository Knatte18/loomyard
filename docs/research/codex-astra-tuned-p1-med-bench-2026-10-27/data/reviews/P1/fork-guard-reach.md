Reachability list:

- Card 1, invalid scope and `card` scope on another key: reachable through config loading once the new shape decodes.
- Card 1, retired `card: [0]` refusal in `decodeConfig`: **not reachable as prescribed**; template filling rejects the sequence before `decodeConfig`.
- Card 2, incompatible flags: reachable before hub resolution.
- Card 2, unknown key and missing card IDs: reachable after loading the configuration.
- Cards 4–5, non-string `gate_key`: reachable in row construction; the cards also update the applicable allowed-key lists.
- Card 7, clean-tree carry and dirty/tree-changed rejection: reachable; the planned slot-holding scenario exercises the carry branch.
- Card 10, malformed `--since`: reachable before hub resolution.
- Card 12, tier2/tier3 static guards: reachable through the named parser and shell-guard inputs.
- Card 14, wrong-tier marker and undeclared/malformed guard tier: reachable during scanning and planning.
- Card 15, missing-tag failure with untagged `TestMain`: reachable through package-level events with no test events.
- Card 15, guard-step missing-tag failure: reachable with empty caller arguments, but **bypassable when caller arguments contain a filtering flag**.
- Card 17, entry-gate skips, errors, pause conditions, re-entry and goto bypass: reachable through the described engine transitions.
- Card 19, malformed checks, unknown operands, nil seams and non-bool values: reachable through parsing, building and evaluation respectively.
- Card 20, retired `switch` key: reachable through Command-row unknown-key checking.
- Card 24, missing tier tags, retained `covers`, and empty resolved output: reachable through its verification pipeline.
- Card 33, untagged tests, multiple-tier constraints and unmarked transitional constraints: reachable through its fixture scan.

Findings:

1. **BLOCKING — consistency — `_lyx/plan/01-gate-key-shape.md`, retired-list refusal paragraph.**
   **The prescribed retired-list refusal is behind an upstream shape rejection.**
   Card 1 requires `decodeConfig` to recognize `card: [0]` and tell the caller to reconcile rather than edit gate configuration.
   `gateslot.LoadConfig` calls `configengine.Load` before `decodeConfig` (`internal/gateslot/config.go:134–143`).
   The loader calls `yamlengine.FillMissing` with the new template (`internal/configengine/config.go:234`); its `fillMapping` rejects an existing sequence wherever the template now holds a mapping (`internal/yamlengine/fill.go:112–115`).
   Thus the named `TestLoadConfig` input never reaches the prescribed branch and instead receives `withConfigWayForward`’s ordinary `lyx config gate` advice (`internal/gateslot/config.go:160–174`).
   The plan needs to place or propagate the reconcile-specific refusal at the earlier boundary, while preserving configengine’s sole ownership of layer reads.

2. **MEDIUM — consistency — `_lyx/plan/15-empty-tier-fails.md:27–32`.**
   **Using caller arguments alone to decide the exemption still lets a filtered caller bypass the guard-step check.**
   The card says guard runs are never exempt but prescribes passing `testRequest.args` to the exemption logic.
   A real narrowed/manual call with `-- -run X` reaches the guard step with that flag still present.
   `runGuardStep` appends its own guard-name filter to a clone of those same caller arguments and passes the original request to `runTier` (`internal/gatecli/tierrun.go:165–174`).
   Under the planned rule, the caller’s `-run` therefore exempts this guard run even when its command omitted the tier tag.
   The planned test covers only empty caller arguments.
   Make the exemption explicitly conditional on the run not being a guard step, and extend the planned fixture row with a filtering caller argument.

Tool-call manifest:

1. Located gate decoding, resolved-config output and counter/planner definitions missing from the inherited manifest.
2. Read gate loading, tier planning, counters, tier execution and the gate template to trace refusal and post-run-check entry paths.
3. Located config shape validation, recipe construction and scenario slot helpers beyond the inherited reads.
4. Read recipe parsing/building, setting lookup, gate-entry resolution, per-name merging and relevant planned guard-check lines.
5. Read lookup tests and wiring references to check whether non-bool operands and changed settings can reach entry evaluation; inspected scenario slot acquisition.
6. Read the config fill path and its shape rejection, plus exact guard-step and lookup wiring lines, to settle upstream reachability and caller-filter propagation.
