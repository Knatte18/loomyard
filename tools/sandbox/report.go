// report.go defines the sandbox-report.json contract shared with the launcher's
// caller (the loomyard tooling described in millhouse#586) and implements
// fetchReport, which reads the agent-written report out of the Hub host repo,
// validates and stamps it, then writes a normalized, fingerprint-stamped copy
// into the loomyard root's .scratch directory.

package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// Sandbox report file naming. reportFileName is the agent-written file inside
// the Hub host repo; reportSourceID is the required value of the report's
// top-level "source" field, used to reject reports from an unrelated producer.
const (
	reportFileName = "sandbox-report.json"
	reportSourceID = "sandbox-report"
)

// sandboxReport is the top-level shape of sandbox-report.json. The agent
// writes only Source and Items; Meta is stamped by fetchReport from the
// authoritative binaryInfo after a clean session exit (see the
// "suite.go stamps meta.fingerprint" Shared Decision).
type sandboxReport struct {
	Source string        `json:"source"`
	Meta   reportMeta    `json:"meta"`
	Items  *[]reportItem `json:"items"`
}

// reportMeta holds provenance metadata attached to a sandboxReport. Today it
// carries only the binary fingerprint, but is its own type so future
// provenance fields can be added without reshaping sandboxReport.
type reportMeta struct {
	Fingerprint reportFingerprint `json:"fingerprint"`
}

// reportFingerprint identifies the exact lyx binary that produced a report,
// mirroring the fields already captured by binaryInfo so a maintainer can
// trace a finding back to the build that triggered it.
type reportFingerprint struct {
	Path    string `json:"path"`
	SHA256  string `json:"sha256"`
	Size    int64  `json:"size"`
	ModTime string `json:"modtime"`
}

// reportItem is a single WARN/FAIL finding recorded by the agent during a
// sandbox session.
type reportItem struct {
	Ref   string `json:"ref"`
	Title string `json:"title"`
	Body  string `json:"body"`
}

// fetchReport reads sandbox-report.json from hostRepoDir, validates it
// against the sandbox-report contract, stamps its meta.fingerprint from info,
// and writes the normalized result to
// <loomyardRoot>/.scratch/sandbox-report-<sha256>.json. It is called once per
// suite run, after a clean (exit-0) agent session, per the "normalized
// re-serialize, fetch only on clean exit" Shared Decision.
func fetchReport(hostRepoDir, loomyardRoot string, info binaryInfo) error {
	reportPath := filepath.Join(hostRepoDir, reportFileName)

	raw, err := os.ReadFile(reportPath)
	if err != nil {
		if os.IsNotExist(err) {
			// A missing report means the agent finished without writing one --
			// surface this distinctly from a parse error so the operator knows
			// the agent itself misbehaved, not that the file was malformed.
			return fmt.Errorf("sandbox report not found at %s: the agent produced no report", reportPath)
		}
		return fmt.Errorf("read sandbox report %s: %w", reportPath, err)
	}

	var report sandboxReport
	if err := json.Unmarshal(raw, &report); err != nil {
		return fmt.Errorf("parse sandbox report %s: %w", reportPath, err)
	}

	if report.Source != reportSourceID {
		return fmt.Errorf("sandbox report has wrong source %q (want %q)", report.Source, reportSourceID)
	}
	// Items is decoded as *[]reportItem so a nil pointer (key absent) can be
	// distinguished from a non-nil pointer to an empty slice (key present,
	// zero findings) -- see the "typed-decode validation with *[]Item" Shared
	// Decision. Only the former is rejected.
	if report.Items == nil {
		return fmt.Errorf("sandbox report is missing its items array")
	}

	// The agent does not know its own fingerprint, so the fetch helper -- which
	// already has the authoritative binaryInfo -- stamps it here, overwriting
	// anything the agent may have written to meta.
	report.Meta.Fingerprint = reportFingerprint{
		Path:    info.Path,
		SHA256:  info.SHA256,
		Size:    info.Size,
		ModTime: info.ModTime.Format(time.RFC3339),
	}

	normalized, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal normalized sandbox report: %w", err)
	}

	scratchDir := filepath.Join(loomyardRoot, ".scratch")
	if err := os.MkdirAll(scratchDir, 0o755); err != nil {
		return fmt.Errorf("create scratch dir %s: %w", scratchDir, err)
	}

	destPath := filepath.Join(scratchDir, "sandbox-report-"+info.SHA256+".json")
	if err := os.WriteFile(destPath, normalized, 0o644); err != nil {
		return fmt.Errorf("write fetched sandbox report %s: %w", destPath, err)
	}

	return nil
}
