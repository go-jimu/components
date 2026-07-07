package fsm_test

import (
	"testing"

	"github.com/go-jimu/components/fsm"
	"github.com/stretchr/testify/require"
)

type recordingOrderContext struct {
	current     fsm.State
	transitions int
}

func newRecordingOrderContext(label fsm.StateLabel) *recordingOrderContext {
	state := fsm.NewSimpleState(label)
	context := &recordingOrderContext{current: state}
	state.SetContext(context)
	return context
}

func (context *recordingOrderContext) CurrentState() fsm.State {
	return context.current
}

func (context *recordingOrderContext) SetState(next fsm.State) error {
	context.current = next
	context.transitions++
	return nil
}

func TestOrderTransitionReturnsErrorWhenTargetBuilderMissing(t *testing.T) {
	sm := fsm.NewStateMachine("missing_builder")
	sm.AddTransition(exampleStateUnpaid, exampleStatePaid, exampleActionPay, nil)
	context := newRecordingOrderContext(exampleStateUnpaid)

	var err error
	require.NotPanics(t, func() {
		err = fsm.Transit(context, sm, exampleActionPay)
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "state builder")
	require.Equal(t, exampleStateUnpaid, context.CurrentState().Label())
	require.Equal(t, 0, context.transitions)
}

func TestOrderTransitionKeepsStateWhenNoConditionMatches(t *testing.T) {
	sm := fsm.NewStateMachine("condition_noop")
	sm.RegisterStateBuilder(exampleStateCanceled, func() fsm.State {
		return fsm.NewSimpleState(exampleStateCanceled)
	})
	sm.AddTransition(exampleStatePaid, exampleStateCanceled, exampleActionRefund, func(fsm.StateContext) bool {
		return false
	})
	context := newRecordingOrderContext(exampleStatePaid)

	err := fsm.Transit(context, sm, exampleActionRefund)

	require.NoError(t, err)
	require.Equal(t, exampleStatePaid, context.CurrentState().Label())
	require.Equal(t, 0, context.transitions)
}

func TestOrderTransitionsReturnsConditionsWithoutCallingThem(t *testing.T) {
	var conditionCalls int
	sm := fsm.NewStateMachine("condition_is_context_responsibility")
	sm.AddTransition(exampleStatePaid, exampleStateCanceled, exampleActionRefund, func(fsm.StateContext) bool {
		conditionCalls++
		return true
	})

	transitions := sm.Transitions(exampleStatePaid, exampleActionRefund)

	require.Len(t, transitions, 1)
	require.Equal(t, exampleStateCanceled, transitions[0].To)
	require.NotNil(t, transitions[0].Condition)
	require.Equal(t, 0, conditionCalls)

	require.True(t, transitions[0].Condition(newRecordingOrderContext(exampleStatePaid)))
	require.Equal(t, 1, conditionCalls)
}
