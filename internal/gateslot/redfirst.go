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
// The cause is a hub scoper that is neither go nor dotnet,
// or a C# change whose tests no filter selects by name or no tier of the gate key can run.
// It is also a C# change whose tier run discovered no test.
// The site passes it, since no check can show the test red there and the writer cannot change that.
const RedFirstUnjudgedExit = 5

// BugLabel is the board label of an entry whose run must show its fix's test failing on the merge base.
const BugLabel = "bug"

// NoTestSeamTitle opens the title of the decision record's added entry that names a bug no test can reach.
// The parent records that entry as its stop, and the red-first gate then passes without running the call.
const NoTestSeamTitle = "No test seam"

// NoBugFixedTitle opens the title of the decision record's added entry that names a bug-labelled entry fixing no bug.
// Such an entry only adds tests that pin existing behaviour, so its tests pass on the merge base by design.
// The parent records that entry, and the red-first gate then passes without running the call.
const NoBugFixedTitle = "No bug fixed"

// RedFirstCall renders the call a site runs to check that the diff's tests fail on the merge base.
// It is `lym gate red-first --gate <key>`, and runs the tiers of the named gate key.
func RedFirstCall(key string) string {
	return "lym gate red-first --gate " + key
}
