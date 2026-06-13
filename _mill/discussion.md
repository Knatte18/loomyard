# Discussion: Build mhgo worktree module

```yaml
task: Build mhgo worktree module
slug: mhgo-worktree-module
status: discussing
parent: main
```

## Problem

mhgo needs a worktree module to manage the full lifecycle of git worktrees: creating
them under the container directory, listing what exists, and tearing them down safely.
Teardown is the hard part: on Windows, NTFS junctions inside a worktree (created by
mill setup) block `git worktree remove` with a lock/permission error. This has caused
repeated manual recovery work in millhouse. The worktree module encodes the correct
teardown sequence once so it is never relearned.

This is milestone 4 in the roadmap: the first module to consume `internal/git` and
`internal/config` as designed.

## Scope

**In:**
- `internal/worktree/` package — the full domain: `add`, `list`, `remove`
- `cmd/mhgo/main.go` — `case "worktree"` dispatch
- `internal/board/init.go` — scaffold `worktree.yaml` in `RunInit` alongside `board.yaml`
- `_mhgo/worktree.yaml` template written by `mhgo init`
- Per-file unit tests with real git repos in `t.TempDir()`
- `docs/modules/worktree.md` — update open questions to answered decisions

**Out:**
- `internal/state` integration — deferred until mux milestone
- Branch deletion on remove (branch lifecycle belongs to a future task module)
- Junction creation on `add` (mill owns that; mhgo only removes on teardown)
- `--from <branch>` flag on `add` (YAGNI)
- Git Worktree Manager VS Code extension integration (user configures manually)

## Decisions

### Container layout is a fixed invariant

- Decision: the container directory is always the parent of the hub (`..` relative
  to the hub root). It is not a config key in `worktree.yaml`.
- Rationale: the layout is locked by design — `<RepoName>Hub/` holds the hub, `_board/`,
  and all worktrees as siblings. Making it configurable adds complexity with no benefit
  for the intended use case.
- Rejected: `container: ..` config key — unnecessary when the invariant is enforced.

### `worktree.yaml` holds only `branch_prefix`

- Decision: `_mhgo/worktree.yaml` has a single field: `branch_prefix` (default `""`).
  Loaded via `internal/config.Load` with the key `"worktree"`. Absent file → defaults.
- Rationale: nothing else varies per-repo. Container is fixed (above). Worktree dir
  naming rule and teardown sequence are code invariants, not config.
- Rejected: `container` key (fixed invariant), `cleanup_links` list (scan instead).

### Worktree directory name = branch name with `/` → `-`

- Decision: branch = `<branch_prefix><slug>`. Directory = branch name with every `/`
  replaced by `-`. Example: prefix `hanf/`, slug `my-task` → branch `hanf/my-task`,
  dir `hanf-my-task`.
- Rationale: directory must be a valid filesystem name; branch namespace separator `/`
  maps naturally to `-`. Aligns dir name with branch name, making origin unambiguous.
- Rejected: always `<slug>` (would diverge from branch name when prefix is non-empty).

### `add` fails if the source worktree is dirty

- Decision: `mhgo worktree add <slug>` checks whether the worktree it is run from has
  uncommitted staged or unstaged changes to tracked files. Untracked files are ignored
  — they stay in the source worktree and do not affect the new branch. Use
  `git status --porcelain --untracked-files=no`. If output is non-empty, exit with
  an error before touching git. No `--force` bypass.
- Rationale: spawning from a dirty working tree is a workflow hazard — tracked changes
  are easy to lose track of across worktrees. Untracked files are irrelevant since
  `add` only creates a new checkout; untracked files remain untouched.
  The source can be any worktree (not necessarily hub/main).
- Rejected: always allow (too permissive); count untracked as dirty (too strict).

### `add` pushes the new branch to remote

- Decision: after `git worktree add`, run `git push -u origin <branch>`. If no remote
  is configured, fail gracefully with a clear JSON error (no silent skip).
- Rationale: remote backup immediately after branch creation is the standard workflow.
  Explicit failure when no remote is configured is better than silent omission.
- Rejected: no push (user would have to remember); silent skip on no-remote (hides config problems).

### `add` fails on duplicate slug or existing branch/directory

- Decision: before creating anything, check whether the branch already exists in git
  and whether the target directory already exists on disk. Either condition is a hard
  error with a specific message.
- Rationale: silent overwrite or orphaned state would be worse. Explicit errors let
  the user correct the situation.
- Rejected: auto-increment suffix (confusing); overwrite existing (destructive).

### `remove` does not delete the branch

- Decision: `mhgo worktree remove <slug>` removes only the worktree directory. It
  never deletes the branch.
- Rationale: a branch is tied to its task (slug), not to the presence of a worktree.
  The same branch can be checked out on another machine. Branch deletion is a task
  lifecycle event owned by a future task module, not the worktree module.
- Rejected: `--delete-branch` flag — out of scope; adds risk for minimal gain now.

### `remove` checks for uncommitted changes; `--force` bypasses

- Decision: default behaviour refuses to remove a worktree that has uncommitted changes
  to tracked files OR untracked files (since `os.RemoveAll` will delete them both).
  Use plain `git status --porcelain` (includes untracked). `--force` overrides this
  check and proceeds regardless.
- Rationale: safe default; both tracked changes and untracked files would be permanently
  lost by the remove. Hard failure unless explicitly forced. Mirrors `git worktree remove`.
- Rejected: always refuse (too rigid); ignore untracked (data loss risk).

### Junction/symlink cleanup: scan worktree root, remove all, cross-platform

- Decision: before calling `git worktree remove`, scan the immediate children of the
  worktree directory and remove every entry that is a symlink (Linux/macOS) or an
  NTFS junction (Windows). Same `os.Remove()` call works on both. No hardcoded list
  of paths; no build tags.
- Rationale: scanning avoids hardcoding mill-specific names (`.wiki`, `.active`, etc.).
  `os.Remove()` on a junction removes the mount point without touching its target,
  identical behaviour to removing a symlink on Linux.
- Rejected: hardcoded path list (brittle); build-tagged platform code (unnecessary
  since `os.Remove()` is cross-platform for this operation); scan-on-failure only
  (the scan is cheap enough to run unconditionally).

### On `git worktree remove` failure: force-remove + prune

- Decision: if `git worktree remove` fails after link cleanup (e.g. a terminal is
  still holding the directory on Windows), fall back to: force-delete the directory
  with `os.RemoveAll`, then `git worktree prune` to clear the stale git registration.
  If the fallback succeeds, return `{"ok":true,...}` with the same JSON shape as the
  normal path (including `links_removed`). If `os.RemoveAll` itself fails (directory
  still locked), return `{"ok":false,"error":"..."}` with a message describing the
  fallback failure — the worktree directory and git registration are left intact.
- Rationale: mirrors the manual recovery sequence that has been needed repeatedly.
  Makes teardown fully automated even under Windows lock conditions.
- Rejected: error on first failure without fallback (requires manual recovery).

### `list` is a thin wrapper over `git worktree list`

- Decision: `mhgo worktree list` runs `git worktree list --porcelain`, parses the
  output, and returns it as JSON. No state registry (deferred). Reports the current
  git state only.
- Porcelain field mapping: each blank-line-delimited block → one JSON object.
  `worktree <path>` → `"path"`, `HEAD <sha>` → `"head"`,
  `branch refs/heads/<name>` → `"branch": "<name>"` (strip `refs/heads/` prefix;
  return short name only), `detached` line → `"branch": "(detached)"`.
  First block in the output = the main worktree → `"main": true`; all others → `"main": false`.
  `bare` line: out of scope — mhgo only operates on non-bare repos; treat as error if encountered.
- Rationale: `internal/state` is deferred to the mux milestone. A thin wrapper gives
  useful output immediately with minimal complexity.
- Rejected: block list on absence of state (too restrictive); silently hide drift (inconsistent).
- Follow-up: `docs/modules/worktree.md` describes a state-reconciled `list` and
  `docs/roadmap.md` milestone 4 mentions the state lib; these need a wording update
  in a follow-up task once state lands.

### No state registry in this milestone

- Decision: `add` and `remove` do not read or write `internal/state`. The worktree
  registry (`slug → { path, branch }`) is deferred until the mux module's design is
  settled, since mux and worktree share the same state document.
- Rationale: task brief explicitly defers this. Worktree module is fully functional
  without it for now.
- Rejected: implement state now (premature; mux design not settled).

### `mhgo init` scaffolds `worktree.yaml` alongside `board.yaml`

- Decision: `RunInit` in `internal/board/init.go` is extended to also write a
  commented `_mhgo/worktree.yaml` template (if absent), and report `worktree_yaml`
  status in its JSON output.
- Rationale: init is the natural entry point for scaffolding all per-module configs.
  Users should not need to create `worktree.yaml` by hand.
- Rejected: separate `mhgo worktree init` subcommand (unnecessary split).

## Technical context

### Existing shared infrastructure to reuse

- `internal/config.Load(baseDir, "worktree", defaults)` — loads `_mhgo/worktree.yaml`,
  handles env-var expansion and missing-file defaults. Already used by board.
- `internal/git.RunGit(args []string, cwd string) (stdout, stderr string, exitCode int, err error)` —
  runs any git command in `cwd`. Returns stdout, stderr, and exit code separately.
  `err` is non-nil only for process-launch failures, not for non-zero git exits —
  callers must inspect `exitCode` to distinguish git errors. Used for all git
  operations in the worktree module.
- `internal/output.Ok` / `internal/output.Err` — JSON output helpers. Follow
  `{"ok":true,...}` / `{"ok":false,"error":"..."}` shape.
- `internal/board/init.go:RunInit` — extend to scaffold `worktree.yaml`.

### Board module as structural template

`internal/worktree/` mirrors the board package layout:
- `worktree.go` — `Worktree` struct + `New(cfg Config) *Worktree` facade
- `config.go` — `Config`, `LoadConfig(baseDir, module string) (Config, error)` called as
  `LoadConfig(cwd, "worktree")`, `DefaultConfig()`
- `cli.go` — `RunCLI(out io.Writer, args []string) int` (same signature as board).
  Each subcommand uses `flag.NewFlagSet` following the board pattern. For `remove`:
  define `--force` as a flag, then `fs.Args()[0]` is the slug. Callers must pass
  flags before the slug: `remove --force <slug>`. This matches Go's `flag` package
  behaviour (stops parsing flags at first non-flag argument).
- `add.go`, `list.go`, `remove.go` — one file per subcommand
- `links.go` — symlink/junction scanner + remover
- `*_test.go` — per-file unit tests

### `cmd/mhgo/main.go` dispatch

Add `case "worktree": return worktree.RunCLI(out, moduleArgs)` alongside the existing
`case "board"`. Import `github.com/Knatte18/mhgo/internal/worktree`. Also update the
package-level doc comment's Modules list to include `worktree` alongside `board` and
`muxpoc`.

### JSON output shapes

```
add:    {"ok":true,"slug":"...","branch":"...","path":"...","pushed":true}
list:   {"ok":true,"worktrees":[{"path":"...","branch":"main","head":"<sha>","main":true}, ...]}
        branch is the short name (refs/heads/ stripped). "(detached)" when HEAD is detached.
        First entry is main:true. CLI flag --force accepted before slug: remove --force <slug>.
remove: {"ok":true,"slug":"...","path":"...","links_removed":2}
        links_removed is the count of symlinks/junctions removed before git worktree remove.
        Same shape whether normal or fallback (os.RemoveAll) path succeeded.
```

Error shape: `{"ok":false,"error":"..."}` with exit code 1.

### Dirty-check implementation

`add`: `RunGit([]string{"status", "--porcelain", "--untracked-files=no"}, sourceWorktreePath)` —
non-empty stdout means tracked changes exist; fail. Untracked files ignored.
If `RunGit` itself returns a non-zero `exitCode` (e.g. cwd is not a git worktree),
return `{"ok":false,"error":"cwd is not a valid git worktree"}` before creating anything.

`remove`: `RunGit([]string{"status", "--porcelain"}, worktreePath)` — non-empty stdout
(tracked changes or untracked files) means dirty; fail unless `--force`.

### Detecting symlinks vs junctions on Windows

`os.Lstat(path).Mode()&os.ModeSymlink != 0` detects both symlinks (Linux) and NTFS
junctions (Windows, where Go reports them as symlinks via `ModeSymlink`). Iterate
`os.ReadDir(worktreePath)` at the root level only (non-recursive), filter by
`ModeSymlink`, call `os.Remove` on each.

### Worktree existence checks in `add`

- Branch exists: `RunGit([]string{"rev-parse", "--verify", branch}, sourceWorktreePath)` —
  `exitCode == 0` means the branch exists. All worktrees in the same repo share refs,
  so running from `sourceWorktreePath` (cwd) is correct; no need to locate the hub.
- Directory exists: `os.Stat(<container>/<dir>)` — error with `os.IsNotExist`.

### Path resolution in `add`

`add` is cwd-authoritative: `os.Getwd()` gives the source worktree path. Container =
`filepath.Dir(sourceWorktreePath)` (one level up). Target path = `filepath.Join(container, dirName)`.

## Testing

### Setup helpers

Every test file that creates a git repo uses a shared helper:
```go
func newTestRepo(t *testing.T) string {
    dir := t.TempDir()
    mustRun(t, dir, "git", "init")
    mustRun(t, dir, "git", "config", "user.email", "test@test.com")
    mustRun(t, dir, "git", "config", "user.name", "Test")
    // write + commit a file so HEAD exists
    os.WriteFile(filepath.Join(dir, "README"), []byte("init"), 0644)
    mustRun(t, dir, "git", "add", ".")
    mustRun(t, dir, "git", "commit", "-m", "init")
    return dir
}
```

For push tests, add a bare remote:
```go
func addRemote(t *testing.T, repoDir string) string {
    bare := t.TempDir()
    mustRun(t, bare, "git", "init", "--bare")
    mustRun(t, repoDir, "git", "remote", "add", "origin", bare)
    mustRun(t, repoDir, "git", "push", "-u", "origin", "master") // or "main"
    return bare
}
```

### Key scenarios

**config_test.go**
- Default config when `_mhgo/worktree.yaml` absent
- `branch_prefix` read from YAML
- Missing `_mhgo/` dir → error containing `run "mhgo init"`. Note: `internal/config.Load`
  emits `not initialized: _mhgo/ directory not found`; worktree's `LoadConfig` must
  re-wrap it as `not initialized here; run "mhgo init"`, mirroring `internal/board/config.go`.

**add_test.go**
- Happy path: clean repo, no remote conflict → worktree created at correct path, branch correct
- Dirty source worktree → error, nothing created
- Branch already exists → error
- Target directory already exists → error
- No remote → error with clear message (graceful fail)
- With remote → worktree created + push succeeds (using bare remote)
- `branch_prefix` non-empty → branch and dir names correct

**list_test.go**
- Empty repo (only main worktree) → one entry, `main: true`
- After `git worktree add` → two entries
- JSON output shape matches spec

**remove_test.go**
- Happy path: clean worktree, no junctions → removed, pruned
- Dirty worktree without `--force` → error, worktree still exists
- Dirty worktree with `--force` → removed
- Worktree with symlinks/junctions → links removed first, then worktree removed
- Non-existent slug → error

**links_test.go**
- Directory with no links → `links_removed: 0`
- Directory with symlinks → all removed, nested dirs and regular files untouched
- `os.Remove` failure on a link → error surfaced

## Q&A log

- **Q:** Does `add` need a `--from <branch>` flag? **A:** No — always branches from HEAD of the worktree where `mhgo` is run.
- **Q:** Should `add` push to remote? **A:** Yes, `-u origin`. Fail gracefully (not silent) if no remote.
- **Q:** What if the source worktree is dirty at `add`? **A:** Hard error, no bypass. Source can be any worktree, not just hub.
- **Q:** Should `remove` delete the branch? **A:** No. Branch is tied to the task (slug) and lives independently of worktrees. Branch deletion belongs to a future task module.
- **Q:** Should junction removal be platform-specific? **A:** No — `os.Remove()` and `os.Lstat().Mode()&ModeSymlink` work cross-platform; no build tags needed.
- **Q:** Should `worktree.yaml` have a `container` field? **A:** No — container is always `..` from hub. Fixed invariant, not configurable.
- **Q:** Does this milestone use `internal/state`? **A:** No — deferred until mux design is settled.
- **Q:** Should worktree dir name be slug or branch name? **A:** Branch name with `/` → `-`. Dir and branch stay in sync.
- **Q:** What does `list` return without state? **A:** `git worktree list --porcelain` parsed as JSON.
