// stopkey.go declares StopKey, the identity of a stop that attempt and settle records name, and its reader off a status.

package sheddriver

import "github.com/Knatte18/loomyard/internal/shedengine"

// StopKey identifies a stop: the status file's current producer, history length and state.
type StopKey struct {
	CurrentProducer string `json:"current_producer"`
	HistoryLength   int    `json:"history_length"`
	State           string `json:"state"`
}

// StopKeyOf reads the stop key off a status.
func StopKeyOf(st shedengine.Status) StopKey {
	return StopKey{CurrentProducer: st.CurrentProducer, HistoryLength: len(st.History), State: string(st.State)}
}
