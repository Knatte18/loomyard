@echo off
REM Launcher for the lyx SANDBOX-REED-WATCH-SUITE: runs the interactive, non-destructive
REM reed live-observation session (live tmux required). The machine-specific parent directory
REM is hardcoded HERE (the base under which sandbox Hubs are created) — the Go
REM tool stays general.
REM cd to the repo root (%~dp0..\.., two levels up from this sandbox\win folder) so `go run` finds go.mod; restore cwd on exit.
pushd "%~dp0..\.."
go run ./tools/sandbox -parent C:\Code reed-watch-suite %*
set EXITCODE=%ERRORLEVEL%
popd
exit /b %EXITCODE%
