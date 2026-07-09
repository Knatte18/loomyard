# Discussion: Rename `_other.go` platform files to `_linux.go`

```yaml
task: Rename _other.go platform files to _linux.go
slug: rename-other-to-linux
status: discussing
parent: main
```

## Problem

lyx targets **Windows + Linux only** — not macOS/BSD. Four platform-seam files use the
`_other.go` filename with an explicit `//go:build !windows` tag. That tag compiles the file on
**any** non-Windows GOOS (macOS, BSD, …), so a `GOOS=darwin` build silently produces an
untested binary and overstates what lyx supports. There is no "other" platform — non-Windows
means Linux, and only Linux.

Renaming each file to `_linux.go` moves the constraint into the filename suffix (`GOOS=linux`).
An unsupported GOOS then finds **no** implementation for these packages and fails to build
honestly instead of compiling something untested. The change is behaviour-preserving on Windows
and Linux; it is a build-constraint honesty fix, the same mechanical shape as the earlier
`codeguide → raddle` rename.

**Why now:** the "other" naming is actively misleading given the Win+Linux-only support scope,
and it lets a Mac build succeed when it must not.

## Scope

**In:**

- `git mv` five files (rename, preserve history):
  - `internal/fslink/fslink_other.go` → `internal/fslink/fslink_linux.go`
  - `internal/proc/proc_other.go` → `internal/proc/proc_linux.go`
  - `internal/proc/proc_other_test.go` → `internal/proc/proc_linux_test.go`
  - `internal/vscode/launch_other.go` → `internal/vscode/launch_linux.go`
  - `internal/muxpoccli/spawnattach_other.go` → `internal/muxpoccli/spawnattach_linux.go`
- Drop the leading `//go:build !windows` line **and its trailing blank line** from all five
  files. No replacement tag — the `_linux.go` suffix alone supplies the `GOOS=linux` constraint.
- Update the two self-referential filename comments (`// proc_other.go —`, `// spawnattach_other.go —`)
  to the new names.
- Reword every "non-Windows" / "POSIX" comment in the renamed files to "Linux" (see the exact
  edits under **Decisions → prose-reword**).
- Fix the stale `proc_other.go` reference in `docs/roadmap.md:113` → `proc_linux.go`.

**Out:**

- The `_windows.go` siblings — **not touched**. They keep their existing explicit
  `//go:build windows` tags; making the whole family suffix-only was explicitly rejected as
  scope creep.
- No behaviour changes. No signature, logic, import, or test-assertion changes.
- No `.go` source outside the five listed files (verified: the only `_other` references anywhere
  are the two filename comments, the `docs/roadmap.md` line, and the `_mill/status.md` task
  title — the last is left as-is since it names the task, not a code file).
- `fslink_linux.go` needs **no** prose reword — it has no "non-Windows"/"POSIX" text, only the
  build tag to drop.

## Decisions

### drop-tag-no-replacement

- Decision: Drop `//go:build !windows` entirely from the renamed files; do **not** add a
  `//go:build linux` tag.
- Rationale: The `_linux.go` suffix already constrains the file to `GOOS=linux`; a redundant tag
  adds nothing and the honesty goal (untested platforms don't build) is met by the suffix alone.
- Rejected: (a) Replacing with `//go:build linux` to mirror the `_windows.go` siblings' explicit
  tags — redundant, no benefit. (b) Also stripping the redundant `//go:build windows` from the
  four windows siblings to make the family suffix-only — real scope creep, out of this task.
- Note: the original task body's premise ("matching how the `_windows.go` siblings rely on their
  suffix, no explicit tag") is factually wrong — the windows siblings **do** carry explicit
  `//go:build windows` tags. The decision to drop the linux tag stands regardless; it is the
  minimal correct change, not a consistency match.

### prose-reword

- Decision: Reword every "non-Windows" and "POSIX" phrase in the renamed files to "Linux",
  because Win+Linux are the only supported platforms — "non-Windows" is exactly Linux, and the
  old wording implied a broader set that does not exist.
- Rationale: Same honesty theme as the rename; the comments should describe the one platform this
  file actually builds for.
- Rejected: Leaving the prose as-is (it would keep implying a generic non-Windows/POSIX family).
- Exact edits (behaviour-preserving comment text only):
  - `proc_linux.go`:
    - `// proc_other.go — non-Windows process control primitives.`
      → `// proc_linux.go — Linux process control primitives.`
    - `// On non-Windows platforms, HideWindow is a no-op (there are no console windows).`
      → `// On Linux, HideWindow is a no-op (there are no console windows).`
    - `// HideWindow is a no-op on non-Windows platforms (no console windows to suppress).`
      → `// HideWindow is a no-op on Linux (no console windows to suppress).`
    - `// On non-Windows, Setsid is the equivalent of Windows CREATE_NEW_PROCESS_GROUP | CREATE_NO_WINDOW:`
      → `// On Linux, Setsid is the equivalent of Windows CREATE_NEW_PROCESS_GROUP | CREATE_NO_WINDOW:`
  - `launch_linux.go`:
    - `// Launch returns an error on non-Windows platforms (POSIX).`
      → `// Launch returns an error on Linux.`
    - `// VS Code launch is a Windows-only feature; POSIX systems are not supported.`
      → `// VS Code launch is a Windows-only feature; Linux is not supported.`
  - `spawnattach_linux.go`:
    - `// spawnattach_other.go — psmux attach for non-Windows.`
      → `// spawnattach_linux.go — psmux attach for Linux.`
    - `// Blocks until the user detaches (normal for non-Windows interactive use).`
      → `// Blocks until the user detaches (normal for Linux interactive use).`

### roadmap-reference-fix

- Decision: Update `docs/roadmap.md:113` `proc_other.go` → `proc_linux.go`.
- Rationale: It is a stale code-file reference (a factual correction), not a roadmap-milestone
  edit, so it falls within the docs-lifecycle rules rather than violating "roadmap is for planned
  milestones only".
- Rejected: Leaving it stale — the task explicitly calls for fixing `_other` references in docs.

## Technical context

- Each of the four packages (`fslink`, `proc`, `vscode`, `muxpoccli`) has a Windows/non-Windows
  file pair. After the rename the pair is `_windows.go` + `_linux.go`; on `GOOS=darwin` these
  packages have no matching file, which is the intended honest build failure.
- The `//go:build !windows` line is line 1 of each file, followed by a blank line 2, then either
  a package-doc comment block or the `package` clause. Dropping **both** line 1 and line 2 keeps
  the remaining file well-formed (comment block or `package` clause becomes the top).
  - Specifically for `proc_linux.go`: after dropping lines 1–2, the file begins with the
    `// proc_linux.go — …` doc-comment block (lines 3–7 today), which is correct.
- Use `git mv` for each rename (memory: renames use `git mv` + surgical edits, never full-file
  rewrites). Apply the tag-drop and prose edits with targeted `Edit` calls after the move.
- No `CONSTRAINTS.md` invariant governs build tags or platform-file naming (verified — no
  matches for build-tag/GOOS/suffix/platform in `CONSTRAINTS.md`).
- The `fslink` module is governed by CLAUDE.md's fslink rule (directory links; junctions on
  Windows, symlinks on Linux). This rename does not alter that behaviour — `fslink_linux.go`
  keeps the same `os.Symlink`-based `CreateDirLink` implementation.

## Constraints

- Behaviour-preserving on Windows and Linux: no logic, signature, import, or test-assertion
  changes — comment/text and filename/build-tag changes only.
- `_windows.go` siblings must remain untouched.
- Renames must preserve git history (`git mv`).
- Docs-lifecycle: the `docs/roadmap.md` edit is a factual reference fix only; do not add
  roadmap notes for this mechanical change (per CLAUDE.md the roadmap is for planned milestones).

## Testing

This is a mechanical, behaviour-preserving rename — no new unit tests. Verification is by
cross-compilation and the existing suite:

- `GOOS=windows go build ./...` — **green** (unchanged Windows path).
- `GOOS=linux go build ./...` — **green** (renamed files now supply the linux impl via suffix).
- `GOOS=linux go vet ./...` — **green**.
- `GOOS=darwin go build ./...` — **must fail** with "build constraints exclude all Go files"
  (or equivalent) for the four packages. This failure is the deliverable, not a regression;
  confirm it happens and that it is caused by the missing platform impl.
- Existing tests pass on the Windows dev host (`go test ./...`), including the renamed
  `proc_linux_test.go` (it only builds/runs on Linux, so on Windows it is simply excluded — its
  Windows counterpart, if any, is unaffected).
- After the edits, grep confirms **zero** remaining `_other` references in `.go` files and in
  `docs/`, and zero remaining "non-Windows"/"POSIX" strings in the five renamed files.

## Q&A log

- **Q:** Build-tag treatment on the renamed `_linux.go` files? **A:** Drop the tag entirely; the
  `_linux.go` suffix supplies the constraint. (Original task premise that the windows siblings
  are tag-free is wrong — they carry `//go:build windows` — but dropping the linux tag is still
  the minimal correct move.)
- **Q:** Reword "non-Windows"/"POSIX" comment prose? **A:** Yes — reword to "Linux" everywhere.
  Win+Linux are the only supported platforms; "other"/"non-Windows" is exactly Linux and nothing
  else, so the old wording is misleading.
- **Q:** Fix the `proc_other.go` reference in `docs/roadmap.md`? **A:** Yes — it's a stale code
  reference, a factual fix within the docs-lifecycle rules.
