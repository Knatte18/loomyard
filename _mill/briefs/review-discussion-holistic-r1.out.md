I have enough to assess the discussion. The claims check out against source; the gaps are in the Linux design specifics the plan writer will need.

MILL_REVIEW_BEGIN
# Review: Facilitate Linux support (Win11-side prep)

```yaml
verdict: GAPS_FOUND
reviewer_model: opushigh
reviewed_file: _mill/discussion.md
date: 2026-07-09
```

## Findings

### [GAP] serverProcessesOnSocket Linux impl unspecified
**Section:** Decisions § proc-tree-reaping
**Issue:** The decision details only the `descendantClosurePIDs` `/proc/<pid>/stat` PPID-walk; it never says how the Linux `serverProcessesOnSocket` discovers server processes on a `-L` socket — and its Windows rationale ("every psmux CLI probe exits 0/1 identically, so the process table is the ONLY liveness signal") does not hold for tmux, whose `list-sessions` returns non-zero when no server runs. Also `"psmux.exe"`/`__warm__` are hardcoded and have no tmux analog.
**Fix:** Decide and record whether Linux ports a `/proc/<pid>/cmdline` socket scan or instead uses tmux's real absence signal, and specify how the socket/server-name match is done.

### [GAP] Single multiplexer version pin spans two version schemes
**Section:** Decisions § config-defaults-and-version-pin / capability-probe
**Issue:** psmux and tmux are distinct binaries with independent `-V` output and version numbering; one "pinned min-version constant" cannot meaningfully compare against both.
**Fix:** Specify a GOOS-selected (per-binary) pin and how the probe parses each binary's `-V` shape.

### [NOTE] Shell family keyed on GOOS vs. config-driven pane shell
**Section:** Decisions § shell-abstraction
**Issue:** `internal/shell` selects pwsh/posix by `runtime.GOOS`, but the pane's actual shell is `cfg.Pwsh` (env-overridable, e.g. `LYX_MUX_PWSH`); a pwsh override on Linux would type posix syntax into a pwsh pane.
**Fix:** Note the assumption that GOOS determines pane-shell family, or key the selector off the configured pane-shell binary.

### [NOTE] Shuttle template does not ship pwsh
**Section:** Decisions § config-defaults-and-version-pin
**Issue:** The decision says make defaults GOOS-aware in "muxengine (and shuttle, where it ships pwsh)", but `internal/shuttleengine/template.yaml` ships `claude`, not `pwsh`/`psmux` — only `muxengine/template.yaml` has those keys.
**Fix:** Drop the shuttle parenthetical or correct it (the pwsh binary is muxengine's `cfg.Pwsh`; shuttle has none).

### [NOTE] .sh-vs-.cmd extension ownership is ambiguous
**Section:** Decisions § sh-launchers (Geometry-invariant note)
**Issue:** Only `MenuLauncherPath`'s `ide-menu.cmd` lives in `hubgeometry`; `ide.cmd`/`warp-checkout.cmd` are built in `warpengine/launchers.go` (not geometry tokens), yet the note says "filename must remain owned by hubgeometry," which overreaches and could mislead the plan on where the `.sh` extension logic lives.
**Fix:** Clarify that only the menu launcher's extension moves to hubgeometry; the other launcher filenames/extensions stay in warpengine.

### [NOTE] .sh generation testability under inline runtime.GOOS
**Section:** Testing § .sh launcher generation
**Issue:** `writeLaunchers` branches on `runtime.GOOS` inline, so the Linux branch cannot be exercised by TDD on a Windows dev/CI box without a seam.
**Fix:** Note extraction of a pure, GOOS-parameterized content builder so both branches are fixture-testable regardless of host OS.

## Verdict

GAPS_FOUND
Two Linux-specific designs (socket/server discovery, version pin) are under-specified for a plan writer.
MILL_REVIEW_END
