// reconcile.go implements the runtime read path (Read), the once-per-process seed/refresh pass
// (Reconcile, ForceRefresh), and the drift-notification warnings that accompany it.
// Reconcile never blocks and never returns a non-zero-affecting error for a drift signal.
// Its drift notices are log lines only, per the seeding-trigger Shared Decision.
// The Source it is given carries the worktree's stencil directory and, optionally, its build-ancestry read.

package stencilstore

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/stencil"
)

// gitattributesName is the file Reconcile seeds under baseDir so a checked-out stencil never
// arrives CRLF-converted regardless of the operator's core.autocrlf setting.
const gitattributesName = ".gitattributes"

// gitattributesContent is the single line Reconcile writes into a fresh .gitattributes.
const gitattributesContent = "*.md text eol=lf\n"

// Read reads name's current content from baseDir with a plain os.ReadFile on every call -- no
// caching, so an on-disk edit takes effect immediately on the next Read.
// It never falls back to a shipped default and never writes; a missing file is reported as an
// error, not silently substituted, per the missing-board-is-a-hard-error Shared Decision.
func Read(baseDir, name string) ([]byte, error) {
	content, err := os.ReadFile(Path(baseDir, name))
	if err != nil {
		return nil, fmt.Errorf("stencilstore: read stencil %q from %s: %w", name, baseDir, err)
	}
	return content, nil
}

// BuildAncestry reports whether a worktree's HEAD holds the commit the running binary was built from.
type BuildAncestry int

const (
	// BuildAncestryUnknown: the binary carries no VCS revision, or reading the ancestry failed.
	BuildAncestryUnknown BuildAncestry = iota
	// BuildInHead: the worktree's HEAD holds the binary's build commit.
	BuildInHead BuildAncestry = 1
	// BuildNotInHead: the worktree's HEAD lacks the binary's build commit.
	BuildNotInHead BuildAncestry = 2
)

// Ordering is the caller's answer to whether a recorded writer's build is older than the running binary.
type Ordering int

const (
	// OrderingUnknown: the ordering could not be read.
	OrderingUnknown Ordering = 0
	// RecordedOlder: the recorded writer's build is older than the running binary.
	RecordedOlder Ordering = 1
	// RecordedNotOlder: the recorded writer's build is the running binary or newer than it.
	RecordedNotOlder Ordering = 2
)

// Source names the worktree a seed pass compares the board copies against, and the binary running it.
// Dir is the worktree's stencil source tree; empty names none and keeps the drift warning silent.
// Build reports the worktree's BuildAncestry; nil means unknown.
// Writer is the running binary, recorded into the banner of every ModeProduction write.
// Older orders a recorded writer against the running binary; nil answers OrderingUnknown.
// The store runs no git itself, so the caller supplies Build and Older.
type Source struct {
	Dir    string
	Build  func() BuildAncestry
	Writer Writer
	Older  func(recorded Writer) Ordering
}

// Reconcile is the once-per-process seed/refresh pass: for every name in registry.Names() it reads
// the on-disk file, classifies it against the registry's shipped default, and acts per the
// edit-detection table (see Classify) and Mode.
// It also seeds baseDir/.gitattributes when absent, and, when source.Dir is non-empty, warns on any
// board-copy-vs-worktree-source drift, consulting source.Build only for a stencil the source is ahead on.
// It returns the baseDir-relative, slash-separated paths it actually wrote, in registry.Names()
// order, and writes nothing at all when every file is already correct.
func Reconcile(baseDir string, registry Registry, mode Mode, source Source) ([]string, error) {
	return reconcileAll(baseDir, registry, mode, source, false)
}

// WritesDue reports whether Reconcile under the same arguments would write any file, writing and logging nothing itself.
func WritesDue(baseDir string, registry Registry, mode Mode, source Source) (bool, error) {
	due, err := pendingWrites(baseDir, registry, mode, source)
	return len(due) > 0, err
}

// reconcileAll is the shared walk behind Reconcile and ForceRefresh; force performs the refresh row whatever the mode or ordering.
func reconcileAll(baseDir string, registry Registry, mode Mode, source Source, force bool) ([]string, error) {
	var written []string
	var notices []refreshNotice
	// The notices are logged on every return, so a pass that fails midway still reports the stencils it left untouched.
	defer func() { logRefreshNotices(notices, source.Writer.Revision) }()

	for _, name := range registry.Names() {
		shipped, known := registry.Default(name)
		if !known {
			continue
		}

		path := Path(baseDir, name)
		onDisk, readErr := os.ReadFile(path)
		exists := readErr == nil
		if readErr != nil && !os.IsNotExist(readErr) {
			return written, fmt.Errorf("stencilstore: reconcile stencil %q: %w", name, readErr)
		}

		state := Classify(onDisk, exists, shipped)
		wrote, notice, writeErr := reconcileOne(path, name, state, onDisk, shipped, mode, source, force)
		if notice.kind != noNotice {
			notices = append(notices, notice)
		}
		if writeErr != nil {
			return written, writeErr
		}
		if wrote {
			written = append(written, filepath.ToSlash(RelPath(name)))
		}
	}

	wroteAttrs, err := seedGitattributes(baseDir)
	if err != nil {
		return written, err
	}
	if wroteAttrs {
		written = append(written, gitattributesName)
	}

	if source.Dir != "" {
		warnPortBackDrift(baseDir, registry, source)
	}

	return written, nil
}

// pendingWrites walks the registry and .gitattributes like reconcileAll, applying the same decision to each file but writing and logging nothing.
// It returns the baseDir-relative, slash-separated paths a reconcile would write.
func pendingWrites(baseDir string, registry Registry, mode Mode, source Source) ([]string, error) {
	var due []string

	for _, name := range registry.Names() {
		shipped, known := registry.Default(name)
		if !known {
			continue
		}

		onDisk, readErr := os.ReadFile(Path(baseDir, name))
		exists := readErr == nil
		if readErr != nil && !os.IsNotExist(readErr) {
			return due, fmt.Errorf("stencilstore: reconcile stencil %q: %w", name, readErr)
		}

		if decide(name, Classify(onDisk, exists, shipped), onDisk, shipped, mode, source, false).kind != actionNone {
			due = append(due, filepath.ToSlash(RelPath(name)))
		}
	}

	if _, err := os.Stat(filepath.Join(baseDir, gitattributesName)); os.IsNotExist(err) {
		due = append(due, gitattributesName)
	} else if err != nil {
		return due, fmt.Errorf("stencilstore: stat %s: %w", filepath.Join(baseDir, gitattributesName), err)
	}

	return due, nil
}

// actionKind is what one registry name's decision does to its file.
type actionKind int

const (
	actionNone actionKind = iota
	// actionWrite writes the shipped default, stamped.
	actionWrite
	// actionRestamp rewrites the on-disk body under a fresh stamp.
	actionRestamp
)

// noticeCase is why a stencil was left untouched, one of the four cases a refresh notice reports.
type noticeCase int

const (
	noNotice noticeCase = iota
	// noticeNotOlder: a production build found the recorded writer not older than itself.
	noticeNotOlder
	// noticeDevBuild: a dev build never refreshes an untouched stencil.
	noticeDevBuild
	// noticeUnstampedBuild: an unstamped build never refreshes an untouched stencil.
	noticeUnstampedBuild
	// noticeEditedBehind: an edited stencil has fallen behind a newer shipped default.
	noticeEditedBehind
)

// refreshNotice is what a decision owes the log: the stencil, the case that left it untouched, and the recorded and running revisions where the case has them.
// The zero value owes nothing.
type refreshNotice struct {
	stencil  string
	kind     noticeCase
	recorded string
	running  string
}

// decision is the outcome of classifying one registry name: the action, the content a restamp writes, and the notice the decision owes.
type decision struct {
	kind    actionKind
	restamp []byte
	notice  refreshNotice
}

// writerFor returns the writer a write records: the running binary under ModeProduction, and none under any other mode.
func writerFor(mode Mode, source Source) Writer {
	if mode == ModeProduction {
		return source.Writer
	}
	return Writer{}
}

// decide applies Reconcile's per-row requirements to one classified registry name without touching disk or the log.
// force performs the refresh row on an untouched stencil whatever the mode or ordering.
func decide(name string, state State, onDisk, shipped []byte, mode Mode, source Source, force bool) decision {
	switch state {
	case StateAbsent:
		return decision{kind: actionWrite}

	case StateUntouched:
		if BodyHash(shipped) == BodyHash(onDisk) {
			return decision{}
		}
		if force {
			return decision{kind: actionWrite}
		}
		switch mode {
		case ModeProduction:
			recorded := ParseWriter(onDisk)
			if recorded.Revision == "" {
				return decision{kind: actionWrite}
			}
			ordering := OrderingUnknown
			if source.Older != nil {
				ordering = source.Older(recorded)
			}
			if ordering == RecordedOlder {
				return decision{kind: actionWrite}
			}
			return decision{notice: refreshNotice{stencil: name, kind: noticeNotOlder, recorded: recorded.Revision, running: source.Writer.Revision}}
		case ModeDev:
			return decision{notice: refreshNotice{stencil: name, kind: noticeDevBuild}}
		default:
			return decision{notice: refreshNotice{stencil: name, kind: noticeUnstampedBuild}}
		}

	case StateReconciled:
		restamped := ApplyWriter(ApplyStamp(onDisk, BodyHash(onDisk)), writerFor(mode, source))
		if string(restamped) == string(onDisk) {
			return decision{}
		}
		return decision{kind: actionRestamp, restamp: restamped}

	case StateEdited:
		if BodyHash(shipped) != BodyHash(onDisk) {
			return decision{notice: refreshNotice{stencil: name, kind: noticeEditedBehind}}
		}
		return decision{}

	default:
		return decision{}
	}
}

// reconcileOne applies one registry name's classified state to disk, per Reconcile's per-row
// requirements, and reports whether it wrote the file and the notice the decision owes.
// The caller logs the notice, so a pass reports each case once.
func reconcileOne(path, name string, state State, onDisk, shipped []byte, mode Mode, source Source, force bool) (bool, refreshNotice, error) {
	if state < StateAbsent || state > StateEdited {
		return false, refreshNotice{}, fmt.Errorf("stencilstore: stencil %q classified as unknown state %v", name, state)
	}

	d := decide(name, state, onDisk, shipped, mode, source, force)

	switch d.kind {
	case actionWrite:
		if err := writeStamped(path, shipped, BodyHash(shipped), writerFor(mode, source)); err != nil {
			return false, d.notice, err
		}
		return true, d.notice, nil
	case actionRestamp:
		if err := os.WriteFile(path, d.restamp, 0o644); err != nil {
			return false, d.notice, fmt.Errorf("stencilstore: restamp stencil %q: %w", name, err)
		}
		return true, d.notice, nil
	default:
		return false, d.notice, nil
	}
}

// logRefreshNotices emits one line per notice case, listing the stencils the case left untouched and naming its remedy once.
// The remedy is named at the point of failure rather than left for a reader to find.
// Without it these warnings read as benign housekeeping while they report that every producer reading the stencil will run on the OLDER on-disk text.
// A dev or unstamped build refuses to refresh so it never clobbers a board's stencils with whatever is in a working tree.
func logRefreshNotices(notices []refreshNotice, running string) {
	for _, kind := range []noticeCase{noticeNotOlder, noticeDevBuild, noticeUnstampedBuild, noticeEditedBehind} {
		var listed []string
		for _, notice := range notices {
			if notice.kind != kind {
				continue
			}
			entry := notice.stencil
			if kind == noticeNotOlder {
				entry += " (recorded " + notice.recorded + ")"
			}
			listed = append(listed, entry)
		}
		if len(listed) == 0 {
			continue
		}
		stencils := strings.Join(listed, ", ")
		switch kind {
		case noticeNotOlder:
			logger.Info("stencilstore: board copies were written by a build that is not older than this binary; left untouched -- run \"lyx stencil sync\" to override", "stencils", stencils, "running", running)
		case noticeDevBuild:
			logger.Warn("stencilstore: dev build does not refresh untouched stencils; producers will read the OLDER on-disk copies -- run \"lyx stencil sync\" to force-refresh them", "stencils", stencils)
		case noticeUnstampedBuild:
			logger.Warn("stencilstore: unstamped build does not refresh untouched stencils; producers will read the OLDER on-disk copies -- run \"lyx stencil sync\", or deploy with update-plugins.sh", "stencils", stencils)
		case noticeEditedBehind:
			logger.Warn("stencilstore: edited stencils have fallen behind a newer shipped default; see lyx stencil diff", "stencils", stencils)
		}
	}
}

// writeStamped writes shipped, stamped with hash and recording writer, to path, creating parent directories as needed.
func writeStamped(path string, shipped []byte, hash string, writer Writer) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("stencilstore: create parent directory for %s: %w", path, err)
	}
	if err := os.WriteFile(path, ApplyWriter(ApplyStamp(shipped, hash), writer), 0o644); err != nil {
		return fmt.Errorf("stencilstore: write stencil at %s: %w", path, err)
	}
	return nil
}

// seedGitattributes writes baseDir/.gitattributes with gitattributesContent when it does not yet
// exist, and reports whether it wrote the file.
// It never rewrites an existing .gitattributes.
func seedGitattributes(baseDir string) (bool, error) {
	path := filepath.Join(baseDir, gitattributesName)
	if _, err := os.Stat(path); err == nil {
		return false, nil
	} else if !os.IsNotExist(err) {
		return false, fmt.Errorf("stencilstore: stat %s: %w", path, err)
	}

	if err := os.MkdirAll(baseDir, 0o755); err != nil {
		return false, fmt.Errorf("stencilstore: create stencils directory %s: %w", baseDir, err)
	}
	if err := os.WriteFile(path, []byte(gitattributesContent), 0o644); err != nil {
		return false, fmt.Errorf("stencilstore: write %s: %w", path, err)
	}
	return true, nil
}

// driftClass names which sides of a board/source/embedded triple changed, and so which remedy the
// port-back drift warning may recommend.
type driftClass int

// String names the class for the warning's structured "class" attribute.
func (c driftClass) String() string {
	switch c {
	case driftHandEdited:
		return "hand-edited"
	case driftSourceAhead:
		return "source-ahead"
	case driftBoth:
		return "both"
	case driftBehind:
		return "behind"
	default:
		return "neither"
	}
}

const (
	// driftHandEdited: the board body no longer matches its own stamp, and the source matches the
	// embedded bytes this binary carries.
	driftHandEdited driftClass = iota
	// driftSourceAhead: the board copy is untouched, and the source differs from the embedded bytes.
	driftSourceAhead
	// driftBoth: the board copy was hand-edited and the source differs from the embedded bytes.
	driftBoth
	// driftNeither: the board copy is untouched and the source equals the embedded bytes, so the
	// board copy is merely older than what this binary carries.
	driftNeither
	// driftBehind: the board copy is untouched and the source differs from the embedded bytes.
	// The worktree also lacks the build commit that deployed the board copy.
	driftBehind driftClass = 4
)

// driftMessagePrefix opens every message classifyPortBackDrift returns; the rest of the message is the remedy.
const driftMessagePrefix = "stencilstore: board copy has drifted from worktree source; "

// classifyPortBackDrift classifies a differing board copy on the two signals the warning turns on:
// hand-edited (Classify reports StateEdited against the embedded bytes) and source-ahead (the source
// body differs from the embedded body).
// A source-ahead copy calls build, once, to tell a worktree behind the build from one genuinely ahead of the deployed binary.
// A nil build counts as unknown.
// It returns the class and the warning's message, which names the remedy that class allows.
func classifyPortBackDrift(boardContent, sourceContent, embedded []byte, build func() BuildAncestry) (driftClass, string) {
	handEdited := Classify(boardContent, true, embedded) == StateEdited
	sourceAhead := BodyHash(sourceContent) != BodyHash(embedded)

	switch {
	case handEdited && sourceAhead:
		return driftBoth, driftMessagePrefix + "both sides changed and promote would overwrite the source's changes -- reconcile by hand"
	case handEdited:
		return driftHandEdited, driftMessagePrefix + "it was hand-edited -- run \"lyx stencil promote <name>\" to port it back"
	case sourceAhead && build != nil && build() == BuildNotInHead:
		return driftBehind, driftMessagePrefix + "this worktree is behind the build that deployed the board copy -- syncing this worktree with main resolves it"
	case sourceAhead:
		return driftSourceAhead, driftMessagePrefix + "the board copy is untouched and a binary older than the source deployed it -- run a production deploy (update-plugins.sh)"
	default:
		return driftNeither, driftMessagePrefix + "the board copy is untouched but older than this binary's embedded bytes (a dev build never refreshes one) -- run \"lyx stencil sync\""
	}
}

// warnPortBackDrift compares each registry name's on-disk board copy against source.Dir's worktree copy.
// It emits one log line for the pass, naming the count, each differing stencil with its drift class and, once, the remedies those classes allow (see classifyPortBackDrift).
// The line logs at Info when every differing stencil is in the behind class, since a sync clears it; otherwise it logs at Warn.
// A missing source file is skipped silently; this comparison never returns an error and never
// affects an exit code, per the drift-notification-is-logger-warn-and-never-blocks Shared Decision.
func warnPortBackDrift(baseDir string, registry Registry, source Source) {
	var drifted []string
	var remedies []string
	onlyBehind := true
	for _, name := range registry.Names() {
		boardPath := Path(baseDir, name)
		boardContent, err := os.ReadFile(boardPath)
		if err != nil {
			continue
		}

		sourcePath := filepath.Join(source.Dir, filepath.FromSlash(RelPath(name)))
		sourceContent, err := os.ReadFile(sourcePath)
		if err != nil {
			continue
		}

		boardBody := NormalizeLF([]byte(stencil.StripLeadingComment(string(boardContent))))
		sourceBody := NormalizeLF([]byte(stencil.StripLeadingComment(string(sourceContent))))
		if string(boardBody) != string(sourceBody) {
			embedded, _ := registry.Default(name)
			class, msg := classifyPortBackDrift(boardContent, sourceContent, embedded, source.Build)
			drifted = append(drifted, name+": "+class.String())
			if remedy := strings.TrimPrefix(msg, driftMessagePrefix); !slices.Contains(remedies, remedy) {
				remedies = append(remedies, remedy)
			}
			onlyBehind = onlyBehind && class == driftBehind
		}
	}
	if len(drifted) == 0 {
		return
	}
	noun := "stencils"
	if len(drifted) == 1 {
		noun = "stencil"
	}
	line := fmt.Sprintf("stencilstore: %d %s drifted from worktree source: %s -- %s", len(drifted), noun, strings.Join(drifted, ", "), strings.Join(remedies, "; "))
	if onlyBehind {
		logger.Info(line)
		return
	}
	logger.Warn(line)
}

// ForceRefresh performs the refresh row even on a stencil a ModeDev or ModeUnstamped pass would leave untouched, whatever the recorded writer's ordering.
// It is the entry point `lyx stencil sync` calls, which is why an explicit sync refreshes even from
// a -dev-stamped binary.
// mode only decides whether the running binary is recorded as the writer: ModeProduction records it, any other mode leaves no recorded revision.
func ForceRefresh(baseDir string, registry Registry, mode Mode, source Source) ([]string, error) {
	return reconcileAll(baseDir, registry, mode, source, true)
}
