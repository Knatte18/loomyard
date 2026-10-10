// verifymodule.go holds verify-nested-module, the plan check that refuses an author-written gate command which tests a nested module's package from outside that module, and verify-module-wide, the check that refuses a card Verify no fork may run.
// The go tool limits a package pattern to the module it runs in, so such a command cannot pass by construction.

package planparser

import (
	"fmt"
	"path"
	"slices"
	"strings"
)

// goValueFlags are the go build and go test flags that take their value as the next field when spelled without "=".
var goValueFlags = map[string]bool{
	"-o": true, "-run": true, "-tags": true, "-count": true, "-timeout": true,
	"-p": true, "-parallel": true, "-cpu": true, "-bench": true, "-benchtime": true,
	"-covermode": true, "-coverpkg": true, "-coverprofile": true, "-cpuprofile": true,
	"-memprofile": true, "-blockprofile": true, "-mutexprofile": true, "-trace": true,
	"-outputdir": true, "-vet": true, "-ldflags": true, "-gcflags": true, "-asmflags": true,
	"-gccgoflags": true, "-mod": true, "-modfile": true, "-overlay": true, "-pkgdir": true,
	"-toolexec": true, "-exec": true, "-fuzz": true, "-fuzztime": true,
	"-fuzzminimizetime": true, "-skip": true, "-shuffle": true, "-list": true,
}

// crossModuleCommand is one go command that names a package in a nested module other than the module it runs in.
type crossModuleCommand struct {
	// Command is the command's text.
	Command string

	// Module is the worktree-relative directory of the nested module the package argument lies in.
	Module string
}

// checkVerifyNestedModule implements verify-nested-module: a `go` command in a card's Verify value, or in the overview's verify chain, must not name a package of a nested module other than the one it runs in.
// A card's value is judged against the nested modules as the plan stands once that card has run, and the overview's against the whole plan.
func checkVerifyNestedModule(plan *Plan, worktreeRoot string) []ValidationError {
	var findings []ValidationError

	for _, c := range plan.Cards {
		if c.Verify == "" {
			continue
		}
		modules := NestedModules(plan, c.Number, worktreeRoot)
		for _, cmd := range crossModuleCommands(c.Verify, modules) {
			findings = append(findings, ValidationError{
				Check: "verify-nested-module",
				Card:  cardID(c),
				Detail: fmt.Sprintf(
					"card %d Verify: `%s` names a package of the nested module %s from outside it; run it as `go -C %s` with module-relative package paths",
					c.Number, cmd.Command, cmd.Module, cmd.Module,
				),
			})
		}
	}

	if plan.Verify != "" {
		highest := 0
		for _, c := range plan.Cards {
			highest = max(highest, c.Number)
		}
		modules := NestedModules(plan, highest, worktreeRoot)
		for _, cmd := range crossModuleCommands(plan.Verify, modules) {
			findings = append(findings, ValidationError{
				Check: "verify-nested-module",
				Detail: fmt.Sprintf(
					"the overview's `## verify:` section: `%s` names a package of the nested module %s from outside it; run it as `go -C %s` with module-relative package paths",
					cmd.Command, cmd.Module, cmd.Module,
				),
			})
		}
	}

	return findings
}

// checkVerifyModuleWide implements verify-module-wide: a `go test`, `go build` or `go vet` in a card's Verify value must not name a module-wide package pattern (one holding `...`, or `all`), and a `go test` must not select the `tmux` or `llm` tier.
// An agent's settings deny such a command, so it can never run under a fork; the way forward is `lyx gate test`, run as a background Bash call.
// The overview's verify chain is not read, since Go runs it itself.
// Like the deny, it matches static shapes only: a command built from a variable or behind `bash -c` passes.
func checkVerifyModuleWide(plan *Plan) []ValidationError {
	var findings []ValidationError

	for _, c := range plan.Cards {
		for _, cmd := range moduleWideCommands(c.Verify) {
			findings = append(findings, ValidationError{
				Check: "verify-module-wide",
				Card:  cardID(c),
				Detail: fmt.Sprintf(
					"card %d Verify: `%s` is module-wide or runs the tmux or llm tier, which no fork may run raw; use `lyx gate test [-C <module>] [--tags <tags>] <packages>` over the card's own packages, run as a background Bash call",
					c.Number, cmd,
				),
			})
		}
	}

	return findings
}

// moduleWideCommands scans one shell chain and returns the text of each go command checkVerifyModuleWide refuses.
func moduleWideCommands(chain string) []string {
	var found []string

	separated := strings.NewReplacer(";", "&&", "\n", "&&").Replace(chain)
	for _, segment := range strings.Split(separated, "&&") {
		fields := strings.Fields(segment)
		for len(fields) > 0 && isEnvAssignment(fields[0]) {
			fields = fields[1:]
		}
		if len(fields) == 0 || fields[0] != "go" {
			continue
		}
		if isModuleWideGoCommand(fields[1:]) {
			found = append(found, strings.Join(fields, " "))
		}
	}
	return found
}

// isModuleWideGoCommand reads the fields after `go` and reports whether the command is a build, test or vet over a module-wide pattern, or a test selecting the tmux or llm tier.
func isModuleWideGoCommand(fields []string) bool {
	i := 0
	for i < len(fields) && strings.HasPrefix(fields[i], "-") {
		if fields[i] == "-C" {
			i++
		}
		i++
	}
	if i >= len(fields) {
		return false
	}
	subcommand := fields[i]
	if subcommand != "build" && subcommand != "test" && subcommand != "vet" {
		return false
	}

	rest := fields[i+1:]
	for j := 0; j < len(rest); j++ {
		field := rest[j]
		switch {
		case field == "--" || field == "-args":
			return false
		case strings.HasPrefix(field, "-"):
			name, value, hasValue := strings.Cut(field, "=")
			if !hasValue && goValueFlags[field] && j+1 < len(rest) {
				j++
				value = rest[j]
			}
			if subcommand == "test" && (name == "-tags" || name == "--tags") && namesGateOnlyTier(value) {
				return true
			}
		case field == "all" || strings.Contains(field, "..."):
			return true
		}
	}
	return false
}

// namesGateOnlyTier reports whether a -tags value lists the tmux or llm build tag.
func namesGateOnlyTier(tags string) bool {
	return slices.ContainsFunc(strings.FieldsFunc(strings.Trim(tags, `"'`), func(r rune) bool { return r == ',' || r == ' ' }), func(tag string) bool {
		return tag == "tmux" || tag == "llm"
	})
}

// crossModuleCommands scans one shell chain and returns each go command whose relative package argument lies in a nested module of modules other than the module the command runs in.
// The run directory starts at the worktree root and follows a literal `cd <dir>`; a go command's `-C <dir>` sets its own directory.
// A `cd` or `-C` to an absolute path, an unresolvable spelling or a directory outside the root ends the scan, since only what can be placed is refused.
func crossModuleCommands(chain string, modules []string) []crossModuleCommand {
	var found []crossModuleCommand
	runDir := "."

	separated := strings.NewReplacer(";", "&&", "\n", "&&").Replace(chain)
	for _, segment := range strings.Split(separated, "&&") {
		fields := strings.Fields(segment)
		for len(fields) > 0 && isEnvAssignment(fields[0]) {
			fields = fields[1:]
		}
		if len(fields) == 0 {
			continue
		}
		switch fields[0] {
		case "cd":
			if len(fields) != 2 {
				return found
			}
			next, ok := resolveRunDir(runDir, fields[1])
			if !ok {
				return found
			}
			runDir = next
		case "go":
			commandDir, packageArgs, ok := goCommandDirAndPackages(runDir, fields[1:])
			if !ok {
				return found
			}
			commandModule := ModuleOf(modules, commandDir)
			for _, arg := range packageArgs {
				module := ModuleOf(modules, path.Join(commandDir, strings.TrimSuffix(arg, "/...")))
				if module != "." && module != commandModule {
					found = append(found, crossModuleCommand{Command: strings.Join(fields, " "), Module: module})
					break
				}
			}
		}
	}
	return found
}

// goCommandDirAndPackages reads the fields after `go`: the directory the command runs in, and its relative package arguments.
// ok is false when a -C directory cannot be placed.
func goCommandDirAndPackages(runDir string, fields []string) (commandDir string, packageArgs []string, ok bool) {
	commandDir = runDir
	i := 0
	for i < len(fields) && strings.HasPrefix(fields[i], "-") {
		flag := fields[i]
		var target string
		switch {
		case flag == "-C" && i+1 < len(fields):
			target = fields[i+1]
			i += 2
		case strings.HasPrefix(flag, "-C="):
			target = strings.TrimPrefix(flag, "-C=")
			i++
		default:
			return runDir, nil, false
		}
		commandDir, ok = resolveRunDir(commandDir, target)
		if !ok {
			return "", nil, false
		}
	}
	if i >= len(fields) {
		return commandDir, nil, true
	}

	// fields[i] is the subcommand.
	rest := fields[i+1:]
	for j := 0; j < len(rest); j++ {
		field := rest[j]
		switch {
		case field == "--" || field == "-args":
			return commandDir, packageArgs, true
		case strings.HasPrefix(field, "-"):
			if !strings.Contains(field, "=") && goValueFlags[field] {
				j++
			}
		case field == "." || field == ".." || strings.HasPrefix(field, "./") || strings.HasPrefix(field, "../"):
			packageArgs = append(packageArgs, field)
		}
	}
	return commandDir, packageArgs, true
}

// resolveRunDir joins target onto dir, both worktree-relative and slash-separated.
// ok is false for an absolute path, a spelling the shell would expand, or a result outside the worktree root.
func resolveRunDir(dir, target string) (string, bool) {
	target = strings.Trim(target, `"'`)
	if target == "" || target == "-" || path.IsAbs(target) || strings.ContainsAny(target, "$`~*\\") {
		return "", false
	}
	joined := path.Join(dir, target)
	if joined == ".." || strings.HasPrefix(joined, "../") {
		return "", false
	}
	return joined, true
}

// isEnvAssignment reports whether field is a leading NAME=value environment assignment.
func isEnvAssignment(field string) bool {
	name, _, found := strings.Cut(field, "=")
	if !found || name == "" {
		return false
	}
	return !slices.ContainsFunc([]rune(name), func(r rune) bool {
		return !(r == '_' || r >= 'A' && r <= 'Z' || r >= 'a' && r <= 'z' || r >= '0' && r <= '9')
	})
}
