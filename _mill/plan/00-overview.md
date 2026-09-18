# Plan: Replace reed's header pane with a status-line and Selvage

```yaml
task: "Replace reed's header pane with a status-line and Selvage"
slug: "reed-header-selvage"
approved: true
started: "20260918-171925"
parent: "main"
root: ""
verify: go build ./...
discussion_sha: 7b409e581b6ab5c42dad91411dbc6666b3ff6221
```

## Batch Index

_The fenced yaml block below is the authoritative DAG mill-go reads to schedule batches.
Every batch lives at `NN-<batch-slug>.md` in this directory and is mirrored as one entry here._

```yaml
batches:
  - number: 1
    name: render-bottom-band
    file: 01-render-bottom-band.md
    depends-on: []
    verify: go test ./internal/reedengine/render/ ./internal/reedengine/
  - number: 2
    name: vocabulary-and-config
    file: 02-vocabulary-and-config.md
    depends-on: [1]
    verify: go test ./internal/tokenvocab/ ./internal/reedengine/ ./internal/hubgeom/ ./internal/standalonegeom/ ./internal/configsync/
  - number: 3
    name: selvage-pane
    file: 03-selvage-pane.md
    depends-on: [2]
    verify: go test ./internal/reedengine/... && go test -tags integration ./internal/reedengine/...
  - number: 4
    name: status-line-pins
    file: 04-status-line-pins.md
    depends-on: [3]
    verify: go test ./internal/reedengine/ && go test -tags integration ./internal/reedengine/
  - number: 5
    name: watchdog-daemon
    file: 05-watchdog-daemon.md
    depends-on: [4]
    verify: go test ./internal/reedcli/ ./internal/reedengine/ ./internal/clihelp/ ./cmd/lyx/
  - number: 6
    name: standalone-watcher
    file: 06-standalone-watcher.md
    depends-on: [5]
    verify: go test ./internal/burlercli/ ./internal/webstercli/
  - number: 7
    name: docs-smokes-and-residue
    file: 07-docs-smokes-and-residue.md
    depends-on: [6]
    verify: go test ./internal/lyxcwd/ ./cmd/lyx/ ./internal/reedcli/ ./tools/sandbox/ && go vet -tags smoke ./internal/reedcli/
```

## Shared Decisions

_Cross-cutting decisions every batch inherits: naming conventions, error-handling posture, test frameworks, style/lint constraints.
One subsection per decision.
Batch-local decisions live in each batch file._

### Decision: band-vocabulary-is-selvage-everywhere

- **Decision:** the fixed-height band is named **Selvage** in every layer it surfaces in — `render.Selvage`/`Params.Selvage`, `ReedState.SelvagePaneID` (`json:"selvagePaneId,omitempty"`), `ensureSelvagePaneLocked`, `splitSelvagePaneAtBottomLocked`, `reed.yaml`'s `selvage:` block — with one deliberate exception: `render`'s internal height helper is `clampBandHeight`, not `clampSelvageHeight`, because the `render` package models a generic fixed band and never learns what the caller uses it for.
- **Rationale:** the discussion's `selvage-is-a-bottom-band-not-a-strand` decision renames rather than adding a position enum, so exactly one band exists and it is at the bottom; keeping `render`'s own internals band-generic preserves the policy/mechanics split `render/types.go`'s package doc already states.
- **Applies to:** all batches

### Decision: no-migration-for-the-renamed-state-field

- **Decision:** an old `reed.json` carrying `headerPaneId` is simply not read. No compatibility shim, no dual-read, no migration step in `loadOrInitStateLocked`.
- **Rationale:** per the discussion's `state-field-rename-with-no-migration` decision the degrade is already correct and costs no code — on the first `up` after the upgrade `SelvagePaneID` is empty, `ensureSelvagePaneLocked` creates Selvage at the bottom, and `planReconcile`'s reap (authorized by an alive Selvage) kills the now-untracked old header pane on the same pass.
- **Applies to:** selvage-pane

### Decision: geometry-tmux-failures-stay-non-fatal

- **Decision:** every new `set-option` this task adds in `internal/reedengine/windowsize.go` is logged via `logger.Warn` naming the socket, the session, the option and the error, and then ignored. None of them is ever returned as an error, and a failure never stops the calls after it.
- **Rationale:** this is the already-live `geometry-tmux-failures-are-non-fatal-everywhere` posture `pinGeometryOptionsLocked` is built on; it is also what makes the Windows/psmux degrade self-correcting, since `readStatusRowsLocked` reads `#{status}` back rather than assuming what was set.
- **Applies to:** status-line-pins

### Decision: windows-status-line-is-an-unbranched-accepted-degrade

- **Decision:** no `runtime.GOOS == "windows"` branch is written for the seven new status-line options. They are issued on every platform and psmux may reject some or all of them; on rejection Windows loses the identity text while Selvage, the layout, the reap rules and the watchdog are all unaffected.
- **Rationale:** per the discussion's `windows-status-line-is-an-accepted-named-degrade` decision, a refused `set-option` fails loudly into the log, changes nothing, and is answered by the `#{status}` readback — the opposite shape from `hookInstalledLocked`'s silent-and-unrecoverable Windows case, which is the one place reed does branch. Skipping the attempt would guarantee the regression rather than risk it.
- **Applies to:** status-line-pins, docs-smokes-and-residue

### Decision: windows-psmux-verification-is-an-open-item-carried-by-this-plan

- **Decision:** the seven status-line options' behaviour under psmux is **unverified**, not verified. Card 47 in `docs-smokes-and-residue` records the verification item in the shipped design doc as an explicitly open item with the exact command to run (`lyx reed up` under psmux, then reading back `#{status}`, `#{status-position}`, `#{status-left}` and `#{window-status-format}`), rather than the doc asserting a settled outcome.
- **Rationale:** the discussion's own "Note for the plan" on that decision requires the plan to carry a Windows verification item and to revisit the decision against its result rather than treating it as settled; nobody on this task has a Windows host, so recording the item is the honest discharge.
- **Applies to:** docs-smokes-and-residue

### Decision: told-geometry-keeps-the-daemon-out-of-reedengine

- **Decision:** `internal/reedcli` owns the watchdog daemon outright — its spawn, its `--hub-path`/`--tmux` flags, its lock path, its discovery loop and its two timing constants. `internal/reedengine` gains exactly one new exported, engine-less function (`ListSessions`) and learns nothing about the daemon's existence.
- **Rationale:** CONSTRAINTS.md's Told-Geometry Invariant bars `reedengine` from importing `lyxcwd`, and the Durable-vs-Ephemeral State Invariant bars it from deriving its own `.lyx` path; `reedcli` already holds the `*lyxcwd.Location`, already imports `hubgeom`, and may import `fabricengine` directly as `hubgeom` does.
- **Applies to:** watchdog-daemon

### Decision: daemon-idle-rule-is-anything-but-an-affirmative-listing

- **Decision:** the daemon's idle counter increments on **anything other than "exit 0 with at least one session name"** — an exit-0 empty listing, a "no server running" error, and any other `list-sessions` failure alike. A non-empty listing resets it to zero; three consecutive increments exit the daemon.
- **Rationale:** the normal last-`down` case is an *error*, not an empty list, so a rule counting only exit-0-empty would leave the daemon's own main exit path undefined; and `internal/reedengine/proctree_windows.go` records that psmux exits identically with and without a server, so any rule reading the error text would be unimplementable there.
- **Applies to:** watchdog-daemon

### Decision: per-card-commits-with-docs-riding-their-own-card

- **Decision:** every card produces exactly one commit. Documentation that describes a mechanism this task changes rides in the batch that changes that mechanism wherever the doc and the code are the same card's concern (e.g. `internal/reedengine/doc.go`'s Selvage sections in `selvage-pane`); the standalone prose deliverables — the module design doc, `manifest/roadmap.md`, `docs/overview.md`, the standalone-API inventory and the sandbox suite — land in `docs-smokes-and-residue`.
- **Rationale:** this repo's CLAUDE.md requires a module doc, `docs/overview.md` and `CONSTRAINTS.md` to move in the same commit as the change they describe; the `CONSTRAINTS.md` exception-list edit therefore rides card 35 in `watchdog-daemon` (the commit that introduces the `watchdog` verb), not the docs batch.
- **Applies to:** all batches

### Decision: the-header-scan-is-re-run-and-its-residue-is-in-scope

- **Decision:** the case-insensitive `header` scan over `.go`/`.md` (excluding `.git/` and `_mill/`) the discussion's Testing section mandates was re-run during planning. Four surviving sites it names that the discussion's own disposition list does **not** enumerate are in scope and carry cards of their own: `cmd/lyx/helptree_test.go:84` (card 38), `cmd/lyx/stencilseed.go:46` (card 39), `internal/reedcli/smoke_panecwd_test.go` (card 52), and `tools/sandbox/SANDBOX-REED-SUITE.md:451` (card 50, a third header-pane-log check beside the two at lines 409 and 427 the discussion did name).
- **Rationale:** the discussion states outright that the plan re-runs that scan and treats any surviving site it names that is not listed as an unhandled case, not as out of scope.
- **Applies to:** watchdog-daemon, docs-smokes-and-residue

### Decision: reedcli-testmain-loses-its-stand-in-but-keeps-its-file

- **Decision:** `internal/reedengine/testmain_test.go` is deleted outright, but `internal/reedcli/testmain_test.go` is **edited**, not deleted: its `os.Args[1] == "reed"` keepalive stand-in branch goes away while the file and its `gitkit.HermeticGitEnv()` call stay.
- **Rationale:** this corrects the discussion's "delete both" wording against the code. `internal/reedcli` has twelve test files carrying git-spawning tokens (`hubforge.`/`gitexec.`/`gitkit.Copy`), so CONSTRAINTS.md's Hermetic Git Test Environment Invariant — enforced by `cmd/lyx/hermeticenv_test.go`'s `TestHermeticGitEnv_GitSpawningPackagesHaveTestMain` — requires that package to keep a `TestMain` calling `gitkit.HermeticGitEnv()`. `internal/reedengine`'s own test files carry none of those tokens, so its `TestMain` has no such obligation and the whole file goes.
- **Applies to:** docs-smokes-and-residue

### Decision: done-gate-stays-as-configured

- **Decision:** `pipeline.done_gate` is left at its already-configured `go test ./... && go test -tags integration ./...` and no batch proposes changing it.
- **Rationale:** the configured gate is already a repo-wide test command covering both the untagged and the integration tier, which is exactly what this task's cross-package spread (reedengine, reedcli, tokenvocab, hubgeom, standalonegeom, configsync, burlercli, webstercli, cmd/lyx) needs. The repo defines no lint command this plan would add to it, and `golangci-lint` is not configured here.
- **Applies to:** all batches

## All Files Touched

_Full union of every `Creates:` / `Edits:` / `Moves:` **target** path across every batch, sorted alphabetically (Move **source** paths are excluded — they disappear, like `Deletes:` tokens)._

- `.gitattributes`
- `CONSTRAINTS.md`
- `cmd/lyx/helptree_test.go`
- `cmd/lyx/stencilseed.go`
- `cmd/lyx/stencilseedgate_test.go`
- `cmd/lyx/tiersleep_test.go`
- `docs/overview.md`
- `internal/burlercli/cli.go`
- `internal/burlercli/run.go`
- `internal/burlercli/wiring.go`
- `internal/burlercli/wiring_test.go`
- `internal/clihelp/annotations_test.go`
- `internal/configsync/configsync_test.go`
- `internal/hubgeom/hubgeom.go`
- `internal/hubgeom/hubgeom_test.go`
- `internal/loomcli/bootstrap.go`
- `internal/reedcli/attach.go`
- `internal/reedcli/cli.go`
- `internal/reedcli/cli_test.go`
- `internal/reedcli/header.go`
- `internal/reedcli/resume.go`
- `internal/reedcli/smoke_dotfill_test.go`
- `internal/reedcli/smoke_lifecycle_test.go`
- `internal/reedcli/smoke_panecwd_test.go`
- `internal/reedcli/smoke_selvage_keepalive_test.go`
- `internal/reedcli/smoke_staterecovery_test.go`
- `internal/reedcli/smoke_statuslineseed_test.go`
- `internal/reedcli/smoke_test.go`
- `internal/reedcli/spawnwatchdog.go`
- `internal/reedcli/spawnwatchdog_test.go`
- `internal/reedcli/statusline.go`
- `internal/reedcli/statusline_test.go`
- `internal/reedcli/testmain_test.go`
- `internal/reedcli/up.go`
- `internal/reedcli/watchdog.go`
- `internal/reedcli/watchdog_integration_test.go`
- `internal/reedcli/watchdog_test.go`
- `internal/reedengine/apply.go`
- `internal/reedengine/apply_test.go`
- `internal/reedengine/attach.go`
- `internal/reedengine/attach_test.go`
- `internal/reedengine/attachgeometry_integration_test.go`
- `internal/reedengine/config.go`
- `internal/reedengine/config_test.go`
- `internal/reedengine/contract_integration_test.go`
- `internal/reedengine/doc.go`
- `internal/reedengine/generation.go`
- `internal/reedengine/generation_test.go`
- `internal/reedengine/geometry.go`
- `internal/reedengine/lifecycle.go`
- `internal/reedengine/lifecycle_test.go`
- `internal/reedengine/lock.go`
- `internal/reedengine/overlay.go`
- `internal/reedengine/overlay_test.go`
- `internal/reedengine/reconcile.go`
- `internal/reedengine/reconcile_test.go`
- `internal/reedengine/render/height.go`
- `internal/reedengine/render/height_test.go`
- `internal/reedengine/render/layout.go`
- `internal/reedengine/render/pins_test.go`
- `internal/reedengine/render/policy.go`
- `internal/reedengine/render/policy_test.go`
- `internal/reedengine/render/rules.go`
- `internal/reedengine/render/rules_test.go`
- `internal/reedengine/render/types.go`
- `internal/reedengine/spawn.go`
- `internal/reedengine/spawn_test.go`
- `internal/reedengine/state.go`
- `internal/reedengine/state_test.go`
- `internal/reedengine/status-line.md`
- `internal/reedengine/statusline.go`
- `internal/reedengine/statusline_test.go`
- `internal/reedengine/statuslinetemplate.go`
- `internal/reedengine/strand_test.go`
- `internal/reedengine/template.go`
- `internal/reedengine/template_posix.yaml`
- `internal/reedengine/template_windows.yaml`
- `internal/reedengine/watchloop.go`
- `internal/reedengine/windowsize.go`
- `internal/reedengine/windowsize_test.go`
- `internal/standalonegeom/reedgeom.go`
- `internal/standalonegeom/standalonegeom_test.go`
- `internal/tokenvocab/doc.go`
- `internal/tokenvocab/render.go`
- `internal/tokenvocab/tokenvocab.go`
- `internal/tokenvocab/tokenvocab_test.go`
- `internal/webstercli/cli.go`
- `internal/webstercli/cli_test.go`
- `internal/webstercli/recoverbatch.go`
- `internal/webstercli/run.go`
- `internal/webstercli/wiring.go`
- `internal/webstercli/wiring_test.go`
- `manifest/designs/reed-fabric-standalone-api.md`
- `manifest/designs/reed-header-selvage.md`
- `manifest/roadmap.md`
- `tools/sandbox/SANDBOX-REED-SUITE.md`
