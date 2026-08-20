# Plan: Extract scout into its own standalone repo

```yaml
task: "Extract scout into its own standalone repo"
slug: "scout-extract-standalone-repo"
approved: false
started: "20260820-095202"
parent: "main"
root: ""
verify: null
```

## Batch Index

_The fenced yaml block below is the authoritative DAG mill-go reads to schedule batches.
Every batch lives at `NN-<batch-slug>.md` in this directory and is mirrored as one entry here._

```yaml
batches:
  - number: 1
    name: quarry-scaffold
    file: 01-quarry-scaffold.md
    depends-on: []
    verify: go -C /home/knatte/Code/quarry/wts/quarry test ./internal/...
  - number: 2
    name: quarry-cli-infra
    file: 02-quarry-cli-infra.md
    depends-on: [1]
    verify: go -C /home/knatte/Code/quarry/wts/quarry test ./internal/...
  - number: 3
    name: port-engine
    file: 03-port-engine.md
    depends-on: [2]
    verify: go -C /home/knatte/Code/quarry/wts/quarry test ./...
  - number: 4
    name: port-cli
    file: 04-port-cli.md
    depends-on: [3]
    verify: go -C /home/knatte/Code/quarry/wts/quarry test ./...
  - number: 5
    name: quarry-live-and-equivalence
    file: 05-quarry-live-and-equivalence.md
    depends-on: [4]
    verify: go -C /home/knatte/Code/quarry/wts/quarry test -tags lsp ./...
  - number: 6
    name: lyx-removal
    file: 06-lyx-removal.md
    depends-on: [5]
    verify: go test ./...
```

## Shared Decisions

### Decision: two-repo worktree authorization and how each card commits

- **Decision:** batches 1-5 write, commit, and push in `/home/knatte/Code/quarry/wts/quarry` (referred to below as **the quarry worktree**);
  batch 6 writes only in this task worktree.
  Every quarry-side command is spelled `git -C /home/knatte/Code/quarry/wts/quarry …` or `go -C /home/knatte/Code/quarry/wts/quarry …`.
  A `cd` into the quarry worktree is forbidden — it corrupts the shell cwd for the rest of the session.
  Each card's `Commit:` message is used for a `git -C /home/knatte/Code/quarry/wts/quarry commit` when that card's paths are quarry-side, and for an ordinary task-worktree commit when they are not.
- **Rationale:** the user created and cloned the quarry repo for this task and authorized cross-worktree writes;
  the authorization is recorded in `_mill/discussion.md`'s `two-repo-worktree-authorization` decision.
  Every batch that writes in quarry also carries a final card that appends to `docs/research/quarry-port-log.md` in **this** worktree, so every batch produces at least one task-worktree commit and mill-go's per-batch cleanliness gate has something to observe.
- **Applies to:** all batches.

### Decision: the plan validator's out-of-worktree-target check is skipped for this plan

- **Decision:** quarry-side files are named by absolute path under `/home/knatte/Code/quarry/wts/quarry/` in `Context:`/`Edits:`/`Creates:`, and `_plan_validate`'s `out-of-worktree-target` check is skipped for this plan.
- **Rationale:** that check exists to stop an implementer being pointed at paths nobody authorized it to touch.
  This task's authorization is explicit and recorded.
  The alternative — hiding the quarry file list in `Requirements:` prose so the check does not see it — would make the plan less honest and less reviewable, not safer.
- **Applies to:** all batches.

### Decision: behavioural equivalence is the acceptance criterion, so nothing is "fixed in passing"

- **Decision:** the port changes import paths, package clauses, path-resolution signatures, and lyx vocabulary — nothing else.
  The 59 `"scoutengine: "` string literals stay verbatim through batches 3 and 4.
  The two known defects (quarry#1, quarry#2) are moved unfixed.
  No CLI verb, flag, or JSON envelope field changes shape.
- **Rationale:** batch 5 compares `quarry` and `lyx scout` JSON envelopes byte for byte for the same queries.
  Any behavioural change made during the port — including a "harmless" error-message rename — forces that comparison to be relaxed, which is exactly the loophole a real regression would hide in.
- **Applies to:** batches 3, 4, 5.

### Decision: the file move is done by a Go program, never by hand and never by `sed`

- **Decision:** `tools/port/main.go` in the quarry worktree copies the named source files and rewrites exactly two categories of token: the four import paths (`github.com/Knatte18/loomyard/internal/scoutengine` -> `github.com/Knatte18/quarry/quarry`, `…/internal/scoutcli` -> `github.com/Knatte18/quarry/internal/cli`, `…/internal/{lock,proc,output}` -> `github.com/Knatte18/quarry/internal/{lock,proc,output}`) and the two package clauses (`package scoutengine` -> `package quarry`, `package scoutcli` -> `package cli`).
  Hand editing is confined to the replaced packages and their call sites.
  `sed` is banned by the operator's global instructions and by the `mill:conversation` skill.
- **Rationale:** transcribing 8 973 lines through an LLM context is expensive and a correctness hazard;
  a closed-set token rewrite is deterministic and reviewable as a diff.
- **Applies to:** batches 3, 4, 5.

### Decision: dependency budget is three direct modules plus cobra's own indirects

- **Decision:** quarry's `go.mod` declares exactly three direct requires — `github.com/spf13/cobra`, `gopkg.in/yaml.v3`, and `github.com/gofrs/flock` — plus whatever indirects cobra pulls in on its own (`github.com/spf13/pflag` and `github.com/inconshreveable/mousetrap`), which arrive with it rather than being chosen.
  A batch that would add a fourth direct dependency stops and reports instead of adding it.
- **Rationale:** the whole point of splitting the nine shared Loomyard packages into copy-verbatim leaves and narrow local replacements is to keep quarry from becoming a Loomyard clone.
- **Applies to:** all batches.

### Decision: one opt-in test tag, `lsp`

- **Decision:** every quarry test needing a real language-server binary on `$PATH` carries `//go:build lsp`.
  The five `//go:build scout` files and the one `//go:build integration` file collapse onto it during the port.
  Verification is `go test ./...` (hermetic) and `go test -tags lsp ./...` (live).
- **Rationale:** a verify command spelled `-tags integration` would have run one file out of six while appearing green.
  `lsp` names the actual precondition;
  `scout` is dead vocabulary once the tool is called quarry.
- **Applies to:** batches 3, 4, 5.

### Decision: config, state, and toolchain cache are three separate path axes

- **Decision:**
  - **Config** — precedence `--config <path>` -> `$QUARRY_CONFIG` -> `os.UserConfigDir()/quarry/servers.yaml` -> built-in registry, resolved entirely in `internal/cli/`. `LoadRegistry` is told a resolved file path and joins nothing.
  - **State** — precedence `--state-dir <path>` -> `$QUARRY_STATE_DIR` -> `os.UserCacheDir()/quarry/<workspace-key>/`, resolved entirely in `internal/cli/`. `DaemonStateFile`/`DaemonLock` are told a leaf state directory and join only `<lang>/daemon.json` and `<lang>/daemon.lock`.
  - **Toolchain cache** — stays engine-derived in `toolchain.go`, with its `lyx` path segment renamed to `quarry`.
- **Rationale:** today's `anchorRoot` conflates config and state, and outside a lyx hub there is no single root that serves both.
  The toolchain cache is neither: no caller has a reason to override where quarry stashes a `gopls` it installed itself.
- **Applies to:** batches 2, 3, 4.

### Decision: machine-global roots go through package-level function-variable seams

- **Decision:** `internal/cli/` declares `var userConfigDir = os.UserConfigDir` and `var userCacheDir = os.UserCacheDir`, and every resolution goes through them.
  Tests redirect both to a `t.TempDir()`. No test isolates itself by changing directory.
- **Rationale:** both stdlib functions are process-global and ignore cwd, so `t.Chdir` cannot reach them, and an env-only approach (`$XDG_CONFIG_HOME`) is silently Linux-specific.
  `toolchain.go` already declares exactly this seam, so this copies an established in-repo pattern rather than inventing a second one.
- **Applies to:** batches 2, 3, 4.

### Decision: supported platforms are linux and windows

- **Decision:** quarry claims linux and windows.
  darwin is out and is filed as a quarry issue.
  Windows support means "works via the native strategy" — the supervised daemon hard-codes a Unix socket and falls back to `ensureNative` there.
- **Rationale:** `internal/proc` has only `proc_linux.go` and `proc_windows.go`, so a verbatim copy does not compile on darwin.
  Writing a darwin implementation is new platform code, unverifiable from this machine, and scope creep on a task whose discipline is behavioural equivalence.
- **Applies to:** batches 1, 5.

### Decision: quarry green before any Loomyard deletion

- **Decision:** batch 6 is the only batch that touches this worktree's source tree, and it depends on batch 5.
  No deletion lands until quarry builds, its hermetic and live tiers pass, and the envelope comparison has passed.
- **Rationale:** the deletion is irreversible in practice, and the port is the part most likely to surface a hidden dependency.
  Keeping Loomyard green and scout-bearing until quarry is proven means a failed port costs nothing.
- **Applies to:** batches 5, 6.

## All Files Touched

- `/home/knatte/Code/quarry/wts/quarry/.gitignore`
- `/home/knatte/Code/quarry/wts/quarry/LICENSE`
- `/home/knatte/Code/quarry/wts/quarry/README.md`
- `/home/knatte/Code/quarry/wts/quarry/cmd/quarry/main.go`
- `/home/knatte/Code/quarry/wts/quarry/docs/port-equivalence.md`
- `/home/knatte/Code/quarry/wts/quarry/docs/scout-agent-usage-findings.md`
- `/home/knatte/Code/quarry/wts/quarry/docs/scout-multilang.md`
- `/home/knatte/Code/quarry/wts/quarry/docs/scout-spike.md`
- `/home/knatte/Code/quarry/wts/quarry/docs/scout-vs-grep.md`
- `/home/knatte/Code/quarry/wts/quarry/docs/servers.yaml.example`
- `/home/knatte/Code/quarry/wts/quarry/go.mod`
- `/home/knatte/Code/quarry/wts/quarry/go.sum`
- `/home/knatte/Code/quarry/wts/quarry/internal/cli/cli.go`
- `/home/knatte/Code/quarry/wts/quarry/internal/cli/cli_test.go`
- `/home/knatte/Code/quarry/wts/quarry/internal/cli/cwdcontext.go`
- `/home/knatte/Code/quarry/wts/quarry/internal/cli/cwdcontext_test.go`
- `/home/knatte/Code/quarry/wts/quarry/internal/cli/exec.go`
- `/home/knatte/Code/quarry/wts/quarry/internal/cli/exec_test.go`
- `/home/knatte/Code/quarry/wts/quarry/internal/cli/paths.go`
- `/home/knatte/Code/quarry/wts/quarry/internal/cli/paths_test.go`
- `/home/knatte/Code/quarry/wts/quarry/internal/cli/resolve_test.go`
- `/home/knatte/Code/quarry/wts/quarry/internal/lock/lock.go`
- `/home/knatte/Code/quarry/wts/quarry/internal/lock/lock_test.go`
- `/home/knatte/Code/quarry/wts/quarry/internal/output/output.go`
- `/home/knatte/Code/quarry/wts/quarry/internal/output/output_test.go`
- `/home/knatte/Code/quarry/wts/quarry/internal/proc/isalive_test.go`
- `/home/knatte/Code/quarry/wts/quarry/internal/proc/killpid_test.go`
- `/home/knatte/Code/quarry/wts/quarry/internal/proc/proc_linux.go`
- `/home/knatte/Code/quarry/wts/quarry/internal/proc/proc_linux_test.go`
- `/home/knatte/Code/quarry/wts/quarry/internal/proc/proc_windows.go`
- `/home/knatte/Code/quarry/wts/quarry/internal/proc/proc_windows_test.go`
- `/home/knatte/Code/quarry/wts/quarry/quarry/daemonstate.go`
- `/home/knatte/Code/quarry/wts/quarry/quarry/daemonstate_test.go`
- `/home/knatte/Code/quarry/wts/quarry/quarry/definition.go`
- `/home/knatte/Code/quarry/wts/quarry/quarry/definition_test.go`
- `/home/knatte/Code/quarry/wts/quarry/quarry/detect.go`
- `/home/knatte/Code/quarry/wts/quarry/quarry/detect_test.go`
- `/home/knatte/Code/quarry/wts/quarry/quarry/doc.go`
- `/home/knatte/Code/quarry/wts/quarry/quarry/ensureserver.go`
- `/home/knatte/Code/quarry/wts/quarry/quarry/ensureserver_integration_test.go`
- `/home/knatte/Code/quarry/wts/quarry/quarry/ensureserver_test.go`
- `/home/knatte/Code/quarry/wts/quarry/quarry/errors.go`
- `/home/knatte/Code/quarry/wts/quarry/quarry/load.go`
- `/home/knatte/Code/quarry/wts/quarry/quarry/load_test.go`
- `/home/knatte/Code/quarry/wts/quarry/quarry/lspclient.go`
- `/home/knatte/Code/quarry/wts/quarry/quarry/lspclient_guard_test.go`
- `/home/knatte/Code/quarry/wts/quarry/quarry/lspclient_test.go`
- `/home/knatte/Code/quarry/wts/quarry/quarry/position.go`
- `/home/knatte/Code/quarry/wts/quarry/quarry/position_test.go`
- `/home/knatte/Code/quarry/wts/quarry/quarry/probe.go`
- `/home/knatte/Code/quarry/wts/quarry/quarry/quarrydaemon_test.go`
- `/home/knatte/Code/quarry/wts/quarry/quarry/refs.go`
- `/home/knatte/Code/quarry/wts/quarry/quarry/refs_integration_test.go`
- `/home/knatte/Code/quarry/wts/quarry/quarry/refs_test.go`
- `/home/knatte/Code/quarry/wts/quarry/quarry/registry.go`
- `/home/knatte/Code/quarry/wts/quarry/quarry/registry_test.go`
- `/home/knatte/Code/quarry/wts/quarry/quarry/seam_enforcement_test.go`
- `/home/knatte/Code/quarry/wts/quarry/quarry/supervised_integration_test.go`
- `/home/knatte/Code/quarry/wts/quarry/quarry/supervised_lsp_test.go`
- `/home/knatte/Code/quarry/wts/quarry/quarry/supervised_test.go`
- `/home/knatte/Code/quarry/wts/quarry/quarry/symbol.go`
- `/home/knatte/Code/quarry/wts/quarry/quarry/symbol_test.go`
- `/home/knatte/Code/quarry/wts/quarry/quarry/toolchain.go`
- `/home/knatte/Code/quarry/wts/quarry/quarry/toolchain_integration_test.go`
- `/home/knatte/Code/quarry/wts/quarry/quarry/toolchain_test.go`
- `/home/knatte/Code/quarry/wts/quarry/tools/port/main.go`
- `CONSTRAINTS.md`
- `README.md`
- `cmd/lyx/configstrictness_test.go`
- `cmd/lyx/constructoranchoring_test.go`
- `cmd/lyx/helptree_test.go`
- `cmd/lyx/hermeticenv_test.go`
- `cmd/lyx/main.go`
- `cmd/lyx/notransients_test.go`
- `cmd/lyx/sandbox_coverage_test.go`
- `cmd/lyx/seamsignature_test.go`
- `cmd/lyx/tierpurity_test.go`
- `contracts/specs/loom-plan-spec.md`
- `docs/benchmarks/running-tests.md`
- `docs/benchmarks/test-suite-timing.md`
- `docs/overview.md`
- `docs/research/quarry-port-log.md`
- `internal/fabriccli/clone.go`
- `internal/fabricengine/junction.go`
- `internal/gitrepo/doc.go`
- `internal/loomshed/loomshed.go`
- `internal/lyxcwd/enforcement_test.go`
- `internal/websterengine/doc.go`
- `manifest/designs/fabric-unified-view.md`
- `manifest/designs/loom.md`
- `manifest/designs/raddle.md`
- `manifest/designs/review-finding-classification.md`
- `manifest/designs/scout-plan-symbol-fields.md`
- `manifest/designs/semantic-index.md`
- `manifest/designs/webster-parallel-execution.md`
- `manifest/parallel-work.md`
- `manifest/roadmap.md`
