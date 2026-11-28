// recordsguard.go — the guard over the drift records, scope.json and judged.json.
// Every write commits both files with a digest trailer,
// and a read tells a file edited by hand from the committed copy.

package driftengine

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/state"
)

// RecordsTrailerKey is the records commit trailer that carries the digest of the two records as that commit wrote them.
const RecordsTrailerKey = "Drift-Records"

// Records is the two drift records as a reader may trust them.
type Records struct {
	// Scope is the scope record, and ScopeFound reports that the file exists.
	Scope      ScopeRecord
	ScopeFound bool
	// Judged is the stored rulings, and JudgedFound reports that the file exists.
	Judged      JudgedRecord
	JudgedFound bool
	// Committed is true when the copies come from the newest Drift-Records commit;
	// it is false when no such commit exists and the files are read as they are.
	Committed bool
	// Mismatch is true when the files on disk are not the newest Drift-Records commit's:
	// a hand edit, or a write interrupted before its commit.
	// The copies are then the committed ones.
	Mismatch bool
}

// RecordsDigest returns the sha256 over each file's presence and content, so an absent file differs from an empty one.
func RecordsDigest(scope, judged []byte, scopeFound, judgedFound bool) string {
	hash := sha256.New()
	for _, file := range []struct {
		found bool
		data  []byte
	}{{scopeFound, scope}, {judgedFound, judged}} {
		if !file.found {
			fmt.Fprint(hash, "absent;")
			continue
		}
		fmt.Fprintf(hash, "present:%d:", len(file.data))
		hash.Write(file.data)
	}
	return hex.EncodeToString(hash.Sum(nil))
}

// readRecordFile reads a record file's bytes, answering found false for an absent file.
func readRecordFile(path string) (data []byte, found bool, err error) {
	data, err = os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("drift: read %s: %w", path, err)
	}
	return data, true, nil
}

// decodeRecord decodes a record's bytes, leaving the zero value for an absent file;
// it accepts keys the type does not declare.
func decodeRecord[T any](name string, data []byte, found bool) (T, error) {
	var record T
	if !found {
		return record, nil
	}
	if err := json.Unmarshal(data, &record); err != nil {
		return record, fmt.Errorf("drift: decode %s: %w", name, err)
	}
	return record, nil
}

// currentDigest returns the digest of the two record files as they are on disk.
func currentDigest(p Paths) (string, error) {
	scope, scopeFound, err := readRecordFile(p.Scope)
	if err != nil {
		return "", err
	}
	judged, judgedFound, err := readRecordFile(p.Judged)
	if err != nil {
		return "", err
	}
	return RecordsDigest(scope, judged, scopeFound, judgedFound), nil
}

// ReadRecords reads the two records as the newest commit carrying the Drift-Records trailer holds them.
// It sets Mismatch when the files on disk differ from that commit's digest.
// With no such commit it returns the files as they are, with Committed and Mismatch false,
// so its records are read unguarded until the first write.
func ReadRecords(d Deps) (Records, error) {
	if err := requireSeams("the records guard", seam{"TrailerCommit", d.TrailerCommit != nil}, seam{"FileAt", d.FileAt != nil}); err != nil {
		return Records{}, err
	}
	commit, found, err := d.TrailerCommit(RecordsTrailerKey, true)
	if err != nil {
		return Records{}, fmt.Errorf("drift: find the newest %s commit: %w", RecordsTrailerKey, err)
	}

	var scopeData, judgedData []byte
	var scopeFound, judgedFound bool
	if found {
		if scopeData, scopeFound, err = d.FileAt(commit.SHA, durableRel(scopeFileName)); err != nil {
			return Records{}, fmt.Errorf("drift: read %s at %s: %w", scopeFileName, commit.SHA, err)
		}
		if judgedData, judgedFound, err = d.FileAt(commit.SHA, durableRel(judgedFileName)); err != nil {
			return Records{}, fmt.Errorf("drift: read %s at %s: %w", judgedFileName, commit.SHA, err)
		}
	} else {
		if scopeData, scopeFound, err = readRecordFile(d.Paths.Scope); err != nil {
			return Records{}, err
		}
		if judgedData, judgedFound, err = readRecordFile(d.Paths.Judged); err != nil {
			return Records{}, err
		}
	}

	records := Records{ScopeFound: scopeFound, JudgedFound: judgedFound, Committed: found}
	if records.Scope, err = decodeRecord[ScopeRecord](scopeFileName, scopeData, scopeFound); err != nil {
		return Records{}, err
	}
	if records.Judged, err = decodeRecord[JudgedRecord](judgedFileName, judgedData, judgedFound); err != nil {
		return Records{}, err
	}
	if found {
		digest, err := currentDigest(d.Paths)
		if err != nil {
			return Records{}, err
		}
		records.Mismatch = digest != commit.Value
	}
	return records, nil
}

// WriteRecords applies change to a copy of the records that ReadRecords returns.
// It then writes both files and commits them together.
// The commit ends in `Drift-Records: <digest of the written files>` and then the extra trailers.
// It refuses while the files on disk differ from the newest Drift-Records commit;
// a write would hide the difference, so only restoring them from that commit clears it.
// A write interrupted before its commit leaves such a difference.
func WriteRecords(d Deps, change func(scope *ScopeRecord, judged *JudgedRecord) error, extra ...fabricengine.Trailer) error {
	if err := requireSeams("the records guard", seam{"CommitWithTrailers", d.CommitWithTrailers != nil}); err != nil {
		return err
	}
	records, err := ReadRecords(d)
	if err != nil {
		return err
	}
	if records.Mismatch {
		return fmt.Errorf("drift: the records differ from their newest %s commit, so nothing is written over them; way forward: lym drift amend <slug> --restore", RecordsTrailerKey)
	}
	scope, judged := records.Scope, records.Judged
	if err := change(&scope, &judged); err != nil {
		return err
	}

	if err := writeRecord(d.Paths, scopeFileName, d.Paths.Scope, scope); err != nil {
		return err
	}
	if err := writeRecord(d.Paths, judgedFileName, d.Paths.Judged, judged); err != nil {
		return err
	}
	digest, err := currentDigest(d.Paths)
	if err != nil {
		return err
	}
	trailers := append([]fabricengine.Trailer{{Key: RecordsTrailerKey, Value: digest}}, extra...)
	if err := d.CommitWithTrailers([]string{durableRel(scopeFileName), durableRel(judgedFileName)}, trailers); err != nil {
		return fmt.Errorf("drift: commit the records: %w", err)
	}
	return nil
}

// writeRecord writes the record file called name through internal/state.
// It keeps the keys of the replaced file that the value's type does not declare.
func writeRecord[T any](p Paths, name, path string, value T) error {
	lock, err := recordLock(p, name)
	if err != nil {
		return err
	}
	if err := state.WriteJSON(path, lock, value); err != nil {
		return fmt.Errorf("drift: write %s: %w", name, err)
	}
	return nil
}

// RestoreRecords writes both records back from the newest Drift-Records commit and removes a file that commit lacks.
// It adds no commit, so the history stays as it was and the files match the commit's digest again.
func RestoreRecords(d Deps) error {
	if err := requireSeams("the records guard", seam{"TrailerCommit", d.TrailerCommit != nil}, seam{"FileAt", d.FileAt != nil}); err != nil {
		return err
	}
	commit, found, err := d.TrailerCommit(RecordsTrailerKey, true)
	if err != nil {
		return fmt.Errorf("drift: find the newest %s commit: %w", RecordsTrailerKey, err)
	}
	if !found {
		return fmt.Errorf("drift: no %s commit holds records to restore", RecordsTrailerKey)
	}
	for _, file := range []struct{ name, path string }{{scopeFileName, d.Paths.Scope}, {judgedFileName, d.Paths.Judged}} {
		data, present, err := d.FileAt(commit.SHA, durableRel(file.name))
		if err != nil {
			return fmt.Errorf("drift: read %s at %s: %w", file.name, commit.SHA, err)
		}
		if !present {
			if err := os.Remove(file.path); err != nil && !errors.Is(err, fs.ErrNotExist) {
				return fmt.Errorf("drift: remove %s: %w", file.name, err)
			}
			continue
		}
		lock, err := recordLock(d.Paths, file.name)
		if err != nil {
			return err
		}
		if err := state.WriteFile(file.path, lock, data); err != nil {
			return fmt.Errorf("drift: restore %s: %w", file.name, err)
		}
	}
	return nil
}
