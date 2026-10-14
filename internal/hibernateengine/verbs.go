// verbs.go implements the operator verbs over the request file: submit, cancel and status, with the caller and platform refusals they share.

package hibernateengine

import (
	"errors"
	"fmt"
	"strings"

	"github.com/Knatte18/loomyard/internal/agentname"
	"github.com/Knatte18/loomyard/internal/tenderengine"
)

// refuseCaller refuses an agent: any strand but the orch's.
// An empty strand is an external caller, and a name that does not parse is an agent.
func refuseCaller(strand string) error {
	if strand != "" {
		name, err := agentname.Parse(strand)
		if err != nil || name.Role != agentname.RoleOrch || name.Slug != "" {
			return errors.New(`hibernate: an agent may not request or cancel a hibernate; way forward: ask your parent session, which runs "lyx hibernate request" or "lyx hibernate cancel" for you`)
		}
	}
	return nil
}

// refusePlatform refuses any platform but Linux.
func refusePlatform(goos string) error {
	if goos != "linux" {
		return fmt.Errorf("hibernate: lyx hibernates the machine on Linux only, and this machine runs %s; way forward: hibernate it from the desktop yourself", goos)
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

// SubmitRequest asks for a hibernate.
type SubmitRequest struct {
	// After is the slug of the batten run to wait for; empty waits for nothing.
	After string
	// AfterAll waits for every batten run to be out of flight.
	AfterAll bool
}

// Submit records the hub's hibernate request and spawns the daemon that carries it out, returning once the daemon is spawned.
// The pending-request check, the spawn and the write share one hold of the request lock, so two racing submits spawn one daemon.
// A pending request whose daemon is dead is replaced after its owned pause entry is lifted.
func Submit(deps Deps, req SubmitRequest) (Request, error) {
	if err := refuseCaller(deps.Caller); err != nil {
		return Request{}, err
	}
	if err := refusePlatform(deps.GOOS); err != nil {
		return Request{}, err
	}
	if _, err := deps.LookPath("systemctl"); err != nil {
		return Request{}, errors.New(`hibernate: systemctl is not on PATH, so lyx cannot hibernate this machine; way forward: put systemctl on PATH, then re-run "lyx hibernate request"`)
	}
	if req.After != "" && req.AfterAll {
		return Request{}, errors.New(`hibernate: --after and --after-all are exclusive; way forward: pass one of them, or neither to hibernate once the pause is confirmed`)
	}
	trigger := TriggerNow
	switch {
	case req.After != "":
		trigger = TriggerAfter
		if err := requireRunStatus(deps, req.After); err != nil {
			return Request{}, err
		}
	case req.AfterAll:
		trigger = TriggerAfterAll
	}

	var submitted Request
	err := updateRequest(deps.RequestPath, func(cur Request, found bool) (Request, error) {
		if found && !cur.Terminal() {
			if daemonLive(deps, cur) {
				return Request{}, fmt.Errorf(`hibernate: a hibernate request is pending in phase %s with daemon pid %d; way forward: run "lyx hibernate cancel", then request again`, cur.Phase, cur.DaemonPID)
			}
			if _, err := liftOwned(deps, cur); err != nil {
				return Request{}, err
			}
		}
		pid, start, err := deps.Spawn()
		if err != nil {
			return Request{}, fmt.Errorf("hibernate: spawn the daemon: %w", err)
		}
		submitted = Request{
			Trigger:     trigger,
			Slug:        req.After,
			Requester:   requesterLabel(deps),
			RequestedAt: deps.Now(),
			DaemonPID:   pid,
			DaemonStart: start,
			Phase:       PhaseWaiting,
		}
		return submitted, nil
	})
	if err != nil {
		return Request{}, err
	}
	return submitted, nil
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

// CancelReport is what a cancel did.
type CancelReport struct {
	// Cancelled reports whether a live request was ended.
	Cancelled bool `json:"cancelled"`
	// Phase is the phase the request was cancelled from.
	Phase         Phase `json:"phase"`
	DaemonStopped bool  `json:"daemon_stopped"`
	// Lifted is the lift of the request's owned pause entry, nil when the request owned none.
	Lifted *tenderengine.ResumeReport `json:"lifted,omitempty"`
}

// Cancel ends the pending request, stops its daemon and lifts the pause entry the request owns.
// No request, and a terminal one, succeed with nothing cancelled.
// A live daemon in the hibernating phase is refused; a dead daemon's request is cancelled in any phase.
func Cancel(deps Deps) (CancelReport, error) {
	if err := refuseCaller(deps.Caller); err != nil {
		return CancelReport{}, err
	}
	if err := refusePlatform(deps.GOOS); err != nil {
		return CancelReport{}, err
	}
	var (
		report  CancelReport
		stopPID int
	)
	err := updateRequest(deps.RequestPath, func(cur Request, found bool) (Request, error) {
		if !found || cur.Terminal() {
			return Request{}, errUnchanged
		}
		live := daemonLive(deps, cur)
		if live && cur.Phase == PhaseHibernating {
			return Request{}, errors.New(`hibernate: the machine is going into hibernation; way forward: none until it wakes, when the daemon lifts its own pause; run "lyx hibernate status" to follow it`)
		}
		report.Cancelled, report.Phase = true, cur.Phase
		if live {
			stopPID = cur.DaemonPID
		}
		cur.Phase, cur.Reason = PhaseCancelled, "cancelled by "+requesterLabel(deps)
		return cur, nil
	})
	if err != nil {
		return CancelReport{}, err
	}
	if !report.Cancelled {
		return report, nil
	}
	if stopPID != 0 {
		if err := deps.Kill(stopPID); err != nil {
			return report, fmt.Errorf("hibernate: stop the daemon: %w", err)
		}
		report.DaemonStopped = true
	}
	// The daemon may have recorded its owned entry after the hold and before the kill.
	cur, found, err := readRequest(deps.RequestPath)
	if err != nil || !found {
		return report, err
	}
	report.Lifted, err = liftOwned(deps, cur)
	return report, err
}

// StatusReport is the request as the file holds it and whether its daemon runs.
type StatusReport struct {
	// Request is nil when there is none.
	Request    *Request `json:"request"`
	DaemonLive bool     `json:"daemon_live"`
}

// Status reads the request and its daemon's liveness, changing nothing; it needs no caller and no platform check.
func Status(deps Deps) (StatusReport, error) {
	cur, found, err := readRequest(deps.RequestPath)
	if err != nil || !found {
		return StatusReport{}, err
	}
	return StatusReport{Request: &cur, DaemonLive: daemonLive(deps, cur)}, nil
}
