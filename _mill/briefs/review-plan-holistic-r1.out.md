I have completed my verification. Here is my review.

MILL_REVIEW_BEGIN
# Review: Rename Cobra modules to <module>cli, extract kernels as <module>engine — holistic

```yaml
verdict: REQUEST_CHANGES
reviewer_model: opushigh
reviewed_file: plan/
date: 2026-06-29
```

## Findings

### [BLOCKING] board cards 1 & 3 mishandle external `board_test` files
**Location:** Batch 1, Cards 1 and 3
**Issue:** Both cards instruct a blanket package-clause change `package board → package boardengine`/`boardcli`, but `board_test.go`, `store_test.go`, `task_test.go`, `layer_test.go`, `render_test.go`, `config_test.go` (card 1) and `cli_test.go`, `help_test.go` (card 3) are `package board_test` (external) — the literal instruction never matches them, and forcing them to internal `boardengine`/`boardcli` is illegal (self-import / two-package dir), breaking the build.
**Fix:** Adopt warp card 9's explicit wording: external `board_test` files become `boardengine_test` / `boardcli_test`; only the internal files (`template_test.go`, `skipenv_internal_test.go`, `cli.go`) become `boardengine`/`boardcli`.

### [NIT] card 20 reconcile_test package vs. qualifier mismatch
**Location:** Batch 7, Card 20
**Issue:** The card declares the test `package configcli` yet writes `configcli.RunCLI(&buf, ...)`; an internal-package file cannot self-qualify (the original `update_test.go` is internal `package update` and calls `RunCLI` unqualified).
**Fix:** Either call `RunCLI` unqualified under `package configcli`, or make the file `package configcli_test` and keep the `configcli.RunCLI` form.

## Verdict

REQUEST_CHANGES — board test-package split instruction is inconsistent with warp's and breaks the build as written.
MILL_REVIEW_END