Affected-set list:

- Board owner fields and payloads: `Task` decoding, unknown-key preservation, `Store.Load`/`Save`, replay, run-lock comparison, upsert/batch/merge, board list/get, and README rendering are covered by cards 2–4.
- Reconcile owner outcome: production callers in batten, loom landing, hibernate, orch, and cleanup all obtain options through `HubSeamOptions`, directly or through `cleanupSeamOptions`; cards 5–6 cover the shared seam, and cards 8–13 cover outcome handling.
- Owner pause label: tender pause creation, pause coverage, InnerRun lock handling, hibernate’s fake pause implementation, and adoption’s selective lift are covered by cards 7, 10–11, 13, and 16.
- Batten claim and resume behavior: arm/pre-run/step/window paths, the claim closure, and existing claim/window tests are covered by card 8.
- Tender owner status: `StatusReport`, `Status`, hub dependency construction, CLI help, existing `TestStatus`, and overview documentation are covered by card 14.
- Open-Window resume wording: all three existing statements in batten CLI help, battenshed documentation, and `docs/overview.md` are covered by card 15.
- Adopt registration and destructive operations: root registration, sandbox exclusion, destructive executor/result registries, and fixture construction are covered by card 16.
- Orch owner-pause installation: both producer and consumer are named by card 13, but their construction/execution order leaves the consumer unwired.

Finding:

**BLOCKING — design — `_lyx/plan/13-wake-and-orch-owner-pause.md:36`**

**The orch command captures the owner-pause callback before the root pre-run installs it.**

Card 13 prescribes installation in `newRoot`’s pre-run and copying the installed callback onto `orchCLI.pauseOwned` in `Command`.
However, `newRoot` calls `orchcli.Command()` while constructing the command tree, before any pre-run executes.
On a fresh process, `Command` therefore copies nil.
Installing the package callback later does not update the receiver’s copied function field.
Consequently, `lyx orch start` always takes the prescribed unwired warning path instead of pausing another machine’s local run, contrary to D4 and the declared `PATTERN-owner-machine`.
The fake receiver tests proposed at line 44 do not exercise this installation lifecycle.

Evidence: `cmd/lyx/main.go:119` declares the runtime pre-run; `cmd/lyx/main.go:147–161` constructs the module commands immediately; `internal/orchcli/cli.go:100` constructs the receiver; card 13 lines 36–38 prescribe the copy and subsequent use.

Proposed correction: keep the existing targets, but change the intent so `Command` defers reading the installed callback until its module pre-run or invocation, after the root pre-run has installed it.
Exercise construction followed by installation followed by invocation in the card’s coverage.

Tool-call manifest:

1. Searched reconcile callers, changed window wording, board readers, and batten entry points because the inherited manifest lacked their complete reader sets.
2. Read cleanup seam extension, README golden coverage, tree-sitter registry, and batten pre-run/window bodies to verify indirect consumers.
3. Read rendering, task JSON handling, replay fields, and store payload validation to follow the owner fields’ serialization and display paths.
4. Read board constructors/wrappers and batten resume reconciliation to check parent resolver propagation and affected resume callers.
5. Searched hostname readers, Bolt APIs, pause/status consumers, loom guards, reed client readers, and store shape for existing competing implementations.
6. Read legacy conversion, Bolt write/push behavior, and reconcile movement; searched hibernate and fixture consumers to check persistent-state propagation.
7. Read `decodedEntry` and migration handling; searched tender/hibernate pause callers and claim tests to resolve decoding and fake compatibility.
8. Read root registration and tender status tests; searched PATTERN consumers and board constructors to check module and status integration.
9. Read board get/text paths and numbered card owner-view rules to verify output consumers and obtain precise plan references.
10. Searched production import edges and read numbered root wiring plus hibernate wake handling to verify callback lifecycle and caller coverage.
11. Read board configuration and skip-environment handling to check the added callback’s configuration consumers and setter context.
