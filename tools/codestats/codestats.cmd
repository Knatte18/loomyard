@echo off
REM Launcher for the codestats scan: writes the markdown report to .scratch\codestats.md
REM and prints where it landed.
REM One fixed path, overwritten every run rather than accumulating a timestamped file per
REM scan — the run's own date and time live inside the report, under Generated.
REM cd to the repo root (%~dp0..\.., two levels up from this tools\codestats folder) so
REM `go run` finds go.mod; restore cwd on exit.
REM Any extra arguments are passed through, so -root, -exclude and -hidden still work.
pushd "%~dp0..\.."
go run ./tools/codestats -out .scratch/codestats.md %*
set EXITCODE=%ERRORLEVEL%
popd
exit /b %EXITCODE%
