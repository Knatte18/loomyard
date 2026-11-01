---
verdict: BLOCKING
findings:
  - id: F1
    severity: BLOCKING
    class: design
    origin: lens:repo-invariants
    location: _lym/plan/12-entrymerge-package.md:3
    summary: "Card 12 creates the test package internal/entrymerge with no TestMain through tmuxkit.Main, so the guard TestTmuxIsolation_EveryTestPackageRunsThroughTmuxkitMain fails its own card gate (PATTERN-test-isolation)"
    fix: "In card 12, add `- internal/entrymerge/testmain_test.go` (in backticks) under **Create:** after merge_test.go, add a **Uses:** section holding `internal/testkit/tmuxkit#Main`, and add the Intent line: `testmain_test.go` is untagged and holds support only, a `TestMain` that runs the tests through `tmuxkit.Main` (PATTERN-test-isolation). Checked with lym loom validate-plan --overlay: no new blocking finding."
  - id: F2
    severity: BLOCKING
    class: design
    origin: lens:repo-invariants
    location: _lym/plan/31-heddle-cli.md:34
    summary: "Cards 31 and 32 create heddlecli.Command() but only card 33 mounts it in newRoot, so the guard TestRegistration_AllModulesRegistered fails the card gates of 31 and 32 (card order)"
    fix: "Move the mount into card 31. Add an **Edit:** section listing `cmd/lym#newRoot`. Change the Intent sentence The chair's cmd/lym/main.go card mounts ... to say this card adds `heddlecli.Command()` to `root.AddCommand` and `heddle` to root's Long Available modules list, which TestRegistration_AllModulesRegistered and the CLI tree test check. Add **ImpactSummary:** Adds the lym heddle subtree and its one mount in the root; no other command changes. In card 33, drop the root.AddCommand and Long paragraph and keep the three seam installs and their pin. Checked with lym loom validate-plan --overlay: no new blocking finding."
  - id: F3
    severity: BLOCKING
    class: consistency
    origin: handler
    location: _lym/plan/26-heddle-compose.md:5
    summary: "Card 26 declares Compose(f File, target, warpRoot string) in internal/heddleengine, outside the PATTERN-fabric-vocabulary owner set, contradicting the entry and the plan's own heddle-contracts-as-ruled decision"
    fix: "In card 26, change the Create signature to `func Compose(f File, target, root string) (text string, ok bool, err error)` and rename warpRoot to root in the Intent lines that name the parameter (the claude-file, codex-file and read-only lines). In card 24, word the doc.go sentence as the worktree-seam and prompt-seam implementations and the target table's file at the worktree root, so no heddle comment spells a side token."
  - id: F4
    severity: BLOCKING
    class: design
    origin: lens:guard-reach
    location: _lym/plan/31-heddle-cli.md:36
    summary: "Card 31's outside-a-hub refusal rests on lymcwd.Resolve, which succeeds in a plain git repository, so set and remove there would write heddle.json and start a sync under the parent's _board; boardcli refuses through boardengine.LoadConfig, a step heddlecli lacks"
    fix: "In card 31's PersistentPreRunE paragraph, add: after Resolve, refuse when fabricengine.BoltDir(location.HubPath) is not a directory, the test fabricengine.ConfigDirs applies, naming running it inside a hub's worktree as the way forward. Add `internal/fabricengine#ConfigDirs` or keep BoltDir under **Uses:** as needed, and make TestHeddleCLI_Refusals' outside-a-hub row a plain git repository made with the hermetic git the file already uses, asserting no heddle.json and no sync start."
  - id: F5
    severity: MEDIUM
    class: design
    origin: handler
    location: _lym/plan/31-heddle-cli.md:43
    summary: "Card 31's new set and remove verbs let any process in a hub worktree, an agent run included, change text that reaches every later session's system prompt and every rendered CLAUDE.local.md, and the Intent states no bound (attack-surface question)"
    fix: "In card 31's Intent, add a paragraph on what set and remove let through and what bounds them: any caller resolving to a hub worktree can change heddle.json; the change is validated by ValidateEntry, reaches a session only through the claude-prompt passages at its next launch and the file targets at the next made, take-in or hand render, never a live worktree, and is committed by the board sync with its own history. Name the audience annotation value each verb carries."
  - id: F6
    severity: MEDIUM
    class: design
    origin: handler
    location: _lym/plan/23-claude-prompt-passages.md:20
    summary: "Cards 23 and 30 make a heddle read failure, an undecodable heddle.json or a Bolt write lock held past the 30 s DefaultBoltLockWait during a Bolt push, refuse every Claude launch in the hub; D4 licenses only no passage for a missing seam, and no Intent states this bound"
    fix: "Settle the seam-error behaviour in cards 22, 23 and 30. Either make StandingPassages log the seam error as a warning naming lym heddle list and answer no passage, so a launch proceeds with lym's sentences only, and change card 23's error row to assert lym's sentences only and a launched Prepare; or keep the refusal and state in card 23's Intent that every launch in the hub fails while heddle.json is undecodable or the Bolt lock is held past the wait, and why that bound is acceptable, routed to the decision record through the parent since D4 does not decide it."
  - id: F7
    severity: MEDIUM
    class: decision
    origin: handler
    location: _lym/plan/00-overview.md:122
    summary: "The record's Constraint says lym heddle is wired through internal/cliwire, while the Shared Decision heddlecli-hub-only leaves cliwire unchanged on a chair ruling the decision record does not carry"
    fix: "Ask the parent to record the departure in the decision record with lym loom decision add, a hub-only CLI resolving through lymcwd.Resolve as boardcli does since PATTERN-cliwire-sole-wiring binds standalone-capable CLIs, and cite that entry in the Shared Decision's Rationale; or wire card 31 through a cliwire.Module descriptor as the Constraint says."
  - id: F8
    severity: MEDIUM
    class: decision
    origin: lens:guard-reach
    location: _lym/plan/00-overview.md:84
    summary: "D7's a claimed path with no replayer fails holds only for the run ledger: with the board registered and heddle's replayer not, card 16 cherry-picks a heddle.json commit, yet the overview says this is how the record's rule holds"
    fix: "Rewrite the overview's Part bolt-replay-seam bullet to say fabric detects a missing registration only for the run ledger, and a path whose replayer is not registered is unclaimed and cherry-picked; ask the parent to record that narrowing of D7 and its acceptance line through lym loom decision add, since fabric names no claimed path of its own."
  - id: F9
    severity: MEDIUM
    class: consistency
    origin: lens:repo-invariants
    location: _lym/plan/34-pattern-heddle-entries.md:3
    summary: "Card 34 widens the PATTERN-durable-vs-ephemeral-state entry to admit heddle's renders and .heddle outside .lym, but its background file still says every never-tracked file lives under .lym and no card edits it"
    fix: "In card 34, add `pattern/PATTERN-durable-vs-ephemeral-state.md` under **Edit:**, and add an Intent paragraph: the background's opening rule gains the same exception, heddle's rendered instruction files and their .heddle temporary directory at a warp root, which are never tracked and sit outside .lym. Checked with lym loom validate-plan --overlay: no new blocking finding."
  - id: F10
    severity: MEDIUM
    class: consistency
    origin: lens:guard-reach
    location: _lym/plan/16-replay-through-registry.md:119
    summary: "Card 16 prescribes that a commit already set aside for an ended ledger part is not tagged twice, but no TestRecordsReplay row reaches that branch"
    fix: "Add one row to card 16's TestRecordsReplay Bolt rows: a local commit changing only the ended run neve's entry and notes.json, with neve done on origin and notes.json also changed on origin, tags the original set-aside/neve/<short sha> once, which tagsOf lists alone, and leaves HEAD on upstream."
  - id: F11
    severity: LOW
    class: consistency
    origin: lens:guard-reach
    location: _lym/plan/25-heddle-write.md:43
    summary: "Card 25's StartSync failure refusal naming lym board sync has a reachable input but no test row reaches it"
    fix: "Add a row to card 25's TestWriter_Refusals in which the recording StartSync returns an error, asserting that heddle.json holds the saved change and the error names lym board sync."
  - id: F12
    severity: LOW
    class: consistency
    origin: lens:guard-reach
    location: _lym/plan/18-seed-warp-exclude.md:52
    summary: "Card 18's CloneAndWire seeding is conditioned on the repo-wide fabric config existing, a state that cannot be false there since the strict WiredNames load before WireJunctionsWith already returned"
    fix: "In card 18's CloneAndWire bullet, drop the clause when the repo-wide fabric config exists, so SeedRootExclude is called unconditionally right after WireJunctionsWith."
  - id: F13
    severity: LOW
    class: decision
    origin: handler
    location: _lym/plan/00-overview.md:63
    summary: "The Shared Decision board names outside fabric stay narrows D9's rule that what lives in Bolt beside the board takes its place from BoltDirName, never BoardDirName: gate config and slots, the shortname record and hub config outside fabric keep deriving from BoardDir"
    fix: "Either extend the D9 cards to move the callers outside fabric that derive Bolt-beside-the-board places from fabricengine.BoardDir onto fabricengine.BoltDir (internal/hubgeom/gateslots.go GateSlots, internal/hubgeom/creelgeom.go CreelGeometry, internal/configcli/configcli.go's config dir uses and the shortname reads), or ask the parent to record the narrowing through lym loom decision add and cite it in the Shared Decision's Rationale."
  - id: F14
    severity: LOW
    class: consistency
    origin: handler
    location: _lym/plan/01-bolt-dir-constant.md:16
    summary: "Card 1 admits internal/lymcwd in runproc's allowedImports, but leaf_enforcement_test.go's file header, which says runproc imports only the standard library, proc, state and logger, is outside that member and stays false"
    fix: "In card 1, add `internal/runproc#TestLeafInvariant_AllowlistOnly` under **Edit:** beside `internal/runproc#allowedImports`, so the card names every member of the file, and add an Intent sentence saying the file header names internal/lymcwd among the imports. Checked with lym loom validate-plan --overlay: no new blocking finding."
---

### [BLOCKING] entrymerge has no TestMain

**Location:** `_lym/plan/12-entrymerge-package.md:3`

**Issue:** Card 12 creates `internal/entrymerge/doc.go` and `merge_test.go` only.
PATTERN-test-isolation requires every test package to run through `tmuxkit.Main`, and the tier0 guard `TestTmuxIsolation_EveryTestPackageRunsThroughTmuxkitMain` (`cmd/lym/tmuxisolation_test.go:258`) fails a package with test files and no `TestMain` (`:247-248`).
`internal/entrymerge` does not exist yet, so nothing else supplies one.

### [BLOCKING] lym heddle is mounted two cards after its package exists

**Location:** `_lym/plan/31-heddle-cli.md:34`

**Issue:** `TestRegistration_AllModulesRegistered` (`cmd/lym/registration_test.go:55-58`, `//lym:guard`, tier0) fails for any package exposing `Command()` that `newRoot()` does not mount.
Card 31 creates `heddlecli.Command()` and leaves the mount to card 33, so the card gates of cards 31 and 32 fail.

### [BLOCKING] A side token in a heddle signature

**Location:** `_lym/plan/26-heddle-compose.md:5`

**Issue:** `internal/heddleengine` is not in PATTERN-fabric-vocabulary's owner set, and "a non-owner file carries no side-named spelling" in identifiers, strings or comments (`TestEnforcement_FabricVocabulary`, `internal/lymcwd/enforcement_test.go:734-756`).
Card 26's `Compose(f File, target, warpRoot string)` spells one in a parameter name, against the plan's own Shared Decision heddle-contracts-as-ruled ("no heddle name, comment or string spells a fabric side token").
Card 24's description of `doc.go` ("the warp-seam and prompt-seam implementations") invites the same spelling in a comment.

### [BLOCKING] The outside-a-hub refusal is not reached in a plain repository

**Location:** `_lym/plan/31-heddle-cli.md:36`

**Issue:** D2 requires every verb to refuse outside a hub.
`lymcwd.Resolve` succeeds at the root of any git worktree.
`boardcli` refuses outside a hub only through the strict `boardengine.LoadConfig(fabricengine.ConfigDirs(layout), "board")` step that follows (`internal/boardcli/cli.go:131-142`).
Card 31 copies the `Resolve` step but has no second step, so `set` would write `heddle.json` into `<parent>/_board` of a plain repository.

### [MEDIUM] Unbounded write path into every agent's system prompt

**Location:** `_lym/plan/31-heddle-cli.md:43`

**Issue:** `set` and `remove` are new verbs whose effect reaches every later Claude launch in the hub (cards 23 and 30) and every rendered `CLAUDE.local.md` (card 29).
Any process resolving to a hub worktree may call them, a run's agent included.
The rubric's attack-surface rule asks the introducing card to state what the verb lets through and what bounds it, and card 31's Intent states neither.

### [MEDIUM] A heddle read failure refuses every launch

**Location:** `_lym/plan/23-claude-prompt-passages.md:20`

**Issue:** Card 30 returns a busy lock as an error, card 22 passes it through, and card 23 fails `Prepare` on it.
`LoadLocked` waits up to `DefaultBoltLockWait` (30 s, card 27), and D3 records that a Bolt push holds that lock across its fetch and push retries.
So every launch during a long push waits and then fails, and a `heddle.json` that does not decode refuses every launch hub-wide.
D4 settles only the missing-seam and outside-a-hub cases.

### [MEDIUM] The cliwire Constraint is not carried

**Location:** `_lym/plan/00-overview.md:122`

**Issue:** The decision record's Constraints say "`lym heddle` is one cobra subtree, wired through `internal/cliwire`".
The plan's Shared Decision heddlecli-hub-only leaves `internal/cliwire` unchanged on a chair ruling, and no `Added after Discussion` entry records the change.
The plan's reading fits PATTERN-cliwire-sole-wiring, which binds standalone-capable CLIs, but the record must say so before the plan departs from it.

### [MEDIUM] D7's missing-replayer rule holds for the ledger alone

**Location:** `_lym/plan/00-overview.md:84`

**Issue:** Fabric names no claimed path, so a `heddle.json` commit with only the board registered has no claimant and is cherry-picked (card 16).
The overview says the record's rule holds "this way", which is true only for the run ledger.
The record's D7 bullet (`decision-record.md:163`) and its acceptance line (`:235`) need the narrowing recorded.

### [MEDIUM] PATTERN background left stale

**Location:** `_lym/plan/34-pattern-heddle-entries.md:3`

**Issue:** The declared overview text admits heddle's renders outside `.lym`, while `pattern/PATTERN-durable-vs-ephemeral-state.md:3` still says "Every never-tracked file lives under `.lym`".

### [MEDIUM] The not-tagged-twice branch has no reaching row

**Location:** `_lym/plan/16-replay-through-registry.md:119`

**Issue:** An ended ledger part beside a non-ledger replayer's loss is a reachable state, and card 16 prescribes one tag for it.
The planned rows cover each condition only alone.

### [LOW] StartSync failure untested

**Location:** `_lym/plan/25-heddle-write.md:43`

**Issue:** Card 25 prescribes a refusal naming `lym board sync` with no test row that makes `StartSync` fail.

### [LOW] Dead condition on clone seeding

**Location:** `_lym/plan/18-seed-warp-exclude.md:52`

**Issue:** `CloneAndWire` loads the repo-wide fabric config strictly through `WiredNames` before `WireJunctionsWith` (`internal/fabriccli/clone.go:103-106`), so "when the repo-wide fabric config exists" is always true where card 18 seeds.

### [LOW] D9's Bolt-beside-the-board rule narrowed to fabric

**Location:** `_lym/plan/00-overview.md:63`

**Issue:** D9 says what lives in Bolt beside the board, config and the `.lym` scratch tree among it, "takes its place from `BoltDirName`, never from `BoardDirName`".
Callers outside fabric still derive such places from `BoardDir`, for example `internal/hubgeom/gateslots.go:17-24` (the gate slot dir and gate config) and `internal/hubgeom/creelgeom.go:19` (the shortname record).
The values are identical, so behaviour is unaffected, but the Shared Decision narrows a Decision without a record entry.

### [LOW] runproc's leaf test header left stale

**Location:** `_lym/plan/01-bolt-dir-constant.md:16`

**Issue:** `internal/runproc/leaf_enforcement_test.go:4` says production code in `internal/runproc` imports only the standard library, `proc`, `state` and `logger`.
Card 1 edits the `allowedImports` member only, and the header sits outside it.

## Rejected

- Test economy, card 12's `TestMerge` against `TestFabricReplay` (`internal/boardengine/replay_test.go`): `entrymerge` is a public surface shared by two packages, its contract test belongs at that surface, and D8 requires the board's rows to stay unchanged.
- Card 28's merge rows against `TestMerge`: D8's acceptance names heddle's replay case explicitly.
- `SeedRootExclude` in place of the record's `SeedWarpExclude`: heddlecli calls it, and PATTERN-fabric-vocabulary bars a side token in an owner-set export used by a non-owner file. The constraint wins, and the overview records the rename.
- Clone's "made" fired in `fabriccli.CloneAndWire` rather than `CloneHub`: `CloneAndWire` is `CloneHub`'s only production caller (`internal/fabriccli/clone.go:46`), and `CloneHub` returns before junctions exist, so D6's own "once the worktree is wired" ordering requires the move.
- Granularity of cards 1 and 3 and of cards 18 to 21: each card is one reviewable unit: the constant and its derived paths, the fabric call-site sweep, the seeding, the seam, and each event site.
- Re-sign arrows: every Edit member whose parameters or results change carries one (`ffWorktree`, `appendPromptText`, `createdStampMatches`, `FabricReplay.Apply`, `applyDesignFile`, `scribbleThenFailReplay.Apply`), and `SeedRootExclude` is a Create.

## Focus departures

- "Card order: ... Settle whether any card uses a symbol a later card declares": settled. Cards 15/16, 19/21 and 22/23 are in order. Cards 31 and 32 against card 33 are F2.
- "Fidelity to the answer key: ... including the chair rulings the Shared Decisions cite (creel #288, #308)": the chair rulings themselves were not read. They live in creel mail, outside the target and the answer key. Fidelity is judged against the decision record alone (F7, F8, F13). Prose-only cards 9 to 11 and PATTERN card 34 carry scope D9 and the record's PATTERN changes license.

## Process note

The step-2 instruction file was read in the same parallel batch as the step-1 file, before step 1's reading finished, which departs from the order those instructions set.
Its content shaped no reading choice in step 1.
