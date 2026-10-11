@echo off
REM Launcher for the token and time count: writes the markdown report to .scratch\token-usage-by-role.md
REM and prints where it landed.
REM One fixed path, overwritten every run rather than accumulating a timestamped file per
REM count — the run's own date and time and the counted slugs live inside the report.
REM cd to the repo root (%~dp0..\.., two levels up from this tools\tokencount folder) so
REM `go run` finds go.mod and -hub defaults to the hub; restore cwd on exit.
REM The arguments are the task slugs to count, partial runs included, and flags such as -hub
REM pass through; with no slugs it counts the most recently active full loom runs, those that
REM reached Finalize (-last, default 6).
pushd "%~dp0..\.."
go run ./tools/tokencount -out .scratch/token-usage-by-role.md %*
set EXITCODE=%ERRORLEVEL%
popd
exit /b %EXITCODE%
