package mediator

var defaultMediator Mediator = nopMediator{}

type nopMediator struct{}

func (nopMediator) Dispatch(Event) error   { return ErrMediatorClosed }
func (nopMediator) Subscribe(EventHandler) {}

// SetDefault sets the default global mediator.
func SetDefault(m Mediator) {
	if m == nil {
		return
	}
	defaultMediator = m
}

func Default() Mediator {
	return defaultMediator
}

func Dispatch(ev Event) {
	defaultMediator.Dispatch(ev)
}

func Subscribe(hdl EventHandler) {
	defaultMediator.Subscribe(hdl)
}
