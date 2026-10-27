---
verdict: BLOCKING
findings:
  - id: F1
    severity: BLOCKING
    class: design
    origin: lens:affected-set
    location: _lyx/plan/01-gate-key-shape.md:68
    summary: The affected-set enumeration misses fixtures, rendered-command assertions and tag consumers
    fix: >-
      Re-close the affected sets before their changing cards execute: card 1 must migrate templateDefaults and the direct Gates assignments in TestCardGateCommand, TestBeginBatch_PromptFile and TestRecoverSpawnOrAttach; card 3 must update the five downstream rendering tests identified below; card 11 must include the tree-sitter guard's package load; card 14 must update TestDerive_OverARepository's Guards expectations; the tag transition must also update tmuxkit's non-test admission switch to recognize tier2 and tier3 alongside the transitional spellings.
      A literal target-list patch is withheld because this round's read-files-only rule prevents the mandatory overlay validation; redundant-package-target, redundant-file-target and caller-uncovered constrain that patch.
  - id: F2
    severity: BLOCKING
    class: design
    origin: handler
    location: _lyx/plan/03-gate-call-renders-key.md:37
    summary: PATTERN growth reaches card gates before the planned headroom reductions
    fix: >-
      Distribute card 23's already-authorized shortening instructions into the earlier PATTERN-changing cards, beginning with card 3, so each card passes its own HEAD-to-working-tree headroom comparison.
      Check cards 7, 19 and 33 independently against the overview left by the preceding card; a final-plan size calculation is insufficient.
      These cards already target PATTERN.md, so the shortening prose can move without adding targets.
      Keep card 23 only for any remaining shortening work, or remove it through lyx loom remove-card if none remains.
  - id: F3
    severity: MEDIUM
    class: design
    origin: handler
    location: _lyx/plan/00-overview.md:59
    summary: The Custom retag exception contradicts the header-edit rule and displaces D5's scan
    fix: >-
      Replace the Custom retag representation with the header-edit representation explicitly prescribed by edit-test-file: Edit member glyphs for every member of each retagged file, with Create or Move groups for helper splits where needed.
      Put the one-tag scan on the last retag card as D5 requires, retaining the D13 follow-up filing after the retag work.
      Correct retag-rules and remove scan-after-last-retag's unsupported exception.
      Use insert-card and remove-card for the resulting card consolidation or replacement; such insertion/removal fixes are exempt from overlay validation.
  - id: F4
    severity: MEDIUM
    class: design
    origin: lens:guard-reach
    location: _lyx/plan/24-retag-cmd-contracts-tools.md:31
    summary: The first-retag checker accepts commands whose tier tag was never checked
    fix: >-
      Require the checker to inspect the actual command of every resolved tier regardless of YAML flow or block style, reject any tier whose command was not inspected, and require exact comma-separated tag membership rather than a suffix match.
      Preserve the no-tiers and covers refusals.
      An executable replacement Verify line is withheld because the read-files-only rule prevents the rubric's required baseline and replacement validate-plan --overlay runs; the replacement must pass the Verify validation checks before it is proposed as a concrete command.
  - id: F5
    severity: MEDIUM
    class: consistency
    origin: handler
    location: _lyx/plan/04-verify-gate-site-key.md:19
    summary: Two signature changes lack the required re-sign declarations
    fix: >-
      Declare the keyed return signature of darnTreeScope on card 4 and the collection return signature of guardRunOf on card 14 with re-sign arrows.
      The intended heads are func darnTreeScope(l *lyxcwd.Location) func(gateKey string) (string, bool, error) and func guardRunOf(plan []tierRun) []tierRun.
      Reconcile same-card caller coverage before applying these declarations: caller-uncovered constrains the change, including darnTreeScope's test caller currently assigned to card 5.
      A combined arrow-and-target patch is withheld under the read-files-only restriction on mandatory overlay validation.
  - id: F6
    severity: MEDIUM
    class: consistency
    origin: lens:way-forward
    location: _lyx/plan/19-recipe-entry-gate.md:43
    summary: The non-bool recovery instructions can leave the halted run's setting unchanged
    fix: >-
      Rewrite card 19's recovery text to repair the provenance layer that the running pair reads: use --layer default from prime for a per-worktree module's board default, and --layer local in the task pair for a value seeded from run_settings.
      A board run_settings correction may additionally preserve the value for future runs, but must not be presented as repairing the current pair on resume.
      Retain the ordinary hub-wide config route for a hub-wide module.
  - id: F7
    severity: MEDIUM
    class: design
    origin: handler
    location: _lyx/plan/10-gate-report.md:4
    summary: New public cross-package APIs are bundled into their consuming feature cards
    fix: >-
      Insert an independently reviewable gateslot record-reader card before the report card, carrying ReadRunRecords, its file-contract coverage and its gateslot documentation; leave report rendering, flags and gatecli documentation in the consumer card.
      Likewise separate configreg.CheckSettingKey and its lookup-preservation coverage from card 19's entry-gate implementation.
      Use lyx loom insert-card for these splits and preserve the API-before-consumer order.
  - id: F8
    severity: LOW
    class: consistency
    origin: lens:way-forward
    location: _lyx/plan/01-gate-key-shape.md:53
    summary: The optional bare config reconcile recovery does not migrate the hub file
    fix: >-
      Keep lyx fabric reconcile as the recovery instruction, or spell the alternative lyx config reconcile --layer default --apply from prime.
      Remove the bare lyx config reconcile alternative from card 1's prescribed refusal and documentation.
---

### [BLOCKING] F1 — Close affected sets at the changing card

**Location:** `_lyx/plan/01-gate-key-shape.md:68`, cards 3, 11, 14 and the retag transition.

**Issue:** The plan covers direct implementations but repeatedly omits existing readers that must change with them.
This is one enumeration-method finding, not a separate finding per missed reader.

- `internal/gateslot/config_test.go:21` declares `templateDefaults.Gates` as `map[string][]int`; card 1 changes the field to `map[string]GateKey` without covering that initializer.
  Direct slice assignments also remain in `TestCardGateCommand` (`internal/websterengine/cardgate_test.go:50`), `TestBeginBatch_PromptFile` (`internal/websterengine/beginbatch_test.go:606`) and `TestRecoverSpawnOrAttach` (`internal/websterengine/recoverbatch_test.go:1726`).
  Updating their shared constructors does not update these assignments, and card 1's tests cannot compile.
- Card 3's rendered-command change leaves exact `--tier` assertions in `TestRenderCardGates_OneBulletPerCardWithAStepSubBulletEach` (`internal/websterengine/cardgate_test.go:64`), `TestRenameBoundary_BatchesCheckAndGateTheirOwnTree` (`internal/websterengine/cardgate_test.go:141`), `TestBeginBatch_PromptFile` (`internal/websterengine/beginbatch_test.go:558`), `TestRecoverSpawnOrAttach` (`internal/websterengine/recoverbatch_test.go:1710`) and `TestRebaselineCmd_EditedCardOfFailedBatchThenRecover` (`internal/webstercli/verbs_test.go:987`).
  These assert Go-rendered values, not static stencil prose.
- Card 14 changes `Targets.Guards` to `map[string][]GuardTest`, but `TestDerive_OverARepository` still compares it with `map[string][]string` at `internal/impactset/derive_integration_test.go:102` and `:162`.
  Card 11's earlier test edit cannot adopt a type first created on card 14.
- `TestTreeSitterLink_OnlyAllowedTestBinariesLinkIt` still loads only `integration,tmux,llm` at `cmd/lyx/treesitterlink_integration_test.go:40`.
  Card 24 changes headers only, so the scan loses the retagged test imports that its all-tier contract and `PATTERN-test-speed` require it to inspect.
- `internal/testkit/tmuxkit/allow_tmux.go:1` enables tmux only under `tmux || llm`; its untagged default is false in `allow_none.go:7`.
  The non-test file is outside card 29's retag instructions.
  A tier2-only or tier3-only binary therefore reaches `afterRun`/`checkSockets` with tmux disallowed (`tmuxkit.go:105`, `:138`), even for registered servers.
  The transitional old command tags mask this until their planned removal.

### [BLOCKING] F2 — Headroom must hold after each card

**Location:** `_lyx/plan/03-gate-call-renders-key.md:37`; `_lyx/plan/23-pattern-shorten.md:7`.

**Issue:** The existing overview is 15,651 bytes, already above the 15,564-byte headroom line declared by `internal/pattern/check.go:24`.
Card 3 replaces a 134-byte line with a 203-byte line, growing it by 69 bytes before card 23 performs any shortening.
`cardGateCommand` runs `lyx loom lint-pattern` on every card (`internal/websterengine/cardgate.go:17`), and that verb compares HEAD with the working tree (`internal/loomcli/lintpattern.go:22`, `internal/patternlint/lint.go:25`).
`pattern.Headroom` rejects this growth (`internal/pattern/headroom.go:12`).
Cards 7 and 19 also add overview lines before card 23.
This is an intermediate execution-order failure; the review round's mechanical projection of the completed plan does not check it.

### [MEDIUM] F3 — Custom is not necessary for header retags

**Location:** `_lyx/plan/00-overview.md:59–69`; cards 24–33.

**Issue:** The shared decision says a header change has no member representation and uses `Custom` to avoid `edit-test-file`.
The format explicitly supplies that representation: for a build-constraint edit, name every member of the file (`.bench/specs/loom/loom-plan-spec.md:569–573`).
The number of files does not make the other card types inapplicable.
Consequently the rubric's last-resort rule is violated across the retag cards.
The same unsupported choice is then used to place the scan in a separate following card, although D5 requires it to land with the last retag card.
The plan's shared decision is not an amendment to the answer key.

### [MEDIUM] F4 — The transition check can pass without checking commands

**Location:** `_lyx/plan/24-retag-cmd-contracts-tools.md:31`.

**Issue:** A tier rendered as `tier0: {command: "go test -json {targets} {args}", all_targets: ./..., timeout_min: 60, counter: go-json}` satisfies the tier-heading pattern and sets `seen`, but never satisfies the separate eight-space `command:` pattern.
With no `covers`, the awk program accepts it despite the missing tier tag.
This is an admitted representation: the gate template recommends flow mappings, the layer merger preserves copied nodes (`internal/yamlengine/layers.go:73–86`), and provenance annotation re-marshals without clearing their styles (`:526–541`).
`printResolved` writes that result verbatim (`internal/configcli/configcli.go:70–80`).
Even a matched command can pass with `-tags nottier0`, because the regex lacks a left tag-token boundary.
D13 identifies this check as the installed binary's only protection against vacuous retag gates.
The defect is what the checker accepts, not the appearance of the words `go test` inside its regex.

### [MEDIUM] F5 — Signature changes need re-sign arrows

**Location:** `_lyx/plan/04-verify-gate-site-key.md:19`; `_lyx/plan/14-guard-step-per-tier.md:24`.

**Issue:** Card 4 says `darnTreeScope` takes the gate key and supplies the keyed `Env.VerifyTreeScope` seam, changing the returned function's signature from the unkeyed function declared at `internal/loomcli/darnwiring.go:75`.
Card 14 says `guardRunOf` returns every guard run, changing its single-pointer result at `internal/gatecli/run.go:202`.
Both remain plain Edit bullets.
These are function-result changes, not the exempt interface-method or struct-field cases.
The current target sets also need checking when the arrows expose their same-card caller obligations.

### [MEDIUM] F6 — Repair the layer the resumed run reads

**Location:** `_lyx/plan/19-recipe-entry-gate.md:43`.

**Issue:** Bare `lyx config <module>` selects the worktree layer for a per-worktree module, not its board default (`internal/configcli/hubwide.go:106–116`).
Also, run settings are copied into the task's local layer by Seed-Child (`internal/battenshed/seamchild.go:139–166`); changing the board entry does not rewrite that layer on an ordinary resume.
The existing board command explicitly says the change applies to the next Seed-Child and directs a running pair to `--layer local` (`internal/boardcli/runsettings.go:74–79`).
Batten's setting lookup reads the task's files (`internal/battencli/wire.go:1350–1356`).
The prescribed repairs can therefore leave the same invalid value in place and reproduce the evaluation failure.

### [MEDIUM] F7 — Separate public APIs from their consumers

**Location:** `_lyx/plan/10-gate-report.md:4`; `_lyx/plan/19-recipe-entry-gate.md:8`.

**Issue:** `gateslot.ReadRunRecords` is a new exported file-contract API consumed by gatecli, while `configreg.CheckSettingKey` is a new exported API used by the entry-gate wiring.
Both are bundled into their consuming feature cards.
The rubric and the format's granularity section require a module's public surface or cross-package symbol to have its own independently reviewable card even when it initially has only one consumer.
The private report helpers and private entry-check helpers can remain bundled with their respective surfaces.

### [LOW] F8 — The alternate reconcile command is a dry run of the wrong layer

**Location:** `_lyx/plan/01-gate-key-shape.md:53`.

**Issue:** Bare `lyx config reconcile` defaults to `apply=false` and no layer (`internal/configcli/configcli.go:729–730`).
Without `--layer default`, it addresses worktree/local configuration rather than the hub file (`:525–565`).
It does not repair the retired `gate.yaml` shape the refusal reports.
The alternative `lyx fabric reconcile` already offered by the card does provide a progressing route.

## Rejected

- Raw-test-command objection to cards 24–32: card 24 inspects configuration with awk; it does not execute the `go test` text inside the regex, and cards 25–32 prescribe no raw Verify command.
- Premature `--gate` rendering in card 1: its Intent explicitly retains the old rendering until card 3, after card 2 creates the flag.
- Card 23 as unlicensed scope: the nineteen shortenings are authorized by the decision record's Added after Discussion entries; their timing, rather than their scope, is defective.
- Card 33 as a duplicate of tier purity: the existing purity and LLM scans do not enforce exactly one tier per test file, so the new scan covers distinct behavior.
- Missing reaching tests for nil entry-gate seams: card 19 explicitly identifies these as fail-loud wiring guards, which the rubric exempts.
- Own-board-entry mutation: card 33 files a different follow-up entry and only gives it a dependency on this task; it does not prescribe editing this task's body or scope.

## Review limits

The supplied tool-use rule says to read files and run nothing.
I therefore ran no tests, board command, quarry command or plan validator, and wrote only this review file.
This conflicts with the rubric's requirement to validate concrete target-list and Verify-command fixes through `validate-plan --overlay`; those literal patches are withheld where noted rather than represented as validated.
The findings are grounded in the current plan, decision record and source reads, not in prior reviews or the support log.
