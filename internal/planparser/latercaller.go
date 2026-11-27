// latercaller.go implements the create-caller-later check.
// It finds a new production function that only a later card can give a caller.
// The check reads plan text alone, so it runs at every gate that runs ValidateFormat.

package planparser

import (
	"fmt"
	"path"
	"slices"
	"strings"
)

// goEntryFunctions are the function names a Go program enters through, which no caller needs.
var goEntryFunctions = []string{"main", "init"}

// TestSupportDirectories are the package directories whose functions the reachability guard counts as reached.
// A function created in one needs no caller, and can be the caller of another.
// Each matches its own directory and every directory below it.
// It is exported so that the guard's own list is held equal to it.
var TestSupportDirectories = []string{"internal/testkit", "internal/gitkit", "internal/hubforge"}

// checkCreateCallerLater implements create-caller-later under go.
// Every production function has to be reachable from a main.
// A card that creates one and changes no production code that could call it fails its own gate on that guard.
// The finding fires when a later card names the function and can call it.
// That card can write the call in production code it changes or in a function it creates.
// That later card is where the first caller goes.
// A function the creating card itself can call stays silent, and so does one no later card names.
// One finding per function, attributed to the creating card, naming the first such later card.
func checkCreateCallerLater(plan *Plan) []ValidationError {
	var findings []ValidationError
	if !plan.GoToolchain() {
		return findings
	}

	for _, creator := range plan.Cards {
		for _, declaration := range creator.Declarations {
			if !isProductionFunctionDeclaration(declaration) || cardCanHoldProductionCaller(plan, creator, declaration.Handle, false) {
				continue
			}
			for _, caller := range plan.Cards {
				if caller.Number <= creator.Number || !cardNamesRef(caller, declaration.Handle) {
					continue
				}
				if !cardCanHoldProductionCaller(plan, caller, declaration.Handle, true) {
					continue
				}
				findings = append(findings, ValidationError{
					Check: "create-caller-later",
					Card:  cardID(creator),
					Ref:   declaration.Handle,
					Detail: fmt.Sprintf(
						"card %d creates %q, but no target of card %d can call it from production code, and card %d names it; "+
							"a function no production code calls fails card %d's gate on the guard that every production function is reachable from a main, "+
							"so move the Create to card %d, the card that adds its first production caller, or list that caller on card %d",
						creator.Number, declaration.Handle, creator.Number, caller.Number,
						creator.Number, caller.Number, creator.Number,
					),
				})
				break
			}
		}
	}

	return findings
}

// isProductionFunctionDeclaration reports whether declaration creates a function or method of production code.
// A declaration in an external test package or a test-support directory, one named as a test, one taking a testing type,
// and a program entry point are not.
func isProductionFunctionDeclaration(declaration CardDeclaration) bool {
	if declarationKeyword(declaration.Decl) != "func" {
		return false
	}
	unit, ok := handleUnit(declaration.Handle)
	if !ok || strings.HasSuffix(unit, "_test") || isTestSupportUnit(unit) {
		return false
	}
	identifier, ok := HandleIdentifier(declaration.Handle)
	if !ok || hasTestPrefix(identifier) || slices.Contains(goEntryFunctions, identifier) {
		return false
	}
	return !strings.Contains(declaration.Decl, "testing.")
}

// isTestSupportUnit reports whether unit is a test-support directory or lies below one.
func isTestSupportUnit(unit string) bool {
	return slices.ContainsFunc(TestSupportDirectories, func(directory string) bool {
		return unit == directory || strings.HasPrefix(unit, directory+"/")
	})
}

// cardCanHoldProductionCaller reports whether card can write a call to callee into production Go code.
// A ref to callee itself does not count.
// Only the Create, Edit, Extends and Custom groups add code; a Delete, Rename, Move or Prosa group adds none.
// A ref whose shape says nothing certain, such as a package self glyph or a bare symbol, counts as able to hold one.
// A function card creates in a test-support directory counts, since the guard reaches it without a caller.
// With ownFunctionsCall, any other function card creates counts too.
// The exceptions are a function whose head names a testing type and one that is a test by name or package.
// Set it to ask whether card can write the call, and leave it unset to ask whether card satisfies the guard alone.
// A new function outside test support is reached only through a caller of its own.
func cardCanHoldProductionCaller(plan *Plan, card Card, callee string, ownFunctionsCall bool) bool {
	declared := make(map[string]string, len(card.Declarations))
	for _, declaration := range card.Declarations {
		unit, _ := handleUnit(declaration.Handle)
		counts := isTestSupportUnit(unit) || ownFunctionsCall && !strings.Contains(declaration.Decl, "testing.")
		if declarationKeyword(declaration.Decl) == "func" && counts {
			continue
		}
		declared[declaration.Handle] = declaration.Decl
	}
	for _, group := range card.TargetGroups {
		switch group.Type {
		case CardTypeCreate, CardTypeEdit, CardTypeExtends, CardTypeCustom:
		default:
			continue
		}
		for _, ref := range group.Refs {
			if ref != callee && refCanHoldProductionCaller(plan, ref, declared) {
				return true
			}
		}
	}
	return false
}

// refCanHoldProductionCaller reports whether ref names production Go code that a call could be written into.
// declared maps handles the ref's card creates to their declaration heads.
// A handle declared there as a function, type or constant is not such code.
func refCanHoldProductionCaller(plan *Plan, ref string, declared map[string]string) bool {
	if IsHandleRef(ref) {
		if isTestHandle(ref) {
			return false
		}
		decl, createdHere := declared[ref]
		if !createdHere {
			return true
		}
		switch declarationKeyword(decl) {
		case "func", "type", "const":
			return false
		}
		return true
	}
	if diskPath, ok := diskPathForRef(plan, ref); ok {
		switch path.Ext(diskPath) {
		case "":
			return !strings.HasSuffix(diskPath, "_test")
		case ".go":
			return !strings.HasSuffix(diskPath, "_test.go")
		}
		return false
	}
	parsed, err := ParseRef(plan, ref)
	if err != nil {
		return true
	}
	return !strings.HasSuffix(parsed.Unit, "_test") && !strings.HasSuffix(parsed.Unit, "_test.go") && !hasTestPrefix(parsed.Name)
}

// isTestHandle reports whether handle declares test code: a member of an external test package, or one named as a test.
func isTestHandle(handle string) bool {
	unit, ok := handleUnit(handle)
	if ok && strings.HasSuffix(unit, "_test") {
		return true
	}
	identifier, ok := HandleIdentifier(handle)
	return ok && hasTestPrefix(identifier)
}

// declarationKeyword returns the Go keyword a declaration head opens with, such as "func" or "type".
func declarationKeyword(decl string) string {
	head := strings.TrimSpace(decl)
	if end := strings.IndexAny(head, " (\t"); end != -1 {
		return head[:end]
	}
	return head
}

// cardNamesRef reports whether ref is one of card's targets or Uses entries.
func cardNamesRef(card Card, ref string) bool {
	return slices.Contains(card.Targets, ref) || slices.Contains(card.Uses, ref)
}
