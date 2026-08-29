# Batch: doc-surface-sweep

```yaml
task: 'reed: pane reap isn''t applied consistently across up/add''s mutating paths'
batch: 'doc-surface-sweep'
number: 3
cards: 5
verify: go test ./internal/reedengine/
depends-on: [2, 4]
```

## Batch Scope

This batch delivers the doc surface batches 1 and 2 falsified: `internal/reedengine/doc.go` (reed's design document — there is no `manifest/designs/reed.md`), `state.go`'s operator-facing corrupt-state remedy, the stray adoption-referencing comments in `strand.go`, `lifecycle.go` and `reconcile.go`, and the two sandbox scenario specs whose verdicts change.
It is one batch because every card is prose over a shared premise set, no card changes runnable behaviour, and the closing sweep card has to see all of them landed to be meaningful.

Two premises are falsified, not one, so the sweep is two greps rather than one: the adoption seam, and the reap-gate's "it needs a bound present pane" premise (whose prose contains no "adopt" at all).

It depends on batch 2 because both premises are only false once adoption is deleted and the chokepoint exists;
writing the replacement prose earlier would document code that is not there yet.
It also depends on batch 4, and runs last of the four for that reason.
Card 12's closing sweep spans `internal/reedcli/*.go`, which includes the three smoke test files batch 4 rewrites;
were batch 4 a parallel sibling rather than a dependency, card 12 would run against a tree still holding batch 4's un-rewritten adoption comments and would have to report every one of them as a missed rewrite.
The sweep is only meaningful as the last thing that happens.

Batch-local decision beyond `## Shared Decisions`: the half-updated state is the hazard here, so a comment left asserting `planPaneTarget` "never adopts the header" is worse than no comment — it implies adoption still exists and merely excludes the header.
Card 12 exists to make that failure detectable rather than trusting the enumeration below.

## Cards

### Card 8: Rewrite doc.go's adoption seams and document the new reap policy

- **Context:**
  - `internal/reedengine/reconcile.go`
  - `internal/reedengine/spawn.go`
  - `internal/reedengine/lifecycle.go`
  - `internal/reedengine/apply.go`
  - `internal/reedengine/strand.go`
  - `internal/reedengine/state.go`
  - `internal/reedengine/generation.go`
  - `CONSTRAINTS.md`
- **Edits:**
  - `internal/reedengine/doc.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Rewrite four passages in `internal/reedengine/doc.go`, each of which asserts something that no longer exists.
  Count them off against the First/Second/Third/Fourth enumeration below and do not stop at three — the fourth is the one passage neither of card 12's sweeps can reach, so nothing downstream would catch it.
  First, the header package-invariant sentence naming adoption as one of the header's three exclusion seams (the sentence listing `ensureHeaderPaneLocked` in `lifecycle.go`, `planPaneTarget` in `spawn.go`, and `planReconcile`'s `exemptPaneIDs` in `reconcile.go` as "the three exclusion seams", reached via the phrase "every strand-accounting, adoption, split-target, and reconcile path").
  With adoption gone the header is excluded from strand accounting, from being the preferred split target, and from both halves of reconcile's kill schedule;
  restate the seams accordingly and drop "adoption" from the enumeration.
  Second, the load-bearing-assumption bullet titled "Dead-pane adoption via remain-on-exit (spawn.go)", whose body asserts that `planPaneTarget` must never adopt such a corpse.
  The `remain-on-exit` mechanism itself is unchanged and must stay documented — it is still the only signal letting reconcile distinguish "the strand's process died" from "the pane is simply gone", and the last-pane-fate bullet immediately following still refers back to it.
  Rewrite the bullet so it documents that mechanism and its consequences without asserting an adoption rule: retitle it, and replace the "must never adopt such a corpse" clause with the fact that a corpse is never a strand's pane because a strand's pane is always freshly split.
  Third, the duplicate-binding paragraph asserting that nothing reed constructs can produce a duplicate-owner table because "planPaneTarget never adopts or splits the header and validateSplitCreatedNewPane guarantees a fresh id".
  The conclusion still holds and must stay — restate its first half as `planPaneTarget` always yielding a split whose result `validateSplitCreatedNewPane` proves genuinely new.
  Fourth, the set-hook bullet's `!anyPlacedStrand` paragraph, which calls that case "reachable for good via the operator remedy state.go documents, which deletes reed.json while the session keeps running untracked".
  That durability claim is the same one card 9 corrects in `state.go` itself, and neither of card 12's grep terms reaches it, so it must be caught here.
  Restate it to the new limit: the remedy still reaches the `!anyPlacedStrand` case, but the session keeps running untracked only until the next mutating verb reaps it.
  The paragraph's actual conclusion — that the surviving hook array is a benefit there, still holding the live header and strips at the budgets reed last computed — is unaffected and stays.
  Then add new prose documenting the tightened rule this task establishes, in the same voice and structure as the surrounding load-bearing-assumption bullets: every pane in a reed session is either the header or a bound strand's pane;
  the untracked reap is authorized by `anyBoundPresent || headerAlive` where the header anchor requires aliveness rather than mere presence, because `launchStrandLocked` makes the gate fire from `AddStrand` and `UpdateStrand`, which never heal a header corpse;
  and the reap runs before pane allocation at one chokepoint inside `launchStrandLocked` so the property holds by construction on every realization path rather than requiring two call sites to stay in sync.
  Record the two consequences the discussion accepted: an `up` against a session with zero tracked strands ends up header-only and full-height, because `applyLayoutLockedOpts` deliberately skips `select-layout` when no strand owns a present pane, and the header snaps back to its configured height the moment a strand pane exists;
  and `RemoveStrand`'s own code is unchanged but its `reconcileApplyPersistLocked` tail inherits the new gate, so removing the last strand now reaps any untracked alive pane in the same verb.
  Leave the session-name-rewrite bullet alone — its closing clause, about substituting separators mapping sibling worktrees onto one session and having "each adopt the other's panes", is about a session-name collision between two worktrees, not about `planPaneTarget`.
  It shares the word, not the concept.
  This is the only "adopt" hit in the file beyond the three passages above;
  `doc.go` carries no `adoptPaneGenerationLocked` prose at all, so do not go looking for a generation-probe paragraph to spare.
- **Commit:** `docs(reedengine): document the deterministic reap policy and drop the adoption seam`

### Card 9: Restate the corrupt-state remedy's promise to its new limit

- **Context:**
  - `internal/reedengine/reconcile.go`
  - `internal/reedengine/apply.go`
  - `internal/reedengine/generation.go`
- **Edits:**
  - `internal/reedengine/state.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `internal/reedengine/state.go`, correct both halves of `unreadableStateError`.
  Its doc comment currently justifies the delete-the-file remedy with the exact behaviour this task removes — that with no strands recorded, `planReconcile`'s untracked reap does not fire because it needs a bound present pane, so the panes and their processes keep running untracked.
  That premise is false once an alive header authorizes the reap.
  The remedy itself stays valid and stays offered: the panes do keep running, `attach` never reconciles, so the operator can still get back to their work.
  Rewrite the comment to state the new limit — the panes survive until the next mutating verb (`up`, `resume`, `add`, or `remove`), which reaps them — and drop the falsified `applyLayoutLocked`/`anyPlacedStrand` justification along with it.
  Keep the R5-F1 provenance, the reason both remedies are offered rather than one chosen, and the paragraph refusing automatic repair, all unchanged.
  Then correct the live error string the function returns.
  Its trailing clause currently promises deleting the file "to keep the session (its panes and their processes keep running, untracked) and lose only reed's strand tracking".
  Restate it so the operator reads the real limit: the panes keep running untracked but are reaped by the next mutating verb, so `attach` is the way back to that work.
  Getting this wrong is an operator-facing correctness bug, not a doc nicety — the string is what someone reads while holding running work they can no longer see.
  Keep the string's existing shape: it still names the file, still names both remedies, and still leads with `lyx reed down`.
  Do not change `unreadableStateError`'s signature.
  Two `ReedState` field comments near the top of the same file also carry the word, and they need opposite treatment — handle each explicitly rather than as one group.
  The `HeaderPaneID` field comment says the header "is excluded from every strand accounting, adoption, reconcile, and layout path a Strand would otherwise be subject to".
  That is pane adoption — the same seam `doc.go`'s header-invariant sentence names, and card 8's twin — so rewrite it: with adoption deleted the header is excluded from strand accounting, from being the preferred split target, and from both halves of reconcile's kill schedule.
  Keep the rest of that comment (the header-is-not-a-strand decision, and what an empty value means) unchanged.
  The `PaneGeneration` field comment's closing sentence — a zero value "is adopted rather than treated as a mismatch" — belongs to `adoptPaneGenerationLocked`'s generation stamp, which this task does not touch.
  Leave that one exactly as it is.
- **Commit:** `docs(reedengine): state the reap limit on the scrubbed-state remedy`

### Card 10: Correct the stray adoption-referencing comments outside spawn.go

- **Context:**
  - `internal/reedengine/spawn.go`
  - `internal/reedengine/doc.go`
- **Edits:**
  - `internal/reedengine/strand.go`
  - `internal/reedengine/lifecycle.go`
  - `internal/reedengine/reconcile.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Three comments outside `spawn.go` assert the deleted adoption seam.
  Correct each in place, changing no code.
  In `internal/reedengine/strand.go`, the `RemoveStrand` kill-loop comment ends by saying that on psmux the reconcile tail simply re-enumerates and re-applies "and planPaneTarget never adopts a corpse".
  Replace that trailing clause — a strand's pane is now always a fresh split, so a corpse is never reused.
  Everything else in that comment (the binary-dependent last-pane fate, the `hasSession` re-probe, the swallow) is unchanged and must stay.
  While in this comment's vicinity, do not alter `RemoveStrand`'s code: it is unchanged by this task, and its inheriting the new reap gate through `reconcileApplyPersistLocked` is the intended consequence of one rule for every call site, not something to special-case back to the old behaviour.
  In `internal/reedengine/lifecycle.go`, the zero-pane-husk comment says such a session "cannot host a strand (there is no pane to adopt or split, and tmux offers no way to add a pane to an empty window)".
  Drop the adoption half;
  the split half and the conclusion still hold.
  In `internal/reedengine/reconcile.go`, `clearConflictingPaneBindings`' doc comment asserts that reed's own construction paths guarantee one owner per pane because "planPaneTarget never adopts or splits the header, validateSplitCreatedNewPane refuses an id that already existed".
  The guarantee still holds and must stay stated;
  restate its first half as `planPaneTarget` never yielding the header as a split target while any non-header pane exists, and its result always being validated as genuinely new.
  Do not change `clearConflictingPaneBindings`' logic — it is unrelated to this fix and shares only the file.
- **Commit:** `docs(reedengine): drop the deleted adoption seam from stray comments`

### Card 11: Restate the M13, M16 and M22 sandbox scenario specs

- **Context:**
  - `internal/reedengine/spawn.go`
  - `internal/reedengine/reconcile.go`
  - `internal/reedengine/apply.go`
  - `_mill/discussion.md`
- **Edits:**
  - `tools/sandbox/SANDBOX-REED-SUITE.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Three scenarios in `tools/sandbox/SANDBOX-REED-SUITE.md` assert pre-fix behaviour;
  leaving them would make the next sandbox pass re-report the fixed behaviour as a regression.
  Restate each `Watch` paragraph to the post-fix expectation, keeping each scenario's `Goal`, its `Verdict` line, and the surrounding document structure intact.
  For **M16** (foreign pane in the reed session): the foreign pane is now reaped one verb earlier — on the follow-up `lyx reed up`, not after the subsequent `add`.
  The surviving requirement is that `up` must still never wipe the pane set, so an empty pane list is still a `FAIL`.
  Restate the reaping expectation accordingly, and update the quoted error string `"session has no panes to adopt or split"` to the new `"session has no panes to split"` (card 4 changed it).
  Keep the note that a *tracked* strand's pane disappearing instead of the foreign one is the `FAIL`, and keep the pointer to the headless coverage — but retarget it, since batch 4 both rewrites `TestSmokeUpWithOnlyForeignPanesKeepsSessionUsable` and adds `TestSmokeForeignPaneIsReapedNotAdoptedByAdd`, which is the faithful M16 regression.
  For **M22** (recovery from a scrubbed reed.json while the session is up): the `Watch` currently expects the rebuilt header "at the very top of the window with the strand stack below it".
  After this change the orphaned strand pane is reaped along with the old header, so the correct expectation is the header alone, occupying the full window, with the `up` converging immediately rather than one verb late.
  Restate the `FAIL` conditions accordingly: an `up` that fails with `no space for new pane` is still a `FAIL`, a header that does not end up topmost is still a `FAIL`, and an added strand whose command never runs is still a `FAIL` — but a surviving old header pane or a surviving orphaned strand pane is now itself a `FAIL`, and a full-height header with nothing below it is the expected `OK`, not a defect.
  Keep the `.lyx` scrub framing and the note that it is a sanctioned operator action.
  For **M13** (add after removing the last strand): only the explanation of the cause changes.
  Its `FAIL` diagnosis attributes a strand flipping to `live: false` with an empty `paneId` to having "adopted a dead leftover pane" — a diagnosis that is impossible once adoption is gone.
  The observable symptom and the `FAIL` condition still stand;
  rewrite only the causal clause.
  Do not touch any other scenario in this file, and do not touch `tools/sandbox/SANDBOX-FABRIC-SUITE.md`, whose several "adopt" hits are fabric's own merge-adoption concept and share the word, not the concept.
- **Commit:** `docs(sandbox): restate M13, M16 and M22 to the post-reap-fix expectations`

### Card 12: Confirm both falsified premises are gone from the tree

- **Context:**
  - `internal/reedengine/doc.go`
  - `internal/reedengine/state.go`
  - `internal/reedengine/spawn.go`
  - `internal/reedengine/reconcile.go`
  - `internal/reedengine/strand.go`
  - `internal/reedengine/lifecycle.go`
  - `internal/reedengine/spawn_test.go`
  - `internal/reedengine/lifecycle_test.go`
  - `internal/reedengine/reconcile_test.go`
  - `internal/reedengine/generation.go`
  - `internal/reedengine/generation_test.go`
  - `internal/reedengine/server.go`
  - `internal/reedcli/up.go`
  - `internal/reedcli/smoke_lifecycle_test.go`
  - `internal/reedcli/smoke_panecwd_test.go`
  - `internal/reedcli/smoke_teardown_test.go`
  - `internal/reedengine/attach_test.go`
  - `internal/reedengine/reapply_test.go`
  - `internal/reedengine/contract_integration_test.go`
  - `tools/sandbox/SANDBOX-REED-SUITE.md`
  - `tools/sandbox/SANDBOX-FABRIC-SUITE.md`
- **Edits:** none
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Re-run both sweeps over the tree and confirm every remaining hit is a legitimate survivor rather than a missed rewrite.
  This card changes no file;
  its output is a judgement, and any hit it cannot justify means an earlier card in this batch (or batch 2, or batch 4) left prose asserting something that no longer exists.
  Run `grep -rn "adopt" internal/reedengine/*.go internal/reedcli/*.go tools/sandbox/*.md` and `grep -rni "untracked reap\|bound present pane\|reap.*does not fire" internal/reedengine/*.go tools/sandbox/*.md`, then give every hit a disposition.
  A hit is a legitimate survivor only when "adopt" means something other than pane adoption.
  The survivors are: the server-rebirth generation probe — `adoptPaneGenerationLocked` itself and the prose around it in `generation.go`, `server.go`, `generation_test.go`, `attach_test.go`, `reapply_test.go`, `contract_integration_test.go`, and `spawn_test.go`'s `TestStatus_NeverReportsAStrandLiveOnAPaneAnotherOwnerClaims` doc comment, whose "the guard adopts rather than clears" sentence is about the generation stamp and not about pane adoption — plus, in `state.go`, the `PaneGeneration` field comment alone and never the `HeaderPaneID` one card 9 rewrites;
  `internal/reedcli/up.go`'s config-key wording about adopting a key;
  `doc.go`'s session-name-rewrite bullet, whose "each adopt the other's panes" clause describes two worktrees colliding on one session name rather than `planPaneTarget`;
  `spawn.go`'s own surviving hit, the `e.adoptPaneGenerationLocked(st)` call inside `loadOrInitStateLocked`, which cards 4 and 6 leave standing because it is the generation probe rather than pane adoption;
  and `tools/sandbox/SANDBOX-FABRIC-SUITE.md`'s merge-adoption prose, which belongs to fabric and not to reed.
  The second sweep has three legitimate survivors of its own, all still true post-fix and all deliberately preserved by earlier cards;
  expect them, and do not report them as missed rewrites.
  `reconcile_test.go`'s `DeadHeaderPaneKeptNotKilled` comment, reading "the dead-pane kill loop, not only the untracked reap, spares it" — it describes the header's exemption from the dead-pane kill, which this task does not change, and card 2 preserves that case unchanged.
  `reconcile.go`'s header-exemption comment above the dead-pane kill loop, reading "exempt from the dead-pane kill too, not only from the untracked reap below" — same rule, same reason, and card 2 changes neither the loop nor this comment.
  And `reconcile.go`'s `clearConflictingPaneBindings` doc comment, where "planReconcile's deterministic untracked reap kills it" describes what happens to a second strand's orphaned pane under a corrupt table — card 10 rewrites only that comment's adoption clause, and this sentence stays true because the reap still kills such a pane.
  Anything else — any surviving text describing `planPaneTarget`'s pane-adoption seam, the initial-pane-adoption behaviour, or the reap gate needing a bound present pane — is a missed rewrite.
  Close one further enumeration item the two greps cannot reach, because its comment contains neither sweep term: `internal/reedengine/lifecycle_test.go`'s fixture comment describing a pane as "the new-session initial pane a fresh boot leaves".
  Read it and confirm it merely labels a scripted fixture's pane set rather than asserting that the initial pane is adopted or survives an `up`;
  it is expected to stand as written, and the disposition to record is "confirmed out".
  Report it as a missed rewrite only if it does make either claim.
  Report each such hit with its file, line, and the batch card that should have covered it, rather than fixing it silently here, so the gap is visible in review.
  Report the disposition of every hit in the round's output.
  If both sweeps come back clean apart from the legitimate survivors above, state that explicitly.
- **Commit:** none

## Batch Tests

`verify: go test ./internal/reedengine/` runs the package's whole untagged suite.
This batch is prose-only in its four editing cards, so there is no new assertion to name;
the gate exists to prove the comment rewrites did not disturb the code they sit against — three of the four edited Go files (`reconcile.go`, `strand.go`, `lifecycle.go`) carry live logic in the same functions whose comments change, and a mis-scoped edit that swallowed a line of code would otherwise reach batch 4 undetected.

The unbounded-within-the-package scope is deliberate and cheap: the package's untagged suite is pure by the Test Tier Purity Invariant (no `exec.Command`, no `hubforge.NewHub`, no sleeps at or above one second), so it runs in seconds, and a batch touching six files across the package has no narrower honest scope.

Card 12 has no automated gate by construction — it is a grep-and-judge card whose finding is prose, and both sweeps it runs are recorded in its `Requirements:` so a reviewer can re-run them verbatim.
`tools/sandbox/SANDBOX-REED-SUITE.md` has no runnable surface at all;
it is graded by the sandbox operator, which is exactly why card 11 must land in the same task as the behaviour change.
