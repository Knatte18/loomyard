@echo off
REM Local launcher for the lyx deploy tool. The machine-specific install dir is
REM hardcoded HERE (a dir on this machine's PATH) — the Go tool stays general.
REM cd to the repo root (%~dp0) so `go run` finds go.mod; restore cwd on exit.
pushd "%~dp0"
go run ./tools/deploy -dest C:\Code\tools\bin %*
set EXITCODE=%ERRORLEVEL%
popd
exit /b %EXITCODE%
