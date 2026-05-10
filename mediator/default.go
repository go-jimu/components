package mediator

var defaultMediator Mediator = nopMediator{}

type nopMediator struct{}

func (nopMediator) Dispatch(Event) error   { return ErrMediatorClosed }
func (nopMediator) Subscribe(EventHandler) {}

// SetDefault sets the default global mediator.
//
// Deprecated: use explicit github.com/go-jimu/components/ddd/event.Dispatcher
// instances for new domain event code.
func SetDefault(m Mediator) {
	if m == nil {
		return
	}
	defaultMediator = m
}

// Deprecated: use explicit github.com/go-jimu/components/ddd/event.Dispatcher
// instances for new domain event code.
func Default() Mediator {
	return defaultMediator
}

// Deprecated: use explicit github.com/go-jimu/components/ddd/event.Dispatcher
// instances for new domain event code.
func Dispatch(ev Event) {
	defaultMediator.Dispatch(ev)
}

// Deprecated: use explicit github.com/go-jimu/components/ddd/event.Dispatcher
// instances for new domain event code.
func Subscribe(hdl EventHandler) {
	defaultMediator.Subscribe(hdl)
}
