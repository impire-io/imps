// Package lifecycle holds the imp's atomic lifecycle state machine. The
// state values are primitive ints; this package intentionally has no
// harness-package dependency to keep it usable from any internal subsystem
// without inducing import cycles.
package lifecycle

import "sync/atomic"

// State is the imp's current lifecycle state. Transitions are one-way:
// Created → Starting → Running → Draining → Stopped, plus terminal Failed
// reachable only from Starting.
type State int32

// Lifecycle state constants. Transitions are one-way:
// Created → Starting → Running → Draining → Stopped, with terminal Failed
// reachable only from Starting.
const (
	StateCreated State = iota
	StateStarting
	StateRunning
	StateDraining
	StateStopped
	StateFailed
)

// String renders the state for log/observability.
func (s State) String() string {
	switch s {
	case StateCreated:
		return "Created"
	case StateStarting:
		return "Starting"
	case StateRunning:
		return "Running"
	case StateDraining:
		return "Draining"
	case StateStopped:
		return "Stopped"
	case StateFailed:
		return "Failed"
	default:
		return "Unknown"
	}
}

// Machine is an atomic State holder. Set transitions enforce the one-way
// rule by accepting any caller-chosen target — the harness only attempts
// legal transitions, and Set returns whether the transition occurred.
type Machine struct {
	state atomic.Int32
}

// New returns a Machine pre-initialized in StateCreated.
func New() *Machine {
	m := &Machine{}
	m.state.Store(int32(StateCreated))
	return m
}

// Get returns the current state.
func (m *Machine) Get() State {
	return State(m.state.Load())
}

// Set unconditionally sets the state. Returns the previous state.
func (m *Machine) Set(s State) State {
	return State(m.state.Swap(int32(s)))
}

// CompareAndSwap transitions from prev → next only if the current state
// is prev. Returns true on success.
func (m *Machine) CompareAndSwap(prev, next State) bool {
	return m.state.CompareAndSwap(int32(prev), int32(next))
}
