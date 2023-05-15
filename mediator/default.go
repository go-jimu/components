package mediator

var defaultMediator Mediator

func SetDefault(m Mediator) {
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
