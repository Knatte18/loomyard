**Summary of what was implemented:**

**Card 1** — Created `internal/boardengine/` (18 files): all domain files from `internal/board` with `package board` → `package boardengine`, external tests renamed `board_test` → `boardengine_test`, internal tests (`template_test.go`) as `package boardengine`. Committed `786ca1b`.

**Card 2** — Created `internal/boardengine/boardtest/` (5 files): all boardtest files updated to import `boardengine`; `bench_test.go` imports both `boardengine` (Task, Config, New, Render, Outputs) and `boardcli` (RunCLI). Committed `e67152d`.

**Card 3** — Created `internal/boardcli/` (4 files): CLI files with `package boardcli`, all domain symbols retargeted to `boardengine.*` (`Board`, `Config`, `Task`, `BriefTask`, `MergeStatusUpdate`, `New`, `LoadConfig`). External tests as `boardcli_test`, internal test uses `boardengine.Config`. Committed `c762928`.

**Card 4** — Retargeted importers: `cmd/lyx/main.go` (`boardcli.Command()`), `internal/configreg/configreg.go` (`boardengine.ConfigTemplate`), `internal/ide/menu.go` (`boardengine.LoadConfig/New`), `internal/initcli/initcli_test.go` (`boardengine.LoadConfig`). Deleted `internal/board` (27 files). Committed `ced928d`.

**Pre-existing failure:** `internal/paths.TestEnforcement/tree-scan` fails because `tools/sandbox/suite.go` uses `os.Getwd` — this failure existed before the batch (confirmed with empty stash test). No new path-invariant violations were introduced; `boardcli/cli.go` uses `paths.Getwd()` per the constraint.

**Coordinator note acknowledged:** Future rename-heavy cards should use `git mv` + surgical `Edit` calls rather than full-file `Write`. The already-committed cards cannot be practically rewritten with history rewrites since they are pushed.

{"status":"success","commit_sha":"ced928dc5708429a483826a84fc2056d2890da06","session_id":"8dd70085-5f61-4e88-a59b-744a6f07a6ff"}