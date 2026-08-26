MILL_REVIEW_BEGIN
# Review: reed: attach doesn't reconcile session geometry with the terminal

```yaml
verdict: REQUEST_CHANGES
reviewer_model: opusmedium
reviewer_self_id: Claude (Anthropic), Opus-class model
reviewed_file: _mill/discussion.md
date: 2026-08-26
```

## Findings

### [BLOCKING:design] Wipe guards are build-time; the apply is post-handover
**Section:** `chain-skipped-when-the-layout-apply-would-be` / `attach-chains-select-layout`
**Issue:** the layout string is planned (and `anyPlacedStrand` / `len(live) < 2` evaluated) in the CLI pre-flight, but the chained `select-layout` executes seconds later, after handover and outside the op lock — in the `lyx loom run` case the loom driver is by the discussion's own words "driving reed ops continuously", so panes can be added/killed in between; the layout body enumerates bare pane numbers (`render/layout.go:39`), so a stale string can be applied against a changed pane set, which is the same class of act the guards exist to prevent.
**Fix:** state the disposition for the build-vs-apply window — e.g. bound it (plan under the lock immediately before exec and accept the residual race explicitly, naming what tmux does with a layout naming a dead pane or omitting a live one), or drop the chain when a driver is live.

### [BLOCKING:consistency] `AttachArgv` error disposition contradicts the escape-hatch rule
**Section:** `engine-owns-the-attach-argv` vs `chain-skipped-when-the-layout-apply-would-be` / `## Constraints`
**Issue:** the builder is `AttachArgv(cols, rows) ([]string, error)` and the Constraints section says the `AttachArgv` call is fallible and must report on the envelope (i.e. abort before handover), while the rejected-alternatives line insists "attach must stay the operator's escape hatch into a session, including a broken one"; the discussion never says which errors the builder returns (state read, `list-panes` failure, op-lock contention against a live loom driver) versus which degrade to the bare argv.
**Fix:** state the rule — either the builder never returns an error that blocks attach (degrade to bare argv on every engine-side failure) or name the exact error classes that do abort pre-flight.

### [BLOCKING:design] Boot-path failure mode for the two new set-options unstated
**Section:** `geometry-options-pinned-at-boot-and-attach`
**Issue:** the decision says the attach-side pins are "idempotent and non-fatal" but says nothing about the boot path, whose neighbouring pins are hard-fatal (`lifecycle.go:399-410` returns an error for `remain-on-exit` and `mouse`); since psmux's support for `status` and especially `window-size` is explicitly unverified by this discussion, a fatal boot-path `set-option` would take reed boot down entirely on Windows.
**Fix:** state the boot-path disposition for both new options (fatal or non-fatal, with the psmux reasoning), rather than leaving a plan writer to copy the adjacent fatal pattern.

### [NIT:scope] Doc inventory names reed's help only
**Section:** `## Constraints` (Documentation Lifecycle) / `## Scope`
**Issue:** `docs/overview.md:298` and `:318` both describe the `attach-session` handover for reed and loom, and `lyx loom run`'s own `Long` describes the same handover, but the doc inventory lists only `reedengine/doc.go`, both `reed.yaml` templates, and `attach`'s `Long`.
**Fix:** say explicitly that loom's `run` help and the two `docs/overview.md` bullets are checked and left unchanged, or add them to the inventory.

## Verdict

REQUEST_CHANGES
Three unstated dispositions: post-handover apply race, builder error handling, boot-path option failure.
MILL_REVIEW_END
