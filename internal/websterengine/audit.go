// audit.go implements webster's own fail-loud policy over the provider-invariant
// shuttleengine.ForkAudit/ForkReport fact shapes: the violation classes a forked implementer or
// Master's own parent session can trigger,
// and the fabric-reference matcher both checks share.
// Unlike burlerengine's read-only cluster-round policy (a fork reviewer must never mutate
// anything), webster's forks are implementers — Write/Edit and repo-native git are the whole point of
// a batch, so CheckFork bans only nesting (Agent calls), fabric references, and writes to the run's
// two contract files (outcome.yaml/summary.md — Master's alone), never batch writes.
// Master's own parent transcript is the mirror image: writes are banned everywhere EXCEPT the run's
// two contract files, since a Master that "helpfully" implements a batch itself or hand-writes a
// batch report defeats the fork-audit design as silently as a named spawn does.
// Every function here is pure — facts in, verdict out — per the discussion's TDD-centre framing;
// all transcript reading stays in claudeengine, per the Shuttle Provider-Seam Invariant.

package websterengine

import (
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/Knatte18/loomyard/internal/shuttleengine"
)

// RefMatcher is the narrow seam CheckFork and CheckParent consult for the fabric-reference violation
// class. *fabricengine.RefScanner satisfies it without any adapter, since that type already has the
// identical method.
type RefMatcher interface {
	// Matches reports whether cmd references fabric's two-checkout mechanism.
	Matches(cmd string) bool
}

// NeverMatches is the pinned RefMatcher supplier for a mode with no fabric repo at all — standalone
// mode's answer where hub mode supplies a real *fabricengine.RefScanner.
// It lives here, beside the interface it implements, rather than in a geometry package that has no
// business knowing webster's audit vocabulary, and it is a named exported type rather than an inline
// literal so every Deps-construction site shares one supplier instead of re-inventing it.
// CheckFork and CheckParent call Matches unguarded, so a nil RefMatcher is a panic on the first
// standalone record-batch — the field must therefore never be nil in either mode.
type NeverMatches struct{}

// Matches always returns false: NeverMatches never sees a fabric reference, because a mode that
// supplies it has no fabric repo to reference.
func (NeverMatches) Matches(string) bool {
	return false
}

// AuditViolationClass discriminates the fail-loud violation classes CheckFork and CheckParent can
// report.
// It exists so a caller (e.g.
// record-batch's error message,
// or a future metrics hook) can branch on WHAT kind of violation occurred without parsing Detail's
// free-text prose.
type AuditViolationClass string

// The set of hard violation classes CheckFork and CheckParent can report.
const (
	// ClassNestedAgent means a fork's own transcript attempted an Agent tool call — forks cannot nest,
	// even when Claude Code denied the attempt.
	ClassNestedAgent AuditViolationClass = "nested-agent"
	// ClassFabricReference means a Bash command (fork or parent) invoked lyx fabric (or an older,
	// pre-cutover single-word spelling of the same subcommand),
	// or referenced the fabric worktree path — agents never touch the fabric repo directly.
	ClassFabricReference AuditViolationClass = "fabric-reference"
	// ClassNamedSpawn means Master's parent transcript recorded one or more Agent calls carrying a
	// name parameter — named forks silently lose inherited context.
	ClassNamedSpawn AuditViolationClass = "named-spawn"
	// ClassParentWrite means Master's parent transcript wrote a file other than the run's two contract
	// files (outcome.yaml, summary.md) — a Master implementing batches itself or hand-writing a batch
	// report.
	ClassParentWrite AuditViolationClass = "parent-write"
	// ClassForkContractWrite means a fork's own transcript wrote one of the run's two contract files
	// (outcome.yaml, summary.md) — those are Master's only permitted writes, and a fork writing them
	// forges the run's terminal judgment.
	ClassForkContractWrite AuditViolationClass = "fork-contract-write"
)

// AuditViolation is one hard fork-audit policy violation observed in either a fork's own transcript
// or Master's parent transcript.
// Every hard violation is an error — the fail-loud posture treats a detected violation as
// build-breaking, not merely logged — so AuditViolation implements the error interface directly and
// a caller can return a []AuditViolation entry (or an aggregate wrapping them) as an ordinary Go
// error.
// TranscriptPath is the fork's TranscriptPath for a CheckFork violation,
// or "" for a CheckParent violation (ForkAudit carries no path for Master's own parent transcript —
// webster tracks Master's session ID separately, in State.MasterSessionID).
// Key is the finding's deterministic identity, filled by CheckFork and CheckParent:
// `fork:<transcript path>:<class>:<ordinal>` for a fork finding (the ordinal counts that class within the transcript),
// `parent:<class>:<ordinal>` for a parent one (1..NamedSpawns for named-spawn, the index into ParentWrites or ParentBashCommands otherwise).
// Path is the transcript-recorded write path for parent-write and fork-contract-write, empty for every other class.
// Command is the Bash command a fabric-reference finding matched, empty for every other class.
type AuditViolation struct {
	Class          AuditViolationClass
	TranscriptPath string
	Detail         string
	Key            string
	Path           string
	Command        string
}

// AuditSeverity is the D4 class of an audit finding: whether it endangers the batch's correctness or only breaks webster's process policy.
type AuditSeverity string

const (
	// AuditSeverityCorrectness marks a finding that can change the run's own state or the code under review.
	AuditSeverityCorrectness AuditSeverity = "correctness"
	// AuditSeverityPolicy marks a finding that breaks a process rule without touching the run's state or tracked content.
	AuditSeverityPolicy AuditSeverity = "policy"
)

// forkKey builds a fork finding's Key from its transcript, class and per-class ordinal.
func forkKey(transcript string, class AuditViolationClass, ordinal int) string {
	return fmt.Sprintf("fork:%s:%s:%d", transcript, class, ordinal)
}

// parentKey builds a parent finding's Key from its class and ordinal.
func parentKey(class AuditViolationClass, ordinal int) string {
	return fmt.Sprintf("parent:%s:%d", class, ordinal)
}

// Error implements the error interface, formatting the violation as a single-line, webster-prefixed
// message a caller can wrap or surface verbatim.
func (v AuditViolation) Error() string {
	if v.TranscriptPath == "" {
		return fmt.Sprintf("webster: %s violation: %s", v.Class, v.Detail)
	}
	return fmt.Sprintf("webster: %s violation in %q: %s", v.Class, v.TranscriptPath, v.Detail)
}

// CheckFork evaluates one fork's transcript facts against webster's implementer policy: Write/Edit
// and repo-native git are explicitly allowed.
// It bans three hard violations: any attempted Agent call, any write to the two contract files
// (outcomePath or summaryPath), and any Bash command referencing the fabric repo.
// fabricRef is the injected RefMatcher — the caller-supplied fabric-reference class matcher (a real
// *fabricengine.RefScanner in hub mode, NeverMatches in standalone) — and is never nil in either
// mode: Matches is called unguarded here, so a nil interface is a panic, which is why NeverMatches
// exists as the pinned no-fabric supplier.
func CheckFork(f shuttleengine.ForkReport, outcomePath, summaryPath, workdir string, fabricRef RefMatcher) []AuditViolation {
	var violations []AuditViolation

	if f.AgentCalls > 0 {
		violations = append(violations, AuditViolation{
			Class:          ClassNestedAgent,
			TranscriptPath: f.TranscriptPath,
			Key:            forkKey(f.TranscriptPath, ClassNestedAgent, 1),
			Detail: fmt.Sprintf(
				"attempted %d Agent tool call(s) — forks cannot nest and must never call the Agent tool, even when the attempt was denied",
				f.AgentCalls,
			),
		})
	}

	cleanOutcome := filepath.Clean(outcomePath)
	cleanSummary := filepath.Clean(summaryPath)
	contractWrites := 0
	for _, w := range f.WritePaths {
		cw := resolveWritePath(workdir, w)
		if cw == cleanOutcome || cw == cleanSummary {
			contractWrites++
			violations = append(violations, AuditViolation{
				Class:          ClassForkContractWrite,
				TranscriptPath: f.TranscriptPath,
				Key:            forkKey(f.TranscriptPath, ClassForkContractWrite, contractWrites),
				Path:           w,
				Detail:         fmt.Sprintf("fork wrote %q — outcome.yaml and summary.md are Master's own contract files; a fork writing either forges the run's terminal judgment", w),
			})
		}
	}

	fabricRefs := 0
	for _, cmd := range f.BashCommands {
		if fabricRef.Matches(cmd) {
			fabricRefs++
			violations = append(violations, AuditViolation{
				Class:          ClassFabricReference,
				TranscriptPath: f.TranscriptPath,
				Key:            forkKey(f.TranscriptPath, ClassFabricReference, fabricRefs),
				Command:        cmd,
				Detail:         fabricReferenceDetail(cmd, "an implementer fork must never touch the fabric repo directly"),
			})
		}
	}

	return violations
}

// resolveWritePath canonicalizes a transcript-recorded write path: cleaned,
// and — when relative — resolved against workdir. This prevents false matches
// when agents mix absolute and relative spellings for the same file.
func resolveWritePath(workdir, path string) string {
	cleaned := filepath.Clean(path)
	if isTranscriptPathAbsolute(path) {
		return cleaned
	}
	return filepath.Join(workdir, cleaned)
}

// isTranscriptPathAbsolute reports whether a transcript-recorded write path is
// already absolute — in either the running OS's native sense (stdlib
// filepath.IsAbs: a drive letter or UNC prefix on Windows) or POSIX-style (a
// leading "/"). Transcript-recorded write paths and this package's own
// workdir/contract-path arguments are always POSIX-style, regardless of the workdir's platform
// (they come from the pane's own working-directory convention, not a raw
// OS path) — so on Windows, stdlib filepath.IsAbs alone reports false for a
// path like "/hub/master-builder/_lyx/webster/outcome.yaml" (Windows requires
// a drive letter or UNC prefix to consider a path absolute), and
// resolveWritePath would incorrectly join an already-absolute path against
// workdir a second time, producing a path that can never match the caller's
// absolute contract paths.
func isTranscriptPathAbsolute(path string) bool {
	return filepath.IsAbs(path) || strings.HasPrefix(path, "/")
}

// CheckParent evaluates Master's own parent-session facts: the mirror image of CheckFork's policy.
// Master must NOT write except to the two contract files (outcomePath and summaryPath).
// It bans three hard violations: any named spawn, any parent write outside contract files, and any
// Bash command referencing the fabric repo.
// fabricRef is the injected RefMatcher — the caller-supplied fabric-reference class matcher (a real
// *fabricengine.RefScanner in hub mode, NeverMatches in standalone) — and is never nil in either
// mode: Matches is called unguarded here, so a nil interface is a panic, which is why NeverMatches
// exists as the pinned no-fabric supplier.
func CheckParent(a shuttleengine.ForkAudit, outcomePath, summaryPath, workdir string, fabricRef RefMatcher) []AuditViolation {
	var violations []AuditViolation

	for i := 1; i <= a.NamedSpawns; i++ {
		violations = append(violations, AuditViolation{
			Class: ClassNamedSpawn,
			Key:   parentKey(ClassNamedSpawn, i),
			Detail: fmt.Sprintf(
				"named spawn %d of %d: a fork was spawned with a name — named forks silently lose inherited context, which is a silent quality-degradation defect, not an advisory",
				i, a.NamedSpawns,
			),
		})
	}

	cleanOutcome := filepath.Clean(outcomePath)
	cleanSummary := filepath.Clean(summaryPath)
	for i, w := range a.ParentWrites {
		cw := resolveWritePath(workdir, w)
		if cw != cleanOutcome && cw != cleanSummary {
			violations = append(violations, AuditViolation{
				Class:  ClassParentWrite,
				Key:    parentKey(ClassParentWrite, i),
				Path:   w,
				Detail: fmt.Sprintf("Master wrote %q — Master may write only its two contract files (outcome.yaml and summary.md); any other write means Master implemented a batch itself or hand-wrote a batch report", w),
			})
		}
	}

	for i, cmd := range a.ParentBashCommands {
		if fabricRef.Matches(cmd) {
			violations = append(violations, AuditViolation{
				Class:   ClassFabricReference,
				Key:     parentKey(ClassFabricReference, i),
				Command: cmd,
				Detail:  fabricReferenceDetail(cmd, "Master must never touch the fabric repo directly; the fabric sync is webstercli's own in-process job"),
			})
		}
	}

	return violations
}

// fabricReferenceDetail words a fabric-reference finding; a command that can change files says it can rewrite run state.
func fabricReferenceDetail(cmd, rule string) string {
	if mutatingCommand(cmd) {
		return fmt.Sprintf("ran a fabric-referencing command (%q) that can rewrite run state — %s", cmd, rule)
	}
	return fmt.Sprintf("ran a fabric-referencing command (%q) — %s", cmd, rule)
}

var (
	readOnlyGit    = stringSet("status", "log", "show", "diff", "rev-parse", "ls-files", "ls-tree", "cat-file", "grep", "blame", "describe", "for-each-ref", "show-ref", "merge-base")
	readOnlyFabric = stringSet("status", "diff", "list", "pairs")
	// readOnlyTools are programs that write nothing without an output redirect.
	readOnlyTools = stringSet("cat", "ls", "grep", "head", "tail", "wc", "stat", "cd", "pwd", "echo", "printf", "true", "test", "[")

	// gitValueOptions are git's global options whose value may follow as a separate word.
	gitValueOptions = stringSet("-C", "-c", "--git-dir", "--work-tree", "--namespace", "--config-env", "--super-prefix")
	// shellKeywords are words that open or join a compound command and run nothing themselves.
	shellKeywords = stringSet("{", "}", "!", "if", "then", "else", "elif", "do", "while", "until")
	// commandWrappers run the command that follows their own options.
	commandWrappers = stringSet("sudo", "env", "nice", "nohup", "command", "exec", "time")
	// xargsValueOptions are xargs options whose value follows as a separate word.
	xargsValueOptions = stringSet("-I", "-n", "-P", "-d", "-E", "-L", "-s", "-a")
	// systemBinDirs are the directories a path-spelled program may run from and still count as its allowlisted name.
	systemBinDirs = stringSet("/bin", "/usr/bin", "/usr/local/bin")
	// harmlessRedirectTargets are output redirect targets that change no file.
	harmlessRedirectTargets = stringSet("/dev/null", "/dev/stdout", "/dev/stderr")
)

func stringSet(words ...string) map[string]bool {
	set := make(map[string]bool, len(words))
	for _, w := range words {
		set[w] = true
	}
	return set
}

// mutatingCommand reports whether the Bash command cmd can change files; it fails closed.
// It splits cmd into simple commands, quote-aware, at `&&`, `||`, `;`, `|`, a lone `&`, a newline and a parenthesis,
// and also reads each `$(...)` or backtick substitution as a command of its own.
// It is true when any simple command carries an output redirect other than descriptor duplication or one to /dev/null, /dev/stdout or /dev/stderr,
// or when it is not a readOnlySegment.
// Only a command whose every simple command is a known read-only program with no write redirect is read-only;
// a command with no simple command and no substitution at all is mutating, so an empty or unparseable command fails closed.
func mutatingCommand(cmd string) bool {
	segments, substitutions := splitShell(cmd)
	if len(segments) == 0 && len(substitutions) == 0 {
		return true
	}
	for _, inner := range substitutions {
		if mutatingCommand(inner) {
			return true
		}
	}
	for _, s := range segments {
		if s.writes || !readOnlySegment(s.words) {
			return true
		}
	}
	return false
}

// shellSegment is one simple command of a Bash command line.
type shellSegment struct {
	// words are the command's words with quotes and escapes removed, redirects and their targets left out.
	words []string
	// writes is true when the command carries an output redirect onto a file.
	writes bool
}

// splitShell splits cmd into its simple commands and returns, separately, the body of every `$(...)` and backtick substitution it carries.
// It honours single and double quotes, backslash escapes, `#` comments and here-document bodies,
// so an operator character inside a quoted argument neither splits the command nor reads as a redirect.
func splitShell(cmd string) (segments []shellSegment, substitutions []string) {
	var (
		seg        shellSegment
		word       strings.Builder
		inWord     bool
		quoted     bool
		pending    byte // 'w' write-redirect target, 'd' dup-redirect target, 'r' input target, 'h' here-document delimiter
		heredocs   []string
		quote      byte
		n          = len(cmd)
		flushWord  func()
		endSegment func()
	)
	flushWord = func() {
		if !inWord {
			return
		}
		w := word.String()
		word.Reset()
		inWord, quoted = false, false
		switch pending {
		case 'w':
			if !harmlessRedirectTargets[w] {
				seg.writes = true
			}
		case 'd':
			if w != "-" && strings.Trim(w, "0123456789") != "" && !harmlessRedirectTargets[w] {
				seg.writes = true
			}
		case 'h':
			heredocs = append(heredocs, w)
		case 'r':
		default:
			seg.words = append(seg.words, w)
		}
		pending = 0
	}
	endSegment = func() {
		flushWord()
		pending = 0
		if len(seg.words) > 0 || seg.writes {
			segments = append(segments, seg)
		}
		seg = shellSegment{}
	}
	// substitution returns the body of the substitution opening at cmd[start:] and the index of its closing character.
	substitution := func(start int) (string, int) {
		if cmd[start] == '`' {
			for j := start + 1; j < n; j++ {
				if cmd[j] == '\\' {
					j++
					continue
				}
				if cmd[j] == '`' {
					return cmd[start+1 : j], j
				}
			}
			return cmd[start+1:], n - 1
		}
		depth := 0
		var inner byte
		for j := start + 1; j < n; j++ {
			c := cmd[j]
			switch {
			case inner != 0:
				if c == inner {
					inner = 0
				}
			case c == '\'' || c == '"':
				inner = c
			case c == '(':
				depth++
			case c == ')':
				depth--
				if depth == 0 {
					return cmd[start+2 : j], j
				}
			}
		}
		return cmd[start+2:], n - 1
	}

	for i := 0; i < n; i++ {
		c := cmd[i]
		if quote == '\'' {
			if c == '\'' {
				quote = 0
			} else {
				word.WriteByte(c)
			}
			continue
		}
		if quote == '"' {
			switch {
			case c == '"':
				quote = 0
			case c == '\\' && i+1 < n:
				i++
				word.WriteByte(cmd[i])
			case c == '`' || (c == '$' && i+1 < n && cmd[i+1] == '('):
				body, end := substitution(i)
				substitutions = append(substitutions, body)
				i = end
			default:
				word.WriteByte(c)
			}
			continue
		}
		switch {
		case c == '\'' || c == '"':
			quote = c
			inWord, quoted = true, true
		case c == '\\' && i+1 < n:
			i++
			if cmd[i] != '\n' {
				word.WriteByte(cmd[i])
				inWord = true
			}
		case c == '`' || (c == '$' && i+1 < n && cmd[i+1] == '('):
			body, end := substitution(i)
			substitutions = append(substitutions, body)
			inWord = true
			i = end
		case c == '#' && !inWord:
			for i+1 < n && cmd[i+1] != '\n' {
				i++
			}
		case c == ' ' || c == '\t':
			flushWord()
		case c == '\n':
			endSegment()
			for _, delim := range heredocs {
				for i+1 < n {
					end := strings.IndexByte(cmd[i+1:], '\n')
					line := cmd[i+1:]
					if end >= 0 {
						line = cmd[i+1 : i+1+end]
						i += end + 1
					} else {
						i = n
					}
					if strings.TrimLeft(line, "\t") == delim {
						break
					}
				}
			}
			heredocs = nil
		case c == ';' || c == '(' || c == ')':
			endSegment()
		case c == '|':
			endSegment()
			if i+1 < n && (cmd[i+1] == '|' || cmd[i+1] == '&') {
				i++
			}
		case c == '&':
			if i+1 < n && cmd[i+1] == '>' {
				continue
			}
			endSegment()
			if i+1 < n && cmd[i+1] == '&' {
				i++
			}
		case c == '>':
			if inWord && !quoted && strings.Trim(word.String(), "0123456789") == "" {
				word.Reset()
				inWord = false
			}
			flushWord()
			pending = 'w'
			if i+1 < n && (cmd[i+1] == '>' || cmd[i+1] == '|') {
				i++
			} else if i+1 < n && cmd[i+1] == '&' {
				i++
				pending = 'd'
			}
		case c == '<':
			if inWord && !quoted && strings.Trim(word.String(), "0123456789") == "" {
				word.Reset()
				inWord = false
			}
			flushWord()
			pending = 'r'
			switch {
			case strings.HasPrefix(cmd[i:], "<<<"):
				i += 2
			case strings.HasPrefix(cmd[i:], "<<-"):
				i += 2
				pending = 'h'
			case strings.HasPrefix(cmd[i:], "<<"):
				i++
				pending = 'h'
			case strings.HasPrefix(cmd[i:], "<>"):
				i++
				pending = 'w'
			case strings.HasPrefix(cmd[i:], "<&"):
				i++
			}
		default:
			word.WriteByte(c)
			inWord = true
		}
	}
	endSegment()
	return segments, substitutions
}

// readOnlySegment reports whether one simple command's words are a known read-only shape.
// After leading shell keywords and `NAME=value` assignments, it is true for a bare assignment,
// a program in readOnlyTools, git with a subcommand in readOnlyGit, no `--output` option, no `-c` or `--config-env` global option and, for grep, no `-O`,
// and `lyx fabric` with a verb in readOnlyFabric (`prune` and `cleanup` only without `--apply`).
// It is also true for `find` with no `-delete` or `-fprint`-family action whose every `-exec` command is read-only,
// for a wrapper (sudo, env, timeout, xargs) whose wrapped command is read-only,
// and for `bash -c` with a script mutatingCommand finds read-only.
// Every other shape is mutating: an unknown program, a program spelled through a variable or through a path outside a system bin directory, `bash` without `-c`, eval, source, sed, perl, awk, any other lyx verb.
func readOnlySegment(words []string) bool {
	for len(words) > 0 && (shellKeywords[words[0]] || (strings.Contains(words[0], "=") && !strings.HasPrefix(words[0], "-"))) {
		words = words[1:]
	}
	if len(words) == 0 {
		return true
	}
	prog, known := programName(words[0])
	if !known {
		return false
	}
	args := words[1:]
	switch {
	case readOnlyTools[prog]:
		return true
	case commandWrappers[prog]:
		for len(args) > 0 && strings.HasPrefix(args[0], "-") {
			args = args[1:]
		}
		return readOnlySegment(args)
	case prog == "timeout":
		for len(args) > 0 && strings.HasPrefix(args[0], "-") {
			args = args[1:]
		}
		if len(args) == 0 {
			return true
		}
		return readOnlySegment(args[1:])
	case prog == "xargs":
		for len(args) > 0 && strings.HasPrefix(args[0], "-") {
			if xargsValueOptions[args[0]] {
				args = args[1:]
			}
			if len(args) > 0 {
				args = args[1:]
			}
		}
		return readOnlySegment(args)
	case prog == "bash" || prog == "sh" || prog == "zsh" || prog == "dash":
		for i, a := range args {
			if strings.HasPrefix(a, "-") && !strings.HasPrefix(a, "--") && strings.Contains(a, "c") && i+1 < len(args) {
				return !mutatingCommand(args[i+1])
			}
		}
		return false
	case prog == "find":
		for i, a := range args {
			switch a {
			case "-delete", "-fprint", "-fprint0", "-fprintf", "-fls":
				return false
			case "-exec", "-execdir", "-ok", "-okdir":
				end := i + 1
				for end < len(args) && args[end] != ";" && args[end] != "+" {
					end++
				}
				if !readOnlySegment(args[i+1 : end]) {
					return false
				}
			}
		}
		return true
	case prog == "git":
		for _, a := range args {
			if a == "--output" || strings.HasPrefix(a, "--output=") {
				return false
			}
		}
		for i := 0; i < len(args); i++ {
			switch {
			case args[i] == "-c" || args[i] == "--config-env" || strings.HasPrefix(args[i], "--config-env="):
				// A config value can name a program git runs, such as core.fsmonitor or diff.external.
				return false
			case gitValueOptions[args[i]]:
				i++
			case strings.HasPrefix(args[i], "-"):
			default:
				return readOnlyGit[args[i]] && !(args[i] == "grep" && grepOpensPager(args[i+1:]))
			}
		}
		return false
	case prog == "lyx":
		if len(args) < 2 || args[0] != "fabric" {
			return false
		}
		verb := args[1]
		if verb == "prune" || verb == "cleanup" {
			for _, a := range args[2:] {
				if a == "--apply" {
					return false
				}
			}
			return true
		}
		return readOnlyFabric[verb]
	}
	return false
}

// grepOpensPager reports whether `git grep`'s args carry `-O` or `--open-files-in-pager`, which run a program on the matched files.
// An `O` anywhere in a short-option cluster counts, and so does any `--open` abbreviation.
func grepOpensPager(args []string) bool {
	for _, a := range args {
		if a == "--" {
			return false
		}
		if strings.HasPrefix(a, "--open") || (strings.HasPrefix(a, "-") && !strings.HasPrefix(a, "--") && strings.Contains(a, "O")) {
			return true
		}
	}
	return false
}

// programName returns the program name a simple command's first word runs, and whether that name can be trusted.
// A bare name is trusted.
// A path-spelled program is trusted only from a system bin directory, since any other path may be a script that shares an allowlisted name.
func programName(word string) (string, bool) {
	if !strings.Contains(word, "/") {
		return word, true
	}
	return path.Base(word), systemBinDirs[path.Dir(word)]
}

// ClassifyViolation assigns v its D4 severity, checking the correctness rule first.
// A fork-contract-write is correctness.
// A parent-write is correctness when its path lies under the run's state, reports or plan directory, or the run's `_lyx` directory (the parent of geom.WebsterDir),
// or when it lies inside the worktree and git does not ignore it; every other parent-write is policy.
// A fabric-reference is policy only when every simple command in its Command is a known read-only program with no write redirect (see mutatingCommand); every other fabric reference is correctness, since a rewrite of the fabric checkout can rewrite run state that a re-run of the cards' verify commands cannot detect.
// Every other class is policy.
// Prefix tests compare link-resolved paths, so a write spelled through a link to the run's `_lyx` still classes as correctness.
// The error return is only the git probe's or the link resolution's failure.
func ClassifyViolation(v AuditViolation, geom Geometry) (AuditSeverity, error) {
	switch v.Class {
	case ClassForkContractWrite:
		return AuditSeverityCorrectness, nil
	case ClassFabricReference:
		if mutatingCommand(v.Command) {
			return AuditSeverityCorrectness, nil
		}
		return AuditSeverityPolicy, nil
	case ClassParentWrite:
	default:
		return AuditSeverityPolicy, nil
	}

	written, err := canonicalPath(resolveWritePath(geom.WorktreeRoot, v.Path))
	if err != nil {
		return "", err
	}
	runDirs := []string{geom.WebsterDir, geom.ReportsDir, geom.PlanDir, filepath.Dir(geom.WebsterDir)}
	for _, dir := range runDirs {
		canon, err := canonicalPath(dir)
		if err != nil {
			return "", err
		}
		if pathWithin(canon, written) {
			return AuditSeverityCorrectness, nil
		}
	}

	worktree, err := canonicalPath(geom.WorktreeRoot)
	if err != nil {
		return "", err
	}
	if !pathWithin(worktree, written) {
		return AuditSeverityPolicy, nil
	}
	ignored, err := ignoredPath(geom.WorktreeRoot, written)
	if err != nil {
		return "", err
	}
	if ignored {
		return AuditSeverityPolicy, nil
	}
	return AuditSeverityCorrectness, nil
}

// pathWithin reports whether path is dir itself or lies beneath it; both must already be canonical.
func pathWithin(dir, path string) bool {
	rel, err := filepath.Rel(dir, path)
	if err != nil {
		return false
	}
	return rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)))
}

// canonicalPath resolves links in path through its nearest existing ancestor and re-joins the missing tail,
// so a path that does not exist yet still compares against a link-resolved directory.
func canonicalPath(path string) (string, error) {
	cleaned := filepath.Clean(path)
	var tail []string
	cur := cleaned
	for {
		resolved, err := filepath.EvalSymlinks(cur)
		if err == nil {
			for i := len(tail) - 1; i >= 0; i-- {
				resolved = filepath.Join(resolved, tail[i])
			}
			return resolved, nil
		}
		if !errors.Is(err, os.ErrNotExist) {
			return "", fmt.Errorf("websterengine: resolve links in %s: %w", cur, err)
		}
		parent := filepath.Dir(cur)
		if parent == cur {
			return cleaned, nil
		}
		tail = append(tail, filepath.Base(cur))
		cur = parent
	}
}

// ForkWarnings evaluates f for webster's warning-only (never round-failing) classes: a fork that
// never returned a final report is sloppiness no mechanism can prevent in advance (the fork ran
// clean but never delivered its findings), so it is collected as a warning rather than a hard
// violation — the same posture burlerengine's auditClusterRound takes for its own ReportReturned ==
// false case.
func ForkWarnings(f shuttleengine.ForkReport) []string {
	if !f.ReportReturned {
		return []string{fmt.Sprintf("fork %q never returned a final report", f.TranscriptPath)}
	}
	return nil
}

// ErrNoForkTranscripts is the sentinel ClassifyAttribution's zero-new-transcript case wraps.
// record-batch's caller (batch 5) issues this as its own hard error AFTER SettleRetry's settle
// window is exhausted — never on the first miss, since a fork's transcript file may not have
// flushed to disk yet the instant the Agent tool call returns (the discussion's "first miss is
// inconclusive" flush-timing caveat).
// A report file existing alongside zero new transcripts does NOT save the batch from this error: a
// report with no fork behind it means Master wrote it itself, which is exactly the defect this
// check exists to catch (pinned check order: transcript count is decided BEFORE report presence).
// A report on disk with no fork transcript is a state a forged report produces, but ALSO a legitimate cross-machine resume of the report-landed-before-record-batch crash window, since fork transcripts live under the machine-local ~/.claude projects dir while state.json and reports are fabric-synced (found live in crucible round fable-r1).
// RecordBatch does not leave the batch wedged on it: it archives the report, keeps the batch record begun, and returns a *ReportArchivedError whose way forward is `begin-batch`, which re-drives the batch with the report path free.
var ErrNoForkTranscripts = errors.New("zero new fork transcripts since the previous batch boundary — the batch was never forked (or its transcript is not on this machine: fork transcripts are machine-local, so a crash window resumed on a different machine cannot re-attribute its report)")

// DefaultSettleWindow is SettleRetry's recommended total wait budget before its caller gives up and
// treats a zero-transcript result as final: a few seconds is enough slack for Claude Code to flush
// a just-returned fork's subagents/<id>.jsonl to disk without meaningfully slowing down a normal
// record-batch call (which almost always finds its transcript on the first scan).
const DefaultSettleWindow = 3 * time.Second

// DefaultSettleTick is SettleRetry's recommended re-scan interval within the settle window:
// frequent enough that a normal (non-degenerate) call resolves within one or two ticks of the
// transcript actually appearing.
const DefaultSettleTick = 250 * time.Millisecond

// Sleeper abstracts time.Sleep so SettleRetry's wait loop never blocks for real under test — a
// recording fake Sleeper lets a test assert exactly how many ticks were requested and drive
// SettleRetry's retry loop to completion instantly, the same clock seam pattern
// shuttleengine's wait.go already establishes for the same reason.
type Sleeper interface {
	// Sleep blocks (in production) or records a request to block (under test)
	// for d.
	Sleep(d time.Duration)
}

// NewTranscripts returns the ForkReport entries in audit whose TranscriptPath is NOT in seen.
// It is a defensive re-filter for callers that may not have used AuditForksIncremental or that are
// re-deriving after a settle retry.
func NewTranscripts(audit shuttleengine.ForkAudit, seen []string) []shuttleengine.ForkReport {
	seenSet := make(map[string]bool, len(seen))
	for _, path := range seen {
		seenSet[path] = true
	}

	var newReports []shuttleengine.ForkReport
	for _, f := range audit.Forks {
		if !seenSet[f.TranscriptPath] {
			newReports = append(newReports, f)
		}
	}
	return newReports
}

// SettleRetry re-invokes fetch on tick's cadence, sleeping via s between attempts, until at least
// one transcript new since seen appears or window elapses — implementing the flush-timing de-risk
// from discussion.md's fork-audit-policy decision ("first miss is inconclusive"): the
// zero-transcript hard error is only ever issued by the CALLER, and only after SettleRetry itself
// has exhausted the settle window, never on this function's first fetch.
// SettleRetry returns as soon as fetch reports one or more new transcripts — it never sleeps out
// the rest of window once it has an answer.
// A fetch error propagates immediately: an audit read that itself failed has nothing safe to retry
// against.
// When window elapses with zero new transcripts, SettleRetry returns the last fetched audit, a nil
// (or empty) newReports slice, and a nil error — it is the caller's job (see ClassifyAttribution)
// to turn that empty result into ErrNoForkTranscripts.
func SettleRetry(
	fetch func() (shuttleengine.ForkAudit, error),
	seen []string,
	window time.Duration,
	tick time.Duration,
	s Sleeper,
) (shuttleengine.ForkAudit, []shuttleengine.ForkReport, error) {
	var elapsed time.Duration

	for {
		audit, err := fetch()
		if err != nil {
			return shuttleengine.ForkAudit{}, nil, err
		}

		newReports := NewTranscripts(audit, seen)
		if len(newReports) > 0 {
			return audit, newReports, nil
		}

		if elapsed >= window {
			return audit, newReports, nil
		}

		s.Sleep(tick)
		elapsed += tick
	}
}

// ClassifyAttribution pins the fork-audit-policy decision's check order over
// newReports, the transcripts SettleRetry (or a direct NewTranscripts call)
// determined are new since the previous batch boundary:
//
//  1. Zero new transcripts: hard error (ErrNoForkTranscripts), REGARDLESS of
//     whether a batch report file exists — a report with no fork behind it means
//     Master wrote it itself. Transcript-count-before-report-presence is what
//     makes that defect unfakeable; the caller must check this BEFORE it even
//     looks for a report file.
//  2. Exactly one new transcript: clean — the normal case. Returns ("", nil).
//  3. More than one new transcript: warning only, never hard — a fork whose
//     Agent call errored mid-flight followed by a direct re-fork, with no
//     record-batch call in between, is legitimate retry behavior, not a defect.
func ClassifyAttribution(newReports []shuttleengine.ForkReport) (warning string, err error) {
	switch len(newReports) {
	case 0:
		return "", ErrNoForkTranscripts
	case 1:
		return "", nil
	default:
		return fmt.Sprintf(
			"%d new fork transcripts since the previous batch boundary — expected exactly one; treating as a fork-error-then-re-fork with no intervening record-batch call",
			len(newReports),
		), nil
	}
}
