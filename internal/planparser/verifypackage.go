// verifypackage.go holds verify-package-missing, the plan check that refuses an author-written Verify or `## verify:` command whose relative package argument names no directory.
// It reads the same shell-chain shapes as verifymodule.go's checks, for `go test`, `go build`, `go vet` and `lyx gate test`.

package planparser

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"
)

// missingPackageArg is one relative package argument of a gate command that names no directory.
type missingPackageArg struct {
	// Command is the command's text.
	Command string

	// Arg is the package argument as written.
	Arg string

	// Hint is the argument with its external-test `_test` suffix removed when that spelling names a directory, and empty otherwise.
	Hint string
}

// checkVerifyPackageMissing implements verify-package-missing: a relative package argument of a `go test`, `go build`, `go vet` or `lyx gate test` command in a card's Verify value, or in the overview's verify chain, must name a directory.
// A card's value is judged against the directories on disk plus those the plan creates through that card, and the overview's against the whole plan's.
// A card's Verify is skipped when that card or an earlier one holds a Move group, and the overview chain when any card does, since a Move's destination lives in Intent prose.
// It reads static spellings only: an argument behind a variable or inside `bash -c` passes.
func checkVerifyPackageMissing(plan *Plan, worktreeRoot string) []ValidationError {
	var findings []ValidationError

	highest := 0
	anyMove := false
	moveSeen := false
	for _, c := range plan.Cards {
		highest = max(highest, c.Number)
		anyMove = anyMove || holdsMoveGroup(c)
	}

	for _, c := range plan.Cards {
		moveSeen = moveSeen || holdsMoveGroup(c)
		if c.Verify == "" || moveSeen {
			continue
		}
		for _, missing := range missingPackageArgs(c.Verify, worktreeRoot, createdDirs(plan, c.Number)) {
			findings = append(findings, ValidationError{
				Check: "verify-package-missing",
				Card:  cardID(c),
				Ref:   missing.Arg,
				Detail: fmt.Sprintf(
					"card %d Verify: `%s` names %s, which is no directory on disk and none that card %d or an earlier card creates; correct the Verify line%s",
					c.Number, missing.Command, missing.Arg, c.Number, missingPackageHint(missing),
				),
			})
		}
	}

	if plan.Verify != "" && !anyMove {
		for _, missing := range missingPackageArgs(plan.Verify, worktreeRoot, createdDirs(plan, highest)) {
			findings = append(findings, ValidationError{
				Check: "verify-package-missing",
				Ref:   missing.Arg,
				Detail: fmt.Sprintf(
					"the overview's `## verify:` section: `%s` names %s, which is no directory on disk and none the plan creates; correct the Verify line%s",
					missing.Command, missing.Arg, missingPackageHint(missing),
				),
			})
		}
	}

	return findings
}

// missingPackageHint returns the detail's external-test clause for missing, or "" when it has no hint.
func missingPackageHint(missing missingPackageArg) string {
	if missing.Hint == "" {
		return ""
	}
	return fmt.Sprintf("; %s is an external test package whose tests live in %s: pass %s", strings.TrimSuffix(missing.Arg, "/..."), strings.TrimSuffix(missing.Hint, "/..."), missing.Hint)
}

// holdsMoveGroup reports whether card has a Move target group.
func holdsMoveGroup(card Card) bool {
	for _, group := range card.TargetGroups {
		if group.Type == CardTypeMove {
			return true
		}
	}
	return false
}

// createdDirs returns the directory of every CardTargetDirs entry that is not delete-only over the cards numbered at or below through.
func createdDirs(plan *Plan, through int) []string {
	var dirs []string
	for _, card := range plan.Cards {
		if card.Number > through {
			continue
		}
		for _, entry := range CardTargetDirs(plan, card) {
			if !entry.DeleteOnly {
				dirs = append(dirs, entry.Dir)
			}
		}
	}
	return dirs
}

// missingPackageArgs scans one shell chain exactly as crossModuleCommands does and returns each relative package argument naming no directory.
// A directory counts when it is on disk under worktreeRoot, or equals or is an ancestor of an entry of created.
// The scan ends at a `cd` or a `-C` or `--dir` that cannot be placed, and at a `cd` in a pipeline stage or after `||`.
func missingPackageArgs(chain, worktreeRoot string, created []string) []missingPackageArg {
	var found []missingPackageArg
	runDir := "."

	exists := func(dir string) bool {
		if dir == ".." || strings.HasPrefix(dir, "../") {
			return false
		}
		for _, c := range created {
			if dir == c || strings.HasPrefix(c, dir+"/") {
				return true
			}
		}
		info, err := os.Stat(filepath.Join(worktreeRoot, filepath.FromSlash(dir)))
		return err == nil && info.IsDir()
	}

	for _, segment := range splitChain(chain) {
		fields := strings.Fields(segment.Text)
		for len(fields) > 0 && isEnvAssignment(fields[0]) {
			fields = fields[1:]
		}
		if len(fields) == 0 {
			continue
		}

		var commandDir string
		var packageArgs []string
		switch {
		case fields[0] == "cd":
			// A cd in a pipeline stage or after `||` may not have run, so the directory is unknown from here on.
			if len(fields) != 2 || segment.Separator == "|" || segment.Separator == "||" {
				return found
			}
			next, ok := resolveRunDir(runDir, fields[1])
			if !ok {
				return found
			}
			runDir = next
			continue
		case fields[0] == "go":
			var subcommand string
			var ok bool
			commandDir, subcommand, packageArgs, ok = goCommandDirAndPackages(runDir, fields[1:])
			if !ok {
				return found
			}
			if subcommand != "test" && subcommand != "build" && subcommand != "vet" {
				continue
			}
		case len(fields) >= 3 && fields[0] == "lyx" && fields[1] == "gate" && fields[2] == "test":
			var ok bool
			commandDir, packageArgs, ok = gateTestDirAndPackages(runDir, fields[3:])
			if !ok {
				return found
			}
		default:
			continue
		}

		for _, arg := range packageArgs {
			if strings.HasSuffix(arg, ".go") {
				continue
			}
			trimmed := strings.TrimSuffix(arg, "/...")
			if exists(path.Join(commandDir, trimmed)) {
				continue
			}
			missing := missingPackageArg{Command: strings.Join(fields, " "), Arg: arg}
			if stem, isTest := strings.CutSuffix(trimmed, "_test"); isTest && exists(path.Join(commandDir, stem)) {
				missing.Hint = stem + strings.TrimPrefix(arg, trimmed)
			}
			found = append(found, missing)
		}
	}
	return found
}

// gateTestDirAndPackages reads the fields after `lyx gate test`: the directory the command runs in and its relative package arguments.
// `-C` and `--dir`, spelled with a space or `=`, set the directory, `--tags` skips its value, and reading stops at `--`.
// ok is false when a directory cannot be placed.
func gateTestDirAndPackages(runDir string, fields []string) (commandDir string, packageArgs []string, ok bool) {
	commandDir = runDir
	for i := 0; i < len(fields); i++ {
		field := fields[i]
		var target string
		hasDir := false
		switch {
		case field == "--":
			return commandDir, packageArgs, true
		case (field == "-C" || field == "--dir") && i+1 < len(fields):
			i++
			target, hasDir = fields[i], true
		case strings.HasPrefix(field, "-C="):
			target, hasDir = strings.TrimPrefix(field, "-C="), true
		case strings.HasPrefix(field, "--dir="):
			target, hasDir = strings.TrimPrefix(field, "--dir="), true
		case field == "--tags":
			i++
		case isRelativePackageSpelling(field):
			packageArgs = append(packageArgs, field)
		}
		if hasDir {
			commandDir, ok = resolveRunDir(commandDir, target)
			if !ok {
				return "", nil, false
			}
		}
	}
	return commandDir, packageArgs, true
}
