package fsm

type (
	// Action define the action type.
	Action string

	// StateLabel define the state type.
	StateLabel string

	// State define the state interface.
	State interface {
		SetContext(StateContext) // set the state context
		Context() StateContext   // return the state context
		Label() StateLabel       // get the state label
	}

	// StateBuilder define the state builder interface.
	StateBuilder func() State

	// Transition defines a transition edge.
	Transition struct {
		To        StateLabel
		Condition Condition
	}

	// StateContext define the state context interface.
	StateContext interface {
		CurrentState() State       // get the current state
		SetState(next State) error // replace the current state
	}

	// Condition guards whether a specific transition edge is allowed.
	Condition func(StateContext) bool

	// StateMachine define the state machine interface.
	StateMachine interface {
		Name() string                                            // get the state machine name
		AddTransition(StateLabel, StateLabel, Action, Condition) // add a transition
		HasTransition(StateLabel, Action) bool                   // check if has transition from a state by action
		Transitions(StateLabel, Action) []Transition             // get transition edges from a state by action
		BuildState(StateLabel) (State, error)                    // build a state by label
		Check() error                                            // Check the completeness of States and Transitions.
		RegisterStateBuilder(StateLabel, StateBuilder)           // register a state
	}
)
