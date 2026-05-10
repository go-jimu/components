// Package outbox provides transaction-time recording and relay primitives for
// reliable integration message publishing.
//
// Record messages through Recorder inside the same transaction as the business
// write. Publish them through Relay after commit. Delivery is at-least-once, so
// consumers must deduplicate by message.Message.ID.
package outbox
