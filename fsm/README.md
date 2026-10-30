# FSM

`fsm` is a small finite state machine package built on **state polymorphism**:
a business object delegates each behavior to its current state, and each
concrete state implements that behavior its own way. An unpaid order's `Pay`
takes the payment; a paid order's `Pay` returns "order has already been paid".

Every business method runs **behavior first, transition second**: the current
state performs or rejects the behavior, and only after it succeeds does
`fsm.Transit` move the object to the next state.

The running example is an order. An unpaid order can only be paid. A paid order
can be refunded in part, which makes it partially refunded, or canceled, which
refunds it in full. A partially refunded order can be refunded again or
canceled, and never paid again. `example_test.go` is the executable version.

## Responsibilities

`fsm` owns the transition table: `from + action -> to` edges, their guards
(`Condition`), building the target state from a registered `StateBuilder`,
checking that edges and builders agree, and the transition flow in `Transit`.

The consumer owns:

- the business object, which implements `StateContext` and holds the current
  state field
- the state behavior interface, whose methods take the business object as a
  parameter
- a base state whose behavior methods return errors, and concrete states that
  override the behaviors they support
- `SetState`, which type-checks the next state and may record domain events or
  bump a version
- concurrency control around the business object

## Steps

### 1. Define labels and actions

```go
const (
	StateUnpaid          fsm.StateLabel = "order.unpaid"
	StatePaid            fsm.StateLabel = "order.paid"
	StatePartialRefunded fsm.StateLabel = "order.partial_refunded"
	StateCanceled        fsm.StateLabel = "order.canceled"

	ActionPay    fsm.Action = "pay"
	ActionRefund fsm.Action = "refund"
	ActionCancel fsm.Action = "cancel"
)
```

### 2. Define the behavior interface and states

Each behavior method receives the order it acts on. States hold only their
label, so restoring an order is `BuildState(label)` and copying an order is
safe.

```go
type OrderState interface {
	fsm.State
	Pay(order *Order, amount int, method string) (string, error)
	Refund(order *Order, amount int) error
	Cancel(order *Order) error
}

// BaseOrderState rejects every behavior; concrete states override what they support.
type BaseOrderState struct {
	*fsm.SimpleState
}

func (state *BaseOrderState) Pay(*Order, int, string) (string, error) {
	return "", fmt.Errorf("%s cannot pay", state.Label())
}

func (state *BaseOrderState) Refund(*Order, int) error {
	return fmt.Errorf("%s cannot refund", state.Label())
}

func (state *BaseOrderState) Cancel(*Order) error {
	return fmt.Errorf("%s cannot cancel", state.Label())
}

type UnpaidOrderState struct{ BaseOrderState }

func (state *UnpaidOrderState) Pay(order *Order, amount int, method string) (string, error) {
	order.paidAmount = amount
	order.paymentMethod = method
	return fmt.Sprintf("receipt:%s:%d", method, amount), nil
}

type PaidOrderState struct{ BaseOrderState }

func (state *PaidOrderState) Pay(*Order, int, string) (string, error) {
	return "", errors.New("order has already been paid")
}

func (state *PaidOrderState) Refund(order *Order, amount int) error {
	return order.refund(amount)
}

func (state *PaidOrderState) Cancel(order *Order) error {
	order.cancel()
	return nil
}

type PartialRefundedOrderState struct{ BaseOrderState }

func (state *PartialRefundedOrderState) Pay(*Order, int, string) (string, error) {
	return "", errors.New("partially refunded order cannot be paid again")
}

func (state *PartialRefundedOrderState) Refund(order *Order, amount int) error {
	return order.refund(amount)
}

func (state *PartialRefundedOrderState) Cancel(order *Order) error {
	order.cancel()
	return nil
}

type CanceledOrderState struct{ BaseOrderState }
```

### 3. Build the machine once

Register a builder for every state that appears in a transition, add the
transitions, and freeze the result into a package-level variable. `MustFreeze`
runs `Check` and panics on a miswired machine, so wiring errors surface at
startup: a missing or unreferenced builder, or one that returns nil or a state
with the wrong label. Call `Freeze` instead when you want the error returned.

```go
var orderMachine = fsm.MustFreeze(newOrderStateMachine())

func newOrderStateMachine() fsm.StateMachine {
	sm := fsm.NewStateMachine("order")
	sm.RegisterStateBuilder(StateUnpaid, func() fsm.State {
		return &UnpaidOrderState{BaseOrderState{fsm.NewSimpleState(StateUnpaid)}}
	})
	sm.RegisterStateBuilder(StatePaid, func() fsm.State {
		return &PaidOrderState{BaseOrderState{fsm.NewSimpleState(StatePaid)}}
	})
	sm.RegisterStateBuilder(StatePartialRefunded, func() fsm.State {
		return &PartialRefundedOrderState{BaseOrderState{fsm.NewSimpleState(StatePartialRefunded)}}
	})
	sm.RegisterStateBuilder(StateCanceled, func() fsm.State {
		return &CanceledOrderState{BaseOrderState{fsm.NewSimpleState(StateCanceled)}}
	})

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
	return sm
}
```

The package-level variable replaces the deprecated registry
(`RegisterStateMachine`, `GetStateMachine`, `MustGetStateMachine`): the
dependency is a compile-time reference instead of a string looked up at
runtime.

### 4. Implement the business object

```go
type Order struct {
	state          OrderState
	paidAmount     int
	paymentMethod  string
	refundedAmount int
	canceled       bool
}

func NewOrder() *Order {
	return &Order{state: &UnpaidOrderState{BaseOrderState{fsm.NewSimpleState(StateUnpaid)}}}
}

func (order *Order) remainingAmount() int          { return order.paidAmount - order.refundedAmount }
func (order *Order) HasRemainingRefundAmount() bool { return order.remainingAmount() > 0 }
func (order *Order) IsFullyRefunded() bool          { return order.remainingAmount() == 0 }

func (order *Order) refund(amount int) error {
	if amount > order.remainingAmount() {
		return errors.New("refund amount exceeds remaining amount")
	}
	order.refundedAmount += amount
	return nil
}

func (order *Order) cancel() {
	order.refundedAmount = order.paidAmount
	order.canceled = true
}

func (order *Order) CurrentState() fsm.State { return order.state }

func (order *Order) SetState(next fsm.State) error {
	state, ok := next.(OrderState)
	if !ok {
		return errors.New("next state is not an order state")
	}
	order.state = state
	return nil
}

func (order *Order) transition(action fsm.Action) error {
	return fsm.Transit(order, orderMachine, action)
}

// Behavior first, transition second.
func (order *Order) Pay(amount int, method string) (string, error) {
	receipt, err := order.state.Pay(order, amount, method)
	if err != nil {
		return "", err
	}
	if err := order.transition(ActionPay); err != nil {
		return "", err
	}
	return receipt, nil
}

func (order *Order) Refund(amount int) error {
	if err := order.state.Refund(order, amount); err != nil {
		return err
	}
	return order.transition(ActionRefund)
}

func (order *Order) Cancel() error {
	if err := order.state.Cancel(order); err != nil {
		return err
	}
	return order.transition(ActionCancel)
}
```

Restore an order from storage with its stored label:

```go
state, err := orderMachine.BuildState(row.StateLabel)
if err != nil {
	return nil, err
}
order := &Order{state: state.(OrderState), paidAmount: row.PaidAmount /* ... */}
```

## Transit and guards

`Transit(order, machine, action)` reads the candidate edges for the current
state and action, evaluates each edge's `Condition` with the order, builds the
first matching target state with `BuildState`, and hands it to
`order.SetState`.

A `Condition` is a **guard** on one `from -> to` edge. It picks the edge after
the behavior already succeeded; the behavior method stays the place that
accepts or rejects the business request. Guards call business methods on the
aggregate, such as `HasRemainingRefundAmount`.

- A nil `Condition` always matches.
- Edges for the same `from + action` are evaluated in the order they were
  added, and the first match wins, so add guarded edges before an unguarded
  one.
- When edges exist but no guard matches, `Transit` returns a `*TransitionError`
  with `Guarded` set and the state stays unchanged. To stay in the current
  state on purpose, add a self-transition such as
  `PartialRefunded -> PartialRefunded`.
- When no edge exists for the state and action, `Transit` returns a
  `*TransitionError` without `Guarded`.

The behavior has already changed the aggregate when `Transit` runs. When a
business method returns an error, from the behavior or from `Transit`, discard
the aggregate by rolling back the transaction; persist only aggregates whose
business method succeeded.

## Checklist

Consumer code is done when every item holds:

- [ ] Each state-specific decision lives in a concrete state's behavior method;
      the business object holds no `if`/`switch` on its current state.
- [ ] Behavior methods take the business object as a parameter; states hold
      only their label.
- [ ] A base state returns an error from every behavior, and each concrete
      state overrides only the behaviors it supports or rejects more
      specifically.
- [ ] Every business method calls its state's behavior first and
      `fsm.Transit` second, through an unexported helper.
- [ ] Public business methods take natural parameters, such as
      `order.Pay(amount, method)`; the machine stays inside the object.
- [ ] The machine is a package-level variable built with `fsm.MustFreeze`, and
      every transition target has a registered builder.
- [ ] `SetState` type-checks the next state and is the only place the state
      field changes after construction or restore.
- [ ] Guarded edges for one `from + action` come before any unguarded edge.
- [ ] Callers discard the aggregate when a business method returns an error.
