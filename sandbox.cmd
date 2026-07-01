@echo off
REM Launcher for the lyx sandbox Hub builder. The machine-specific parent directory is
REM hardcoded HERE (the base under which sandbox Hubs are created) — the Go tool stays general.
REM cd to the repo root (%~dp0) so `go run` finds go.mod; restore cwd on exit.
REM %~dp0 is also the loomyard repo root: the fetched sandbox-report.json lands
REM under its .scratch/ directory. The trailing "." after %~dp0 prevents the
REM directory's trailing backslash from escaping the closing quote.
pushd "%~dp0"
go run ./tools/sandbox -parent C:\Code -loomyard "%~dp0." %*
set EXITCODE=%ERRORLEVEL%
popd
exit /b %EXITCODE%
