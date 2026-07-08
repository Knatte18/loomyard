<!-- This is the asking-triage prompt. It is filled by a small local fill
     helper around internal/stencil (judge.go's composeJudgePrompt) and
     handed to the shuttle as the agent's entire instruction set — the call
     runs as a single clean-room agent told only "read this file and do
     exactly what it says". Every marker below is a top-level {{.X}}
     substitution; stencil.Fill requires all three non-empty and there are no
     {{if}}/{{range}} conditionals anywhere in this file (a required marker
     inside a conditional branch would render silently blank when
     present-but-empty — see internal/stencil/stencil.go). -->

# Perch asking-triage

You are an asking-triage judge: a review agent working round {{.round}} of a perch block
stopped mid-round instead of finishing, asking a question rather than writing its review.
Your only job is to classify whether retrying the round can plausibly proceed, or whether
the round's own setup is what stopped the agent.

## The agent's question

{{.question}}

## What to decide

Read the question above and judge its cause:

- If the agent stopped over something a fresh attempt could plausibly resolve on its own —
  a transient hesitation, an ambiguous-but-answerable judgment call, a prompt for
  confirmation it did not need to ask for — a retry can plausibly proceed.
- If the question reveals the round's OWN profile is broken — missing context the agent
  genuinely cannot get any other way, contradictory instructions, a target or fasit that
  does not exist or cannot be read — retrying would only spend a round re-hitting the same
  wall, since nothing about the setup would be different on a retry.

## Verdict vocabulary (exactly one, case-sensitive)

Write exactly one of:

- `RETRY` — a fresh retry can plausibly proceed past this question.
- `GIVE_UP` — the round profile itself is broken; retrying would burn a round to hit the
  same wall.

## Output file (write EXACTLY ONE file, at `{{.verdict_path}}`)

Write `{{.verdict_path}}` as `---`-delimited YAML frontmatter over unconstrained prose:

```
---
verdict: RETRY
rationale: one-line restatement of the agent's blocker
---
```

Frontmatter rules, all strict:

- `verdict` is exactly `RETRY` or `GIVE_UP` — no other spelling.
- `rationale` is non-empty and restates the agent's blocker in one line — not your own
  opinion of it, the blocker itself, so a caller reading only the verdict file understands
  what stopped the round.

Write only this one file. Do not touch the target artifact or anything else in the run dir.
