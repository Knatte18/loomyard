// arbiter.go declares the records of a run's arbiter and their readers and writers:
// the role name, the stop an attempt arbitrates, one attempt's directory contents (attempt.json, prompt.md, verdict.yaml, envelope.json and the applied mark) and the ephemeral in-flight marker.
// Every function takes the directory or path it works on; internal/shedrun declares where those live.
// The records sit in this package, not in loomcli, so battencli reads the marker and matches arbiter strands without importing another CLI package.

package loomengine

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/Knatte18/loomyard/internal/agentname"
	"github.com/Knatte18/loomyard/internal/shedengine"
)

// ArbiterRole is the agent-name role of an arbiter strand; a second arbiter of a run takes the numbered form.
const ArbiterRole = "arbiter"

// ArbiterVerdictResume and ArbiterVerdictHandOn are the two verdict words an arbiter writes.
const (
	ArbiterVerdictResume = "resume"
	ArbiterVerdictHandOn = "hand-on"
)

const (
	arbiterAttemptFileName  = "attempt.json"
	arbiterPromptFileName   = "prompt.md"
	arbiterVerdictFileName  = "verdict.yaml"
	arbiterEnvelopeFileName = "envelope.json"
	arbiterAppliedFileName  = "applied"
)

// IsArbiterStrand reports whether name is a valid agent name whose role is the arbiter role, plain or numbered.
func IsArbiterStrand(name string) bool {
	parsed, err := agentname.Parse(name)
	return err == nil && agentname.MatchesRole(parsed.Role, ArbiterRole)
}

// ArbiterStopKey identifies the stop an attempt arbitrates: the status file's current producer, history length and state.
type ArbiterStopKey struct {
	CurrentProducer string `json:"current_producer"`
	HistoryLength   int    `json:"history_length"`
	State           string `json:"state"`
}

// StopKeyOf reads the stop key off a status.
func StopKeyOf(st shedengine.Status) ArbiterStopKey {
	return ArbiterStopKey{CurrentProducer: st.CurrentProducer, HistoryLength: len(st.History), State: string(st.State)}
}

// ArbiterAttempt is attempt.json: the record the arbiter verb writes before it spawns, so the attempt counts against the spawn budget whatever happens next.
type ArbiterAttempt struct {
	Number       int            `json:"number"`
	StopKey      ArbiterStopKey `json:"stop_key"`
	PauseBefore  string         `json:"pause_before"`
	PauseAfter   string         `json:"pause_after"`
	SpawnedAt    time.Time      `json:"spawned_at"`
	Deadline     time.Time      `json:"deadline"`
	StrandGUID   string         `json:"strand_guid"`
	EnvelopePath string         `json:"envelope_path"`
	ReportPath   string         `json:"report_path"`

	// Dir is the attempt's directory, filled by the readers and never serialized.
	Dir string `json:"-"`
}

// WriteArbiterAttempt creates the attempt directory dir and writes a as its attempt.json.
func WriteArbiterAttempt(dir string, a ArbiterAttempt) error {
	return writeArbiterJSON(filepath.Join(dir, arbiterAttemptFileName), a)
}

// ReadArbiterAttempt reads the attempt.json in dir, and sets the result's Dir to dir.
func ReadArbiterAttempt(dir string) (ArbiterAttempt, error) {
	var a ArbiterAttempt
	found, err := readArbiterJSON(filepath.Join(dir, arbiterAttemptFileName), &a)
	if err != nil {
		return ArbiterAttempt{}, err
	}
	if !found {
		return ArbiterAttempt{}, fmt.Errorf("loom: arbiter attempt directory %q has no %s", dir, arbiterAttemptFileName)
	}
	a.Dir = dir
	return a, nil
}

// ListArbiterAttempts reads every subdirectory of arbiterDir as an attempt, sorted by Number.
// An absent arbiterDir lists none.
// A subdirectory with no readable attempt.json is an error naming it, since the verb writes that file before anything else and the count of attempts is the spawn budget.
func ListArbiterAttempts(arbiterDir string) ([]ArbiterAttempt, error) {
	entries, err := os.ReadDir(arbiterDir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("loom: read arbiter directory: %w", err)
	}
	var attempts []ArbiterAttempt
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		attempt, err := ReadArbiterAttempt(filepath.Join(arbiterDir, entry.Name()))
		if err != nil {
			return nil, err
		}
		attempts = append(attempts, attempt)
	}
	slices.SortFunc(attempts, func(a, b ArbiterAttempt) int { return a.Number - b.Number })
	return attempts, nil
}

// ArbiterPromptPath names the prompt.md of the attempt directory dir.
func ArbiterPromptPath(dir string) string { return filepath.Join(dir, arbiterPromptFileName) }

// ArbiterVerdictPath names the verdict.yaml of the attempt directory dir.
func ArbiterVerdictPath(dir string) string { return filepath.Join(dir, arbiterVerdictFileName) }

// ArbiterEnvelopePath names the envelope.json of the attempt directory dir.
func ArbiterEnvelopePath(dir string) string { return filepath.Join(dir, arbiterEnvelopeFileName) }

// ArbiterVerdict is verdict.yaml, the agent's own result.
// Verdict is ArbiterVerdictResume or ArbiterVerdictHandOn.
// Goto names the row a resume moves the run back to, and is empty for a resume that continues where the run stands.
type ArbiterVerdict struct {
	Verdict  string   `yaml:"verdict"`
	Reason   string   `yaml:"reason"`
	Verbs    []string `yaml:"verbs"`
	Goto     string   `yaml:"goto"`
	Question string   `yaml:"question"`
}

// ReadArbiterVerdict reads and validates the verdict file at path; an absent file returns found false.
// It refuses a file that does not parse, an unknown key, an unknown verdict word, an empty reason, a hand-on without a question and a goto on a hand-on.
func ReadArbiterVerdict(path string) (ArbiterVerdict, bool, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return ArbiterVerdict{}, false, nil
	}
	if err != nil {
		return ArbiterVerdict{}, false, fmt.Errorf("loom: read arbiter verdict %q: %w", path, err)
	}
	var v ArbiterVerdict
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	decoder.KnownFields(true)
	if err := decoder.Decode(&v); err != nil {
		return ArbiterVerdict{}, false, fmt.Errorf("loom: parse arbiter verdict %q: %w", path, err)
	}
	switch {
	case v.Verdict != ArbiterVerdictResume && v.Verdict != ArbiterVerdictHandOn:
		return ArbiterVerdict{}, false, fmt.Errorf("loom: arbiter verdict %q: verdict %q is neither %q nor %q", path, v.Verdict, ArbiterVerdictResume, ArbiterVerdictHandOn)
	case strings.TrimSpace(v.Reason) == "":
		return ArbiterVerdict{}, false, fmt.Errorf("loom: arbiter verdict %q: reason is empty", path)
	case v.Verdict == ArbiterVerdictHandOn && strings.TrimSpace(v.Question) == "":
		return ArbiterVerdict{}, false, fmt.Errorf("loom: arbiter verdict %q: a %s needs a question for the parent", path, ArbiterVerdictHandOn)
	case v.Verdict == ArbiterVerdictHandOn && v.Goto != "":
		return ArbiterVerdict{}, false, fmt.Errorf("loom: arbiter verdict %q: a %s moves no run, so it carries no goto", path, ArbiterVerdictHandOn)
	}
	return v, true, nil
}

// ArbiterEnvelope is envelope.json, the arbiter verb's validated result for an attempt.
// Cause is empty for the agent's own verdict and names the failure that made the verb hand on otherwise.
// Relaunch is the full re-launch line a resume carries.
type ArbiterEnvelope struct {
	Verdict     string   `json:"verdict"`
	Reason      string   `json:"reason"`
	Question    string   `json:"question"`
	Goto        string   `json:"goto"`
	Verbs       []string `json:"verbs"`
	Cause       string   `json:"cause"`
	Attempt     int      `json:"attempt"`
	AttemptDir  string   `json:"attempt_dir"`
	VerdictPath string   `json:"verdict_path"`
	Relaunch    string   `json:"relaunch"`
}

// WriteArbiterEnvelope writes e at path, creating its directory.
func WriteArbiterEnvelope(path string, e ArbiterEnvelope) error {
	return writeArbiterJSON(path, e)
}

// ReadArbiterEnvelope reads the envelope at path; an absent file returns found false.
func ReadArbiterEnvelope(path string) (ArbiterEnvelope, bool, error) {
	var e ArbiterEnvelope
	found, err := readArbiterJSON(path, &e)
	if err != nil || !found {
		return ArbiterEnvelope{}, false, err
	}
	return e, true, nil
}

// MarkArbiterApplied writes the applied mark in the attempt directory dir, recording that the re-launch consumed the attempt.
func MarkArbiterApplied(dir string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("loom: create arbiter attempt directory: %w", err)
	}
	if err := os.WriteFile(filepath.Join(dir, arbiterAppliedFileName), nil, 0o644); err != nil {
		return fmt.Errorf("loom: write arbiter applied mark: %w", err)
	}
	return nil
}

// ArbiterApplied reports whether the attempt directory dir carries the applied mark.
func ArbiterApplied(dir string) (bool, error) {
	_, err := os.Stat(filepath.Join(dir, arbiterAppliedFileName))
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("loom: stat arbiter applied mark: %w", err)
	}
	return true, nil
}

// ArbiterMarker is the ephemeral in-flight marker: it exists while an arbiter of the run may be live.
type ArbiterMarker struct {
	Attempt    int       `json:"attempt"`
	StrandGUID string    `json:"strand_guid"`
	Deadline   time.Time `json:"deadline"`
}

// WriteArbiterMarker writes m at path, creating its directory.
func WriteArbiterMarker(path string, m ArbiterMarker) error {
	return writeArbiterJSON(path, m)
}

// ReadArbiterMarker reads the marker at path; an absent file returns found false.
func ReadArbiterMarker(path string) (ArbiterMarker, bool, error) {
	var m ArbiterMarker
	found, err := readArbiterJSON(path, &m)
	if err != nil || !found {
		return ArbiterMarker{}, false, err
	}
	return m, true, nil
}

// RemoveArbiterMarker removes the marker at path; an absent one is not an error.
func RemoveArbiterMarker(path string) error {
	if err := os.Remove(path); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("loom: remove arbiter marker: %w", err)
	}
	return nil
}

// writeArbiterJSON creates path's directory and writes v there as indented JSON.
func writeArbiterJSON(path string, v any) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return fmt.Errorf("loom: encode arbiter record %q: %w", path, err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("loom: create arbiter record directory: %w", err)
	}
	if err := os.WriteFile(path, append(data, '\n'), 0o644); err != nil {
		return fmt.Errorf("loom: write arbiter record %q: %w", path, err)
	}
	return nil
}

// readArbiterJSON decodes the JSON file at path into v; an absent file returns found false.
func readArbiterJSON(path string, v any) (found bool, err error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("loom: read arbiter record %q: %w", path, err)
	}
	if err := json.Unmarshal(data, v); err != nil {
		return false, fmt.Errorf("loom: parse arbiter record %q: %w", path, err)
	}
	return true, nil
}
