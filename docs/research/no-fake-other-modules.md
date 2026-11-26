# Fakes of other modules: the rule and the inventory

Task `no-fake-other-modules`, 2026-11-26, read from the tree at `248543129`.
The rule is [PATTERN-test-stand-ins](../../pattern/PATTERN-test-stand-ins.md).

## Findings

- Fakes of other modules are the normal way to test across modules here, in nearly every area.
  Most inventory rows read `real`.
- `testkit/shuttlefake` is not a stand-in for an LLM session.
  Its `Engine` replaces `claudeengine` and `codexengine`, and its `Reed` replaces `reedengine`, both repository modules.
  The rule for outside things therefore does not cover it.
  The LLM stand-in the rule asks for sits below the engines and tmux, plays recordings, and does not exist yet.
- No `tier3` test compares a live session with a recording.
  `TestSmokeTurnsHookCapture` can record, and a corpus case is promoted from its output by hand.
- Much of what the engines' tests read as provider output is hand-written or names no source.
  [The shuttle inventory](no-fake-other-modules/shuttle-reed.md) lists which.
- Several formats have no writer a test could drive, so their fakes cannot be replaced yet.
  `planparser` has no plan writer, and `testkit/plankit` renders its format by hand.
- The rule moves many tests up a tier.
  Real git, fabric and hubs are `tier1`; a real reed is `tier2` until a tmux stand-in can be used from above `reedengine`.
  The gate cost of that was not measured.

## Conflicts settled

| Stood | Now |
|---|---|
| `PATTERN-test-speed`: a behaviour testable with a fake or an in-memory fixture is tested at `tier0` that way | The lowest tier the real modules run in. The module's own fixtures and outside stand-ins keep `tier0`, and another module is never faked to lower a tier. |
| `PATTERN-test-isolation`: kits hold fakes, such as `shedfake` | Kits hold builders, fixtures, the scan harness and outside stand-ins. A kit that fakes a repository module lasts until a sweep removes it. |

## Verdicts

Each per-area file gives every stand-in found one verdict.

- `real`: the real module is driven instead.
- `goes`: the stand-in and its cases are removed, or the cases move to the owner's tests.
- `own`: the module's own state or seam; it stays.
- `external`: it stands for something outside the repository; it stays and still owes recordings or a contract test.

| Area | Inventory |
|---|---|
| Webster, shed, plan and review | [webster-shed.md](no-fake-other-modules/webster-shed.md) |
| Loom, landing and drift | [loom-landing.md](no-fake-other-modules/loom-landing.md) |
| Shuttle, reed and sessions | [shuttle-reed.md](no-fake-other-modules/shuttle-reed.md) |
| Creel, orch, Slack, dobby and board | [mail-board.md](no-fake-other-modules/mail-board.md) |
| Fabric, git, hub and batten | [fabric-batten.md](no-fake-other-modules/fabric-batten.md) |
| Gate, config, report, tools and the rest | [other-packages.md](no-fake-other-modules/other-packages.md) |

## The shared kits

| Kit | Stands for | Verdict |
|---|---|---|
| `shuttlefake` | Provider engines and `reedengine` | `real`; the golden `TokencountRunReads` moves to `claudeengine` |
| `shedfake` | The shuttle, burler, seat, merge and webster run seams | The fakes go; `CallOK` and `RequireOutcome` stay |
| `creelfake` | `creelcli`'s delivery, hub directory and Slack bridge | `real`; its `Clock` stays |
| `prkit` | `pullrequest.API` | `goes`: callers drive the real client over recorded GitHub answers |
| `indexkit` | `planglyph` | `goes` |
| `takeoverkit` | The takeover write | `goes`: tests call the production take-over |
| `envkit` | Every seam of the recipe `Env` and landing deps | `real`; `NilSeams` stays |
| `boardkit` | `WriteBoard` frames `board.json` itself | `WriteBoard` writes through `boardengine`; the rest stays |
| `locationkit` | A resolved `lymcwd.Location` | `real`, see the open questions |
| `plankit` | `planparser`'s plan format | `real` once `planparser` has a writer |
| `hubforge` | `ReanchorPair`, `NewRecordsStopError` and `SeedConfig`'s bytes | `real`; the hub fixtures already are |

## Sweep order

The sweep is follow-up work, in this order, since each step unblocks the next.

1. Give owners the writers the tests lack: plans, parts files, run status, board entries, summaries.
2. Build one stand-in per outside boundary, with recordings and contract tests: the LLM session, tmux, GitHub, Slack
   and the `go` toolchain.
3. Replace the swaps that stay in `tier0`: records and config through their owners' writers.
4. Remove the kits that fake modules, area by area, with the LLM and tmux stand-in under them.
5. Move what needs git or a hub to `tier1`, watching each key's gate time.

## Open questions for the operator

Each is a point where the rule as written costs much or cannot be met.
The rule stays strict on all of them until the operator says otherwise.

- **Values fed to pure functions.**
  A literal `lymcwd.Location`, `shedengine.Status` or reconcile report handed to a pure decision is another module's output.
  Producing each for real moves a large share of `tier0` to `tier1`, and some shapes cannot be provoked.
- **Failures nothing can provoke.**
  Error rows that only an injected failure reaches are removed under the rule.
- **Retired formats.**
  A migration test needs a file today's writer cannot produce.
- **Version skew.**
  `stencilstore`'s newer-default cases need a registry the real one cannot be.
- **Tick-by-tick behaviour.**
  `shuttleengine`'s wait loop depends on what reed and the pane show at each tick, which real tmux does not repeat.
- **Ports that exist to avoid an import.**
  `pawlengine` and `tenderengine` rule tests become CLI tests when their seams are filled for real.
- **A tmux stand-in from above.**
  `reedengine`'s tmux seam is unexported, so a package above it stays `tier2` until reed exports one or a kit holds it.

## What the rule admits

The inventory raised cases the operator's decision does not spell out.
The PATTERN admits these, and each is open to the operator's veto.

- A scripted implementation of an interface many modules fill, in the declaring module's tests.
- A value or file the code under test never interprets.
- A clock, a sleeper, a random or id source, and a recording wrapper around the real module.
- A config file written by `configengine.Set` over its owner's template.
- Removing another module's file, or making it unreadable.
- An agent-written file hand-written in its own parser's tests.

"Module" is read as a Go package, so an engine's CLI package may not fake its engine.

## Method and limits

One reader per area classified from searches and excerpts, not from whole files, so small literals may be missing.
Only spot checks were made across the areas.
No test was run and no replacement was tried, so "what replaces it" is a proposal, import cycles unchecked.
The "Hard cases" closing each per-area file are its reader's own notes; the two sections above say where each kind ended.
