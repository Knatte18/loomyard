// tiersleep_test.go extends the Test Tier Purity Invariant guard (tierpurity_test.go) with an
// AST-based check: an untagged test file containing a literal time.Sleep(...)
// call whose duration is a compile-time constant of at least one second is flagged the same way a
// banned spawn token is, unless the file is on the allowedLongSleepers allowlist.
// A real-time-blocking sleep in a file that runs on every plain `go test` slows the offline Tier 1
// loop exactly like the spawns tierpurity_test.go already guards against;
// this file adds the detection logic, tierpurity_test.go wires it in.

package main

import (
	"go/ast"
	"go/parser"
	"go/token"
	"strconv"
	"testing"
)

// allowedLongSleepers is the real-time-wait guard's allowlist: module-relative,
// slash-separated file paths permitted to contain a literal time.Sleep(...) of at
// least one second in an untagged test file, each with a one-line reason — mirroring
// tierpurity_test.go's allowedSpawners style.
var allowedLongSleepers = map[string]string{
	"internal/reedengine/testmain_test.go": "untagged header-pane-keepalive TestMain stand-in whose body loops calling " +
		"time.Sleep(time.Hour) at line 31 — intentional and safe because TestMain itself " +
		"intercepts this exact invocation shape (os.Args[1] == \"reed\") as a deliberate " +
		"stand-in for the real `lyx reed header --blocking` keepalive, not an accidental " +
		"recursive re-exec; it is not reachable during a normal `go test` run (no test " +
		"invokes the binary with a leading \"reed\" argument)",
	"internal/reedcli/testmain_test.go": "same shape and the same TestMain-intercept rationale at its own matching " +
		"loop on line 35 — this file's TestMain also calls lyxtest.HermeticGitEnv() later in " +
		"the same function, but only on the code path reached when the \"reed\" branch above " +
		"is NOT taken; the sleep loop itself is an infinite for loop that never returns, so " +
		"HermeticGitEnv() never executes on the sleep-loop path and plays no part in why that " +
		"path is safe",
}

// findLongLiteralSleep parses data as Go and finds time.Sleep(...) calls with duration >= 1 second.
func findLongLiteralSleep(fset *token.FileSet, filename string, data []byte) (evidence string, found bool) {
	file, err := parser.ParseFile(fset, filename, data, 0)
	if err != nil {
		return "", false
	}

	ast.Inspect(file, func(n ast.Node) bool {
		if found {
			// Already found a match; stop descending further, but let ast.Inspect
			// finish unwinding rather than trying to abort the walk early.
			return false
		}
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		pkgIdent, ok := sel.X.(*ast.Ident)
		if !ok || pkgIdent.Name != "time" || sel.Sel.Name != "Sleep" {
			return true
		}
		if len(call.Args) != 1 {
			return true
		}

		isLong, unresolvable := sleepDurationAtLeastOneSecond(file, call.Args[0])
		if isLong {
			evidence = fset.Position(call.Pos()).String()
			found = true
			return false
		}
		if unresolvable {
			evidence = fset.Position(call.Pos()).String() + " (duration argument could not be resolved to a known constant)"
			found = true
			return false
		}
		return true
	})

	return evidence, found
}

// sleepDurationAtLeastOneSecond reports whether arg is a duration of at least one second or unresolvable.
func sleepDurationAtLeastOneSecond(file *ast.File, arg ast.Expr) (isLong bool, unresolvable bool) {
	switch e := arg.(type) {
	case *ast.SelectorExpr:
		// Case (a): a bare time.<Scale> selector, e.g. time.Sleep(time.Hour).
		pkgIdent, ok := e.X.(*ast.Ident)
		if !ok || pkgIdent.Name != "time" {
			return false, true
		}
		return timeScaleAtLeastOneSecond(e.Sel.Name)

	case *ast.BinaryExpr:
		// Case (b): a multiplication of literal and time.* scale, e.g. 2 * time.Second.
		if e.Op != token.MUL {
			return false, true
		}
		litExpr, scaleExpr, ok := splitLiteralAndScale(e.X, e.Y)
		if !ok {
			return false, true
		}
		// Recurse into case (a)'s scale-selector check purely to confirm the
		// non-literal operand resolves to a known time.* scale; its isLong
		// answer describes the bare selector alone, not the multiplied result,
		// so the actual comparison below recomputes from scaleInNanoseconds.
		if _, scaleUnresolvable := sleepDurationAtLeastOneSecond(file, scaleExpr); scaleUnresolvable {
			return false, true
		}
		litValue, parseErr := strconv.ParseFloat(litExpr.Value, 64)
		if parseErr != nil {
			return false, true
		}
		scaleNanos := scaleInNanoseconds(scaleExpr.(*ast.SelectorExpr).Sel.Name)
		return litValue*scaleNanos >= 1e9, false

	case *ast.Ident:
		// Case (c): a bare identifier — look for a same-file top-level const/var declaration.
		valueExpr, ok := findTopLevelDeclValue(file, e.Name)
		if !ok {
			return false, true
		}
		return sleepDurationAtLeastOneSecond(file, valueExpr)

	default:
		return false, true
	}
}

// timeScaleAtLeastOneSecond reports whether a time.<name> scale is at least one second.
func timeScaleAtLeastOneSecond(name string) (isLong bool, unresolvable bool) {
	switch name {
	case "Second", "Minute", "Hour":
		return true, false
	case "Millisecond", "Microsecond", "Nanosecond":
		return false, false
	default:
		return false, true
	}
}

// scaleInNanoseconds returns the nanosecond value of a recognized time.<name> scale
// selector. Callers must only pass a name already confirmed resolvable by
// timeScaleAtLeastOneSecond (via sleepDurationAtLeastOneSecond's case (a) recursion);
// an unrecognized name returns 0, matching the "not long" conservative default there.
func scaleInNanoseconds(name string) float64 {
	switch name {
	case "Nanosecond":
		return 1
	case "Microsecond":
		return 1e3
	case "Millisecond":
		return 1e6
	case "Second":
		return 1e9
	case "Minute":
		return 60 * 1e9
	case "Hour":
		return 3600 * 1e9
	default:
		return 0
	}
}

// splitLiteralAndScale identifies numeric literal and time.<Scale> in a BinaryExpr, regardless of order.
func splitLiteralAndScale(x, y ast.Expr) (lit *ast.BasicLit, scale ast.Expr, ok bool) {
	if l, isLit := x.(*ast.BasicLit); isLit && (l.Kind == token.INT || l.Kind == token.FLOAT) {
		return l, y, true
	}
	if l, isLit := y.(*ast.BasicLit); isLit && (l.Kind == token.INT || l.Kind == token.FLOAT) {
		return l, x, true
	}
	return nil, nil, false
}

// findTopLevelDeclValue searches file for a const/var with the given name and returns its value.
func findTopLevelDeclValue(file *ast.File, name string) (ast.Expr, bool) {
	for _, decl := range file.Decls {
		genDecl, ok := decl.(*ast.GenDecl)
		if !ok || (genDecl.Tok != token.CONST && genDecl.Tok != token.VAR) {
			continue
		}
		for _, spec := range genDecl.Specs {
			valueSpec, ok := spec.(*ast.ValueSpec)
			if !ok {
				continue
			}
			for i, ident := range valueSpec.Names {
				if ident.Name != name {
					continue
				}
				if i >= len(valueSpec.Values) {
					return nil, false
				}
				return valueSpec.Values[i], true
			}
		}
	}
	return nil, false
}

// TestFindLongLiteralSleep_DetectsAllArgumentForms verifies detection against crafted code
// snippets.
func TestFindLongLiteralSleep_DetectsAllArgumentForms(t *testing.T) {
	tests := []struct {
		name string
		src  string
		want bool
	}{
		{
			name: "bare_scale_at_least_one_second",
			src:  "package p\nimport \"time\"\nfunc f() { time.Sleep(time.Hour) }\n",
			want: true,
		},
		{
			name: "bare_scale_under_one_second",
			src:  "package p\nimport \"time\"\nfunc f() { time.Sleep(time.Millisecond) }\n",
			want: false,
		},
		{
			name: "multiplication_at_least_one_second",
			src:  "package p\nimport \"time\"\nfunc f() { time.Sleep(2 * time.Second) }\n",
			want: true,
		},
		{
			name: "multiplication_under_one_second",
			src:  "package p\nimport \"time\"\nfunc f() { time.Sleep(500 * time.Millisecond) }\n",
			want: false,
		},
		{
			name: "named_const_resolved_by_identifier",
			src:  "package p\nimport \"time\"\nconst waitLong = time.Hour\nfunc f() { time.Sleep(waitLong) }\n",
			want: true,
		},
		{
			name: "identifier_with_no_resolvable_declaration",
			src:  "package p\nimport \"time\"\nfunc f() { time.Sleep(undeclaredDuration) }\n",
			want: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fset := token.NewFileSet()
			_, found := findLongLiteralSleep(fset, tt.name+".go", []byte(tt.src))
			if found != tt.want {
				t.Errorf("findLongLiteralSleep(%s) found = %v; want %v", tt.name, found, tt.want)
			}
		})
	}
}
