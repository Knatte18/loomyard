I’ll read the review instructions in order, check the target against the repository and its rules, and write only the review file.

I’ve read the 33 cards, the decision record, and the relevant gate and recipe rules. I’m checking the existing callers and test fixtures to trace the effects of the proposed changes.

Reading manifest:
- Plan and answer key: `_lyx/plan/00-overview.md`, cards 1–33, and `_lyx/discussion/decision-record.md`.
- Review rules: step 1, focus directive, and step 2.
- Repository rules: `PATTERN.md` and backgrounds for test tiers, speed, economy, recipe settings, tolerant records, gate slots, fabric git, verified trees, and CLI wiring.
- Gate execution: `internal/gatecli/run.go`, `scope.go`, `tiers.go`, and `tierrun.go`; flag validation, scope resolution, tier planning, slot acquisition, guard execution, and reporting.
- Gate callers: search results for `GateCall`, `GuardTier`, and `TreeScopeVerify` in gate, loom, Webster, and landing packages.

Guard reachability inventory:

- Card 1: invalid scope and forbidden `card` scope reach `decodeConfig`; the retired-list refusal does **not** reach it because config loading rejects the changed shape first.
- Card 2: flag conflicts reach pre-hub validation; unknown keys and missing card IDs reach config/scope validation.
- Cards 4–5: malformed `gate_key` can reach the recipe constructors/resolver.
- Card 7: clean/dirty, cache miss, changed tree, argument mismatch, and slot-free carry branches are reachable through manual gate runs.
- Card 10: malformed `--since` can reach parsing before hub resolution.
- Card 12: static tier-tag refusals are reachable.
- Card 14: wrongly tiered guard files and undeclared/malformed guard-tier configurations are reachable.
- Card 15: zero-test packages, including an untagged `TestMain`, reach the post-run check; explicit filters reach the prescribed exemption.
- Card 17: false/error gates, skip cycles, pause boundaries, resume, goto bypass, and targeted relaunch are reachable.
- Card 19: malformed checks reach parsing; unknown operands and missing wiring reach Build; non-bool values reach evaluation.
- Card 20: retired `switch` reaches unknown-key validation; forced Open-Window reaches the setting override.
- Card 24: the verification guard is bypassable by valid flow-style tier mappings and by tag names merely ending in the expected tier name.
- Card 33: the stated malformed tag fixtures reach the new scan.

Findings:

1. **MEDIUM — consistency — `_lyx/plan/01-gate-key-shape.md:53`**
   **The prescribed retired-list refusal is intercepted by config loading.**
   Card 1 places the new decoding/shape behavior in `decodeConfig`, but `gateslot.LoadConfig` calls `configengine.Load` first (`internal/gateslot/config.go:134–139`).
   That loader calls `yamlengine.FillMissing` (`internal/configengine/config.go:234`), whose mapping-shape check rejects a sequence where the new template has a mapping (`internal/yamlengine/fill.go:112–115`).
   Consequently the named `card: [0]` input never reaches the planned decoding refusal; the existing `withConfigWayForward` instead directs the caller to `lyx config gate`.
   The plan needs to handle this upstream failure so its named `TestLoadConfig` row can reach the promised reconcile guidance.

2. **MEDIUM — design — `_lyx/plan/24-retag-cmd-contracts-tools.md:31`**
   **The only transition check can accept tier commands missing their required tag.**
   The AWK program sets `seen` on a tier header but checks commands only on separate eight-space-indented `command:` lines.
   A valid flow-style tier entry such as `tier0: {command: "go test -json {targets} {args}", ...}` sets `seen` and never sets `bad`.
   The YAML pipeline preserves node styles: open-map contents are carried whole in `internal/yamlengine/layers.go:69–86`, and `AnnotateLeaves` marshals those nodes (`internal/yamlengine/layers.go:526–542`).
   This form is supported and explicitly illustrated in `internal/gateslot/template.yaml:5`.
   Independently, `[^ ]*` before the tier name lets `-tags nottier0` satisfy the check for `tier0`.
   D13 identifies this Verify line as the only protection while the installed binary lacks the new check, so these inputs leave retagged tests unexecuted without refusing the card.
   The command needs validation of each tier’s actual command and exact tag tokens, with rejection when no command was checked for a tier.

Tool-call manifest:

1. Searched scope-test and config definitions; the scope-test filename was absent from inherited context.
2. Located actual scope tests, config decoding/template, and resolved-output paths to trace preconditions and rendering.
3. Read scope fixture entry points and recipe parse/build paths to establish reaching inputs.
4. Read counter events and found shed pause/persistence sites to trace zero-test and entry-gate branches.
5. Read provenance, shed step/relaunch, and gate config loading to identify upstream checks.
6. Located YAML annotation/shape checks and gate resolvers to trace rendering and recipe admission.
7. Read YAML annotation, retired-key handling, and gate resolver entry checks.
8. Read YAML merge behavior and scope-planning tests to check style preservation and guard skipping.
9. Read config loading, setting lookup, CLI flag assembly, and Card 24 line locations.
10. Read template fill checks and setting-lookup wiring to confirm upstream interception.
11. Read YAML resolution and precise evidence lines to confirm style preservation and finding locations.
