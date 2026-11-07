// status_enforcement_test.go is the tripwire on reads of quarry's resolve-status vocabulary.
// It parses the production files directly under internal/planglyph.
// It fails on a read of that vocabulary outside an allowlisted function.
// A consumer that reads the vocabulary without a guard fails open when quarry widens it.
// It fails open the same way on a pre-resolution rejection, so each new read must fail closed before it ships.
// It does not verify that the allowlisted consumers handle the vocabulary correctly; their own tests do.
// It covers this package alone: internal/quarrycli also reads .Status,
// but it renders quarry's answer back rather than deciding a plan or gate from it.

package planglyph

import (
	"go/ast"
	"go/parser"
	"go/token"
	"testing"

	"github.com/Knatte18/loomyard/internal/testkit/scankit"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// statusVocabularySelectors is the closed set of selector names that read quarry's resolve-status vocabulary.
// statusHitsIn matches exactly these names.
//
// "Status" is the field itself.
// "Known" is the predicate quarry.Status.Known, and "Rejected" is the predicate quarry.ResolveResult.Rejected.
//
// "Rejected" is true only when Status is empty.
// It reports false for an out-of-vocabulary status, where !Known() reports true.
// A consumer written as `if r.Rejected() { ... } else { treat as resolved }` fails open on a vocabulary widening,
// and it names no Status selector, so a scan keyed on Status alone would miss it.
// "Known" is matched too: it is the fail-closed spelling, and its consumers belong in the same allowlist.
//
// "Unit" is the last spelling: ResolveResult.Unit is a Status-typed field,
// so a consumer that branches on it alone names no other selector in this set.
// The match is by name alone, so a Unit selector on a non-status type trips it too.
// Such a consumer needs an allowlist row, like any status reader.
var statusVocabularySelectors = map[string]bool{
	"Status":   true,
	"Known":    true,
	"Rejected": true,
	"Unit":     true,
}

// statusHit records one selector that reads the resolve-status vocabulary.
// Its `fn` field holds the enclosing function's name, or the empty string when the selector sits at package scope.
// A hit at package scope can never be allowlisted.
type statusHit struct {
	fn string
}

// statusConsumerKey returns the allowlist key of one (file, function) pair:
// the file's module-relative path and the enclosing function, joined by a colon.
func statusConsumerKey(rel, fn string) string {
	return rel + ":" + fn
}

// allowedStatusConsumers is the tripwire's allowlist of functions that read the vocabulary.
// Each key names a (file, function) pair that handles the vocabulary fail-closed.
// Each readable status is handled explicitly.
// Every other value, the zero value included, goes through a default or else arm.
// A new vocabulary read must fail closed, then be added here; until it is, the tripwire fails.
var allowedStatusConsumers = []scankit.Entry{
	{Key: "internal/planglyph/resolve.go:statusFindings", Why: "fail-closed status reader"},
	{Key: "internal/planglyph/resolve.go:resolveVerdict", Why: "fail-closed status reader: an unreadable answer is missing"},
	{Key: "internal/planglyph/resolve.go:unreadableStatusDetail", Why: "fail-closed status reader"},
	{Key: "internal/planglyph/create.go:createFindings", Why: "fail-closed status reader"},
	{Key: "internal/planglyph/donecheck.go:doneCheckVerdicts", Why: "fail-closed status reader"},
	{Key: "internal/planglyph/donecheck.go:anyMemberMissing", Why: "decides only whether to list declarations for a finding's detail, never a verdict; any answer but not_found lists nothing"},
	{Key: "internal/planglyph/donecheck.go:searchedDescription", Why: "reads glyph.Glyph.Unit, a name-shape match for the status vocabulary's Unit selector that is no resolve status"},
	{Key: "internal/planglyph/donecheck.go:signatureMismatches", Why: "reads glyph.Glyph.Unit, a name-shape match for the status vocabulary's Unit selector that is no resolve status"},
	{Key: "internal/planglyph/donecheck.go:signatureMismatchFinding", Why: "reads glyph.Glyph.Unit, a name-shape match for the status vocabulary's Unit selector that is no resolve status"},
	{Key: "internal/planglyph/donecheck.go:declarationList", Why: "reads glyph.Glyph.Unit, a name-shape match for the status vocabulary's Unit selector that is no resolve status"},
	{Key: "internal/planglyph/deleteorder.go:answerSymbols", Why: "fail-closed status reader"},
	{Key: "internal/planglyph/handle.go:renameDeclSource", Why: "fail-closed status reader"},
	{Key: "internal/planglyph/resign.go:RebindResigns", Why: "fail-closed status reader: only found passes, every other answer is resign-not-done"},
	{
		Key: "internal/planglyph/resign.go:nameResigns",
		Why: "reads Unit-named selectors that are NOT the status vocabulary -- glyph.Glyph.Unit, quarry.Declaration.Unit and quarry.NameResult.Unit, compared in its own echo guard -- so the reviewed function earns its row here rather than a carve-out in the matcher",
	},
	{
		Key: "internal/planglyph/resign.go:sameMemberIgnoringParams",
		Why: "reads glyph.Glyph.Unit, a name-shape match for the status vocabulary's Unit selector that is no resolve status",
	},
	{
		Key: "internal/planglyph/handle.go:CanonicalizeHandles",
		Why: "reads Unit-named selectors that are NOT the status vocabulary -- quarry.NameResult.Unit and quarry.Declaration.Unit, the batched Name call's echo pair, compared to each other in its own fail-closed echo guard -- but the scan is a name-shape check and cannot tell those apart from ResolveResult.Unit, so the reviewed function earns its row here rather than a carve-out in the matcher",
	},
}

// statusHitsIn returns one statusHit for each selector in astFile that reads the vocabulary.
// A selector inside a top-level function body is tagged with that function's name.
// A selector in any other top-level declaration, such as a var or const, is tagged with the empty string.
func statusHitsIn(astFile *ast.File) []statusHit {
	var hits []statusHit
	find := func(node ast.Node, fn string) {
		ast.Inspect(node, func(n ast.Node) bool {
			sel, ok := n.(*ast.SelectorExpr)
			if ok && sel.Sel != nil && statusVocabularySelectors[sel.Sel.Name] {
				hits = append(hits, statusHit{fn: fn})
			}
			return true
		})
	}
	for _, decl := range astFile.Decls {
		fd, ok := decl.(*ast.FuncDecl)
		if !ok {
			find(decl, "")
			continue
		}
		if fd.Body == nil {
			continue // no body to scan (an external/assembly declaration).
		}
		find(fd.Body, fd.Name.Name)
	}
	return hits
}

// TestStatusEnforcement_NoOutOfAllowlistConsumer fails on a vocabulary read outside an allowlisted function.
// It scans the production files directly under internal/planglyph.
// Test files are skipped: a test's own assertions against a synthetic status are not a consumer.
// It spawns no process and carries no build tag.
func TestStatusEnforcement_NoOutOfAllowlistConsumer(t *testing.T) {
	tiergate.Require(t, 0)
	allow := scankit.NewAllowlist(allowedStatusConsumers)

	var failures []string
	scanned := scankit.Walk(t, scankit.Options{Roots: []string{"internal/planglyph"}, Shallow: true}, func(f *scankit.File) {
		for _, hit := range statusHitsIn(f.AST(t, parser.ParseComments)) {
			if allow.Allowed(statusConsumerKey(f.Rel, hit.fn)) {
				continue
			}
			label := hit.fn
			if label == "" {
				label = "<package scope>"
			}
			failures = append(failures, f.Rel+": "+label)
		}
	})

	scankit.RequireFloor(t, scanned, planglyphScanMinFiles, "planglyph status scan")
	allow.RequireNoStale(t)

	if len(failures) > 0 {
		t.Errorf(".Status tripwire fired for consumer(s) not in the allowlist: %v -- a new .Status "+
			"consumer must handle the vocabulary fail-closed (quarry.Status.Known(), or a boolean "+
			"derived only after a vocabulary guard) and then be added to allowedStatusConsumers in "+
			"status_enforcement_test.go; see that file's own doc comment",
			failures)
	}
}

// TestStatusHitsIn_CatchesOutOfAllowlistConsumer is a self-test on a synthetic source string.
// The string reads the vocabulary from a function outside allowedStatusConsumers, and statusHitsIn must report it.
func TestStatusHitsIn_CatchesOutOfAllowlistConsumer(t *testing.T) {
	tiergate.Require(t, 0)
	const src = `package fakeplanglyph

func newUnguardedConsumer(r quarry.ResolveResult) bool {
	return r.Status == "found"
}
`
	fset := token.NewFileSet()
	astFile, err := parser.ParseFile(fset, "fakeplanglyph.go", src, 0)
	if err != nil {
		t.Fatalf("parse fixture source: %v", err)
	}

	got := statusHitsIn(astFile)
	if len(got) != 1 || got[0].fn != "newUnguardedConsumer" {
		t.Errorf("statusHitsIn(...) = %+v; want exactly one hit tagged \"newUnguardedConsumer\"", got)
	}
}

// TestStatusHitsIn_CatchesRejectedConsumer checks that a Rejected read trips statusHitsIn.
// The fixture is the fail-open shape: a consumer that treats "not a pre-resolution rejection" as "resolved".
// That is wrong for an out-of-vocabulary non-empty status, for which Rejected reports false,
// and the fixture names no Status selector, so a scan keyed on the field alone would miss it.
func TestStatusHitsIn_CatchesRejectedConsumer(t *testing.T) {
	tiergate.Require(t, 0)
	const src = `package fakeplanglyph

func newRejectedConsumer(r quarry.ResolveResult) bool {
	if r.Rejected() {
		return false
	}
	return true
}
`
	fset := token.NewFileSet()
	astFile, err := parser.ParseFile(fset, "fakeplanglyph.go", src, 0)
	if err != nil {
		t.Fatalf("parse fixture source: %v", err)
	}

	got := statusHitsIn(astFile)
	if len(got) != 1 || got[0].fn != "newRejectedConsumer" {
		t.Errorf("statusHitsIn(...) = %+v; want exactly one hit tagged \"newRejectedConsumer\" -- a Rejected() read is a status-vocabulary consumer and must trip the tripwire", got)
	}
}

// TestStatusHitsIn_CatchesUnitConsumer checks that a read of Unit trips statusHitsIn.
// ResolveResult.Unit draws from the same vocabulary, so a consumer reading only Unit is a vocabulary consumer.
// The fixture names no Status, Known or Rejected selector, so the match must cover Unit.
func TestStatusHitsIn_CatchesUnitConsumer(t *testing.T) {
	tiergate.Require(t, 0)
	const src = `package fakeplanglyph

func newUnitConsumer(r quarry.ResolveResult) bool {
	return r.Unit == "found"
}
`
	fset := token.NewFileSet()
	astFile, err := parser.ParseFile(fset, "fakeplanglyph.go", src, 0)
	if err != nil {
		t.Fatalf("parse fixture source: %v", err)
	}

	got := statusHitsIn(astFile)
	if len(got) != 1 || got[0].fn != "newUnitConsumer" {
		t.Errorf("statusHitsIn(...) = %+v; want exactly one hit tagged \"newUnitConsumer\" -- a Unit read is a status-vocabulary consumer and must trip the tripwire", got)
	}
}

// TestStatusHitsIn_CatchesKnownConsumer checks that a read of Known trips statusHitsIn.
// Reading the vocabulary through the Known predicate, rather than the field, still needs an allowlist row.
func TestStatusHitsIn_CatchesKnownConsumer(t *testing.T) {
	tiergate.Require(t, 0)
	const src = `package fakeplanglyph

func newKnownConsumer(s quarry.Status) bool {
	return s.Known()
}
`
	fset := token.NewFileSet()
	astFile, err := parser.ParseFile(fset, "fakeplanglyph.go", src, 0)
	if err != nil {
		t.Fatalf("parse fixture source: %v", err)
	}

	got := statusHitsIn(astFile)
	if len(got) != 1 || got[0].fn != "newKnownConsumer" {
		t.Errorf("statusHitsIn(...) = %+v; want exactly one hit tagged \"newKnownConsumer\"", got)
	}
}

// TestStatusHitsIn_UnrelatedSelectorNotCaught checks that statusHitsIn matches on the selector's own name.
// A read of another field of the same type must not be reported as a vocabulary read.
func TestStatusHitsIn_UnrelatedSelectorNotCaught(t *testing.T) {
	tiergate.Require(t, 0)
	const src = `package fakeplanglyph

func readsSomethingElse(r quarry.ResolveResult) string {
	return r.Target
}
`
	fset := token.NewFileSet()
	astFile, err := parser.ParseFile(fset, "fakeplanglyph.go", src, 0)
	if err != nil {
		t.Fatalf("parse fixture source: %v", err)
	}

	if got := statusHitsIn(astFile); len(got) != 0 {
		t.Errorf("statusHitsIn(...) = %+v; want empty -- reading an unrelated field is not a Status consumer", got)
	}
}
