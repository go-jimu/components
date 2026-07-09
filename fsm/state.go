package fsm

import "fmt"

type (
	// SimpleState is a reusable State implementation for concrete state structs
	// that only need context and label storage.
	SimpleState struct {
		context StateContext
		label   StateLabel
	}

	// TransitionError reports a missing transition for the current state/action.
	TransitionError struct {
		Action  Action
		Current StateLabel
	}

	// StateBuilderError reports a missing, nil, or label-mismatched state builder.
	StateBuilderError struct {
		State  StateLabel
		Actual StateLabel
	}

	// StateMachineCheckError reports state-machine wiring problems found by Check.
	StateMachineCheckError struct {
		MissingStateBuilders      []StateLabel
		UnreferencedStateBuilders []StateLabel
		InvalidStateBuilders      []StateBuilderError
	}

	// StateMachineDefinitionError reports invalid state-machine setup input.
	StateMachineDefinitionError struct {
		Message string
	}
)

var _ State = (*SimpleState)(nil)

func NewSimpleState(label StateLabel) *SimpleState {
	return &SimpleState{
		label: label,
	}
}

func (s *SimpleState) Label() StateLabel {
	return s.label
}

func (s *SimpleState) Context() StateContext {
	return s.context
}

func (s *SimpleState) SetContext(context StateContext) {
	s.context = context
}

func NewTransitionError(current StateLabel, action Action) *TransitionError {
	return &TransitionError{
		Action:  action,
		Current: current,
	}
}

func (te *TransitionError) Error() string {
	return fmt.Sprintf("transition from %s with action %s not found", te.Current, te.Action)
}

func NewStateBuilderError(state StateLabel) *StateBuilderError {
	return &StateBuilderError{
		State: state,
	}
}

func NewStateBuilderMismatchError(expected, actual StateLabel) *StateBuilderError {
	return &StateBuilderError{
		State:  expected,
		Actual: actual,
	}
}

func (sbe *StateBuilderError) Error() string {
	if sbe.Actual != "" {
		return fmt.Sprintf("state builder for %s returned state %s", sbe.State, sbe.Actual)
	}
	return fmt.Sprintf("state builder for %s not found", sbe.State)
}

func (err *StateMachineCheckError) Error() string {
	return "state machine check failed"
}

func newStateMachineDefinitionError(message string) *StateMachineDefinitionError {
	return &StateMachineDefinitionError{Message: message}
}

func (err *StateMachineDefinitionError) Error() string {
	return err.Message
}
