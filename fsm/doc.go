// Package fsm provides small finite state machine primitives built on state
// polymorphism: a business object delegates each behavior to its current State,
// and each concrete state implements that behavior its own way.
//
// Business methods run behavior first, transition second. The current state's
// behavior method, which takes the business object as a parameter, performs or
// rejects the request; after it succeeds, Transit moves the object to the next
// state. States hold only their label, so restoring an object from a stored
// label is BuildState and copying an object is safe.
//
// A StateMachine holds the transition table: from + action -> to edges, each
// optionally guarded by a Condition, and a StateBuilder per state. Build it
// once and keep it in a package-level variable with MustFreeze, which runs
// Check and panics on a miswired machine.
//
// Transit evaluates the candidate edges in the order they were added, builds
// the first matching target state, and hands it to StateContext.SetState,
// which the business object implements to type-check the state and keep domain
// side effects local. When no edge exists, or edges exist but no Condition
// matches, Transit returns a *TransitionError and the state stays unchanged.
// The behavior has already changed the business object by then, so callers
// discard an object whose business method returned an error.
//
// A StateMachine guards its own tables with a mutex; the consumer owns
// concurrency control around the business object.
//
// README.md walks through the full pattern with a checklist for consumer
// code, and example_test.go runs it.
package fsm
