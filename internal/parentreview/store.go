// store.go is the on-disk round store: Latest and BeginRound for the round lifecycle, the verb-facing transitions, and the exported mutators the gate closure drives.

package parentreview

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/state"
)

const (
	roundPrefix  = "round-"
	requestFile  = "request.json"
	briefFile    = "brief.md"
	deliveryFile = "delivery.json"
	verdictFile  = "verdict.json"
	reviewFile   = "review.md"
)

// Request states.
const (
	StateOpen       = "open"
	StateExpired    = "expired"
	StateSuperseded = "superseded"
)

// Verdict kinds.
const (
	VerdictApprove = "approve"
	VerdictReject  = "reject"
)

// Refusals the CLI maps onto way-forward messages.
var (
	// ErrNoOpenRequest means there is no round, no request, or the latest round was superseded.
	ErrNoOpenRequest = errors.New("parentreview: no open review request")
	// ErrExpired means the request passed on its wait bound.
	ErrExpired = errors.New("parentreview: review request expired")
	// ErrVerdictRecorded means the round already carries a verdict.
	ErrVerdictRecorded = errors.New("parentreview: verdict already recorded")
	// ErrEmptyReviewFile means a reject came without a readable, non-empty review file.
	ErrEmptyReviewFile = errors.New("parentreview: review file missing or empty")
)

// Request is request.json.
type Request struct {
	Slug           string    `json:"slug"`
	Round          int       `json:"round"`
	Reviewer       string    `json:"reviewer"`
	OpenedAt       time.Time `json:"opened_at"`
	DecisionRecord string    `json:"decision_record"`
	SupportLog     string    `json:"support_log"`
	State          string    `json:"state"`
}

// Delivery is delivery.json.
type Delivery struct {
	DeliveredAt    time.Time `json:"delivered_at"`
	FailedReason   string    `json:"failed_reason"`
	Prompts        int       `json:"prompts"`
	LastPromptAt   time.Time `json:"last_prompt_at"`
	WaitingNotifys int       `json:"waiting_notifies"`
	CapWarned      bool      `json:"cap_warned"`
}

// Verdict is verdict.json.
type Verdict struct {
	Kind       string    `json:"kind"`
	RecordedAt time.Time `json:"recorded_at"`
	Consumed   bool      `json:"consumed"`
}

// Round is the decoded view of one round directory.
type Round struct {
	Number   int
	Dir      string
	Request  *Request
	Delivery Delivery
	Verdict  *Verdict
}

// RequestPath is the round's request.json path, the one the delivery prompt names.
func (r Round) RequestPath() string { return filepath.Join(r.Dir, requestFile) }

// ReviewPath is the round's copied review.md path.
func (r Round) ReviewPath() string { return filepath.Join(r.Dir, reviewFile) }

// OpenSpec is what the gate hands OpenRequest.
type OpenSpec struct {
	Slug           string
	Reviewer       string
	DecisionRecord string
	SupportLog     string
	Brief          string
}

// Store is the told geometry of one run's parent-review exchange.
type Store struct {
	// Root is the told _lyx/reviews/parent-review directory.
	Root string
	// LockDir is Root's .lyx mirror, holding one lock file per round.
	LockDir string
	// Now is the clock; nil means time.Now.
	Now func() time.Time
}

func (s Store) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

func (s Store) roundDir(n int) string { return filepath.Join(s.Root, roundPrefix+strconv.Itoa(n)) }

func (s Store) lockPath(n int) string {
	return filepath.Join(s.LockDir, roundPrefix+strconv.Itoa(n)+".lock")
}

func (s Store) latestNumber() (int, error) {
	entries, err := os.ReadDir(s.Root)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, fmt.Errorf("parentreview: read %s: %w", s.Root, err)
	}
	latest := 0
	for _, e := range entries {
		if !e.IsDir() || !strings.HasPrefix(e.Name(), roundPrefix) {
			continue
		}
		n, err := strconv.Atoi(strings.TrimPrefix(e.Name(), roundPrefix))
		if err == nil && n > latest {
			latest = n
		}
	}
	return latest, nil
}

func (s Store) load(n int) (Round, error) {
	if err := os.MkdirAll(s.LockDir, 0o755); err != nil {
		return Round{}, fmt.Errorf("parentreview: mkdir %s: %w", s.LockDir, err)
	}
	dir := s.roundDir(n)
	lp := s.lockPath(n)
	r := Round{Number: n, Dir: dir}
	req, found, err := state.ReadJSON[Request](filepath.Join(dir, requestFile), lp)
	if err != nil {
		return Round{}, err
	}
	if found {
		r.Request = &req
	}
	if r.Delivery, _, err = state.ReadJSON[Delivery](filepath.Join(dir, deliveryFile), lp); err != nil {
		return Round{}, err
	}
	v, found, err := state.ReadJSON[Verdict](filepath.Join(dir, verdictFile), lp)
	if err != nil {
		return Round{}, err
	}
	if found {
		r.Verdict = &v
	}
	return r, nil
}

// Latest returns the highest-numbered round, or false when there is none.
func (s Store) Latest() (Round, bool, error) {
	n, err := s.latestNumber()
	if err != nil || n == 0 {
		return Round{}, false, err
	}
	r, err := s.load(n)
	if err != nil {
		return Round{}, false, err
	}
	return r, true, nil
}

// BeginRound marks the latest round superseded when it is still open with no verdict, then creates the next round directory empty.
func (s Store) BeginRound() (Round, error) {
	n, err := s.latestNumber()
	if err != nil {
		return Round{}, err
	}
	if n > 0 {
		cur, err := s.load(n)
		if err != nil {
			return Round{}, err
		}
		if cur.Request != nil && cur.Request.State == StateOpen && cur.Verdict == nil {
			if err := s.setRequestState(n, StateSuperseded); err != nil {
				return Round{}, err
			}
		}
	}
	next := n + 1
	if err := os.MkdirAll(s.roundDir(next), 0o755); err != nil {
		return Round{}, fmt.Errorf("parentreview: mkdir round: %w", err)
	}
	return Round{Number: next, Dir: s.roundDir(next)}, nil
}

func (s Store) setRequestState(n int, st string) error {
	return state.UpdateJSON(filepath.Join(s.roundDir(n), requestFile), s.lockPath(n), func(cur Request, found bool) (Request, error) {
		if !found {
			return cur, ErrNoOpenRequest
		}
		cur.State = st
		return cur, nil
	})
}

// OpenRequest writes request.json and brief.md into the latest round, creating round 1 when none exists.
// It is a no-op error-free return when the latest round already has a request.
func (s Store) OpenRequest(spec OpenSpec) (Round, error) {
	r, ok, err := s.Latest()
	if err != nil {
		return Round{}, err
	}
	if !ok {
		if r, err = s.BeginRound(); err != nil {
			return Round{}, err
		}
	}
	if r.Request != nil {
		return r, nil
	}
	if err := os.MkdirAll(s.LockDir, 0o755); err != nil {
		return Round{}, fmt.Errorf("parentreview: mkdir %s: %w", s.LockDir, err)
	}
	if err := os.WriteFile(filepath.Join(r.Dir, briefFile), []byte(spec.Brief), 0o644); err != nil {
		return Round{}, fmt.Errorf("parentreview: write brief: %w", err)
	}
	req := Request{
		Slug:           spec.Slug,
		Round:          r.Number,
		Reviewer:       spec.Reviewer,
		OpenedAt:       s.now(),
		DecisionRecord: spec.DecisionRecord,
		SupportLog:     spec.SupportLog,
		State:          StateOpen,
	}
	if err := state.WriteJSON(filepath.Join(r.Dir, requestFile), s.lockPath(r.Number), req); err != nil {
		return Round{}, err
	}
	r.Request = &req
	return r, nil
}

func (s Store) updateDelivery(n int, mutate func(d *Delivery)) error {
	return state.UpdateJSON(filepath.Join(s.roundDir(n), deliveryFile), s.lockPath(n), func(cur Delivery, _ bool) (Delivery, error) {
		mutate(&cur)
		return cur, nil
	})
}

// RecordPrompt records one carried prompt on the latest round: it takes a waiting notify when fromNotify is set, and otherwise bumps the gate prompt count; either way it stamps the last-prompt time.
func (s Store) RecordPrompt(fromNotify bool) error {
	r, ok, err := s.Latest()
	if err != nil {
		return err
	}
	if !ok {
		return ErrNoOpenRequest
	}
	now := s.now()
	return s.updateDelivery(r.Number, func(d *Delivery) {
		if fromNotify {
			if d.WaitingNotifys > 0 {
				d.WaitingNotifys--
			}
		} else {
			d.Prompts++
		}
		d.LastPromptAt = now
	})
}

// MarkExpired marks the latest round's request expired.
func (s Store) MarkExpired() error {
	r, ok, err := s.Latest()
	if err != nil {
		return err
	}
	if !ok {
		return ErrNoOpenRequest
	}
	return s.setRequestState(r.Number, StateExpired)
}

// MarkConsumed marks the latest round's verdict consumed.
func (s Store) MarkConsumed() error {
	r, ok, err := s.Latest()
	if err != nil {
		return err
	}
	if !ok {
		return ErrNoOpenRequest
	}
	return state.UpdateJSON(filepath.Join(r.Dir, verdictFile), s.lockPath(r.Number), func(cur Verdict, found bool) (Verdict, error) {
		if !found {
			return cur, ErrNoOpenRequest
		}
		cur.Consumed = true
		return cur, nil
	})
}

// MarkCapWarned records that the cap Warn was logged for the latest round.
func (s Store) MarkCapWarned() error {
	r, ok, err := s.Latest()
	if err != nil {
		return err
	}
	if !ok {
		return ErrNoOpenRequest
	}
	return s.updateDelivery(r.Number, func(d *Delivery) { d.CapWarned = true })
}

// openRound returns the latest round when it carries an open request, else the matching refusal.
func (s Store) openRound() (Round, error) {
	r, ok, err := s.Latest()
	if err != nil {
		return Round{}, err
	}
	if !ok || r.Request == nil || r.Request.State == StateSuperseded {
		return Round{}, ErrNoOpenRequest
	}
	return r, nil
}

// RecordDelivered stamps delivered-at on the latest open request, or, when failedReason is non-empty, records the reason, logs a Warn and leaves delivered-at empty.
func (s Store) RecordDelivered(failedReason string) error {
	r, err := s.openRound()
	if err != nil {
		return err
	}
	if r.Request.State == StateExpired {
		return ErrExpired
	}
	now := s.now()
	if failedReason != "" {
		logger.Warn("parent review notice not delivered", "slug", r.Request.Slug, "reviewer", r.Request.Reviewer, "reason", failedReason)
	}
	return s.updateDelivery(r.Number, func(d *Delivery) {
		if failedReason != "" {
			d.FailedReason = failedReason
			d.DeliveredAt = time.Time{}
			return
		}
		d.DeliveredAt = now
		d.FailedReason = ""
	})
}

// openUnsettled is the shared refusal chain of AddNotify and RecordVerdict.
func (s Store) openUnsettled() (Round, error) {
	r, err := s.openRound()
	if err != nil {
		return Round{}, err
	}
	if r.Verdict != nil {
		return Round{}, ErrVerdictRecorded
	}
	if r.Request.State == StateExpired {
		return Round{}, ErrExpired
	}
	return r, nil
}

// AddNotify adds one waiting notify to the latest open request.
func (s Store) AddNotify() error {
	r, err := s.openUnsettled()
	if err != nil {
		return err
	}
	return s.updateDelivery(r.Number, func(d *Delivery) { d.WaitingNotifys++ })
}

// RecordVerdict records an approve or reject on the latest open request, copying reviewPath to review.md when given.
// A reject needs a readable, non-empty review file.
func (s Store) RecordVerdict(kind, reviewPath string) error {
	if kind != VerdictApprove && kind != VerdictReject {
		return fmt.Errorf("parentreview: unknown verdict kind %q", kind)
	}
	r, err := s.openUnsettled()
	if err != nil {
		return err
	}
	var data []byte
	if reviewPath != "" {
		data, err = os.ReadFile(reviewPath)
		if err != nil && kind == VerdictApprove {
			return fmt.Errorf("parentreview: read review file: %w", err)
		}
		if kind == VerdictReject && (err != nil || len(strings.TrimSpace(string(data))) == 0) {
			return ErrEmptyReviewFile
		}
	} else if kind == VerdictReject {
		return ErrEmptyReviewFile
	}
	if reviewPath != "" {
		if err := os.WriteFile(r.ReviewPath(), data, 0o644); err != nil {
			return fmt.Errorf("parentreview: write review: %w", err)
		}
	}
	now := s.now()
	return state.UpdateJSON(filepath.Join(r.Dir, verdictFile), s.lockPath(r.Number), func(cur Verdict, found bool) (Verdict, error) {
		if found {
			return cur, ErrVerdictRecorded
		}
		return Verdict{Kind: kind, RecordedAt: now}, nil
	})
}

// WaitNote is a one-line note for the latest round while it has an open request without a verdict, and empty otherwise.
func (s Store) WaitNote() (string, error) {
	r, ok, err := s.Latest()
	if err != nil || !ok {
		return "", err
	}
	if r.Request == nil || r.Request.State != StateOpen || r.Verdict != nil {
		return "", nil
	}
	note := fmt.Sprintf("waiting on reviewer %s since %s; ", r.Request.Reviewer, r.Request.OpenedAt.Format(time.RFC3339))
	if r.Delivery.DeliveredAt.IsZero() {
		note += "notice not delivered"
		if r.Delivery.FailedReason != "" {
			note += ": " + r.Delivery.FailedReason
		}
	} else {
		note += "notice delivered at " + r.Delivery.DeliveredAt.Format(time.RFC3339)
	}
	return note, nil
}
