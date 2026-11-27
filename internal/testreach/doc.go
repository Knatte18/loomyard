// Package testreach finds the test functions a diff adds or changes.
// It runs no git: a caller hands it the file contents of each side it compares.
//
// Module names the Go module the files belong to, and a FileSet maps a worktree-relative slash path to a file's contents.
// ChangedTests finds the test functions of the `_test.go` files of both sets that the head adds or changes.
// A caller puts in each set every test file of each package whose tests it wants compared,
// so a test that moved between files of one package is seen on both sides.
// A file outside the module is passed over.
// So is a file under testdata or a directory opening with "." or "_".
// A test is matched across the sets by package and name, whatever its file,
// so a test moved verbatim to another file of its package is unchanged.
// A name that either side declares in more than one file of its package is matched by package, file and name instead.
// A test is changed when it is new or its declaration text differs;
// a test only the base holds is no changed test.
// A file that does not parse is an error naming the file.
package testreach
