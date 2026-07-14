package fsm_test

import (
	"errors"
	"testing"

	"github.com/go-jimu/components/fsm"
	"github.com/stretchr/testify/require"
)

func TestNewStateMachineRequiresName(t *testing.T) {
	require.PanicsWithValue(t, "name is required", func() {
		fsm.NewStateMachine("")
	})
}

func TestRegistryIgnoresNilAndReportsMissingMachine(t *testing.T) {
	require.Error(t, fsm.RegisterStateMachine(nil))

	sm, ok := fsm.GetStateMachine("registry_missing_machine")
	require.False(t, ok)
	require.Nil(t, sm)
	require.PanicsWithValue(t, "state machine not found", func() {
		fsm.MustGetStateMachine("registry_missing_machine")
	})
}

func TestRegistryRejectsInvalidAndDuplicateMachines(t *testing.T) {
	invalid := fsm.NewStateMachine("registry_invalid")
	require.NoError(t, invalid.AddTransition(exampleStateUnpaid, exampleStatePaid, exampleActionPay, nil))
	require.Error(t, fsm.RegisterStateMachine(invalid))

	first := fsm.NewStateMachine("registry_duplicate")
	require.NoError(t, first.RegisterStateBuilder(exampleStateUnpaid, newExampleUnpaidOrderState))
	require.NoError(t, first.RegisterStateBuilder(exampleStatePaid, newExamplePaidOrderState))
	require.NoError(t, first.AddTransition(exampleStateUnpaid, exampleStatePaid, exampleActionPay, nil))
	require.NoError(t, fsm.RegisterStateMachine(first))

	second := fsm.NewStateMachine("registry_duplicate")
	require.NoError(t, second.RegisterStateBuilder(exampleStateUnpaid, newExampleUnpaidOrderState))
	require.NoError(t, second.RegisterStateBuilder(exampleStateCanceled, newExampleCanceledOrderState))
	require.NoError(t, second.AddTransition(exampleStateUnpaid, exampleStateCanceled, exampleActionCancel, nil))
	require.Error(t, fsm.RegisterStateMachine(second))
	require.NoError(t, second.AddTransition(exampleStateCanceled, exampleStatePaid, exampleActionPay, nil))

	registered := fsm.MustGetStateMachine("registry_duplicate")
	transitions := registered.Transitions(exampleStateUnpaid, exampleActionPay)
	require.Len(t, transitions, 1)
	require.Equal(t, exampleStatePaid, transitions[0].To)
	require.False(t, registered.HasTransition(exampleStateUnpaid, exampleActionCancel))
}

type staticRuntimeMachine struct {
	name     string
	checkErr error
}

func (sm staticRuntimeMachine) Name() string {
	return sm.name
}

func (sm staticRuntimeMachine) HasTransition(fsm.StateLabel, fsm.Action) bool {
	return false
}

func (sm staticRuntimeMachine) Transitions(fsm.StateLabel, fsm.Action) []fsm.Transition {
	return nil
}

func (sm staticRuntimeMachine) BuildState(label fsm.StateLabel) (fsm.State, error) {
	return fsm.NewSimpleState(label), nil
}

func (sm staticRuntimeMachine) Check() error {
	return sm.checkErr
}

func TestRegistryAcceptsCheckedRuntimeMachineWithoutFreeze(t *testing.T) {
	require.Error(t, fsm.RegisterStateMachine(staticRuntimeMachine{}))

	checkErr := errors.New("custom check failed")
	require.ErrorIs(t, fsm.RegisterStateMachine(staticRuntimeMachine{
		name:     "registry_custom_invalid",
		checkErr: checkErr,
	}), checkErr)

	require.NoError(t, fsm.RegisterStateMachine(staticRuntimeMachine{name: "registry_custom_runtime"}))
	registered := fsm.MustGetStateMachine("registry_custom_runtime")
	require.Equal(t, "registry_custom_runtime", registered.Name())
}

func TestStateMachineHasTransitionDistinguishesFromAndAction(t *testing.T) {
	sm := fsm.NewStateMachine("has_transition")

	require.Error(t, sm.AddTransition("", exampleStatePaid, exampleActionPay, nil))
	require.Error(t, sm.AddTransition(exampleStateUnpaid, "", exampleActionPay, nil))
	require.Error(t, sm.AddTransition(exampleStateUnpaid, exampleStatePaid, "", nil))

	require.False(t, sm.HasTransition(exampleStateUnpaid, exampleActionPay))
	require.False(t, sm.HasTransition("order.unknown", exampleActionPay))

	require.NoError(t, sm.AddTransition(exampleStateUnpaid, exampleStatePaid, exampleActionPay, nil))
	require.True(t, sm.HasTransition(exampleStateUnpaid, exampleActionPay))
	require.False(t, sm.HasTransition(exampleStateUnpaid, exampleActionCancel))

	require.NoError(t, sm.AddTransition(exampleStateUnpaid, exampleStateCanceled, exampleActionCancel, nil))
	require.True(t, sm.HasTransition(exampleStateUnpaid, exampleActionCancel))
}

func TestStateMachineTransitionsReturnsCopy(t *testing.T) {
	sm := fsm.NewStateMachine("transition_copy")
	require.NoError(t, sm.AddTransition(exampleStatePaid, exampleStatePartialRefunded, exampleActionRefund, nil))
	require.NoError(t, sm.AddTransition(exampleStatePaid, exampleStateCanceled, exampleActionRefund, nil))

	transitions := sm.Transitions(exampleStatePaid, exampleActionRefund)
	require.Len(t, transitions, 2)
	transitions[0].To = exampleStateCanceled

	copiedAgain := sm.Transitions(exampleStatePaid, exampleActionRefund)
	require.Len(t, copiedAgain, 2)
	require.Equal(t, exampleStatePartialRefunded, copiedAgain[0].To)
	require.Equal(t, exampleStateCanceled, copiedAgain[1].To)
	require.Nil(t, sm.Transitions("order.unknown", exampleActionRefund))
	require.Nil(t, sm.Transitions(exampleStatePaid, exampleActionPay))
}

func TestStateMachineBuildStateReportsMissingAndNilBuilders(t *testing.T) {
	sm := fsm.NewStateMachine("build_state_errors")
	require.Error(t, sm.RegisterStateBuilder("", func() fsm.State {
		return fsm.NewSimpleState(exampleStatePaid)
	}))
	require.Error(t, sm.RegisterStateBuilder(exampleStatePaid, nil))

	state, err := sm.BuildState(exampleStatePaid)
	require.Nil(t, state)
	require.Error(t, err)
	require.EqualError(t, err, "state builder for order.paid not found")

	require.NoError(t, sm.RegisterStateBuilder(exampleStatePaid, func() fsm.State {
		return nil
	}))
	state, err = sm.BuildState(exampleStatePaid)
	require.Nil(t, state)
	require.Error(t, err)
	require.EqualError(t, err, "state builder for order.paid not found")
}

func TestStateMachineBuildStateRejectsMismatchedBuilderLabel(t *testing.T) {
	sm := fsm.NewStateMachine("build_state_mismatch")
	require.NoError(t, sm.RegisterStateBuilder(exampleStatePaid, func() fsm.State {
		return fsm.NewSimpleState(exampleStateCanceled)
	}))

	state, err := sm.BuildState(exampleStatePaid)

	require.Nil(t, state)
	require.Error(t, err)
	var builderErr *fsm.StateBuilderError
	require.ErrorAs(t, err, &builderErr)
	require.Equal(t, exampleStatePaid, builderErr.State)
	require.Equal(t, exampleStateCanceled, builderErr.Actual)
	require.EqualError(t, err, "state builder for order.paid returned state order.canceled")
}

func TestStateMachineCheckReportsInvalidBuilders(t *testing.T) {
	sm := fsm.NewStateMachine("check_invalid_builder")
	require.NoError(t, sm.RegisterStateBuilder(exampleStateUnpaid, newExampleUnpaidOrderState))
	require.NoError(t, sm.RegisterStateBuilder(exampleStatePaid, func() fsm.State {
		return fsm.NewSimpleState(exampleStateCanceled)
	}))
	require.NoError(t, sm.RegisterStateBuilder(exampleStateCanceled, func() fsm.State {
		return nil
	}))
	require.NoError(t, sm.AddTransition(exampleStateUnpaid, exampleStatePaid, exampleActionPay, nil))
	require.NoError(t, sm.AddTransition(exampleStatePaid, exampleStateCanceled, exampleActionCancel, nil))

	err := sm.Check()

	require.Error(t, err)
	var checkErr *fsm.StateMachineCheckError
	require.ErrorAs(t, err, &checkErr)
	require.Equal(t, []fsm.StateBuilderError{
		{State: exampleStateCanceled},
		{State: exampleStatePaid, Actual: exampleStateCanceled},
	}, checkErr.InvalidStateBuilders)
}

func TestStateMachineFreezeChecksAndPreventsMutation(t *testing.T) {
	sm := fsm.NewStateMachine("freeze")
	require.NoError(t, sm.RegisterStateBuilder(exampleStateUnpaid, newExampleUnpaidOrderState))
	require.NoError(t, sm.RegisterStateBuilder(exampleStatePaid, newExamplePaidOrderState))
	require.NoError(t, sm.AddTransition(exampleStateUnpaid, exampleStatePaid, exampleActionPay, nil))

	runtimeMachine, err := sm.Freeze()

	require.NoError(t, err)
	require.True(t, runtimeMachine.HasTransition(exampleStateUnpaid, exampleActionPay))
	require.Error(t, sm.AddTransition(exampleStatePaid, exampleStateCanceled, exampleActionCancel, nil))
	require.Error(t, sm.RegisterStateBuilder(exampleStateCanceled, newExampleCanceledOrderState))
	require.False(t, runtimeMachine.HasTransition(exampleStatePaid, exampleActionCancel))
}

func TestTransitReportsMissingTransitionWithoutChangingState(t *testing.T) {
	sm := fsm.NewStateMachine("missing_transition")
	context := newRecordingOrderContext(exampleStateUnpaid)

	err := fsm.Transit(context, sm, exampleActionPay)

	var transitionErr *fsm.TransitionError
	require.ErrorAs(t, err, &transitionErr)
	require.Equal(t, exampleStateUnpaid, transitionErr.Current)
	require.Equal(t, exampleActionPay, transitionErr.Action)
	require.EqualError(t, err, "transition from order.unpaid with action pay not found")
	require.Equal(t, exampleStateUnpaid, context.CurrentState().Label())
	require.Equal(t, 0, context.transitions)
}

type nilCurrentStateContext struct{}

func (context nilCurrentStateContext) CurrentState() fsm.State {
	return nil
}

func (context nilCurrentStateContext) SetState(fsm.State) error {
	return errors.New("must not set state")
}

func TestTransitRejectsNilInputs(t *testing.T) {
	sm := fsm.NewStateMachine("nil_inputs")

	require.EqualError(t, fsm.Transit(nil, sm, exampleActionPay), "state context is required")
	require.EqualError(t, fsm.Transit(newRecordingOrderContext(exampleStateUnpaid), nil, exampleActionPay), "state machine is required")
	require.EqualError(t, fsm.Transit(nilCurrentStateContext{}, sm, exampleActionPay), "current state is required")
}

type rejectingStateContext struct {
	current  fsm.State
	rejected fsm.State
	err      error
}

func newRejectingStateContext(label fsm.StateLabel, err error) *rejectingStateContext {
	state := fsm.NewSimpleState(label)
	context := &rejectingStateContext{
		current: state,
		err:     err,
	}
	state.SetContext(context)
	return context
}

func (context *rejectingStateContext) CurrentState() fsm.State {
	return context.current
}

func (context *rejectingStateContext) SetState(next fsm.State) error {
	context.rejected = next
	return context.err
}

func TestTransitPropagatesSetStateErrorAfterPreparingNextState(t *testing.T) {
	expectedErr := errors.New("reject next state")
	context := newRejectingStateContext(exampleStatePaid, expectedErr)
	sm := fsm.NewStateMachine("set_state_error")
	require.NoError(t, sm.RegisterStateBuilder(exampleStateCanceled, func() fsm.State {
		return fsm.NewSimpleState(exampleStateCanceled)
	}))
	require.NoError(t, sm.AddTransition(exampleStatePaid, exampleStateCanceled, exampleActionCancel, nil))

	err := fsm.Transit(context, sm, exampleActionCancel)

	require.ErrorIs(t, err, expectedErr)
	require.NotNil(t, context.rejected)
	require.Equal(t, exampleStateCanceled, context.rejected.Label())
	require.Same(t, context, context.rejected.Context())
	require.Equal(t, exampleStatePaid, context.CurrentState().Label())
}

func TestTransitUsesFirstMatchingTransition(t *testing.T) {
	var partialBuilderCalls int
	var canceledBuilderCalls int

	context := newRecordingOrderContext(exampleStatePaid)
	sm := fsm.NewStateMachine("first_matching_transition")
	require.NoError(t, sm.RegisterStateBuilder(exampleStatePartialRefunded, func() fsm.State {
		partialBuilderCalls++
		return fsm.NewSimpleState(exampleStatePartialRefunded)
	}))
	require.NoError(t, sm.RegisterStateBuilder(exampleStateCanceled, func() fsm.State {
		canceledBuilderCalls++
		return fsm.NewSimpleState(exampleStateCanceled)
	}))
	require.NoError(t, sm.AddTransition(exampleStatePaid, exampleStatePartialRefunded, exampleActionRefund, func(fsm.StateContext) bool {
		return true
	}))
	require.NoError(t, sm.AddTransition(exampleStatePaid, exampleStateCanceled, exampleActionRefund, func(fsm.StateContext) bool {
		return true
	}))

	err := fsm.Transit(context, sm, exampleActionRefund)

	require.NoError(t, err)
	require.Equal(t, exampleStatePartialRefunded, context.CurrentState().Label())
	require.Equal(t, 1, partialBuilderCalls)
	require.Equal(t, 0, canceledBuilderCalls)
	require.Equal(t, 1, context.transitions)
}
