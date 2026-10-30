// Package validation provides small validation helpers.
//
// Notification collects validation errors so all of them are reported at
// once. Its Err returns nil when nothing was added, and otherwise an *ErrChain
// that keeps the original errors: errors.Is and errors.As match any of them,
// and ErrChain.Errors lists them in order, for example to report failures per
// field.
//
// Specification models a reusable boolean predicate, combined with And, Or,
// Not, All, Any, and their negated forms.
package validation
