Guard reachability list:

- Card 1: unknown scope and forbidden `card` scope are reachable; the retired-list-shape refusal is intercepted upstream.
- Card 2: mutually exclusive flags, unknown keys, and missing card IDs are reachable through the named CLI tests; explicit targets deliberately replace configured scope.
- Cards 4–5: non-string `gate_key` refusals are reachable once the entries accept that key.
- Card 6: removal of the tree-scope command guards is licensed by D14; replacement calls use the configured gate key.
- Cards 7–8: dirty-tree, fingerprint, argument, module, and ancestry conditions are reachable; carried runs bypass slots as specified.
- Card 10: malformed `--since` is reachable before hub resolution.
- Card 12: scoped raw commands containing `tier2` or `tier3` reach the extended tag checks.
- Card 14: wrong-tier markers, undeclared guard tiers, and commands lacking placeholders are reachable.
- Card 15: zero-test runs, including an untagged `TestMain`, reach the new check; caller filters deliberately exempt runs, while generated guard filters do not.
- Card 17: skip chains, evaluation errors, pauses, resume, goto bypass, and relaunch paths are reachable. A two-row `OnDone` cycle reaches the chain limit; `Shed.validate` rejects only direct self-`OnDone` routes.
- Card 19: malformed checks, unknown settings/segments, and non-bool bare settings are reachable; nil seams are explicitly wiring guards.
- Cards 20–21: forced Open-Window and budget-zero Describe paths are reachable.
- Card 24: the transition guard runs, but accepts a command lacking the required tag.
- Card 33: its specified fixture cases reach the planned static scanner.

Findings:

1. **MEDIUM · consistency · `_lyx/plan/01-gate-key-shape.md:53` — The retired-list refusal cannot be reached through the prescribed load test.**  
   The card changes `decodeConfig` and prescribes loading `gates: {card: [0]}` through `TestLoadConfig`, expecting a reconcile instruction. Once the template declares `card` as a mapping, `configengine.Load` calls `yamlengine.FillMissing`, which rejects that sequence before `decodeConfig` runs. `LoadConfig` then wraps the upstream error with its existing `lyx config gate` instruction. The card does not change that upstream handling, so its reaching test cannot observe the promised refusal.  
   Evidence: `internal/gateslot/config.go:134–141`; `internal/configengine/config.go:234`; `internal/yamlengine/fill.go:112–116`; `internal/gateslot/config_test.go:143`.

2. **BLOCKING · decision · `_lyx/plan/24-retag-cmd-contracts-tools.md:31` — The only transition check accepts a different tag ending in the tier’s name.**  
   The expression `-tags[ =][^ ]*` followed by `tier` requires no boundary before the required tag. For example, tier0’s command `go test -tags nottier0 ./...` satisfies the expression even though it does not compile `tier0` files. With other tiers valid and no `covers`, the Verify line succeeds. D13 and the Constraints require this check to fail when a tier lacks its own tag, because the installed binary cannot detect the resulting vacuous pass.  
   Evidence: the Verify expression at `_lyx/plan/24-retag-cmd-contracts-tools.md:31`; D13 at `_lyx/discussion/decision-record.md:215`, and its transition constraint at line 357. `configengine.Provenance` preserves the resolved scalar through YAML annotation (`internal/configengine/layers.go:629`, `internal/yamlengine/layers.go:526–541`), so this command reaches the expression as written.

Tool-call manifest:

1. Located config loading, resolved-output rendering, and template-validation paths missing from the inherited manifest.
2. Read `configengine.Load`, template lookup, and resolved-output tests to trace upstream refusal ordering.
3. Read provenance, YAML fill/prune behavior, recipe shape checks, CLI flag construction, and scope fixtures to establish reachable entry paths.
4. Read `fillMapping`, annotation encoding, and `Shed.validate` to confirm the list-shape interception and skip-cycle reachability; located the pertinent plan lines.
5. Read `TestLoadConfig` and CLI refusal tests, and located exact loader and D13 evidence lines to confirm the findings’ test and contract grounding.
