MILL_REVIEW_BEGIN
# Review: loom: code-writing skills — comments, build, testing

```yaml
verdict: REQUEST_CHANGES
reviewer_model: opusmedium
reviewer_self_id: claude-opus-5 (self-assessment; matches the model id declared to me)
reviewed_file: _mill/discussion.md
date: 2026-08-24
```

## Findings

### [BLOCKING:decision] golang-build/golang-testing have no disposition
**Section:** Decisions / Problem
**Issue:** Two of the seven shipped skills get no Decision entry at all, while Problem asserts millhouse's skills "are outdated in several concrete places" and `manifest/roadmap.md:21` states these two are "close to a verbatim port of millhouse's equivalents" — so the discussion neither names the outdated places nor says whether the port carries them.
**Fix:** Add a Decision naming what was ported verbatim, what was changed, and which of the "outdated in several concrete places" items apply to these two files.

### [BLOCKING:consistency] golang-build's toolchain contradicts this repo
**Section:** Scope / Testing
**Issue:** `plugins/scribe/skills/golang-build/SKILL.md` mandates `goimports -w` + `golangci-lint run` and says "If either is missing, report ... then stop" — neither tool appears anywhere else in the repo (no `.golangci.*`, no CI workflow, no doc reference; grep finds the SKILL.md only) — and its `go test ./...` "run all tests found" default contradicts `docs/benchmarks/running-tests.md`'s three documented invocations (`./...`, `-tags integration`, `-tags smoke`).
**Fix:** State whether the skill defines the toolchain this repo should adopt or must be reconciled with the existing two-tier scheme, and record that as a Decision.

### [BLOCKING:consistency] prose bans "any" as an empty intensifier
**Section:** Decisions — "prose and conversation split out of comment content"
**Issue:** `prose/SKILL.md:21` lists "any" among empty intensifiers under a "remove the word, if it means the same, delete it" test, but "any" is a determiner — prose itself uses it load-bearingly four times (`any multi-line prose`, `any skill`, `any text`), as do the sibling skills, so the rule as written is self-refuting and undercuts Testing's "read against its own stated rules" claim.
**Fix:** Decide whether "any" stays on the list, and if so on what non-intensifier grounds.

### [NIT:consistency] Design doc keeps a dangling cross-reference
**Section:** Decisions — "Design doc points to the skill, not the reverse"
**Issue:** `manifest/designs/code-comment-conventions.md:38` still says "excluding the two exceptions above", but the exceptions moved into the skill and no longer appear in that document.
**Fix:** Note the leftover reference so the follow-on plan resolves it with the rest of the pointer rewrite.

### [NIT:consistency] Pointer-Rule citation rests on a false premise
**Section:** Decisions — "Design doc points to the skill, not the reverse"
**Issue:** The decision says the Producer Pointer-Rule Invariant was cited against the design doc, but `CONSTRAINTS.md`'s own scope bullet exempts "design docs restating the rule for a human reader" — the outcome survives on the portability argument, the invariant argument does not.
**Fix:** Drop or correct the invariant citation and rest the decision on portability alone.

### [NIT:design] Hook command's shell portability unstated
**Section:** Decisions — "Always-active mechanism"
**Issue:** `hooks/hooks.json` ships a POSIX single-quoted `echo`, in a repo whose whole `internal/shell` seam exists because pwsh and posix quoting differ; the decision never says whether the nudge is expected to work on the Windows targets fabric supports.
**Fix:** State the intended platforms for the hook, or note portability as an accepted open item.

## Verdict

REQUEST_CHANGES
Three blocking gaps: unported-skill disposition, a toolchain contradiction, and a self-refuting prose rule.
MILL_REVIEW_END
