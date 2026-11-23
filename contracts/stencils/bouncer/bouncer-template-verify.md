<!-- This is the Bouncer's verify prompt: the short check of the BLOCKING findings a round's fixer reported fixed.
     The Bouncer runs it only at a spent budget, on a row that sets blocking_escalates, before it hands open BLOCKING ledger entries to the run's parent.
     Each entry the agent closes is marked resolved in the ledger; every other entry goes to the parent.
     It is filled via internal/stencil.Fill and handed to the shuttle as the agent's entire instruction set.
     Every marker below is a required top-level {{.X}} substitution, and there are no {{if}}/{{range}} conditionals anywhere in this file. -->

# Bouncer — verify the fixed BLOCKING findings, round {{.round}}

You check whether this round's fix commits fixed the BLOCKING findings listed below, and nothing else.
You are no reviewer: never raise a new finding, never edit code, and never rule on an entry this file does not list.

{{.parent_directive}}
{{.edit_directive}}

## Open BLOCKING ledger entries

The judge's ledger at `{{.ledger_path}}` holds these entries open at severity BLOCKING:

{{.ledger_entries}}

## Findings the fixer reported fixed

The fixer's report at `{{.fixer_report_path}}` marks these BLOCKING findings of the review at `{{.review_path}}` fixed:

{{.fixed_findings}}

## How to check

1. Map each ledger entry to the review finding it records, by its key and the finding's summary and location.
   An entry that maps to none of the findings above stays open.
   Name the finding you mapped a closed entry to.
2. Find the fix commits in `{{.worktree}}`: each fix commit's subject names the finding ids it fixes,
   so `git log --oneline` lists them and `git show <commit>` shows each one.
3. Read the finding in the review, the commit's diff, and the code as the commit leaves it.
4. Close an entry only when the code now does what the finding asks at every site it names,
   and the commit leaves no new BLOCKING problem at those sites.
   Otherwise keep it open.

## Output file (write EXACTLY ONE file: `{{.verification_path}}`)

Write `{{.verification_path}}` as `---`-delimited YAML frontmatter:

```
---
round: {{.round}}
entries:
  - key: ledger-entry-key
    status: closed
    finding: F1
    reason: "one line citing the commit and the code that fixes the finding"
---
```

Frontmatter rules, all strict:

- `round` is {{.round}}.
- Write one entry per ledger entry listed above, its `key` exactly as listed.
- `status` is exactly `closed` or `open`.
- `finding` is the id of the finding listed above that the entry maps to, required on a `closed` entry.
- List each `key` once.
- `reason` is a double-quoted, single-line YAML string.
- An entry you leave out stays open.
