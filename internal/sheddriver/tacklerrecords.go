// tackler.go declares the records of a run's tackler and their readers and writers:
// the role name, the verdict words, the worktree snapshot an attempt is judged against, one attempt's directory contents (attempt.json, prompt.md, verdict.yaml, case-brief.md, envelope.json and the applied mark) and the ephemeral in-flight marker.
// Every function takes the directory or path it works on; internal/shedrun declares where those live.

package sheddriver

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"time"

	"gopkg.in/yaml.v3"
)

// TacklerRole is the agent-name role of a tackler strand; a second tackler of a run takes the numbered form.
const TacklerRole = "tackler"

// The three verdict words a tackler writes.
const (
	TacklerVerdictResume   = "resume"
	TacklerVerdictHandBack = "hand-back"
	TacklerVerdictEscalate = "escalate"
)

const (
	tacklerAttemptFileName   = "attempt.json"
	tacklerPromptFileName    = "prompt.md"
	tacklerVerdictFileName   = "verdict.yaml"
	tacklerCaseBriefFileName = "case-brief.md"
	tacklerEnvelopeFileName  = "envelope.json"
	tacklerAppliedFileName   = "applied"
)

// WorktreeSnapshot is the state of a worktree a tackler must leave as it found it:
// the HEAD commit, the branch, and the content hash of every dirty path.
type WorktreeSnapshot struct {
	Head   string            `json:"head"`
	Branch string            `json:"branch"`
	Dirty  map[string]string `json:"dirty"`
}

// Equal reports whether other records the same HEAD, branch, dirty-path set and dirty-path content hashes.
// A nil and an empty Dirty map are equal.
func (s WorktreeSnapshot) Equal(other WorktreeSnapshot) bool {
	return s.Head == other.Head && s.Branch == other.Branch && maps.Equal(s.Dirty, other.Dirty)
}

// TacklerAttempt is attempt.json: the record the Go driver writes before it spawns a tackler, so the attempt counts against the spawn budget whatever happens next.
type TacklerAttempt struct {
	Number       int              `json:"number"`
	StopKey      StopKey          `json:"stop_key"`
	PauseBefore  string           `json:"pause_before"`
	PauseAfter   string           `json:"pause_after"`
	Snapshot     WorktreeSnapshot `json:"snapshot"`
	SpawnedAt    time.Time        `json:"spawned_at"`
	Deadline     time.Time        `json:"deadline"`
	EnvelopePath string           `json:"envelope_path"`
	ReportPath   string           `json:"report_path"`
	StrandGUID   string           `json:"strand_guid"`

	// Dir is the attempt's directory, filled by the readers and never serialized.
	Dir string `json:"-"`
}

// WriteTacklerAttempt creates the attempt directory dir and writes a as its attempt.json.
func WriteTacklerAttempt(dir string, a TacklerAttempt) error {
	return writeRecordJSON(filepath.Join(dir, tacklerAttemptFileName), a)
}

// ReadTacklerAttempt reads the attempt.json in dir, and sets the result's Dir to dir.
func ReadTacklerAttempt(dir string) (TacklerAttempt, error) {
	var a TacklerAttempt
	found, err := readRecordJSON(filepath.Join(dir, tacklerAttemptFileName), &a)
	if err != nil {
		return TacklerAttempt{}, err
	}
	if !found {
		return TacklerAttempt{}, fmt.Errorf("sheddriver:tackler attempt directory %q has no %s", dir, tacklerAttemptFileName)
	}
	a.Dir = dir
	return a, nil
}

// ListTacklerAttempts reads every subdirectory of tacklerDir as an attempt, sorted by Number.
// An absent tacklerDir lists none.
// A subdirectory with no readable attempt.json is an error naming it, since the driver writes that file before anything else and the count of attempts is the spawn budget.
func ListTacklerAttempts(tacklerDir string) ([]TacklerAttempt, error) {
	entries, err := os.ReadDir(tacklerDir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("sheddriver:read tackler directory: %w", err)
	}
	var attempts []TacklerAttempt
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		attempt, err := ReadTacklerAttempt(filepath.Join(tacklerDir, entry.Name()))
		if err != nil {
			return nil, err
		}
		attempts = append(attempts, attempt)
	}
	slices.SortFunc(attempts, func(a, b TacklerAttempt) int { return a.Number - b.Number })
	return attempts, nil
}

// TacklerPromptPath names the prompt.md of the attempt directory dir.
func TacklerPromptPath(dir string) string { return filepath.Join(dir, tacklerPromptFileName) }

// TacklerVerdictPath names the verdict.yaml of the attempt directory dir.
func TacklerVerdictPath(dir string) string { return filepath.Join(dir, tacklerVerdictFileName) }

// TacklerCaseBriefPath names the case-brief.md of the attempt directory dir, which only an escalate writes.
func TacklerCaseBriefPath(dir string) string { return filepath.Join(dir, tacklerCaseBriefFileName) }

// TacklerEnvelopePath names the envelope.json of the attempt directory dir.
func TacklerEnvelopePath(dir string) string { return filepath.Join(dir, tacklerEnvelopeFileName) }

// TacklerVerdict is verdict.yaml, the agent's own result.
// Verdict is TacklerVerdictResume, TacklerVerdictHandBack or TacklerVerdictEscalate.
// Goto names the row a resume moves the run back to, and is empty for a resume that continues where the run stands.
// Question is addressed to the run's parent and is set on a hand-back.
type TacklerVerdict struct {
	Verdict  string `yaml:"verdict"`
	Reason   string `yaml:"reason"`
	Goto     string `yaml:"goto"`
	Question string `yaml:"question"`
}

// ReadTacklerVerdict reads the verdict file at path; an absent file returns found false.
// It refuses a file that does not decode, an unknown key and a verdict word outside the three.
func ReadTacklerVerdict(path string) (TacklerVerdict, bool, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return TacklerVerdict{}, false, nil
	}
	if err != nil {
		return TacklerVerdict{}, false, fmt.Errorf("sheddriver:read tackler verdict %q: %w", path, err)
	}
	var v TacklerVerdict
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	decoder.KnownFields(true)
	if err := decoder.Decode(&v); err != nil {
		return TacklerVerdict{}, false, fmt.Errorf("sheddriver:parse tackler verdict %q: %w", path, err)
	}
	switch v.Verdict {
	case TacklerVerdictResume, TacklerVerdictHandBack, TacklerVerdictEscalate:
	default:
		return TacklerVerdict{}, false, fmt.Errorf("sheddriver:tackler verdict %q: verdict %q is none of %q, %q or %q", path, v.Verdict, TacklerVerdictResume, TacklerVerdictHandBack, TacklerVerdictEscalate)
	}
	return v, true, nil
}

// TacklerEnvelope is envelope.json, the Go driver's validated result for an attempt.
// Cause is empty for the agent's own verdict and names the failure that made the driver hand back otherwise.
type TacklerEnvelope struct {
	Verdict       string `json:"verdict"`
	Reason        string `json:"reason"`
	Question      string `json:"question"`
	Goto          string `json:"goto"`
	Cause         string `json:"cause"`
	Attempt       int    `json:"attempt"`
	AttemptDir    string `json:"attempt_dir"`
	VerdictPath   string `json:"verdict_path"`
	CaseBriefPath string `json:"case_brief_path"`
}

// WriteTacklerEnvelope writes e at path, creating its directory.
func WriteTacklerEnvelope(path string, e TacklerEnvelope) error {
	return writeRecordJSON(path, e)
}

// ReadTacklerEnvelope reads the envelope at path; an absent file returns found false.
func ReadTacklerEnvelope(path string) (TacklerEnvelope, bool, error) {
	var e TacklerEnvelope
	found, err := readRecordJSON(path, &e)
	if err != nil || !found {
		return TacklerEnvelope{}, false, err
	}
	return e, true, nil
}

// MarkTacklerApplied writes the applied mark in the attempt directory dir, recording that the re-launch consumed the attempt.
func MarkTacklerApplied(dir string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("sheddriver:create tackler attempt directory: %w", err)
	}
	if err := os.WriteFile(filepath.Join(dir, tacklerAppliedFileName), nil, 0o644); err != nil {
		return fmt.Errorf("sheddriver:write tackler applied mark: %w", err)
	}
	return nil
}

// TacklerApplied reports whether the attempt directory dir carries the applied mark.
func TacklerApplied(dir string) (bool, error) {
	_, err := os.Stat(filepath.Join(dir, tacklerAppliedFileName))
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("sheddriver:stat tackler applied mark: %w", err)
	}
	return true, nil
}

// TacklerMarker is the ephemeral in-flight marker: it exists while a tackler of the run may be live.
type TacklerMarker struct {
	Attempt    int       `json:"attempt"`
	StrandGUID string    `json:"strand_guid"`
	Deadline   time.Time `json:"deadline"`
}

// WriteTacklerMarker writes m at path, creating its directory.
func WriteTacklerMarker(path string, m TacklerMarker) error {
	return writeRecordJSON(path, m)
}

// ReadTacklerMarker reads the marker at path; an absent file returns found false.
func ReadTacklerMarker(path string) (TacklerMarker, bool, error) {
	var m TacklerMarker
	found, err := readRecordJSON(path, &m)
	if err != nil || !found {
		return TacklerMarker{}, false, err
	}
	return m, true, nil
}

// RemoveTacklerMarker removes the marker at path; an absent one is not an error.
func RemoveTacklerMarker(path string) error {
	if err := os.Remove(path); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("sheddriver:remove tackler marker: %w", err)
	}
	return nil
}
