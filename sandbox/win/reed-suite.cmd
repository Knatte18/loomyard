@echo off
REM Launcher for the lyx SANDBOX-REED-SUITE: runs the interactive reed black-box
REM agent session (live tmux required). The machine-specific parent directory
REM is hardcoded HERE (the base under which sandbox Hubs are created) — the Go
REM tool stays general.
REM cd to the repo root (%~dp0..\.., two levels up from this sandbox\win folder) so `go run` finds go.mod; restore cwd on exit.
pushd "%~dp0..\.."
go run ./tools/sandbox -parent C:\Code reed-suite %*
set EXITCODE=%ERRORLEVEL%
popd
exit /b %EXITCODE%
