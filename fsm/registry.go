package fsm

import "sync"

type stateMachineRegistry struct {
	mu       sync.RWMutex
	machines map[string]StateMachine
}

var defaultRegistry = &stateMachineRegistry{machines: make(map[string]StateMachine)}

func (registry *stateMachineRegistry) register(sm StateMachine) {
	if sm == nil {
		return
	}

	registry.mu.Lock()
	defer registry.mu.Unlock()

	registry.machines[sm.Name()] = sm
}

func (registry *stateMachineRegistry) get(name string) (StateMachine, bool) {
	registry.mu.RLock()
	defer registry.mu.RUnlock()

	sm, ok := registry.machines[name]
	return sm, ok
}

func RegisterStateMachine(sm StateMachine) {
	defaultRegistry.register(sm)
}

func GetStateMachine(name string) (StateMachine, bool) {
	return defaultRegistry.get(name)
}

func MustGetStateMachine(name string) StateMachine {
	sm, ok := GetStateMachine(name)
	if !ok {
		panic("state machine not found")
	}
	return sm
}
