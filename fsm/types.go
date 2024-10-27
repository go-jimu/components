package fsm

type (
	// Action define the action type.
	Action string

	// StateLabel define the state type.
	StateLabel string

	// State define the state interface.
	State interface {
		SetContext(StateContext) // set the state context
		Context() StateContext
		Label() StateLabel // get the state label
	}

	// StateBuilder define the state builder interface.
	StateBuilder func() State

	// StateContext define the state context interface.
	StateContext interface {
		CurrentState() State                      // get the current state
		TransitionTo(next State, by Action) error // transition to the next state
	}

	// StateMachine define the state machine interface.
	StateMachine interface {
		Name() string                                              // get the state machine name
		AddTransition(from, to StateLabel, action Action) error    // add a transition
		HasTransition(from StateLabel, action Action) bool         // check if has transition from one state to another
		TransitionToNext(StateContext, Action) error               // transition to the next state
		Check() error                                              // Check the completeness of States and Transitions.
		RegisterStateBuilder(label StateLabel, state StateBuilder) // register a state
	}
)
