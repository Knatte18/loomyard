// Package fixtries reads a subject's retired fix-tries file.
//
// The file recorded, in the subject's adopt scratch directory, how many fix tasks the subject's check had filed.
// The tries now live in the hold, and no production code writes the file.
// The upgrade's first read is `Read`'s only caller: it carries the count a hub upgraded with into the hold.
// The caller hands in the subject's directory; the package derives no path of its own.
//
// It imports only `internal/state`, so a package that adopt itself imports can read the count without importing adopt.
package fixtries
