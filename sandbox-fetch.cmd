@echo off
REM Launcher for the lyx sandbox fetch: collects the agent report into this repo's .scratch/.
REM The machine-specific parent directory is hardcoded HERE (the base under which
REM sandbox Hubs are created) — the Go tool stays general.
REM cd to the repo root (%~dp0) so `go run` finds go.mod; restore cwd on exit.
REM %~dp0 is also the loomyard repo root: the fetched sandbox-report.json lands
REM under its .scratch/ directory. The trailing "." after %~dp0 prevents the
REM directory's trailing backslash from escaping the closing quote.
pushd "%~dp0"
go run ./tools/sandbox -parent C:\Code -loomyard "%~dp0." fetch %*
set EXITCODE=%ERRORLEVEL%
popd
exit /b %EXITCODE%
