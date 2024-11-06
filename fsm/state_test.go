package fsm_test

import (
	"errors"
	"fmt"
	"log/slog"
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
	Items            []string
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

func NewShoppingCart(n int) *ShoppingCart {
	cart := &ShoppingCart{
		ID:               strconv.FormatInt(time.Now().UnixMicro(), 36),
		Items:            make([]string, 0, n),
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

func (sc *ShoppingCart) TransitionTo(state fsm.State, by fsm.Action) error {
	if sc.State.Label() == state.Label() {
		return nil
	}

	ss, ok := state.(ShoppingCartState)
	if !ok {
		return oops.Wrap(errors.New("state is not a ShoppingCartState"))
	}

	if !fsm.MustGetStateMachine(sc.StateMachineName).HasTransition(sc.State.Label(), by) {
		return oops.Wrap(fsm.NewTransitionError(sc.State.Label(), by))
	}

	original := sc.State.Label()
	current := state.Label()

	state.SetContext(sc)
	sc.State = ss

	sc.Events.Add(
		&EventTransitionState{
			From:           original,
			To:             current,
			Action:         by,
			ShoppingCartID: sc.ID,
		})
	return nil
}

func (sc *ShoppingCart) AddItem(items ...string) error {
	sm := fsm.MustGetStateMachine(sc.StateMachineName)
	if !sm.HasTransition(sc.State.Label(), ActionAdd) {
		return oops.Wrap(fsm.NewTransitionError(sc.State.Label(), ActionAdd))
	}

	if err := sc.State.AddItem(items...); err != nil {
		if tranErr := sm.TransitionToNext(sc, ActionFail); tranErr != nil {
			return oops.Join(err, tranErr)
		}
		return err
	}

	_ = sm.TransitionToNext(sc, ActionAdd)
	return nil
}

func (sc *ShoppingCart) Remove(_ ...string) error {
	sm := fsm.MustGetStateMachine(sc.StateMachineName)
	if !sm.HasTransition(sc.State.Label(), ActionRemove) {
		return oops.Wrap(fsm.NewTransitionError(sc.State.Label(), ActionRemove))
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
	if len(items) > 10 {
		return fmt.Errorf("items is too many")
	}
	sc.Items = append(sc.Items, items...)
	return nil
}

func TransitionToFully(sc fsm.StateContext) bool {
	cart := sc.(*ShoppingCart)
	slog.Info("check condition", slog.Int("cap", cap(cart.Items)), slog.Int("length", len(cart.Items)))
	return len(cart.Items) == 10
}

func TestStateMachine(t *testing.T) {
	sm := fsm.NewStateMachine("shopping_cart")
	fsm.RegisterStateMachine(sm)
	sm.RegisterStateBuilder(StateAddPending, NewAddPendingState)
	sm.RegisterStateBuilder(StateCheckedOut, func() fsm.State {
		return fsm.NewSimpleState(StateCheckedOut)
	})

	sm.AddTransition(StateAddPending, StateCheckoutPending, ActionCreate, nil)

	err := sm.Check()
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

	sm.AddTransition(StateAddPending, StateCheckoutPending, ActionAdd, nil)
	sm.AddTransition(StateAddPending, StateAddFailed, ActionFail, nil)

	sc := NewShoppingCart(10)
	assert.Equal(t, StateAddPending, sc.CurrentState().Label())
	err := sc.AddItem("a", "b", "c")
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

	sm.AddTransition(StateAddPending, StateCheckoutPending, ActionAdd, nil)
	sm.AddTransition(StateAddPending, StateAddFailed, ActionFail, nil)
	err := sm.Check()
	assert.NoError(t, err)

	sc := NewShoppingCart(10)
	assert.Equal(t, StateAddPending, sc.CurrentState().Label())

	items := make([]string, 0)
	for i := 0; i < 11; i++ {
		items = append(items, strconv.Itoa(i))
	}
	err = sc.AddItem(items...)
	assert.Error(t, err)
	assert.Equal(t, StateAddFailed, sc.CurrentState().Label())
}

func TestStateTransition_WithCondition(t *testing.T) {
	sm := fsm.NewStateMachine("shopping_cart")
	fsm.RegisterStateMachine(sm)
	sm.RegisterStateBuilder(StateAddPending, NewAddPendingState)
	sm.RegisterStateBuilder(StateCheckoutPending, NewCheckoutPendingState)

	sm.AddTransition(StateAddPending, StateCheckoutPending, ActionAdd, TransitionToFully)
	err := sm.Check()
	assert.NoError(t, err)

	sc := NewShoppingCart(10)
	assert.Equal(t, StateAddPending, sc.CurrentState().Label())
	err = sc.AddItem("a", "b", "c")
	assert.NoError(t, err)
	assert.Equal(t, StateAddPending, sc.CurrentState().Label())

	// add ten items
	err = sc.AddItem("d", "e", "f", "g", "h", "i", "j")
	assert.NoError(t, err)
	assert.Equal(t, StateCheckoutPending, sc.CurrentState().Label())
}
