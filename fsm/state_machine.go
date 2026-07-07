package fsm

import (
	"errors"
	"sort"
	"sync"

	"github.com/samber/oops"
)

// simpleStateMachine is the default implementation of StateMachine.
type simpleStateMachine struct {
	mu          sync.RWMutex
	name        string
	transitions map[StateLabel]map[Action][]Transition
	builders    map[StateLabel]StateBuilder
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

func (sm *simpleStateMachine) AddTransition(from, to StateLabel, action Action, condition Condition) {
	if from == "" || to == "" || action == "" {
		return
	}

	sm.mu.Lock()
	defer sm.mu.Unlock()

	if _, ok := sm.transitions[from]; !ok {
		sm.transitions[from] = make(map[Action][]Transition)
		sm.transitions[from][action] = []Transition{{To: to, Condition: condition}}
		return
	}
	if _, ok := sm.transitions[from][action]; !ok {
		sm.transitions[from][action] = []Transition{{To: to, Condition: condition}}
		return
	}
	sm.transitions[from][action] = append(sm.transitions[from][action], Transition{To: to, Condition: condition})
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

func (sm *simpleStateMachine) RegisterStateBuilder(label StateLabel, builder StateBuilder) {
	if label == "" || builder == nil {
		return
	}

	sm.mu.Lock()
	defer sm.mu.Unlock()

	sm.builders[label] = builder
}

func (sm *simpleStateMachine) Check() error {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	missedTransition := make([]StateLabel, 0)
	missedStateBuilder := make(map[StateLabel]struct{})

	for from, nexts := range sm.transitions {
		missedStateBuilder[from] = struct{}{}
		for _, next := range nexts {
			for _, tran := range next {
				missedStateBuilder[tran.To] = struct{}{}
			}
		}
	}

	for label := range sm.builders {
		_, exists := missedStateBuilder[label]
		if exists {
			delete(missedStateBuilder, label)
			continue
		}
		missedTransition = append(missedTransition, label)
	}

	if len(missedStateBuilder) > 0 || len(missedTransition) > 0 {
		var errWrap oops.OopsErrorBuilder
		if len(missedStateBuilder) > 0 {
			builders := make([]StateLabel, 0, len(missedStateBuilder))
			for builder := range missedStateBuilder {
				builders = append(builders, builder)
			}
			sortStateLabels(builders)
			errWrap = errWrap.With("missed_state_builders", builders)
		}
		if len(missedTransition) > 0 {
			sortStateLabels(missedTransition)
			errWrap = errWrap.With("missed_transitions", missedTransition)
		}
		return errWrap.Wrap(errors.New("state machine check failed"))
	}
	return nil
}

func sortStateLabels(labels []StateLabel) {
	sort.Slice(labels, func(i, j int) bool {
		return labels[i] < labels[j]
	})
}
