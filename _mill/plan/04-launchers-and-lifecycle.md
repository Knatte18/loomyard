# Batch: launchers-and-lifecycle

```yaml
task: dev/test lyx.exe separated from production deploy
batch: launchers-and-lifecycle
number: 4
cards: 3
verify: null
depends-on: [3]
```

## Batch Scope

Ships the operator-facing launchers and completes the documentation lifecycle. Adds the
`deploy-dev` sibling launchers (no hardcoded path — they just call `go run ./tools/deploy
-dev`), gitignores the derived `.dev-bin/` directory, records the new "Dev/Prod Binary
Separation" invariant in `CONSTRAINTS.md`, and closes out the design doc (roadmap Planned→Done,
delete `manifest/designs/dev-test-binary.md`). Depends on batch 3 so the invariant text can
reference the now-existing `resolveLyx` + guard test. Pure config/docs/shell — no Go surface,
so `verify: null`.

## Cards

### Card 14: Add deploy-dev launchers and gitignore .dev-bin

- **Context:**
  - `deploy.cmd`
- **Edits:**
  - `.gitignore`
- **Creates:**
  - `deploy-dev.cmd`
  - `deploy-dev`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Create `deploy-dev.cmd` mirroring `deploy.cmd`'s structure (comment header,
  `pushd "%~dp0"`, capture/restore exit code, `popd`) but invoking `go run ./tools/deploy -dev
  %*` — no `-dest`, no hardcoded path. Create `deploy-dev` as a POSIX shell script: `#!/usr/bin/env
  bash` shebang, `cd` to the script's own directory (repo root) e.g. via `cd "$(dirname
  "$0")"`, then `exec go run ./tools/deploy -dev "$@"`; mark it executable (mode 0755). Add a
  single line `.dev-bin/` to `.gitignore` (place it near the existing compiled-binary block
  that already ignores `/lyx` and `lyx.exe`, with a short comment explaining it is the derived
  dev-only deploy target).
- **Commit:** `feat(deploy): add deploy-dev launchers and gitignore .dev-bin`

### Card 15: Record the Dev/Prod Binary Separation invariant

- **Context:**
  - `_mill/discussion.md`
- **Edits:**
  - `CONSTRAINTS.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Add a new `## Dev/Prod Binary Separation` invariant section to
  `CONSTRAINTS.md`, matching the heading + bullet style of the existing invariants (e.g.
  `## Sandbox Suite Coverage`). State: the sandbox tooling resolves the dev binary from the
  derived `.dev-bin` (falling back to PATH) through `resolveLyx`, never a bare-PATH `lyx`
  lookup — covering both `lookPath("lyx")` and the separator-free `exec.Command("lyx", …)` /
  `exec.CommandContext("lyx", …)` form; the dev binary is never installed to prod's location;
  `.dev-bin/` is gitignored; `.dev-bin` is prepended only to the agent child-process PATH,
  never the operator's. Note enforcement is partly the guard test
  (`tools/sandbox/pathresolve_guard_test.go`) and partly review discipline. Draw the wording
  from the "New invariant to add" bullet in `_mill/discussion.md`'s Constraints section.
- **Commit:** `docs(constraints): add Dev/Prod Binary Separation invariant`

### Card 16: Close out the design doc in the roadmap

- **Context:**
  - `manifest/designs/dev-test-binary.md`
- **Edits:**
  - `manifest/roadmap.md`
- **Creates:** none
- **Deletes:**
  - `manifest/designs/dev-test-binary.md`
- **Moves:** none
- **Requirements:** Per the documentation lifecycle, delete `manifest/designs/dev-test-binary.md`.
  In `manifest/roadmap.md`, remove the `dev/test lyx.exe separated from production deploy` item
  from the `## Planned` section (currently the entry linking to
  `designs/dev-test-binary.md`) and add a corresponding entry under `## Done`, matching the
  existing Done-entry style (bold title + one-line description). The Done entry must NOT link to
  the deleted design doc — instead reference the new `Dev/Prod Binary Separation` invariant in
  `CONSTRAINTS.md`. Ensure no dangling link to `designs/dev-test-binary.md` remains anywhere in
  `manifest/roadmap.md`.
- **Commit:** `docs(roadmap): mark dev-test-binary done, delete design doc`

## Batch Tests

`verify: null` — this batch has no runnable code surface: two launcher scripts, a `.gitignore`
line, and Markdown edits/deletion. There is no automated test for the documentation lifecycle
(confirmed: no `manifest`/`roadmap` guard test exists), so correctness is by review. The
overview-level `go build ./tools/...` still runs at this batch boundary and confirms the Go
tree remains buildable after the lifecycle edits.
