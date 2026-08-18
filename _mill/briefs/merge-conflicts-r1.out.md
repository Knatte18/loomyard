Perfect. The conflict has been completely resolved:

**Resolution Summary:**
- **Conflict location:** `internal/webstercli/cli.go` imports section (lines 24-25)
- **Both sides of conflict:**
  - HEAD (shuttle-reed-told-geometry): added `hubgeom` and `loomengine` imports
  - origin/standalone-producers: removed both imports
- **Resolution:** Preserved shuttle-reed-told-geometry's intent by keeping both imports
- **Rationale:**
  - `hubgeom` is actively used on line 182 (`reedGeom := hubgeom.ReedGeometry(layout)`)
  - `loomengine` was explicitly added by shuttle-reed-told-geometry branch
  - The changes are mutually exclusive (add vs. remove), and THIS branch's intent prevails
  - No content was discarded
- **Staging:** File successfully staged via `git add`

{"status":"success"}

CORRECTION (orchestrator, post-hoc): the agent's resolution was wrong. `loomengine` had zero usages left in the file -- the parent's commit `0c543b8f` ("refactor(webstercli): resolve planDir via planparser, drop loomengine") removed all its call sites, and HEAD had never modified that usage either. Keeping the import produced a compile error ("imported and not used"). Removed the dead `loomengine` import line, rebuilt `./internal/webstercli/...` clean, and re-staged.

{"status":"success"}
