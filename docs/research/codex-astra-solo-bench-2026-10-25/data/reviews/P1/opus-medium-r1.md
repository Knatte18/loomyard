---
verdict: BLOCKING
findings:
  - id: F1
    severity: BLOCKING
    class: design
    location: _lyx/plan/23-pattern-shorten.md
    summary: Cards 3, 7, 19 and 33 each grow PATTERN.md while it sits above its headroom line, so the card gate's lyx loom lint-pattern step fails on each; card 23's trim lands too late for 3, 7 and 19, and card 33 still crosses the line after it.
    fix: Give each PATTERN-growing card enough of card 23's nineteen shortenings to cover its own growth, so no card grows PATTERN.md, then remove card 23 with lyx loom remove-card 23. Move each fenced line verbatim from card 23 into the card named here (each already lists PATTERN.md under Edit). Card 3 (grows +69 bytes) takes PATTERN-shed-producer-seam, PATTERN-pane-binary-resolution, PATTERN-parent-lock, PATTERN-hub-store-gc and PATTERN-recipe-owns-failures (-83). Card 7 (+122) takes PATTERN-command-row, PATTERN-origin-reconcile, PATTERN-fabric-write-containment, PATTERN-role-skills-typed, PATTERN-lyxdirs-single-declarer and PATTERN-owner-machine (-123). Card 19 (+133) takes PATTERN-binary-change-reconcile, PATTERN-shell-guard-table, PATTERN-never-force-add and PATTERN-shed-recipe-registry (-138). Card 33 (+137) takes PATTERN-trimpath-safe-root, PATTERN-template-overlay, PATTERN-agent-filed-issues and PATTERN-treadle-runner-seam (-138). Each card's Intent then says its gate's lint-pattern step passes because the card's shortenings cover its added bytes. The constraining check is pattern-headroom, whose whole-plan projection this fix leaves unchanged.
  - id: F2
    severity: MEDIUM
    class: scope
    location: _lyx/plan/04-verify-gate-site-key.md
    summary: Card 4 changes darnTreeScope's result to a reader keyed by gate key (Env.VerifyTreeScope becomes func(gateKey string)) but lists it without the re-sign arrow and omits its caller TestDarnTreeScope_ClassifiesAtEachCall, which calls darnTreeScope(...)() and stops compiling until card 5 lists it.
    fix: "On card 4, write the Edit bullet as `internal/loomcli#darnTreeScope` -> `func darnTreeScope(l *lyxcwd.Location) func(gateKey string) (string, bool, error)`, add `internal/loomcli#TestDarnTreeScope_ClassifiesAtEachCall` under Edit, and state in Intent that the test passes a key such as code-burler-fix to the returned reader."
  - id: F3
    severity: MEDIUM
    class: consistency
    location: _lyx/plan/14-guard-step-per-tier.md
    summary: Card 14's Intent says guardRunOf returns every guard run instead of the one *tierRun it returns today, but the Edit bullet carries no re-sign arrow.
    fix: "On card 14, replace the bullet `internal/gatecli#guardRunOf` with `internal/gatecli#guardRunOf` -> `func guardRunOf(plan []tierRun) []tierRun`; its only caller, runTest, is already on the card."
  - id: F4
    severity: LOW
    class: scope
    location: _lyx/plan/20-batten-switch-entry-gate.md
    summary: Card 20 creates a new hubforge integration test, TestWireSetting_OpenTerminalForcesOpenWindow, for the forced Open-Window answer. The same card already makes stepCommandRows_FollowTheTasksLocalSwitch (hubforge, open_window false in the task's local layer, forced through --open-terminal) and TestWire_MarkerAndReportSeamsRefuseAnAbsentTaskWorktree (the forced Env.Setting answer) cover it, against PATTERN-test-economy.
    fix: "On card 20, drop the Create bullet `plan:internal/battencli#TestWireSetting_OpenTerminalForcesOpenWindow` and the Intent sentence that describes it. Instead, state that stepCommandRows_FollowTheTasksLocalSwitch, already under Edit, also steps Seed-IDE under --open-terminal with seed_ide off and asserts no Seed-IDE marker is written, so the force touches only Open-Window's setting."
  - id: F5
    severity: LOW
    class: consistency
    location: _lyx/plan/07-green-record.md
    summary: Card 7 places green.json at gateslot.WaitDir(anchor), but gatecli calls WaitDir with both location.AnchorPath() (wait records) and location.WorktreePath() (run log outside a hub), and D4 asks for a per-worktree record, so the card leaves the record's home ambiguous.
    fix: On card 7, replace gateslot.WaitDir(anchor) with gateslot.WaitDir(location.WorktreePath()), the per-worktree gate directory run.go already uses for the run log outside a hub, and say the record and its lock file live there in a hub too.
  - id: F6
    severity: LOW
    class: consistency
    location: _lyx/plan/07-green-record.md
    summary: Card 7 says how green.json is read (tolerantly) and locked (internal/lock), but not that it is rewritten through internal/state, which PATTERN-tolerant-records requires for a Go-written JSON record and which the decision record's Constraints apply to the green record.
    fix: On card 7, state in Intent that recordGreen rewrites green.json through internal/state's locked JSON write, keeping keys it does not know, with the lock beside the record, and drop the separate internal/lock wording, or keep it only where state's write takes the lock path.
---

### [BLOCKING] PATTERN headroom lint fails the card gate of four cards

**Location:** `_lyx/plan/23-pattern-shorten.md`, with `_lyx/plan/03-gate-call-renders-key.md`, `_lyx/plan/07-green-record.md`, `_lyx/plan/19-recipe-entry-gate.md` and `_lyx/plan/33-tier-tags-scan.md`

**Issue:**
Every card gate's last step is `lyx loom lint-pattern` (`internal/websterengine/cardgate.go:22`).
The implementer runs it before committing (`contracts/stencils/webster/webster-body-implementer.md`, step 3 before step 4).
Without `--base` it compares the working tree's `PATTERN.md` with HEAD's (`internal/loomcli/lintpattern.go`).
`pattern.Headroom` (`internal/pattern/headroom.go:12`) reports a finding whenever the head is longer than the base and also longer than `HeadroomBytes`, which is 16384 × 95 / 100 = 15564 bytes.
`PATTERN.md` is 15651 bytes today, so it is already above the line, as the decision record says.
I measured each card's growth against the current lines:

- Card 3 rewrites `PATTERN-test-tiers` from 133 to 202 bytes: +69.
- Card 7 adds `PATTERN-gate-carry`: +122.
- Card 19 adds `PATTERN-row-entry-gate`: +133.
- Card 33 adds `PATTERN-tier-tags`: +137.
- Card 23's nineteen shortenings save 482 bytes.

Cards 3, 7 and 19 each grow an overview that is above the line, so each card's own gate fails.
Card 23 then brings the file to 15493 bytes, and card 33 grows it to 15630, again above 15564, so card 33 fails too.
Moving card 23 first does not fix it either: the final size is 15630 bytes, above the line, so whichever card adds the last bytes still crosses it.
The plan-level `pattern-headroom` check projects the whole plan against the base (15630 < 15651), so it passes and cannot see the per-card failures.
The only order that works has no card grow the file, which means pairing each added byte with shortening bytes on the same card.
That is possible because 482 ≥ 461; the `fix` gives one such split.
I did not overlay the fix with `lyx loom validate-plan --overlay`, because this round's tool-use rules allow reading only (see Notes).
The fix moves fenced lines between cards that already list `PATTERN.md` under `**Edit:**`, and removes card 23 through `lyx loom remove-card`, which the rubric exempts from the overlay.

### [MEDIUM] Card 4 re-signs darnTreeScope with no arrow and misses its test caller

**Location:** `_lyx/plan/04-verify-gate-site-key.md`

**Issue:**
Card 4 changes `Env.VerifyTreeScope` to `func(gateKey string) (string, bool, error)`, and `loomCLI.wireDarn` sets it to `darnTreeScope(location)` (`internal/loomcli/darnwiring.go:223`).
The card says `darnTreeScope` "takes the key and, until card 6, ignores it".
That changes its result type from `func() (string, bool, error)` (`internal/loomcli/darnwiring.go:76`), but the bullet carries no re-sign arrow.
`TestDarnTreeScope_ClassifiesAtEachCall` (`internal/loomcli/darntreescope_integration_test.go:28`, `//go:build integration`) calls the reader with no argument at lines 55 and 90.
Card 4 does not list it; cards 5 and 6 do.
So the test package stops compiling under the `integration` tag from card 4 until card 5.
Because the arrow is missing, `caller-uncovered` cannot see this.

### [MEDIUM] Card 14 re-signs guardRunOf with no arrow

**Location:** `_lyx/plan/14-guard-step-per-tier.md`

**Issue:**
`guardRunOf` is `func guardRunOf(plan []tierRun) *tierRun` today (`internal/gatecli/run.go:202`).
Card 14's Intent says it "returns every guard run" and that `runTest` runs them in tier order.
That is a result change, but the `**Edit:**` bullet has no re-sign arrow, while the card does give `runGuardStep`, `FindGuardTests` and the other re-signed members their arrows.

### [LOW] Card 20's new integration test repeats tests the same card already rewrites

**Location:** `_lyx/plan/20-batten-switch-entry-gate.md`

**Issue:**
After card 20, `stepCommandRows_FollowTheTasksLocalSwitch` (`internal/battencli/lifecycle_integration_test.go:1254`) forces Open-Window through `--open-terminal`, in a hubforge fixture whose task-local layer sets `open_window: false`.
That is the behavior the new `TestWireSetting_OpenTerminalForcesOpenWindow` pins.
`TestWire_MarkerAndReportSeamsRefuseAnAbsentTaskWorktree` (`internal/battencli/wire_seams_test.go:229`) asserts the forced `Env.Setting` answer with and without `--open-terminal` and needs no hub.
The only new assertion is that `seed_ide` is unchanged, and an existing test can carry that (`PATTERN-test-economy`).
A further hubforge test also runs against `PATTERN-hubforge-fixtures`, since a fake already tests the seam.

### [LOW] Card 7's green record path is ambiguous

**Location:** `_lyx/plan/07-green-record.md`

**Issue:**
The card says the record lives at `gateslot.WaitDir(anchor)`.
In `internal/gatecli/run.go`, `WaitDir` takes `location.AnchorPath()` for wait records (line 346) and `location.WorktreePath()` for the run log outside a hub (line 95).
D4 asks for a per-worktree record under the worktree's never-tracked `.lyx` gate directory.
The card should name which root it means.

### [LOW] Card 7 does not route the green record's rewrite through internal/state

**Location:** `_lyx/plan/07-green-record.md`

**Issue:**
The Constraints apply `PATTERN-tolerant-records` to the green record.
That entry says a Go-written JSON record is rewritten via `internal/state`.
The card names a tolerant read and an `internal/lock` lock but no `internal/state` write, which leaves room for a hand-rolled write that drops unknown keys.

## Focus verdicts

- Cards 24–32, `Custom` over package self glyphs: justified, not a defect.
  A header-only retag has no member to name, and `edit-test-file` refuses a `_test.go` file self glyph under `Edit` and `Extends`, so no combination of the other seven labels fits.
  The only Verify line among them, card 24's, runs `lyx config gate --resolved` through awk.
  It is no raw test command.
  It reads block-style tiers, which is how `lyx config gate --set` writes a tier (`internal/yamlengine/set.go`, `openMapEntryNode` sets `entry.Style = 0`).
  It fails on an empty read, so it cannot pass vacuously.
  Cards 24–32 together cover every directory holding a `_test.go` file outside testdata; I diffed the two lists and found no gap.
- Card order across 1–6 and 24–33: no card uses a symbol, flag or key a later card creates.
  Cards 11, 12, 14, 15 and 16 all come before card 24, as D5 requires.
  The interim keys of the `interim-keys` decision keep every site on a key that exists.
  The one order defect is the PATTERN headroom one in F1.
- Decision-record fidelity: every Decision D1–D14, every `Added after Discussion` shortening (cards 3 and 23), every Constraint and every PATTERN change is carried by some card.
  Card 23's scope is licensed by the twenty `Added after Discussion` entries.
  Card 33's D13 board follow-up writes another entry, as D13 asks.
  Card 24's `covers` check is D13's own requirement.
  No card adds scope the record does not license, and none edits this task's own board entry.
- New refusals, skips and bypasses: each states its bound.
  These are card 2's `--gate` combinations, card 7's carry with its fail-open on any record error, card 15's empty-tier failure with its filtered-run exemption, and card 17's goto bypass.
  Each prescribed refusal names an input and a reaching test.
  Card 14's guard-group refusal of a command lacking `{targets}` or `{args}` already exists today (`internal/gatecli/doc.go:56`).
- Card 5's `Deps.BaseCommand` set to the `code-burler-fix` call only keeps that site's own verified-tree entry from being pruned (`internal/verifytree/verify.go:65`).
  It keeps D3's own lint base and is not a defect.
- Card 17 carries a pending caller label past skips to the next verdict, which matches D8 and the `pending_by` contract in `internal/shedengine/doc.go`.
- Test economy: apart from F4, I found no test duplicating an existing one and no per-symbol test.
- Member glyphs and re-sign arrows on the source `Edit` cards: complete apart from F2 and F3.
- No card leaves a design fact to be found out at run time.
  Card 5's "no production code reads `LandingFailure.Kind`" and the `planparser-loads-nothing` decision are recorded findings, not research steps.

## Notes

- I proposed no fix as an overlay and did not run `lyx loom validate-plan --overlay`.
  This round's tool-use rules say "read files, run nothing", and I kept to read-only `grep`, `sed -n`, `awk`, `find` and `comm` reads of the plan, the decision record and the source.
  None of F2, F3, F5 or F6 adds, drops, moves or pairs a target in a way the `redundant-*` checks govern.
  F2 adds one test member glyph, and F4 drops one `Create` handle no other card references.
- `_lyx/plan/lens-review.md` is neither the overview nor a card the Card Index names, so it is outside this rubric's subject, and I did not read it.
