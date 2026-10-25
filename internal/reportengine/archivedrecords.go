// archivedrecords.go decodes the parent-review rounds and alignment attempts that archived runs hold.
// Nothing writes these records any more; the readers only decode what past runs left in their trees.

package reportengine

import (
	"bytes"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/Knatte18/loomyard/internal/loomengine"
)

const (
	parentReviewRoundPrefix   = "round-"
	parentReviewRequestFile   = "request.json"
	parentReviewDeliveryFile  = "delivery.json"
	parentReviewVerdictFile   = "verdict.json"
	parentReviewAlignmentFile = "alignment.json"
	alignmentAttemptFile      = "attempt.json"

	// parentReviewDirName is the directory under the reviews root that holds an archived run's parent-review rounds.
	parentReviewDirName = "parent-review"

	// parentReviewAlignmentApplied is the alignment state of a round whose agent verdict was recorded as the round's verdict.
	parentReviewAlignmentApplied = "applied"

	// alignmentVerdictApprove is the verdict word of an alignment attempt that approved.
	alignmentVerdictApprove = "approve"
)

// parentReviewRequest is a round's request.json.
type parentReviewRequest struct {
	Slug           string    `json:"slug"`
	Round          int       `json:"round"`
	Reviewer       string    `json:"reviewer"`
	OpenedAt       time.Time `json:"opened_at"`
	DecisionRecord string    `json:"decision_record"`
	SupportLog     string    `json:"support_log"`
	State          string    `json:"state"`
	Cap            int       `json:"cap,omitempty"`
}

// parentReviewDelivery is a round's delivery.json.
type parentReviewDelivery struct {
	DeliveredAt    time.Time `json:"delivered_at"`
	FailedReason   string    `json:"failed_reason"`
	Prompts        int       `json:"prompts"`
	LastPromptAt   time.Time `json:"last_prompt_at"`
	WaitingNotifys int       `json:"waiting_notifies"`
	CapWarned      bool      `json:"cap_warned"`
	Pings          int       `json:"pings"`
	LastPingAt     time.Time `json:"last_ping_at"`
}

// parentReviewVerdict is a round's verdict.json.
type parentReviewVerdict struct {
	Kind        string    `json:"kind"`
	RecordedAt  time.Time `json:"recorded_at"`
	Consumed    bool      `json:"consumed"`
	Superseding bool      `json:"superseding,omitempty"`
}

// parentReviewAlignment is a round's alignment.json: the alignment agent's attempt for the round.
type parentReviewAlignment struct {
	Attempt   string    `json:"attempt"`
	Strand    string    `json:"strand"`
	SpawnedAt time.Time `json:"spawned_at"`
	State     string    `json:"state"`
	Report    string    `json:"report,omitempty"`
	Reason    string    `json:"reason,omitempty"`
}

// parentReviewRound is the decoded view of one archived parent-review round directory.
// Request, Verdict and Alignment are nil when their file is absent.
type parentReviewRound struct {
	Number    int
	Dir       string
	Request   *parentReviewRequest
	Delivery  parentReviewDelivery
	Verdict   *parentReviewVerdict
	Alignment *parentReviewAlignment
}

// parentReviewRoundNumber parses a round directory name, `round-<N>` with N a positive decimal; anything else is false.
func parentReviewRoundNumber(dirName string) (int, bool) {
	digits, ok := strings.CutPrefix(dirName, parentReviewRoundPrefix)
	if !ok || digits == "" {
		return 0, false
	}
	for _, r := range digits {
		if r < '0' || r > '9' {
			return 0, false
		}
	}
	number, err := strconv.Atoi(digits)
	if err != nil || number < 1 {
		return 0, false
	}
	return number, true
}

// parseParentReviewRound decodes one round directory's files, keyed by file name, as they sit in a git tree.
// Number is set and Dir stays empty; each file is decoded only when present, an unknown key reads, and a file that does not decode is an error naming it.
func parseParentReviewRound(number int, files map[string][]byte) (parentReviewRound, error) {
	round := parentReviewRound{Number: number}
	if data, ok := files[parentReviewRequestFile]; ok {
		var request parentReviewRequest
		if err := json.Unmarshal(data, &request); err != nil {
			return parentReviewRound{}, fmt.Errorf("reportengine: decode %s: %w", parentReviewRequestFile, err)
		}
		round.Request = &request
	}
	if data, ok := files[parentReviewDeliveryFile]; ok {
		if err := json.Unmarshal(data, &round.Delivery); err != nil {
			return parentReviewRound{}, fmt.Errorf("reportengine: decode %s: %w", parentReviewDeliveryFile, err)
		}
	}
	if data, ok := files[parentReviewVerdictFile]; ok {
		var verdict parentReviewVerdict
		if err := json.Unmarshal(data, &verdict); err != nil {
			return parentReviewRound{}, fmt.Errorf("reportengine: decode %s: %w", parentReviewVerdictFile, err)
		}
		round.Verdict = &verdict
	}
	if data, ok := files[parentReviewAlignmentFile]; ok {
		var alignment parentReviewAlignment
		if err := json.Unmarshal(data, &alignment); err != nil {
			return parentReviewRound{}, fmt.Errorf("reportengine: decode %s: %w", parentReviewAlignmentFile, err)
		}
		round.Alignment = &alignment
	}
	return round, nil
}

// parentReviewDirRel returns the tree-relative directory that holds an archived run's parent-review rounds.
func parentReviewDirRel() string {
	return filepath.Join(loomengine.LoomReviewsDirRel(), parentReviewDirName)
}

// alignmentAttempt is an archived alignment attempt's attempt.json.
// Dir is the attempt's tree-relative directory, filled by the reader and never serialized.
type alignmentAttempt struct {
	Number       int       `json:"number"`
	Key          string    `json:"key"`
	Point        string    `json:"point"`
	Token        string    `json:"token"`
	SpawnedAt    time.Time `json:"spawned_at"`
	Deadline     time.Time `json:"deadline"`
	StrandGUID   string    `json:"strand_guid"`
	Snapshot     string    `json:"snapshot"`
	Closed       bool      `json:"closed"`
	ClosedReason string    `json:"closed_reason"`
	Applied      string    `json:"applied"`
	Notified     bool      `json:"notified"`

	Dir string `json:"-"`
}

// alignmentVerdict is an archived alignment attempt's verdict file.
type alignmentVerdict struct {
	Verdict    string   `yaml:"verdict"`
	Reason     string   `yaml:"reason"`
	Question   string   `yaml:"question"`
	Superseded []string `yaml:"superseded"`
}

// parseAlignmentAttempt decodes an attempt directory's files, keyed by file name, as they sit in a git tree.
// The verdict is the file `verdict-<token>.yaml` the attempt's token names, nil when that file is absent, and it is not validated.
// It errors, naming the file, for an attempt.json that is absent or does not decode and for a verdict file that does not decode.
func parseAlignmentAttempt(files map[string][]byte) (alignmentAttempt, *alignmentVerdict, error) {
	attemptData, ok := files[alignmentAttemptFile]
	if !ok {
		return alignmentAttempt{}, nil, fmt.Errorf("reportengine: attempt directory has no %s", alignmentAttemptFile)
	}
	var attempt alignmentAttempt
	if err := json.Unmarshal(attemptData, &attempt); err != nil {
		return alignmentAttempt{}, nil, fmt.Errorf("reportengine: parse %s: %w", alignmentAttemptFile, err)
	}
	verdictName := "verdict-" + attempt.Token + ".yaml"
	verdictData, ok := files[verdictName]
	if !ok {
		return attempt, nil, nil
	}
	var verdict alignmentVerdict
	if err := yaml.NewDecoder(bytes.NewReader(verdictData)).Decode(&verdict); err != nil {
		return alignmentAttempt{}, nil, fmt.Errorf("reportengine: parse %s: %w", verdictName, err)
	}
	return attempt, &verdict, nil
}
