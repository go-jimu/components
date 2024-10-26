package fsm

type (
	// Factory define the factory interface.
	Factory interface {
		Register(StateMachine)
		Get(string) (StateMachine, bool)
	}

	// simpleStateFactory implements the Factory interface.
	simpleStateFactory struct {
		machines map[string]StateMachine
	}
)

var defaultFactory Factory

func NewStateFactory() Factory {
	return &simpleStateFactory{machines: make(map[string]StateMachine)}
}

// Register a new statemachine.
func (factory *simpleStateFactory) Register(sm StateMachine) {
	factory.machines[sm.Name()] = sm
}

// Get the state machine by its name.
func (factory *simpleStateFactory) Get(id string) (StateMachine, bool) {
	sm, ok := factory.machines[id]
	return sm, ok
}

func RegisterStateMachine(sm StateMachine) {
	defaultFactory.Register(sm)
}

func GetStateMachine(name string) (StateMachine, bool) {
	return defaultFactory.Get(name)
}

func MustGetStateMachine(name string) StateMachine {
	sm, ok := GetStateMachine(name)
	if !ok {
		panic("state machine not found")
	}
	return sm
}

func init() { //nolint:gochecknoinits // no reason
	defaultFactory = NewStateFactory()
}
