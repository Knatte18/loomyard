I’ll read the instructions in order, check the target against the fasit and repository code, and write only the review file.

I’ve read all 33 cards and the decision record. I’m now tracing the gate and entry-gate call paths, including config migration, resume behavior, history readers, and tests.

Reading manifest:
- Plan and criteria: `_lyx/plan/00-overview.md`, cards 1–33, `_lyx/discussion/decision-record.md`, `.bench/instr/focus.md`, and `.bench/stencils/loom/loom-template-plan.md`.
- Repository rules: `PATTERN.md` and the testing, recipe lookup, verified-tree, tolerant-records, failure visibility, gate-slot, config ownership, parser, CLI, and refusal backgrounds.
- Gate execution: `internal/gatecli/run.go`, `scope.go`, `tiers.go`, and `counters.go`; `internal/gateslot/tiers.go` and `runlog.go`.
- Gate callers: `internal/webstercli/beginbatch.go`, `internal/loomcli/roundgate.go`, and `darnwiring.go`; references to gate lists, commands, guard tiers, and tree-scope settings.
- Shed state: `internal/shedengine/run.go` entry, routing, budget, and persistence functions; history-reader references across shed, loom, reports, batten, and the driver.

Guard reachability list:

- Card 1: unknown scope and forbidden `card` scope are reachable; the prescribed legacy-list refusal is intercepted by `configengine.Load`.
- Card 2: incompatible flags are reachable before hub resolution; unknown keys and missing `--card` are reachable after config load.
- Cards 4–5: non-string `gate_key` is reachable through the row-config resolvers.
- Card 7: clean/dirty, changed-tree, changed-fingerprint, exact-args and malformed-record branches are reachable.
- Card 10: malformed `--since` is reachable before hub resolution; absent record directories are reachable.
- Card 12: the new static tag matches are reachable through the existing guards.
- Card 14: wrong-tier markers and undeclared/malformed guard-tier commands are reachable; guards inside another tier’s scope can escape scheduling.
- Card 15: missing-tag zero-test failures are reachable; an original caller filter can still exempt a guard run.
- Card 17: skip, resume, routed re-entry, explicit-goto bypass, pause and evaluation-error paths are reachable; a two-row skip cycle reaches the chain bound without violating shedengine’s self-edge check.
- Card 19: malformed checks, unknown operands/segments and non-bool bare settings are reachable; nil seams are explicitly wiring guards.
- Card 20: retired `switch` rejection and forced Open-Window evaluation are reachable.
- Card 24: its tag check can accept a command lacking the actual tier tag.
- Card 33: untagged, multi-tier and unauthorized transitional-tag fixtures reach the planned parser checks.

Findings:

1. **BLOCKING — scope — `_lyx/plan/01-gate-key-shape.md:53` — The named legacy-list input cannot reach the prescribed reconcile refusal through the planned decoder changes.**  
   Card 1 changes the template’s `gates.card` from a sequence to a mapping and prescribes a `TestLoadConfig` row with `card: [0]` that must recommend reconcile, never `lyx config gate`.
   `gateslot.LoadConfig` calls `configengine.Load` before `decodeConfig` (`internal/gateslot/config.go:134–141`).
   That load calls `yamlengine.FillMissing` (`internal/configengine/config.go:234`), which rejects a sequence where the template now has a mapping (`internal/yamlengine/fill.go:112–117`).
   The failure returns through unchanged `withConfigWayForward`, whose recommendation is `lyx config gate`.
   Neither the upstream load/refusal path nor its wrapper is covered by the card’s targets.
   The planned test therefore cannot observe its required recovery message through the stated implementation.

2. **BLOCKING — scope — `_lyx/plan/14-guard-step-per-tier.md:58` — Guard exclusion remains package-wide instead of relative to each guard’s own tier.**  
   The card prescribes grouping guards “outside the scope” by tier, but D6 requires guards outside **their own tier’s** scope.
   Current `targetsFor` removes every guard in an impacted package before gate planning (`internal/impactset/targets.go:126–130`).
   Current `scanGuards` likewise excludes every package in `sc.modules` irrespective of tier (`internal/gatecli/scope.go:138–153`, inherited reading).
   Consider a `card` gate listing tier0 over a package containing a tier1 guard: tier0 does not compile that guard, yet the package-wide exclusion removes it from the guard step.
   Card 14 changes the payload to `GuardTest` but never prescribes replacing this exclusion or testing this case; its two-group tests concern guards already outside the scope.
   The same defect becomes consequential with card 8’s distinct per-tier diff bases.

3. **MEDIUM — consistency — `_lyx/plan/15-empty-tier-fails.md:27` — Passing `testRequest.args` does not ensure guard runs are never exempt.**  
   Lines 27–28 say guard runs are not exempt, then prescribe using the original caller’s arguments to decide the filter exemption.
   A call with `--guards -- -run Something` retains that original `-run`; `runGuardStep` appends its own overriding guard filter after it (`internal/gatecli/tierrun.go:159–174`).
   Thus the guard package has a known matching guard filter, but `emptyTierFailure` sees the caller’s original filter and exempts a missing-tag, zero-test guard run.
   The planned guard fixture explicitly gives the call empty arguments (`_lyx/plan/15-empty-tier-fails.md:32`), so it misses this path.
   Guard-step identity must take precedence over the caller-filter exemption.

4. **BLOCKING — design — `_lyx/plan/24-retag-cmd-contracts-tools.md:31` — The mandatory pre-retag verification accepts a different tag ending in the tier name.**  
   The regex contains `-tags[ =][^ ]*` immediately before the expected `tierN`, without a tag-token boundary before that name.
   A tier0 command containing `go test -tags nottier0 ...` passes the regex despite never enabling `tier0`.
   Such command text is accepted by the tier decoder, which checks that the command is nonempty and validates its other fields, not its build tags (`internal/gateslot/tiers.go:120–153`, inherited reading).
   This violates D13’s explicit constraint that the first retag Verify fail unless each command names its actual tier tag, at the point where the installed binary lacks D7.
   Separately, the Verify only validates lines beginning with eight spaces and `command:`; valid flow-style tier mappings can set `seen` without ever validating a command.
   `printResolved` prints `Provenance` directly (`internal/configcli/configcli.go:68–80`), and the YAML resolve/annotation paths retain node styles rather than normalizing every tier into block form (`internal/yamlengine/resolve.go:31–47`, `internal/yamlengine/layers.go:526–537`).
   The command should validate decoded tier commands and exact tag tokens, including that every observed tier supplied a checked command.
   I have not proposed a replacement Verify command, since that requires the handler’s overlay validation under the rubric.

Tool-call manifest:

1. Read gateslot load/decode and shedbuild assembly, and located setting/gate resolvers: upstream validation and constructor sequencing were absent from the inherited manifest.
2. Read `SettingLookup`, gate-entry resolution, and searched YAML/config shape handling: needed to trace settings and legacy-list refusals before their proposed guards.
3. Read configengine loading, guard invocation arguments, recipe parsing, and resolved-output entry points: needed concrete upstream rejection and filter propagation.
4. Read numbered legacy-shape checks, plan refusal/filter lines, gate template, and located CLI tests: needed exact evidence locations and current template semantics.
5. Read setting wiring, resolved-output implementation references, and CLI refusal tests: needed to distinguish cached loom lookup from batten’s fresh lookup and verify pre-hub flag reachability.
6. Read guard-set derivation, relevant tests, config provenance and setting wiring: needed to determine which guards reach planning and whether resolved YAML is normalized.
7. Read YAML resolution/annotation, shed validation, and numbered guard scheduling evidence: needed to verify flow-style handling, skip-cycle reachability and per-tier exclusion.
8. Read exact open-map setter behavior, resolved printing, card 24 Verify and review severity rules: needed to avoid incorrectly claiming the normal setter preserves flow style and to ground the Verify finding.
