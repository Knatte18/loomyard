// verbs.go implements the operator verbs over the request file: submit, cancel and status.
// It also holds the caller and platform refusals the verbs share.

package hibernateengine

import (
	"errors"
	"fmt"
	"strings"

	"github.com/Knatte18/loomyard/internal/agentname"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/tenderengine"
)

// dobbyDownWayForward is the way forward a submit names when the prime's dobby, which carries the request, does not run.
const dobbyDownWayForward = `the prime's dobby carries the request and does not run; run "lym beam start root" to start it, and it takes the request up`

// refuseCaller refuses an agent: any strand but an orch's, the prime's or a beam's.
// An empty strand is an external caller, and a name that does not parse is an agent.
func refuseCaller(strand string) error {
	if strand != "" {
		name, err := agentname.Parse(strand)
		if err != nil || name.Role != agentname.RoleOrch {
			return errors.New(`hibernate: an agent may not request or cancel a hibernate; way forward: ask your parent session, which runs "lym hibernate request" or "lym hibernate cancel" for you`)
		}
	}
	return nil
}

// refusePlatform refuses any platform but Linux.
func refusePlatform(goos string) error {
	if goos != "linux" {
		return fmt.Errorf("hibernate: lym hibernates the machine on Linux only, and this machine runs %s; way forward: hibernate it from the desktop yourself", goos)
	}
	return nil
}

// requesterLabel is the caller's strand name, or EXTERNAL.
func requesterLabel(deps Deps) string {
	if deps.Caller == "" {
		return requesterExternal
	}
	return deps.Caller
}

// liftOwned lifts the pause entry r owns by identity, and lifts nothing, returning nil, for a request that owns none.
func liftOwned(deps Deps, r Request) (*tenderengine.ResumeReport, error) {
	if r.Owned == nil {
		return nil, nil
	}
	report, err := deps.Lift(*r.Owned)
	if err != nil {
		return nil, fmt.Errorf("hibernate: lift the request's pause: %w", err)
	}
	return &report, nil
}

// SubmitRequest holds a hibernate request's options: what it waits for, and whether the wake lifts its pause.
type SubmitRequest struct {
	// After is the slug of the batten run to wait for; empty waits for nothing.
	After string
	// AfterAll waits for every batten run to be out of flight.
	AfterAll bool
	// ResumeOnWake makes the wake lift the request's pause, so paused runs go on by themselves.
	ResumeOnWake bool
}

// refuseKeptPause refuses a new request over a terminal request whose owned pause entry is still live.
// Replacing that request would lose the only record that lets "lym hibernate cancel" lift the entry.
// A request that owns no entry, or whose entry is already lifted, passes.
func refuseKeptPause(deps Deps, r Request) error {
	if r.Owned == nil {
		return nil
	}
	status, err := deps.TenderStatus()
	if err != nil {
		return fmt.Errorf(`hibernate: read the hub to check the previous request's pause: %w; way forward: fix what breaks the hub reading, as "lym tender status" shows, then request again`, err)
	}
	for _, entry := range status.Live {
		if entry.Scope == r.Owned.Scope && entry.Caller == r.Owned.Caller && entry.PausedAt.Equal(r.Owned.PausedAt) {
			return fmt.Errorf(`hibernate: the previous request ended %s and its pause entry is still live; way forward: run "lym hibernate cancel" or "lym tender resume" to lift it, then request again`, r.Phase)
		}
	}
	return nil
}

// SubmitReport is the request a submit wrote and whether the prime's dobby, which carries it, runs.
type SubmitReport struct {
	Request   Request `json:"request"`
	DobbyLive bool    `json:"dobby_live"`
	// WayForward names the step that starts the prime's dobby, empty while it runs.
	WayForward string `json:"way_forward,omitempty"`
}

// Submit records the hub's hibernate request, carried by no process yet, and returns at once;
// the prime's dobby takes it up.
// The pending-request check and the write share one hold of the request lock, so of two racing submits one is refused.
// A pending request is refused whether or not a process carries it.
// A terminal request whose owned pause entry is still live is refused too, so the entry a wake kept is not forgotten.
// A prime's dobby that does not run, or whose liveness does not read, is reported with the way forward that starts it.
func Submit(deps Deps, req SubmitRequest) (SubmitReport, error) {
	if err := refuseCaller(deps.Caller); err != nil {
		return SubmitReport{}, err
	}
	if err := refusePlatform(deps.GOOS); err != nil {
		return SubmitReport{}, err
	}
	if _, err := deps.LookPath("systemctl"); err != nil {
		return SubmitReport{}, errors.New(`hibernate: systemctl is not on PATH, so lym cannot hibernate this machine; way forward: put systemctl on PATH, then re-run "lym hibernate request"`)
	}
	if req.After != "" && req.AfterAll {
		return SubmitReport{}, errors.New(`hibernate: --after and --after-all are exclusive; way forward: pass one of them, or neither to hibernate once the pause is confirmed`)
	}
	trigger := TriggerNow
	switch {
	case req.After != "":
		trigger = TriggerAfter
		if err := requireRunStatus(deps, req.After); err != nil {
			return SubmitReport{}, err
		}
	case req.AfterAll:
		trigger = TriggerAfterAll
	}

	var submitted Request
	err := updateRequest(deps.RequestPath, func(cur Request, found bool) (Request, error) {
		if found && !cur.Terminal() {
			return Request{}, fmt.Errorf(`hibernate: a hibernate request is pending in phase %s; way forward: run "lym hibernate cancel", then request again`, cur.Phase)
		}
		if found {
			if err := refuseKeptPause(deps, cur); err != nil {
				return Request{}, err
			}
		}
		submitted = Request{
			Trigger:      trigger,
			Slug:         req.After,
			Requester:    requesterLabel(deps),
			RequestedAt:  deps.Now(),
			ResumeOnWake: req.ResumeOnWake,
			Phase:        PhaseWaiting,
		}
		return submitted, nil
	})
	if err != nil {
		return SubmitReport{}, err
	}
	report := SubmitReport{Request: submitted}
	live, err := deps.DobbyLive()
	if err != nil {
		logger.Warn("hibernate: whether the prime's dobby runs does not read", "error", err)
	}
	report.DobbyLive = live
	if !live {
		report.WayForward = dobbyDownWayForward
	}
	return report, nil
}

// requireRunStatus refuses a slug with no batten status, naming the slugs that have one.
func requireRunStatus(deps Deps, slug string) error {
	_, found, err := deps.RunStatus(slug)
	if err != nil {
		return fmt.Errorf("hibernate: read the batten status of %q: %w", slug, err)
	}
	if found {
		return nil
	}
	slugs, err := deps.RunSlugs()
	if err != nil {
		return fmt.Errorf("hibernate: list the batten runs: %w", err)
	}
	have := "no batten run has one yet"
	if len(slugs) > 0 {
		have = strings.Join(slugs, ", ")
	}
	return fmt.Errorf("hibernate: %q has no batten status in this hub; way forward: name one of the slugs that has one: %s", slug, have)
}

// CancelReport is the outcome of a cancel: whether a live request was ended, and the pause entry lifted for it.
type CancelReport struct {
	// Cancelled reports whether a live request was ended.
	Cancelled bool `json:"cancelled"`
	// Phase is the phase the request was cancelled from.
	Phase Phase `json:"phase"`
	// Lifted is the lift of the request's owned pause entry, nil when the request owned none.
	Lifted *tenderengine.ResumeReport `json:"lifted,omitempty"`
}

// Cancel ends the pending request and lifts the pause entry it owns.
// The dobby that carries the request stops its sequence at the next step and lifts what that sequence added since.
// No request, and a terminal one, succeed with nothing cancelled;
// a terminal request still has the pause entry it owns lifted, so a pause its sequence left live is not kept.
// A request a live daemon carries in the hibernating phase is refused;
// one no live daemon carries is cancelled in any phase.
func Cancel(deps Deps) (CancelReport, error) {
	if err := refuseCaller(deps.Caller); err != nil {
		return CancelReport{}, err
	}
	if err := refusePlatform(deps.GOOS); err != nil {
		return CancelReport{}, err
	}
	var report CancelReport
	err := updateRequest(deps.RequestPath, func(cur Request, found bool) (Request, error) {
		if !found || cur.Terminal() {
			return Request{}, errUnchanged
		}
		if cur.Phase == PhaseHibernating && daemonLive(deps, cur) {
			return Request{}, errors.New(`hibernate: the machine is going into hibernation; way forward: none until it wakes; run "lym hibernate status" to follow it, and once the request has ended "lym hibernate cancel" lifts a pause the wake kept`)
		}
		report.Cancelled, report.Phase = true, cur.Phase
		cur.Phase, cur.Reason = PhaseCancelled, "cancelled by "+requesterLabel(deps)
		return cur, nil
	})
	if err != nil {
		return CancelReport{}, err
	}
	// The request is read again after the hold, so a terminal request's kept entry is lifted too.
	// An entry the sequence records after this read is lifted by the sequence once it sees the cancel.
	// The lift is by identity, so an entry already lifted lifts nothing.
	cur, found, err := readRequest(deps.RequestPath)
	if err != nil || !found {
		return report, err
	}
	report.Lifted, err = liftOwned(deps, cur)
	return report, err
}

// StatusReport is the request as the file holds it and whether the daemon that carries it runs.
type StatusReport struct {
	// Request is nil when there is none.
	Request    *Request `json:"request"`
	DaemonLive bool     `json:"daemon_live"`
}

// Status reads the request and whether the daemon that carries it runs, changing nothing; it needs no caller and no platform check.
func Status(deps Deps) (StatusReport, error) {
	cur, found, err := readRequest(deps.RequestPath)
	if err != nil || !found {
		return StatusReport{}, err
	}
	return StatusReport{Request: &cur, DaemonLive: daemonLive(deps, cur)}, nil
}
