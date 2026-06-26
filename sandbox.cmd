@echo off
REM Launcher for the lyx sandbox Hub builder. The machine-specific parent directory is
REM hardcoded HERE (the base under which dogfood Hubs are created) — the Go tool stays general.
REM cd to the repo root (%~dp0) so `go run` finds go.mod; restore cwd on exit.
pushd "%~dp0"
go run ./tools/sandbox -parent C:\Code %*
set EXITCODE=%ERRORLEVEL%
popd
exit /b %EXITCODE%
