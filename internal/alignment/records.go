// records.go declares the alignment agent's role and points, its attempt records, its verdict schema and the decision snapshot.
// Every function takes the directory or path it works on; internal/shedrun declares where those live.

package alignment

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
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
)

// Role is the agent-name role of an alignment strand; a second agent of a run takes the numbered form.
const Role = "alignment"

// Point names where in a run the agent is asked, and is a single path segment.
type Point string

const (
	// PointParentReview is the parent review of the writer's discussion.
	PointParentReview Point = "parent-review"
	// PointPRGate is the approval of the run's pull request.
	PointPRGate Point = "pr-gate"
)

// The three verdict words the agent writes.
const (
	VerdictApprove  = "approve"
	VerdictReject   = "reject"
	VerdictEscalate = "escalate"
)

const (
	attemptFileName = "attempt.json"
	promptFileName  = "prompt.md"
)

// Attempt is attempt.json: the record written before the agent spawns, so a restarted driver finds the attempt.
// Key is the idempotence key the session reuses an attempt under, and Token is random per spawn and names the agent's output files.
// PRNumber and HeadSHA are set at PointPRGate only.
// Applied is the verdict Go applied, empty until then.
type Attempt struct {
	Number       int       `json:"number"`
	Key          string    `json:"key"`
	Point        Point     `json:"point"`
	Token        string    `json:"token"`
	SpawnedAt    time.Time `json:"spawned_at"`
	Deadline     time.Time `json:"deadline"`
	StrandGUID   string    `json:"strand_guid"`
	Snapshot     string    `json:"snapshot"`
	PRNumber     int       `json:"pr_number"`
	HeadSHA      string    `json:"head_sha"`
	Closed       bool      `json:"closed"`
	ClosedReason string    `json:"closed_reason"`
	Applied      string    `json:"applied"`
	Notified     bool      `json:"notified"`

	// Dir is the attempt's directory, filled by the readers and never serialized.
	Dir string `json:"-"`
}

// VerdictPath names the agent's verdict file, which carries the attempt's token.
func (a Attempt) VerdictPath() string {
	return filepath.Join(a.Dir, "verdict-"+a.Token+".yaml")
}

// ReportPath names the agent's report file, which carries the attempt's token.
func (a Attempt) ReportPath() string {
	return filepath.Join(a.Dir, "report-"+a.Token+".md")
}

// PromptPath names the prompt.md of the attempt.
func (a Attempt) PromptPath() string {
	return filepath.Join(a.Dir, promptFileName)
}

// WriteAttempt creates a.Dir and writes a as its attempt.json.
func WriteAttempt(a Attempt) error {
	data, err := json.MarshalIndent(a, "", "  ")
	if err != nil {
		return fmt.Errorf("alignment: encode attempt %q: %w", a.Dir, err)
	}
	if err := os.MkdirAll(a.Dir, 0o755); err != nil {
		return fmt.Errorf("alignment: create attempt directory: %w", err)
	}
	path := filepath.Join(a.Dir, attemptFileName)
	if err := os.WriteFile(path, append(data, '\n'), 0o644); err != nil {
		return fmt.Errorf("alignment: write attempt %q: %w", path, err)
	}
	return nil
}

// ReadAttempt reads the attempt.json in dir and sets the result's Dir to dir.
// A directory without the file is an error.
func ReadAttempt(dir string) (Attempt, error) {
	a, found, err := readAttemptFile(dir)
	if err != nil {
		return Attempt{}, err
	}
	if !found {
		return Attempt{}, fmt.Errorf("alignment: attempt directory %q has no %s", dir, attemptFileName)
	}
	return a, nil
}

// ListAttempts reads every subdirectory of root that holds an attempt.json, sorted by Number.
// An absent root lists none, and a subdirectory without the file is skipped.
// A file that does not parse is an error naming it.
func ListAttempts(root string) ([]Attempt, error) {
	entries, err := os.ReadDir(root)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("alignment: read attempts directory: %w", err)
	}
	var attempts []Attempt
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		attempt, found, err := readAttemptFile(filepath.Join(root, entry.Name()))
		if err != nil {
			return nil, err
		}
		if found {
			attempts = append(attempts, attempt)
		}
	}
	slices.SortFunc(attempts, func(a, b Attempt) int { return a.Number - b.Number })
	return attempts, nil
}

// readAttemptFile reads dir's attempt.json; an absent file returns found false.
func readAttemptFile(dir string) (a Attempt, found bool, err error) {
	path := filepath.Join(dir, attemptFileName)
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return Attempt{}, false, nil
	}
	if err != nil {
		return Attempt{}, false, fmt.Errorf("alignment: read attempt %q: %w", path, err)
	}
	if err := json.Unmarshal(data, &a); err != nil {
		return Attempt{}, false, fmt.Errorf("alignment: parse attempt %q: %w", path, err)
	}
	a.Dir = dir
	return a, true, nil
}

// Verdict is the agent's verdict file.
// Question is the escalation's question for the run's parent, and Superseded lists the board slugs an approve supersedes.
type Verdict struct {
	Verdict    string   `yaml:"verdict"`
	Reason     string   `yaml:"reason"`
	Question   string   `yaml:"question"`
	Superseded []string `yaml:"superseded"`
}

// ReadVerdict reads and validates the verdict file of a; an absent file returns found false.
// It refuses a file that does not parse or carries an unknown key, and a verdict word other than the three.
// It also refuses an empty reason, an escalate with no question, a superseded list on anything but an approve, and a missing or empty report file.
func ReadVerdict(a Attempt) (Verdict, bool, error) {
	path := a.VerdictPath()
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return Verdict{}, false, nil
	}
	if err != nil {
		return Verdict{}, false, fmt.Errorf("alignment: read verdict %q: %w", path, err)
	}
	var v Verdict
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	decoder.KnownFields(true)
	if err := decoder.Decode(&v); err != nil {
		return Verdict{}, false, fmt.Errorf("alignment: parse verdict %q: %w", path, err)
	}
	switch {
	case v.Verdict != VerdictApprove && v.Verdict != VerdictReject && v.Verdict != VerdictEscalate:
		return Verdict{}, false, fmt.Errorf("alignment: verdict %q: verdict %q is none of %q, %q or %q", path, v.Verdict, VerdictApprove, VerdictReject, VerdictEscalate)
	case strings.TrimSpace(v.Reason) == "":
		return Verdict{}, false, fmt.Errorf("alignment: verdict %q: reason is empty", path)
	case v.Verdict == VerdictEscalate && strings.TrimSpace(v.Question) == "":
		return Verdict{}, false, fmt.Errorf("alignment: verdict %q: an %s needs a question for the parent", path, VerdictEscalate)
	case v.Verdict != VerdictApprove && len(v.Superseded) > 0:
		return Verdict{}, false, fmt.Errorf("alignment: verdict %q: only an %s supersedes board entries", path, VerdictApprove)
	}
	report, err := os.ReadFile(a.ReportPath())
	if err != nil {
		return Verdict{}, false, fmt.Errorf("alignment: verdict %q: read the report: %w", path, err)
	}
	if strings.TrimSpace(string(report)) == "" {
		return Verdict{}, false, fmt.Errorf("alignment: verdict %q: the report %q is empty", path, a.ReportPath())
	}
	return v, true, nil
}

// ParseAttemptDir parses an attempt directory's files, keyed by file name, as they sit in a git tree.
// The verdict is the file the attempt's Token names, and is nil when that file is absent.
// An unknown key reads, none of ReadVerdict's checks run, and the attempt's Dir stays empty.
// It errors, naming the file, for an attempt.json that is absent or does not decode and for a verdict file that does not decode.
func ParseAttemptDir(files map[string][]byte) (Attempt, *Verdict, error) {
	attemptData, ok := files[attemptFileName]
	if !ok {
		return Attempt{}, nil, fmt.Errorf("alignment: attempt directory has no %s", attemptFileName)
	}
	var attempt Attempt
	if err := json.Unmarshal(attemptData, &attempt); err != nil {
		return Attempt{}, nil, fmt.Errorf("alignment: parse %s: %w", attemptFileName, err)
	}
	verdictName := filepath.Base(attempt.VerdictPath())
	verdictData, ok := files[verdictName]
	if !ok {
		return attempt, nil, nil
	}
	var verdict Verdict
	if err := yaml.NewDecoder(bytes.NewReader(verdictData)).Decode(&verdict); err != nil {
		return Attempt{}, nil, fmt.Errorf("alignment: parse %s: %w", verdictName, err)
	}
	return attempt, &verdict, nil
}

// Snapshot returns the hex SHA-256 over the decision record's bytes and the notes' texts, in the order given.
// Each part is length-prefixed, so a changed record or note changes it, and so does an added or removed note.
func Snapshot(decisionRecord []byte, notes []string) string {
	hash := sha256.New()
	writePart := func(part []byte) {
		fmt.Fprintf(hash, "%d:", len(part))
		hash.Write(part)
	}
	writePart(decisionRecord)
	for _, note := range notes {
		writePart([]byte(note))
	}
	return hex.EncodeToString(hash.Sum(nil))
}
