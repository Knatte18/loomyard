// status_enforcement_test.go is the resolve-status tripwire: it parses every production .go file
// directly under internal/planglyph and flags any ast.SelectorExpr reading quarry's status
// vocabulary -- a Sel named "Status", "Known" or "Rejected" (statusVocabularySelectors) -- that sits
// outside an allowlisted enclosing function. quarry.ResolveResult.Status is a vocabulary owned by
// quarry.Statuses (today StatusFound, StatusMultipart, StatusNotFound, StatusAmbiguous) plus the
// zero value (quarry's own pre-resolution rejection shape); a consumer that reads it without a
// vocabulary guard fails OPEN the moment quarry widens the vocabulary or answers with a
// pre-resolution rejection, exactly the family of defect crucible rounds opus-high-r9 (R9-6) and
// fable-high-r10 (F1) both found and fixed one call site at a time. This test does not re-verify
// that today's allowlisted consumers actually handle the vocabulary correctly -- their own
// regression tests do that. It exists so that a NEW status read, added later without reading this
// file, is caught before it ships: the failure message instructs the author to make the new
// consumer fail closed (quarry.Status.Known(), the canonical spelling of that guard, or a boolean
// derived only after one) and then add it here, rather than letting silence stand in for review.
//
// The scan matches all THREE spellings the vocabulary can be read through, not the field alone.
// quarry v0.2.0 introduced Status.Known and ResolveResult.Rejected. A scan keyed on "Status" caught
// the first only incidentally -- Known is called ON r.Status, so the field selector is still there --
// and the second not at all, since Rejected names no "Status" anywhere. That blind spot pointed the
// wrong way: Rejected is the WEAKER predicate (Status == "", and therefore false for an
// out-of-vocabulary non-empty status, where !Known() is true), so the one new spelling able to ship
// a fail-open consumer was precisely the one the tripwire could not see (crucible round
// opus5-high-r1, F2). See statusVocabularySelectors.
//
// The scan is deliberately scoped to this package alone. internal/quarrycli also reads .Status, but
// it renders quarry's own answer verbatim back to the operator rather than making a plan/gate
// decision from it -- it is outside the validation surface this invariant hardens, and adding it to
// the scan would either force it into planglyph's allowlist shape for no reason or produce a
// permanent false positive.

package planglyph

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// statusVocabularySelectors is the closed set of selector names that read quarry's resolve-status
// vocabulary, and therefore the set statusHitsIn matches.
//
// "Status" is the field itself. "Known" and "Rejected" are the two predicates quarry v0.2.0 added
// (quarry.Status.Known, quarry.ResolveResult.Rejected); before that bump the field was the only way
// to read the vocabulary, which is why the original scan matched "Status" alone.
//
// Matching "Rejected" is the load-bearing addition, because the two predicates are NOT complements:
// Rejected is Status == "", so it is FALSE for an out-of-vocabulary non-empty status, while
// !Known() is TRUE for it. A consumer written as `if r.Rejected() { ... } else { treat as resolved }`
// therefore fails OPEN on exactly the vocabulary-widening case this tripwire exists to catch -- and
// named no "Status" selector, so it shipped invisible to the scan (crucible round opus5-high-r1,
// F2). "Known" is matched too, for uniformity: it is the fail-closed spelling and its consumers
// belong in the same allowlist, reviewed by the same rule.
var statusVocabularySelectors = map[string]bool{
	"Status":   true,
	"Known":    true,
	"Rejected": true,
}

// statusHit records one ast.SelectorExpr reading the resolve-status vocabulary (a Sel named by
// statusVocabularySelectors), tagged with the name of the function it was found inside -- the empty
// string when the selector sits at package scope, outside any function body entirely (a var/const
// initializer), which can never be allowlisted.
type statusHit struct {
	fn string
}

// allowedStatusConsumer names one (file, function) pair verified fail-closed today: the four
// readable statuses are handled explicitly and everything else -- the zero value included -- is
// routed through a default/else arm rather than silently passed or dropped.
type allowedStatusConsumer struct {
	file string
	fn   string
}

// allowedStatusConsumers is the tripwire's own allowlist. Adding a new .Status consumer means
// making it fail closed (see this file's own doc comment) and then adding its (file, function) pair
// here -- the test is what forces that review, not a style preference.
var allowedStatusConsumers = []allowedStatusConsumer{
	{"resolve.go", "statusFindings"},
	{"resolve.go", "unreadableStatusDetail"},
	{"create.go", "createFindings"},
	{"donecheck.go", "doneCheckVerdicts"},
	{"handle.go", "renameDeclSource"},
	{"containment.go", "resolveContainment"},
}

// statusHitsIn returns every statusHit astFile's declarations contain: for each top-level
// *ast.FuncDecl, every "Status" selector inside its body, tagged with the function's own name; for
// every other top-level declaration (a package-level var or const), every "Status" selector inside
// it, tagged with the empty string since it sits outside any function.
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

// TestStatusEnforcement_NoOutOfAllowlistConsumer parses every production .go file directly under
// internal/planglyph (a _test.go file is skipped: the invariant is about production reads, and a
// test's own assertions against a synthetic Status are not a second consumer) and fails when any
// "Status" selector sits outside a function named in allowedStatusConsumers. It spawns no process
// and carries no build tag, resolving the package directory from runtime.Caller(0) exactly as
// internal/cliwire/bannedecl_enforcement_test.go does.
func TestStatusEnforcement_NoOutOfAllowlistConsumer(t *testing.T) {
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("could not determine planglyph source directory location")
	}
	dir := filepath.Dir(thisFile)

	allowed := make(map[[2]string]bool, len(allowedStatusConsumers))
	for _, a := range allowedStatusConsumers {
		allowed[[2]string{a.file, a.fn}] = true
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read directory %s: %v", dir, err)
	}

	var failures []string
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}

		fset := token.NewFileSet()
		astFile, err := parser.ParseFile(fset, filepath.Join(dir, name), nil, parser.ParseComments)
		if err != nil {
			t.Fatalf("parse %s: %v", name, err)
		}

		for _, hit := range statusHitsIn(astFile) {
			if allowed[[2]string{name, hit.fn}] {
				continue
			}
			label := hit.fn
			if label == "" {
				label = "<package scope>"
			}
			failures = append(failures, name+": "+label)
		}
	}

	if len(failures) > 0 {
		t.Errorf(".Status tripwire fired for consumer(s) not in the allowlist: %v -- a new .Status "+
			"consumer must handle the vocabulary fail-closed (quarry.Status.Known(), or a boolean "+
			"derived only after a vocabulary guard) and then be added to allowedStatusConsumers in "+
			"status_enforcement_test.go; see that file's own doc comment",
			failures)
	}
}

// TestStatusHitsIn_CatchesOutOfAllowlistConsumer is the tripwire's own seeded self-test: a synthetic
// source string reading .Status from a function not in allowedStatusConsumers must be caught by
// statusHitsIn.
func TestStatusHitsIn_CatchesOutOfAllowlistConsumer(t *testing.T) {
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

// TestStatusHitsIn_CatchesRejectedConsumer is the Rejected-spelling self-test, and the one that
// pins F2's fix rather than merely restating the original scan.
//
// The fixture is the exact fail-open shape: a consumer that treats "not a pre-resolution rejection"
// as "resolved". That is wrong for an out-of-vocabulary non-empty status -- Rejected reports false
// for it -- and it names no "Status" selector anywhere, so the pre-fix matcher returned zero hits
// and the consumer shipped unreviewed.
func TestStatusHitsIn_CatchesRejectedConsumer(t *testing.T) {
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

// TestStatusHitsIn_CatchesKnownConsumer pins the Known spelling for the same reason, so the
// allowlist obligation cannot be dodged by reading the vocabulary through the predicate rather than
// the field.
func TestStatusHitsIn_CatchesKnownConsumer(t *testing.T) {
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

// TestStatusHitsIn_UnrelatedSelectorNotCaught confirms the widened match still discriminates on the
// selector's own name -- a field read that merely happens to share a receiver with a Status read
// must not false-positive.
func TestStatusHitsIn_UnrelatedSelectorNotCaught(t *testing.T) {
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
