@echo off
REM Launcher for the lyx sandbox Hub builder: clones (or with -reset re-clones) the Hub.
REM The machine-specific parent directory is hardcoded HERE (the base under which
REM sandbox Hubs are created) — the Go tool stays general.
REM cd to the repo root (%~dp0) so `go run` finds go.mod; restore cwd on exit.
pushd "%~dp0"
go run ./tools/sandbox -parent C:\Code build %*
set EXITCODE=%ERRORLEVEL%
popd
exit /b %EXITCODE%
