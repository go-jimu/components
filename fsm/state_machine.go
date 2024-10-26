package fsm

import (
	"errors"
	"fmt"
	"sync"

	"github.com/samber/oops"
)

// simpleStateMachine is the default implementation of StateMachine.
type simpleStateMachine struct {
	mu          sync.RWMutex
	name        string
	transitions map[StateLabel]map[Action]StateLabel
	builders    map[StateLabel]StateBuilder
}

func NewStateMachine(name string) StateMachine {
	if name == "" {
		panic("name is required")
	}
	return &simpleStateMachine{
		name:        name,
		transitions: make(map[StateLabel]map[Action]StateLabel),
		builders:    make(map[StateLabel]StateBuilder),
	}
}

func (sm *simpleStateMachine) Name() string {
	return sm.name
}

func (sm *simpleStateMachine) AddTransition(from, to StateLabel, action Action) error {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	nexts, ok := sm.transitions[from]
	if !ok {
		nexts = make(map[Action]StateLabel)
		sm.transitions[from] = nexts
	}
	conflict, ok := nexts[action]
	if ok {
		return fmt.Errorf("transition already exists: %s -(%s)-> %s", from, action, conflict)
	}
	nexts[action] = to
	return nil
}

func (sm *simpleStateMachine) HasTransition(from StateLabel, action Action) bool {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	if nexts, ok := sm.transitions[from]; ok {
		if _, ok = nexts[action]; ok {
			return true
		}
	}
	return false
}

func (sm *simpleStateMachine) TransitionToNext(from StateLabel, action Action) (State, bool) {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	if nexts, ok := sm.transitions[from]; ok {
		if to, ok := nexts[action]; ok {
			if builder, ok := sm.builders[to]; ok {
				return builder(), true
			}
		}
	}
	return nil, false
}

func (sm *simpleStateMachine) RegisterStateBuilder(label StateLabel, builder StateBuilder) {
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
			missedStateBuilder[next] = struct{}{}
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
			errWrap = errWrap.With("missed_state_builders", builders)
		}
		if len(missedTransition) > 0 {
			errWrap = errWrap.With("missed_transitions", missedTransition)
		}
		return errWrap.Wrap(errors.New("state machine check failed"))
	}
	return nil
}
