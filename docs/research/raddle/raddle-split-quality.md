# What gets revised in a raddle split

A study of every raddle run of this hub up to 2026-11-27, and the stencil changes it points to.
No stencil or code is changed here.

## Findings

- The hub holds 38 raddle runs: 37 filed and one abandoned (`claude-account-weights`).
- Of the filed splits, 21 kept the parts, recipes, profiles and dependencies of their first stored proposal.
  12 changed only dependencies, and 4 changed parts, recipe or profile
  (`motif-raddle-recipe`, `parent-reviews-decision-record`, `testmap-upkeep-design`, `redundant-test-finder`).
- In the raddle panes the operator asked for a change to the proposal in one run, `fabric-test-speed`.
  In 19 runs the operator typed a request for a parent review, and any revision there came from that review.
- The most common revision is a missing dependency on work already running or released.
- The corrections to part count and recipe are about size and cost:
  too many solid tapestries (the operator), or one part too large for one review (the orch, citing the operator's rule).
  The stencil has no size rule and no recipe or profile rule, and its one cost sentence pushes toward larger parts.
- No run was revised for parts that overlap a package: the `packages-shared` check refuses that before anyone sees it.

The operator's view on 2026-11-27 was that the proposals are "for absurd. For ofte." ("too absurd, too often").
The records bear this out for the large splits and not as a general rate:
the two clearest cases are a six-tapestry proposal on 2026-11-11 and a seven-tapestry one on 2026-11-26.
The operator took the second as first proposed after weighing it.
An unrevised split is not shown to be right, since several approvals came with no comment on the split.

## Sources and limits

- The raddle store on the board, `raddle/<slug>/`: the Sketch's record, the first proposal that passed the checks
  (`cold-proposal.yaml`) and the filed one.
- The session transcripts of the raddle agents, and of the prime orch for what the operator said outside the panes.
- `machine-name-config` ran on another machine: only its stored proposals are known.
- The six-tapestry proposal of 2026-11-11 is known from the operator's own account only:
  no stored record or session on this machine matches it.
- Changes the agent made before its first proposal passed the checks are not revisions here.
- Classifying a revision is a judgment.

## Classes and counts

A count is the number of runs with at least one revision of the class from that author.
"Operator" is the operator's own words, typed in the pane or the orch chat, or relayed as theirs.
"Orch" is the orch's own review.
"Split perch" is the one perch-mode run, `adopt-build-resplit`.

| Class | Operator | Orch | Split perch | Questioned, no change |
|---|---|---|---|---|
| Wrong order or dependencies | 1 | 12 | 1 | 1 |
| Too many or too few parts | 2 | 4 | 0 | 1 |
| Wrong recipe or profile | 2 | 1 | 0 | 4 |
| Wrong cut lines | 0 | 1 | 1 | 1 |
| Parts that overlap a package | 0 | 0 | 0 | 0 |
| Missing or invented parts | 0 | 0 | 1 | 0 |
| Wording | 0 | 7 | 1 | 0 |
| Other: built a superseded decision | 0 | 2 | 0 | 0 |
| Other: where the entries are filed | 2 | 1 | 0 | 0 |
| Other: size estimate | 1 | 0 | 0 | 0 |
| Other: proposal table | 1 | 0 | 0 | 0 |
| Other: package list or interface placement | 0 | 1 | 1 | 0 |

## Revisions and the words that asked for them

The operator's words are verbatim, typos included, with a translation.

### Wrong order or dependencies

| Run | Who | Change | Words |
|---|---|---|---|
| `fabric-test-speed` | operator | the largest saving heads the shared-package chain | "GJrø det koneservativt. COncurrewncy er ikke viktig" (Do it conservatively. Concurrency is not important) |
| `fabric-test-speed` | orch, on the operator's word | that part to depend on nothing | "Part dotgit-worktree-reader depends on nothing" |
| `reed-slack-relay` | orch | depend on a running task | "Add depends_on creel-cross-hub-mail to slack-relay." |
| `codex-same-as-claude` | orch | depend on a running task | "make part 1 depend on weaver-implementer so they never run side by side." |
| `code-review-result-checks` | orch | depend on a released task | "Order it after drift-judge-and-line-range (in flight, driftengine and loom wiring) and twill-fixer-fork-writes" |
| `config-keys-nested-by-area` | orch | depend on eight tasks | "the part is a rename and must run alone: set depends_on to every task released and not done at filing" |
| `dobby-run-watch` | orch | depend on two tasks | "Chain it: depends_on config-keys-nested (the rename runs alone, [...]) and dotnet-gate-fixes" |
| `keep-landed-records` | orch | depend on three tasks | "Set depends_on: config-keys-nested [...], pr-modes-no-agent and describe-review-and-sync (your two open risks; both change landing)." |
| `later-transition-push-wait` | orch | depend on six tasks | "give async-run-pusher depends_on on harden-fabric-undriven-cases (fabricengine), weft-main-push-race [...] and the running peers" |
| `orch-full-autonomy-mode` | orch | depend on two running tasks | "so the tapestry starts on top of them instead of merging around them." |
| `sandbox-closed-area-build` | orch | depend on a released rename | "area-creel-identity must depend on config-keys-nested too" |
| `testmap-upkeep-design` | orch | depend on an overlapping task | "check whether gate-scope-coverage touches internal/gatecli or internal/testcover and, if so, chain part 1 after it" |
| `adopt-redesign` | orch | drop an edge to landed work | "has landed and is gone from the board. Change: drop that edge" |
| `adopt-build-resplit` | split perch | an unordered part moved off the head of the chain | "board-prints-root-beam heads the depends_on chain, so the whole build waits on it, though the record gives D8 no order" |

Questioned in `fabric-test-speed`: "Hva med avhengigheter? å" (What about dependencies?).

Ten of the orch's runs add an edge to work outside the split.
The orch left an edge out where the overlap was small:
"a merge-time matter only" (`gate-builds-testmap`), "a rebase, not a design conflict" (`review-rounds-converge`).
One added edge cost something: in `keep-landed-records` it removed what two of the part's decisions build on.

**Passage and gap.**
Step 1 says to read "The runs in flight: `lym loom peers`", and no sentence says what to do with them.
The only overlap rule is between the split's own parts:
"keep two parts with no order between them off the same package."

### Too many or too few parts

| Run | Who | Change | Words |
|---|---|---|---|
| 2026-11-11 proposal | operator | six solid tapestries and a darn became four and a darn | "Senest i dag foreslo den å splitte i SEKS Solid Tapestry + en solid Darn. Det var absurdt mye og dyrt." (As late as today it proposed SIX solid tapestries and a solid darn. That was absurdly much and expensive.) |
| `fabric-git-free-core` | operator | 7 parts to 3, then back to 7 | "Vi kan ikek ha SYV tapestry solid. ALT for dyrt" (We can't have SEVEN solid tapestries. FAR too expensive), then "7 tapestryu (med feridg motif) ikke er så alt for galt likvel for en så stor endring" (7 tapestries are not so bad after all for so large a change) |
| `motif-raddle-recipe` | orch, citing the operator | a 1,305-line darn over 15 packages cut in two | "Part 2, arras-recipe-perch, is too large for one darn [...] the operator wants darns that are localized" |
| `parent-reviews-decision-record` | orch, citing the operator | an 1,825-line darn cut in two | "is large for a darn [...] smaller darns stay: each part gets its own run and its own review perches" |
| `testmap-upkeep-design` | orch, citing the operator | a 2,030-line darn cut in two | "split testmap-upkeep-pass into two chained darns so no part nears 2000 lines" |
| `claude-account-weights` | orch, citing the operator | one 3,400-line tapestry cut into three darns | "the one tapestry part is too large (about 3,400 lines, 14 packages, mean cell cost $38)" |

Questioned in `reed-slack-relay`:
"Tapestry på solid er generlet dyrt og omfattende. Hvorfor er en egen liten Darn lagt etterpå?"
(A solid tapestry is expensive and extensive in general. Why is a separate small darn added after it?).

The rule behind the orch's four, from the operator on 2026-11-11:
"Mindre darns betyr mer kontrollert og kontroll. Og flere perches. Og det er en fordel."
(Smaller darns mean more control. And more review rounds. And that is an advantage.)

**Passage and gap.**
"A tapestry's large fixed start cost often makes fewer, larger parts, or one tapestry, cheaper than many small tasks."
It argues for merging on cost, and "the part count stays what the work gives" never says what the work gives.
No sentence bounds a part's size.

Larger darns than the ones cut passed unrevised (1,990, 3,010 and, mostly deletions, 8,296 lines),
as did single tapestries over 5,000 lines.
Large single tapestries were defended on a shared interface or plan (`testmap-uses-design`, `later-transition-push-wait`).

### Wrong recipe or profile

| Run | Who | Change | Words |
|---|---|---|---|
| `redundant-test-finder` | operator | a darn part from `solid` to `light` | "Det vil ha mange Darn Solid, ikke mange light?" (It wants many solid darns, not many light ones?) |
| `fabric-git-free-core` | operator, as the orch put it | tapestries to darns, then back | "darn (solid or light) for the parts that are mechanical moves, keeping tapestry only where a part needs its own plan" |
| `claude-account-weights` | orch, citing the operator | tapestry to three solid darns | "smaller darns stay" |

Questioned with no change:

- `hardener`: "Hvorfor tapestryu og ikke flere darns?" (Why a tapestry and not several darns?)
- `adopt-redesign`: "Skal vi virke.ig ha hjel FIRE solid tapestryu?" (Are we really having FOUR solid tapestries?)
- `fabric-test-speed`: "Alt er Darn?" (Is it all darn?)
- `testmap-uses-design`, after filing: "Vi burde kanskje gtt for 4 darns i stedet? Den foreslo det"
  (Maybe we should have gone for 4 darns instead? It proposed that.)

In the first stored proposals, 71 of 90 parts are `solid`.
The orch's standing rule, kept for the operator in its handoff notes, is:
"The default recipe is darn; darn `solid` only for data risk, a wide change or a bundle, else `light`."

**Passage and gap.**
The stencil says only to weigh "what the cost report shows for each recipe and profile".
It has no default and no condition for `solid`.

### Wrong cut lines

| Run | Who | Change | Words |
|---|---|---|---|
| `adopt-redesign` | orch | migration, callers, writers and criteria moved to later parts | "move the writer, the retry and the criterion [...] to `adopt-foreground-leftovers`." |
| `adopt-build-resplit` | split perch | setters and criteria moved to the parts that use them | "the part lands functions only its tests call"; "carries three criteria whose behavior a later part builds" |

Questioned in `gate-scope-by-coverage-map`:
"Part 3 virker ne del mer omfattende enn de to andre?" (Part 3 seems a good deal larger than the other two?).

**Gap.**
"Never let a part guess at the interface of a part that runs beside it" covers parallel parts only.
Nothing says a part in a chain must land whole.

### Parts that overlap a package

No revision.
The one refusal was the check's, on the orch's edit in `fabric-test-speed`.
No stencil change.

### Missing or invented parts

| Run | Who | Change | Words |
|---|---|---|---|
| `adopt-build-resplit` | split perch | two pieces of work no part built were added | "D2's drop of this machine's hand-over marks on an adopt's takeover entry is built by no part" |

Found by this study and by no one at the time:
`fabric-test-speed` added a re-measure task where the record says "The split files the re-measure as a board note."

**Gap.**
"every decision, risk, criterion and PATTERN change in it must land in a part" works at the level of ids,
so a clause inside a decision can be built by no part.
Nothing forbids a part for what the record files as a note.

### Wording

| Run | Who | Words |
|---|---|---|
| `reed-slack-relay` | orch | "Name the live PATTERN entry" |
| `adopt-redesign` | orch | "retitle to \"…never launching fresh over a live holder\"." |
| `config-keys-nested-by-area` | orch | "add pr_review_parent_timeout_min (loom, from autonomy mode) to the keys to nest" |
| `sandbox-closed-area-build` | orch | "Carry the D6 attach criterion whole" |
| `kde-window-placement` | orch | "carry the lost paragraph" |
| `claude-account-weights` | orch | "carry each of the two BLOCKING Discussion-Review findings into the part it belongs to." |
| `fabric-git-free-core` | orch | "carry it into every part's body and criteria" |
| `adopt-build-resplit` | split perch | "its contract keeps no fix-tries.json reader" |

**Passage and gap.**
The proposal may carry a criterion or open risk only as the record's whole bullet.
The record parser can cut a multi-line bullet short (`kde-window-placement`, `sandbox-closed-area-build`),
and a superseded bullet cannot be reworded.
The stencil says nothing about either case.
The PATTERN mapping rule is in the stencil since 2026-11-06, after the `reed-slack-relay` run.

### Other

| Run | Class | Who | Words |
|---|---|---|---|
| `slack-v2` | superseded decision | orch | "do not take this proposal as it stands. The entry body's section 'Changed after the Discussion (operator, 2026-11-08)' overrides D3 and D6, and the proposal still builds the old record" |
| `redundant-test-finder` | superseded decision | orch | "the proposal misses an operator decision written into the Sketch's board body after the Motif" |
| `fabric-test-speed` | where filed | operator | "men under NYtt prosjekt for dette. sidne det s åmye" (but under a NEW project, since there is so much) |
| `codex-same-as-claude` | where filed | operator | "HVORFOR I ALLE DAGER tar Raddle og putter NYE prosjekter under gamle prosjetker?????" (WHY on earth does raddle put NEW projects under old ones?) |
| `testmap-upkeep-design` | where filed | orch | "file the tasks under the existing project test-suite-trim" (the proposal format could not express it) |
| `fabric-git-free-core` | size estimate | operator | "hvordan kan 58,6k linjer kode bli over 70k linjer kode, som den foreslo?" (how can 58.6k lines of code become over 70k, as it proposed?) |
| `fabric-test-speed` | table | operator | "Dette bør listes i tabellen din" (This should be listed in your table) |
| `adopt-redesign` | package list | orch | "add `boardengine` to its packages" |
| `adopt-build-resplit` | interface placement | split perch | "needs adoptcli's fix-tries reader, which its takes does not list" |

The table complaint is fixed: the interactive mode text has a dependency column since 2026-11-07,
and recipe and profile columns since 2026-11-11.

Line counts that disagree with their own cost line passed every review:

| Run | Package sizes | Cost line |
|---|---|---|
| `redundant-test-finder`, one part | 205 | about 285 |
| `config-keys-nested-by-area` | 3,250 | about 3,500 |
| `hardener` | 14,424 | about 15,600 |
| `code-review-result-checks` | 2,600 | about 2,650 |
| `adopt-redesign`, one part | 2,150 | about 2,300 |
| `fabric-git-free-core` | 49,255 | about 51,000 |

## Proposed stencil changes

All are to `contracts/stencils/raddle/raddle-template-agent.md` unless named otherwise.
The size limits in the second change are this study's judgment from the cuts above, not a measured threshold.

### Order and dependencies

Add in Step 3, after the bullet on ordering parts:

```markdown
- Compare each part's packages with the changed paths of every run in flight and with every released task.
  Give the part `depends_on` on each one that changes a package the part changes,
  changes behaviour the part reads, or builds what an open risk of the record names.
  A part that renames or moves names across the repository depends on every released task that is not done.
  Leave an edge out only when the overlap is a few lines that merge by hand,
  or when the other task removes what the part builds on, and tell the operator which and why.
```

### Part count and size

Replace the bullet that starts "A tapestry's large fixed start cost" with:

```markdown
- Size each part so one review covers a bounded change.
  Cut a `darn` part past about 1,500 changed lines, deleted lines left out, or past about eight packages,
  into chained darns wherever a seam holds.
  A saving in fixed cost is no reason to merge parts:
  the operator takes smaller darns and more reviews over one large part.
- Keep work in one `tapestry` only when it shares one interface or data shape that a single plan must fix,
  and name that interface in the part's `cost_line`.
- A split with more than two `tapestry` parts is expensive.
  For each, say why it needs its own plan and cannot run as darns,
  and put the cheaper split and its price in `next_best`.
```

### Recipe and profile

Add in Step 3:

```markdown
- `darn` is the default recipe.
  Give a darn part the profile `light`, and `solid` only when it risks data, changes widely
  or bundles unrelated changes; say which in the text around the table.
  A part that only writes docs or measures takes `docs`.
```

### Cut lines

Add after the bullet on guessing an interface:

```markdown
- Every part lands whole.
  It holds no code whose only production caller a later part builds,
  and every criterion and open risk it carries can be observed once it lands.
  Move the code or the criterion to the part that completes it.
```

### Missing or invented parts

Add in Step 3:

```markdown
- Map each step and clause of every decision to a part, not only each decision id.
  For a decision that more than one part builds, say in each part's contract which steps it builds.
- Add no part for work no decision of the record holds,
  and none for what the record says is filed as a note: tell the operator what to file by hand.
```

### Wording

Add in Step 4:

```markdown
A part carries a criterion or an open risk only as the record's whole bullet.
When a bullet is cut short by the record's layout, or a later decision supersedes it,
ask your parent to replace it in the record before you ask for approval.
Until then, state the full or later rule in the part's `contract.takes`, and say so to the operator.
```

### Superseded decisions

Add to the list in Step 1:

```markdown
- The Sketch's own board entry: `lym board get '{"slug":"{{.slug}}"}'`.
  A decision its body states that the record does not hold, or that changes the record, is later than the record and wins.
  Build the later decision, and ask your parent to put it in the record before approval.
```

### Size estimates

Add in Step 3, after the bullet on cost lines:

```markdown
- The line count a `cost_line` states is the sum of that part's package sizes.
  Count deleted lines apart from changed lines, and moved tests that will be deleted as deleted.
```

### Where the entries are filed

The level of a filed project is chosen by Raddle-File from the Sketch's own `project` field,
and the proposal has no key for it, so letting the operator choose is a code and spec change.
The stencil can at least make the level visible before approval. Add in Step 4:

```markdown
In the text around the table, say which project, or the top level, the filed entries land under.
```

### The split perch's rubric

The perch can only enforce what its rubric names,
and `contracts/stencils/loom/loom-rubric-raddle-review.md` has no size rule and no condition for a profile.
Replace the bullet "Each part's recipe and profile fit it." with these, so a perch-mode run is held to the same rules:

```markdown
- **Each part is bounded.**
  A `darn` part past about 1,500 changed lines, deleted lines left out, or past about eight packages,
  with a seam that holds, is a finding.
  So is a `tapestry` part with no shared interface or data shape that needs one plan.
- **Each part's recipe and profile fit it.**
  `darn` is the default and `light` its profile; `solid` needs a data risk, a wide change or a bundle.
  Each cost line rests on the cells it cites and states the sum of its part's package sizes.
- **Running work is ordered.**
  A part with no `depends_on` on a run in flight or a released task that changes the same package
  or behaviour the part reads, where the overlap is more than a few lines, is a finding.
- **Every part lands whole.**
  Code whose only production caller a later part builds, or a criterion a later part's work makes true, is a finding.
- **The later decision wins.**
  A part that builds what the Sketch's board entry or a later decision has replaced is a finding.
```

## Runs

Each table is the first stored proposal, in its part order.
The cut shows as the decisions each part builds and its largest packages.
The last column is how the filed part differs.
The revisions are in the tables above; "none" means no one asked for one.
Dates are the raddle session's.

| Run | Date | The Sketch's record | Revisions by class |
|---|---|---|---|
| `adopt-build-resplit` | 11-16 | all that the adopt work had left to build | order, cut, missing, wording, interface (perch) |
| `adopt-redesign` | 11-15 | runs and hub as one state machine kept on GitHub | order, cut, wording, packages |
| `adopt-robust` | 11-09 | a predictable, resumable takeover of a hub | none |
| `board-viewer` | 11-17 | an interactive board view in VS Code | none |
| `board-viewer-editor` | 11-19 | the view also edits the board | none |
| `claude-account-pick-simple` | 11-18 | one Claude account per worktree | none |
| `claude-account-weights` | 11-11 | pick accounts by weight and headroom | parts, recipe, wording; abandoned |
| `code-review-result-checks` | 11-18 | Code-Review catches result-level issues | order |
| `codex-same-as-claude` | 11-16 | a Codex session gets what a Claude session gets | order, where filed |
| `config-keys-nested-by-area` | 11-23 | nest config keys by area | order, wording |
| `creel-cross-hub-mail` | 11-05 | mail across hubs | none |
| `decision-record-replace` | 11-11 | edit or replace a decision record | none |
| `dobby-run-watch` | 11-26 | the dobby watches every run it drives | order |
| `fabric-git-free-core` | 11-26 | a git-free fabric core | parts, recipe, estimate, wording |
| `fabric-single-repo-mode` | 11-18 | a hub as one repository | none |
| `fabric-test-speed` | 11-07 | cut fabric test time | order, where filed, table |
| `gate-builds-testmap` | 11-17 | build coverage maps in the gate | none |
| `gate-scope-by-coverage-map` | 11-11 | a coverage gate scope | none |
| `hardener` | 11-19 | an autonomous hardening recipe | none |
| `kde-window-placement` | 11-27 | place terminal windows on KDE | wording |
| `keep-landed-records` | 11-26 | keep landed runs' review records | order |
| `later-transition-push-wait` | 11-23 | no transition waits for a push | order |
| `machine-name-config` | 11-09 | a short name per machine | unknown (other machine) |
| `motif-raddle-recipe` | 11-08 | the arras recipe | parts |
| `move-task-by-handback` | 11-22 | move a running task to another parent | none |
| `orch-full-autonomy-mode` | 11-22 | an orch that settles every halt | order |
| `parent-reviews-decision-record` | 11-11 | an extra review of the decision record | parts |
| `redundant-test-finder` | 11-08 | find redundant tests mechanically | recipe, superseded |
| `reed-slack-relay` | 11-05 | follow orchs from Slack | order, wording |
| `review-rounds-converge` | 11-17 | review rounds that stop circling | none |
| `sandbox-closed-area-build` | 11-26 | a closed area agents cannot act outside | order, wording |
| `single-repo-records-branches` | 11-19 | records off the code branch | none (a slug the board refused) |
| `slack-machine-bridge` | 11-12 | orch mail between machines | none |
| `slack-v2` | 11-08 | a Slack channel per orch | superseded |
| `testmap-checked-in` | 11-12 | coverage maps in git | none |
| `testmap-upkeep-design` | 11-11 | keep coverage maps fresh | parts, order, where filed |
| `testmap-uses-design` | 11-18 | three uses of coverage maps | none |
| `weaver` | 11-16 | a cold-start implementer | none |

### adopt-build-resplit

| Part | Recipe | Lines | Decisions | Packages | Depends on | Filed |
|---|---|---|---|---|---|---|
| `board-prints-root-beam` | darn/light | 125 | D8 | boardcli, boardengine +2 | - | after `adopt-foreground` |
| `transition-push-takeover` | tapestry/solid | 3180 | D1 D2 | fabricengine, battencli, landingshed +16 | `board-prints-root-beam` | 3170 lines, depends on nothing |
| `machine-registry-holds-catchup` | tapestry/solid | 2570 | D3 D4 D5 | boardengine, dobbyengine, machinecli +23 | `transition-push-takeover` | 2410 lines |
| `adopt-foreground` | tapestry/solid | 3650 | D6 | adoptcli, adoptrecord, dobbyengine +8 | `machine-registry-holds-catchup` | 3920 lines |
| `leftover-pairs-teardown` | darn/solid | 540 | D7 | dobbyengine, beamcli, pairteardown +5 | `adopt-foreground` | same |
| `hub-stop-all` | darn/solid | 425 | D9 | hibernateengine, beamcli, orchcli +2 | `leftover-pairs-teardown` | same |
| `machine-start-launchers` | darn/solid | 970 | D10 | machinecli, desktoplauncher +2 | `leftover-pairs-teardown` | same |

### adopt-redesign

| Part | Recipe | Lines | Decisions | Packages | Depends on | Filed |
|---|---|---|---|---|---|---|
| `orch-resume-own-session` | darn/solid | 550 | D9 D10 | orchcli, runproc +2 | `test-only-functions-in-prod` | depends on nothing |
| `run-fsm-tracked-records` | tapestry/solid | 2540 | D1 D3 | runfsm, battencli, dobbyengine +11 | `test-only-functions-in-prod` | 2470 lines, depends on nothing |
| `push-before-act-takeover` | tapestry/solid | 2320 | D2 D4 | fabricengine, battencli, sheddriver +9 | `run-fsm-tracked-records` | 2410 lines |
| `bolt-holds-catch-up` | tapestry/solid | 2400 | D5 D6 D7 | boardengine, dobbyengine, pauserecord +17 | the push part and the orch part | 2150 lines |
| `adopt-foreground-leftovers` | tapestry/solid | 3640 | D8 D11 | adoptcli, adoptrecord, dobbyengine +8 | `bolt-holds-catch-up` | 4030 lines |

### adopt-robust

| Part | Recipe | Lines | Decisions | Packages | Depends on | Filed |
|---|---|---|---|---|---|---|
| `adopt-worker-record` | tapestry/solid | 2170 | 8 decisions | adoptcli, adoptrecord, beamcli +6 | `testmap-dobby-refresh` | same |
| `adopt-owner-handover` | darn/light | 1170 | D4 D11 | dobbyengine, fabricengine, orchcli +3 | `adopt-worker-record`, `fabric-serial-phase-tests` | same |
| `adopt-here-archives` | darn/light | 610 | D10 D12 | adoptcli, fabricengine, shortcutcli | `adopt-owner-handover` | same |

### claude-account-weights

First proposal: one `claude-account-pick`, tapestry/solid, 3,370 lines, D1-D9, no dependency.
Last proposal, never filed, three chained darn/solid parts:

| Part | Lines | Decisions | Depends on |
|---|---|---|---|
| `claude-accounts-readers` | 1270 | D1 D6 | - |
| `claude-account-record` | 1860 | D2 D3 D4 D7 D8 | `claude-accounts-readers` |
| `claude-account-statusline` | 760 | D5 D9 | `claude-account-record` |

The operator then dropped the Sketch's approach:
"Dette virker veldig komplisert. Jeg må tenke på pm det er verdt det."
(This seems very complicated. I have to think about whether it is worth it.)

### codex-same-as-claude

| Part | Recipe | Lines | Decisions | Packages | Depends on | Filed |
|---|---|---|---|---|---|---|
| `prompt-engine-per-launch` | darn/solid | 1350 | D3 D4 | contracts/stencils, shuttleengine +19 | - | after `weaver-implementer` |
| `codex-standing-instructions` | tapestry/solid | 1790 | D5-D9 | codexengine, claudeengine, heddleengine +5 | `prompt-engine-per-launch` | same |
| `codex-wording-engine-blocks` | darn/solid | 1990 | D2 | contracts/stencils, contracts/specs +10 | `codex-standing-instructions` | same |
| `codex-question-deny-nudge` | darn/solid | 1260 | D12 | codexengine, bashlookup, claudeengine +2 | `codex-standing-instructions` | same |
| `launch-manifest-parity` | tapestry/solid | 1500 | D1 D10 D11 | shuttleparity, shuttleengine +3 | the wording and question-deny parts | same |

### Two-part runs

| Run and part | Recipe | Lines | Decisions | Packages | Depends on | Filed |
|---|---|---|---|---|---|---|
| `decision-record-replace`: `record-file-upsert` | darn/light | 700 | D4 D5 | boardcli, boardengine +2 | - | same |
| `decision-record-replace`: `decision-record-replace-verb` | darn/solid | 1470 | D1 D2 D3 D6 | loomcli, fabricengine +5 | `record-file-upsert` | same |
| `fabric-single-repo-mode`: `single-repo-fabric-mode` | tapestry/solid | 5990 | 9 decisions | fabricengine, hubforge, lymcwd +6 | - | same |
| `fabric-single-repo-mode`: `single-repo-code-identity` | tapestry/solid | 3580 | D8 D9 D10 | fabricengine, landingshed +16 | first part, `landingshed-design-simplify` | same |
| `machine-name-config`: `machine-name-file` | darn/solid | 375 | 6 decisions | machinename, standalonestate +3 | - | same |
| `machine-name-config`: `machine-name-verb` | tapestry/solid | 940 | D5 D6 D8 | machinecli, boardengine, cmd/lym | `machine-name-file` | same |
| `reed-slack-relay`: `slack-relay` | tapestry/solid | 2580 | 8 decisions | slackengine, slackcli, creelcli +13 | - | after `creel-cross-hub-mail` |
| `reed-slack-relay`: `slack-wait-reminders` | darn/light | 350 | D8 | dobbyengine, beamcli +2 | `slack-relay` | same |
| `weaver`: `implementer-shared-run-plan` | darn/solid | 3010 | D11 | runplan, websterengine +8 | - | same |
| `weaver`: `weaver-implementer` | tapestry/solid | 7460 | 13 decisions | weaverengine, weavercli, loomcli +15 | first part | same |

### Single-part runs

| Run: part | Recipe | Lines | Decisions | Largest packages | Depends on | Filed |
|---|---|---|---|---|---|---|
| `board-viewer`: `board-viewer-build` | darn/solid | 1823 | 6 | boardengine, boardcli, vscode | - | same |
| `board-viewer-editor`: `board-viewer-server` | tapestry/solid | 5320 | 12 | boardengine, boardserve, boardcli | - | same |
| `claude-account-pick-simple`: `claude-accounts-per-worktree` | tapestry/solid | 2355 | 10 | claudeengine, shuttleengine, reedengine | `leftover-strands-and-runs` | same |
| `code-review-result-checks`: `review-result-checks` | tapestry/solid | 2600 | 7 | driftengine, loomcli, landingshed | - | after `twill-fixer-fork-writes` |
| `config-keys-nested-by-area`: `config-keys-nested` | tapestry/solid | 3250 | 9 | loomengine, configsync, configengine | - | after eight tasks |
| `creel-cross-hub-mail`: `creel-cross-hub-build` | tapestry/solid | 3280 | 9 | creelengine, creelcli, hubregistry | `slack-relay` | same |
| `dobby-run-watch`: `dobby-run-watch-build` | tapestry/solid | 1005 | 8 | dobbyengine, beamcli, battencli | - | after two tasks |
| `gate-builds-testmap`: `gate-test-map-mode` | tapestry/solid | 5877 | 9 | gatecli, testcover | - | same |
| `hardener`: `hardener-campaign` | tapestry/solid | 14424 | 12 | treadleengine, hardenshed, crucible | `single-repo-fabric-mode` | same |
| `kde-window-placement`: `kde-window-placement-build` | tapestry/solid | 3390 | 12 | kwinplace, reedcli, desktophook | two tasks | same |
| `keep-landed-records`: `keep-landed-run-records` | tapestry/solid | 2104 | 7 | landingshed, reportengine, loomcli | - | after three tasks |
| `later-transition-push-wait`: `async-run-pusher` | tapestry/solid | 2745 | 7 | fabricengine, battencli, loomcli | - | after six tasks |
| `move-task-by-handback`: `orch-move-running-task` | darn/light | 91 | 8 | contracts/orchrole, heddleengine | `per-task-settings-one-lookup` | same |
| `orch-full-autonomy-mode`: `orch-autonomy-mode` | tapestry/solid | 2752 | 10 | landingshed, loomcli, pawlengine | - | after two tasks |
| `review-rounds-converge`: `review-rounds-converge-build` | tapestry/solid | 3060 | 6 | shedadapters, burlerengine, fabricengine | - | same |
| `slack-machine-bridge`: `slack-machine-bridge-build` | tapestry/solid | 4000 | 11 | slackengine, creelengine, creelcli | - | same |
| `slack-v2`: `slack-push-channels` | tapestry/solid | 5390 | 11 | slackengine, slackcli | - | same cut, contract and criteria rewritten |
| `testmap-checked-in`: `testmap-maps-in-weft` | tapestry/solid | 2040 | 11 | gatecli, fabricengine, testcover | - | same |
| `testmap-uses-design`: `testmap-reach-triage-mutate` | tapestry/solid | 5120 | 12 | gatecli, testreach, testcover | - | same |

### fabric-git-free-core

Seven chained tapestry/solid parts, filed in the same cut and order after the detour through three.

| Part | First lines | Filed lines | Decisions | Largest packages |
|---|---|---|---|---|
| `fabric-core-foundation` | 5885 | 4885 | 9 | fabriclayout, fabricengine, fabriccore |
| `fabric-core-push-origin` | 11770 | 8870 | 10 | fabricengine, fabriccore, boardengine |
| `fabric-core-teardown` | 11750 | 7650 | 10 | fabricengine, fabriccore, fabriccli |
| `fabric-core-merge-landing` | 9600 | 6600 | 10 | fabricengine, fabriccore, shedengine |
| `fabric-core-pull-replay` | 10720 | 7920 | 10 | fabricengine, fabriccore, boardengine |
| `fabric-core-add-clone` | 14630 | 8730 | 10 | fabricengine, fabriccore, ideengine |
| `fabric-core-records-guards` | 8400 | 4600 | 11 | fabricengine, fabriccore, docs/research/speed |

The three-part version was one tapestry/solid (foundation and push, 17,655 lines)
and two chained darn/solid parts (32,070 and 23,030 lines).

### fabric-test-speed

| Part | Recipe | Lines | Decisions | Packages | Depends on | Filed |
|---|---|---|---|---|---|---|
| `fabric-test-folds` | darn/light | 500 | D6 | fabricengine | - | after `dotgit-worktree-reader` |
| `hubforge-registered-templates` | darn/solid | 450 | D2 | hubforge | - | same |
| `hubforge-slim-copy` | darn/light | 120 | D3 | hubforge | registered templates | same |
| `git-config-from-go` | darn/light | 150 | D4 | gitkit, hubforge | slim copy | same |
| `fabric-anchor-origin-templates` | darn/light | 150 | D7 | fabricengine | registered templates, test folds | same |
| `fabric-livestate-merge-templates` | darn/light | 200 | D7 | fabricengine | registered templates, test folds, anchor origin | same |
| `dotgit-worktree-reader` | darn/solid | 780 | D1 | dotgit, fabricengine, websterengine | livestate merge | depends on nothing |
| `copyhub-fixtures-and-hook` | darn/light | 40 | D5 | fabricengine, adoptcli | dotgit reader | after livestate merge |
| `hubreconcile-templates` | darn/light | 120 | D5 D7 D8 | hubreconcile | registered templates | same |
| `loomcli-templates` | darn/light | 80 | D7 | loomcli | registered templates | same |
| `battencli-templates` | darn/light | 50 | D7 | battencli | registered templates | same |
| `fabric-test-speed-remeasure` | darn/docs | 60 | D9 D10 | docs/research/speed | five of the parts above | also after dotgit reader |

Filed as a project instead of loose tasks.

### gate-scope-by-coverage-map

| Part | Recipe | Lines | Decisions | Packages | Depends on | Filed |
|---|---|---|---|---|---|---|
| `testmap-cross-package` | darn/solid | 800 | D2 | gatecli, testcover | `testmap-upkeep` | same |
| `coverage-test-select` | darn/solid | 900 | D4 D5 | testcover | `testmap-cross-package` | same |
| `gate-scope-coverage` | darn/solid | 980 | D1 D3 D6 D7 D8 | gatecli, impactset, gateslot | `coverage-test-select` | same |

### motif-raddle-recipe

| Part | Recipe | Lines | Decisions | Packages | Depends on | Filed |
|---|---|---|---|---|---|---|
| `project-release-members` | darn/light | 230 | D8 | boardengine, boardcli | - | same |
| `arras-recipe-perch` | darn/solid | 1305 | D1-D5 D7 | loomcli, loomengine +13 | - | 865 lines, D1-D3, after the new part |
| `raddle-perch-mode` (added) | darn/solid | - | - | loomcli, loomshed, raddleparser | - | 530 lines, D4 D5 D7 |
| `arras-family-filing` | darn/solid | 510 | D6 D10 | boardengine, loomshed +3 | the first two | also after the new part |
| `arras-shipped-text` | darn/docs | 115 | D9 | skills, contracts/orchrole +3 | all above | also after the new part |

### parent-reviews-decision-record

| Part | Recipe | Lines | Decisions | Packages | Depends on | Filed |
|---|---|---|---|---|---|---|
| `discussion-review-parent-seat` | darn/solid | 1825 | D1-D7 | burlerengine, loomcli, shedadapters +5 | - | 1020 lines, D1-D3 D6 D7, after the new part |
| `parent-review-ask-verbs` (added) | darn/solid | - | - | loomcli, burlerengine | - | 845 lines, D4 D5 |

### redundant-test-finder

| Part | Recipe | Lines | Decisions | Packages | Depends on | Filed |
|---|---|---|---|---|---|---|
| `testcover-engine` | darn/solid | 1620 | D1-D4 | testcover, cmd/testtiming +2 | - | same |
| `testmap-verb` | darn/solid | 930 | D5 | gatecli, testcover +4 | `testcover-engine` | same |
| `testmap-dobby-refresh` | darn/solid | 340 | D6 | dobbyengine, beamcli, runproc | `testmap-verb` | darn/light, 290 lines |
| `gate-redundancy-report` | darn/solid | 1100 | D7 | gatecli, gateslot +2 | `testmap-verb` | same |
| `review-reads-redundancy` | darn/light | 205 | D8 D9 | shedadapters, shedrecipe +3 | `gate-redundancy-report` | same |

### sandbox-closed-area-build

| Part | Recipe | Lines | Decisions | Packages | Depends on | Filed |
|---|---|---|---|---|---|---|
| `area-creel-identity` | darn/solid | 1270 | D8 D14 | creelengine, hubregistry +5 | - | after `config-keys-nested` |
| `area-seccomp-core` | tapestry/solid | 6350 | 12 decisions | areaengine, areacli, reedcli +18 | identity part, `config-keys-nested` | same |
| `area-apparmor-barrier` | darn/solid | 450 | D15 | areaengine, areacli | `area-seccomp-core` | same |
| `area-host-exits` | darn/solid | 750 | D13 | areaengine, areacli +6 | `area-apparmor-barrier` | same |

### single-repo-records-branches

| Part | Recipe | Lines | Decisions | Packages | Depends on | Filed |
|---|---|---|---|---|---|---|
| `single-repo-records-on-code-removal` | darn/solid | 8296 | D5 D6 D7 | fabricengine, fabriccli, statuscommit +12 | - | renamed `single-repo-code-records-removal` |
| `single-repo-namespace-layout` | tapestry/solid | 2470 | 7 decisions | fabricengine, gitrepo, fabriccli +4 | first part | same |

### testmap-upkeep-design

| Part | Recipe | Lines | Decisions | Packages | Depends on | Filed |
|---|---|---|---|---|---|---|
| `testmap-selector-upkeep-rules` | darn/solid | 770 | D2 D3 D6 D13 | testcover, gatecli | `coverage-test-select` | also after `gate-scope-coverage` |
| `testmap-upkeep-pass` | darn/solid | 2030 | 8 decisions | gatecli, testcover, gateslot | selector part | cut in two, below |
| `testmap-upkeep-plan` (added) | darn/solid | - | - | gatecli, testcover, gateslot | - | 1230 lines, D1 D4 D5 D7 D12, after the selector part |
| `testmap-upkeep-runs` (added) | darn/solid | - | - | gatecli, gateslot | - | 750 lines, D8-D10, after the plan part |
| `testmap-upkeep-dobby` | darn/solid | 620 | D8 D11 | dobbyengine, configengine +3 | pass part, `creel-notice-hygiene` | after the runs part |
