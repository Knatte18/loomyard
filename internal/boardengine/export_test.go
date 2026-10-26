// export_test.go exposes the run-lock judgment to the external tests, for the changes no board write can reach.

package boardengine

// CheckRunLocks judges the net change from loaded to after as a board write does.
var CheckRunLocks = checkRunLocks
