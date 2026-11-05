package fsm

import "fmt"

type (
	// SimpleState is a reusable State implementation for concrete state structs
	// that only need label storage.
	SimpleState struct {
		label StateLabel
	}

	// TransitionError reports that no transition could be taken for the current
	// state/action. Guarded is false when no edge is registered, and true when
	// edges exist but none of their conditions matched.
	TransitionError struct {
		Action  Action
		Current StateLabel
		Guarded bool
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

// NewSimpleState returns a SimpleState with label. Concrete states embed it,
// usually through the consumer's base state, to implement State.
func NewSimpleState(label StateLabel) *SimpleState {
	return &SimpleState{
		label: label,
	}
}

// Label returns the state's label.
func (s *SimpleState) Label() StateLabel {
	return s.label
}

// NewTransitionError returns a TransitionError for an action with no edge from
// current. Guarded is false.
func NewTransitionError(current StateLabel, action Action) *TransitionError {
	return &TransitionError{
		Action:  action,
		Current: current,
	}
}

func newGuardedTransitionError(current StateLabel, action Action) *TransitionError {
	return &TransitionError{
		Action:  action,
		Current: current,
		Guarded: true,
	}
}

// Error describes the missing or unmatched transition.
func (te *TransitionError) Error() string {
	if te.Guarded {
		return fmt.Sprintf("no transition condition matched from %s with action %s", te.Current, te.Action)
	}
	return fmt.Sprintf("transition from %s with action %s not found", te.Current, te.Action)
}

// NewStateBuilderError returns a StateBuilderError for a state whose builder
// is missing or returned nil.
func NewStateBuilderError(state StateLabel) *StateBuilderError {
	return &StateBuilderError{
		State: state,
	}
}

// NewStateBuilderMismatchError returns a StateBuilderError for a builder of
// expected that returned a state labeled actual.
func NewStateBuilderMismatchError(expected, actual StateLabel) *StateBuilderError {
	return &StateBuilderError{
		State:  expected,
		Actual: actual,
	}
}

// Error describes the builder problem.
func (sbe *StateBuilderError) Error() string {
	if sbe.Actual != "" {
		return fmt.Sprintf("state builder for %s returned state %s", sbe.State, sbe.Actual)
	}
	return fmt.Sprintf("state builder for %s not found", sbe.State)
}

// Error returns a fixed summary; the fields list each problem.
func (err *StateMachineCheckError) Error() string {
	return "state machine check failed"
}

func newStateMachineDefinitionError(message string) *StateMachineDefinitionError {
	return &StateMachineDefinitionError{Message: message}
}

// Error returns Message.
func (err *StateMachineDefinitionError) Error() string {
	return err.Message
}
