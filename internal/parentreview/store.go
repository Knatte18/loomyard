// store.go is the on-disk round store: Latest and BeginRound for the round lifecycle, the verb-facing transitions, and the exported mutators the gate closure drives.

package parentreview

import (
	"encoding/json"
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
	// alignmentFile records the alignment agent's attempt for the round.
	alignmentFile = "alignment.json"
)

// Alignment states.
const (
	// AlignmentRunning means the agent was spawned and its verdict has not been applied or given up on.
	AlignmentRunning = "running"
	// AlignmentApplied means the agent's approve or reject was recorded as the round's verdict.
	AlignmentApplied = "applied"
	// AlignmentFallback means the round went to the reviewer after the agent escalated, failed or never answered.
	AlignmentFallback = "fallback"
	// AlignmentStopped means a verdict from the parent, the wait bound or the writer's end closed the agent first.
	AlignmentStopped = "stopped"
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
	// ErrEmptyReviewFile means a reject, or an approve carrying a review file, came without a readable, non-empty one.
	ErrEmptyReviewFile = errors.New("parentreview: review file missing or empty")
	// ErrNotAtCap means the latest round is not the cap's reject, so there is nothing to supersede.
	ErrNotAtCap = errors.New("parentreview: latest round is not the cap's reject")
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
	// Cap is the reject cap the gate held when it opened the round; zero means none was recorded.
	Cap int `json:"cap,omitempty"`
}

// Delivery is delivery.json.
type Delivery struct {
	DeliveredAt    time.Time `json:"delivered_at"`
	FailedReason   string    `json:"failed_reason"`
	Prompts        int       `json:"prompts"`
	LastPromptAt   time.Time `json:"last_prompt_at"`
	WaitingNotifys int       `json:"waiting_notifies"`
	CapWarned      bool      `json:"cap_warned"`
	// Pings counts the keep-alives carried and LastPingAt stamps the latest; a delivery.json written before keep-alives decodes with both zero.
	Pings      int       `json:"pings"`
	LastPingAt time.Time `json:"last_ping_at"`
}

// Verdict is verdict.json.
type Verdict struct {
	Kind       string    `json:"kind"`
	RecordedAt time.Time `json:"recorded_at"`
	Consumed   bool      `json:"consumed"`
	// Superseding is set only on an approve that replaced the cap's reject.
	Superseding bool `json:"superseding,omitempty"`
}

// Alignment is alignment.json: the alignment agent's attempt for one round.
type Alignment struct {
	// Attempt is the attempt directory the agent works in.
	Attempt   string    `json:"attempt"`
	Strand    string    `json:"strand"`
	SpawnedAt time.Time `json:"spawned_at"`
	// State is one of the Alignment* constants.
	State string `json:"state"`
	// Report is the agent's report path, empty when it wrote none.
	Report string `json:"report,omitempty"`
	// Reason says why the agent was stopped or the round fell back.
	Reason string `json:"reason,omitempty"`
}

// Round is the decoded view of one round directory.
type Round struct {
	Number    int
	Dir       string
	Request   *Request
	Delivery  Delivery
	Verdict   *Verdict
	Alignment *Alignment
}

// RequestPath is the round's request.json path.
func (r Round) RequestPath() string { return filepath.Join(r.Dir, requestFile) }

// BriefPath is the round's brief.md path, the one the delivery prompt names: the brief alone tells a reviewer with no prior context what to read and how to submit.
func (r Round) BriefPath() string { return filepath.Join(r.Dir, briefFile) }

// ReviewPath is the round's copied review.md path.
func (r Round) ReviewPath() string { return filepath.Join(r.Dir, reviewFile) }

// OpenSpec is what the gate hands OpenRequest.
type OpenSpec struct {
	Slug           string
	Reviewer       string
	DecisionRecord string
	SupportLog     string
	Brief          string
	// Cap is the reject cap recorded on the request.
	Cap int
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

// roundNumbers lists the numbers of every round directory under Root.
func (s Store) roundNumbers() ([]int, error) {
	entries, err := os.ReadDir(s.Root)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("parentreview: read %s: %w", s.Root, err)
	}
	var nums []int
	for _, e := range entries {
		if !e.IsDir() || !strings.HasPrefix(e.Name(), roundPrefix) {
			continue
		}
		if n, err := strconv.Atoi(strings.TrimPrefix(e.Name(), roundPrefix)); err == nil {
			nums = append(nums, n)
		}
	}
	return nums, nil
}

func (s Store) latestNumber() (int, error) {
	nums, err := s.roundNumbers()
	if err != nil {
		return 0, err
	}
	latest := 0
	for _, n := range nums {
		if n > latest {
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
	a, found, err := state.ReadJSON[Alignment](filepath.Join(dir, alignmentFile), lp)
	if err != nil {
		return Round{}, err
	}
	if found {
		r.Alignment = &a
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

// peekLatest returns the highest-numbered round's request and verdict, or false when there is no round.
// It takes no round lock and creates no directory, so a caller on a polling cadence leaves the exchange untouched; the Delivery stays zero.
func (s Store) peekLatest() (Round, bool, error) {
	n, err := s.latestNumber()
	if err != nil || n == 0 {
		return Round{}, false, err
	}
	r := Round{Number: n, Dir: s.roundDir(n)}
	req, found, err := readHeldJSON[Request](filepath.Join(r.Dir, requestFile))
	if err != nil {
		return Round{}, false, err
	}
	if found {
		r.Request = &req
	}
	v, found, err := readHeldJSON[Verdict](filepath.Join(r.Dir, verdictFile))
	if err != nil {
		return Round{}, false, err
	}
	if found {
		r.Verdict = &v
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

// RejectedRounds counts the rounds under Root whose verdict is a reject.
func (s Store) RejectedRounds() (int, error) {
	return s.rejectedRoundsExcept(0)
}

// rejectedRoundsExcept is RejectedRounds without round skip, for a caller holding that round's lock.
func (s Store) rejectedRoundsExcept(skip int) (int, error) {
	nums, err := s.roundNumbers()
	if err != nil {
		return 0, err
	}
	count := 0
	for _, n := range nums {
		if n == skip {
			continue
		}
		r, err := s.load(n)
		if err != nil {
			return 0, err
		}
		if r.Verdict != nil && r.Verdict.Kind == VerdictReject {
			count++
		}
	}
	return count, nil
}

// PrepareRound is Discussion-Write's fresh-spawn preparation.
// It returns the latest round unchanged when it holds no request, or its verdict is a reject or a superseding approve, and otherwise calls BeginRound.
func (s Store) PrepareRound() (Round, error) {
	r, ok, err := s.Latest()
	if err != nil {
		return Round{}, err
	}
	if ok && (r.Request == nil || r.Verdict != nil && (r.Verdict.Kind == VerdictReject || r.Verdict.Superseding)) {
		return r, nil
	}
	return s.BeginRound()
}

// SupersedeCapReject replaces the latest round's reject with a superseding approve, keeping review.md.
// It refuses with ErrNotAtCap unless, read under the round lock, the latest round's verdict is a reject, its request carries a positive Cap, and the rejected rounds, that one included, reach it.
func (s Store) SupersedeCapReject() error {
	r, ok, err := s.Latest()
	if err != nil {
		return err
	}
	if !ok {
		return ErrNotAtCap
	}
	now := s.now()
	return state.UpdateJSON(filepath.Join(r.Dir, verdictFile), s.lockPath(r.Number), func(cur Verdict, found bool) (Verdict, error) {
		if !found || cur.Kind != VerdictReject {
			return cur, ErrNotAtCap
		}
		req, reqFound, err := readHeldJSON[Request](filepath.Join(r.Dir, requestFile))
		if err != nil {
			return cur, err
		}
		if !reqFound || req.Cap <= 0 {
			return cur, ErrNotAtCap
		}
		earlier, err := s.rejectedRoundsExcept(r.Number)
		if err != nil {
			return cur, err
		}
		if earlier+1 < req.Cap {
			return cur, ErrNotAtCap
		}
		return Verdict{Kind: VerdictApprove, Superseding: true, RecordedAt: now}, nil
	})
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
		Cap:            spec.Cap,
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

// RecordPing records one carried keep-alive on the latest round: it bumps the ping count and stamps the last-ping time, touching no prompt count, last-prompt time or waiting notify.
func (s Store) RecordPing() error {
	r, ok, err := s.Latest()
	if err != nil {
		return err
	}
	if !ok {
		return ErrNoOpenRequest
	}
	now := s.now()
	return s.updateDelivery(r.Number, func(d *Delivery) {
		d.Pings++
		d.LastPingAt = now
	})
}

// MarkExpired marks the latest round's request expired.
// It refuses with ErrVerdictRecorded when a verdict landed since the caller's read, checked under the round lock, so a recorded verdict is never expired unread.
func (s Store) MarkExpired() error {
	r, ok, err := s.Latest()
	if err != nil {
		return err
	}
	if !ok {
		return ErrNoOpenRequest
	}
	return state.UpdateJSON(filepath.Join(r.Dir, requestFile), s.lockPath(r.Number), func(cur Request, found bool) (Request, error) {
		if !found {
			return cur, ErrNoOpenRequest
		}
		if _, verdicted, err := readHeldJSON[Verdict](filepath.Join(r.Dir, verdictFile)); err != nil || verdicted {
			if err == nil {
				err = ErrVerdictRecorded
			}
			return cur, err
		}
		cur.State = StateExpired
		return cur, nil
	})
}

// readHeldJSON decodes the JSON file at path for a caller already holding the round lock, reporting false when it does not exist.
// It reads through the standard library because state.ReadJSON would acquire the held lock again and hang.
func readHeldJSON[T any](path string) (T, bool, error) {
	var v T
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return v, false, nil
		}
		return v, false, fmt.Errorf("parentreview: read %s: %w", path, err)
	}
	if err := json.Unmarshal(data, &v); err != nil {
		return v, false, fmt.Errorf("parentreview: decode %s: %w", path, err)
	}
	return v, true, nil
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

// RecordAlignment writes alignment.json on the latest round.
func (s Store) RecordAlignment(a Alignment) error {
	r, ok, err := s.Latest()
	if err != nil {
		return err
	}
	if !ok {
		return ErrNoOpenRequest
	}
	return state.WriteJSON(filepath.Join(r.Dir, alignmentFile), s.lockPath(r.Number), a)
}

// SettleAlignment sets the latest round's alignment record to settled with the given report and reason.
// It refuses with ErrNoOpenRequest when the round has no alignment record.
func (s Store) SettleAlignment(settled, report, reason string) error {
	r, ok, err := s.Latest()
	if err != nil {
		return err
	}
	if !ok {
		return ErrNoOpenRequest
	}
	return state.UpdateJSON(filepath.Join(r.Dir, alignmentFile), s.lockPath(r.Number), func(cur Alignment, found bool) (Alignment, error) {
		if !found {
			return cur, ErrNoOpenRequest
		}
		cur.State, cur.Report, cur.Reason = settled, report, reason
		return cur, nil
	})
}

// WriteBrief replaces the latest round's brief.md, under the round lock.
// It refuses with ErrNoOpenRequest when the round has no request.
func (s Store) WriteBrief(brief string) error {
	r, ok, err := s.Latest()
	if err != nil {
		return err
	}
	if !ok {
		return ErrNoOpenRequest
	}
	// The write rides on a lock-held no-op update of request.json, since state offers no locked write of other files.
	return state.UpdateJSON(filepath.Join(r.Dir, requestFile), s.lockPath(r.Number), func(cur Request, found bool) (Request, error) {
		if !found {
			return cur, ErrNoOpenRequest
		}
		if err := os.WriteFile(r.BriefPath(), []byte(brief), 0o644); err != nil {
			return cur, fmt.Errorf("parentreview: write brief: %w", err)
		}
		return cur, nil
	})
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
// A round that already carries a verdict can no longer use the stamp: it returns ErrVerdictRecorded, writes nothing and logs nothing.
// The Warn is logged only after the reason is written.
func (s Store) RecordDelivered(failedReason string) error {
	r, err := s.openRound()
	if err != nil {
		return err
	}
	if r.Request.State == StateExpired {
		return ErrExpired
	}
	now := s.now()
	err = state.UpdateJSON(filepath.Join(r.Dir, deliveryFile), s.lockPath(r.Number), func(cur Delivery, _ bool) (Delivery, error) {
		if _, verdicted, err := readHeldJSON[Verdict](filepath.Join(r.Dir, verdictFile)); err != nil || verdicted {
			if err == nil {
				err = ErrVerdictRecorded
			}
			return cur, err
		}
		if failedReason != "" {
			cur.FailedReason = failedReason
			cur.DeliveredAt = time.Time{}
			return cur, nil
		}
		cur.DeliveredAt = now
		cur.FailedReason = ""
		return cur, nil
	})
	if err != nil {
		return err
	}
	if failedReason != "" {
		logger.Warn("parent review notice not delivered", "slug", r.Request.Slug, "reviewer", r.Request.Reviewer, "reason", failedReason)
	}
	return nil
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
// A reject needs a readable, non-empty review file, and so does an approve that is given one; an approve with no path copies nothing.
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
		if err != nil || len(strings.TrimSpace(string(data))) == 0 {
			return ErrEmptyReviewFile
		}
	} else if kind == VerdictReject {
		return ErrEmptyReviewFile
	}
	now := s.now()
	// The request is re-read and review.md written under the verdict's own round lock,
	// so a request the gate expired since openUnsettled's read refuses here, and a refused verdict never overwrites another's review.md.
	return state.UpdateJSON(filepath.Join(r.Dir, verdictFile), s.lockPath(r.Number), func(cur Verdict, found bool) (Verdict, error) {
		if found {
			return cur, ErrVerdictRecorded
		}
		req, ok, err := readHeldJSON[Request](filepath.Join(r.Dir, requestFile))
		if err != nil {
			return cur, err
		}
		if !ok || req.State == StateSuperseded {
			return cur, ErrNoOpenRequest
		}
		if req.State == StateExpired {
			return cur, ErrExpired
		}
		if reviewPath != "" {
			if err := os.WriteFile(r.ReviewPath(), data, 0o644); err != nil {
				return cur, fmt.Errorf("parentreview: write review: %w", err)
			}
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
