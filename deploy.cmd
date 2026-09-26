@echo off
REM Local launcher for the lyx deploy tool. The install dir is the machine's own
REM `go env GOBIN`, else GOPATH\bin -- set it per machine with `go env -w GOBIN=<dir>`;
REM pass -dest <dir> for a one-off override.
REM cd to the repo root (%~dp0) so `go run` finds go.mod; restore cwd on exit.
pushd "%~dp0"
go run ./tools/deploy %*
set EXITCODE=%ERRORLEVEL%
popd
exit /b %EXITCODE%
