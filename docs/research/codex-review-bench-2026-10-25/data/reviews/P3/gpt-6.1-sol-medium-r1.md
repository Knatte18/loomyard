---
verdict: BLOCKING
findings:
  - id: F1
    severity: BLOCKING
    class: consistency
    location: _lyx/plan/06-reed-reclaim-dead-name.md:32
    summary: Reclaim silently exempts dead holders with children without a recorded decision
    fix: In card 6, remove the children condition from the successful no-op cases and the childless qualification from ImpactSummary. Attempt the specified ordinary nonrecursive removal for an otherwise eligible holder and propagate its existing children refusal; change the planned child-holder test row to expect that refusal and no new strand, rather than successful numbering. Do not introduce recursive teardown.
  - id: F2
    severity: MEDIUM
    class: design
    location: _lyx/plan/10-orch-note-gate-no-reply.md:62
    summary: The note-request test still prescribes assertions on static stencil prose
    fix: In card 10's TestWatcher_NoteRequestRendersTemplateFileBeforeTyping instructions, remove the new assertion that the hard request contains no --reply and remove the retained assertion of the soft stencil's literal defer command. Keep assertions on the rendered note and template paths and template-file existence; cover no-reply progression and defer behavior through the already planned watcher gate tests.
  - id: F3
    severity: MEDIUM
    class: scope
    location: _lyx/plan/00-overview.md:14
    summary: Two changed packages have no card carrying the required package documentation
    fix: Use lyx loom insert-card to append a documentation card after the current card 13, with a Prosa group containing only internal/loomcli/doc.go and internal/shuttleengine/claudeengine/doc.go. Its Intent must document landing's removal of present shell-only conflict strands in loomcli and the engine-or-exec requirement for the binary configuration in claudeengine. Include no prose assertions or new tests.
---

### [BLOCKING] Dead non-leaf holders silently retain the canonical name

**Location:** _lyx/plan/06-reed-reclaim-dead-name.md:32

**Issue:** D6 identifies live holders, hidden holders and paused pairs as the cases that keep a requested name. Card 6 adds a successful no-op for a dead holder with children, explicitly tests it, and restricts its impact to childless holders. Consequently a role add can succeed under a numbered name even though the canonical holder meets D6's reclaim conditions. The existing `removeStrandLocked` in `internal/reedengine/strand.go` already refuses nonrecursive removal of a non-leaf; that refusal does not justify silently falling through to numbering. The decision record contains no amendment licensing this extra successful fallback.

### [MEDIUM] Static handoff wording is still pinned by tests

**Location:** _lyx/plan/10-orch-note-gate-no-reply.md:62

**Issue:** Card 10 removes obsolete wording assertions but explicitly adds a negative assertion for `--reply` and keeps the literal soft-request `lyx creel post --reply <id> defer` assertion. These are fixed stencil text, not Go-filled values or a family-wide rule. `PATTERN-test-economy` and the rubric forbid such assertions. The existing `TestWatcher_NoteRequestRendersTemplateFileBeforeTyping` can continue checking the paths Go fills and file-rendering order, while the planned `TestWatcher_NoteReplyGate` and existing soft-defer behavior test verify the actual cycle contract.

### [MEDIUM] Package documentation coverage is incomplete

**Location:** _lyx/plan/00-overview.md:14

**Issue:** The decision record's in-scope documentation explicitly includes the `doc.go` of each changed package. Card 4 changes loomcli's conflict-stop behavior but carries neither its package documentation nor a later documentation update. Card 3 changes claudeengine's binary template comment, but its package documentation also has no owning card. Both files exist: loomcli's package overview describes its lifecycle, and claudeengine's configuration paragraph lists `binary`. Other changed packages have their documentation carried explicitly, so these two omissions leave the settled documentation scope incomplete.

## Rejected

- The focused order concern is not a defect: card 1 supplies both pane command fields before card 3; card 2 supplies the age probe before card 3; card 5 supplies Elsewhere before card 13; and card 8 supplies WithdrawCycleRequest before card 12. The status work in cards 12 and 13 follows those prerequisites.
- Cards 6, 12 and 13 state their mutation bounds: exact-name reclamation with session-limited pane removal, marker-only withdrawal without changing a started cycle, and watcher restart only for a live tracked strand under start.lock with watch.lock arbitrating duplicate spawns.
- The principal focused refusal/skip branches have concrete test inputs: present shell-only versus absent conflict strands in card 4, live/hidden/paused/grace holders in card 6, an empty .git directory and teardown during wire/status in card 7, marker replacement during Hold in card 8, and withdraw/clear failures in card 11. ResolveWorktree has no acting-cwd gate, and the existing TestHubDelivery already reaches the empty-.git-directory resolve failure, so that skip fixture is reachable.
- Source implementation edits name member glyphs; the specified directoryRows and planReconcile signature changes carry re-sign arrows. No card carries a Verify line naming a raw test command.
