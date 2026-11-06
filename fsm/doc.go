// Package fsm provides small finite state machine primitives built on state
// polymorphism: a business object delegates each behavior to its current State,
// and each concrete state implements that behavior its own way.
//
// A StateMachine holds the transition table: from + action -> to edges, each
// optionally guarded by a Condition, and a StateBuilder per state. After a
// behavior succeeds, Transit moves the business object along the first
// matching edge. The StateMachine locks its own tables; the consumer owns
// concurrency control around the business object.
//
// # Rules for consumer code
//
// Consumer code is done when every rule holds:
//
//   - Each state-specific decision lives in a concrete state's behavior
//     method; the business object holds no if or switch on its current state.
//   - The consumer defines a state interface that embeds State and declares
//     one behavior method per business method, taking the business object as
//     a parameter. States hold only their label, typically by embedding
//     *SimpleState, so copying the business object is safe.
//   - A base state returns an error from every behavior, and each concrete
//     state embeds it and overrides only the behaviors it supports or rejects
//     more specifically.
//   - Behavior first, transition second: every business method calls its
//     state's behavior, then Transit, through an unexported helper such as
//     func (o *Order) transition(a fsm.Action) error { return fsm.Transit(o, orderMachine, a) }.
//   - Public business methods take natural parameters, such as
//     order.Pay(amount, method); the machine stays inside the object.
//   - The machine is a package-level variable built with MustFreeze, and
//     every transition target has a registered StateBuilder.
//   - SetState type-checks the next state against the consumer's state
//     interface and is the only place the state field changes after
//     construction or restore. Restore stored objects with BuildState(label).
//   - A Condition only picks the edge, by calling business methods such as
//     HasRemainingRefundAmount; the behavior method accepts or rejects the
//     request. Guarded edges for one from + action come before any unguarded
//     edge. To stay in the current state on purpose, add a self-transition.
//   - Callers discard the business object when a business method returns an
//     error, since the behavior may already have changed it: roll the
//     transaction back and persist only after success.
//
// # Errors
//
// Inspect errors with errors.As. Transit returns a *TransitionError when no
// edge exists (Guarded false) or no Condition matched (Guarded true); a
// *StateBuilderError when the target has no builder or the builder returns nil
// or a state with another label; a *StateMachineDefinitionError for a nil
// context, machine, or current state; and any error from SetState. Check and
// Freeze report wiring problems as a *StateMachineCheckError, which MustFreeze
// turns into a panic. AddTransition and RegisterStateBuilder return a
// *StateMachineDefinitionError for empty input or a frozen machine.
//
// README.md walks through the order example step by step, and
// example_test.go runs it.
package fsm
