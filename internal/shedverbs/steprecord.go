// steprecord.go holds the record helpers behind `lyx shed step`'s own per-invocation record: an
// in-flight file written before the producer call and the printed envelope written when the body
// returns, both under Spec.StepsDir, plus the last_step summary status reads back from them.
//
// A record write failure is logged and otherwise ignored, so keeping records can never change the
// envelope on stdout or the exit code.

package shedverbs

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/Knatte18/loomyard/internal/logger"
)

const (
	inflightSuffix = ".inflight.json"
	envelopeSuffix = ".json"
)

// inflightRecord is the content of a `<trace_id>.inflight.json` file.
// It also carries the build identity of the lyx that ran the step; a record written before those
// fields existed decodes as an unknown identity.
type inflightRecord struct {
	TraceID   string    `json:"trace_id"`
	PID       int       `json:"pid"`
	StartedAt time.Time `json:"started_at"`
	BuildIdentity
}

// LastStep is the status envelope's last_step value: the most recent step record's trace id,
// whether its envelope file exists, and that file's path once it does. It also carries the build
// identity recorded for that step, and binary_changed: true when the running lyx is a different
// known build. An undecodable record reports an unknown identity and binary_changed false.
type LastStep struct {
	TraceID       string `json:"trace_id"`
	Finished      bool   `json:"finished"`
	EnvelopePath  string `json:"envelope_path,omitempty"`
	VCSRevision   string `json:"vcs_revision"`
	VCSModified   bool   `json:"vcs_modified"`
	BinaryChanged bool   `json:"binary_changed"`
}

// stepRecorder writes one step invocation's records. The zero value, and one built over an empty
// dir, keeps nothing.
type stepRecorder struct {
	dir     string
	traceID string
	build   BuildIdentity
	buf     bytes.Buffer
}

// newStepRecorder returns a recorder for traceID under dir that records build as the step's build
// identity; an empty dir or trace id disables it.
func newStepRecorder(dir, traceID string, build BuildIdentity) *stepRecorder {
	if dir == "" || traceID == "" {
		return &stepRecorder{}
	}
	return &stepRecorder{dir: dir, traceID: traceID, build: build}
}

func (r *stepRecorder) enabled() bool { return r.dir != "" }

// begin writes the in-flight record: the trace id, this process's pid, the start time and the
// build identity.
func (r *stepRecorder) begin() {
	if !r.enabled() {
		return
	}
	data, err := json.Marshal(inflightRecord{TraceID: r.traceID, PID: os.Getpid(), StartedAt: time.Now().UTC(), BuildIdentity: r.build})
	if err == nil {
		err = os.MkdirAll(r.dir, 0o755)
	}
	if err == nil {
		err = os.WriteFile(filepath.Join(r.dir, r.traceID+inflightSuffix), data, 0o644)
	}
	if err != nil {
		logger.Warn("shed: step record: write in-flight record failed", "dir", r.dir, "error", err.Error())
	}
}

// tee returns a writer that passes every write through to out and keeps a copy for finish.
func (r *stepRecorder) tee(out io.Writer) io.Writer {
	if !r.enabled() {
		return out
	}
	return io.MultiWriter(out, &r.buf)
}

// finish writes the captured envelope line to `<trace_id>.json`.
func (r *stepRecorder) finish() {
	if !r.enabled() {
		return
	}
	if err := os.WriteFile(filepath.Join(r.dir, r.traceID+envelopeSuffix), r.buf.Bytes(), 0o644); err != nil {
		logger.Warn("shed: step record: write envelope record failed", "dir", r.dir, "error", err.Error())
	}
}

// lastStepOf reads dir for the most recent in-flight record and reports it, or nil when dir is
// empty, unreadable or holds none. running is the build identity of the calling lyx, compared with
// the chosen record's recorded one to set binary_changed.
func lastStepOf(dir string, running BuildIdentity) *LastStep {
	if dir == "" {
		return nil
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	type candidate struct {
		traceID string
		started time.Time
		build   BuildIdentity
	}
	var cands []candidate
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, inflightSuffix) {
			continue
		}
		c := candidate{traceID: strings.TrimSuffix(name, inflightSuffix)}
		if data, err := os.ReadFile(filepath.Join(dir, name)); err == nil {
			var rec inflightRecord
			if json.Unmarshal(data, &rec) == nil {
				c.started = rec.StartedAt
				c.build = rec.BuildIdentity
			}
		}
		if c.started.IsZero() {
			if info, err := e.Info(); err == nil {
				c.started = info.ModTime()
			}
		}
		cands = append(cands, c)
	}
	if len(cands) == 0 {
		return nil
	}
	sort.Slice(cands, func(i, j int) bool {
		if !cands[i].started.Equal(cands[j].started) {
			return cands[i].started.Before(cands[j].started)
		}
		return cands[i].traceID < cands[j].traceID
	})
	last := cands[len(cands)-1]
	out := &LastStep{
		TraceID:       last.traceID,
		VCSRevision:   last.build.Revision,
		VCSModified:   last.build.Modified,
		BinaryChanged: binaryChanged(running, last.build),
	}
	envelope := filepath.Join(dir, last.traceID+envelopeSuffix)
	if _, err := os.Stat(envelope); err == nil {
		out.Finished = true
		out.EnvelopePath = envelope
	}
	return out
}
