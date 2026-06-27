MILL_REVIEW_BEGIN
# Review: Built-in CLI help: lyx self-documents modules & commands

```yaml
verdict: GAPS_FOUND
reviewer_model: opushigh
reviewed_file: _mill/discussion.md
date: 2026-06-27
```

## Findings

### [GAP] Single-writer RunCLI seam vs cobra stdout/stderr split
**Section:** Decisions → integration-style-c-preserve-seam / exit-and-error-contract; Testing
**Issue:** `unknown-command-human-text` routes cobra errors+suggestions to **stderr**, but the preserved adapter `RunCLI(out io.Writer, args) int` exposes one writer and the shown body only does `c.SetOut(out)` — so it is unspecified where cobra's stderr text lands and how in-process tests (the named weft/ide unknown-command assertions) capture it. The binary must split stdout/stderr; the seam cannot.
**Fix:** State whether the adapter wires `c.SetErr(out)` (merging, so tests can assert) while `main.go` keeps `SetErr(os.Stderr)` separate, and acknowledge the unknown-command exact text differs (`for "board"` via seam vs `for "lyx board"` via root) so tests don't over-pin it.

### [NOTE] `--json` undefined on non-help / real-execution paths
**Section:** Decisions → json-help-form
**Issue:** `--json` is a persistent root flag inherited by every leaf, but its effect is defined only "on a help path"; `lyx board list --json` (real handler) leaves `--json` parsed-but-unused, with no statement of no-op vs reject (real output is already JSON).
**Fix:** Declare `--json` a no-op on non-help execution paths (or an error), so a plan writer doesn't have to guess.

### [NOTE] muxpoc shared pre-dispatch is not a verbatim per-case move
**Section:** Technical context (muxpoc); Decisions → flags-to-pflag
**Issue:** muxpoc builds `cfg` from the tuning flags + `paths.Resolve(layout)` once before the switch (cli.go:54–94); under cobra this shared setup has no `RunE` home, and running `paths.Resolve` on the new no-arg listing path would wrongly require a git repo.
**Fix:** Note that muxpoc's pre-dispatch setup moves to a `PersistentPreRunE` (which cobra skips on the no-arg/help listing), not "verbatim into RunE bodies".

### [NOTE] Tree-walk tests must tolerate cobra's auto commands
**Section:** Testing → drift-guard / help-tree completeness
**Issue:** Cobra auto-adds `help` and `completion` (with its `powershell`/`bash`/… children); a pinned exact-set equality check or naive Short-walk would also see these.
**Fix:** Say the pinned sets are superset checks (or explicitly exclude `help`/`completion`) to avoid brittleness.

## Verdict
GAPS_FOUND
Adapter must define how cobra's stderr surface flows through the single-writer seam.
MILL_REVIEW_END