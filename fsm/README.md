# FSM

`fsm` is a small finite state machine package built on **state polymorphism**:
a business object delegates each behavior to its current state, and each
concrete state implements that behavior its own way. An unpaid order's `Pay`
takes the payment; a paid order's `Pay` returns "order has already been paid".

The running example is an order. An unpaid order can only be paid. A paid order
can be refunded in part, which makes it partially refunded, or canceled, which
refunds it in full. A partially refunded order can be refunded again or
canceled, and never paid again. `example_test.go` is the executable version.

The rules this walkthrough satisfies, and the bar for done in your own code, are
"Rules for consumer code" in `go doc -all github.com/go-jimu/components/fsm`;
error types are under "Errors" there.

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
transitions, and freeze the result into a package-level variable.
`RegisterStateBuilder` and `AddTransition` fail only on empty input or a frozen
machine, so join their errors and panic. `MustFreeze` runs `Check`, so a
miswired machine panics at startup; call `Freeze` instead to get the error.

```go
var orderMachine = fsm.MustFreeze(newOrderStateMachine())

func newOrderStateMachine() fsm.StateMachine {
	sm := fsm.NewStateMachine("order")
	if err := errors.Join(
		sm.RegisterStateBuilder(StateUnpaid, func() fsm.State {
			return &UnpaidOrderState{BaseOrderState{fsm.NewSimpleState(StateUnpaid)}}
		}),
		sm.RegisterStateBuilder(StatePaid, func() fsm.State {
			return &PaidOrderState{BaseOrderState{fsm.NewSimpleState(StatePaid)}}
		}),
		sm.RegisterStateBuilder(StatePartialRefunded, func() fsm.State {
			return &PartialRefundedOrderState{BaseOrderState{fsm.NewSimpleState(StatePartialRefunded)}}
		}),
		sm.RegisterStateBuilder(StateCanceled, func() fsm.State {
			return &CanceledOrderState{BaseOrderState{fsm.NewSimpleState(StateCanceled)}}
		}),
		sm.AddTransition(StateUnpaid, StatePaid, ActionPay, nil),
		sm.AddTransition(StatePaid, StatePartialRefunded, ActionRefund, func(sc fsm.StateContext) bool {
			return sc.(*Order).HasRemainingRefundAmount()
		}),
		sm.AddTransition(StatePaid, StateCanceled, ActionRefund, func(sc fsm.StateContext) bool {
			return sc.(*Order).IsFullyRefunded()
		}),
		sm.AddTransition(StatePaid, StateCanceled, ActionCancel, nil),
		sm.AddTransition(StatePartialRefunded, StatePartialRefunded, ActionRefund, func(sc fsm.StateContext) bool {
			return sc.(*Order).HasRemainingRefundAmount()
		}),
		sm.AddTransition(StatePartialRefunded, StateCanceled, ActionRefund, func(sc fsm.StateContext) bool {
			return sc.(*Order).IsFullyRefunded()
		}),
		sm.AddTransition(StatePartialRefunded, StateCanceled, ActionCancel, nil),
	); err != nil {
		panic(err)
	}
	return sm
}
```

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
