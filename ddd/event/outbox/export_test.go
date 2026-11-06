package outbox

// Running reports whether o's Relay runs, for tests that dispatch once it
// does.
func Running(o *Outbox) bool { return o.running.Load() }
