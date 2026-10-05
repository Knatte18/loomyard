// Package shedverbs implements the generic cobra verb bodies -- run, step, status, pause, and goto -- shared by every module that arms a *shedengine.Shed onto a CLI subtree.
// goto only reads its Spec and calls shedengine.Goto, which owns the status-file mutation.
// It owns those verb bodies and nothing else: it derives no path of its own, imports no resolver, and never calls os.Getwd or internal/lyxcwd -- every path this package touches reaches it told, through a Spec the arming module fills.
// It also imports no <module>cli package, which is what keeps a consuming
// module's own imports acyclic: internal/shedcli (or any other CLI module wiring shedverbs.Verbs
// onto its own subtree) can safely import shedverbs, but shedverbs never imports back.
// The one resolving import it admits is internal/logger, for boundary logging and the two
// sink-location accessors, per the Shed Verb-Set Invariant.
//
// step prints a short envelope by default.
// Before printing, it writes the full envelope to its per-invocation record under Spec.StepsDir, and the short envelope names that record's path as "envelope_path".
// The --full flag, or a record that could not be written, makes step print the full envelope instead, byte-identical to the record.
// The record, the exit code and the run's state are the same with or without --full.
package shedverbs
