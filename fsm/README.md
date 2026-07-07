# FSM

`fsm` is a small finite state machine package for polymorphic state objects.
Its core model is: the same business object delegates a behavior to its current
state, and different concrete states implement that behavior differently.

For example, an unpaid order and a paid order can both expose `Pay()`, but
`UnpaidOrderState.Pay()` performs the payment while `PaidOrderState.Pay()`
returns an error because the order is already paid.

In the order example below, an unpaid order cannot cancel or refund. A paid
order can be canceled, which means a full refund and a terminal canceled state.
A paid order can also receive partial refunds; if balance remains, it becomes a
partially refunded order. A partially refunded order can be refunded repeatedly,
can be canceled for the remaining balance, and cannot be paid again.

## Responsibilities

The FSM package owns:

- transition definitions: `from + action -> to`
- transition guards as edge metadata through `Condition`
- exposing candidate transition edges through `StateMachine.Transitions`
- constructing the target state through a registered `StateBuilder`
- the standard transition flow through `Transit`
- checking that transitions and builders are wired consistently

The consumer owns:

- the business object that implements `StateContext`
- the current state field on that object
- the state behavior interface, such as `Pay`, `Refund`, and `Cancel`
- a base/default state implementation whose behavior methods return errors
- concrete state implementations that accept, reject, or vary each behavior
- accepting the next state through `SetState`, including type checks, recording
  events, or incrementing versions
- concurrency control around the business object

## Setup Pattern

Define stable labels and actions:

```go
const (
	StateUnpaid            fsm.StateLabel = "order.unpaid"
	StatePaid              fsm.StateLabel = "order.paid"
	StatePartialRefunded   fsm.StateLabel = "order.partial_refunded"
	StateCanceled          fsm.StateLabel = "order.canceled"

	ActionPay    fsm.Action = "pay"
	ActionRefund fsm.Action = "refund"
	ActionCancel fsm.Action = "cancel"
)
```

Define a state behavior interface and concrete states:

```go
type OrderState interface {
	fsm.State
	Pay(amount int, method string) (string, error)
	Refund(amount int) error
	Cancel() error
}

type BaseOrderState struct {
	*fsm.SimpleState
}

func (state *BaseOrderState) Pay(int, string) (string, error) {
	return fmt.Errorf("%s cannot pay", state.Label())
}

func (state *BaseOrderState) Refund(int) error {
	return fmt.Errorf("%s cannot refund", state.Label())
}

func (state *BaseOrderState) Cancel() error {
	return fmt.Errorf("%s cannot cancel", state.Label())
}

type UnpaidOrderState struct {
	BaseOrderState
}

func (state *UnpaidOrderState) Pay(amount int, method string) (string, error) {
	order := state.Context().(*Order)
	order.paidAmount = amount
	order.paymentMethod = method
	return fmt.Sprintf("receipt:%s:%d", method, amount), nil
}

type PaidOrderState struct {
	BaseOrderState
}

func (state *PaidOrderState) Pay(int, string) (string, error) {
	return "", errors.New("order has already been paid")
}

func (state *PaidOrderState) Refund(amount int) error {
	order := state.Context().(*Order)
	if amount > order.remainingAmount() {
		return errors.New("refund amount exceeds remaining amount")
	}
	order.refundedAmount += amount
	return nil
}

func (state *PaidOrderState) Cancel() error {
	order := state.Context().(*Order)
	order.refundedAmount = order.paidAmount
	order.canceled = true
	return nil
}

type PartialRefundedOrderState struct {
	BaseOrderState
}

func (state *PartialRefundedOrderState) Pay(int, string) (string, error) {
	return "", errors.New("partially refunded order cannot be paid again")
}

func (state *PartialRefundedOrderState) Refund(amount int) error {
	order := state.Context().(*Order)
	if amount > order.remainingAmount() {
		return errors.New("refund amount exceeds remaining amount")
	}
	order.refundedAmount += amount
	return nil
}

func (state *PartialRefundedOrderState) Cancel() error {
	order := state.Context().(*Order)
	order.refundedAmount = order.paidAmount
	order.canceled = true
	return nil
}
```

Register every state builder used by transitions, then add transitions. The
registered builder should return the concrete state implementation for that
label:

```go
sm := fsm.NewStateMachine("order")
sm.RegisterStateBuilder(StateUnpaid, func() fsm.State {
	return &UnpaidOrderState{
		BaseOrderState: BaseOrderState{
			SimpleState: fsm.NewSimpleState(StateUnpaid),
		},
	}
})
sm.RegisterStateBuilder(StatePaid, func() fsm.State {
	return &PaidOrderState{
		BaseOrderState: BaseOrderState{
			SimpleState: fsm.NewSimpleState(StatePaid),
		},
	}
})
sm.RegisterStateBuilder(StatePartialRefunded, NewPartialRefundedOrderState)
sm.RegisterStateBuilder(StateCanceled, NewCanceledOrderState)
sm.AddTransition(StateUnpaid, StatePaid, ActionPay, nil)
sm.AddTransition(StatePaid, StatePartialRefunded, ActionRefund, func(sc fsm.StateContext) bool {
	return sc.(*Order).HasRemainingRefundAmount()
})
sm.AddTransition(StatePaid, StateCanceled, ActionRefund, func(sc fsm.StateContext) bool {
	return sc.(*Order).IsFullyRefunded()
})
sm.AddTransition(StatePaid, StateCanceled, ActionCancel, nil)
sm.AddTransition(StatePartialRefunded, StatePartialRefunded, ActionRefund, func(sc fsm.StateContext) bool {
	return sc.(*Order).HasRemainingRefundAmount()
})
sm.AddTransition(StatePartialRefunded, StateCanceled, ActionRefund, func(sc fsm.StateContext) bool {
	return sc.(*Order).IsFullyRefunded()
})
sm.AddTransition(StatePartialRefunded, StateCanceled, ActionCancel, nil)

if err := sm.Check(); err != nil {
	return err
}
fsm.RegisterStateMachine(sm)
```

Call `Check` during startup or in tests. It catches missing state builders and
registered builders that are not referenced by any transition.

## Runtime Pattern

The business object implements `StateContext`:

```go
type Order struct {
	stateMachineName string
	state            OrderState
	paidAmount     int
	paymentMethod  string
	refundedAmount int
	canceled       bool
}

func (order *Order) remainingAmount() int {
	return order.paidAmount - order.refundedAmount
}

func (order *Order) HasRemainingRefundAmount() bool {
	return order.remainingAmount() > 0
}

func (order *Order) IsFullyRefunded() bool {
	return order.remainingAmount() == 0
}

func NewOrder() *Order {
	initial := NewUnpaidOrderState().(OrderState)
	order := &Order{
		stateMachineName: "order",
		state:            initial,
	}
	initial.SetContext(order)
	return order
}

func (order *Order) CurrentState() fsm.State {
	return order.state
}

func (order *Order) transition(action fsm.Action) error {
	return fsm.Transit(order, fsm.MustGetStateMachine(order.stateMachineName), action)
}

func (order *Order) SetState(next fsm.State) error {
	state, ok := next.(OrderState)
	if !ok {
		return errors.New("next state is not an order state")
	}
	order.state = state
	return nil
}
```

A business operation should usually follow this order:

```go
func (order *Order) Pay(amount int, method string) (string, error) {
	receipt, err := order.CurrentState().(OrderState).Pay(amount, method)
	if err != nil {
		return "", err
	}
	if err := order.transition(ActionPay); err != nil {
		return "", err
	}
	return receipt, nil
}

func (order *Order) Refund(amount int) error {
	if err := order.CurrentState().(OrderState).Refund(amount); err != nil {
		return err
	}
	return order.transition(ActionRefund)
}

func (order *Order) Cancel() error {
	if err := order.CurrentState().(OrderState).Cancel(); err != nil {
		return err
	}
	return order.transition(ActionCancel)
}
```

Do not put the business side effect in `StateMachine`, and do not use
`HasTransition` as the primary permission check before calling the state method.
The concrete state method decides whether the behavior is allowed. For an
already-paid order, `PaidOrderState.Pay()` should return the business error.
After the state behavior succeeds, the private `transition` helper asks
`fsm.Transit` to perform the standard state-change flow. `Transit` reads
candidate edges from `StateMachine.Transitions`, evaluates each edge's
`Condition`, builds the matched state with `BuildState`, calls
`next.SetContext(order)`, and delegates assignment to `order.SetState`.

Keep FSM plumbing out of the public business method signature. Callers should
call `order.Pay()`, not `order.Pay(sm)`.

Use `fsm.Transit` for the transition loop instead of reimplementing it on every
business object. Keep business-specific acceptance rules in `SetState`, where
the aggregate can type-check the next state and record any domain events or
version changes.

A useful pattern is to embed a base state that implements every behavior method
with a default error. Each concrete state overrides only the behaviors it
supports or wants to reject with a more specific error. That keeps state-specific
behavior on the state type itself instead of accumulating `if`/`else` or
`switch` checks on the business object.

## Conditions

`Condition` is a transition guard: it decides whether a specific transition edge
from one state to another state is allowed. It belongs in the transition table,
but is evaluated by `fsm.Transit`, not by `StateMachine`.

```go
sm.AddTransition(StatePaid, StatePartialRefunded, ActionRefund, func(sc fsm.StateContext) bool {
	order := sc.(*Order)
	return order.HasRemainingRefundAmount()
})
sm.AddTransition(StatePaid, StateCanceled, ActionRefund, func(sc fsm.StateContext) bool {
	order := sc.(*Order)
	return order.IsFullyRefunded()
})
```

Prefer calling business methods on the aggregate instead of reading or
recomputing its fields in transition registration code.

The call chain is explicit: `order.Refund(amount)` first delegates to
`CurrentState().(OrderState).Refund(amount)`. If that method succeeds, it calls
the private `order.transition(ActionRefund)` helper. That helper calls
`fsm.Transit(order, sm, ActionRefund)`. `Transit` reads candidates from
`StateMachine.Transitions`, evaluates each registered `Condition` with the order
as `StateContext`, builds the matched state with `BuildState`, sets the next
state's context, and calls `order.SetState`.

Important details:

- `nil` condition means unconditional.
- Transitions for the same `from + action` are checked in the order they were
  added.
- The first matching transition wins.
- If no condition matches, `Transit` leaves the current state unchanged.
- Put unconditional transitions after conditional transitions when both share
  the same `from + action`.

## Common Agent Mistakes

- Treating the transition table as the behavior model. The behavior model is
  the current state's method implementation.
- Putting state-specific behavior behind `if`/`else` or `switch` in the
  business object instead of overriding methods on concrete states.
- Forgetting to give the base state default error implementations for behavior
  methods that are unavailable in most states.
- Calling `HasTransition` before the state method and accidentally bypassing
  errors from states like `PaidOrderState.Pay()`.
- Calling the transition helper before executing the current state's business
  method.
- Updating the business object's state without going through
  `fsm.Transit`.
- Forgetting to call `SetContext` on the state returned by `BuildState` inside
  `Transit`.
- Treating `Condition` as the only business validation instead of a transition
  guard for one `from -> to` edge.
- Registering a transition target without a matching `StateBuilder`.
- Registering only `fsm.NewSimpleState` for states that need behavior methods.
- Adding an unconditional transition before a conditional transition for the
  same `from + action`.

See `example_test.go` for an executable usage example.
