# Webster implementer job — read your cards, implement, commit, report

## The FRESH-READ rule

Inherited context can be stale.
A file you looked at during orientation — your own, or one Master read before forking you — is not necessarily the version on disk right now: a prior batch's own card commits may have changed it since.
Before you edit anything, re-read — in THIS turn — every card file this section points you at, plus every file that card's own target list and `Uses:` list name.
Only your own reads, taken now, are current;
content you merely inherited is not.

## Prior-batch context

{{.prev_digest}}

This is the immediately preceding batch's own persisted digest, rendered as a fixed one-line summary by `begin-batch` — the literal string `none (first batch)` when you are the first executed batch's fork.
It is Go-rendered from the persisted record, never something you need to go derive yourself.

## Your cards — implement each in declared order, build+test+verify+commit per card

{{.card_pointers}}

For EACH card file listed above, in the order listed:

1. Read the card file.
   It is your whole instruction for that card.
   If its `**Intent:**` field is empty, fall back to that card's one-line intent from the Card Index in `_lyx/plan/00-overview.md`, matched by the same NN/slug.
2. A card names its targets under its own type label(s) and what it reads under `**Uses:**`; see `{{.specs_dir}}/loom/loom-plan-spec.md` for the full grammar. Make exactly the changes the card describes, in exactly the targets its type labels name.
   Locate them with one `lyx quarry resolve <glyph>...` call over the card's target and `Uses:` glyphs: each answer gives the member's `file` and its `start` and `end` lines, so read those spans instead of grepping for them or reading whole files.
   A `plan:` handle names a member the card creates and does not resolve yet.
3. Run each step of the card's gate from the list below, in order, from `{{.worktree_root}}`.
   Each step runs as its own Bash call with `run_in_background`, and you wait for its exit, so neither a slot wait nor the run is killed by the Bash tool's timeout.
   A failure here is the card's own gate — fix it before moving on;
   this gate is implicit in every card, never optional.

{{.card_gates}}

4. Commit the card to the repo — normal dev git, run from `{{.worktree_root}}` — never any `_lyx` path.
   One commit per card is the norm.
   The commit subject is `N: <name>` — the card's own number and heading name (e.g. `1: alpha`) — unless the card FILE carries a `**Commit:**` line, which pins the exact subject to use verbatim.
   This subject shape is the plan's resume trail: a fresh session reads from `git log` exactly which card was reached.
   You never call the Agent tool yourself (no nested forks — this is banned),
   and you are never passed a name of your own when spawned.
5. If the card declares its own `verify:` line, run it immediately after committing that card, each line of it as its own Bash call with `run_in_background`, waited for.
   A non-zero exit fails the card exactly like the gate in step 3 — there is no separate "deferred verify" concept;
   every card's gate (the steps above, plus its own `verify:` when it declares one) is checked right after that card's own commit, never bundled into a later card.

If any card's gate fails, or a test the card broke fails, and you cannot fix it within your self-fix bound (see next section), stop and report `status: FAILED` — do not continue to a later card on top of a broken one.

## The sandbox rule

Never clone the hub's remotes, and never run a mutating `lyx fabric` verb.
A scratch hub is a Go test fixture over local bare repositories (`hubforge`).
`lyx fabric` refuses its mutating verbs from your strand; the refusal is the rule's owner.
When the work itself needs a refused verb, stop and report `status: FAILED`, so the orch runs the verb.

## The batch gate — once, after the last card

{{.batch_gate}}

After the last card's commit and that card's own `verify:`, and before writing the report, run the batch gate once, each step as its own Bash call with `run_in_background`, waited for.
A failure counts as a gate failure under the self-fix bound below and makes the report `FAILED` once the bound runs out.

## Slot-busy exits and raw `go` runs

A step that exits with code `75`, with a JSON error naming the slot holders, found every gate slot held: that is not a failure.
It costs no self-fix attempt, never makes the report `FAILED`, and you re-run the same step.

You may run raw `go test` on the batch's listed packages only.
A module-wide run or a `tmux`-tier run goes through `lyx gate test`, never a raw `go` command.

## Bounded self-fix, then stop

If a card's gate (the steps above,
or its own `verify:` when it declares one) or the batch gate fails, you get at most `{{.self_fix_cap}}` in-session fix attempts before you stop trying that card: fix, re-run the gate, and repeat, up to that bound — never more, and never fewer when a fix is plausible.

Any failing test you observe, in the gate or in any other test run you make, counts as that card's gate failure when the card's change caused it.
The card caused a failure when either holds:

- the failing test exercises code or files the card changed;
- a repo scan or enforcement test flags a file the card wrote.

You fix such a failure under the same `{{.self_fix_cap}}`-attempt bound as any gate failure, and report `status: FAILED` when the bound runs out.
A failure the card did not cause is never yours to fix and never a `FAILED` on its own: name it in your final reply to Master and move on.

## Your final action: the minimal batch-report

Your LAST action of this session — after every card above is committed (or you have given up per the bound above) — is writing the batch-report YAML file to `{{.report_path}}`.
Nothing you do after this file exists is read by anyone: write it last, and write it exactly once.
The report is deliberately minimal — Master reads ONLY these three fields:

```yaml
status: OK | FAILED
head_sha: <the commit SHA your worktree is at right now>
deviations:
  - <path>
```

`status` is `OK` when every card above is committed and every gate it ran, the batch gate included, passed;
`FAILED` when you stopped after exhausting the self-fix bound on some card or on the batch gate, whether its gate failed or a test the card broke did. `head_sha` is your worktree's current HEAD commit SHA — capture it with `git rev-parse HEAD` as your very last read before writing the report, so it reflects every commit you made. `deviations` is the list of worktree-relative paths you changed OUTSIDE the deviation union — the batch's own target glyphs across its cards, reported as the paths you touched rather than resolved by you: under the glyph alphabet a symbol-shaped target is a glyph, not a package-qualified name, and the mechanical glyph scope guard (`internal/planglyph`'s `ScopeGuard`, run over the record-batch delta) is what actually compares your work against the union — your own job here is only to report what you touched, never to estimate the comparison yourself. `Uses:` stays out of the union because it is read rather than written. Omit `deviations` entirely when you made no such changes. `deviations` is ALWAYS informational: a non-empty list never makes `status` `FAILED` on its own — only a failed card gate, a failed card `verify:`, a failed batch gate or a test the card broke does, never a slot-busy exit.
