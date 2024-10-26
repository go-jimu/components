package fsm

import "fmt"

type (
	SimpleState struct {
		context StateContext
		label   StateLabel
	}

	TransitionError struct {
		Action  Action
		Current StateLabel
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
