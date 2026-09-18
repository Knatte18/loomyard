MILL_REVIEW_BEGIN
# Review: Rename loom CLI run/drive/step for verb/engine symmetry, plus rename ly-supervise

```yaml
duration_s: 125.0
verdict: REQUEST_CHANGES
reviewer_model: opus
reviewer_self_id: Claude Opus 5 (claude-opus-5)
reviewed_file: _mill/discussion.md
date: 2026-09-18
```

## Findings

### [BLOCKING:design] Scope is a closed whitelist with no catch-all rule
**Section:** §Scope "In" / §Technical context
**Issue:** The "In" list enumerates specific files/packages, yet the discussion itself concedes ~233 hits across ~57 files must each be read; the enumeration is already demonstrably short — `README.md:20,37,42,204` (including its own "Convenience alias: `lyx run` → `lyx loom run`" line) appears nowhere in scope, and verb-naming comments live in packages scope never names: `internal/loomengine/config.go:253`, `internal/loomengine/seed.go:138`, `internal/frictionengine/spec.go:29`, `internal/webstercli/wiring.go:116`, `internal/loomengine/seedownership_test.go:77-78`, `internal/loomshed/seed_test.go:95,101`.
**Fix:** Replace the file whitelist with a stated enumeration *rule* (e.g. "every hit of a bounded grep set across the whole tree, each read and classified verb-vs-noun"), naming only the exclusions and the known false-positive sites.

### [NIT:scope] Third refusal string omitted; "Both" is false
**Demoted-from:** BLOCKING
**Section:** §Technical context "pause.go:40 emits the same shape…Both refusal strings" / §Testing "Refusal-text coverage"
**Issue:** `internal/loomcli/status.go:116` emits the same unseeded-status refusal naming `lyx loom run` — a third operator-facing site the discussion never lists, and the Testing section's two scenarios (foreground `run`, `pause`) leave it uncovered.
**Fix:** Add `status.go`'s refusal to the technical inventory and to the refusal-text coverage scenarios, and drop the "Both" framing.

### [NIT:decision] No disposition for historical `ly-supervise` mentions
**Demoted-from:** BLOCKING
**Section:** §Decisions `skill-renamed-to-ly-drive` / §Scope
**Issue:** Scope covers the skill only under `plugins/ly/…` plus `INDEX.md`, and the designs bullet is keyed to "a renamed verb" — the skill name is not a verb; unaddressed are retrospective records asserting what *already shipped*: `manifest/roadmap.md:120,157` (Done entries), `manifest/designs/loom-step.md:3,22` (which names the literal path `plugins/ly/skills/ly-supervise/SKILL.md`), plus `reed-mailbox.md:7,15`, `reed-header-selvage.md:72`, `self-report-tier1.md:17`, and Go comments `internal/shedadapters/bouncer_seed_test.go:475`, `internal/loomcli/smoke_bootstrapwiring_test.go:147`.
**Fix:** State one rule for historical/Done prose — rewrite retroactively, or preserve the old name with a one-time "(now `ly-drive`)" gloss — and extend the scope rule to skill-name hits outside `plugins/`.

### [NIT:consistency] Refusal-text test presented as new; it exists
**Section:** §Testing "Refusal-text coverage"
**Issue:** "Cover both in `internal/loomcli`'s own tests so a stale verb name … fails at `go test`" reads as new work, but `internal/loomcli/cli_test.go:129-151` `TestVerbRefusals` already covers drive and pause with `wantRemedy: "lyx loom run"`.
**Fix:** Reframe as "adapt `TestVerbRefusals`'s expected remedy and add a `status` row" so the plan writer does not author a duplicate test.

## Verdict

REQUEST_CHANGES
Enumeration method, a third refusal site, and historical skill-name prose all need resolving.
_Note: 2 finding(s) demoted from BLOCKING to NIT by the stage's blocking-class ceiling; current blocking_count is 1._
MILL_REVIEW_END
