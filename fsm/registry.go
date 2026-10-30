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

// RegisterStateMachine checks, freezes, and stores sm in a process-wide
// registry under its name.
//
// Deprecated: keep the machine in a package-level variable built with
// MustFreeze and pass it to Transit. A global registry hides the dependency
// behind a string name, fails only at runtime when the name is wrong or not
// yet registered, and shares state between tests.
func RegisterStateMachine(sm RuntimeStateMachine) error {
	return defaultRegistry.register(sm)
}

// GetStateMachine returns a machine stored by RegisterStateMachine.
//
// Deprecated: see RegisterStateMachine.
func GetStateMachine(name string) (RuntimeStateMachine, bool) {
	return defaultRegistry.get(name)
}

// MustGetStateMachine returns a machine stored by RegisterStateMachine and
// panics if none is stored under name.
//
// Deprecated: see RegisterStateMachine.
func MustGetStateMachine(name string) RuntimeStateMachine {
	sm, ok := GetStateMachine(name)
	if !ok {
		panic("state machine not found")
	}
	return sm
}
