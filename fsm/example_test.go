package fsm_test

import (
	"errors"
	"fmt"

	"github.com/go-jimu/components/fsm"
)

const (
	exampleStateMachineName                    = "order"
	exampleStateUnpaid          fsm.StateLabel = "order.unpaid"
	exampleStatePaid            fsm.StateLabel = "order.paid"
	exampleStatePartialRefunded fsm.StateLabel = "order.partial_refunded"
	exampleStateCanceled        fsm.StateLabel = "order.canceled"
	exampleActionPay            fsm.Action     = "pay"
	exampleActionRefund         fsm.Action     = "refund"
	exampleActionCancel         fsm.Action     = "cancel"
)

// exampleOrderMachine is built, checked, and frozen once, like a domain
// package would do with a package-level variable.
var exampleOrderMachine = fsm.MustFreeze(newExampleOrderStateMachine())

type exampleOrder struct {
	machine        fsm.RuntimeStateMachine
	state          exampleOrderState
	paidAmount     int
	paymentMethod  string
	refundedAmount int
	canceled       bool
}

func (order *exampleOrder) remainingAmount() int {
	return order.paidAmount - order.refundedAmount
}

func (order *exampleOrder) hasRemainingRefundAmount() bool {
	return order.remainingAmount() > 0
}

func (order *exampleOrder) isFullyRefunded() bool {
	return order.remainingAmount() == 0
}

// exampleOrderState is the behavior interface. Each method receives the order
// it acts on, so states hold no reference to an order.
type exampleOrderState interface {
	fsm.State
	Pay(order *exampleOrder, amount int, method string) (string, error)
	Refund(order *exampleOrder, amount int) error
	Cancel(order *exampleOrder) error
}

type exampleBaseOrderState struct {
	*fsm.SimpleState
}

type exampleUnpaidOrderState struct {
	exampleBaseOrderState
}

type examplePaidOrderState struct {
	exampleBaseOrderState
}

type examplePartialRefundedOrderState struct {
	exampleBaseOrderState
}

type exampleCanceledOrderState struct {
	exampleBaseOrderState
}

func newExampleOrder() *exampleOrder {
	return &exampleOrder{
		machine: exampleOrderMachine,
		state: &exampleUnpaidOrderState{
			exampleBaseOrderState: exampleBaseOrderState{SimpleState: fsm.NewSimpleState(exampleStateUnpaid)},
		},
	}
}

func (order *exampleOrder) CurrentState() fsm.State {
	return order.state
}

func (order *exampleOrder) transition(action fsm.Action) error {
	return fsm.Transit(order, order.machine, action)
}

func (order *exampleOrder) SetState(next fsm.State) error {
	state, ok := next.(exampleOrderState)
	if !ok {
		return errors.New("next state is not an order state")
	}
	order.state = state
	return nil
}

func (order *exampleOrder) Pay(amount int, method string) (string, error) {
	receipt, err := order.state.Pay(order, amount, method)
	if err != nil {
		return "", err
	}
	if err := order.transition(exampleActionPay); err != nil {
		return "", err
	}
	return receipt, nil
}

func (order *exampleOrder) Refund(amount int) error {
	if err := order.state.Refund(order, amount); err != nil {
		return err
	}
	return order.transition(exampleActionRefund)
}

func (order *exampleOrder) Cancel() error {
	if err := order.state.Cancel(order); err != nil {
		return err
	}
	return order.transition(exampleActionCancel)
}

func (state *exampleBaseOrderState) Pay(*exampleOrder, int, string) (string, error) {
	return "", fmt.Errorf("%s cannot pay", state.Label())
}

func (state *exampleBaseOrderState) Refund(*exampleOrder, int) error {
	return fmt.Errorf("%s cannot refund", state.Label())
}

func (state *exampleBaseOrderState) Cancel(*exampleOrder) error {
	return fmt.Errorf("%s cannot cancel", state.Label())
}

func (state *exampleUnpaidOrderState) Pay(order *exampleOrder, amount int, method string) (string, error) {
	if amount <= 0 {
		return "", errors.New("payment amount must be positive")
	}
	order.paidAmount = amount
	order.paymentMethod = method
	return fmt.Sprintf("receipt:%s:%d", method, amount), nil
}

func (state *examplePaidOrderState) Pay(*exampleOrder, int, string) (string, error) {
	return "", errors.New("order has already been paid")
}

func (state *examplePaidOrderState) Refund(order *exampleOrder, amount int) error {
	return refundExampleOrder(order, amount)
}

func (state *examplePaidOrderState) Cancel(order *exampleOrder) error {
	cancelExampleOrder(order)
	return nil
}

func (state *examplePartialRefundedOrderState) Pay(*exampleOrder, int, string) (string, error) {
	return "", errors.New("partially refunded order cannot be paid again")
}

func (state *examplePartialRefundedOrderState) Refund(order *exampleOrder, amount int) error {
	return refundExampleOrder(order, amount)
}

func (state *examplePartialRefundedOrderState) Cancel(order *exampleOrder) error {
	cancelExampleOrder(order)
	return nil
}

func refundExampleOrder(order *exampleOrder, amount int) error {
	if amount <= 0 {
		return errors.New("refund amount must be positive")
	}
	if amount > order.remainingAmount() {
		return errors.New("refund amount exceeds remaining amount")
	}
	order.refundedAmount += amount
	return nil
}

func cancelExampleOrder(order *exampleOrder) {
	order.refundedAmount = order.paidAmount
	order.canceled = true
}

func newExampleUnpaidOrderState() fsm.State {
	return &exampleUnpaidOrderState{
		exampleBaseOrderState: exampleBaseOrderState{
			SimpleState: fsm.NewSimpleState(exampleStateUnpaid),
		},
	}
}

func newExamplePaidOrderState() fsm.State {
	return &examplePaidOrderState{
		exampleBaseOrderState: exampleBaseOrderState{
			SimpleState: fsm.NewSimpleState(exampleStatePaid),
		},
	}
}

func newExamplePartialRefundedOrderState() fsm.State {
	return &examplePartialRefundedOrderState{
		exampleBaseOrderState: exampleBaseOrderState{
			SimpleState: fsm.NewSimpleState(exampleStatePartialRefunded),
		},
	}
}

func newExampleCanceledOrderState() fsm.State {
	return &exampleCanceledOrderState{
		exampleBaseOrderState: exampleBaseOrderState{
			SimpleState: fsm.NewSimpleState(exampleStateCanceled),
		},
	}
}

func newExampleOrderStateMachine() fsm.StateMachine {
	sm := fsm.NewStateMachine(exampleStateMachineName)
	sm.RegisterStateBuilder(exampleStateUnpaid, newExampleUnpaidOrderState)
	sm.RegisterStateBuilder(exampleStatePaid, newExamplePaidOrderState)
	sm.RegisterStateBuilder(exampleStatePartialRefunded, newExamplePartialRefundedOrderState)
	sm.RegisterStateBuilder(exampleStateCanceled, newExampleCanceledOrderState)
	sm.AddTransition(exampleStateUnpaid, exampleStatePaid, exampleActionPay, nil)
	sm.AddTransition(exampleStatePaid, exampleStatePartialRefunded, exampleActionRefund, func(sc fsm.StateContext) bool {
		return sc.(*exampleOrder).hasRemainingRefundAmount()
	})
	sm.AddTransition(exampleStatePaid, exampleStateCanceled, exampleActionRefund, func(sc fsm.StateContext) bool {
		return sc.(*exampleOrder).isFullyRefunded()
	})
	sm.AddTransition(exampleStatePaid, exampleStateCanceled, exampleActionCancel, nil)
	sm.AddTransition(exampleStatePartialRefunded, exampleStatePartialRefunded, exampleActionRefund, func(sc fsm.StateContext) bool {
		return sc.(*exampleOrder).hasRemainingRefundAmount()
	})
	sm.AddTransition(exampleStatePartialRefunded, exampleStateCanceled, exampleActionRefund, func(sc fsm.StateContext) bool {
		return sc.(*exampleOrder).isFullyRefunded()
	})
	sm.AddTransition(exampleStatePartialRefunded, exampleStateCanceled, exampleActionCancel, nil)
	return sm
}

func ExampleMustFreeze_orderPayment() {
	paidOrder := newExampleOrder()
	receipt, _ := paidOrder.Pay(42, "card")
	fmt.Println(receipt, paidOrder.CurrentState().Label())
	_ = paidOrder.Refund(12)
	fmt.Println(paidOrder.CurrentState().Label(), paidOrder.refundedAmount, paidOrder.remainingAmount())
	_ = paidOrder.Refund(10)
	fmt.Println(paidOrder.CurrentState().Label(), paidOrder.refundedAmount, paidOrder.remainingAmount())
	fmt.Println(paidOrder.Pay(5, "card"))
	_ = paidOrder.Cancel()
	fmt.Println(paidOrder.CurrentState().Label(), paidOrder.refundedAmount, paidOrder.canceled)

	unpaidOrder := newExampleOrder()
	fmt.Println(unpaidOrder.Cancel())
	_, _ = unpaidOrder.Pay(10, "cash")
	_ = unpaidOrder.Cancel()
	fmt.Println(unpaidOrder.CurrentState().Label(), unpaidOrder.refundedAmount, unpaidOrder.canceled)
	// Output:
	// receipt:card:42 order.paid
	// order.partial_refunded 12 30
	// order.partial_refunded 22 20
	//  partially refunded order cannot be paid again
	// order.canceled 42 true
	// order.unpaid cannot cancel
	// order.canceled 10 true
}
