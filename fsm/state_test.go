package fsm_test

import (
	"testing"

	"github.com/go-jimu/components/fsm"
	"github.com/stretchr/testify/require"
)

func TestStateMachineCheckReportsOrderWiringErrors(t *testing.T) {
	exampleStateArchived := fsm.StateLabel("order.archived")
	exampleStateReturned := fsm.StateLabel("order.returned")
	sm := fsm.NewStateMachine("order_check")
	sm.RegisterStateBuilder(exampleStateUnpaid, newExampleUnpaidOrderState)
	sm.RegisterStateBuilder(exampleStateCanceled, newExampleCanceledOrderState)
	sm.RegisterStateBuilder(exampleStateArchived, func() fsm.State {
		return fsm.NewSimpleState(exampleStateArchived)
	})
	sm.AddTransition(exampleStateUnpaid, exampleStatePaid, exampleActionPay, nil)
	sm.AddTransition(exampleStateUnpaid, exampleStateReturned, exampleActionCancel, nil)

	var err error
	require.NotPanics(t, func() {
		err = sm.Check()
	})

	require.Error(t, err)
	require.EqualError(t, err, "state machine check failed")
	var checkErr *fsm.StateMachineCheckError
	require.ErrorAs(t, err, &checkErr)
	require.Equal(t, []fsm.StateLabel{exampleStatePaid, exampleStateReturned}, checkErr.MissingStateBuilders)
	require.Equal(t, []fsm.StateLabel{exampleStateArchived, exampleStateCanceled}, checkErr.UnreferencedStateBuilders)
}

func TestOrderPayDelegatesToUnpaidStateThenTransitionsToPaid(t *testing.T) {
	order := newExampleOrder()

	receipt, err := order.Pay(42, "card")

	require.NoError(t, err)
	require.Equal(t, "receipt:card:42", receipt)
	require.Equal(t, exampleStatePaid, order.CurrentState().Label())
	require.Equal(t, 42, order.paidAmount)
	require.Equal(t, "card", order.paymentMethod)
}

func TestPaidOrderRejectsSecondPayFromStateMethod(t *testing.T) {
	order := newExampleOrder()
	_, err := order.Pay(42, "card")
	require.NoError(t, err)

	receipt, err := order.Pay(5, "cash")

	require.Error(t, err)
	require.Empty(t, receipt)
	require.EqualError(t, err, "order has already been paid")
	require.Equal(t, exampleStatePaid, order.CurrentState().Label())
	require.Equal(t, 42, order.paidAmount)
	require.Equal(t, "card", order.paymentMethod)
}

func TestUnpaidOrderRejectsCancelFromStateMethod(t *testing.T) {
	order := newExampleOrder()

	err := order.Cancel()

	require.EqualError(t, err, "order.unpaid cannot cancel")
	require.Equal(t, exampleStateUnpaid, order.CurrentState().Label())
	require.False(t, order.canceled)
}

func TestOrderRefundCanStayPartialAcrossMultipleRefunds(t *testing.T) {
	order := newExampleOrder()
	_, err := order.Pay(42, "card")
	require.NoError(t, err)

	require.NoError(t, order.Refund(12))
	require.Equal(t, exampleStatePartialRefunded, order.CurrentState().Label())
	require.Equal(t, 12, order.refundedAmount)
	require.Equal(t, 30, order.remainingAmount())

	require.NoError(t, order.Refund(10))
	require.Equal(t, exampleStatePartialRefunded, order.CurrentState().Label())
	require.Equal(t, 22, order.refundedAmount)
	require.Equal(t, 20, order.remainingAmount())
}

func TestOrderTransitionCallsConditionsAfterStateBehavior(t *testing.T) {
	const machineName = "order_condition_called"

	var paidToPartialCalls int
	var paidToCanceledCalls int
	var partialToPartialCalls int
	var partialToCanceledCalls int

	sm := fsm.NewStateMachine(machineName)
	sm.RegisterStateBuilder(exampleStateUnpaid, newExampleUnpaidOrderState)
	sm.RegisterStateBuilder(exampleStatePaid, newExamplePaidOrderState)
	sm.RegisterStateBuilder(exampleStatePartialRefunded, newExamplePartialRefundedOrderState)
	sm.RegisterStateBuilder(exampleStateCanceled, newExampleCanceledOrderState)
	sm.AddTransition(exampleStateUnpaid, exampleStatePaid, exampleActionPay, nil)
	sm.AddTransition(exampleStatePaid, exampleStatePartialRefunded, exampleActionRefund, func(sc fsm.StateContext) bool {
		paidToPartialCalls++
		return sc.(*exampleOrder).hasRemainingRefundAmount()
	})
	sm.AddTransition(exampleStatePaid, exampleStateCanceled, exampleActionRefund, func(sc fsm.StateContext) bool {
		paidToCanceledCalls++
		return sc.(*exampleOrder).isFullyRefunded()
	})
	sm.AddTransition(exampleStatePartialRefunded, exampleStatePartialRefunded, exampleActionRefund, func(sc fsm.StateContext) bool {
		partialToPartialCalls++
		return sc.(*exampleOrder).hasRemainingRefundAmount()
	})
	sm.AddTransition(exampleStatePartialRefunded, exampleStateCanceled, exampleActionRefund, func(sc fsm.StateContext) bool {
		partialToCanceledCalls++
		return sc.(*exampleOrder).isFullyRefunded()
	})
	order := newExampleOrder()
	order.machine = fsm.MustFreeze(sm)
	_, err := order.Pay(42, "card")
	require.NoError(t, err)

	require.NoError(t, order.Refund(12))
	require.Equal(t, exampleStatePartialRefunded, order.CurrentState().Label())
	require.Equal(t, 1, paidToPartialCalls)
	require.Equal(t, 0, paidToCanceledCalls)
	require.Equal(t, 0, partialToPartialCalls)
	require.Equal(t, 0, partialToCanceledCalls)

	require.NoError(t, order.Refund(30))
	require.Equal(t, exampleStateCanceled, order.CurrentState().Label())
	require.Equal(t, 1, paidToPartialCalls)
	require.Equal(t, 0, paidToCanceledCalls)
	require.Equal(t, 1, partialToPartialCalls)
	require.Equal(t, 1, partialToCanceledCalls)
}

func TestOrderRefundTransitionsToCanceledWhenNothingRemains(t *testing.T) {
	order := newExampleOrder()
	_, err := order.Pay(42, "card")
	require.NoError(t, err)

	require.NoError(t, order.Refund(12))
	require.NoError(t, order.Refund(30))

	require.Equal(t, exampleStateCanceled, order.CurrentState().Label())
	require.Equal(t, 42, order.refundedAmount)
	require.Equal(t, 0, order.remainingAmount())
	require.False(t, order.canceled)
}

func TestPartialRefundedOrderCancelFullyRefundsAndCancels(t *testing.T) {
	order := newExampleOrder()
	_, err := order.Pay(42, "card")
	require.NoError(t, err)
	require.NoError(t, order.Refund(12))

	require.NoError(t, order.Cancel())

	require.Equal(t, exampleStateCanceled, order.CurrentState().Label())
	require.Equal(t, 42, order.refundedAmount)
	require.True(t, order.canceled)
}

func TestOrderRefundRejectsTooLargeAmountWithoutTransition(t *testing.T) {
	order := newExampleOrder()
	_, err := order.Pay(10, "cash")
	require.NoError(t, err)

	err = order.Refund(11)

	require.EqualError(t, err, "refund amount exceeds remaining amount")
	require.Equal(t, exampleStatePaid, order.CurrentState().Label())
	require.Equal(t, 0, order.refundedAmount)
}

// Intent: a state restored from a stored label needs no wiring to the order,
// because states receive the order as a parameter.
func TestOrderRestoredFromLabelWorksWithoutWiring(t *testing.T) {
	state, err := exampleOrderMachine.BuildState(exampleStatePaid)
	require.NoError(t, err)
	orderState, ok := state.(exampleOrderState)
	require.True(t, ok)
	order := &exampleOrder{
		machine:    exampleOrderMachine,
		state:      orderState,
		paidAmount: 42,
	}

	require.NoError(t, order.Refund(12))
	require.Equal(t, exampleStatePartialRefunded, order.CurrentState().Label())
	require.Equal(t, 12, order.refundedAmount)
}

// Intent: copying an order yields an independent order; behavior on the copy
// must not change the original through a shared state reference.
func TestCopiedOrderIsIndependent(t *testing.T) {
	original := newExampleOrder()
	_, err := original.Pay(42, "card")
	require.NoError(t, err)

	copied := *original
	require.NoError(t, copied.Refund(12))

	require.Equal(t, 12, copied.refundedAmount)
	require.Equal(t, 0, original.refundedAmount)
	require.Equal(t, exampleStatePaid, original.CurrentState().Label())
}

// Intent: MustFreeze fails fast on a miswired machine.
func TestMustFreezePanicsOnInvalidMachine(t *testing.T) {
	sm := fsm.NewStateMachine("must_freeze_invalid")
	require.NoError(t, sm.AddTransition(exampleStateUnpaid, exampleStatePaid, exampleActionPay, nil))

	require.PanicsWithValue(t,
		`fsm: state machine "must_freeze_invalid" is invalid: state machine check failed`,
		func() { fsm.MustFreeze(sm) })
}
