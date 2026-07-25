@echo off
REM Local launcher for the lyx DEV deploy target. No machine-specific path here —
REM -dev resolves the derived .dev-bin directory (see tools/internal/devbin), so
REM this launcher never overwrites the production install deploy.cmd targets.
REM cd to the repo root (%~dp0) so `go run` finds go.mod; restore cwd on exit.
pushd "%~dp0"
go run ./tools/deploy -dev %*
set EXITCODE=%ERRORLEVEL%
popd
exit /b %EXITCODE%
