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

// Source names the worktree a seed pass compares the board copies against.
// Dir is the worktree's stencil source tree; empty names none and keeps the drift warning silent.
// Build reports the worktree's BuildAncestry; nil means unknown.
// The store runs no git itself, so the caller supplies Build.
type Source struct {
	Dir   string
	Build func() BuildAncestry
}

// Reconcile is the once-per-process seed/refresh pass: for every name in registry.Names() it reads
// the on-disk file, classifies it against the registry's shipped default, and acts per the
// edit-detection table (see Classify) and dev/prod Mode.
// It also seeds baseDir/.gitattributes when absent, and, when source.Dir is non-empty, warns on any
// board-copy-vs-worktree-source drift, consulting source.Build only for a stencil the source is ahead on.
// It returns the baseDir-relative, slash-separated paths it actually wrote, in registry.Names()
// order, and writes nothing at all when every file is already correct.
func Reconcile(baseDir string, registry Registry, mode Mode, source Source) ([]string, error) {
	var written []string

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
		wrote, writeErr := reconcileOne(path, name, state, onDisk, shipped, mode)
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

// reconcileOne applies one registry name's classified state to disk, per Reconcile's per-row
// requirements, and reports whether it wrote the file.
func reconcileOne(path, name string, state State, onDisk, shipped []byte, mode Mode) (bool, error) {
	switch state {
	case StateAbsent:
		if err := writeStamped(path, shipped, BodyHash(shipped)); err != nil {
			return false, err
		}
		return true, nil

	case StateUntouched:
		if BodyHash(shipped) == BodyHash(onDisk) {
			return false, nil
		}
		if mode == ModeDev {
			// The remedy is named at the point of failure rather than left for a reader to find,
			// because without it this warning reads as benign housekeeping while it is in fact
			// reporting that every producer reading this stencil will run on the OLDER on-disk text.
			// A dev build refuses to refresh so it never clobbers a board's stencils with whatever
			// is in a working tree, and the refusal is one-way: an older installed binary running in
			// prod mode DOES refresh, so it can downgrade a board's stencils and the newer dev build
			// can then only warn about it, on every single invocation, forever.
			logger.Warn("stencilstore: dev build does not refresh an untouched stencil; producers will read the OLDER on-disk copy -- run \"lyx stencil sync\" to force-refresh it", "stencil", name, "path", path)
			return false, nil
		}
		if err := writeStamped(path, shipped, BodyHash(shipped)); err != nil {
			return false, err
		}
		return true, nil

	case StateReconciled:
		restamped := ApplyStamp(onDisk, BodyHash(onDisk))
		if string(restamped) == string(onDisk) {
			return false, nil
		}
		if err := os.WriteFile(path, restamped, 0o644); err != nil {
			return false, fmt.Errorf("stencilstore: restamp stencil %q: %w", name, err)
		}
		return true, nil

	case StateEdited:
		if BodyHash(shipped) != BodyHash(onDisk) {
			logger.Warn("stencilstore: edited stencil has fallen behind a newer shipped default; see lyx stencil diff", "stencil", name)
		}
		return false, nil

	default:
		return false, fmt.Errorf("stencilstore: stencil %q classified as unknown state %v", name, state)
	}
}

// writeStamped writes shipped, stamped with hash, to path, creating parent directories as needed.
func writeStamped(path string, shipped []byte, hash string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("stencilstore: create parent directory for %s: %w", path, err)
	}
	if err := os.WriteFile(path, ApplyStamp(shipped, hash), 0o644); err != nil {
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
		return driftBoth, "stencilstore: board copy has drifted from worktree source; both sides changed and promote would overwrite the source's changes -- reconcile by hand"
	case handEdited:
		return driftHandEdited, "stencilstore: board copy has drifted from worktree source; it was hand-edited -- run \"lyx stencil promote <name>\" to port it back"
	case sourceAhead && build != nil && build() == BuildNotInHead:
		return driftBehind, "stencilstore: board copy has drifted from worktree source; this worktree is behind the build that deployed the board copy -- syncing this worktree with main resolves it"
	case sourceAhead:
		return driftSourceAhead, "stencilstore: board copy has drifted from worktree source; the board copy is untouched and a binary older than the source deployed it -- run a production deploy (update-plugins.sh)"
	default:
		return driftNeither, "stencilstore: board copy has drifted from worktree source; the board copy is untouched but older than this binary's embedded bytes (a dev build never refreshes one) -- run \"lyx stencil sync\""
	}
}

// warnPortBackDrift compares each registry name's on-disk board copy against source.Dir's worktree
// copy and emits one log line per differing stencil, naming the stencil, its drift class and
// the remedy that class allows (see classifyPortBackDrift).
// A behind class logs at Info, since a sync clears it; every other class logs at Warn.
// A missing source file is skipped silently; this comparison never returns an error and never
// affects an exit code, per the drift-notification-is-logger-warn-and-never-blocks Shared Decision.
func warnPortBackDrift(baseDir string, registry Registry, source Source) {
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
			if class == driftBehind {
				logger.Info(msg, "stencil", name, "class", class.String())
				continue
			}
			logger.Warn(msg, "stencil", name, "class", class.String())
		}
	}
}

// ForceRefresh performs the refresh row even on a stencil a ModeDev pass would leave untouched.
// It is the entry point `lyx stencil sync` calls, which is why an explicit sync refreshes even from
// a -dev-stamped binary.
func ForceRefresh(baseDir string, registry Registry, source Source) ([]string, error) {
	return Reconcile(baseDir, registry, ModeProduction, source)
}
