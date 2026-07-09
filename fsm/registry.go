package fsm

import "sync"

type stateMachineRegistry struct {
	mu       sync.RWMutex
	machines map[string]RuntimeStateMachine
}

var defaultRegistry = &stateMachineRegistry{machines: make(map[string]RuntimeStateMachine)}

func (registry *stateMachineRegistry) register(sm RuntimeStateMachine) error {
	if sm == nil {
		return newStateMachineDefinitionError("state machine is required")
	}
	if sm.Name() == "" {
		return newStateMachineDefinitionError("state machine name is required")
	}

	registry.mu.Lock()
	defer registry.mu.Unlock()

	if _, exists := registry.machines[sm.Name()]; exists {
		return newStateMachineDefinitionError("state machine already registered")
	}

	if freezer, ok := sm.(interface {
		Freeze() (RuntimeStateMachine, error)
	}); ok {
		frozen, err := freezer.Freeze()
		if err != nil {
			return err
		}
		sm = frozen
	} else if err := sm.Check(); err != nil {
		return err
	}

	registry.machines[sm.Name()] = sm
	return nil
}

func (registry *stateMachineRegistry) get(name string) (RuntimeStateMachine, bool) {
	registry.mu.RLock()
	defer registry.mu.RUnlock()

	sm, ok := registry.machines[name]
	return sm, ok
}

func RegisterStateMachine(sm RuntimeStateMachine) error {
	return defaultRegistry.register(sm)
}

func GetStateMachine(name string) (RuntimeStateMachine, bool) {
	return defaultRegistry.get(name)
}

func MustGetStateMachine(name string) RuntimeStateMachine {
	sm, ok := GetStateMachine(name)
	if !ok {
		panic("state machine not found")
	}
	return sm
}
