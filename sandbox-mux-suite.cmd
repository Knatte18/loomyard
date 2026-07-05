@echo off
REM Launcher for the lyx SANDBOX-MUX-SUITE: runs the interactive mux black-box
REM agent session (live psmux required). The machine-specific parent directory
REM is hardcoded HERE (the base under which sandbox Hubs are created) — the Go
REM tool stays general.
REM cd to the repo root (%~dp0) so `go run` finds go.mod; restore cwd on exit.
pushd "%~dp0"
go run ./tools/sandbox -parent C:\Code mux-suite %*
set EXITCODE=%ERRORLEVEL%
popd
exit /b %EXITCODE%
