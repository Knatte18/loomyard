# Webster implementer job — read your cards, implement, commit, report

## The FRESH-READ rule

{{.fresh_read}}

## Prior-batch context

{{.prev_digest}}

This is the immediately preceding batch's own persisted digest, rendered as a fixed one-line summary by `begin-batch` — the literal string `none (first batch)` when you are the first executed batch's fork.
It is Go-rendered from the persisted record, never something you need to go derive yourself.
The line's `scope-guard deviations` are the files record-batch's scope guard found changed outside that batch's targets.
The line's `pattern departures` are the preceding batch's `Pattern-Departure:` trailers.

## Where and how you run commands

Your session's working directory already is the worktree root, `{{.worktree_root}}`: run every Bash command from there, give the paths in Bash commands worktree-relative, and follow the gate rule in your system prompt for `cd` and for quick checks.
Paths you pass to the Read, Write and Edit tools stay absolute.
You never run a Bash call in the background: every card gate step, every card `verify:` line and the batch gate runs as a foreground Bash call under the gate rule, and you end your turn only after writing the report.

## Your cards — implement each in declared order, build+test+verify+commit per card

{{.card_pointers}}

For EACH card file listed above, in the order listed:

1. {{.card_read}}
2. A card names its targets under its own type label(s) and what it reads under `**Uses:**`; see `{{.specs_dir}}/loom/loom-plan-spec.md` for the full grammar. Make exactly the changes the card describes, in exactly the targets its type labels name.
   A deviation is a change outside the batch's card targets as the scope guard counts them: a member target covers that member, a file target every member of that file, a package target every member of the files directly in its directory, and a rename is inside when either its old or its new name is a target.
   `**Uses:**` entries add no coverage, so a change to a member a card only reads is a deviation unless a target of some card in the batch covers it.
   Three deviations are sanctioned, each within its bound: the guard-step fix in step 3, only in a file the guard step's failure names; the scan-error fix the guard-fix line in the card gate names, only in that file and under the card it names; and a fix to a failure the card caused, under the bounded self-fix section's two-pronged test and its attempt cap.
   No other change outside the targets is made.
   The scope guard records every deviation, sanctioned ones included; you list, compare and report none, and a deviation alone never makes the report `FAILED`.
   For each symbol pair of a `Rename` card, run the hub's rename tool from the list below, then grep for zero references to the old name and update the doc-comment opening; a file pair is `git mv` plus surgical Edits.

{{.rename_tools}}

{{.locate}}
3. Run each step of the card's gate from the list below, in order, each as its own foreground Bash call under the gate rule in your system prompt.
   A failure here is the card's own gate — fix it before moving on;
   this gate is implicit in every card, never optional.

{{.card_gates}}

   A card gate's guard step, the `lyx gate test --tags integration … -- -run` step over the repository's `//lyx:guard` tests, can fail on a file the card does not list, such as the pinned set in a `cmd/lyx` test.
   Fix it under the card's commit.
   Fix the code that broke the guard where that is the right fix, and change a pinned set or an allowlist only when the pin itself must move.
   Change only a file the guard step's failure names this way, the first sanctioned deviation in step 2's rule.

4. Commit the card to the repo — normal dev git, run from your working directory — never any `_lyx` path.
   One commit per card is the norm.
   The commit subject is `N: <name>` — the card's own number and heading name (e.g. `1: alpha`) — unless the card FILE carries a `**Commit:**` line, which pins the exact subject to use verbatim.
   This subject shape is the plan's resume trail: a fresh session reads from `git log` exactly which card was reached.
   You never call the Agent tool yourself (no nested forks — this is banned),
   and you are never passed a name of your own when spawned.
5. If the card declares its own `verify:` line, run it immediately after committing that card, each line of it as its own foreground Bash call under the gate rule.
   A non-zero exit fails the card exactly like the gate in step 3 — there is no separate "deferred verify" concept;
   every card's gate (the steps above, plus its own `verify:` when it declares one) is checked right after that card's own commit, never bundled into a later card.

A card that departs from its own text because a PATTERN entry forbids the card's way records the departure as a `Pattern-Departure:` trailer in that card's commit, in the shape the `## Pattern-Departure trailer` section of `{{.specs_dir}}/loom/loom-plan-spec.md` defines.

If any card's gate fails, or a test the card broke fails, and you cannot fix it within your self-fix bound (see next section), stop and report `status: FAILED` — do not continue to a later card on top of a broken one.

## The sandbox rule

Never clone the hub's remotes, and never run a mutating `lyx fabric` verb.
A scratch hub is a Go test fixture over local bare repositories (`hubforge`).
`lyx fabric` refuses its mutating verbs from your strand; the refusal is the rule's owner.
When the work itself needs a refused verb, stop and report `status: FAILED`, so the orch runs the verb.

The card gate steps, the batch gate steps, `lyx loom lint-comments`, `lyx loom lint-pattern`{{.card_pack_allowed}} and `lyx quarry resolve` are always allowed from any strand, and none of them is a fabric reference.
The sandbox ban covers mutating `lyx fabric` verbs only.

## The batch gate — once, after the last card

{{.batch_gate}}

After the last card's commit and that card's own `verify:`, and before writing the report, run the batch gate once, each step as its own foreground Bash call under the gate rule.
A failure counts as a gate failure under the self-fix bound below and makes the report `FAILED` once the bound runs out.

## Slot-busy exits and raw test runs

A step that exits with code `75`, answering `slot_free: false` and naming the slot holders, found every gate slot held: that is not a failure.
It costs no self-fix attempt, never makes the report `FAILED`, and you re-run the same step in a turn that does nothing else, so the prompt cache stays warm between calls.

You may run a raw test run of the hub's language on the batch's listed packages only.
The hub's verify tiers follow:

{{.verify_tiers}}

A tier the list says is compiled rather than run is compiled, never run, as the list says.
With `none`, a raw run is still limited to the listed packages and to what the sentences below allow.
A module-wide, `tmux`-tier or `llm`-tier run, which the shell guard refuses raw, goes through `lyx gate test`.
A `lyx gate test` step runs as written, whatever its `--tags`, so the list's compile-only rule governs raw runs and runs no gate step names, never a rendered card or batch gate step.
A test under the `tmux` or `llm` tag that no gate step names is compiled, never run, through `lyx gate test --tags <tag> <packages> -- -run '^$'`, and the list's compile-only rule adds tiers beyond those two;
running those tiers is left to the gates after webster.

## Bounded self-fix, then stop

If a card's gate (the steps above,
or its own `verify:` when it declares one) or the batch gate fails, you get at most `{{.self_fix_cap}}` in-session fix attempts before you stop trying that card: fix, re-run the gate, and repeat, up to that bound — never more, and never fewer when a fix is plausible.

Any failing test you observe, in the gate or in any other test run you make, counts as that card's gate failure when the card's change caused it.
The card caused a failure when either holds:

- the failing test exercises code or files the card changed;
- a repo scan or enforcement test flags a file the card wrote.

You fix such a failure under the same `{{.self_fix_cap}}`-attempt bound as any gate failure, and report `status: FAILED` when the bound runs out.
A failure the card did not cause is never yours to fix, and it is left to the plan-level verify gate.
The plan's verify command follows:

{{.plan_verify}}

When it is a command, a batch gate step that fails only on failures the batch's cards did not cause, by the two-pronged test above, does not make the report `FAILED`; when it is `none`, such a failure stays `FAILED`.
A misjudged failure passes the batch and is caught by the plan-level verify gate, which runs that command and spawns a fixer when it fails, so the catch covers what the command runs.

## Your final action: the minimal batch-report

Your LAST action of this session — after every card above is committed (or you have given up per the bound above) — is writing the batch-report YAML file to `{{.report_path}}`.
Nothing you do after this file exists is read by anyone: write it last, and write it exactly once.
The report is deliberately minimal — `record-batch` reads ONLY these two fields:

```yaml
status: OK | FAILED
head_sha: <the commit SHA your worktree is at right now>
```

`status` is `OK` when every card above is committed and every gate it ran, the batch gate included, passed;
`FAILED` when you stopped after exhausting the self-fix bound on some card or on the batch gate, whether its gate failed or a test the card broke did. `head_sha` is your worktree's current HEAD commit SHA — capture it with `git rev-parse HEAD` as your very last read before writing the report, so it reflects every commit you made.
Only a failed card gate, a failed card `verify:`, a failed batch gate or a test the card broke makes `status` `FAILED`, never a slot-busy exit.
A batch gate step that fails only on failures the batch's cards did not cause does not make it `FAILED` when the plan's verify command above is a command, and does when it is `none`.{{.kickstart_pack}}
