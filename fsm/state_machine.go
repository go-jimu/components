package fsm

import (
	"errors"
	"sync"

	"github.com/samber/oops"
)

// simpleStateMachine is the default implementation of StateMachine.
type simpleStateMachine struct {
	mu          sync.RWMutex
	name        string
	transitions map[StateLabel]map[Action][]transition
	builders    map[StateLabel]StateBuilder
}

type transition struct {
	to        StateLabel
	condition Condition
}

func NewStateMachine(name string) StateMachine {
	if name == "" {
		panic("name is required")
	}
	return &simpleStateMachine{
		name:        name,
		transitions: make(map[StateLabel]map[Action][]transition),
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
		sm.transitions[from] = make(map[Action][]transition)
		sm.transitions[from][action] = []transition{{to: to, condition: condition}}
		return
	}
	if _, ok := sm.transitions[from][action]; !ok {
		sm.transitions[from][action] = []transition{{to: to, condition: condition}}
		return
	}
	sm.transitions[from][action] = append(sm.transitions[from][action], transition{to: to, condition: condition})
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

func (sm *simpleStateMachine) TransitionToNext(sc StateContext, action Action) error {
	if !sm.HasTransition(sc.CurrentState().Label(), action) {
		return NewTransitionError(sc.CurrentState().Label(), action)
	}

	var next StateLabel
	sm.mu.RLock()

	trans := sm.transitions[sc.CurrentState().Label()][action]
	for _, tran := range trans {
		if tran.condition == nil {
			next = tran.to
			break
		}
		if tran.condition(sc) {
			next = tran.to
			break
		}
	}
	if next != "" {
		builder := sm.builders[next]
		sm.mu.RUnlock()
		return sc.TransitionTo(builder(), action)
	}
	sm.mu.RUnlock()
	return nil
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
				missedStateBuilder[tran.to] = struct{}{}
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
			errWrap = errWrap.With("missed_state_builders", builders)
		}
		if len(missedTransition) > 0 {
			errWrap = errWrap.With("missed_transitions", missedTransition)
		}
		return errWrap.Wrap(errors.New("state machine check failed"))
	}
	return nil
}
