// coveragemapreader_test.go enforces the reader clause of `PATTERN-coverage-map-reader`.
// Only internal/testreach reads coverage maps across packages, so a new reader of one package's map goes through it too.
// The three single-package readers that call testcover.ReadMap directly are pinned by file.

package main

import (
	"go/ast"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/Knatte18/loomyard/internal/testkit/scankit"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// coverageMapReaderOwners are the directories whose files read coverage maps without a pin.
var coverageMapReaderOwners = []string{"internal/testcover/", "internal/testreach/"}

// coverageMapSingleReaders pins the files outside the owners that call testcover.ReadMap, each with its reason.
var coverageMapSingleReaders = []scankit.Entry{
	{Key: "internal/gatecli/redundancy.go", Why: "the redundancy step reads the map of the one package it judges"},
	{Key: "internal/gatecli/redundant.go", Why: "`lym gate redundant` reads the map of the one package it confirms"},
	{Key: "internal/gatecli/testmappackage.go", Why: "the map run reads the map it rewrites, to keep the entries of a failed build"},
}

// changedFuncsFinderFile is the one file outside internal/testreach that calls testreach.ChangedFuncs,
// so the gate keeps one finder of a diff's changed functions.
const changedFuncsFinderFile = "internal/gatecli/reach.go"

// indexWriterFile is the one file outside internal/testreach that calls testreach.WriteIndex:
// the map run, which from the prime alone writes the index of the shared maps.
const indexWriterFile = "internal/gatecli/testmap.go"

// coverageMapReaderMinScannedFiles is the vacuous-scan floor for the module-wide walk.
const coverageMapReaderMinScannedFiles = 100

// TestCoverageMapReaderBoundary walks the module's non-test Go files.
// It fails on a call of testcover.ReadMap outside internal/testcover, internal/testreach and the pinned readers.
// It also fails when a pinned file no longer holds the call, so the list tracks the tree.
// It fails on a call of testreach.ChangedFuncs outside the pinned finder file, and when that file no longer holds it.
// It fails on a call of testreach.WriteIndex outside the pinned map run file, and when that file no longer holds it.
//
//lym:guard
func TestCoverageMapReaderBoundary(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	pinned := scankit.NewAllowlist(coverageMapSingleReaders)
	var finderCalled, indexWritten atomic.Bool
	scanned := scankit.Walk(t, scankit.Options{}, func(f *scankit.File) {
		for _, owner := range coverageMapReaderOwners {
			if strings.HasPrefix(f.Rel, owner) {
				return
			}
		}
		fset := f.FileSet()
		ast.Inspect(f.AST(t, 0), func(node ast.Node) bool {
			selector, ok := node.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			pkg, ok := selector.X.(*ast.Ident)
			if !ok {
				return true
			}
			switch {
			case pkg.Name == "testcover" && selector.Sel.Name == "ReadMap":
				if !pinned.Allowed(f.Rel) {
					t.Errorf("`PATTERN-coverage-map-reader` violated: %s:%d calls testcover.ReadMap; "+
						"read the maps through internal/testreach (ReadPackageMap for one package, Open across packages), "+
						"or pin the file in coverageMapSingleReaders with the reason it must read one package's map itself",
						f.Rel, fset.Position(selector.Pos()).Line)
				}
			case pkg.Name == "testreach" && selector.Sel.Name == "ChangedFuncs":
				if f.Rel == changedFuncsFinderFile {
					finderCalled.Store(true)
					break
				}
				t.Errorf("`PATTERN-coverage-map-reader` violated: %s:%d calls testreach.ChangedFuncs; "+
					"find a diff's changed functions through changedFuncsAt in %s",
					f.Rel, fset.Position(selector.Pos()).Line, changedFuncsFinderFile)
			case pkg.Name == "testreach" && selector.Sel.Name == "WriteIndex":
				if f.Rel == indexWriterFile {
					indexWritten.Store(true)
					break
				}
				t.Errorf("`PATTERN-coverage-map-reader` violated: %s:%d calls testreach.WriteIndex; "+
					"only the prime's map run in %s writes the index of the shared maps",
					f.Rel, fset.Position(selector.Pos()).Line, indexWriterFile)
			}
			return true
		})
	})
	scankit.RequireFloor(t, scanned, coverageMapReaderMinScannedFiles, "coverage map reader boundary guard")
	for _, file := range pinned.Stale() {
		t.Errorf("`PATTERN-coverage-map-reader`: pinned file %s no longer calls testcover.ReadMap; "+
			"remove its entry from coverageMapSingleReaders", file)
	}
	if !finderCalled.Load() {
		t.Errorf("`PATTERN-coverage-map-reader`: %s no longer calls testreach.ChangedFuncs; "+
			"update changedFuncsFinderFile to the file that holds the one finder", changedFuncsFinderFile)
	}
	if !indexWritten.Load() {
		t.Errorf("`PATTERN-coverage-map-reader`: %s no longer calls testreach.WriteIndex; "+
			"update indexWriterFile to the file that holds the one index writer", indexWriterFile)
	}
}
