// redfirst.go holds what `lym gate red-first` and the site that runs it share.
// They share the call, its exit codes and the record names.

package gateslot

// RedFirstGreenExit is the exit code of a `lym gate red-first` call.
// Every test the diff adds or changes passed on the base in that call.
// The site reads it as a fix whose test would have passed before the fix.
const RedFirstGreenExit = 3

// RedFirstNoTestExit is the exit code of a `lym gate red-first` call whose diff adds or changes no test.
const RedFirstNoTestExit = 4

// RedFirstUnjudgedExit is the exit code of a `lym gate red-first` call that cannot judge the diff's tests on the base.
// The hub's scoper is not go, or the diff changes a non-Go file of a Go package, which the base run cannot put back.
// Only a parent's recorded NoTestSeamTitle decision lifts it.
const RedFirstUnjudgedExit = 5

// BugLabel is the board label of an entry whose run must show its fix's test failing on the merge base.
const BugLabel = "bug"

// NoTestSeamTitle opens the title of the decision record's added entry that names a bug no test can reach.
// The parent records that entry as its stop, and the red-first gate then passes without running the call.
const NoTestSeamTitle = "No test seam"

// RedFirstCall renders the call a site runs to check that the diff's tests fail on the merge base.
// It is `lym gate red-first --gate <key>`, and runs the tiers of the named gate key.
func RedFirstCall(key string) string {
	return "lym gate red-first --gate " + key
}
