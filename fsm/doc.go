// Package fsm provides small finite state machine primitives for polymorphic
// state objects.
//
// The central model is: the same business object delegates a behavior to its
// current State, and different concrete states implement that behavior
// differently. For example, UnpaidOrderState.Pay may perform the payment, while
// PaidOrderState.Pay returns an error because the order is already paid.
//
// The package owns transition definitions and target state construction after a
// behavior has succeeded. The consumer owns the business object, its current
// concrete state, state-specific behavior methods, and the side effects that
// happen during a transition.
//
// A common consumer pattern is to define a base state whose behavior methods
// return default errors, then embed it in concrete states and override only the
// methods supported by that state. That keeps state-specific behavior in
// concrete state types instead of adding if/else or switch checks to the
// business object.
//
// The usual setup flow is:
//
//   - create a StateMachine with NewStateMachine
//   - register every state builder used by transitions
//   - add transitions with AddTransition
//   - call Check during application startup or tests
//   - call Freeze, or register the machine with RegisterStateMachine, before
//     using it at runtime
//
// The usual runtime flow is:
//
//   - the business object implements StateContext
//   - each business method type-asserts CurrentState to the business state
//     interface and executes behavior with natural parameters and return values
//   - if behavior succeeds, call Transit with the business object, a
//     RuntimeStateMachine, and action, usually through a private helper on the
//     business object
//   - Transit gets candidate edges from the RuntimeStateMachine, evaluates their
//     conditions, builds the matching state, sets its context, and delegates
//     state replacement to StateContext.SetState
//
// Keep state-machine lookup and transition plumbing out of public business
// method signatures. Prefer methods such as Order.Pay() that delegate to the
// current state and then transition after the behavior succeeds.
//
// StateContext.SetState is intentionally implemented by the business object so
// it can type-check the next state and keep any domain side effects local.
//
// Do not use HasTransition as the primary business permission check before
// calling the current state's behavior method. That bypasses state polymorphism:
// an invalid behavior should normally be rejected by the concrete state method
// itself.
//
// A Condition is a transition guard evaluated by Transit to decide whether a
// specific from-state to to-state edge is allowed. A nil condition is
// unconditional. If transitions exist for the current state and action but no
// condition matches, Transit leaves the current state unchanged.
//
// StateMachine protects its transition and builder tables with a mutex and can
// be frozen into the read-only RuntimeStateMachine surface, but it does not
// synchronize the consumer's StateContext. The consumer is responsible for any
// concurrency control around its business object.
package fsm
