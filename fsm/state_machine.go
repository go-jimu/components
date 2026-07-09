package fsm

import (
	"sort"
	"sync"
)

// simpleStateMachine is the default implementation of StateMachine.
type simpleStateMachine struct {
	mu          sync.RWMutex
	name        string
	transitions map[StateLabel]map[Action][]Transition
	builders    map[StateLabel]StateBuilder
	frozen      bool
}

func NewStateMachine(name string) StateMachine {
	if name == "" {
		panic("name is required")
	}
	return &simpleStateMachine{
		name:        name,
		transitions: make(map[StateLabel]map[Action][]Transition),
		builders:    make(map[StateLabel]StateBuilder),
	}
}

func (sm *simpleStateMachine) Name() string {
	return sm.name
}

func (sm *simpleStateMachine) AddTransition(from, to StateLabel, action Action, condition Condition) error {
	if from == "" {
		return newStateMachineDefinitionError("transition from state is required")
	}
	if to == "" {
		return newStateMachineDefinitionError("transition to state is required")
	}
	if action == "" {
		return newStateMachineDefinitionError("transition action is required")
	}

	sm.mu.Lock()
	defer sm.mu.Unlock()

	if sm.frozen {
		return newStateMachineDefinitionError("state machine is frozen")
	}

	if _, ok := sm.transitions[from]; !ok {
		sm.transitions[from] = make(map[Action][]Transition)
	}
	sm.transitions[from][action] = append(sm.transitions[from][action], Transition{To: to, Condition: condition})
	return nil
}

func (sm *simpleStateMachine) HasTransition(from StateLabel, action Action) bool {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	if nexts, ok := sm.transitions[from]; ok {
		if trans, ok2 := nexts[action]; ok2 && len(trans) > 0 {
			return true
		}
	}
	return false
}

func (sm *simpleStateMachine) Transitions(from StateLabel, action Action) []Transition {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	nexts, ok := sm.transitions[from]
	if !ok {
		return nil
	}

	trans := nexts[action]
	if len(trans) == 0 {
		return nil
	}

	copied := make([]Transition, len(trans))
	copy(copied, trans)
	return copied
}

func (sm *simpleStateMachine) BuildState(label StateLabel) (State, error) {
	builder, ok := sm.stateBuilder(label)
	if !ok {
		return nil, NewStateBuilderError(label)
	}

	state := builder()
	if state == nil {
		return nil, NewStateBuilderError(label)
	}
	if state.Label() != label {
		return nil, NewStateBuilderMismatchError(label, state.Label())
	}
	return state, nil
}

func (sm *simpleStateMachine) stateBuilder(label StateLabel) (StateBuilder, bool) {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	builder, ok := sm.builders[label]
	if !ok || builder == nil {
		return nil, false
	}
	return builder, true
}

func (sm *simpleStateMachine) RegisterStateBuilder(label StateLabel, builder StateBuilder) error {
	if label == "" {
		return newStateMachineDefinitionError("state builder label is required")
	}
	if builder == nil {
		return newStateMachineDefinitionError("state builder is required")
	}

	sm.mu.Lock()
	defer sm.mu.Unlock()

	if sm.frozen {
		return newStateMachineDefinitionError("state machine is frozen")
	}

	sm.builders[label] = builder
	return nil
}

func (sm *simpleStateMachine) Check() error {
	transitions, builders := sm.snapshot()

	missingStateBuilderSet := make(map[StateLabel]struct{})
	for from, nexts := range transitions {
		missingStateBuilderSet[from] = struct{}{}
		for _, next := range nexts {
			for _, tran := range next {
				missingStateBuilderSet[tran.To] = struct{}{}
			}
		}
	}

	unreferencedStateBuilders := make([]StateLabel, 0)
	for label := range builders {
		_, exists := missingStateBuilderSet[label]
		if exists {
			delete(missingStateBuilderSet, label)
			continue
		}
		unreferencedStateBuilders = append(unreferencedStateBuilders, label)
	}

	missingStateBuilders := make([]StateLabel, 0, len(missingStateBuilderSet))
	for label := range missingStateBuilderSet {
		missingStateBuilders = append(missingStateBuilders, label)
	}
	sortStateLabels(missingStateBuilders)
	sortStateLabels(unreferencedStateBuilders)

	invalidStateBuilders := make([]StateBuilderError, 0)
	for label, builder := range builders {
		state := builder()
		if state == nil {
			invalidStateBuilders = append(invalidStateBuilders, *NewStateBuilderError(label))
			continue
		}
		if state.Label() != label {
			invalidStateBuilders = append(invalidStateBuilders, *NewStateBuilderMismatchError(label, state.Label()))
		}
	}
	sortStateBuilderErrors(invalidStateBuilders)

	if len(missingStateBuilders) > 0 || len(unreferencedStateBuilders) > 0 || len(invalidStateBuilders) > 0 {
		return &StateMachineCheckError{
			MissingStateBuilders:      missingStateBuilders,
			UnreferencedStateBuilders: unreferencedStateBuilders,
			InvalidStateBuilders:      invalidStateBuilders,
		}
	}
	return nil
}

func (sm *simpleStateMachine) snapshot() (map[StateLabel]map[Action][]Transition, map[StateLabel]StateBuilder) {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	transitions := make(map[StateLabel]map[Action][]Transition, len(sm.transitions))
	for from, nexts := range sm.transitions {
		transitions[from] = make(map[Action][]Transition, len(nexts))
		for action, edges := range nexts {
			copied := make([]Transition, len(edges))
			copy(copied, edges)
			transitions[from][action] = copied
		}
	}

	builders := make(map[StateLabel]StateBuilder, len(sm.builders))
	for label, builder := range sm.builders {
		builders[label] = builder
	}
	return transitions, builders
}

func (sm *simpleStateMachine) Freeze() (RuntimeStateMachine, error) {
	if err := sm.Check(); err != nil {
		return nil, err
	}

	sm.mu.Lock()
	defer sm.mu.Unlock()
	sm.frozen = true
	return sm, nil
}

func sortStateBuilderErrors(errors []StateBuilderError) {
	sort.Slice(errors, func(i, j int) bool {
		return errors[i].State < errors[j].State
	})
}

func sortStateLabels(labels []StateLabel) {
	sort.Slice(labels, func(i, j int) bool {
		return labels[i] < labels[j]
	})
}
