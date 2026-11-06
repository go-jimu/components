package fsm

type (
	// Action names a business operation that can move an object between
	// states, such as "pay".
	Action string

	// StateLabel identifies a state. It is the value to persist and restore
	// with BuildState.
	StateLabel string

	// State is one state of a business object. The consumer's state
	// interface embeds it; see "Rules for consumer code".
	State interface {
		Label() StateLabel
	}

	// StateBuilder returns a new state whose Label is the label it is
	// registered under.
	StateBuilder func() State

	// Transition is one edge out of a from + action pair: the target state and
	// its guard.
	Transition struct {
		To        StateLabel
		Condition Condition
	}

	// StateContext is implemented by the business object. CurrentState
	// returns its state; SetState type-checks next, stores it, and may record
	// domain events. Transit calls SetState only after an edge matched.
	StateContext interface {
		CurrentState() State
		SetState(next State) error
	}

	// Condition guards one transition edge. Transit calls it with the
	// business object after the behavior succeeded; a nil Condition always
	// matches.
	Condition func(StateContext) bool

	// RuntimeStateMachine is the read-only surface Transit uses. Transitions
	// returns a copy of the edges from a state on an action, in the order they
	// were added. BuildState returns a new state from the builder registered
	// for a label. Check reports wiring problems: a state referenced by an
	// edge without a builder, a builder no edge references, or a builder that
	// returns nil or a state with another label. See "Errors".
	RuntimeStateMachine interface {
		Name() string
		HasTransition(StateLabel, Action) bool
		Transitions(StateLabel, Action) []Transition
		BuildState(StateLabel) (State, error)
		Check() error
	}

	// StateMachine is the build-time surface returned by NewStateMachine.
	// AddTransition(from, to, action, condition) appends an edge; a nil
	// condition always matches. RegisterStateBuilder replaces any builder
	// already registered for the label. Freeze runs Check, then rejects
	// further AddTransition and RegisterStateBuilder calls. See "Errors".
	StateMachine interface {
		RuntimeStateMachine
		AddTransition(StateLabel, StateLabel, Action, Condition) error
		RegisterStateBuilder(StateLabel, StateBuilder) error
		Freeze() (RuntimeStateMachine, error)
	}
)
