// Package testreach is the one cross-package reader of the per-test coverage maps of internal/testcover.
// It runs no git: a caller hands it the file contents of each side it compares.
//
// ChangedFuncs finds the functions a diff changes.
// Module names the Go module the maps hold, and a FileSet maps a worktree-relative slash path to a file's contents.
// A caller puts in each set every Go file of each package whose files it wants compared,
// so a function that moved between files of one package is seen on both sides.
// Only production Go files of the module count.
// A test file and a file outside the module are passed over.
// So is a file under testdata or a directory opening with "." or "_".
// A function is matched across the sets by package and name, whatever its file,
// so a function moved verbatim to another file of its package is unchanged.
// A name that either side defines in more than one file of its package is matched by package, file and name instead.
// A function is changed when it is new or its declaration text differs;
// a function only the base holds is no changed function.
// A file that does not parse is an error naming the file.
// ChangedTests finds the test functions a diff adds or changes, from the `_test.go` files of the same sets.
// ChangedLines returns the 1-based lines of a file's head text that a line diff against its base text adds or changes.
//
// Open reads the coverage maps of the module from two directories, each a testcover.Dir:
// the hub's shared maps and the worktree's own maps, which the prime keeps none of.
// An absent directory reads as one with no maps.
// For a package both hold, the worktree's map wins when Fresh holds for it; otherwise the map built later wins.
// A map that holds only a build failure, names a block or function outside its tables or does not parse is passed over,
// and a map that does not parse is logged, so one corrupt map never stops the other packages from answering.
// ReadPackageMap reads one package's map with the same pass-over rules.
// In its own package a test reaches a function when its blocks hold a block of the function;
// in another package, when the function is among the functions its map lists.
// An entry with an outcome or a spawn mark reaches nothing.
// SpawnMarked and StaleLinkers name the two caveats of an answer:
// a package whose map holds a test that may run code in another process,
// and the packages whose maps recorded a linked package's files before they changed.
//
// The reverse table of the shared maps, from function to reaching tests, is a cache file beside them, the index.
// Open uses the index when its stamp equals the directory's current stamp.
// An absent index, an unreadable one or one whose stamp differs is never refused: Open builds the table in memory.
// Open never writes the index; WriteIndex does.
// Bound: a map rewritten at the same size within one modification-time tick keeps its stamp.
// Open can then serve the index's old pairs until the next WriteIndex; every answer is advisory.
// The worktree's own maps are few and are read whole, never indexed.
package testreach
