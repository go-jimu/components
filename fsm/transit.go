package fsm

// Transit moves context along the first edge from its current state on action
// whose Condition matches, evaluating edges in the order they were added: it
// builds the target with sm.BuildState and hands it to context.SetState.
// Errors other than SetState's leave the state unchanged. Call it after the
// behavior succeeded; see "Rules for consumer code" and "Errors".
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
