---
verdict: BLOCKING
findings:
  - id: "owner-write-skip-bypasses-publication"
    severity: "BLOCKING"
    class: "consistency"
    location: "_lyx/plan/03-board-owner-writes.md:52"
    origin: "handler"
    summary: "Owner writes can report success without publishing ownership to Bolt"
    fix: "In card 3's Intent, require owner-changing SetOwners requests to complete the pull, commit and confirmed push even when BOARD_SKIP_GIT or BOARD_SKIP_PUSH is set, or refuse before writing with instructions to unset those variables and rerun; change the existing TestSetOwners skip row to cover that refusal and use card 8's already-planned fake Set seam for its no-git claim test."
  - id: "short-owner-invariant-changes-rule"
    severity: "BLOCKING"
    class: "consistency"
    location: "_lyx/plan/17-orch-role-and-pattern.md:18"
    origin: "handler"
    summary: "The shortened invariant forbids the declared Finalize exception and overstates origin-only reads"
    fix: "Rewrite card 17's fenced overview entry to retain the post-parent-merge Finalize exception explicitly and refer to the background for the effective-owner defaults and the distinct origin/local read sources, keeping the full declared text verbatim in the background and retaining the required no-growth wording cuts; describe any trailer as a departure from the card rather than authorization to change the invariant."
  - id: "prime-claim-overwrites-pulled-owner"
    severity: "MEDIUM"
    class: "consistency"
    location: "_lyx/plan/03-board-owner-writes.md:47"
    origin: "lens:guard-reach"
    summary: "A first-act prime claim can overwrite the owner discovered by its own pull"
    fix: "Specify in card 3 that a Prime request without Takeover rechecks prime_owner inside the post-pull write callback and leaves an existing other owner unchanged, reporting prime in Lost; extend the already-planned TestSetOwners table with a stale local board lacking prime_owner whose remote already records another machine."
  - id: "adopter-first-retains-merge-remedy"
    severity: "MEDIUM"
    class: "consistency"
    location: "_lyx/plan/12-finalize-owner-checks.md:45"
    origin: "lens:way-forward"
    summary: "Appending the adopter-first repair retains contradictory merge-and-push advice"
    fix: "Change card 12's pushParent Intent so a positively identified adopter-first landing replaces the ordinary classified recovery with the squash-removal recovery while retaining the underlying failure detail and failed-push record; extend its existing TestFinalize_MergeMarkPushOrderAndVerdict rows to assert that this halt contains no merge-origin or push-by-hand remedy, and that ordinary recovery remains when origin does not hold the work."
  - id: "repair-assumes-squash-is-head"
    severity: "MEDIUM"
    class: "consistency"
    location: "_lyx/plan/12-finalize-owner-checks.md:45"
    origin: "lens:way-forward"
    summary: "The parent HEAD is not a reliable identity for the stale task squash"
    fix: "Replace card 12's parent-head shortcut with a requirement to identify the task's actual local landing commit across Finalize re-entry before constructing the reset or rebase command, preserving other local commits and withholding a destructive command when that identity cannot be established; extend the already-planned repair table with a stale task squash followed by an unrelated local commit and assert that the command names the squash rather than HEAD."
  - id: "adopt-held-by-hub-network-pause"
    severity: "MEDIUM"
    class: "scope"
    location: "_lyx/plan/16-adopt-module.md:55"
    origin: "lens:way-forward"
    summary: "A standing hub pause prevents adopt from reaching its promised takeover and recovery"
    fix: "In card 16's hubDeps and adopt Intent, define a temporary network exemption for the exact standing hub entry around takeover steps 1 through 3, restore it on every exit before any resume, preserve the pause record and all other pauses, and state this bound explicitly; extend the already-planned adoption integration test to install the real pause gate and verify that takeover completes while the hub pause remains and resume stays covered."
  - id: "affected-set-enumeration-incomplete"
    severity: "MEDIUM"
    class: "design"
    location: "_lyx/plan/04-board-owner-views.md:22; _lyx/plan/10-innerrun-owner-check.md:38; _lyx/plan/12-finalize-owner-checks.md:40"
    origin: "lens:affected-set"
    summary: "The affected-set enumeration misses the compact owner view and shared seam inventories"
    fix: "Complete the affected-set enumeration for card 4's RenderCompact output and TestRenderCompact coverage and for cards 10 and 12's new optional fields in envkit.nilLegal, including their existing inventory tests; a concrete target-list patch is withheld because this round's read-files/run-nothing rule prevents the required validate-plan --overlay baseline and proposed-fix checks, and any eventual patch must satisfy redundant-package-target and redundant-file-target."
  - id: "public-api-granularity"
    severity: "MEDIUM"
    class: "design"
    location: "_lyx/plan/16-adopt-module.md:16"
    origin: "handler"
    summary: "Independent public APIs are bundled into their consumer cards"
    fix: "Use lyx loom insert-card to give the independent public surfaces their own prerequisite cards with their supporting types and coverage, including ParentSlug from card 3, ParentHoldsTaskWork from card 5, PauseOwnedElsewhere from card 11, LocalOnlyCommits from card 12, ClientAttached from card 15, and ResumeInWindow, DropLocal and CloneSecond from card 16; keep private helpers and inseparable constructors with their surface and let the insertion command renumber the plan."
---

### [BLOCKING] Ownership must be published before it is reported recorded

**Location:** _lyx/plan/03-board-owner-writes.md:52
**Issue:** The card explicitly treats both skip modes as successful recorded owner writes despite performing no push.
D3 requires an owner write's push before a start or owner-writing resume proceeds, and D5 requires it before takeover resumes anything.
The declared PATTERN-owner-machine also places the post-write re-read on the Bolt just pushed; the skip branch supplies no such pushed state.
These are production environment variables, with no test-only restriction in internal/boardengine/skipenv.go:16.
The existing Bolt.PushRecorded implementation deliberately does nothing for either skip flag (internal/fabricengine/bolt.go:88), so that API does not independently enforce the new requirement.
An unowned legacy run can consequently be resumed with ownership recorded only locally, leaving another machine free to make a different claim.

### [BLOCKING] Compression changes the ownership rule

**Location:** _lyx/plan/17-orch-role-and-pattern.md:18
**Issue:** The literal overview entry says starting, resuming and landing are allowed only when no other machine owns the entry, without the declared exception for completing a Finalize already past its parent-side merge.
It also makes ownership uniformly judged against origin in ReconcileOrigin, omitting the distinct local-Bolt reads explicitly required by the declared invariant.
Cards 10 and 12 deliberately implement the omitted exception and local reads.
The added-after-Discussion decision at decision-record.md:216 authorizes a short overview and the full text in its background, not a contradictory rule.
The line can remain short while naming the exception and referring to the full read-source rules.
The Pattern-Departure contract at .bench/specs/loom/loom-plan-spec.md:277 records an implementation's departure from its card to obey PATTERN; it does not waive the declared invariant.
This is a semantic contradiction, not a line-length or headroom finding.

### [MEDIUM] First-act prime claims need the post-pull guard too

**Location:** _lyx/plan/03-board-owner-writes.md:47
**Issue:** The setter writes the prime owner whenever Prime is set, while the following non-Takeover guard explicitly protects task slugs only.
Card 8's resume calls claimPrimeOwner before its resume reconcile.
With an old local Bolt lacking prime_owner and an origin that already records another machine, that preflight requests a first claim.
PullThenCommitWritten pulls before invoking the write callback under its lock (internal/fabricengine/bolt.go:145).
The planned unconditional prime write then overwrites the newly fetched owner in an ordinary fast-forward commit.
No competing push or replay is needed, so origin-wins replay cannot preserve the first claim.
This contradicts D2's first-act semantics and D5's explicit takeover boundary.

### [MEDIUM] The special repair must replace ordinary recovery advice

**Location:** _lyx/plan/12-finalize-owner-checks.md:45
**Issue:** Card 12 appends the repair to the current non-transient push error.
Finalize.pushParent includes the classified error verbatim at internal/landingshed/finalize.go:328.
PushFailedError.Error appends its existing way forward at internal/fabricengine/pushclassified.go:40, and a non-fast-forward code push names merging origin and pushing at internal/fabricengine/failedpush.go:249.
The adopter-first halt would therefore offer both merging the two landings and dropping the stale squash.
D6 explicitly requires dropping that squash and never merging it, because the task is already landed on origin.
Merely testing that the new rebase/reset text is present will not catch the contradictory old remedy.

### [MEDIUM] A later parent commit makes the repair target wrong

**Location:** _lyx/plan/12-finalize-owner-checks.md:45
**Issue:** The card identifies the task squash using the parent's current HEAD even when it lists multiple local commits.
A Finalize may re-enter after its merge and after a later local commit was added.
The existing parent guard accepts a changed tip that still holds the task's work (internal/landingshed/parentguard.go:112), and the already-contained merge proceeds to pushParent (internal/landingshed/finalize.go:232).
For local history base -> task-squash -> unrelated-commit, HEAD identifies unrelated-commit.
The proposed rebase would drop that commit while retaining the stale task squash.
D6 instead requires identifying the task squash by its own SHA and preserving other unpushed work.
The planned test with merely another commit listed must distinguish a later commit from one preceding the squash.

### [MEDIUM] Adoption cannot reach its hub-pause recovery

**Location:** _lyx/plan/16-adopt-module.md:55
**Issue:** The card explicitly supports a hub-paused pair, including the drop-local bound, and promises an envelope naming the standing hub pause and its lift.
Production root pre-run installs the network pause gate (cmd/lyx/main.go:125), and remote git calls enter it before their deadline starts (internal/gitexec/gitexec.go:192).
The ordinary moving reconcile in step 1 therefore waits on the hub pause before adoption reaches its local-choice branch, owner write or recovery envelope.
The parent-lock exemption applies only to slug entries; a hub entry continues to hold the wait (internal/pauserecord/hold.go:62 and :245).
This is not a request to lift an operator pause implicitly: D5 requires that pause to remain.
The missing piece is a bounded takeover-only network exemption, with the ordinary pause restored before resume.

### [MEDIUM] Enumerate derived readers and reflective tests

**Location:** _lyx/plan/04-board-owner-views.md:22; _lyx/plan/10-innerrun-owner-check.md:38; _lyx/plan/12-finalize-owner-checks.md:40
**Issue:** The same affected-set omission appears in an explicit output renderer and in shared reflection-based tests.
Card 4 enriches BriefTask and JSON output, but list --text goes through writeListing (internal/boardcli/cli.go:694) and RenderCompact's explicit columns (internal/boardcli/text.go:20).
That view still omits the owner, contrary to D9's owner-bearing board list; its TestRenderCompact is at internal/boardcli/text_test.go:19.
No card carries that renderer or its existing coverage.

Cards 10 and 12 add optional function fields to InnerRunDeps and landingshed.Deps.
Those structs are recursively inspected by envkit.NilSeams, whose exemptions are an explicit path map, not derived from comments (internal/testkit/envkit/envkit.go:43 and :502).
The omitted paths are InnerRun.OwnerCheck, InnerRun.PauseOwned, InnerRun.ParentHoldsTaskWork, InnerRun.Restepped, Landing.ParentHoldsTaskWork, Landing.ParentOwner, Landing.OriginParentHoldsWork and Landing.ParentCommitsOriginLacks.
TestWire_EverySeamFilled (internal/battencli/wire_seams_test.go:99) and TestFullEnv_FillsEverySeam / TestNilSeams_ReportsExactPaths (internal/testkit/envkit/envkit_test.go:14 and :57) will see the new nil fields.
Card 11's production InnerRun wiring does not populate the shared FullEnv fixture or batten's intentionally absent landing dependencies.
This is one design finding about completing the affected-set enumeration, rather than one finding per omitted member.

### [MEDIUM] Separate independently reviewable public surfaces

**Location:** _lyx/plan/16-adopt-module.md:16
**Issue:** Card 16 combines the adoption CLI with a destructive fabric API, a batten resume API and a second-clone fixture API.
Those are independently reviewable public surfaces in three other packages, and the CLI consumes them across package boundaries.
The same bundling occurs for ParentSlug in card 3, ParentHoldsTaskWork in card 5, PauseOwnedElsewhere in card 11, LocalOnlyCommits in card 12 and ClientAttached in card 15.
The rubric and plan spec's Card granularity section require such surfaces to have their own cards even when one card is the first consumer.
This finding does not request separate cards for each field, private helper or inseparable constructor.
The proposed insertion/reordering fix falls under the rubric's insert-card exception to overlay validation.

## Rejected

- guard-reach: Construction-time capture of a nil orch pause seam is not established by card 13.
  Command already fills the receiver's other seams inside its PersistentPreRunE closure (internal/orchcli/cli.go:171), which runs after the root hook, so the card can be implemented as written without capturing nil at command-tree construction.
- handler: The short overview/background arrangement and wording-only headroom cuts are not themselves defects.
  They are expressly authorized by the added-after-Discussion decision; only the contradictory content of the prescribed short rule is retained above.
- handler: No later-card symbol dependency was established for cards 2–6 and 8–13.
  The owner records precede the setter, the owner-reader API precedes its wiring, the pause label precedes its consumers, and the InnerRun seams precede their batten wiring.
- handler: No missing re-sign arrow was established for the focused Edit cards.
  Card 5 re-signs classifyEntries explicitly; the other examined additions change struct fields or method bodies without prescribing changed function signatures.
- handler: The focused tests in cards 7, 10 and 14 introduce owner-specific behavior rather than duplicating the existing pause, parent-lock or status assertions.
  Card 16's unit takeover table and second-clone integration scenario exercise different boundaries; no redundant-test finding is retained merely because their feature names overlap.

## Scope and instruction conflicts

The decision record was the answer key throughout.
D1 is carried by card 1, D2 by cards 2–3 and the first-act callers, D3 by cards 8–11, D4 by cards 5–7 and 13, D5 by card 16, D6 by cards 10–12, D7 by card 12, D8 by card 15 and D9 by cards 4, 8, 14 and 17.
Cards 15–17 do not add unlicensed feature scope merely by implementing those decisions.
Takeover's explicit caller restriction, the Finalize carve-out criteria and the stated nil-seam defaults provide bounds; the concrete defects retained above are separate from those permitted choices.
The owner-refusal inputs named in cards 8–10 and 12–13 can reach their branches with preceding pause and parent checks satisfied; an earlier pause refusal is not evidence that every owner refusal is unreachable.

The read-files/run-nothing instruction prevented executing the requested board get, tests, quarry resolution and overlay validation.
No board content, test result or successful overlay is claimed.
For the affected-set finding, the required target-edit proposal is withheld rather than presenting an unvalidated glyph patch; the conflicting overlay requirement and its target-redundancy constraints remain explicit in its fix field.
All other proposed fixes change existing Intent/test scenarios or use the prescribed card-insertion route.
No target file, prior-round review or support log was edited or used as an answer key.

