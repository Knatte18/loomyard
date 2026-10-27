---
verdict: BLOCKING
findings:
  - id: F1
    severity: BLOCKING
    class: consistency
    origin: lens:affected-set
    location: _lyx/plan/16-adopt-module.md:52
    summary: The engine-less adopt module lacks its required CLI architecture exception
    fix: Insert a Prosa prerequisite card with lyx loom insert-card before adopt-module, targeting pattern/PATTERN-cli-cobra.md and adding adoptcli to its Package naming deviations as an engine-less composition of the owning modules, consistent with D5.
  - id: F2
    severity: BLOCKING
    class: design
    origin: handler
    location: _lyx/plan/03-board-owner-writes.md:51
    summary: The deferred-push test must wait out the production retry duration
    fix: Amend card 3's Intent so OwnerRequest carries an optional injected retry sleeper, SetOwners defaults it to the production one-second wait, and TestSetOwners supplies a nonblocking sleeper while holding the push lock across both attempts and asserting the retry count and refusal.
  - id: F3
    severity: MEDIUM
    class: consistency
    origin: handler
    location: _lyx/plan/03-board-owner-writes.md:46
    summary: The standalone owner setter has no route to the pair-parent resolver
    fix: Amend card 3's Intent to carry ParentOf on OwnerRequest and apply it to the directly loaded store before the non-takeover effective-owner check; amend card 8's hubOwnerSeams wiring to pass its ParentOf into SetOwners requests, and extend the already planned TestSetOwners with an unowned child whose pair parent and prime have different owners, testing both the permitted parent-owner claim and the refused prime-owner claim.
  - id: F4
    severity: MEDIUM
    class: consistency
    origin: lens:guard-reach
    location: _lyx/plan/13-wake-and-orch-owner-pause.md:36
    summary: The owner-pause callback can be copied before the root hook installs it
    fix: Amend card 13's Intent to copy the installed callback in orch's PersistentPreRunE after the root hook, rather than during command construction, and extend its existing TestStart_ReconcilesTheHubFirst target to cover construction before installation followed by runtime binding and the owned-elsewhere pause.
  - id: F5
    severity: MEDIUM
    class: consistency
    origin: lens:way-forward
    location: _lyx/plan/09-loom-owner-check.md:31
    summary: The shed owner verdict loses the offline provenance needed for D3's recovery message
    fix: Amend card 9's Intent so ownerVerdict also carries Unreachable, runOwnerVerdict sets it on the local-Bolt fallback, and refuseOwnedElsewhere names the local owner and rerunning the requested verb once origin answers for that case; extend its already planned start/resume refusal table to assert this message while retaining the owned_elsewhere envelope kind.
  - id: F6
    severity: MEDIUM
    class: design
    origin: handler
    location: _lyx/plan/16-adopt-module.md:16
    summary: The adopt card bundles independent public surfaces from three other packages
    fix: Use lyx loom insert-card to split ResumeInWindow, DropLocal and CloneSecond into independently reviewable prerequisite cards before adopt-module, carrying each surface's supporting declarations and coverage with it; keep DropLocalResult, TestDropLocal and the destruction registrations together with DropLocal, and retain adopt's orchestration and integration coverage on adopt-module.
---

### [BLOCKING] Register adopt's architecture exception

**Location:** _lyx/plan/16-adopt-module.md:52

**Issue:** Card 16 deliberately introduces an engine-less CLI, as D5 requires, but no card updates the authoritative package-naming deviations.
`pattern/PATTERN-cli-cobra.md:27` requires a module CLI to import its corresponding engine, and lines 28–35 enumerate the exceptions, including cleanup's equivalent composition.
Adopt must be recorded there rather than acquiring an engine contrary to D5.
This is a `PATTERN-cli-cobra` fit failure, not a request to change the decided architecture.

### [BLOCKING] Inject the owner-write retry wait

**Location:** _lyx/plan/03-board-owner-writes.md:51

**Issue:** Card 3 prescribes a retry after one second and a `TestSetOwners` case holding the push lock across that retry at line 58.
Its specified request fields and setter signature provide no timing injection, so the prescribed test exercises the real wait.
`pattern/PATTERN-test-speed.md`, under Time is injectable, requires a struct field or parameter for a duration a test would otherwise wait out and forbids production-duration waits in every tier.
The integration tag does not exempt this case.
By comparison, card 10 explicitly uses the existing fake clock, whose `Sleep` in `internal/battenshed/innerrun_test.go:61` advances time without blocking.

### [MEDIUM] Give SetOwners the same parent geometry as its callers

**Location:** _lyx/plan/03-board-owner-writes.md:46

**Issue:** Card 3 wires `Config.ParentOf` through `Board` to stores constructed by the Board methods, but `SetOwners(boardDir, req)` explicitly loads its store directly.
The listed OwnerRequest fields contain no parent resolver, and the same card specifies that a nil resolver treats every parent as the prime.
`internal/boardengine/board.go:55` confirms that the Board critical section is a distinct construction path; the standalone setter does not inherit its fields.
For an unowned child of a pair owned by machine B under a prime owned by machine A, the start's correctly wired owner read permits B, but the setter's fallback rejects B as though A owned the child.
It also fails the card's promised non-takeover protection when the setter is asked directly to claim as A.
D2's pair-parent fallback and card 8's preflight must agree with the write-time check.

### [MEDIUM] Bind the orch callback after installation

**Location:** _lyx/plan/13-wake-and-orch-owner-pause.md:36

**Issue:** The card installs the process callback in the root pre-run but says Command copies it into the receiver.
`cmd/lyx/main.go:161` calls `orchcli.Command()` while constructing the command tree, before the root pre-run at line 119 executes.
A construction-time copy therefore retains nil in a fresh process and takes the card's unwired-warning path instead of D4's owner pause.
The existing orch runtime wiring lives inside its persistent pre-run, including the reconcile assignment at `internal/orchcli/cli.go:195`; that is the appropriate binding phase.
The planned direct fake-pause cases do not exercise installation order.

### [MEDIUM] Preserve the offline refusal's recovery route

**Location:** _lyx/plan/09-loom-owner-check.md:31

**Issue:** Card 9's result contains only Owner and Elsewhere, so its local fallback after an unreachable reconcile is indistinguishable from a fresh origin verdict.
Its single foreign-owner message then recommends adopt, although card 16 refuses adoption while origin is unreachable.
D3 explicitly requires the local-foreign-owner case to name the owner and a rerun once origin answers.
Card 8 carries that distinction for batten, but card 9 drops it for the shed resume verbs.
The proposed test already reaches both verbs before bootstrap, so this can be covered within its existing table.

### [MEDIUM] Separate adopt's independent public prerequisites

**Location:** _lyx/plan/16-adopt-module.md:16

**Issue:** The card creates the adopt command together with an exported batten resume entry point, a destructive fabric API with its own result and tests, and a second-clone fixture builder.
These are independently reviewable public surfaces in other packages, not private helpers inseparable from adopt's implementation.
The rubric's granularity rule explicitly gives cross-package public surfaces their own cards even when only one consumer needs them.
DropLocal's existing planned TestDropLocal and destruction-registration changes make its separate unit particularly concrete.
The current order can be preserved by placing these prerequisites immediately before adopt.

## Rejected

- A dependency on later-created owner symbols was not established: the owner records, setter, reader, pause label, child-refusal kind and pause function are introduced before their consumers; InstallOwnerPause and its root wiring are on the same card.
- The Takeover switch, Finalize's past-merge exemption and nil owner seams have stated behavior and bounds; their mere existence is not an additional attack-surface finding.
- The preceding pause checks do not make the owner-refusal tests unreachable: an owned-elsewhere run need not already have a covering pause.
- Cards 7 and 14 extend existing pause/status tests, and card 10 adds ownership-trigger behavior absent from the existing parent-lock cases; no redundant-test finding was established for those additions.
- Card 17's shortened PATTERN overview line and its trailer are licensed by the explicit Added after Discussion decision, provided the full declared rule remains verbatim in the background and other entry cuts preserve their rules.
- The inherited Task decode shapes use a Task-derived alias, so adding OwnerMachine does not require separately rewriting decodedEntry or storedEntryFrom.

The review used file reads only.
The instruction to run nothing prevented the requested board command; no board content was inferred or used as an alternative answer key.
No target-changing overlay validation was run: the fixes above change Intent prose or insert/split cards, for which the rubric expressly excludes overlays.
All three fork reports were received before this consolidated file was written.
