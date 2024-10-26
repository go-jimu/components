package fsm_test

import (
	"fmt"
	"testing"

	"github.com/go-jimu/components/fsm"
	"github.com/samber/oops"
	"github.com/stretchr/testify/assert"
)

// ShoppingCart is StateContext implementation.
type ShoppingCart struct {
	ID               string
	StateMachineName string
	State            ShoppingCartState
	Items            [10]string
	Version          int
}

type (
	ShoppingCartState interface {
		fsm.State
		AddItem() error
		Remove() error
		Checkout() error
		Succeed() // eg. add_pending -> adding -> pay_pending
		Fail()    // eg. add_pending -> adding -> add_failed
	}

	BaseShoppingCartState struct {
		*fsm.SimpleState
	}

	EmptyState struct {
		BaseShoppingCartState
	}

	ItemAddedState struct {
		BaseShoppingCartState
	}
)

var (
	ActionMarkAsSucceed fsm.Action = "MARK_AS_SUCCEED"
	ActionMarkAsFail    fsm.Action = "MARK_AS_FAIL"
	ActionCreate        fsm.Action = "CREATE"
	ActionAdd           fsm.Action = "ADD"
	ActionRemove        fsm.Action = "REMOVE"
	ActionCheckout      fsm.Action = "CHECKOUT"
)

const (
	StateEmpty      fsm.StateLabel = "state.empty"
	StateItemAdded  fsm.StateLabel = "state.added"
	StateCheckedOut fsm.StateLabel = "state.checkedout"
)

func NewShoppingCart() *ShoppingCart {
	state := NewEmptyState()
	cart := &ShoppingCart{
		Items:            [10]string{},
		StateMachineName: "shopping_cart",
	}
	cart.TransitionTo(state)
	return cart
}

func (sc *ShoppingCart) CurrentState() fsm.State {
	return sc.State
}

func (sc *ShoppingCart) TransitionTo(state fsm.State) {
	state.SetContext(sc)
	sc.State = state.(ShoppingCartState)
}

func (sc *ShoppingCart) AddItem(_ ...string) error {
	sm := fsm.MustGetStateMachine(sc.StateMachineName)
	if !sm.HasTransition(sc.State.Label(), ActionAdd) {
		return fsm.NewTransitionError(sc.State.Label(), ActionAdd)
	}
	if err := sc.State.AddItem(); err != nil {
		return err
	}

	_ = sm.TransitionToNext(sc, ActionAdd)
	return nil
}

func (sc *ShoppingCart) Remove(_ ...string) error {
	sm := fsm.MustGetStateMachine(sc.StateMachineName)
	if !sm.HasTransition(sc.State.Label(), ActionRemove) {
		return fsm.NewTransitionError(sc.State.Label(), ActionRemove)
	}
	if err := sc.State.Remove(); err != nil {
		return err
	}

	_ = sm.TransitionToNext(sc, ActionRemove)
	return nil
}

func (base *BaseShoppingCartState) AddItem() error {
	return fsm.NewTransitionError(base.Label(), ActionAdd)
}

func (base *BaseShoppingCartState) Remove() error {
	return fsm.NewTransitionError(base.Label(), ActionRemove)
}

func (base *BaseShoppingCartState) Checkout() error {
	return fsm.NewTransitionError(base.Label(), ActionCheckout)
}

func (base *BaseShoppingCartState) Succeed() {
}

func (base *BaseShoppingCartState) Fail() {
}

func NewEmptyState() fsm.State {
	base := BaseShoppingCartState{fsm.NewSimpleState(StateEmpty)}
	return &EmptyState{
		BaseShoppingCartState: base,
	}
}

func NewItemAddedState() fsm.State {
	base := BaseShoppingCartState{fsm.NewSimpleState(StateItemAdded)}
	return &ItemAddedState{
		BaseShoppingCartState: base,
	}
}

// AddItem is a valid transition from EmptyState.
func (state *EmptyState) AddItem() error {
	_, ok := state.Context().(*ShoppingCart)
	if !ok {
		return fmt.Errorf("context is not a *ShoppingCart")
	}
	return nil
}

func TestStateMachine(t *testing.T) {
	sm := fsm.NewStateMachine("shopping_cart")
	fsm.RegisterStateMachine(sm)
	sm.RegisterStateBuilder(StateEmpty, NewEmptyState)
	sm.RegisterStateBuilder(StateCheckedOut, func() fsm.State {
		return fsm.NewSimpleState(StateCheckedOut)
	})

	err := sm.AddTransition(StateEmpty, StateItemAdded, ActionCreate)
	assert.NoError(t, err)

	err = sm.Check()
	assert.Error(t, err)
	oopsErr := err.(oops.OopsError)
	assert.EqualValues(t,
		oopsErr.Context(),
		map[string]any{
			"missed_state_builders": []fsm.StateLabel{StateItemAdded},
			"missed_transitions":    []fsm.StateLabel{StateCheckedOut},
		},
	)
}

func TestStateContext(t *testing.T) {
	sm := fsm.NewStateMachine("shopping_cart")
	fsm.RegisterStateMachine(sm)
	sm.RegisterStateBuilder(StateEmpty, NewEmptyState)
	sm.RegisterStateBuilder(StateItemAdded, NewItemAddedState)

	err := sm.AddTransition(StateEmpty, StateItemAdded, ActionAdd)
	assert.NoError(t, err)

	sc := NewShoppingCart()
	assert.Equal(t, StateEmpty, sc.CurrentState().Label())
	err = sc.AddItem()
	assert.NoError(t, err)

	err = sc.Remove()
	assert.Error(t, err)
}
