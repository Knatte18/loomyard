MILL_REVIEW_BEGIN
# Review: self-report Tier 2: per-agent friction notes for unsupervised runs

```yaml
duration_s: 203.0
verdict: REQUEST_CHANGES
reviewer_model: opus
reviewer_self_id: Anthropic Claude, Opus-class (runtime reports claude-opus-5)
reviewed_file: _mill/discussion.md
date: 2026-09-12
```

## Findings

### [NIT:consistency] Template key + strict Load breaks existing loom.yaml
**Demoted-from:** BLOCKING
**Section:** "One `loom.yaml` key is both the model spec and the kill switch" / Constraints (Config Strictness)
**Issue:** `configengine.load` runs `yamlengine.MissingKeys(template, fileBytes)` and hard-errors "missing keys: …; run \"lyx config reconcile\"" (`internal/configengine/config.go:110-123`), and `loomengine.LoadConfig` is on the strict `Load` path (`internal/loomengine/config.go:176`) — so shipping `friction`/`friction_timeout_min` in `template.yaml` makes every already-seeded worktree's `loom.yaml` fail to load, which directly contradicts both "the default is on" and the Constraints section's "treat an absent key as Tier 2 off so an un-migrated worktree degrades quietly". The testing item "an absent `friction` key yields the zero value" is unachievable as written.
**Fix:** Decide explicitly: either the keys go in the template and `lyx config reconcile` is the stated migration (accepting a hard error until run), or they stay out of the template and off-by-default — and restate "absent" as present-but-empty, since a genuinely missing key can never reach the parse.

### [NIT:scope] Webster injection sites misidentified; recovery prompt unlisted
**Demoted-from:** BLOCKING
**Section:** Scope "In" / Technical context ("The `internal/pattern` precedent")
**Issue:** `render.go:183` is `RenderRecoveryPrompt`, not the implementer fork; `RenderForkPrompt` (`render.go:145`) has no `pattern` wiring, no `anchorRoot` parameter, and uses plain `stencil.Fill` (`:162`) — so the claim "already wired through four of the five composers … the injection points are proven" is false for the fork, and the fork needs the same `FillOptional` conversion the discussion attributes to Discussion-Write alone. `RenderRecoveryPrompt`, a real implementer-class spawn, has no stated disposition anywhere.
**Fix:** Re-enumerate the composer inventory against `render.go` (fork, recovery, master), state whether the cold-start recovery strand gets the directive, and note that two composers — not one — need the `Fill` → `FillOptional` conversion.

### [BLOCKING:design] Nothing creates `.lyx/loom/friction/` before an agent writes
**Section:** "Notes live under the ephemeral `.lyx` tree" / "Zero notes means no agent is spawned at all"
**Issue:** `Reflect` treats a missing directory as "skipped" and the composers only inject a path string; no party is assigned the `os.MkdirAll` (compare `internal/burlerengine/engine.go:112-116`, which creates its own `.lyx/burler` dir before writing). If the directory does not exist, every note write depends on provider-specific parent-dir creation, and a failure produces exactly the silent zero-notes state the marker-warn decision exists to prevent.
**Fix:** Name the owner and the moment of creation (composer-side per injection, or a single create at the `run.go` seed-time clear), and state the failure behavior when creation fails.

### [NIT:consistency] `friction` envelope key is not on `lyx loom run`'s envelope
**Demoted-from:** BLOCKING
**Section:** "The reflection step can never change the run's outcome"
**Issue:** The decision says the key is "reported in the `lyx loom run` success envelope" and justifies it by operator legibility, but the named keys (`outcome`/`halted_producer`/`reason`/`history_length`) belong to `drive.go:138-143`, and `lyx loom run` spawns `loom drive` detached with stdout/stderr redirected to the driver log (`internal/loomcli/run.go:230-232`) before handing off to tmux — so an operator running `loom run` never sees that envelope.
**Fix:** Restate the decision as drive's envelope (written to the driver log), and say explicitly whether driver-log-only visibility is accepted or whether a second surface (logger `Info`, status file) is required for the operator.

### [NIT:design] Who emits the missing-marker Warn is ambiguous
**Section:** "A stencil that never got the marker warns, rather than degrading silently"
**Issue:** The decision says each of the five composers "logs once at `Warn`" but also that the check is "a single exported helper in `internal/friction`" — and the proposed Friction Leaf Invariant admits `internal/logger`, which only matters if the helper itself logs.
**Fix:** State whether the helper logs or returns a bool the caller logs on, since the leaf's import set follows from that answer.

## Verdict

REQUEST_CHANGES
Config-strictness contradiction, mis-enumerated webster composers, unowned directory creation, wrong envelope.
_Note: 3 finding(s) demoted from BLOCKING to NIT by the stage's blocking-class ceiling; current blocking_count is 1._
MILL_REVIEW_END
