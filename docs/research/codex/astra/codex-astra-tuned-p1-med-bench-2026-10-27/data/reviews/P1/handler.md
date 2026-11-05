---
verdict: BLOCKING
findings:
  - id: F1
    severity: BLOCKING
    class: consistency
    origin: lens:guard-reach
    location: _lyx/plan/01-gate-key-shape.md:53
    summary: The retired-list refusal is unreachable at the prescribed decoding boundary
    fix: Revise card 1 to handle the retired-list shape error at the config-loading boundary before decodeConfig, preserving configengine ownership of layer reads, and retain its TestLoadConfig row as an end-to-end assertion of the reconcile advice. A concrete replacement target list is withheld under the read-only execution restriction; any expanded targets must satisfy redundant-file-target and redundant-package-target and receive the required overlay validation.
  - id: F2
    severity: BLOCKING
    class: consistency
    origin: lens:way-forward
    location: _lyx/plan/19-recipe-entry-gate.md:43
    summary: The non-bool recovery can edit the wrong layer and cannot repair a valid numeric operand
    fix: Amend card 19's refusal and TestBuild_EntryGate cases to distinguish a wrongly written bare operand from an invalid setting value. For a legitimate numeric setting, prescribe a comparison or the appropriate budget operand in the recipe. For a bad default-layer value, prescribe lyx config <module> --layer default from the prime with the operator hand-off; retain the hub-wide and run_settings alternatives for their respective sources.
  - id: F3
    severity: MEDIUM
    class: consistency
    origin: lens:guard-reach
    location: _lyx/plan/15-empty-tier-fails.md:27
    summary: A caller filter still exempts a guard run from the missing-tag check
    fix: In card 15's Intent, make filter exemption conditional on !run.guardStep as well as the caller arguments. Extend its already planned TestEmptyTierFailure guard row to include caller args containing -run X and require failure when that guard tier's command lacks its tag.
  - id: F4
    severity: MEDIUM
    class: consistency
    origin: lens:way-forward
    location: _lyx/plan/01-gate-key-shape.md:53
    summary: Bare config reconcile is not an equivalent repair for the hub gate file
    fix: Remove the parenthetical lyx config reconcile alternative from card 1's retired-list refusal, retaining lyx fabric reconcile from the prime and its role hand-off.
  - id: F5
    severity: MEDIUM
    class: design
    origin: lens:affected-set
    location: _lyx/plan/11-package-load-tags.md:13
    summary: The package-load inventory misses the tree-sitter dependency guard
    fix: Extend card 24's Intent, whose existing cmd/lyx package target covers this function, to update TestTreeSitterLink_OnlyAllowedTestBinariesLinkIt's go list tag set to tier0,tier1,tier2,tier3,integration,tmux,llm in the same card as its header retag. Include this loader in the D13 follow-up description on card 33.
  - id: F6
    severity: MEDIUM
    class: design
    origin: lens:affected-set
    location: _lyx/plan/18-skipped-history-readers.md:26
    summary: The history-reader inventory incorrectly excludes a skipped Darn-Write
    fix: Correct card 18's claim that a skip cannot name Darn-Write and require latestDarnOutcome to ignore OutcomeSkipped when selecting an actual writer verdict, with a history containing a real writer verdict followed by a skipped Darn-Write as coverage. A concrete expanded target list is withheld under the read-only execution restriction; the required overlay validation must precede proposing that list, without pairing a file or package target with its member.
  - id: F7
    severity: MEDIUM
    class: design
    origin: handler
    location: _lyx/plan/00-overview.md:62
    summary: The retag cards use Custom to avoid an explicitly supported Edit representation
    fix: Replace the retag-rules justification with the edit-test-file contract's header-edit representation, naming the existing members of each affected test file under Edit and declaring any extracted support separately. Do not propose test-file self glyphs under Edit or mix package targets with their members. Concrete replacement target lists are withheld because this round forbids execution of the required overlay validator; edit-test-file, redundant-file-target and redundant-package-target constrain the correction.
  - id: F8
    severity: MEDIUM
    class: consistency
    origin: handler
    location: _lyx/plan/00-overview.md:68
    summary: The scan is deliberately scheduled after the last retag instead of with it as D5 requires
    fix: Restructure the final retag and scan into one ordinary multi-label card so the last retag commit carries the one-tag scan, and remove scan-after-last-retag from Shared Decisions. Remove the resulting redundant card through lyx loom remove-card, never by deleting or renaming card files manually; retain the D13 follow-up work in the resulting plan.
  - id: F9
    severity: MEDIUM
    class: consistency
    origin: handler
    location: _lyx/plan/04-verify-gate-site-key.md:19
    summary: Two prescribed result-type changes omit their re-sign arrows
    fix: On card 4, give darnTreeScope the arrow func darnTreeScope(l *lyxcwd.Location) func(gateKey string) (string, bool, error). On card 14, give guardRunOf the arrow func guardRunOf(plan []tierRun) []tierRun, matching the planned return of every guard run. These replace declaration heads on existing targets and add no target.
---

### [BLOCKING] Retired-list refusal cannot be reached

**Location:** `_lyx/plan/01-gate-key-shape.md:53`.
**Issue:** The prescribed `card: [0]` input fails before `decodeConfig` can provide the required reconcile advice.
`gateslot.LoadConfig` calls `configengine.Load` first (`internal/gateslot/config.go:134`), which calls `yamlengine.FillMissing` (`internal/configengine/config.go:234`).
With the new template's mapping at `gates.card`, `fillMapping` rejects the existing sequence (`internal/yamlengine/fill.go:112`).
`gates` is a per-name map, not an open map (`internal/gateslot/config.go:60`), so the open-map exemption does not avoid this rejection.
The load instead returns the existing editor advice through `withConfigWayForward`.
This violates the rubric's reachability requirement and the constraint that new refusals provide a usable way forward.

### [BLOCKING] Entry-gate recovery does not address the admitted state

**Location:** `_lyx/plan/19-recipe-entry-gate.md:43`.
**Issue:** A bare `loom.review_max_bounces` passes the known-key check but returns an integer, legitimately declared as such in `internal/loomengine/config.go:470`.
Changing it to a bool would break the typed configuration load at `internal/loomengine/config.go:1227`; the recipe operand needs correction instead.
The proposed default-layer repair also selects the wrong file for per-worktree modules: `selectedLayer` defaults them to the worktree layer (`internal/configcli/hubwide.go:102`), while loom and batten are per-worktree registrations (`internal/configreg/configreg.go:120`, `:129`).
Thus the named recovery can leave the task's offending default unchanged.
This is a `PATTERN-refusal-way-forward` violation.

### [MEDIUM] Caller filters bypass the guard check

**Location:** `_lyx/plan/15-empty-tier-fails.md:27`.
**Issue:** Using `testRequest.args` prevents the guard's own appended filter from triggering the exemption, but does not prevent a caller's filter from doing so.
`runGuardStep` preserves caller args, appends its own `-run`, and passes the original request to `runTier` (`internal/gatecli/tierrun.go:165`).
A narrowed call with `-- -run X` therefore still exempts the guard run under the proposed implementation, contrary to D7's explicit rule.
The planned test covers only empty caller args.

### [MEDIUM] The alternative reconcile command makes no repair

**Location:** `_lyx/plan/01-gate-key-shape.md:53`.
**Issue:** Bare `lyx config reconcile` addresses the worktree's own/local files, not the hub's default files (`internal/configcli/configcli.go:527`).
It also defaults to dry-run (`internal/configcli/configcli.go:730`).
The primary fabric-reconcile route is usable, but the purported equivalent alternative is not.

### [MEDIUM] A test-dependency loader keeps only the old tags

**Location:** `_lyx/plan/11-package-load-tags.md:13`.
**Issue:** `TestTreeSitterLink_OnlyAllowedTestBinariesLinkIt` explicitly promises to inspect every tier's test binaries but invokes `go list -test -deps -tags integration,tmux,llm` (`cmd/lyx/treesitterlink_integration_test.go:33`, `:40`).
Card 11's three-loader inventory omits it, and card 24 prescribes only retagging its header.
After retagging, imports exclusive to ordinary tier-tagged test files disappear from this dependency check, weakening the boundary and risking stale-allowlist failures.
This is an affected-set omission in the D5 conversion, not an authored dependency-edge complaint.

### [MEDIUM] A skip can name the writer row

**Location:** `_lyx/plan/18-skipped-history-readers.md:26`.
**Issue:** Card 19 allows an entry gate on any recipe row, including Darn-Write.
`latestDarnOutcome` selects a Darn-Write entry while excluding only `OutcomeReroute` (`internal/loomcli/darnwiring.go:110`).
It would therefore return `skipped` as the writer's outcome, despite D8 requiring skips to represent passage rather than a producer call.
Card 18's explicit assertion that a skip never names Darn-Write is false for the supported recipe surface.

### [MEDIUM] Custom is not a last resort for header edits

**Location:** `_lyx/plan/00-overview.md:62`, cards 24–32.
**Issue:** The overview argues that a header edit has no member to name and that rejecting test-file targets makes Custom necessary.
The authoritative `edit-test-file` contract expressly supplies the representation for precisely this case: name every member of the file for a header-only change (`.bench/specs/loom/loom-plan-spec.md:573`).
The number of files does not invalidate that representation or license Custom under the rubric's last-resort rule.
This finding concerns misuse of Custom, not re-derivation of the mechanical edit-test-file check: the existing Custom cards do not trigger that check.

### [MEDIUM] The final retag lacks its required scan

**Location:** `_lyx/plan/00-overview.md:68`, `_lyx/plan/33-tier-tags-scan.md:15`.
**Issue:** D5 explicitly requires the one-tag scan to land with the last retag card.
The plan instead records a new shared decision to land it afterward, citing the Custom label restriction.
That is a departure from the answer key, not merely a missing dependency field.
The standard Edit/Create representation permits the scan and final retag to share a commit.

### [MEDIUM] Re-sign arrows are missing

**Location:** `_lyx/plan/04-verify-gate-site-key.md:19`, `_lyx/plan/14-guard-step-per-tier.md:24`.
**Issue:** Card 4 changes the result of `darnTreeScope` from an unkeyed closure (`internal/loomcli/darnwiring.go:76`) to a keyed closure, but lists a bare Edit glyph.
Card 14 changes `guardRunOf` from one `*tierRun` (`internal/gatecli/run.go:202`) to every guard run (`_lyx/plan/14-guard-step-per-tier.md:61`), also without an arrow.
Both are function result changes, not exempt struct-field or interface-method changes.

## Rejected

- The affected-set lens also proposed resetting `rerouteCount` and Batten failure episodes on skips.
  Their current Done-based behavior is established, but D8's segment bounce-budget reset does not unambiguously settle these separate reroute-cap and failure-notice policies; those extensions are not retained as proven defects.
- No raw test command is executed by card 24's Verify line: its `go test` text is an awk pattern inspecting configuration.
  Cards 25–32 have no Verify lines, and their absence is permitted.
- Card 23's shortening scope is licensed by the recorded parent amendments.
- Card 33 writes a different board entry, as D13 requires; it does not edit this task's body or scope.
- Cards 1–6 explicitly stage the key rename before --gate rendering, and the retag cards follow cards 11–16; no forward dependency was established in those sequences.
- No redundant existing one-tag scan or new prescribed production-duration sleep was established.

## Focus departures

- The instruction to check every existing-code claim was not completed exhaustively across the thousand-file retag surface.
  The review followed the named implementation paths and the forks' bounded affected-set searches; it does not claim a complete repository-wide audit.
- The requested board read and proposed-target overlay runs were not executed because step 1 also restricts evidence gathering to reading files and running nothing.
  Target-expanding fixes above therefore explicitly withhold concrete replacement lists; the card-removal correction uses the rubric's insert/remove exception.
  No tests, plan validator, board mutation, target edit, or mutating git command was run.
