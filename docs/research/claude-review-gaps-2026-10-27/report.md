# What the Claude lens fan misses that Codex Astra finds

Desk study of 2026-10-27 over the Astra benches of that day on the plans `P1` (`gate-key-shape`, the tier-tag and entry-gate plan) and `P4` (`machine-handover`).
No reviewer ran: the sources are the benches' `report.md` and `data/` files, the Claude fan's round-1 plan reviews in `loomyard-weft`, and the review-plan stencils at HEAD and at the two code commits of `docs/research/codex-review-bench-2026-10-25/data/inputs.txt`.
Nothing was built, benched or edited; every proposal below is a hypothesis tied to named findings.

## Conclusion

The Astra benches hold 28 distinct `unique-real` defects on the two plans, none graded false: 18 on `P4` and 10 on `P1`.
Nine are `BLOCKING`, 15 `MEDIUM`, 4 `LOW`.
They fall into six causes, and the first four account for 23 of the 28.

| Group | Cause | Defects | Lens that owns the kind | Does the lens text ask for the reading? |
|---|---|---|---|---|
| G1 | A guard or seam is not reached by every entry point, or is copied before it is installed | 6 | `guard-reach` | in part: it asks for recurrence, not for a caller search or an install-time check |
| G2 | A prescribed check does not do what the card says on the input it meets | 4 | `guard-reach`, rubric "Reachability of a prescribed refusal" | in part: it asks whether the state can occur, not for the input's path through earlier checks |
| G3 | A reader, renderer, inventory or state switch outside the card is left wrong | 7 | `affected-set` | in part: it names tests, callers, docs and fixtures only |
| G4 | A way forward names a step that cannot succeed, or a message that contradicts it | 6 | `way-forward` | yes, but not the named command's own resolution or the final message text |
| G5 | Two cards or a card and its own record disagree on the same input | 3 | none | no |
| G6 | A rubric rule is applied to the cards the focus names and not to all | 2 | handler | the rubric asks; nothing makes the sweep complete |

- Astra's finders are the Claude fan's own lens names: 24 of the 28 defects are labelled a fork finding in at least one bench, from `affected-set`, `guard-reach` or `way-forward`; four are a handler's own (`P4-14`, `P4-15`, `P1-05`, `P1-06`).
  The lens set does not need a new lens; the gaps are in what a lens's procedure makes a fork do.
- The Claude fan does not look short of budget: its round-1 review used 489k fresh tokens on `P4` and 384k on `P1`, of which the three forks used 79k and 92k, about 26k and 31k each.
  The tuned Astra forks used 26k to 42k each on `P1` and 107k to 121k each on `P4`, and the tuned `P4` handler 286k.
  The measures differ (Opus counts cache creation as fresh input; Astra counts input less cache hits, and its forks' inherited prefix was only partly cached), so the figures show that the gap is not plainly one of spend.
- Nine of the 28 are `BLOCKING`, and no Claude finding names the same defect as any of them.
  The nearest is `P1`'s `F2`, a method finding about hand-enumerated readers that names other members than `P1-01`.
  The six groups therefore matter for the verdict and not only for the finding count.
- Six proposals follow, one per group, each as text for a named stencil.
  All six suit the Codex prompt as written, since the Astra fork prompts carry the same lens text, boilerplate and holistic paragraph.
  A later bench decides whether any of them catches what it targets.

## Population and method

A row is in the population when a `data/match.tsv` of the eight benches `codex-astra-*-2026-10-27` labels it `unique-real`; the solo bench of 2026-10-25 is outside the set the task names.
A defect found by several benches counts once, under its highest bench severity.
Two rows are dropped findings the handler left out; both are counted, since the lens reported them.
The ids are `P4-nn` and `P1-nn`, ordered by group.

Benches: `fan` (cold-call fan), `fork` (forked fan), `ctx` (context parity), `solo-h` (solo high), `tuned` (tuned prompt on `P4`), `p4m` (tuned, medium, `P4`), `p1h` and `p1m` (tuned, `P1`, high and medium).

| Id | Plan | Severity | Defect | Benches | Group |
|---|---|---|---|---|---|
| `P4-01` | `P4` | `BLOCKING` | card 13 copies the owner-pause seam in `orchcli.Command` at construction, before the root pre-run installs it | fan, fork, ctx, tuned, p4m | G1 |
| `P4-02` | `P4` | `BLOCKING` | card 3 lets `BOARD_SKIP_GIT` and `BOARD_SKIP_PUSH` return an unpushed owner write as recorded | fan, fork, ctx, tuned, solo-h | G1 |
| `P4-03` | `P4` | `MEDIUM` | the `Takeover` guard covers slugs only, so a prime claim replaces an owner the setter's pull found | fan, fork, ctx | G1 |
| `P4-04` | `P4` | `MEDIUM` | `Finalize.Call` runs `holdParent`, which fetches the parent tips, before the owner checks, so an offline run never reaches them | fan, fork, tuned | G1 |
| `P4-05` | `P4` | `MEDIUM` | `Fabric.settleUnmovedWeft` replays on pull and the push after it never meets `retryWeftPush` | fan | G1 |
| `P4-17` | `P4` | `BLOCKING` | card 9 guards `loom start` and `resume` only; `loomPreRun` and `loomPreStepUnmarked` resume a seeded run with no owner check | tuned | G1 |
| `P4-07` | `P4` | `LOW` | card 10's one-minute input never fires: `SleepWatch.Observe` records only a gap above the tolerance, and a test pins equality as no sleep | fan, tuned | G2 |
| `P1-02` | `P1` | `BLOCKING` | card 24's verify regex `go test[^&;\|]*-tags[ =][^ ]*tier0` accepts `-tags nottier0` | p1h | G2 |
| `P1-07` | `P1` | `BLOCKING` | card 1's retired-list refusal sits in `decodeConfig`, but `configengine.Load` runs first and its `FillMissing` rejects the old shape | p1m | G2 |
| `P1-08` | `P1` | `MEDIUM` | card 15 passes the caller's args to the exemption, and the guard run appends its `-run` to a clone of them, so a caller `-run X` exempts a guard run | p1m | G2 |
| `P4-09` | `P4` | `LOW` | card 16 adds `adoptcli` to no `RunCLI` list of `cmd/lyx/seamsignature_test.go`; the lists already omit others | fan (dropped) | G3 |
| `P4-12` | `P4` | `MEDIUM` | `list --text` goes through `RenderCompact`, which drops `owner_machine`, and the new nil seams are in no `envkit.nilLegal` entry | fork, ctx | G3 |
| `P4-18` | `P4` | `MEDIUM` | battencli integration callers call `wire` on an unarmed receiver, then the real `CreateWorktree` closure, which card 8 makes call a nil owner setter | tuned | G3 |
| `P1-01` | `P1` | `BLOCKING` | `cmd/lyx/treesitterlink_integration_test.go` lists the old tag spelling and `tmuxkit/allow_tmux.go` is built under `tmux \|\| llm` only; card 11 omits the one and card 24 retags only its header | p1h, p1m | G3 |
| `P1-04` | `P1` | `BLOCKING` | card 20 retires the switch key but `commandRefWayForward` still tells the operator to set it, and `TestCommandEntry` pins that text | p1h | G3 |
| `P1-09` | `P1` | `MEDIUM` | `latestDarnOutcome` excludes only `OutcomeReroute`, so a skipped `Darn-Write` is returned as the writer's outcome | p1m | G3 |
| `P1-10` | `P1` | `MEDIUM` | `rerouteCount` and batten's failure episode close at `Done`, not at a skip, against the decision record | p1m (dropped) | G3 |
| `P4-06` | `P4` | `MEDIUM` | the adopter-first repair is appended to a push error whose own text says to merge origin and push | fan, ctx | G4 |
| `P4-08` | `P4` | `LOW` | card 16 refuses both local-choice flags and an empty selection with no correction named | fan | G4 |
| `P4-10` | `P4` | `MEDIUM` | card 16 plans no network-gate exemption, so adopt's first reconcile waits behind a standing hub pause | fork, ctx | G4 |
| `P4-11` | `P4` | `MEDIUM` | card 12's repair names the task's squash by the parent head's SHA, which a later local commit makes another commit | ctx | G4 |
| `P4-16` | `P4` | `MEDIUM` | card 9's `ownerVerdict` drops the unreachable-origin provenance, so the fallback gives the unconditional adopt message | p4m | G4 |
| `P1-03` | `P1` | `BLOCKING` | card 19 sends a default-layer value to `lyx config <module>` from the prime, but `selectedLayer` picks the prime worktree's file for a module that is not hub-wide | p1h, p1m | G4 |
| `P4-13` | `P4` | `MEDIUM` | card 3 puts an absent slug in `Lost` while card 8 expects an absent entry to pass | fork, solo-h, tuned | G5 |
| `P4-14` | `P4` | `LOW` | card 16 spawns `lyx loom resume` while its opening paragraph decides in-process calls | fork | G5 |
| `P4-15` | `P4` | `MEDIUM` | card 3's `SetOwners` carries no `ParentOf`, so a nil resolver reads every parent as the prime and disagrees with card 8's owner read | p4m | G5 |
| `P1-05` | `P1` | `BLOCKING` | card 20's `TestWireSetting_OpenTerminalForcesOpenWindow` repeats a case an existing test covers after the card's own migration; arguable, the solo Opus bench graded it `LOW` | p1h | G6 |
| `P1-06` | `P1` | `MEDIUM` | cards 10 and 19 bundle exported cross-package symbols into feature cards against the rubric's Granularity rule; arguable, each has one consumer | p1h | G6 |

## Evidence and its limits

- The Claude round-1 reviews are the weft files `_lyx/reviews/plan/round-1-review.md` at the commits `c36025a89` (`P1`) and `045707759` (`P4`).
  The `P1` review has 12 findings and the `P4` review 11; the `origin:` key tags 7 of each as a lens finding and the rest as the handler's.
- The forks' final messages and the handler's reading manifest are not in the weft history for these rounds, so what a Claude fork read is unknown.
  A group whose cause is reading depth is therefore an inference from the lens text and the review text; the groups say so.
  The stencil at HEAD now saves each fork's report to a lens-report file (`burler-cluster-fan.md`), so a later round can answer the question.
- The review-plan lens texts, `affected-set`, `guard-reach` and `way-forward`, are identical at both code commits and at HEAD.
  The fork boilerplate, the handler's holistic paragraph and the reading block are identical between the two code commits, and at HEAD differ only by the `lyx` to `lym` rename and the lens-report paragraph; the plan rubric differs between the commits in its `Extends` lines and its source-file wording.
- The Astra benches ran one run per cell, graded by hand by one grader; a group built on one or two defects, such as G5 and G6, is a hypothesis.

## G1: entry points and install order

Defects: `P4-01`, `P4-02`, `P4-03`, `P4-04`, `P4-05`, `P4-17`.
The group holds three of the nine `BLOCKING` defects.

**Why Claude missed them.**
The `guard-reach` text asks the right question: "trace the sequence of states a real entry point passes through" and whether "the guarded site recurs elsewhere on a path the card does not guard".
At plan time the same lens did find two recurrences on `P4` (the `retryWeftPush` replay, and `ArmAt` building a receiver with no seams; `_lyx/plan/lens-review.md`), so the lens works when the search is made.
The round-1 review then closed three of this group's claims in its focus notes as "not a defect": `OwnerRequest.Takeover` "is bounded in card 3", and the nil-seam rules are "bounded to hand-built test receivers, with production wiring pinned by `TestLandingDeps_EveryFieldPopulated`, `newLoomCLI` and `Command`".
`P4-03` breaks the first claim: the bound covers slugs and not a prime claim.
`P4-01` concerns the `Command` wiring the second note accepts: `Command` is built before the pre-run that installs the seam it copies.
The review took the card's statement of the bound as the answer, and its text records no trace of the construction order in `main.go`.
`P4-02` is a plan-time lens fix: the lens review lists "card 3 reads `ApplySkipEnv`" as the repair of a test-row finding, and the round-1 review has no `BOARD_SKIP` finding.
The Astra `guard-reach` fork of the tuned bench found `P4-17` after reading "loom arming and the actual start/resume command entry points" (its tool call 2), a read the same six-call cap leaves to a Claude fork.

**Proposal 1, `internal/lenses/template.yaml`, lens `guard-reach`.**
Replace the lens's last two sentences with:

> A guard whose state cannot occur, and a guarded site that recurs unguarded, are findings. Find the recurrences by a search for the callers of the guarded function, and treat each command, hook and pre-run that calls it, and each environment variable or flag that turns the guarded step off, as an entry point. For a value a card copies or captures, say when it is assigned and when the copy runs, and check that the copy comes later. Your list: per guard, the entry points that reach the guarded site and which of them the guard covers.

Add to `contracts/stencils/burler/burler-fork-reading.md`, after the bullet on callers and callees:

> - for each guard, refusal or seam a card adds: the callers of the guarded function, the order in which the root constructs its commands and runs its pre-run, and the definition of every environment switch that turns the guarded step off;

It targets `P4-01`, `P4-02`, `P4-03`, `P4-04`, `P4-05` and `P4-17`.
**Codex:** the Astra fork prompts carry the lens text unchanged, so the lens edit reaches Codex when the prompts are rebuilt; the reading bullet is the same sentence as the tuned block's first clause, made specific.

## G2: a prescribed check on the input it meets

Defects: `P4-07`, `P1-02`, `P1-07`, `P1-08`.

**Why Claude missed them.**
The plan rubric has the bullet "Reachability of a prescribed refusal", which names this exact failure ("one whose named input an upstream check rejects; the finding names that upstream check"), and the `P1` focus directive asked for it for cards 2, 7, 15, 17 and 18.
The `P1` review's `## Rejected` section records focus checks on verify lines, card order and test economy and none on reach, so the reach check left no record either way.
`P1-07` needs a chain two hops past the card (`gateslot.LoadConfig` to `configengine.Load` to `FillMissing`, `config.go:234` and `fill.go:112`), and the fork boilerplate names "one hop beyond" as the limit of a tool call: "such as a caller or a spawned process one hop beyond it".
`P1-02` and `P4-07` need no reading beyond the card and the code it names, only a run of the card's own text on a boundary input (`nottier0`, a gap equal to the tolerance).
No Claude lens does that for a plan: the plan-writing fan has `row-reachability`, but the review-plan fan does not.

**Proposal 2, `internal/lenses/template.yaml`, lens `guard-reach`.**
Add before `Your list`:

> For each check a card prescribes in text, such as a regex, comparison, threshold, refusal or verify line, run it by hand on the nearest input that must not pass and the nearest that must, and follow the input of the card's test row through every loader, filler and validator that runs before the check, to the first that rejects, rewrites or drops it.

In the fork boilerplate (`burler-cluster-fan.md`), change "one hop beyond it" to "beyond it along the input's path".
It targets `P4-07`, `P1-02`, `P1-07` and `P1-08`.
**Codex:** yes, the same sentence; the tuned block's third clause covers tests that wait and not checks that miss.

## G3: readers, renderers and state switches outside the card

Defects: `P4-09`, `P4-12`, `P4-18`, `P1-01`, `P1-04`, `P1-09`, `P1-10`.

**Why Claude missed them.**
The `affected-set` text asks for "everything that reads or asserts it, tests, callers, docs and fixtures", and the Claude fork found a member set on both plans (`P1-F2` names six test and helper readers, `P4-F2` and `P4-F7` two docs).
The missed readers are of other kinds: a build tag spelled inside a `go list -tags` string in a test (`P1-01`); a compact renderer that copies fields one at a time (`P4-12`); an inventory that enumerates a type's nil-able fields (`P4-12`); a text pinned by a test of a retired key (`P1-04`); an existing function that switches on a state value the card adds (`P1-09`, `P1-10`); direct callers of a changed function that bypass the wiring (`P4-18`).
The rubric's "Finding class" files "a recurring enumeration gap ... once as a `design` finding about the method", and the lens has a card that covers part of a set be "one finding about the method, naming the members it missed"; the review lists the members the fork found, and nothing makes the search complete.
`P4-09` is not a miss: the Claude review rejected the same `seamsignature_test.go` pin (`## Rejected`), and the Astra handler called it marginal; it is counted because Astra reported it.

**Proposal 3, `internal/lenses/template.yaml`, lens `affected-set`.**
Add before `Your list`:

> Search each name a card retires, renames or adds in every spelling it takes: a key, a build tag, a flag, a string inside a command line or a regex. Include every function that renders, copies field by field, switches on or filters the changed type or state, and every table, allow-list or every-field test that enumerates its fields or values. Your list: per change, the searches you ran and the cards that cover the hits.

Extend the reading bullet of `burler-fork-reading.md` on outputs and keys to read: "every test, caller, doc, fixture, renderer, inventory and command line that reads, spells or asserts it".
It targets `P4-12`, `P4-18`, `P1-01`, `P1-04`, `P1-09` and `P1-10`.
**Codex:** yes; Astra `affected-set` forks reported six of the seven under the unchanged lens text, so the edit asks the Claude forks for the search those forks made.

## G4: ways forward

Defects: `P4-06`, `P4-08`, `P4-10`, `P4-11`, `P4-16`, `P1-03`.

**Why Claude missed them.**
`way-forward` asks for "the next step it names" and whether "that step can succeed from the state the refusal leaves", and the Claude fan reported six `way-forward` findings on the two plans (`P1`'s `F6`, `F7`, `F8` and `F9`; `P4`'s `F5` and `F10`).
The six missed ones need a read the text does not name: the named command's own resolution (`selectedLayer` and `hubwide` for `P1-03`; the network-gate hold for `P4-10`), the whole text the operator sees when a card appends to an existing error (`P4-06`), the state an earlier step drops before the message is built (`P4-16`), and a fact that a later commit falsifies (`P4-11`).
`P1-03` sits in the sentence of Claude's `P1-F6` and has a different defect; the Claude fix for `F6` keeps the very command the Astra finding shows to be wrong, "`lyx config <module>` from the prime".

**Proposal 4, `internal/lenses/template.yaml`, lens `way-forward`.**
Add before `Your list`:

> For each way forward, read the code of the command it names as it runs from the state the refusal leaves, including how it resolves a layer or scope and which gates or holds it waits behind. Read the whole text the operator sees, including any text the card appends to or takes from an existing error, and check that no sentence in it names a competing next step. Your list: per refusal, the final text, the next step, and the first precondition that fails or none.

It targets `P4-06`, `P4-08`, `P4-10`, `P4-11`, `P4-16` and `P1-03`.
**Codex:** yes, the same sentence.

## G5: two cards, or a card and its own record, disagree

Defects: `P4-13`, `P4-14`, `P4-15`.

**Why Claude missed them.**
None of the three review-plan lenses owns a comparison across cards.
The handler's holistic paragraph in `burler-cluster-fan.md` owns "what no lens owns, such as the target against its fasit and its board entry", which names a comparison against the record and not one between cards.
The Astra handler found two of the three itself (`P4-14`, `P4-15`), and `P4-13` first as a fork finding and then as the handler's own, so the sweep is within reach of the handler.
The Claude review has consistency findings of a neighbouring kind, a card against the declared text (`P4`'s `F6`, by the handler) and a card against the code it projects through (`P1`'s `F5`, by `guard-reach`).

**Proposal 5, `contracts/stencils/burler/burler-cluster-fan.md`, the handler's holistic paragraph.**
Append to the sentence that ends "reading further only where your review needs it":

> Include a cross-card pass: for each state, set or value that two or more cards read or write, such as an owner, a set like `Lost` or a resolver, list the cards and compare what each states for the same input, an absent entry and a nil seam included, and compare each card's opening paragraph with its steps; a pair that disagrees is a finding.

It targets `P4-13`, `P4-14` and `P4-15`.
**Codex:** yes; the Astra step-2 file carries the same holistic paragraph, so the sentence goes in the same place.

## G6: rubric rules applied to the cards the focus names

Defects: `P1-05`, `P1-06`.

**Why Claude missed them.**
The `P1` focus directive named test economy for "card 33 and any card planning a test" and granularity for no card, and the Claude review applied the rules to other cards (`P1`'s `F12` on card 7 for test economy, `P4`'s `F9` for granularity); the two Astra findings are on cards 10, 19 and 20, which the focus did not name.
Both are arguable by the bench's own grading, and `P1-05` is `LOW` in the solo Opus bench; this is the weakest group.

**Proposal 6, `contracts/stencils/burler/burler-cluster-fan.md`, the handler's holistic paragraph.**
Append:

> Before you judge the rubric's granularity and test-economy rules, write one line per card with its new exported symbols and its new tests, and judge from that table, so every card is judged and not only the cards the focus names.

It targets `P1-05` and `P1-06`.
**Codex:** yes, the same sentence in the same paragraph.

## Cross-cutting observations

- **The tool-call cap reads the same and spends differently.**
  Both fork boilerplates allow at most six tool calls.
  An Astra call is a shell command that batches reads: each of the six ledger lines of the tuned `affected-set` fork on `P4` names two to four reads or searches ("Searched hostname readers, board migration paths and Open-Window text consumers", "Read direct-wiring integration fixtures, CLI registration/signature guards and hub cloning").
  By the tools' design a Claude fork's `Read` and `Grep` calls are one file or one pattern each, and the boilerplate limits its Bash to git reads.
  Whether the Claude forks ran out of calls is unknown because their ledgers are not kept; a bench arm that raises the Claude cap, or lets a call bundle searches, would show it.
  No proposal changes the cap.
- **The focus directive is not the difference.**
  The seed focus is built from the overview, the lens review and the record's headings, and the Claude review closed several of its items as "not a defect" with the card's own stated bound as the evidence (`P4`'s attack-surface and nil-seam items).
  The Astra prompts carry the same Claude round's focus file, and Astra found defects on those same items (`P4-01`, `P4-03`).
  What differs is what a fork does with a stated bound, which Proposal 1 addresses.
- **The plan reaches the review hardened.**
  The plan-writing fan has eight lenses (`twill-plan`), against three at review, and `P4`'s lens review lists 48 lines of findings and their repairs; both engines review the same hardened plan, so the defects above are the ones eight lenses and a plan writer left.
  A fix the plan writer made in answer to a lens, such as `P4-02`'s `ApplySkipEnv` read, is outside every later lens's question unless the review re-checks it.

## What the study does not show

- That any proposal catches its targets: none was run, and each target was found by Astra at high or medium effort with the Claude reviewers' context and, for the tuned benches, a reading block.
- That the six groups hold on other plans: the two plans are loom plans of 17 (`P4`) and 33 (`P1`) cards.
- That Astra is right where the bench graded it right: the grading is by one grader reading the snapshot's code, the reports mark `P1-07` and `P1-10` as read and not run, and they call `P4-09`, `P4-13`, `P4-15`, `P1-05` and `P1-06` arguable or marginal.
- What the Claude forks read; see the evidence limits.

## Data

No file under `data/`: every figure comes from the files named in the text.
The defect table is built from the `unique-real` rows of the `data/match.tsv` of each of the eight Astra benches of 2026-10-27, and from the Claude round-1 review and usage files named under "Evidence and its limits".
