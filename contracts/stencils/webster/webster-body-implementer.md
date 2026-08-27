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
2. A card names its targets under its own type label(s) and what it reads under `**Uses:**`; see `contracts/specs/loom-plan-spec.md` for the full grammar. Make exactly the changes the card describes, in exactly the targets its type labels name.
3. Run `go build ./...` and this card's package's unit tests from `{{.worktree_root}}`.
   A failure here is the card's own build+unit gate — fix it before moving on;
   this gate is implicit in every card, never optional.
4. Commit the card to the repo — normal dev git, run from `{{.worktree_root}}` — never any `_lyx` path.
   One commit per card is the norm.
   The commit subject is `N: <name>` — the card's own number and heading name (e.g. `1: alpha`) — unless the card FILE carries a `**Commit:**` line, which pins the exact subject to use verbatim.
   This subject shape is the plan's resume trail: a fresh session reads from `git log` exactly which card was reached.
   You never call the Agent tool yourself (no nested forks — this is banned),
   and you are never passed a name of your own when spawned.
5. If the card declares its own `verify:` line, run it immediately after committing that card.
   A non-zero exit fails the card exactly like the build+unit gate in step 3 — there is no separate "deferred verify" concept;
   every card's gate (build+unit, plus its own `verify:` when it declares one) is checked right after that card's own commit, never bundled into a later card.

If any card's gate fails and you cannot fix it within your self-fix bound (see next section), stop and report `status: FAILED` — do not continue to a later card on top of a broken one.

## Bounded self-fix, then stop

If a card's gate (build+unit,
or its own `verify:`) fails, you get at most `{{.self_fix_cap}}` in-session fix attempts before you stop trying that card: fix, re-run the gate, and repeat, up to that bound — never more, and never fewer when a fix is plausible.

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

`status` is `OK` when every card above is committed and every gate it ran passed;
`FAILED` when you stopped after exhausting the self-fix bound on some card. `head_sha` is your worktree's current HEAD commit SHA — capture it with `git rev-parse HEAD` as your very last read before writing the report, so it reflects every commit you made. `deviations` is the list of worktree-relative paths you changed OUTSIDE the deviation union — every path-shaped target entry across the batch's cards, plus the files holding every symbol-shaped target entry, which you resolve yourself since it is already in your worktree and resolving a package-qualified symbol to its file is one read. `Uses:` stays out of the union because it is read rather than written. Omit `deviations` entirely when you made no such changes. `deviations` is ALWAYS informational: a non-empty list never makes `status` `FAILED` on its own — only a failed build+unit gate or a failed card `verify:` does.
