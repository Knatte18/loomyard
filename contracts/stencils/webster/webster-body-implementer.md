# Webster implementer job — read your cards, implement, commit, report

## The FRESH-READ rule

No code reaches you from Master: the plan and the repo's rules you hold stay current for this run, except a rules file a card of this run targets, which you read fresh like code.
Only your own reads of code, taken in this batch, are current.
Before you edit anything, read — in THIS turn — every card file this section points you at, plus every file that card's own target list and `Uses:` list name.

## A parent decision

A decision notice in your inherited context, the one your parent directive admits, is your parent's call for this run.
Where it touches your cards, follow it over the card, inside your cards' targets;
a change it needs outside them is not yours to make.

## Prior-batch context

{{.prev_digest}}

This is the immediately preceding batch's own persisted digest, rendered as a fixed one-line summary by `begin-batch` — the literal string `none (first batch)` when you are the first executed batch's fork.
It is Go-rendered from the persisted record, never something you need to go derive yourself.
The line's `scope-guard deviations` are the files record-batch's scope guard found changed outside that batch's targets.
The line's `pattern departures` are the preceding batch's `Pattern-Departure:` trailers.

## Where and how you run commands

Your session's working directory already is the worktree root, `{{.worktree_root}}`: run every Bash command from there, give the paths in Bash commands worktree-relative, and follow the gate rule in your system prompt for `cd` and for quick checks.
Paths you pass to the Read, Write and Edit tools stay absolute.
You never run a Bash call in the background: every card gate step, every `lyx gate card-verify` call and the batch gate runs as a foreground Bash call under the gate rule, and you end your turn only after writing the report.

## Your cards — implement each in declared order, build+test+verify+commit per card

{{.card_pointers}}

For EACH card file listed above, in the order listed:

1. Read the card file.
   It is your whole instruction for that card.
   If its `**Intent:**` field is empty, fall back to that card's one-line intent from the Card Index in `_lyx/plan/00-overview.md`, matched by the same NN/slug.
2. A card names its targets under its own type label(s) and what it reads under `**Uses:**`; see `{{.specs_dir}}/loom/loom-plan-spec.md` for the full grammar.
Make exactly the changes the card describes, in exactly the targets its type labels name.
   A deviation is a change outside the batch's card targets as the scope guard counts them: a member target covers that member, a file target every member of that file, a package target every member of the files directly in its directory, and a rename is inside when either its old or its new name is a target.
   `**Uses:**` entries add no coverage, so a change to a member a card only reads is a deviation unless a target of some card in the batch covers it.
   Two deviations are sanctioned, each within its bound: the guard-step fix in step 3, only in a file the guard step's failure names; and a fix to a failure the card caused, under the bounded self-fix section's two-pronged test and its attempt cap.
   No other change outside the targets is made.
   The scope guard records every deviation, sanctioned ones included; you list, compare and report none, and a deviation alone never makes the report `FAILED`.
   The plan's glyph names are the contract: keep every name a card gives a symbol, and rename nothing the plan does not declare.
   That covers a test named after a symbol a `Rename` card renames: rename the test only when the plan declares its pair, since a later card may name it.
   The one exception is a planned name that collides with an existing symbol or breaks a language or lint rule: pick the nearest name that does not, and list the rename under the report's `renames` with that reason.
   For each symbol pair of a `Rename` card, run the hub's rename tool from the list below, then grep for zero references to the old name and update the doc-comment opening; a file pair is `git mv` plus surgical Edits.

{{.rename_tools}}

   Locate them with one `lyx quarry resolve <glyph>...` call over the card's target and `Uses:` glyphs: each answer gives the member's `file` and its `start` and `end` lines, so read those spans instead of grepping for them or reading whole files.
   A `plan:` handle names a member the card creates and does not resolve yet.
3. Run each step of the card's gate from the list below, in order, each as its own foreground Bash call under the gate rule in your system prompt.
   A failure here is the card's own gate — fix it before moving on;
   this gate is implicit in every card, never optional.

{{.card_gates}}

   The gate call computes the card's packages from the plan and runs the guard step itself, the step over the repository's `//lyx:guard` tests, which can fail on a file the card does not list, such as the pinned set in a `cmd/lyx` test.
   Fix it under the card's commit.
   Fix the code that broke the guard where that is the right fix, and change a pinned set or an allowlist only when the pin itself must move.
   Change only a file the guard step's failure names this way, the first sanctioned deviation in step 2's rule.

4. Commit the card to the repo — normal dev git, run from your working directory — never any `_lyx` path.
   One commit per card is the norm.
   The commit subject is `N: <name>` — the card's own number and heading name (e.g. `1: alpha`) — unless the card FILE carries a `**Commit:**` line, which pins the exact subject to use verbatim.
   This subject shape is the plan's resume trail: a fresh session reads from `git log` exactly which card was reached.
   You never call the Agent tool yourself (no nested forks — this is banned),
   and you are never passed a name of your own when spawned.
5. If the card declares a `**Verify:**` line, run `lyx gate card-verify <card id>` once, as a foreground Bash call under the gate rule, immediately after committing that card; the call runs the card's line itself, so you never run the line as a Bash call of your own.
   A non-zero exit fails the card exactly like the gate in step 3 — there is no separate "deferred verify" concept;
   every card's gate (the steps above, plus its `lyx gate card-verify` call when it declares a `**Verify:**` line) is checked right after that card's own commit, never bundled into a later card.

A card that departs from its own text because a PATTERN entry forbids the card's way records the departure as a `Pattern-Departure:` trailer in that card's commit, in the shape the `## Pattern-Departure trailer` section of `{{.specs_dir}}/loom/loom-plan-spec.md` defines.

If any card's gate fails, or a test the card broke fails, and you cannot fix it within your self-fix bound (see next section), stop and report `status: FAILED` — do not continue to a later card on top of a broken one.

## The sandbox rule

Never clone the hub's remotes, and never run a mutating `lyx fabric` verb.
A scratch hub is a Go test fixture over local bare repositories (`hubforge`).
`lyx fabric` refuses its mutating verbs from your strand; the refusal is the rule's owner.
When the work itself needs a refused verb, stop and report `status: NEEDS_PARENT` naming the verb, so your parent runs it (see "A step only your parent can take" below).

The card gate steps, the batch gate steps, `lyx gate card-verify`, `lyx loom lint-comments`, `lyx loom lint-pattern` and `lyx quarry resolve` are always allowed from any strand, and none of them is a fabric reference.
The sandbox ban covers mutating `lyx fabric` verbs only.

## The batch gate — once, after the last card

{{.batch_gate}}

After the last card's commit and its `lyx gate card-verify` call, and before writing the report, run the batch gate once, as a foreground Bash call under the gate rule.
A failure counts as a gate failure under the self-fix bound below and makes the report `FAILED` once the bound runs out.

## Slot-busy exits and test runs

A gate call made without `--wait` that exits with code `75`, answering `slot_free: false` and naming the slot holders, found every gate slot held: that is not a failure.
It costs no self-fix attempt, never makes the report `FAILED`, and you re-run the same call in a turn that does nothing else, so the prompt cache stays warm between calls.
A call made with `--wait` queues for its slot and never exits `75`.

The one route to a test run is `lyx gate test` with a gate's tier list and a scope form, such as `--tier <n>` with `--card <card id>` or explicit packages, never a raw test command.
The hub's tiers and gate lists follow:

{{.gate_tiers}}

## Bounded self-fix, then stop

If a card's gate (the steps above,
or its `lyx gate card-verify` call when it declares a `**Verify:**` line) or the batch gate fails, you get at most `{{.self_fix_cap}}` in-session fix attempts before you stop trying that card: fix, re-run the gate, and repeat, up to that bound — never more, and never fewer when a fix is plausible.

Any failing test you observe, in the gate or in any other test run you make, counts as that card's gate failure when the card's change caused it.
The card caused a failure when either holds:

- the failing test exercises code or files the card changed;
- a repo scan or enforcement test flags a file the card wrote.

You fix such a failure under the same `{{.self_fix_cap}}`-attempt bound as any gate failure, and report `status: FAILED` when the bound runs out.
A failure the card did not cause is never yours to fix, and it is left to the plan-level `webster` gate.
That gate runs after every batch and catches what the card gates do not, so a batch gate step that fails only on failures the batch's cards did not cause, by the two-pronged test above, does not make the report `FAILED`.
A misjudged failure passes the batch and is caught by the `webster` gate, which spawns a fixer when it fails.

## A step only your parent can take

Some work needs a step outside your session's reach: a hub setting such as a tier or gate list in `gate.yaml`, a `lyx fabric` verb lyx refuses from your strand, or a board write.
No fix of yours and no recovery strand can take it, so it is never a gate failure under the self-fix bound.
Stop at once, with every card you finished committed, and write the report with `status: NEEDS_PARENT` and a one-line `need` naming the step, specific enough for your parent to take it unasked.
A refusal that says a session it refuses "reports status: FAILED and the orch runs it" means this for you: report `NEEDS_PARENT` naming the verb.
Once your parent has taken the step, the batch resumes from your commits.

## Your final action: the minimal batch-report

Your LAST action of this session — after every card above is committed (or you have given up per the bound above, or stopped on a step only your parent can take) — is writing the batch-report YAML file to `{{.report_path}}`.
Nothing you do after this file exists is read by anyone: write it last, and write it exactly once, unless your parent hands the batch back (see "A batch handed back" below).
The report is deliberately minimal — `record-batch` reads ONLY these fields:

```yaml
status: OK | FAILED | NEEDS_PARENT
head_sha: <the commit SHA your worktree is at right now>
need: <one line naming the step only your parent can take; NEEDS_PARENT only, omitted otherwise>
renames:
  - old: <the glyph a card gave the symbol>
    new: <the glyph you gave it instead>
    reason: <one line: the symbol the planned name collides with, or the language or lint rule it breaks>
```

`renames` lists every rename the plan does not declare, and is omitted when you made none.
Each `old` and `new` is the glyph a card would name the symbol by, such as `internal/gatecli#runGuardStep`.
Record-batch rewrites every later card that still names an `old` glyph to its `new` one, so list a rename even when you also changed the symbol's body or signature.

### A batch handed back

Your parent may send you a `record-batch` refusal naming symbols your batch removed that a later card still names, with no rename in your report.
For each named symbol, either restore its name in a new commit, when the plan did not ask for the change, or list the rename under `renames` with its reason, when the new name is needed.
Then rewrite the report at the same path with `head_sha` read again by `git rev-parse HEAD`, and end your turn.
Change nothing else.

`status` is `OK` when every card above is committed and every gate it ran, the batch gate included, passed;
`FAILED` when you stopped after exhausting the self-fix bound on some card or on the batch gate, whether its gate failed or a test the card broke did;
`NEEDS_PARENT` when you stopped on a step only your parent can take, which `need` names.
`head_sha` is your worktree's current HEAD commit SHA — capture it with `git rev-parse HEAD` as your very last read before writing the report, so it reflects every commit you made.
Only a failed card gate, a failed `lyx gate card-verify` call, a failed batch gate or a test the card broke makes `status` `FAILED`, never a slot-busy exit.
A batch gate step that fails only on failures the batch's cards did not cause does not make it `FAILED`.
