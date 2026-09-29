@echo off
REM Put the checked-out, committed code into production: mirror every installed loomyard plugin
REM into the Claude Code plugin cache and build lyx.exe into the Go bin dir (`go env GOBIN`, else
REM GOPATH\bin). This is the only production route; it refuses a dirty tree. Dev builds go through
REM deploy-dev.cmd. Logic lives in tools/deploy (shared with update-plugins.sh).
REM cd to the repo root (%~dp0) so `go run` finds go.mod; restore cwd on exit.
pushd "%~dp0"
go run ./tools/deploy %*
set EXITCODE=%ERRORLEVEL%
popd
exit /b %EXITCODE%
