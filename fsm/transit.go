package fsm

// Transit executes the standard transition flow for a StateContext.
func Transit(context StateContext, sm StateMachine, action Action) error {
	transitions := sm.Transitions(context.CurrentState().Label(), action)
	if len(transitions) == 0 {
		return NewTransitionError(context.CurrentState().Label(), action)
	}

	for _, transition := range transitions {
		if transition.Condition != nil && !transition.Condition(context) {
			continue
		}

		next, err := sm.BuildState(transition.To)
		if err != nil {
			return err
		}

		next.SetContext(context)
		return context.SetState(next)
	}
	return nil
}
