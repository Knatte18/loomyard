// reconcile.go implements the runtime read path (Read) and the once-per-process seed and refresh pass
// (Reconcile, ForceRefresh), with the drift warnings that accompany it.
// Drift notices are log lines only; they never fail a pass.

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

// gitattributesName is the file under baseDir that keeps a checked-out stencil from arriving CRLF-converted.
// It keeps the stencil's line endings as shipped, whatever the operator's core.autocrlf setting.
const gitattributesName = ".gitattributes"

// gitattributesContent is the single line written into a fresh .gitattributes.
const gitattributesContent = "*.md text eol=lf\n"

// Read reads name's current content from baseDir with a plain os.ReadFile on every call.
// It does no caching, so an on-disk edit takes effect on the next Read.
// It never falls back to a shipped default and never writes.
// A missing file is reported as an error, not silently substituted.
func Read(baseDir, name string) ([]byte, error) {
	content, err := os.ReadFile(Path(baseDir, name))
	if err != nil {
		return nil, fmt.Errorf("stencilstore: read stencil %q from %s: %w", name, baseDir, err)
	}
	return content, nil
}

// BuildAncestry is the state of a worktree's HEAD relative to the commit the running binary was built from.
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

// Reconcile is the once-per-process seed and refresh pass.
// For every registry name it reads the on-disk file and classifies it against the shipped default.
// It writes or restamps the file as the mode and the recorded writer allow.
// It also seeds baseDir/.gitattributes when absent.
// When source.Dir is non-empty, it warns on each board copy that drifts from the worktree source.
// It consults source.Build only for a stencil the source is ahead on.
// It returns the baseDir-relative, slash-separated paths it wrote, in registry.Names() order.
// When every file is already correct, it writes nothing.
func Reconcile(baseDir string, registry Registry, mode Mode, source Source) ([]string, error) {
	return reconcileAll(baseDir, registry, mode, source, false)
}

// WritesDue reports whether a pass with these arguments would write any file.
// It writes and logs nothing itself.
func WritesDue(baseDir string, registry Registry, mode Mode, source Source) (bool, error) {
	due, err := pendingWrites(baseDir, registry, mode, source)
	return len(due) > 0, err
}

// reconcileAll walks the registry, writes each stencil that is due, and seeds .gitattributes.
// When force is set, an untouched stencil is refreshed whatever the mode or the recorded writer's ordering.
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

// pendingWrites reports the paths a reconcile pass would write, without writing or logging anything.
// Each path is baseDir-relative and slash-separated.
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

// noticeCase is why a stencil was left untouched; each refresh notice carries one.
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

// refreshNotice is one entry a decision owes the log.
// It names the stencil, the case that left it untouched, and the recorded and running revisions if any.
// The zero value owes nothing.
type refreshNotice struct {
	stencil  string
	kind     noticeCase
	recorded string
	running  string
}

// decision is the outcome of classifying one registry name.
// It holds the action, the content a restamp writes, and the notice owed.
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

// decide works out what one classified registry name needs, without touching disk or the log.
// When force is set, an untouched stencil is refreshed whatever the mode or ordering.
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

// reconcileOne applies one classified registry name's state to disk.
// It reports whether it wrote the file, and the notice the decision owes.
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

// logRefreshNotices emits one log line per notice case, listing the stencils that case left untouched.
// Each warning names its remedy, because it reports that producers will read the older on-disk text.
// A dev or unstamped build refuses to refresh,
// so it never overwrites a board's stencils with whatever a working tree holds.
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
			logger.Info("stencilstore: board copies were written by a build that is not older than this binary; left untouched -- run \"lym stencil sync\" to override", "stencils", stencils, "running", running)
		case noticeDevBuild:
			logger.Warn("stencilstore: dev build does not refresh untouched stencils; producers will read the OLDER on-disk copies -- run \"lym stencil sync\" to force-refresh them", "stencils", stencils)
		case noticeUnstampedBuild:
			logger.Warn("stencilstore: unstamped build does not refresh untouched stencils; producers will read the OLDER on-disk copies -- run \"lym stencil sync\", or deploy with update-plugins.sh", "stencils", stencils)
		case noticeEditedBehind:
			logger.Warn("stencilstore: edited stencils have fallen behind a newer shipped default; see lym stencil diff", "stencils", stencils)
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

// seedGitattributes writes baseDir/.gitattributes with gitattributesContent when the file is absent.
// It reports whether it wrote the file.
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

// driftClass names which of the board copy, the source, and the embedded bytes changed.
// The port-back drift warning recommends only the remedy that class allows.
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
	// driftHandEdited: the board body no longer matches its own stamp,
	// and the source matches the embedded bytes this binary carries.
	driftHandEdited driftClass = iota
	// driftSourceAhead: the board copy is untouched, and the source differs from the embedded bytes.
	driftSourceAhead
	// driftBoth: the board copy was hand-edited and the source differs from the embedded bytes.
	driftBoth
	// driftNeither: the board copy is untouched and the source equals the embedded bytes,
	// so the board copy is merely older than what this binary carries.
	driftNeither
	// driftBehind: the board copy is untouched and the source differs from the embedded bytes.
	// The worktree also lacks the build commit that deployed the board copy.
	driftBehind driftClass = 4
)

// driftMessagePrefix opens every drift warning message; the rest of the message is the remedy.
const driftMessagePrefix = "stencilstore: board copy has drifted from worktree source; "

// classifyPortBackDrift classifies a differing board copy on two signals:
// whether the copy is hand-edited against the embedded bytes, and whether the source body differs from them.
// A source-ahead copy calls build, once, to tell a worktree behind the build from one ahead of the deployed binary.
// A nil build counts as unknown.
// It returns the class and the warning's message, which names the remedy that class allows.
func classifyPortBackDrift(boardContent, sourceContent, embedded []byte, build func() BuildAncestry) (driftClass, string) {
	handEdited := Classify(boardContent, true, embedded) == StateEdited
	sourceAhead := BodyHash(sourceContent) != BodyHash(embedded)

	switch {
	case handEdited && sourceAhead:
		return driftBoth, driftMessagePrefix + "both sides changed and promote would overwrite the source's changes -- reconcile by hand"
	case handEdited:
		return driftHandEdited, driftMessagePrefix + "it was hand-edited -- run \"lym stencil promote <name>\" to port it back"
	case sourceAhead && build != nil && build() == BuildNotInHead:
		return driftBehind, driftMessagePrefix + "this worktree is behind the build that deployed the board copy -- syncing this worktree with main resolves it"
	case sourceAhead:
		return driftSourceAhead, driftMessagePrefix + "the board copy is untouched and a binary older than the source deployed it -- run a production deploy (update-plugins.sh)"
	default:
		return driftNeither, driftMessagePrefix + "the board copy is untouched but older than this binary's embedded bytes (a dev build never refreshes one) -- run \"lym stencil sync\""
	}
}

// warnPortBackDrift compares each registry name's on-disk board copy with the worktree copy under source.Dir.
// It logs one line for the pass, counting the drifted stencils and naming each with its drift class.
// The line also states each remedy those classes allow, once.
// It logs at Info when every drifted stencil is behind, since a sync clears that; otherwise it logs at Warn.
// A missing source file is skipped silently.
// The comparison never returns an error and never affects an exit code.
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

// ForceRefresh refreshes an untouched stencil in every mode, whatever the recorded writer's ordering.
// It is the entry point `lym stencil sync` calls, so an explicit sync refreshes even from a -dev-stamped binary.
// mode only decides whether the running binary is recorded as the writer.
// ModeProduction records it; any other mode leaves no recorded revision.
func ForceRefresh(baseDir string, registry Registry, mode Mode, source Source) ([]string, error) {
	return reconcileAll(baseDir, registry, mode, source, true)
}
