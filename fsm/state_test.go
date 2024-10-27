package fsm_test

import (
	"fmt"
	"strconv"
	"testing"
	"time"

	"github.com/go-jimu/components/fsm"
	"github.com/go-jimu/components/mediator"
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
	Events           mediator.EventCollection
}

type (
	ShoppingCartState interface {
		fsm.State
		AddItem(...string) error
		Remove() error
		Checkout() error
		Fail() // eg. add_pending -> adding -> add_failed
	}

	BaseShoppingCartState struct {
		*fsm.SimpleState
	}

	AddPendingState struct {
		BaseShoppingCartState
	}

	AddFailedState struct {
		BaseShoppingCartState
	}

	CheckoutPendingState struct {
		BaseShoppingCartState
	}
)

type (
	EventTransitionState struct {
		From           fsm.StateLabel
		To             fsm.StateLabel
		Action         fsm.Action
		ShoppingCartID string
	}
)

var (
	ActionFail     fsm.Action = "FAIL"
	ActionCreate   fsm.Action = "CREATE"
	ActionAdd      fsm.Action = "ADD"
	ActionRemove   fsm.Action = "REMOVE"
	ActionCheckout fsm.Action = "CHECKOUT"
)

const (
	StateAddPending      fsm.StateLabel = "state.add_pending"
	StateAddFailed       fsm.StateLabel = "state.add_failed"
	StateCheckoutPending fsm.StateLabel = "state.checkout_pending"
	StateCheckedOut      fsm.StateLabel = "state.checkedout"
)

var (
	EventKindTransition mediator.EventKind = "event.kind.transition"
)

func (ev *EventTransitionState) Kind() mediator.EventKind {
	return EventKindTransition
}

func NewShoppingCart() *ShoppingCart {
	cart := &ShoppingCart{
		ID:               strconv.FormatInt(time.Now().UnixMicro(), 36),
		Items:            [10]string{},
		StateMachineName: "shopping_cart",
		State:            NewAddPendingState().(ShoppingCartState),
		Events:           mediator.NewEventCollection(),
	}
	cart.State.SetContext(cart)
	return cart
}

func (sc *ShoppingCart) CurrentState() fsm.State {
	return sc.State
}

func (sc *ShoppingCart) TransitionTo(state fsm.State, by fsm.Action) {
	original := sc.State.Label()
	current := state.Label()
	state.SetContext(sc)
	sc.State = state.(ShoppingCartState)

	sc.Events.Add(
		&EventTransitionState{
			From:           original,
			To:             current,
			Action:         by,
			ShoppingCartID: sc.ID,
		})
}

func (sc *ShoppingCart) AddItem(items ...string) error {
	sm := fsm.MustGetStateMachine(sc.StateMachineName)
	if !sm.HasTransition(sc.State.Label(), ActionAdd) {
		return fsm.NewTransitionError(sc.State.Label(), ActionAdd)
	}
	if err := sc.State.AddItem(items...); err != nil {
		sm.TransitionToNext(sc, ActionFail) // Added_Failed
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

func (base *BaseShoppingCartState) AddItem(...string) error {
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

func NewAddPendingState() fsm.State {
	base := BaseShoppingCartState{fsm.NewSimpleState(StateAddPending)}
	return &AddPendingState{
		BaseShoppingCartState: base,
	}
}

func NewAddFailedState() fsm.State {
	base := BaseShoppingCartState{fsm.NewSimpleState(StateAddFailed)}
	return &AddFailedState{
		BaseShoppingCartState: base,
	}
}

func NewCheckoutPendingState() fsm.State {
	base := BaseShoppingCartState{fsm.NewSimpleState(StateCheckoutPending)}
	return &CheckoutPendingState{
		BaseShoppingCartState: base,
	}
}

// AddItem is a valid transition from EmptyState.
func (state *AddPendingState) AddItem(items ...string) error {
	sc, ok := state.Context().(*ShoppingCart)
	if !ok {
		return fmt.Errorf("context is not a *ShoppingCart")
	}
	if len(items) > cap(sc.Items) {
		return fmt.Errorf("items is too many")
	}
	copy(sc.Items[0:len(sc.Items)], items)
	return nil
}

func TestStateMachine(t *testing.T) {
	sm := fsm.NewStateMachine("shopping_cart")
	fsm.RegisterStateMachine(sm)
	sm.RegisterStateBuilder(StateAddPending, NewAddPendingState)
	sm.RegisterStateBuilder(StateCheckedOut, func() fsm.State {
		return fsm.NewSimpleState(StateCheckedOut)
	})

	err := sm.AddTransition(StateAddPending, StateCheckoutPending, ActionCreate)
	assert.NoError(t, err)

	err = sm.Check()
	assert.Error(t, err)
	oopsErr := err.(oops.OopsError)
	assert.EqualValues(t,
		oopsErr.Context(),
		map[string]any{
			"missed_state_builders": []fsm.StateLabel{StateCheckoutPending},
			"missed_transitions":    []fsm.StateLabel{StateCheckedOut},
		},
	)
}

func TestStateContext(t *testing.T) {
	sm := fsm.NewStateMachine("shopping_cart")
	fsm.RegisterStateMachine(sm)
	sm.RegisterStateBuilder(StateAddPending, NewAddPendingState)
	sm.RegisterStateBuilder(StateCheckoutPending, NewCheckoutPendingState)

	err := sm.AddTransition(StateAddPending, StateCheckoutPending, ActionAdd)
	assert.NoError(t, err)
	err = sm.AddTransition(StateAddPending, StateAddFailed, ActionFail)
	assert.NoError(t, err)

	sc := NewShoppingCart()
	assert.Equal(t, StateAddPending, sc.CurrentState().Label())
	err = sc.AddItem("a", "b", "c")
	assert.NoError(t, err)
	assert.Equal(t, StateCheckoutPending, sc.CurrentState().Label())
	t.Log(sc.Items)

	err = sc.Remove()
	assert.Error(t, err)
}

func TestStateTransition_HandleFail(t *testing.T) {
	sm := fsm.NewStateMachine("shopping_cart")
	fsm.RegisterStateMachine(sm)
	sm.RegisterStateBuilder(StateAddPending, NewAddPendingState)
	sm.RegisterStateBuilder(StateCheckoutPending, NewCheckoutPendingState)
	sm.RegisterStateBuilder(StateAddFailed, NewAddFailedState)

	err := sm.AddTransition(StateAddPending, StateCheckoutPending, ActionAdd)
	assert.NoError(t, err)
	err = sm.AddTransition(StateAddPending, StateAddFailed, ActionFail)
	assert.NoError(t, err)
	err = sm.Check()
	assert.NoError(t, err)

	sc := NewShoppingCart()
	assert.Equal(t, StateAddPending, sc.CurrentState().Label())

	items := make([]string, 0)
	for i := 0; i <= cap(sc.Items); i++ {
		items = append(items, strconv.Itoa(i))
	}
	err = sc.AddItem(items...)
	assert.Error(t, err)
	assert.Equal(t, StateAddFailed, sc.CurrentState().Label())
}
