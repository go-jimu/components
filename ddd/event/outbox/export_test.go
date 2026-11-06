package outbox

// Running reports whether o's Relay takes records, for tests that dispatch
// once it does.
func Running(o *Outbox) bool { return o.taking.Load() }
