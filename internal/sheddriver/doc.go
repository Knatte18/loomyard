// Package sheddriver is the shed layer's Go driver and its tackler.
// It is a told-geometry engine: it is handed every path it works on and reaches recipe behaviour only through seams the arming module fills.
//
// The records both the driver and loom's arbiter read live here.
// StopKey identifies the stop an attempt or a settle record names.
// SettleRecord is one line of the settle-records file, one per action the driver takes at a stop;
// the repair cap, the one verify retry and the one hand-over per build are counted from these records, so a restarted driver keeps every cap.
// The tackler's records are the attempt directory's attempt, verdict, case brief and envelope files, the applied mark and the ephemeral in-flight marker.
// The records sit in this package so that a CLI package reads them without importing another CLI package.
package sheddriver
