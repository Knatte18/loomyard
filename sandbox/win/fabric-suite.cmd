@echo off
REM Launcher for the lyx SANDBOX-FABRIC-SUITE: runs the interactive fabric
REM black-box agent session against the same shared Hub the other suites use
REM (run sandbox/build.cmd first if it does not exist yet).
REM The machine-specific parent directory is hardcoded HERE (the base under which
REM sandbox Hubs are created) — the Go tool stays general.
REM cd to the repo root (%~dp0..\.., two levels up from this sandbox\win folder) so `go run` finds go.mod; restore cwd on exit.
pushd "%~dp0..\.."
go run ./tools/sandbox -parent C:\Code fabric-suite %*
set EXITCODE=%ERRORLEVEL%
popd
exit /b %EXITCODE%
