// cluster.go holds the fail-loud policy that Engine.Review enforces over a cluster round's shuttleengine.ForkAudit.
// It covers the fork-count contract and the per-fork violations a disobedient fork reviewer can trigger.
// auditClusterRound is the single place that turns the raw audit facts into burler's hard-error and warning split.
// auditFixRound applies the same per-fork checks to a twill fixer's self-check forks.
// A wrong fork count there is a warning.

package burlerengine

import (
	"errors"
	"fmt"
	"path"
	"slices"
	"strings"

	"github.com/Knatte18/loomyard/internal/shuttleengine"
)

// ErrClusterForksMissing is the sentinel error for a cluster round's fork audit.
// It marks an audit that does not carry exactly the requested number of fork reviewers.
var ErrClusterForksMissing = errors.New("the cluster round did not produce exactly the requested number of fork reviewers")

// mutatingGitSubcommands holds the git subcommands that change repository or working-tree state.
var mutatingGitSubcommands = map[string]bool{
	"add": true, "commit": true, "push": true, "pull": true, "fetch": true, "merge": true,
	"rebase": true, "reset": true, "restore": true, "rm": true, "mv": true, "checkout": true,
	"switch": true, "stash": true, "apply": true, "cherry-pick": true, "tag": true, "branch": true,
}

// gitValuedGlobalOptions holds git's global options whose value is the following word.
// That word is never read as the subcommand.
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

// gitTagValuedListOptions holds the git tag list-output options whose value may be the following word.
// A value such as -version:refname is never read as an option.
var gitTagValuedListOptions = map[string]bool{"--sort": true, "--format": true}

// gitBranchReadOptions holds the git branch long options that only read branches.
// Such an option never creates, deletes, renames, copies or moves a branch, or changes its upstream or description.
// The value reports whether the option puts git branch in list mode.
// In list mode every operand is a pattern rather than a branch to create.
var gitBranchReadOptions = map[string]bool{
	"--list": true, "--contains": true, "--no-contains": true, "--merged": true,
	"--no-merged": true, "--points-at": true, "--show-current": true,
	"--all": false, "--verbose": false, "--remotes": false, "--ignore-case": false, "--format": false, "--sort": false,
	"--column": false, "--no-column": false, "--color": false, "--no-color": false, "--abbrev": false,
	"--no-abbrev": false, "--omit-empty": false,
}

// gitBranchReadShortOptions holds the git branch short options that only read.
// The value reports whether the option puts git branch in list mode.
var gitBranchReadShortOptions = map[rune]bool{'l': true, 'v': false, 'a': false, 'r': false, 'i': false}

// gitBranchValuedReadOptions holds the git branch read options whose value is the next word unless given with =.
// A value such as -committerdate is never read as an option or an operand.
var gitBranchValuedReadOptions = map[string]bool{"--format": true, "--sort": true, "--points-at": true}

// gitRepositoryOptions holds git's global options that name the repository or working tree apart from the directory git runs in.
var gitRepositoryOptions = map[string]bool{"--git-dir": true, "--work-tree": true}

// gitInitValuedOptions holds the git init options whose value is the following word.
// That word is never read as the directory.
var gitInitValuedOptions = map[string]bool{
	"-b": true, "--initial-branch": true, "--template": true, "--separate-git-dir": true,
	"--object-format": true, "--ref-format": true,
}

// gitWorktreeAddValuedOptions holds the git worktree add options whose value is the following word.
// That word is never read as the path.
var gitWorktreeAddValuedOptions = map[string]bool{"-b": true, "-B": true, "--reason": true}

// gitCloneValuedOptions holds the git clone options whose value is the following word.
// That word is never read as the source or the directory.
var gitCloneValuedOptions = map[string]bool{
	"-o": true, "--origin": true, "-b": true, "--branch": true, "-u": true, "--upload-pack": true,
	"--reference": true, "--reference-if-able": true, "--separate-git-dir": true, "--depth": true,
	"--shallow-since": true, "--shallow-exclude": true, "-c": true, "--config": true, "--template": true,
	"-j": true, "--jobs": true, "--filter": true, "--server-option": true, "--bundle-uri": true, "--ref-format": true,
}

// gitCloneOriginOptions holds the git clone options that name the remote the clone points at its source.
var gitCloneOriginOptions = map[string]bool{"-o": true, "--origin": true}

// gitRemoteValuedOptions holds the git remote add options whose value is the following word.
// That word is never read as the remote's name or URL.
var gitRemoteValuedOptions = map[string]bool{"-t": true, "-m": true}

// gitPushValuedOptions holds the git push options whose value is the following word.
// That word is never read as the destination.
var gitPushValuedOptions = map[string]bool{"-o": true, "--push-option": true, "--receive-pack": true, "--exec": true}

// gitPushRedirectKeyPrefixes holds the prefixes of the config keys that can change where a push goes.
// An include key counts because the file it names may hold any of the others.
var gitPushRedirectKeyPrefixes = []string{"remote.", "url.", "branch.", "include.", "includeif."}

// gitRemoteChangingVerbs holds the git remote verbs that add, re-point, rename or remove a remote.
var gitRemoteChangingVerbs = map[string]bool{"add": true, "set-url": true, "rename": true, "remove": true, "rm": true}

// The fork-audit rules a state-changing git invocation can break, each worded as the violation message names it.
const (
	ruleOutsideCreated               = "a state-changing git command outside every repository the fork created"
	rulePushAfterLink                = "a push after ln, since a link may lead out of every repository the fork created"
	rulePushAfterRedirectKey         = "a push under or after a git config key that may redirect it"
	rulePushAfterUnknownRemoteChange = "a push after a git remote change in a directory the audit cannot resolve"
	rulePushWithRepo                 = "a push with --repo"
	rulePushSetsUpstreamOutside      = "a push with -u, --set-upstream or --recurse-submodules from outside every repository the fork created"
	rulePushNoDestination            = "a push with no destination"
	rulePushToRemoteOutside          = "a push to a remote the fork did not point into a repository it created"
	rulePushToPathOutside            = "a push to a URL or path outside every repository the fork created"
)

// mutatingGitCommand is a fork's Bash command that runs a state-mutating git invocation.
// The invocation may change state in a repository the fork did not create.
type mutatingGitCommand struct {
	// command is the Bash command as the transcript holds it.
	command string
	// rule names the audit rule the command's first counted invocation broke.
	rule string
	// pushFrom is the fork-created repository the command pushed from, when every such invocation of it is a push from one;
	// it is "" otherwise.
	pushFrom string
}

// mutatingGitCommands scans commands, one fork's Bash commands in transcript order.
// It returns, in order, each command that may change state through git in a repository the fork did not create.
// Each result names the rule the command's first counted invocation broke.
// A command is split into segments at ;, &, | and newlines,
// and a segment is a git invocation when its first word is git, bare or path-prefixed.
// Only the subcommand word decides: a mutating verb named as an argument, as in git help cherry-pick, never counts.
// The exceptions are git tag and git branch.
// A git tag counts only when it does not merely list tags.
// A git branch counts only when it does not merely list branches or show the current one.
// A -h or --help among the global options or directly after the subcommand only prints usage and never counts either;
// further along it is an option value or an operand, as in git commit -m -h.
//
// A mutating invocation does not count when its directory lies inside a repository the fork created earlier.
// The fork creates one by git init, by git clone, or by git worktree add from such a repository.
// So a fork may experiment on a throwaway repository.
// The directory is resolved lexically from the command's unknown starting directory.
// It applies the command's own cd segments first, then the invocation's -C options.
// Any invocation with --git-dir or --work-tree counts, as does one after a bare cd or cd -.
// Their target is not the resolved directory.
// A git init or git clone whose relative path is the starting directory or an ancestor of it records no repository.
// The same holds for a path that leaves the directory through .., since it may reinitialize the target.
// Nor does one whose path is or encloses a directory an earlier git call ran in outside every created repository.
// A .. that would climb above a leading ~ or $ word leaves the directory unresolvable.
// Such a directory matches no recorded repository.
//
// A git push does not count when its destination leads into a repository the fork created:
// a remote of the pushing repository that points into one, or a word written as a local path inside one.
// A relative path is read from the top level of the repository the push or remote call runs in, as git reads it.
// git remote add, git remote set-url and git clone point a remote; git remote rename and git remote remove forget one.
// A remote that stopped leading into a created repository never leads into one again.
// A git worktree shares the remotes of the repository it was added from.
// A push with no destination or with --repo counts.
// So does a push from outside every created repository with -u, --set-upstream or --recurse-submodules.
// So does every push after a config key that may redirect it.
// Such a key starts with remote., url., branch., include. or includeif.
// It is named by git config, -c, --config-env, or a git clone -c or --config.
// The same holds after a git remote change whose directory is unknown.
// It holds after any ln too, since the link may lead out.
func mutatingGitCommands(commands []string) []mutatingGitCommand {
	var repos forkRepositories
	var found []mutatingGitCommand
	for _, cmd := range commands {
		if pushFrom, rule := repos.runsMutatingGit(cmd); rule != "" {
			found = append(found, mutatingGitCommand{command: cmd, rule: rule, pushFrom: pushFrom})
		}
	}
	return found
}

// forkRepositories holds what one fork's commands created: repositories, and the remotes that lead into them.
type forkRepositories struct {
	// roots holds the lexically resolved directories of the repositories the fork created.
	roots []string
	// worktreeSources maps the root of a created worktree to the created repository whose config it shares.
	worktreeSources map[string]string
	// entered holds the lexically resolved directories git calls ran in outside every created repository.
	// The fork did not make them, so no later git init or git clone may claim one or an ancestor of one.
	entered []string
	// remotes maps a created repository's root to its remote names.
	// A name maps to true when the remote leads into a created repository.
	remotes map[string]map[string]bool
	// pushRedirect names the rule that makes every later push count; it is "" while no call has set one.
	// The first call that can redirect a push sets it, by naming a config key, changing a remote or making a link.
	pushRedirect string
}

// redirectPushes makes every later push count under rule, unless an earlier call already did under its own.
func (repos *forkRepositories) redirectPushes(rule string) {
	if repos.pushRedirect == "" {
		repos.pushRedirect = rule
	}
}

// runsMutatingGit reports the rule broken by the first counted git invocation of cmd, "" when none counts.
// An invocation counts when it may change state outside repos.
// It also returns the created repository cmd pushed from, when every such invocation of cmd is a push from one.
// It records each repository and remote cmd creates, and makes every later push count after an ln.
func (repos *forkRepositories) runsMutatingGit(cmd string) (pushFrom, rule string) {
	onlyOwnPushes := true
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
		case isLinkExecutable(words[0]):
			repos.redirectPushes(rulePushAfterLink)
		case isGitExecutable(words[0]):
			from, broken := repos.gitInvocationMutates(parseGitInvocation(words[1:], dir, dirKnown))
			if broken == "" {
				continue
			}
			if rule == "" {
				pushFrom, rule = from, broken
			}
			onlyOwnPushes = onlyOwnPushes && from != ""
		}
	}
	if !onlyOwnPushes {
		pushFrom = ""
	}
	return pushFrom, rule
}

// gitInvocationMutates reports the rule invocation breaks by changing state outside repos, "" when it breaks none.
// For a push it also returns the created repository it ran from, "" when it ran from none.
// It records the repository and the remote that invocation creates.
func (repos *forkRepositories) gitInvocationMutates(invocation gitInvocation) (pushFrom, rule string) {
	if invocation.helpRequested || len(invocation.args) > 0 && isHelpFlag(invocation.args[0]) {
		return "", ""
	}
	defer repos.enter(invocation)
	redirects := invocation.redirectsPush
	switch invocation.subcommand {
	case "config":
		redirects = redirects || namesPushRedirectKey(invocation.args)
	case "clone":
		redirects = redirects || namesPushRedirectKey(cloneConfigValues(invocation.args))
	}
	if redirects {
		repos.redirectPushes(rulePushAfterRedirectKey)
	}
	switch invocation.subcommand {
	case "init":
		directory := "."
		if operands := gitOperands(invocation.args, gitInitValuedOptions); len(operands) > 0 {
			directory = operands[0]
		}
		repos.record(invocation, directory)
	case "clone":
		repos.recordClone(invocation)
	case "worktree":
		if source, inside := repos.rootOf(invocation); inside && len(invocation.args) > 0 && invocation.args[0] == "add" {
			if operands := gitOperands(invocation.args[1:], gitWorktreeAddValuedOptions); len(operands) > 0 {
				if root, recorded := repos.record(invocation, operands[0]); recorded {
					if repos.worktreeSources == nil {
						repos.worktreeSources = map[string]string{}
					}
					repos.worktreeSources[root] = repos.configRoot(source)
				}
			}
		}
	case "remote":
		repos.recordRemote(invocation)
	case "tag":
		if gitTagListsOnly(invocation.args) {
			return "", ""
		}
	case "branch":
		if gitBranchReadsOnly(invocation.args) {
			return "", ""
		}
	case "push":
		return repos.pushMutates(invocation)
	}
	if !mutatingGitSubcommands[invocation.subcommand] {
		return "", ""
	}
	if _, inside := repos.rootOf(invocation); inside {
		return "", ""
	}
	return "", ruleOutsideCreated
}

// pushMutates reports the rule invocation, a git push, breaks, "" when it breaks none.
// A push breaks a rule when it may change a repository the fork did not create.
// It also returns the created repository the push ran from, "" when it ran from none.
func (repos *forkRepositories) pushMutates(invocation gitInvocation) (pushFrom, rule string) {
	from, _ := repos.rootOf(invocation)
	if repos.pushRedirect != "" {
		return from, repos.pushRedirect
	}
	for _, arg := range invocation.args {
		if arg == "--repo" || strings.HasPrefix(arg, "--repo=") {
			return from, rulePushWithRepo
		}
		// Outside every created repository, -u and --recurse-submodules write into the repository the push runs from.
		if from == "" && (arg == "-u" || arg == "--set-upstream" || arg == "--recurse-submodules" || strings.HasPrefix(arg, "--recurse-submodules=")) {
			return from, rulePushSetsUpstreamOutside
		}
	}
	destinations := gitOperands(invocation.args, gitPushValuedOptions)
	if len(destinations) == 0 {
		return from, rulePushNoDestination
	}
	destination := destinations[0]
	if leadsIn, named := repos.remotes[repos.configRoot(from)][destination]; named {
		if leadsIn {
			return from, ""
		}
		return from, rulePushToRemoteOutside
	}
	if isGitURL(destination) {
		return from, rulePushToPathOutside
	}
	// A destination that names no remote the fork pointed counts unless it is written as a path:
	// a bare word may name a remote the transcript never shows being set.
	looksLikePath := isAnchoredShellPath(destination) || strings.HasPrefix(destination, ".") || strings.Contains(destination, "/")
	if !looksLikePath {
		return from, rulePushToRemoteOutside
	}
	if !repos.leadsInto(invocation, destination) {
		return from, rulePushToPathOutside
	}
	return from, ""
}

// recordClone records the repository a git clone creates.
// The repository sits at the clone's directory operand, or at the name git derives from its source.
// The clone's origin, or the remote -o or --origin names, leads into a created repository when the source does.
func (repos *forkRepositories) recordClone(invocation gitInvocation) {
	operands := gitOperands(invocation.args, gitCloneValuedOptions)
	if len(operands) == 0 {
		return
	}
	source := operands[0]
	directory := cloneDirectoryName(source)
	if len(operands) > 1 {
		directory = operands[1]
	}
	root, recorded := repos.record(invocation, directory)
	if !recorded {
		return
	}
	origin := "origin"
	for i, arg := range invocation.args {
		if name, value, attached := strings.Cut(arg, "="); attached && name == "--origin" {
			origin = value
		} else if gitCloneOriginOptions[arg] && i+1 < len(invocation.args) {
			origin = invocation.args[i+1]
		}
	}
	repos.setRemote(root, origin, repos.sourceLeadsIn(invocation, source))
}

// cloneDirectoryName returns the directory git clone derives from source when given none:
// its last path element, without a trailing /.git or .git.
func cloneDirectoryName(source string) string {
	trimmed := strings.TrimSuffix(strings.TrimRight(source, "/"), "/.git")
	return strings.TrimSuffix(path.Base(trimmed), ".git")
}

// recordRemote follows a git remote call.
// git remote add and git remote set-url mark the named remote as leading into a created repository or not.
// The remote is the one of the repository whose config the call runs in.
// A set-url --add or --push keeps the remote leading in only when it did and the URL does.
// A set-url --delete never keeps it, since the URLs left are unknown.
// git remote rename and git remote remove forget every name they touch.
// A call whose directory is unknown may change the remotes of any created repository.
// It makes every later push count.
func (repos *forkRepositories) recordRemote(invocation gitInvocation) {
	if len(invocation.args) == 0 || !gitRemoteChangingVerbs[invocation.args[0]] {
		return
	}
	if !invocation.dirKnown {
		repos.redirectPushes(rulePushAfterUnknownRemoteChange)
		return
	}
	root, inside := repos.rootOf(invocation)
	if !inside {
		return
	}
	key := repos.configRoot(root)
	operands := gitOperands(invocation.args[1:], gitRemoteValuedOptions)
	if invocation.args[0] != "add" && invocation.args[0] != "set-url" {
		for _, name := range operands {
			repos.setRemote(key, name, false)
		}
		return
	}
	if len(operands) < 2 {
		return
	}
	leadsIn := !isGitURL(operands[1]) && repos.leadsInto(invocation, operands[1])
	if slices.Contains(invocation.args, "--add") || slices.Contains(invocation.args, "--push") {
		leadsIn = leadsIn && repos.remotes[key][operands[0]]
	}
	if slices.Contains(invocation.args, "--delete") {
		leadsIn = false
	}
	repos.setRemote(key, operands[0], leadsIn)
}

// setRemote marks the remote name of the repository keyed key as leading into a created repository or not.
// A remote marked as not leading in stays so:
// once the audit lost track of where it pushes, a later URL inside does not restore that trust.
func (repos *forkRepositories) setRemote(key, name string, leadsIn bool) {
	if current, known := repos.remotes[key][name]; known && !current {
		return
	}
	if repos.remotes == nil {
		repos.remotes = map[string]map[string]bool{}
	}
	if repos.remotes[key] == nil {
		repos.remotes[key] = map[string]bool{}
	}
	repos.remotes[key][name] = leadsIn
}

// record adds operand, resolved against invocation's directory, to repos, and returns the recorded root.
// It records nothing when the directory is unknown.
// It also records nothing when operand resolves to the starting directory or an ancestor of it.
// A path leaving the directory through .. is skipped too.
// Such a path may name the starting directory under its own basename.
// So is a directory an earlier git call ran in outside every created repository, or an ancestor of one:
// the fork did not make it.
func (repos *forkRepositories) record(invocation gitInvocation, operand string) (string, bool) {
	if !invocation.dirKnown {
		return "", false
	}
	created := resolveShellPath(invocation.dir, operand)
	if created == "." || created == ".." || strings.HasPrefix(created, "../") || repos.enclosesEntered(created) {
		return "", false
	}
	repos.roots = append(repos.roots, created)
	return created, true
}

// rootOf returns the created repository invocation's directory lies inside, and reports whether there is one.
// An unknown directory lies inside none.
func (repos *forkRepositories) rootOf(invocation gitInvocation) (string, bool) {
	if !invocation.dirKnown {
		return "", false
	}
	return repos.rootContaining(invocation.dir)
}

// rootContaining returns the innermost created repository that dir lies inside, and reports whether there is one.
func (repos *forkRepositories) rootContaining(dir string) (string, bool) {
	innermost := ""
	for _, root := range repos.roots {
		if (dir == root || strings.HasPrefix(dir, root+"/")) && len(root) > len(innermost) {
			innermost = root
		}
	}
	return innermost, innermost != ""
}

// configRoot returns the created repository whose config the repository at root uses.
// A worktree shares the config, and so the remotes, of the repository it was added from.
func (repos *forkRepositories) configRoot(root string) string {
	if source, shared := repos.worktreeSources[root]; shared {
		return source
	}
	return root
}

// enter remembers the directory invocation ran in when it lies inside no created repository.
func (repos *forkRepositories) enter(invocation gitInvocation) {
	if !invocation.dirKnown {
		return
	}
	if _, inside := repos.rootContaining(invocation.dir); !inside {
		repos.entered = append(repos.entered, invocation.dir)
	}
}

// enclosesEntered reports whether created is, or is an ancestor of, a directory that enter remembered.
func (repos *forkRepositories) enclosesEntered(created string) bool {
	prefix := strings.TrimSuffix(created, "/") + "/"
	for _, dir := range repos.entered {
		if dir == created || strings.HasPrefix(dir, prefix) {
			return true
		}
	}
	return false
}

// leadsInto reports whether p lies inside a created repository.
// p is a local path or remote URL given to a git call run in invocation's directory.
// Git reads a relative p from the top level of the repository the call runs in.
// So a relative p leads nowhere when that directory lies inside no created repository.
func (repos *forkRepositories) leadsInto(invocation gitInvocation, p string) bool {
	base := ""
	if !isAnchoredShellPath(p) {
		root, inside := repos.rootOf(invocation)
		if !inside {
			return false
		}
		base = root
	}
	_, inside := repos.rootContaining(resolveShellPath(base, p))
	return inside
}

// sourceLeadsIn reports whether source is a local path inside a created repository.
// source is the source of a git clone run in invocation's directory.
// A clone runs in no repository, so git reads a relative source from that directory.
func (repos *forkRepositories) sourceLeadsIn(invocation gitInvocation, source string) bool {
	if isGitURL(source) || !isAnchoredShellPath(source) && !invocation.dirKnown {
		return false
	}
	_, inside := repos.rootContaining(resolveShellPath(invocation.dir, source))
	return inside
}

// isGitURL reports whether word, a git clone source, remote URL or push destination, is a URL or host:path location:
// it holds a colon with no slash before it.
func isGitURL(word string) bool {
	slash, colon := strings.Index(word, "/"), strings.Index(word, ":")
	return colon >= 0 && (slash < 0 || colon < slash)
}

// namesPushRedirectKey reports whether a word of words names a config key that can change where a push goes.
// A word counts alone, as a key=value pair, or as the value of a --name=value option.
func namesPushRedirectKey(words []string) bool {
	for _, word := range words {
		if _, value, attached := strings.Cut(word, "="); attached && strings.HasPrefix(word, "--") {
			word = value
		}
		lower := strings.ToLower(word)
		for _, prefix := range gitPushRedirectKeyPrefixes {
			if strings.HasPrefix(lower, prefix) {
				return true
			}
		}
	}
	return false
}

// cloneConfigValues returns the settings args, the words after git clone, give with -c or --config.
// Only these can set a config key; the source and directory operands are never keys.
func cloneConfigValues(args []string) []string {
	var values []string
	for i, arg := range args {
		switch {
		case (arg == "-c" || arg == "--config") && i+1 < len(args):
			values = append(values, args[i+1])
		case strings.HasPrefix(arg, "--config="):
			values = append(values, strings.TrimPrefix(arg, "--config="))
		case strings.HasPrefix(arg, "-c") && len(arg) > 2 && !strings.HasPrefix(arg, "--"):
			values = append(values, arg[2:])
		}
	}
	return values
}

// gitInvocation is one git call of a shell segment, with the directory it acts on.
type gitInvocation struct {
	// subcommand is the first non-option word after git's global options, "" when there is none.
	subcommand string
	// args holds the words after subcommand.
	args []string
	// helpRequested reports whether a -h or --help stood among the global options.
	helpRequested bool
	// redirectsPush reports whether a global -c or --config-env named a config key that can change where a push goes.
	redirectsPush bool
	// dir is the lexically resolved directory git acts on, meaningful only when dirKnown.
	dir string
	// dirKnown reports whether dir names the repository git acts on.
	dirKnown bool
}

// parseGitInvocation parses args, the words after git, for a command run from dir.
// It skips the value of each valued global option and applies each -C to the directory.
// The result's dirKnown reports whether dir is known,
// and it is cleared when --git-dir or --work-tree names the repository apart from the directory.
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
		case strings.HasPrefix(args[i], "--config-env="):
			if namesPushRedirectKey([]string{strings.TrimPrefix(args[i], "--config-env=")}) {
				invocation.redirectsPush = true
			}
		case gitValuedGlobalOptions[args[i]]:
			if (args[i] == "-c" || args[i] == "--config-env") && i+1 < len(args) && namesPushRedirectKey(args[i+1:i+2]) {
				invocation.redirectsPush = true
			}
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

// gitOperands returns the non-option words of args in order.
// It skips the value of each option that valuedOptions holds, and every word after a -- is an operand.
func gitOperands(args []string, valuedOptions map[string]bool) []string {
	var operands []string
	for i := 0; i < len(args); i++ {
		switch {
		case args[i] == "--":
			return append(operands, args[i+1:]...)
		case valuedOptions[args[i]]:
			i++
		case strings.HasPrefix(args[i], "-"):
		default:
			operands = append(operands, args[i])
		}
	}
	return operands
}

// isAnchoredShellPath reports whether p names its directory independently of the current directory:
// absolute, or led by a ~ or a $ variable.
func isAnchoredShellPath(p string) bool {
	return strings.HasPrefix(p, "/") || strings.HasPrefix(p, "~") || strings.HasPrefix(p, "$")
}

// resolveShellPath lexically resolves p against dir.
// An anchored p stands as written and cleaned;
// if a .. would climb above its leading ~ or $ word, the result is ../ followed by p.
// That result lies inside no anchored root.
// A relative p joins dir; under a relative dir the result stays relative to the starting directory,
// so a result such as .. or ../x escapes it.
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
// A bare git tag lists tags;
// otherwise a list-mode option must stand before any -- with no tag-writing option beside it.
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
// Every option must be a known read-only option.
// A short-option cluster such as -avv counts letter by letter,
// and a long option counts in its --name=value form too.
// Any other option keeps the command mutating.
// An operand, any word after --, makes git branch create a branch.
// A list-mode option such as --list or --contains turns operands into patterns instead.
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
// An unquoted < or > redirection is no word.
// The operator, its target word, and any unquoted file-descriptor number before it are dropped;
// an & after the operator is dropped too, so 2>/dev/null is never read as an operand.
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

// isLinkExecutable reports whether word names the ln binary, bare or behind a directory path.
func isLinkExecutable(word string) bool {
	return word == "ln" || strings.HasSuffix(word, "/ln")
}

// isGitExecutable reports whether word names the git binary, bare or behind a directory path.
func isGitExecutable(word string) bool {
	return word == "git" || strings.HasSuffix(word, "/git") || strings.HasSuffix(word, `\git`)
}

// auditClusterRound enforces the cluster-round fail-loud policy.
// Checks run in a fixed order; the first violation is returned as a hard error.
// The error is wrapped with its way forward, which says when the audit found no target change.
func auditClusterRound(audit *shuttleengine.ForkAudit, wantN int) ([]string, error) {
	gotN := 0
	if audit != nil {
		gotN = len(audit.Forks)
	}
	if audit == nil || gotN != wantN {
		return nil, fmt.Errorf("burler: %w (requested %d, spawned %d)", ErrClusterForksMissing, wantN, gotN)
	}

	if violation := forkViolation(audit); violation != nil {
		return nil, fmt.Errorf("burler: %w; %s", violation, violationWayForward(violation))
	}
	return unreturnedForkWarnings(audit), nil
}

// forkViolationError is the first fork rule an audit found broken.
type forkViolationError struct {
	message string
	// targetUntouched reports whether the audit found no target change.
	// It holds when no fork made a write or edit call.
	// Every counted git invocation must also be a push from a repository its fork created.
	targetUntouched bool
	// pushFrom is the fork-created repository the audit's first push-only command pushed from, "" when there is none.
	pushFrom string
}

func (e *forkViolationError) Error() string { return e.message }

// forkViolation returns the first violation of audit, or nil when the forks are clean.
// The checks run in this order: a nested spawn call, a write or edit call, a git-mutating command, and a named spawn.
// The error carries no burler prefix, so each caller adds its own.
func forkViolation(audit *shuttleengine.ForkAudit) *forkViolationError {
	untouched := true
	pushFrom := ""
	for _, fork := range audit.Forks {
		untouched = untouched && fork.WriteCalls == 0
		for _, cmd := range mutatingGitCommands(fork.BashCommands) {
			untouched = untouched && cmd.pushFrom != ""
			if pushFrom == "" {
				pushFrom = cmd.pushFrom
			}
		}
	}
	violation := func(format string, args ...any) *forkViolationError {
		return &forkViolationError{message: fmt.Sprintf(format, args...), targetUntouched: untouched, pushFrom: pushFrom}
	}
	for _, fork := range audit.Forks {
		if fork.AgentCalls > 0 {
			return violation("fork %q attempted %d nested spawn call(s) — forks cannot nest and must never spawn a fork or subagent, even when the attempt was denied", fork.TranscriptPath, fork.AgentCalls)
		}
	}
	for _, fork := range audit.Forks {
		if fork.WriteCalls > 0 {
			return violation("fork %q attempted %d write/edit tool call(s) — a fork must never mutate a file", fork.TranscriptPath, fork.WriteCalls)
		}
	}
	for _, fork := range audit.Forks {
		if found := mutatingGitCommands(fork.BashCommands); len(found) > 0 {
			return violation("fork %q ran a git-mutating command (%q), counted as %s — a fork must never run a state-mutating git command that may change a repository it did not create", fork.TranscriptPath, found[0].command, found[0].rule)
		}
	}
	if audit.NamedSpawns > 0 {
		return violation("%d fork(s) were spawned with a name — named forks silently lose inherited context, which is a silent quality-degradation defect, not an advisory", audit.NamedSpawns)
	}
	return nil
}

// violationWayForward words what the target holds after violation and the way forward from it.
// It says when the audit found no target change, so the operator is never sent to undo a change the audit did not find.
func violationWayForward(violation *forkViolationError) string {
	switch {
	case violation.targetUntouched && violation.pushFrom != "":
		return fmt.Sprintf("no target change was found: the audit found no write or edit call and no state-changing git command outside a repository a fork created, and the push from %s went to a destination the audit could not place; way forward: check that destination is neither the target's repository nor one of its remotes, and resume the run", violation.pushFrom)
	case violation.targetUntouched:
		return "no target change was found: the audit found no write or edit call and no state-changing git command outside a repository a fork created; way forward: resume the run"
	}
	return "way forward: inspect the target's working tree, HEAD, branches, tags and remotes for what the fork changed, undo it, and resume the run"
}

// unreturnedForkWarnings returns one warning per fork of audit that never returned a final report.
// Such a fork is sloppiness, not a failure.
func unreturnedForkWarnings(audit *shuttleengine.ForkAudit) []string {
	var warnings []string
	for _, fork := range audit.Forks {
		if !fork.ReportReturned {
			warnings = append(warnings, fmt.Sprintf("fork %q never returned a final report", fork.TranscriptPath))
		}
	}
	return warnings
}

// auditFixRound checks the fork audit of a twill fixer that spawned self-check forks for wantN lenses.
// A fork violation is a hard error, wrapped with its way forward, which says when the audit found no target change.
// A fork count other than wantN, zero included, is a warning.
// The fix is already on disk; failing the half would redo a finished fix.
// It returns the warnings, including one per fork that never returned a report.
// It also returns the audited fork count, which is 0 for a nil audit.
// A fresh subagent the fixer started to make its fixes writes on the fixer's behalf:
// it is no self-check fork, so this audit neither checks nor counts it.
// A name given to it is no named-fork violation.
// A subagent belongs to whichever subagent the session itself spawned at the head of its spawner chain.
// Under a fresh subagent it is part of that subagent's work, so it is neither checked nor counted.
// Under a fork it is checked like that fork, so what it changed in the target reaches the way forward.
func auditFixRound(audit *shuttleengine.ForkAudit, wantN int) (warnings []string, forks int, err error) {
	if audit == nil {
		return []string{fmt.Sprintf("twill fixer spawned 0 fork(s) for %d lens(es)", wantN)}, 0, nil
	}
	selfCheck := *audit
	selfCheck.Forks = nil
	selfCheck.NamedSpawns = audit.NamedForkSpawns
	reports := map[string]shuttleengine.ForkReport{}
	for _, fork := range audit.Forks {
		reports[fork.TranscriptPath] = fork
	}
	for _, fork := range audit.Forks {
		head := fork
		for head.SpawnerTranscriptPath != "" {
			head = reports[head.SpawnerTranscriptPath]
		}
		if !head.FreshSubagent {
			selfCheck.Forks = append(selfCheck.Forks, fork)
		}
	}
	if violation := forkViolation(&selfCheck); violation != nil {
		return nil, len(selfCheck.Forks), fmt.Errorf("burler: twill fixer %w; %s", violation, violationWayForward(violation))
	}
	forks = len(selfCheck.Forks)
	if forks != wantN {
		warnings = append(warnings, fmt.Sprintf("twill fixer spawned %d fork(s) for %d lens(es)", forks, wantN))
	}
	warnings = append(warnings, unreturnedForkWarnings(&selfCheck)...)
	return warnings, forks, nil
}
