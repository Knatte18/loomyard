@echo off
REM Launcher for the lyx SANDBOX-PERCH-SUITE: runs the interactive perch
REM black-box agent session (live psmux and a logged-in claude required). The
REM machine-specific parent directory is hardcoded HERE (the base under which
REM sandbox Hubs are created) — the Go tool stays general.
REM cd to the repo root (%~dp0) so `go run` finds go.mod; restore cwd on exit.
pushd "%~dp0"
go run ./tools/sandbox -parent C:\Code perch-suite %*
set EXITCODE=%ERRORLEVEL%
popd
exit /b %EXITCODE%
