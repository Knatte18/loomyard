MILL_REVIEW_BEGIN
# Review: self-report Tier 2: per-agent friction notes for unsupervised runs

```yaml
duration_s: 139.0
verdict: REQUEST_CHANGES
reviewer_model: opus
reviewer_self_id: claude-opus-4-class (self-assessed; exact build unknown)
reviewed_file: _mill/discussion.md
date: 2026-09-12
```

## Findings

### [NIT:consistency] "Five spawn sites" contradicts the seven-composer inventory
**Demoted-from:** BLOCKING
**Section:** Decisions §"Five spawn sites get the directive" vs Scope + Technical context
**Issue:** The decision enumerates five sites (Discussion-Write, Plan-Write, Burler, webster fork, webster Master) and the config decision repeats "no directive is injected at any of the five sites", while Scope, the composer table, and the Q&A at the end all inject at seven — Recovery (`render.go:177`) and Integration (`render.go:210`) included; verified both are real composers.
**Fix:** Restate the decision as seven injection points, list Recovery and Integration with their rationale, and purge the residual "five" wording from the config and Q&A entries.

### [BLOCKING:design] Enablement + note-path threading to the composers is unspecified and self-contradicted
**Section:** Decisions §"The directive is injected the way `internal/pattern` already does it" + Technical context §"Config plumbing"
**Issue:** `friction.Directive(frictionDir, notePath, stencilsDir, role)` carries no enabled signal (unlike `pattern.Directive`, which probes `PATTERN.md` on disk — verified `internal/pattern/pattern.go:78-84`), yet Technical context states the friction config values "land on the `frictionengine.Deps` the drive-time call site builds, not on `Env`" — leaving no stated route for the on/off value, `frictionDir`, and per-spawn `notePath` to reach the loom SpecSource closures, `burlerengine`'s deps, and `websterengine.RunDeps`.
**Fix:** Name how "off" is expressed at the composer boundary (e.g. empty `frictionDir` ⇒ `("", nil)`, mirroring pattern's empty-`anchorPath` case) and which deps structs carry the friction dir/enabled value into each of the three engines.

### [BLOCKING:design] Nothing creates the friction directory on a direct `lyx loom drive`
**Section:** Decisions §"`internal/loomcli/run.go` owns creating the directory" + §"Consumed notes are archived"
**Issue:** Creation is exclusively `run.go`'s, but `loom drive` is a real non-hidden foreground verb (`internal/loomcli/drive.go:23-24`), and `Reflect` renames the directory away on a clean `RunBlocked` reflection — a subsequent direct `lyx loom drive` then runs with no friction directory at all, so every note write silently fails, which is the exact zero-notes-no-signal state this decision exists to prevent.
**Fix:** State the disposition for the drive-only path — either drive also ensures the directory (create, never clear), or `Reflect` recreates an empty directory after archiving — and say which.

### [NIT:consistency] `run.go:101` does not today distinguish first seed from re-entry
**Section:** Decisions §"Consumed notes are archived…" ("`run.go:101` … already distinguishes a genuine first seed from a re-entry")
**Issue:** The shipped line is `if err := loomshed.Seed(...); err != nil && !errors.Is(err, loomshed.ErrSeedExists)` — both outcomes fall through the same branch and the error is not retained, so the clear/create split requires restructuring that statement, not just adding a call beside it.
**Fix:** Reword to "the call site can cheaply distinguish them once the error is bound to a variable", so the plan writer expects the restructure.

### [NIT:scope] Note-id source named for only four of seven sites
**Section:** Decisions §"Filename uniqueness comes from a caller-supplied id"
**Issue:** Ids are named for Discussion-Write, Plan-Write, Burler round, and webster fork; Recovery, Integration, and Master have no stated id source.
**Fix:** Name the identity string for those three (e.g. batch id for Recovery, a fixed literal for Integration and Master).

## Verdict

REQUEST_CHANGES
Composer count contradicts itself; enablement threading and the drive-only create path are unresolved.
_Note: 1 finding(s) demoted from BLOCKING to NIT by the stage's blocking-class ceiling; current blocking_count is 2._
MILL_REVIEW_END
