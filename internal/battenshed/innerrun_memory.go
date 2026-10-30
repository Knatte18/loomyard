// innerrun_memory.go implements InnerRun's per-run memory record: what one pass of the Run-Shed row knows that the next pass, a new process, must find again.

package battenshed

import (
	"errors"
	"fmt"
	"path/filepath"
	"time"

	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/parentlock"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/state"
)

// innerRunMemoryFileSuffix is the fixed suffix of the memory record and of its lock file, joined onto the producer's own name.
// Both end in it, so one entry of runMarkerSuffixes clears the pair with the other run markers.
const innerRunMemoryFileSuffix = "-memory.json"

// innerRunMemoryFile returns the path of the record holding what InnerRun remembers between passes.
func innerRunMemoryFile(scratchDir, producer string) string {
	return filepath.Join(scratchDir, producer+innerRunMemoryFileSuffix)
}

// innerRunMemoryLockFile returns the path of the lock guarding the memory record.
func innerRunMemoryLockFile(scratchDir, producer string) string {
	return filepath.Join(scratchDir, producer+"-lock"+innerRunMemoryFileSuffix)
}

// innerRunMemory is the record InnerRun keeps in its scratch directory so a pass, which is a new process, continues what the pass before it was doing.
// It is a tolerant JSON record: a reader refuses no key it does not know, and a rewrite keeps the keys it does not know.
// A missing record reads as empty, and so does one that does not decode.
type innerRunMemory struct {
	// Episode is the child's current state episode and the delivery of its notices.
	Episode memoryEpisode `json:"episode"`
	// DriverBusyNoticed is whether the orch was noticed of the current stall on a busy driver pane.
	DriverBusyNoticed bool `json:"driver_busy_noticed"`
	// OwnedBy, OwnedSubject, OwnerNoticed, OwnerDue and OwnerCovered are the owner trigger's state.
	OwnedBy      string `json:"owned_by"`
	OwnedSubject string `json:"owned_subject"`
	OwnerNoticed bool   `json:"owner_noticed"`
	OwnerDue     bool   `json:"owner_due"`
	OwnerCovered bool   `json:"owner_covered"`
	// Failures is each open seam-failure episode's failed-tick count, by the episode's key.
	Failures map[string]int `json:"failures"`
	// Restep is the binary re-step's interval and last reading.
	Restep memoryRestep `json:"restep"`
	// WatchedAnswer is how the watched marker was last answered.
	WatchedAnswer int `json:"watched_answer"`

	// IdleWatching is whether the parent lock step holds an idle watch, IdleOwner the owner it counts for and IdleWaiting each waiting agent's first-seen awake reading.
	IdleWatching bool                     `json:"idle_watching"`
	IdleOwner    parentlock.Owner         `json:"idle_owner"`
	IdleWaiting  map[string]time.Duration `json:"idle_waiting"`

	// ResumeRetryAt, NotParkedLogged, BusyLogged, BusyReason and BusyKey are the awaiting arm's resume-retry state.
	ResumeRetryAt   time.Time `json:"resume_retry_at"`
	NotParkedLogged bool      `json:"not_parked_logged"`
	BusyLogged      bool      `json:"busy_logged"`
	BusyReason      string    `json:"busy_reason"`
	BusyKey         string    `json:"busy_key"`
	// LastProbe is when a pass last did the work that costs a process or a multiplexer round trip.
	LastProbe time.Time `json:"last_probe"`
	// Reason and ReasonKeys are the wait reason last written and the keys it was logged under.
	Reason     string          `json:"reason"`
	ReasonKeys map[string]bool `json:"reason_keys"`
	// PauseWarned and CoverageWarned are the pause-flag and pause-record read errors last warned about.
	PauseWarned    string `json:"pause_warned"`
	CoverageWarned string `json:"coverage_warned"`
	// StartFallback is the revive's mark that the halted reason also names `lym loom start`.
	StartFallback bool `json:"start_fallback"`
	// Respawns is how many times the running child was respawned with no write to its status file between, and RespawnStatusTime the status file's modification time after the last respawn.
	Respawns          int       `json:"respawns"`
	RespawnStatusTime time.Time `json:"respawn_status_time"`
}

// memoryEpisode is noticeEpisode as the record holds it.
type memoryEpisode struct {
	Known        bool                      `json:"known"`
	State        shedengine.State          `json:"state"`
	Since        time.Time                 `json:"since"`
	ArbiterOffAt time.Time                 `json:"arbiter_off_at"`
	Deliveries   map[string]memoryDelivery `json:"deliveries"`
}

// memoryDelivery is noticeDelivery as the record holds it.
type memoryDelivery struct {
	Attempts     int       `json:"attempts"`
	LastTry      time.Time `json:"last_try"`
	StrandWarned bool      `json:"strand_warned"`
	Dropped      bool      `json:"dropped"`
}

// memoryRestep is restepWatch as the record holds it.
type memoryRestep struct {
	Stop        string        `json:"stop"`
	Build       string        `json:"build"`
	Interval    time.Duration `json:"interval"`
	NextAt      time.Time     `json:"next_at"`
	Attempts    int           `json:"attempts"`
	DirtyLogged []string      `json:"dirty_logged"`
}

// loadMemory reads the memory record into the producer and into a fresh childWait, which becomes the Call's wait state.
// A missing record reads as empty, and one that does not decode reads as empty with one Warn.
// A read that fails for any other reason is returned.
func (p *innerRunProducer) loadMemory() error {
	p.cur = &childWait{}
	memory, found, err := state.ReadJSON[innerRunMemory](innerRunMemoryFile(p.scratchDir, p.name), innerRunMemoryLockFile(p.scratchDir, p.name))
	if err != nil {
		if !errors.Is(err, state.ErrDecode) {
			return fmt.Errorf("battenshed: %s: read memory record: %w", p.name, err)
		}
		logger.Warn("battenshed: the run's memory record does not decode; reading it as empty", "producer", p.name, "slug", p.slug, "error", err)
		memory, found = innerRunMemory{}, false
	}
	if !found {
		memory = innerRunMemory{}
	}
	p.applyMemory(memory)
	return nil
}

// applyMemory sets the producer's and the current wait's remembered state from memory.
func (p *innerRunProducer) applyMemory(memory innerRunMemory) {
	deliveries := make(map[string]*noticeDelivery, len(memory.Episode.Deliveries))
	for key, d := range memory.Episode.Deliveries {
		deliveries[key] = &noticeDelivery{attempts: d.Attempts, lastTry: d.LastTry, strandWarned: d.StrandWarned, dropped: d.Dropped}
	}
	p.episode = noticeEpisode{known: memory.Episode.Known, state: memory.Episode.State, since: memory.Episode.Since, arbiterOffAt: memory.Episode.ArbiterOffAt, deliveries: deliveries}
	p.driverBusyNoticed = memory.DriverBusyNoticed
	p.ownedBy, p.ownedSubject, p.ownerNoticed, p.ownerDue, p.ownerCovered = memory.OwnedBy, memory.OwnedSubject, memory.OwnerNoticed, memory.OwnerDue, memory.OwnerCovered
	p.failures.Restore(memory.Failures)
	p.restep = restepWatch{stop: memory.Restep.Stop, build: memory.Restep.Build, interval: memory.Restep.Interval, nextAt: memory.Restep.NextAt, attempts: memory.Restep.Attempts, dirtyLogged: memory.Restep.DirtyLogged}
	p.watchedAnswer = watchedAnswer(memory.WatchedAnswer)
	p.respawns, p.respawnStatusTime = memory.Respawns, memory.RespawnStatusTime

	w := p.cur
	if memory.IdleWatching && p.deps.ParentLock != nil {
		w.lock = lockWatch{idle: parentlock.RestoreIdleWatch(p.deps.ParentLock.IdleBound, memory.IdleWaiting), idleOwner: memory.IdleOwner}
	}
	w.resumeRetryAt, w.notParkedLogged = memory.ResumeRetryAt, memory.NotParkedLogged
	w.busyLogged, w.busyReason, w.busyKey = memory.BusyLogged, memory.BusyReason, memory.BusyKey
	w.lastProbe = memory.LastProbe
	w.reason, w.reasonKeys = memory.Reason, memory.ReasonKeys
	w.pauseWarned, w.coverageWarned = memory.PauseWarned, memory.CoverageWarned
	w.revival = driverRevival{startFallback: memory.StartFallback}
}

// saveMemory writes what the producer and the current wait remember to the memory record.
// A failed write warns and changes no verdict: the next pass then starts from the record before this one.
func (p *innerRunProducer) saveMemory() {
	w := p.cur
	memory := innerRunMemory{
		Episode: memoryEpisode{Known: p.episode.known, State: p.episode.state, Since: p.episode.since, ArbiterOffAt: p.episode.arbiterOffAt, Deliveries: map[string]memoryDelivery{}},

		DriverBusyNoticed: p.driverBusyNoticed,
		OwnedBy:           p.ownedBy,
		OwnedSubject:      p.ownedSubject,
		OwnerNoticed:      p.ownerNoticed,
		OwnerDue:          p.ownerDue,
		OwnerCovered:      p.ownerCovered,
		Failures:          p.failures.Ticks(),
		Restep:            memoryRestep{Stop: p.restep.stop, Build: p.restep.build, Interval: p.restep.interval, NextAt: p.restep.nextAt, Attempts: p.restep.attempts, DirtyLogged: p.restep.dirtyLogged},
		WatchedAnswer:     int(p.watchedAnswer),
		Respawns:          p.respawns,
		RespawnStatusTime: p.respawnStatusTime,

		ResumeRetryAt:   w.resumeRetryAt,
		NotParkedLogged: w.notParkedLogged,
		BusyLogged:      w.busyLogged,
		BusyReason:      w.busyReason,
		BusyKey:         w.busyKey,
		LastProbe:       w.lastProbe,
		Reason:          w.reason,
		ReasonKeys:      w.reasonKeys,
		PauseWarned:     w.pauseWarned,
		CoverageWarned:  w.coverageWarned,
		StartFallback:   w.revival.startFallback,
	}
	for key, d := range p.episode.deliveries {
		memory.Episode.Deliveries[key] = memoryDelivery{Attempts: d.attempts, LastTry: d.lastTry, StrandWarned: d.strandWarned, Dropped: d.dropped}
	}
	if w.lock.idle != nil {
		memory.IdleWatching = true
		memory.IdleOwner = w.lock.idleOwner
		memory.IdleWaiting = w.lock.idle.Waiting()
	}
	if err := state.WriteJSON(innerRunMemoryFile(p.scratchDir, p.name), innerRunMemoryLockFile(p.scratchDir, p.name), memory); err != nil {
		logger.Warn("battenshed: could not write the run's memory record; the next pass starts from the one before", "producer", p.name, "slug", p.slug, "error", err)
	}
}

// RespawnCap is how many times the row respawns a running child with no write to its status file between, before it leaves the child to the operator.
const RespawnCap = respawnCap

// RespawnCapReached reports whether the memory record of the row producer under scratchDir shows its running child respawned the cap of times with no write to the child's status file between.
// statusModified is the status file's modification time now: a write after the last respawn is progress and reads as not capped, as the row's own pass reads it.
// It lets a reader outside the row, such as the dobby that sends the driver-dead notice, name the cap; a missing record reads as false.
func RespawnCapReached(scratchDir, producer string, statusModified time.Time) (bool, error) {
	memory, found, err := state.ReadJSON[innerRunMemory](innerRunMemoryFile(scratchDir, producer), innerRunMemoryLockFile(scratchDir, producer))
	if err != nil || !found {
		return false, err
	}
	if statusModified.After(memory.RespawnStatusTime) {
		return false, nil
	}
	return memory.Respawns >= respawnCap, nil
}
