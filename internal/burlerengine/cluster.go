// cluster.go implements the fail-loud policy Engine.Review enforces over a cluster round's
// shuttleengine.ForkAudit: the fork-count contract and the per-fork violation classes a disobedient
// fork reviewer can trigger.
// auditClusterRound is the single place that turns the raw audit facts (shuttleengine's own
// knowledge — never this package's) into burler's hard-error/warning split, per the
// fail-loud-posture Shared Decision.
// auditFixRound applies the same per-fork checks to a twill fixer's self-check forks, where a wrong fork count is a warning.

package burlerengine

import (
	"errors"
	"fmt"
	"path"
	"strings"

	"github.com/Knatte18/loomyard/internal/shuttleengine"
)

// ErrClusterForksMissing is the sentinel for when a cluster round's ForkAudit does not carry
// exactly the requested number of fork reviewers.
var ErrClusterForksMissing = errors.New("the cluster round did not produce exactly the requested number of fork reviewers")

// mutatingGitSubcommands holds the git subcommands that change repository or working-tree state.
var mutatingGitSubcommands = map[string]bool{
	"add": true, "commit": true, "push": true, "pull": true, "fetch": true, "merge": true,
	"rebase": true, "reset": true, "restore": true, "rm": true, "mv": true, "checkout": true,
	"switch": true, "stash": true, "apply": true, "cherry-pick": true, "tag": true, "branch": true,
}

// gitValuedGlobalOptions holds git's global options whose value is the following word, so that word is never read as the subcommand.
var gitValuedGlobalOptions = map[string]bool{
	"-C": true, "-c": true, "--git-dir": true, "--work-tree": true, "--namespace": true, "--config-env": true, "--attr-source": true,
}

// gitTagListOptions holds the git tag options that put it in list mode, where it only reads tags.
var gitTagListOptions = map[string]bool{
	"-l": true, "--list": true, "--points-at": true, "--contains": true, "--no-contains": true,
	"--merged": true, "--no-merged": true, "-n": true,
}

// gitTagWritingOptions holds the git tag options that create, delete or sign a tag.
var gitTagWritingOptions = map[string]bool{
	"-d": true, "--delete": true, "-a": true, "--annotate": true, "-s": true, "--sign": true,
	"-u": true, "--local-user": true, "-f": true, "--force": true, "-m": true, "--message": true,
	"-F": true, "--file": true, "-e": true, "--edit": true,
}

// gitTagValuedListOptions holds the git tag list-output options whose value may be the following word, so a value such as -version:refname is never read as an option.
var gitTagValuedListOptions = map[string]bool{"--sort": true, "--format": true}

// gitBranchReadOptions holds the git branch long options that never create, delete, rename, copy or move a branch or change its upstream or description.
// The value reports whether the option puts git branch in list mode, where every operand is a pattern rather than a branch to create.
var gitBranchReadOptions = map[string]bool{
	"--list": true, "--contains": true, "--no-contains": true, "--merged": true,
	"--no-merged": true, "--points-at": true, "--show-current": true,
	"--all": false, "--verbose": false, "--remotes": false, "--ignore-case": false, "--format": false, "--sort": false,
	"--column": false, "--no-column": false, "--color": false, "--no-color": false, "--abbrev": false,
	"--no-abbrev": false, "--omit-empty": false,
}

// gitBranchReadShortOptions holds the git branch short options that only read, with the value reporting whether the option puts git branch in list mode.
var gitBranchReadShortOptions = map[rune]bool{'l': true, 'v': false, 'a': false, 'r': false, 'i': false}

// gitBranchValuedReadOptions holds the git branch read options whose value is the following word when not given in the = form, so a value such as -committerdate is never read as an option or an operand.
var gitBranchValuedReadOptions = map[string]bool{"--format": true, "--sort": true, "--points-at": true}

// gitRepositoryOptions holds git's global options that name the repository or working tree apart from the directory git runs in.
var gitRepositoryOptions = map[string]bool{"--git-dir": true, "--work-tree": true}

// gitInitValuedOptions holds the git init options whose value is the following word, so that word is never read as the directory.
var gitInitValuedOptions = map[string]bool{
	"-b": true, "--initial-branch": true, "--template": true, "--separate-git-dir": true,
	"--object-format": true, "--ref-format": true,
}

// gitWorktreeAddValuedOptions holds the git worktree add options whose value is the following word, so that word is never read as the path.
var gitWorktreeAddValuedOptions = map[string]bool{"-b": true, "-B": true, "--reason": true}

// firstMutatingGitCommand returns the first of commands, one fork's Bash commands in transcript order, that invokes git with a state-mutating subcommand against a repository the fork did not create, and reports whether there is one.
// A command is split into segments at ;, &, | and newlines, and a segment is a git invocation when its first word is git, bare or path-prefixed.
// Only the subcommand word decides, so a mutating verb named as an argument, as in git help cherry-pick, never counts.
// The exceptions are git tag, which counts only when it does not merely list tags, and git branch, which counts only when it does not merely list branches or show the current one.
// A -h or --help among the global options or directly after the subcommand only prints usage and never counts either;
// further along it is an option value or an operand, as in git commit -m -h.
//
// A mutating invocation whose directory lies inside a repository the fork created earlier, by git init or by git worktree add from such a repository, does not count, so a fork may experiment on a throwaway repository.
// The directory is resolved lexically from each command's unknown starting directory, through the command's own cd segments and then the invocation's -C options.
// git push, an invocation with --git-dir or --work-tree and one after a bare cd or cd - always count, since their target is not the resolved directory.
// A git init whose relative path is the starting directory, an ancestor of it or leaves it through .. creates no recorded repository, since it may reinitialize the target.
// A .. that would climb above a leading ~ or $ word leaves the directory unresolvable, so it matches no recorded repository either.
func firstMutatingGitCommand(commands []string) (string, bool) {
	var repos forkRepositories
	for _, cmd := range commands {
		if repos.runsMutatingGit(cmd) {
			return cmd, true
		}
	}
	return "", false
}

// forkRepositories holds the lexically resolved directories of the repositories one fork created.
type forkRepositories []string

// runsMutatingGit reports whether cmd invokes git with a state-mutating subcommand outside repos, and records each repository cmd creates.
func (repos *forkRepositories) runsMutatingGit(cmd string) bool {
	dir, dirKnown := ".", true
	for _, words := range splitShellSegments(cmd) {
		switch {
		case len(words) == 0:
		case words[0] == "cd":
			if len(words) < 2 || words[1] == "-" {
				dirKnown = false
			} else if dirKnown || isAnchoredShellPath(words[1]) {
				dir, dirKnown = resolveShellPath(dir, words[1]), true
			}
		case isGitExecutable(words[0]):
			if repos.gitInvocationMutates(parseGitInvocation(words[1:], dir, dirKnown)) {
				return true
			}
		}
	}
	return false
}

// gitInvocationMutates reports whether invocation changes state outside repos, and records the repository invocation creates.
func (repos *forkRepositories) gitInvocationMutates(invocation gitInvocation) bool {
	if invocation.helpRequested || len(invocation.args) > 0 && isHelpFlag(invocation.args[0]) {
		return false
	}
	switch invocation.subcommand {
	case "init":
		directory, found := firstOperand(invocation.args, gitInitValuedOptions)
		if !found {
			directory = "."
		}
		repos.record(invocation, directory)
	case "worktree":
		if len(invocation.args) > 0 && invocation.args[0] == "add" && repos.contain(invocation) {
			if worktreePath, found := firstOperand(invocation.args[1:], gitWorktreeAddValuedOptions); found {
				repos.record(invocation, worktreePath)
			}
		}
	case "tag":
		if gitTagListsOnly(invocation.args) {
			return false
		}
	case "branch":
		if gitBranchReadsOnly(invocation.args) {
			return false
		}
	}
	if !mutatingGitSubcommands[invocation.subcommand] {
		return false
	}
	return invocation.subcommand == "push" || !repos.contain(invocation)
}

// record adds operand, resolved against invocation's directory, to repos.
// It records nothing when the directory is unknown or operand resolves to the starting directory, an ancestor of it or a path leaving it through ..;
// such a path may name the starting directory under its own basename.
func (repos *forkRepositories) record(invocation gitInvocation, operand string) {
	if !invocation.dirKnown {
		return
	}
	created := resolveShellPath(invocation.dir, operand)
	if created == "." || created == ".." || strings.HasPrefix(created, "../") {
		return
	}
	*repos = append(*repos, created)
}

// contain reports whether invocation's directory is known and lies inside one of repos.
func (repos forkRepositories) contain(invocation gitInvocation) bool {
	if !invocation.dirKnown {
		return false
	}
	for _, root := range repos {
		if invocation.dir == root || strings.HasPrefix(invocation.dir, root+"/") {
			return true
		}
	}
	return false
}

// gitInvocation is one git call of a shell segment, with the directory it acts on.
type gitInvocation struct {
	// subcommand is the first non-option word after git's global options, "" when there is none.
	subcommand string
	// args holds the words after subcommand.
	args []string
	// helpRequested reports whether a -h or --help stood among the global options.
	helpRequested bool
	// dir is the lexically resolved directory git acts on, meaningful only when dirKnown.
	dir string
	// dirKnown reports whether dir names the repository git acts on.
	dirKnown bool
}

// parseGitInvocation parses args, the words after git, run from dir, skipping each valued global option's value and applying each -C to the directory.
// dirKnown reports whether dir is known, and the result's directory is unknown too when --git-dir or --work-tree names the repository apart from it.
func parseGitInvocation(args []string, dir string, dirKnown bool) gitInvocation {
	invocation := gitInvocation{dir: dir, dirKnown: dirKnown}
	for i := 0; i < len(args); i++ {
		name, _, _ := strings.Cut(args[i], "=")
		switch {
		case args[i] == "-C" && i+1 < len(args):
			i++
			invocation.dir = resolveShellPath(invocation.dir, args[i])
		case gitRepositoryOptions[name]:
			invocation.dirKnown = false
			if name == args[i] {
				i++
			}
		case gitValuedGlobalOptions[args[i]]:
			i++
		case isHelpFlag(args[i]):
			invocation.helpRequested = true
			return invocation
		case strings.HasPrefix(args[i], "-"):
		default:
			invocation.subcommand, invocation.args = args[i], args[i+1:]
			return invocation
		}
	}
	return invocation
}

// firstOperand returns the first non-option word of args, skipping the value of each option valuedOptions holds, and reports whether there is one.
// After a -- it returns the next word, whatever its spelling.
func firstOperand(args []string, valuedOptions map[string]bool) (string, bool) {
	for i := 0; i < len(args); i++ {
		switch {
		case args[i] == "--":
			if i+1 < len(args) {
				return args[i+1], true
			}
			return "", false
		case valuedOptions[args[i]]:
			i++
		case strings.HasPrefix(args[i], "-"):
		default:
			return args[i], true
		}
	}
	return "", false
}

// isAnchoredShellPath reports whether p names its directory independently of the current directory: absolute, or led by a ~ or a $ variable.
func isAnchoredShellPath(p string) bool {
	return strings.HasPrefix(p, "/") || strings.HasPrefix(p, "~") || strings.HasPrefix(p, "$")
}

// resolveShellPath lexically resolves p against dir.
// An anchored p stands as written and cleaned; if a .. would climb above its leading ~ or $ word, the result is ../ followed by p, which lies inside no anchored root.
// A relative p joins dir; under a relative dir the result stays relative to the starting directory, so a result such as .. or ../x escapes it.
func resolveShellPath(dir, p string) string {
	if !isAnchoredShellPath(p) {
		if !isAnchoredShellPath(dir) {
			return path.Join(dir, p)
		}
		p = dir + "/" + p
	}
	if strings.HasPrefix(p, "/") {
		return path.Clean(p)
	}
	anchor, rest, _ := strings.Cut(p, "/")
	cleaned := path.Clean(strings.TrimLeft(rest, "/"))
	if cleaned == ".." || strings.HasPrefix(cleaned, "../") {
		return "../" + p
	}
	if cleaned == "." {
		return anchor
	}
	return anchor + "/" + cleaned
}

// gitTagListsOnly reports whether args, the words after git tag, only list tags.
// Bare git tag lists, and otherwise a list-mode option must stand before any -- with no tag-writing option beside it.
// A long option counts in its --name=value form too, and -n also with its line count attached, as in -n5.
// Any other short-option cluster, such as -ln, is not parsed and keeps the command mutating.
func gitTagListsOnly(args []string) bool {
	if len(args) == 0 {
		return true
	}
	listMode := false
	for i := 0; i < len(args); i++ {
		arg := args[i]
		name, _, _ := strings.Cut(arg, "=")
		switch {
		case arg == "--":
			return listMode
		case gitTagWritingOptions[name]:
			return false
		case gitTagListOptions[name] || isGitTagLineCount(arg):
			listMode = true
		case gitTagValuedListOptions[arg]:
			i++
		case len(arg) > 2 && arg[0] == '-' && arg[1] != '-':
			return false
		}
	}
	return listMode
}

// isGitTagLineCount reports whether word is git tag's -n option with its line count attached, as in -n5.
func isGitTagLineCount(word string) bool {
	count, found := strings.CutPrefix(word, "-n")
	if !found || count == "" {
		return false
	}
	for _, r := range count {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// gitBranchReadsOnly reports whether args, the words after git branch, only list branches or show the current one.
// Every option must be a known read-only option, a short-option cluster such as -avv counting letter by letter, and a long option counting in its --name=value form too;
// any other option keeps the command mutating.
// An operand, any word after --, makes git branch create a branch unless a list-mode option such as --list or --contains turns operands into patterns.
func gitBranchReadsOnly(args []string) bool {
	listMode, hasOperand := false, false
	for i := 0; i < len(args); i++ {
		arg := args[i]
		name, _, valueAttached := strings.Cut(arg, "=")
		switch {
		case arg == "--":
			return listMode || !hasOperand && i+1 == len(args)
		case strings.HasPrefix(arg, "--"):
			lists, known := gitBranchReadOptions[name]
			if !known {
				return false
			}
			listMode = listMode || lists
			if gitBranchValuedReadOptions[name] && !valueAttached {
				i++
			}
		case len(arg) > 1 && arg[0] == '-':
			for _, letter := range arg[1:] {
				lists, known := gitBranchReadShortOptions[letter]
				if !known {
					return false
				}
				listMode = listMode || lists
			}
		default:
			hasOperand = true
		}
	}
	return listMode || !hasOperand
}

// splitShellSegments splits cmd into segments at unquoted ;, &, | and newlines, each segment a list of words.
// A single or double quoted run is part of one word with its quotes removed.
// An unquoted < or > redirection is no word: the operator, an unquoted file-descriptor number attached before it, an & attached after it and the target word that follows are all dropped, so 2>/dev/null is never read as an operand.
// A backslash-newline pair is removed, so a continued line stays in its segment.
// A # at the start of a word comments out the rest of its line.
// Any other backslash outside single quotes escapes the next character, so an escaped quote opens no quoted run.
func splitShellSegments(cmd string) [][]string {
	var segments [][]string
	var words []string
	var word strings.Builder
	inWord := false
	wordQuoted := false
	var quote rune
	dropNextWord := false
	endWord := func() {
		if inWord {
			if !dropNextWord {
				words = append(words, word.String())
			}
			dropNextWord = false
			word.Reset()
			inWord = false
			wordQuoted = false
		}
	}
	endSegment := func() {
		endWord()
		dropNextWord = false
		segments = append(segments, words)
		words = nil
	}

	runes := []rune(cmd)
	for i := 0; i < len(runes); i++ {
		r := runes[i]
		switch {
		case quote == '"' && r == '\\' && i+1 < len(runes):
			i++
			word.WriteRune(runes[i])
		case quote != 0:
			if r == quote {
				quote = 0
			} else {
				word.WriteRune(r)
			}
		case r == '\\' && i+1 < len(runes) && runes[i+1] == '\n':
			i++
		case r == '#' && !inWord:
			for i+1 < len(runes) && runes[i+1] != '\n' {
				i++
			}
		case r == '\\' && i+1 < len(runes):
			i++
			word.WriteRune(runes[i])
			inWord = true
			wordQuoted = true
		case r == '\'' || r == '"':
			quote = r
			inWord = true
			wordQuoted = true
		case r == '<' || r == '>':
			if fd := word.String(); inWord && !wordQuoted && fd != "" && strings.Trim(fd, "0123456789") == "" {
				word.Reset()
				inWord = false
			}
			endWord()
			for i+1 < len(runes) && (runes[i+1] == '<' || runes[i+1] == '>') {
				i++
			}
			if i+1 < len(runes) && runes[i+1] == '&' {
				i++
			}
			dropNextWord = true
		case strings.ContainsRune(";&|\n", r):
			endSegment()
		case r == ' ' || r == '\t' || r == '\r':
			endWord()
		default:
			word.WriteRune(r)
			inWord = true
		}
	}
	endSegment()
	return segments
}

// isHelpFlag reports whether word asks git for usage output.
func isHelpFlag(word string) bool {
	return word == "-h" || word == "--help"
}

// isGitExecutable reports whether word names the git binary, bare or behind a directory path.
func isGitExecutable(word string) bool {
	return word == "git" || strings.HasSuffix(word, "/git") || strings.HasSuffix(word, `\git`)
}

// auditClusterRound enforces the cluster-round fail-loud policy. Checks run in
// a fixed order; the first violation is returned as a hard error.
func auditClusterRound(audit *shuttleengine.ForkAudit, wantN int) ([]string, error) {
	gotN := 0
	if audit != nil {
		gotN = len(audit.Forks)
	}
	if audit == nil || gotN != wantN {
		return nil, fmt.Errorf("burler: %w (requested %d, spawned %d)", ErrClusterForksMissing, wantN, gotN)
	}

	if violation := forkViolation(audit); violation != nil {
		return nil, fmt.Errorf("burler: %w", violation)
	}
	return unreturnedForkWarnings(audit), nil
}

// forkViolation returns the first Agent-call, write, mutating-git or named-spawn violation of audit, checked in that order, or nil when the forks are clean.
// The error carries no burler prefix, so each caller adds its own.
func forkViolation(audit *shuttleengine.ForkAudit) error {
	for _, fork := range audit.Forks {
		if fork.AgentCalls > 0 {
			return fmt.Errorf("fork %q attempted %d Agent tool call(s) — forks cannot nest and must never call the Agent tool, even when the attempt was denied", fork.TranscriptPath, fork.AgentCalls)
		}
	}
	for _, fork := range audit.Forks {
		if fork.WriteCalls > 0 {
			return fmt.Errorf("fork %q attempted %d write/edit tool call(s) — a fork must never mutate a file", fork.TranscriptPath, fork.WriteCalls)
		}
	}
	for _, fork := range audit.Forks {
		if cmd, found := firstMutatingGitCommand(fork.BashCommands); found {
			return fmt.Errorf("fork %q ran a git-mutating command (%q) — a fork must never run a state-mutating git command", fork.TranscriptPath, cmd)
		}
	}
	if audit.NamedSpawns > 0 {
		return fmt.Errorf("%d fork(s) were spawned with a name — named forks silently lose inherited context, which is a silent quality-degradation defect, not an advisory", audit.NamedSpawns)
	}
	return nil
}

// unreturnedForkWarnings returns one warning per fork of audit that never returned a final report; such a fork is sloppiness, not a failure.
func unreturnedForkWarnings(audit *shuttleengine.ForkAudit) []string {
	var warnings []string
	for _, fork := range audit.Forks {
		if !fork.ReportReturned {
			warnings = append(warnings, fmt.Sprintf("fork %q never returned a final report", fork.TranscriptPath))
		}
	}
	return warnings
}

// auditFixRound enforces the twill fixer's fork policy over audit, the audit of a fixer that spawned self-check forks for wantN lenses.
// A fork violation is a hard error, wrapped with its way forward; a fork count other than wantN, none included, is a warning, since the fix is on disk and failing the half would redo a finished fix.
// It returns the warnings, which also include one per fork that never returned a report, and the audited fork count, 0 for a nil audit.
func auditFixRound(audit *shuttleengine.ForkAudit, wantN int) (warnings []string, forks int, err error) {
	if audit == nil {
		return []string{fmt.Sprintf("twill fixer spawned 0 fork(s) for %d lens(es)", wantN)}, 0, nil
	}
	if violation := forkViolation(audit); violation != nil {
		return nil, len(audit.Forks), fmt.Errorf("burler: twill fixer %w; way forward: inspect the target's working-tree changes, revert what the fork wrote, and resume the run", violation)
	}
	forks = len(audit.Forks)
	if forks != wantN {
		warnings = append(warnings, fmt.Sprintf("twill fixer spawned %d fork(s) for %d lens(es)", forks, wantN))
	}
	warnings = append(warnings, unreturnedForkWarnings(audit)...)
	return warnings, forks, nil
}
