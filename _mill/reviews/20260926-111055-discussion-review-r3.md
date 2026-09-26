MILL_REVIEW_BEGIN
# Review: shed: the LLM driver as a generic stepper and mender

```yaml
duration_s: 153.3
verdict: REQUEST_CHANGES
reviewer_model: opusmedium
reviewed_file: _mill/discussion.md
date: 2026-09-26
```

## Findings

### [BLOCKING:design] Interrupted handback row vs repair path undecided
**Section:** Decisions `repair-scope`, `repair-cap` **Issue:** `repair-scope` routes "an interrupted invocation" onto the repair path (which may act through `lyx reed …` on the failed step's strands), but it never says whether the current skill's `interrupt_policy: handback`/absent-policy branch survives. That branch stops because a live agent may still be running, and re-invoking restarts it; `repair-cap` keeps only the re-invoke cap. **Fix:** State the disposition per interrupt sub-case (advanced / `reinvoke` / `handback` or absent policy): does `handback` still hand back unconditionally, or may a repair touch that row's strand?

### [BLOCKING:design] repair-cap row identity unreadable from an error envelope
**Section:** Decision `repair-cap` **Issue:** The cap keys on "same `producer` and `history_length` as the envelope before the first repair", but a five-kind error envelope carries neither field (`internal/shedverbs/step.go` builds it as `kind` plus the three new keys), and an interrupted invocation has no envelope. So the prior success envelope's `producer` names the row that already ran, not the failing one. **Fix:** Name the source of the counting key, for example `current_producer`/`history_length` from a `lyx shed status` read taken before the first repair, the same read the interrupt branch already uses.

### [NIT:consistency] Scope Docs list misses edits decided elsewhere
**Section:** Scope `In` (Docs bullet) **Issue:** `repair-scope` and `## Constraints` commit to amending the Fabric Git Invariant's text, and `recipe-blind-skill` and Technical context move text into `lyx loom start`'s `Long` help (`start.go`) and rewrite `internal/loomcli/cli.go`'s step `Long` help. None of the three appears in the Scope list, which names only the Shed Verb-Set Invariant for `CONSTRAINTS.md`. **Fix:** Add all three to the Scope Docs bullet.

### [NIT:consistency] step.go kind comment contradicts the new unseeded disposition
**Section:** Decision `repair-cap` **Issue:** Only the one-retry wording is slated to drop. But the same comment block in `internal/shedverbs/step.go` also says `KindUnseeded` needs an operator decision and gets no retry, and `repair-scope` now sends `kind: unseeded` down the repair path. **Fix:** Say the whole kind-disposition comment is rewritten to point at the skill, not just the retry sentence.

### [NIT:scope] Status envelope key-set pin not listed
**Section:** Technical context "Pins to update" **Issue:** `internal/loomcli/status_test.go`'s `TestStatusCmd_EnvelopeKeySet` asserts the status envelope's key set in both directions, so adding `trace_dir` breaks it; the pins list and Testing name only the step key-set tests. **Fix:** Add it to the pins list and to the `internal/loomcli` testing bullet.

### [NIT:design] Step-file path vs subshell cd
**Section:** Decisions `recipe-blind-skill`, `orchestrator-fork` **Issue:** Step files are "relative to the driving session's cwd", but every `lyx` call runs as `(cd <drive-dir> && lyx …)`, so a redirect written inside that subshell lands under the drive directory instead. **Fix:** State that the redirect target is absolute, or resolved before the subshell `cd`.

## Verdict

REQUEST_CHANGES
Interrupted-handback disposition and the repair-cap counting key need a decision before planning.
MILL_REVIEW_END
