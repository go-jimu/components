package fsm_test

import (
	"testing"

	"github.com/go-jimu/components/fsm"
	"github.com/samber/oops"
	"github.com/stretchr/testify/require"
)

func registerExampleOrderStateMachine(t *testing.T) {
	t.Helper()

	sm := newExampleOrderStateMachine()
	require.NoError(t, sm.Check())
	fsm.RegisterStateMachine(sm)
}

func TestStateMachineCheckReportsOrderWiringErrors(t *testing.T) {
	sm := fsm.NewStateMachine("order_check")
	sm.RegisterStateBuilder(exampleStateUnpaid, newExampleUnpaidOrderState)
	sm.RegisterStateBuilder(exampleStateCanceled, newExampleCanceledOrderState)
	sm.AddTransition(exampleStateUnpaid, exampleStatePaid, exampleActionPay, nil)

	err := sm.Check()

	require.Error(t, err)
	oopsErr := err.(oops.OopsError)
	require.EqualValues(t,
		map[string]any{
			"missed_state_builders": []fsm.StateLabel{exampleStatePaid},
			"missed_transitions":    []fsm.StateLabel{exampleStateCanceled},
		},
		oopsErr.Context(),
	)
}

func TestOrderPayDelegatesToUnpaidStateThenTransitionsToPaid(t *testing.T) {
	registerExampleOrderStateMachine(t)
	order := newExampleOrder()

	receipt, err := order.Pay(42, "card")

	require.NoError(t, err)
	require.Equal(t, "receipt:card:42", receipt)
	require.Equal(t, exampleStatePaid, order.CurrentState().Label())
	require.Equal(t, 42, order.paidAmount)
	require.Equal(t, "card", order.paymentMethod)
}

func TestPaidOrderRejectsSecondPayFromStateMethod(t *testing.T) {
	registerExampleOrderStateMachine(t)
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
	registerExampleOrderStateMachine(t)
	order := newExampleOrder()

	err := order.Cancel()

	require.EqualError(t, err, "order.unpaid cannot cancel")
	require.Equal(t, exampleStateUnpaid, order.CurrentState().Label())
	require.False(t, order.canceled)
}

func TestOrderRefundCanStayPartialAcrossMultipleRefunds(t *testing.T) {
	registerExampleOrderStateMachine(t)
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
	require.NoError(t, sm.Check())
	fsm.RegisterStateMachine(sm)

	order := newExampleOrder()
	order.stateMachineName = machineName
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
	registerExampleOrderStateMachine(t)
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
	registerExampleOrderStateMachine(t)
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
	registerExampleOrderStateMachine(t)
	order := newExampleOrder()
	_, err := order.Pay(10, "cash")
	require.NoError(t, err)

	err = order.Refund(11)

	require.EqualError(t, err, "refund amount exceeds remaining amount")
	require.Equal(t, exampleStatePaid, order.CurrentState().Label())
	require.Equal(t, 0, order.refundedAmount)
}
