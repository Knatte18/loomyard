@echo off
REM Launcher for the plan card graph: writes plandag.md, plandag.dot and, with dot on PATH,
REM plandag.svg into .scratch\plandag and prints where each landed.
REM Fixed file names, overwritten every run; the report names the plan it drew.
REM The one argument is the plan directory, made absolute here (%~f1) because the tool runs
REM from the repo root (%~dp0..\.., two levels up from this tools\plandag folder) so
REM `go run` finds go.mod; restore cwd on exit:
REM plandag.cmd ..\some-pair\_lym\plan
if "%~1"=="" (
	echo usage: plandag.cmd ^<plan-dir^> 1>&2
	exit /b 2
)
set PLAN_DIR=%~f1
pushd "%~dp0..\.."
go run ./tools/plandag -out .scratch/plandag "%PLAN_DIR%"
set EXITCODE=%ERRORLEVEL%
popd
exit /b %EXITCODE%
