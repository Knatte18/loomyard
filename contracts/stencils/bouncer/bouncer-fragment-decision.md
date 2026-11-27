<!-- This is the decision detail of the Bouncer's escalation brief for cause `decision`: the questions the review cannot settle itself, the BLOCKING findings the round's review holds, the refusal the recorded answer would meet, and the verbs that record it.
     renderDecisionDetail fills it and passes the result as the brief's cause_detail marker, after the cause list.
     Every marker below is a top-level {{.X}} substitution: decision_findings, blocking_findings, refusal, answer_route and decide_verbs, each Go-filled and non-empty. -->

### Decision findings

Each finding below is a question the review could not settle itself.
Where an arbiter runs, it settles each finding the board entry and the decision record answer before the parent sees it.
Settle each remaining finding yourself when it is a run-level call, and put it to the operator only when it is a design question, unless a recorded operator decision answers it.

{{.decision_findings}}

### BLOCKING findings of the round's review

{{.blocking_findings}}

### Refusal

{{.refusal}}

When a refusal is named, the round is re-judged before the recorded answer acts while budget is left.
An `accept` passes the round over every BLOCKING finding listed once a judgment is admitted, except that over a CONTINUE or CIRCLING verdict an open BLOCKING ledger entry of another class holds it: the round then runs another round, or inside the soft-cap window escalates again with cause budget, which only the parent accepts. An `accept` at or past the hard cap halts the run if the refusal still holds at the budget.
A `continue` runs another round whose fresh reviewers re-review the artifact, so a listed BLOCKING finding reaches that round's fixer only if they raise it again.

### Way forward

{{.answer_route}}

{{.decide_verbs}}

Then resume the run with `lym loom resume`.
