package fsm

// Transit executes the standard transition flow for a StateContext.
//
// It returns a *TransitionError when no edge is registered for the current
// state and action, or when edges exist but none of their conditions match. To
// stay in the current state, register an explicit self-transition.
func Transit(context StateContext, sm RuntimeStateMachine, action Action) error {
	if context == nil {
		return newStateMachineDefinitionError("state context is required")
	}
	if sm == nil {
		return newStateMachineDefinitionError("state machine is required")
	}

	current := context.CurrentState()
	if current == nil {
		return newStateMachineDefinitionError("current state is required")
	}

	transitions := sm.Transitions(current.Label(), action)
	if len(transitions) == 0 {
		return NewTransitionError(current.Label(), action)
	}

	for _, transition := range transitions {
		if transition.Condition != nil && !transition.Condition(context) {
			continue
		}

		next, err := sm.BuildState(transition.To)
		if err != nil {
			return err
		}

		return context.SetState(next)
	}
	return newGuardedTransitionError(current.Label(), action)
}
