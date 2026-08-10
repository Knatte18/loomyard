MILL_REVIEW_BEGIN
# Review: gitexec: decide whether RunGit should return a typed error carrying stderr

```yaml
verdict: REQUEST_CHANGES
reviewer_model: opus
reviewer_self_id: Claude (Anthropic), Opus-class model
reviewed_file: _mill/discussion.md
date: 2026-08-10
```

## Findings

### [BLOCKING:design] Pull/Fetch withhold stderr deliberately, test-pinned
**Section:** `gitrepo.run` — the second copy of the shape; the-migration-is-a-two-message-merge
**Issue:** `pull.go:19` and `pull.go:33` are listed among the "five discard stderr" sites, but their godoc (`pull.go:14-17`, `:31`) states raw stderr is *deliberately* not folded in, and `pull_test.go:87` and `:119` fail if `err.Error()` contains `"fatal:"` — a `%w`-wrapped `GitError` would embed stderr and break both.
**Fix:** Add a merge-rule clause beside the sentinel clause for exit paths that intentionally suppress stderr (either keep raw, or use `GitError.ExitCode` without `%w`), and name the pinned tests.

### [BLOCKING:design] Mixed tri-state sites have no stated disposition
**Section:** predicate-sites-are-real-and-must-stay-expressible; `gitrepo` tri-states
**Issue:** `ancestry.go:26` is classified as a predicate keeping the raw form, but its `default:` branch (`ancestry.go:36`) is a genuine failure returning `git exited %d` with stderr discarded — exactly the bare-exit-code class the change exists to close. The rationale's premise ("predicate sites discard stderr correctly — there is no diagnostic because there is no failure") is false for tri-state sites whose non-{0,1} codes are failures.
**Fix:** State the disposition for the mixed class (e.g. checked form plus `errors.As`/`ExitCode == 1` recovery) rather than folding it into "raw, permanently correct".

### [NIT:scope] gitrepo predicate inventory omits `HasUnpushed`
**Demoted-from:** BLOCKING
**Section:** The predicate-site inventory — "`gitrepo` tri-states and quiet probes — 3 call sites"
**Issue:** `push.go:133` (`rev-list --count @{u}..HEAD`) returns `true, nil` on `code != 0` — its godoc says "rev-list errors fold into (true, nil)" — a plain non-zero-exit-as-answer site, yet it appears only in the discard list, never in the shape list the discussion calls the load-bearing evidence.
**Fix:** Add it to the predicate shape list (or state why it is checked), and note that the gitrepo 21 sites need per-site raw/checked shapes, not just a count.

### [BLOCKING:design] "Both boundary assertions must change" depends on an undecided detail
**Section:** guard-test-with-justification-comments → "The Client Boundary guard does not currently tolerate the gitrepo pair"
**Issue:** `gitrepoboundary_test.go:174/:177` only break if the checked sibling calls `gitexec.` directly; a sibling implemented on top of `r.run` leaves `gitexecTotal == 1` and both assertions passing (only the `:167` set-equality would move). The discussion records the two-assertion break "as fact, not as something for the implementer to confirm" without deciding how the sibling is implemented.
**Fix:** Decide whether the gitrepo checked sibling calls `gitexec.Run` directly or wraps `r.run`, and make the guard-change claim conditional on that choice.

## Findings (non-blocking)

### [NIT:design] `Error()` renders args unquoted
**Section:** giterror-shape
**Issue:** `git <args joined by space>` is ambiguous and not copy-pasteable when an arg contains a space (commit messages, `--filter=...`, paths).
**Fix:** Say in the verdict whether joining is bare or `%q`-per-arg, so the implementer is not deciding it.

## Verdict

REQUEST_CHANGES
Four gaps: suppressed-stderr contract, tri-state class, a missing predicate, an overstated guard fact.
MILL_REVIEW_END
