// entries_drift.go implements the three Drift registry entries: driftBaselineEntry, driftAwaitEntry and driftVerifyEntry.
// Each reads its keys, checks the Env.Drift seams its producer reads and hands the producer a copy of Env.Drift filled with the run-wide seat runner and clock.

package shedrecipe

import (
	"fmt"

	"github.com/Knatte18/loomyard/internal/driftengine"
	"github.com/Knatte18/loomyard/internal/shedengine"
)

const (
	driftSubjectWayForward = "way forward: set subject to decision-record or board-entry in the row's config"
	driftKeyWayForward     = "way forward: remove the key; the on and off switch of a Drift row is its entry_gate"
)

// driftSeams are the Env.Drift seams a Drift entry checks before it builds its producer.
type driftSeams []struct {
	field string
	seam  any
}

// requireDriftSeams errors naming the first seam of seams that is nil.
func requireDriftSeams(entry string, seams driftSeams) error {
	for _, s := range seams {
		if err := requireSeam(entry, s.field, s.seam); err != nil {
			return err
		}
	}
	return nil
}

// driftDepsFrom returns a copy of env.Drift whose seat runner and clock are the run-wide ones.
func driftDepsFrom(env Env) driftengine.Deps {
	deps := env.Drift
	deps.Seats = env.Seats
	deps.Now = env.Now
	return deps
}

// driftSubject reads the row's required subject key and refuses a value outside the two the Drift agents read a task from.
func driftSubject(entry string, row RowIdentity, cfg Config) (string, error) {
	subject, err := configString(cfg, "subject", true)
	if err != nil {
		return "", fmt.Errorf("shedrecipe: %s: row %q: %w; %s", entry, row.Name, err, driftSubjectWayForward)
	}
	if subject != driftengine.SubjectDecisionRecord && subject != driftengine.SubjectBoardEntry {
		return "", fmt.Errorf("shedrecipe: %s: row %q: config key %q must be %q or %q, got %q; %s", entry, row.Name, "subject", driftengine.SubjectDecisionRecord, driftengine.SubjectBoardEntry, subject, driftSubjectWayForward)
	}
	return subject, nil
}

// driftBaselineEntry is the Constructor for the "DriftBaseline" registry row.
// The row's required "subject" is decision-record or board-entry.
// It checks Env.Seats and the Env.Drift seams the producer reads, and fills the producer's seat runner and clock from Env.
func driftBaselineEntry(row RowIdentity, cfg Config, env Env) (shedengine.ShedProducer, error) {
	const entry = "DriftBaseline"
	if err := configRejectUnknown(cfg, "subject"); err != nil {
		return nil, fmt.Errorf("%w; row %q: %s", err, row.Name, driftKeyWayForward)
	}
	subject, err := driftSubject(entry, row, cfg)
	if err != nil {
		return nil, err
	}
	if err := requireSeam(entry, "Seats", env.Seats); err != nil {
		return nil, err
	}
	d := env.Drift
	err = requireDriftSeams(entry, driftSeams{
		{"Drift.Config", d.Config},
		{"Drift.Status", d.Status},
		{"Drift.Head", d.Head},
		{"Drift.BranchPoint", d.BranchPoint},
		{"Drift.DirtyPaths", d.DirtyPaths},
		{"Drift.Commit", d.Commit},
		{"Drift.BaselineTable", d.BaselineTable},
		{"Drift.StartCapture", d.StartCapture},
		{"Drift.RunCapture", d.RunCapture},
	})
	if err != nil {
		return nil, err
	}
	return driftengine.NewDriftBaseline(row.Name, driftengine.Row{Subject: subject}, driftDepsFrom(env)), nil
}

// driftAwaitEntry is the Constructor for the "DriftAwait" registry row.
// The row takes no config key and spawns nothing, so it checks only the Env.Drift seams the producer reads.
func driftAwaitEntry(row RowIdentity, cfg Config, env Env) (shedengine.ShedProducer, error) {
	const entry = "DriftAwait"
	if err := configRejectUnknown(cfg); err != nil {
		return nil, fmt.Errorf("%w; row %q: %s", err, row.Name, driftKeyWayForward)
	}
	d := env.Drift
	err := requireDriftSeams(entry, driftSeams{
		{"Drift.Head", d.Head},
		{"Drift.BranchPoint", d.BranchPoint},
		{"Drift.Commit", d.Commit},
		{"Drift.Sleep", d.Sleep},
	})
	if err != nil {
		return nil, err
	}
	return driftengine.NewDriftAwait(row.Name, driftDepsFrom(env)), nil
}

// driftVerifyEntry is the Constructor for the "DriftVerify" registry row.
// The row's required "subject" is decision-record or board-entry, and its required "gates" holds exactly one entry, named verify, which the agent's table runs once the session committed code.
// It checks Env.Seats and the Env.Drift seams the producer reads, and fills the producer's seat runner, clock and gate findings file from Env.
func driftVerifyEntry(row RowIdentity, cfg Config, env Env) (shedengine.ShedProducer, error) {
	const entry = "DriftVerify"
	if err := configRejectUnknown(cfg, "subject", "gates", "gate_key"); err != nil {
		return nil, fmt.Errorf("%w; row %q: %s", err, row.Name, driftKeyWayForward)
	}
	subject, err := driftSubject(entry, row, cfg)
	if err != nil {
		return nil, err
	}
	gates, err := resolveRowGateSpec(entry, row.Name, cfg, env)
	if err != nil {
		return nil, err
	}
	if len(gates) != 1 || gates[0].Name != "verify" {
		return nil, fmt.Errorf("shedrecipe: %s: row %q: config key %q must hold exactly one entry, named %q; way forward: set gates to a list of one entry named verify", entry, row.Name, "gates", "verify")
	}
	if err := requireSeam(entry, "Seats", env.Seats); err != nil {
		return nil, err
	}
	d := env.Drift
	err = requireDriftSeams(entry, driftSeams{
		{"Drift.Config", d.Config},
		{"Drift.Status", d.Status},
		{"Drift.Head", d.Head},
		{"Drift.DirtyPaths", d.DirtyPaths},
		{"Drift.Commit", d.Commit},
		{"Drift.VerifyTable", d.VerifyTable},
		{"Drift.Compare", d.Compare},
	})
	if err != nil {
		return nil, err
	}
	deps := driftDepsFrom(env)
	deps.GateFindingsFile = env.gateFindingsPath(row.Name)
	return driftengine.NewDriftVerify(row.Name, driftengine.Row{Subject: subject}, gates[0], deps), nil
}
