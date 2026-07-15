MILL_REVIEW_BEGIN
# Review: Decide tmux mouse-mode default for lyx mux — holistic

```yaml
verdict: APPROVE
reviewer_model: sonnethigh
reviewed_file: plan/ + source
date: 2026-07-15
```

## Findings

None. Verified against the plan's six cards:

- Card 1: `mouseOption` in `internal/muxengine/mouse.go:23-32` trims/lowercases, returns `"on"`/`"off"`, errors on every other value including `""`; godoc mirrors `debugLogArgs`' shape and states the empty-string-is-error contract explicitly. `mouse_test.go` is table-driven, covers case/whitespace variants and the four invalid cases (`yes`, `1`, `banana`, `""`).
- Card 2: `Config.Mouse string \`yaml:"mouse"\`` (`config.go:52`) sits adjacent to `DebugLog` with a matching-shape comment. Both `template_posix.yaml:10` and `template_windows.yaml:10` carry byte-identical `mouse: ${env:LYX_MUX_MOUSE:-off}` lines with the required inline documentation — no drift between the two templates.
- Card 3: `ensureServerAndSessionLocked` (`lifecycle.go:188-203`) validates `mouseOption(e.cfg.Mouse)` immediately after `debugLogArgs`, before the capability probe; the resolved value is applied unconditionally via `set-option -g mouse <resolved>` (`lifecycle.go:359-361`) right after `remain-on-exit`, wrapped as `"set mouse: %w"`. The already-up early return (`lifecycle.go:218-233`) is untouched, preserving no-live-toggle semantics.
- Card 4: `config_test.go:82-84` asserts `cfg.Mouse == "off"` in the existing `TestLoadConfig_TemplateDefaultsResolve`, following the same assertion style as the other defaulted fields.
- Card 5: `mouse_boot_integration_test.go` is `//go:build integration`-tagged, self-skips on missing binary, builds `Engine` via `New(cfg, layout)` on a temp-dir `hubgeometry.Layout`, tears down via `t.Cleanup`/`kill-server`, and covers both the fresh-boot-pins-both-directions case and the no-live-toggle-without-restart case exactly as specified; `readMouseOption` mirrors `contract_integration_test.go`'s command-construction style.
- Card 6: `doc.go`'s "Subcommand set" sentence now lists `set-option -g mouse` (line 48), and a new "Mouse boot pin" bullet (lines 87-93) matches the style/content of the "Dead-pane adoption via remain-on-exit" bullet.

Shared Decisions (mouse-value-contract, explicit-set-both-ways-at-boot, helper-lives-in-mouse.go, docs-target-reconciliation, integration-test-gating) are applied consistently; no cross-batch contract issues (single batch, self-contained); no out-of-plan files — all 15 provided files map to the plan's "All Files Touched" list plus declared context files; no constraint violations found (Hub Geometry, Test Tier Purity, Hermetic Git Env invariants all respected — no raw git/exec spawns added to untagged files, no geometry-token literals introduced).

## Verdict

APPROVE
Implementation matches the plan's six cards and Shared Decisions exactly; no blocking or nit findings.
MILL_REVIEW_END
