# Review: reed: attach doesn't reconcile session geometry with the terminal

```yaml
verdict: APPROVE
reviewer_model: orchestrator
reviewed_file: _mill/discussion.md
date: 2026-08-26
```

## Findings

### [NIT:consistency] Config Strictness Invariant citation misdescribes actual enforcement
**Demoted-from:** BLOCKING
**Section:** Decision `width-height-keys-keep-their-name-and-change-their-meaning`
**Issue:** The rationale claims "the Config Strictness Invariant makes an unknown key a hard failure — a rename is a breaking change to every existing worktree." Verified against source: `internal/configengine/config.go`'s `load()` hard-fails only via `yamlengine.MissingKeys` when the TEMPLATE names a key the on-disk file lacks — it never checks for a key the file has that the template lacks. `internal/reedengine/config.go:50`'s `yaml.Unmarshal(resolved, &cfg)` uses no `KnownFields`/strict decoding, so an extra key is silently dropped at unmarshal time. `internal/configsync`'s `Reconcile` reports such a key as `removed` and drops it on materialize — no error there either. CONSTRAINTS.md's actual "Config Strictness Invariant" section documents `Load` vs `LoadOrTemplate` on an ABSENT `_lyx/`/config file, not unknown keys within an existing one — no mechanism in the repo hard-fails on an unknown key.
**Suggested fix:** Restate the rationale on the true mechanism: renaming `width`/`height` would break existing worktrees because the *new* key name would be MISSING from every already-materialized `reed.yaml`, which `Load`'s real `MissingKeys` check does hard-fail on ("missing keys: ...; run \"lyx config reconcile\"") — while the *old* key's on-disk value would simply go unused (no error, no `KnownFields` rejection). The "keep the names" conclusion likely still holds, but a plan writer needs the real failure mode, not the stated one, particularly since it changes what a migration/compat path would actually need to handle.

## Verdict

APPROVE
One decision's rationale cites a constraint mechanism that does not match verified source behavior; every other decision, file:line reference, and live-tmux fact checked out exactly as stated.
_Note: 1 finding(s) demoted from BLOCKING to NIT by the stage's blocking-class ceiling; current blocking_count is 0._
