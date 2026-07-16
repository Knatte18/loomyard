MILL_REVIEW_BEGIN
# Review: Built-in operator console pane in mux

```yaml
verdict: GAPS_FOUND
reviewer_model: opushigh
reviewed_file: _mill/discussion.md
date: 2026-07-16
```

## Findings

### [GAP] `lyx mux header` vs JSON-envelope invariant
**Section:** three-part text pipeline (Q2/Q6/Q7/Q13); Constraints
**Issue:** The verb prints raw token-substituted header text then blocks forever, but the CLI/Cobra Invariant requires all output via the `internal/output` JSON envelope, and the existing interactive-handoff exception only covers handing stdio to *another* interactive program (not a self-displaying blocking verb).
**Fix:** Decide and record how `lyx mux header` is exempted from the envelope rule (extend the interactive-handoff carve-out explicitly, with pre-flight errors staying on the envelope).

### [GAP] `repo` token derivation undecided
**Section:** three-part text pipeline; Technical context (repo derives from WorktreeRoot/HubSuffix)
**Issue:** Two derivations are offered — "basename of `WorktreeRoot`" vs "the `HubSuffix` convention" — which yield different strings (worktree/task name vs stripped repo name), and `Hub = filepath.Dir(WorktreeRoot)` is not guaranteed to be a `-HUB` dir, so "`repo` always-resolvable" is unverified.
**Fix:** Pick one derivation and confirm it is always non-empty, so the TDD token spec is testable and stencil's strict-unfilled check won't error.

### [GAP] HeaderText() error path vs keepalive guarantee
**Section:** header pane is the persistent keepalive (Q8); pure-header (Q1)
**Issue:** If `HeaderText()` fails (bad template, unresolvable/empty token via stencil's strict error), it is unspecified whether `lyx mux header` still blocks (keepalive holds) or exits (structural keepalive lost at boot).
**Fix:** State that the verb blocks even on render failure (or relies on remain-on-exit corpsing), so the "always-on, structural" guarantee survives a template error.

### [NOTE] Header config lives in GOOS-selected template files
**Section:** header-config-block + asset naming (Q4/Q12/Q14)
**Issue:** The `header:` block is said to go in `template.go ConfigTemplate`, but the actual embedded YAML is in `template_posix.go`/`template_windows.go`; `template.go` is only an accessor.
**Fix:** Note that both GOOS template assets must gain the `header:` block.

### [NOTE] Oversized header height starves strand stack
**Section:** top-band-in-render (Q3)
**Issue:** `height_rows` is config-driven; a value near/over window height shrinks the strand `Box` to zero, but the clamp/failure behaviour (vs `MinFullRows` in height.go) is unaddressed.
**Fix:** Specify clamping or a max on header height so strands retain minimum rows.

### [NOTE] Testing a forever-blocking verb
**Section:** Testing (`lyx mux header` verb)
**Issue:** "prints HeaderText() output and blocks" gives no mechanism (context/signal) to test the verb without hanging the test.
**Fix:** Name how the block is made test-observable (e.g. context cancellation or a testing seam).

## Verdict

GAPS_FOUND
Envelope exemption, `repo` derivation, and header error-path vs keepalive must be resolved before planning.
MILL_REVIEW_END
