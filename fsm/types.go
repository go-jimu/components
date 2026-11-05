package fsm

type (
	// Action names a business operation that can move an object between
	// states, such as "pay".
	Action string

	// StateLabel identifies a state. It is the value to persist and restore
	// with BuildState.
	StateLabel string

	// State is one state of a business object. It holds no reference to the
	// object: behavior methods on the consumer's state interface take the
	// object as a parameter, so states stay independent of any one instance.
	State interface {
		Label() StateLabel // get the state label
	}

	// StateBuilder returns a new state whose Label is the label it is
	// registered under.
	StateBuilder func() State

	// Transition defines a transition edge.
	Transition struct {
		To        StateLabel
		Condition Condition
	}

	// StateContext is implemented by the business object. CurrentState
	// returns its state; SetState type-checks next, stores it, and may record
	// domain events. Transit calls SetState only after an edge matched.
	StateContext interface {
		CurrentState() State       // get the current state
		SetState(next State) error // replace the current state
	}

	// Condition guards one transition edge. Transit calls it with the
	// business object after the behavior succeeded; a nil Condition always
	// matches.
	Condition func(StateContext) bool

	// RuntimeStateMachine is the read-only state machine surface used at runtime.
	RuntimeStateMachine interface {
		Name() string                                // get the state machine name
		HasTransition(StateLabel, Action) bool       // check if has transition from a state by action
		Transitions(StateLabel, Action) []Transition // get transition edges from a state by action
		BuildState(StateLabel) (State, error)        // build a state by label
		Check() error                                // check the completeness of States and Transitions
	}

	// StateMachine is the build-time state machine surface.
	StateMachine interface {
		RuntimeStateMachine
		AddTransition(StateLabel, StateLabel, Action, Condition) error // add a transition
		RegisterStateBuilder(StateLabel, StateBuilder) error           // register a state
		Freeze() (RuntimeStateMachine, error)                          // validate and make the machine read-only
	}
)
