// beamawait.go implements BeamAwait, the pawl recipe's Beam-Await row: the Go producer that holds a run until a land request it accepts passes the landing re-check, and the wait it wakes on.

package pawlengine

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/shedengine"
)

const (
	// NameBeamAwait is the pawl recipe's Beam-Await row name.
	NameBeamAwait = "Beam-Await"

	// finalizeName is the row whose status holds the beam's landing: a run at it holds its beam.
	// The package keeps its own copy, since it must not import the package that declares the row.
	finalizeName = "Finalize"

	// awaitFloor is the shortest wake and the wait the timer starts at and returns to on an event.
	awaitFloor = time.Second
	// awaitCeiling is the longest wait while the row idles.
	awaitCeiling = time.Minute
)

// BeamAwait is the Beam-Await producer.
type BeamAwait struct {
	name string
	deps Deps
}

var _ shedengine.ShedProducer = (*BeamAwait)(nil)

// NewBeamAwait returns the Beam-Await producer for the row called name.
// It refuses a Deps with an unset seam it uses; only Watch may be nil.
func NewBeamAwait(name string, deps Deps) (*BeamAwait, error) {
	seams := append(deps.pathsSeams(),
		seam{"Slug", deps.Slug != ""},
		seam{"RunID", deps.RunID != ""},
		seam{"SetStatus", deps.SetStatus != nil},
		seam{"EntryProducer", deps.EntryProducer != nil},
		seam{"OwnedElsewhere", deps.OwnedElsewhere != nil},
		seam{"StopDobby", deps.StopDobby != nil},
		seam{"StartDobby", deps.StartDobby != nil},
		seam{"DobbyLive", deps.DobbyLive != nil},
		seam{"Recheck", deps.Recheck != nil},
		seam{"ReadyStands", deps.ReadyStands != nil},
		seam{"StampWake", deps.StampWake != nil},
		seam{"Notify", deps.Notify != nil},
		seam{"Now", deps.Now != nil},
		seam{"Sleep", deps.Sleep != nil},
	)
	if err := requireSeams(name, seams...); err != nil {
		return nil, err
	}
	return &BeamAwait{name: name, deps: deps}, nil
}

// nextAwaitWait is the wait after current: a second after an event, otherwise double current up to a minute and never under a second.
func nextAwaitWait(current time.Duration, event bool) time.Duration {
	if event {
		return awaitFloor
	}
	return min(max(current*2, awaitFloor), awaitCeiling)
}

// Call holds the run at the row, spawning no agent, until a land request naming this run passes the landing re-check, and then is Done.
// An entry that finds an accepted landing with no land request settles it itself: a standing ready record is re-checked and lands the beam, and with none the hold is lifted and the dobby restarted.
// A beam another machine owns removes any request and yields, deciding nothing.
// A failing re-check lifts the landing hold again, restarts the dobby the landing stopped, removes the request, posts the blockers and keeps waiting.
// Each wake that completes without ending the row is stamped; a failed stamp is logged only.
// A cancelled context is an error.
func (p *BeamAwait) Call(ctx context.Context) (shedengine.Outcome, shedengine.OutputPointer, error) {
	if err := entryErr(ctx, p.name); err != nil {
		return "", shedengine.OutputPointer{}, err
	}
	run := &awaitRun{BeamAwait: p, interrupted: p.interruptedLanding()}
	wait := awaitFloor
	for {
		result, err := run.wake(ctx)
		if err != nil {
			return "", shedengine.OutputPointer{}, err
		}
		if result.finished {
			return result.outcome, shedengine.OutputPointer{}, nil
		}
		if err := p.deps.StampWake(p.deps.Now()); err != nil {
			logger.Warn("pawlengine: the completed wake could not be stamped", "producer", p.name, "error", err)
		}
		event, err := p.pause(ctx, wait, run.lastWake)
		if err != nil {
			if cerr := cancelErr(ctx, p.name); cerr != nil {
				return "", shedengine.OutputPointer{}, cerr
			}
			return "", shedengine.OutputPointer{}, fmt.Errorf("pawlengine: %s: wait: %w", p.name, err)
		}
		wait = nextAwaitWait(wait, event || result.acted)
	}
}

// interruptedLanding reports whether the row is entered with no land request standing while the entry's status still names Finalize or a row of Deps.RetiredRows:
// the landing hold a crash left between the accept's removals and the row's Done, or the one an older binary wrote under a name the recipe has since retired.
// A failed read reads as not interrupted and is logged.
func (p *BeamAwait) interruptedLanding() bool {
	_, found, err := ReadLandRequest(p.deps.Paths)
	if err != nil {
		logger.Warn("pawlengine: the land request could not be read on entry", "producer", p.name, "error", err)
		return false
	}
	if found {
		return false
	}
	producer, err := p.deps.EntryProducer()
	if err != nil {
		logger.Warn("pawlengine: the entry's status could not be read on entry", "producer", p.name, "error", err)
		return false
	}
	return producer == finalizeName || slices.Contains(p.deps.RetiredRows, producer)
}

// awaitRun is the state of one Call: its producer, what its entry found and when it last woke.
type awaitRun struct {
	*BeamAwait
	// interrupted is true when the entry found an accepted landing it must lift: the hold standing, or the dobby-stop marker standing with no ready record.
	interrupted bool
	// entered is true once the entry's status write succeeded.
	entered  bool
	lastWake time.Time
}

// awaitWake is what one wake decided.
type awaitWake struct {
	outcome shedengine.Outcome
	// finished is true when the row ends with outcome.
	finished bool
	// acted is true when the wake answered a request or left one for a prompt retry, so the timer returns to a second.
	acted bool
}

// wake runs one wake: the owner read, the settling of an accepted landing the entry finds and the entry's writes while they are pending, and the land request.
func (r *awaitRun) wake(ctx context.Context) (awaitWake, error) {
	r.lastWake = r.deps.Now()
	if err := cancelErr(ctx, r.name); err != nil {
		return awaitWake{}, err
	}
	owned, err := r.deps.OwnedElsewhere()
	if err != nil {
		logger.Warn("pawlengine: the beam's owner could not be read, so this wake decides nothing", "producer", r.name, "error", err)
		return awaitWake{}, nil
	}
	if owned {
		if err := RemoveLandRequest(r.deps.Paths); err != nil {
			logger.Warn("pawlengine: a land request of a beam another machine owns could not be removed", "producer", r.name, "error", err)
		}
		return awaitWake{outcome: shedengine.Yield, finished: true}, nil
	}
	if !r.entered {
		settled, result, err := r.settleEntry(ctx)
		if err != nil {
			return awaitWake{}, err
		}
		if settled {
			return result, nil
		}
	}
	if !r.entered && !r.enter() {
		return awaitWake{}, nil
	}
	req, found, err := ReadLandRequest(r.deps.Paths)
	if err != nil {
		logger.Warn("pawlengine: the land request could not be read", "producer", r.name, "error", err)
		return awaitWake{}, nil
	}
	if !found {
		return awaitWake{}, nil
	}
	if req.RunID != r.deps.RunID {
		if err := RemoveLandRequest(r.deps.Paths); err != nil {
			logger.Warn("pawlengine: a land request of another run could not be removed", "producer", r.name, "error", err)
		}
		return awaitWake{acted: true}, nil
	}
	return r.accept(ctx)
}

// enter writes the status naming this row and, once it is written, the entry's markers and notices.
// An entry that lifts an interrupted landing also removes the dobby-stop marker and restarts the dobby, whether or not the marker stood.
// It reports whether the entry completed; a failed status write is retried at the next wake.
func (r *awaitRun) enter() bool {
	if err := r.deps.SetStatus(NameBeamAwait); err != nil {
		logger.Warn("pawlengine: the entry's status could not be written, so it is retried at the next wake", "producer", r.name, "error", err)
		return false
	}
	r.entered = true

	marked, err := markerStands(r.deps.Paths.AwaitMarker)
	if err != nil {
		logger.Warn("pawlengine: the await marker could not be read", "producer", r.name, "error", err)
	}
	if r.interrupted {
		if err := removeFile(r.deps.Paths.DobbyStopMarker); err != nil {
			logger.Warn("pawlengine: the dobby-stop marker of an interrupted landing could not be removed", "producer", r.name, "error", err)
		}
		line := fmt.Sprintf("the beam's landing was interrupted: its hold is lifted and its dobby restarted; run `lym beam ready %s` to land once you are satisfied", r.deps.Slug)
		if err := r.deps.StartDobby(); err != nil {
			line = fmt.Sprintf("the beam's landing was interrupted and its hold is lifted, but its dobby could not be restarted (%v); run `lym beam start %s`, then `lym beam ready %s` to land once you are satisfied", err, r.deps.Slug, r.deps.Slug)
		}
		r.notify(line)
	}
	if !marked {
		if err := writeMarker(r.deps.Paths.AwaitMarker, r.deps.Now()); err != nil {
			logger.Warn("pawlengine: the await marker could not be written", "producer", r.name, "error", err)
		}
		return true
	}
	if !r.interrupted {
		r.noticeDownDobby()
	}
	return true
}

// noticeDownDobby posts one notice per episode when a re-entry with no land request standing finds the beam's dobby down.
// The sent notice is recorded in the await marker and skipped on later re-entries until a re-entry sees the dobby live or a refusal restarts it.
// It reads the dobby only on a re-entry, since the dobby Beam-Up's steps just spawned may not hold its lock yet on the first entry.
func (r *awaitRun) noticeDownDobby() {
	if _, found, err := ReadLandRequest(r.deps.Paths); err != nil || found {
		return
	}
	live, err := r.deps.DobbyLive()
	if err != nil {
		logger.Warn("pawlengine: whether the beam's dobby runs could not be read", "producer", r.name, "error", err)
		return
	}
	recorded, err := downNoticeRecorded(r.deps.Paths.AwaitMarker)
	if err != nil {
		logger.Warn("pawlengine: the await marker could not be read for the sent dobby-down notice", "producer", r.name, "error", err)
	}
	if live {
		r.clearDownNotice(recorded)
		return
	}
	if recorded {
		return
	}
	if !r.notify(fmt.Sprintf("the beam's dobby is down; run `lym beam start %s` to start it, then `lym beam land %s` to land", r.deps.Slug, r.deps.Slug)) {
		return
	}
	if err := recordDownNotice(r.deps.Paths.AwaitMarker, r.deps.Now()); err != nil {
		logger.Warn("pawlengine: the sent dobby-down notice could not be recorded", "producer", r.name, "error", err)
	}
}

// clearDownNotice drops the sent dobby-down notice from the await marker when recorded is true.
func (r *awaitRun) clearDownNotice(recorded bool) {
	if !recorded {
		return
	}
	if err := writeMarker(r.deps.Paths.AwaitMarker, r.deps.Now()); err != nil {
		logger.Warn("pawlengine: the sent dobby-down notice could not be cleared", "producer", r.name, "error", err)
	}
}

// accept answers a request naming this run.
// It writes the status naming Finalize, so the landing hold stands, then stops the dobby and re-checks the beam, and ends the row when the re-check passes.
// A failed Finalize write keeps the request for the next wake.
// The await marker goes first and the land request last, so a crash between the removals leaves either the request, which the next wake accepts, or the ready record standing with no await marker, which the entry settles.
func (r *awaitRun) accept(ctx context.Context) (awaitWake, error) {
	return r.landOrRefuse(ctx, r.deps.Paths.AwaitMarker, r.deps.Paths.DobbyStopMarker, r.deps.Paths.LandRequest)
}

// recheckStanding runs the accept's re-check for a landing the entry found with no land request, so the beam lands with no request and the dobby is never asked to write one again.
// A passing re-check removes the await marker and then the dobby-stop marker and ends the row; a failing one is refused as the accept refuses it, and the entry then completes with the await marker written.
func (r *awaitRun) recheckStanding(ctx context.Context) (awaitWake, error) {
	wake, err := r.landOrRefuse(ctx, r.deps.Paths.AwaitMarker, r.deps.Paths.DobbyStopMarker)
	if err != nil || wake.finished || !r.entered {
		return wake, err
	}
	r.interrupted = false
	marked, err := markerStands(r.deps.Paths.AwaitMarker)
	if err != nil {
		logger.Warn("pawlengine: the await marker could not be read", "producer", r.name, "error", err)
		return wake, nil
	}
	if !marked {
		if err := writeMarker(r.deps.Paths.AwaitMarker, r.deps.Now()); err != nil {
			logger.Warn("pawlengine: the await marker could not be written", "producer", r.name, "error", err)
		}
	}
	return wake, nil
}

// landOrRefuse writes the status naming Finalize, so the landing hold stands, then stops the dobby and re-checks the beam.
// A passing re-check removes the files of remove in order and ends the row; a failing one is refused.
// A failed Finalize write decides nothing, so the landing is retried at the next wake.
func (r *awaitRun) landOrRefuse(ctx context.Context, remove ...string) (awaitWake, error) {
	if err := r.deps.SetStatus(finalizeName); err != nil {
		logger.Warn("pawlengine: the landing hold could not be written, so the landing is retried at the next wake", "producer", r.name, "error", err)
		return awaitWake{}, nil
	}
	blockers := r.landChecks()
	if err := cancelErr(ctx, r.name); err != nil {
		return awaitWake{}, err
	}
	if len(blockers) > 0 {
		return r.refuse(blockers), nil
	}
	for _, path := range remove {
		if err := removeFile(path); err != nil {
			logger.Warn("pawlengine: a file of the landing could not be removed", "producer", r.name, "error", err)
		}
	}
	return awaitWake{outcome: shedengine.Done, finished: true}, nil
}

// settleEntry settles, before the entry writes any status, an accepted landing that left no land request.
// The landing is found by the hold the board names, by the ready record standing with the await marker absent, or by the dobby-stop marker standing.
// With the ready record standing it re-checks the beam itself; with none it leaves the hold to be lifted by enter, marking the entry interrupted when the dobby-stop marker stands.
// It reports whether the wake is decided; a failed read decides nothing and is retried at the next wake.
func (r *awaitRun) settleEntry(ctx context.Context) (bool, awaitWake, error) {
	_, requested, err := ReadLandRequest(r.deps.Paths)
	if err != nil {
		logger.Warn("pawlengine: the land request could not be read on entry", "producer", r.name, "error", err)
		return true, awaitWake{}, nil
	}
	if requested {
		return false, awaitWake{}, nil
	}
	stopMarked, err := markerStands(r.deps.Paths.DobbyStopMarker)
	if err != nil {
		logger.Warn("pawlengine: the dobby-stop marker could not be read on entry", "producer", r.name, "error", err)
	}
	awaiting, err := markerStands(r.deps.Paths.AwaitMarker)
	if err != nil {
		logger.Warn("pawlengine: the await marker could not be read on entry", "producer", r.name, "error", err)
		awaiting = true
	}
	if !r.interrupted && !stopMarked && awaiting {
		return false, awaitWake{}, nil
	}
	ready, err := r.deps.ReadyStands()
	if err != nil {
		logger.Warn("pawlengine: the ready record could not be read on entry, so it is retried at the next wake", "producer", r.name, "error", err)
		return true, awaitWake{}, nil
	}
	if ready {
		wake, err := r.recheckStanding(ctx)
		return true, wake, err
	}
	if stopMarked {
		r.interrupted = true
	}
	return false, awaitWake{}, nil
}

// landChecks writes the dobby-stop marker, stops the dobby and runs the re-check.
// It returns the blockers, empty when the beam may land; a failed step is one blocker naming its error.
func (r *awaitRun) landChecks() []string {
	if err := writeMarker(r.deps.Paths.DobbyStopMarker, r.deps.Now()); err != nil {
		return []string{err.Error()}
	}
	if err := r.deps.StopDobby(); err != nil {
		return []string{fmt.Sprintf("the beam's dobby could not be stopped: %v", err)}
	}
	blockers, err := r.deps.Recheck()
	if err != nil {
		return []string{fmt.Sprintf("the landing re-check failed: %v", err)}
	}
	if len(blockers) > 0 {
		return blockers
	}
	return nil
}

// refuse answers a failed re-check: it lifts the landing hold by writing the status naming this row, restarts the dobby while the dobby-stop marker stands, removes the request and posts the blockers.
// A failed status write keeps the request and the marker, restarts nothing and posts nothing, so the hold is never left standing behind a dropped request, and the wait returns to the floor so the retry is soon.
// A failed restart leaves the marker standing and adds `lym beam start` to the posted notice.
func (r *awaitRun) refuse(blockers []string) awaitWake {
	if err := r.deps.SetStatus(NameBeamAwait); err != nil {
		logger.Warn("pawlengine: the landing hold could not be lifted, so the request and the marker are kept for the next wake", "producer", r.name, "error", err)
		return awaitWake{acted: true}
	}
	// The status naming this row is the entry's own write, so an entry that had not completed has now.
	r.entered = true
	stopped, err := markerStands(r.deps.Paths.DobbyStopMarker)
	if err != nil {
		logger.Warn("pawlengine: the dobby-stop marker could not be read, so the dobby is restarted", "producer", r.name, "error", err)
		stopped = true
	}
	var restartErr error
	if stopped {
		if restartErr = r.deps.StartDobby(); restartErr == nil {
			if recorded, err := downNoticeRecorded(r.deps.Paths.AwaitMarker); err == nil {
				r.clearDownNotice(recorded)
			}
			if err := removeFile(r.deps.Paths.DobbyStopMarker); err != nil {
				logger.Warn("pawlengine: the dobby-stop marker could not be removed", "producer", r.name, "error", err)
			}
		}
	}
	if err := RemoveLandRequest(r.deps.Paths); err != nil {
		logger.Warn("pawlengine: the answered land request could not be removed", "producer", r.name, "error", err)
	}

	line := fmt.Sprintf("the beam cannot land yet: %s; fix them, then run `lym beam land %s` to ask again", strings.Join(blockers, "; "), r.deps.Slug)
	if restartErr != nil {
		line = fmt.Sprintf("%s; the beam's dobby could not be restarted (%v), so run `lym beam start %s`", line, restartErr, r.deps.Slug)
	}
	r.notify(line)
	return awaitWake{acted: true}
}

// notify posts line to the beam's orch and reports whether the post succeeded.
// A failure is logged with the line and changes nothing else, since the request is already answered; with no live orch strand it reaches only the log and `lym batten status`.
func (r *awaitRun) notify(line string) bool {
	if err := r.deps.Notify(line); err != nil {
		logger.Warn("pawlengine: the notice to the beam's orch failed", "producer", r.name, "error", err, "notice", line)
		return false
	}
	return true
}

// pause waits wait, or until Watch delivers, and reports whether it did.
// An event still waits out the rest of awaitFloor since lastWake, so no wake follows another by less than a second.
func (p *BeamAwait) pause(ctx context.Context, wait time.Duration, lastWake time.Time) (bool, error) {
	if p.deps.Watch == nil {
		return false, p.deps.Sleep(ctx, wait)
	}
	sleepCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	slept := make(chan error, 1)
	go func() { slept <- p.deps.Sleep(sleepCtx, wait) }()
	select {
	case err := <-slept:
		return false, err
	case <-p.deps.Watch:
		cancel()
		<-slept
		if rest := awaitFloor - p.deps.Now().Sub(lastWake); rest > 0 {
			return true, p.deps.Sleep(ctx, rest)
		}
		return true, nil
	}
}
