# Repository Guidance for Code Agents

This repository is a Go component library. Keep package changes small,
backward-compatible when practical, and verified with `go test ./...`.

## FSM Usage

When working with `github.com/go-jimu/components/fsm`, follow this contract:

- The core model is state polymorphism: the same business object delegates a
  behavior to its current state, and each concrete state implements that
  behavior differently.
- `StateMachine` owns transition definitions, exposes candidate transition
  edges, and builds target states. It does not own business behavior or execute
  transitions for business objects.
- The consumer's business object implements `fsm.StateContext` and owns the
  current state field.
- Use `fsm.Transit(ctx, sm, action)` as the standard transition flow. Do not
  reimplement the transition loop in each business object.
- The consumer should define a state behavior interface, such as `Pay()`,
  `Refund()`, and `Cancel()`, and concrete states should implement it.
- Prefer a base/default state that implements every behavior method with an
  error, then embed it in concrete states and override only supported behavior.
- Invalid behavior belongs in the concrete state method. For example,
  `PaidOrderState.Pay()` should return the "already paid" error.
- Do not move state-specific behavior into `if`/`else` or `switch` statements on
  the business object.
- `StateContext.SetState` belongs to the business object. It should type-check
  the built state, assign it to the object's current state field, and may record
  domain events or version changes.
- Business methods should call the current state's behavior method directly,
  pass natural parameters and return values, then call
  `fsm.Transit` through an unexported helper only after the behavior succeeds.
- Do not expose `fsm.StateMachine` as a parameter on business methods such as
  `Order.Pay(sm)`. Keep state-machine lookup and transition plumbing inside the
  object through an unexported helper.
- Do not use `HasTransition` as the primary permission check before calling the
  current state's behavior method; that bypasses state polymorphism.
- `Condition` is a transition guard evaluated by `fsm.Transit` to decide
  whether one specific `from -> to` edge is allowed. It is not a
  substitute for the business method's validation.
- A nil `Condition` is unconditional. For the same `from + action`, transitions
  are evaluated in add order and the first match wins.
- If no condition matches, `fsm.Transit` leaves the current state unchanged.
- Always register state builders for transition targets and call `Check` during
  setup or tests.

Read `fsm/README.md` and `fsm/example_test.go` before generating or modifying
FSM consumer code.
